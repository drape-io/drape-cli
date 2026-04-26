package api

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// withFastPoll shrinks the poll backoff so tests can exercise multiple iterations
// without sleeping a real second. Returns a cleanup func.
func withFastPoll(t *testing.T) {
	t.Helper()
	origInitial, origMax := pollInitialInterval, pollMaxInterval
	pollInitialInterval = 1 * time.Millisecond
	pollMaxInterval = 5 * time.Millisecond
	t.Cleanup(func() {
		pollInitialInterval = origInitial
		pollMaxInterval = origMax
	})
}

func TestPollWithBackoff_LastPollCatchesSuccess(t *testing.T) {
	// Regression: the deadline check must run AFTER the fetch, so a batch
	// that finalizes during our sleep window still wins. Old code returned
	// "timed out" here even though the next fetch would have seen "completed".
	//
	// Sleep interval (50ms) >> timeout (10ms), so the deadline elapses during
	// the sleep. Old (deadline-check-first) code returns a timeout error here;
	// new code catches "completed" on the second fetch.
	origInitial, origMax := pollInitialInterval, pollMaxInterval
	pollInitialInterval = 50 * time.Millisecond
	pollMaxInterval = 50 * time.Millisecond
	t.Cleanup(func() {
		pollInitialInterval = origInitial
		pollMaxInterval = origMax
	})

	calls := 0
	fetchFn := func() (*pollResult, error) {
		calls++
		if calls == 1 {
			return &pollResult{Status: "running"}, nil
		}
		return &pollResult{Status: "completed"}, nil
	}

	c := &Client{}
	err := c.pollWithBackoff(10*time.Millisecond, "Batch", fetchFn)
	if err != nil {
		t.Fatalf("expected nil err (last-poll caught completed), got %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 fetches (running, completed), got %d", calls)
	}
}

func TestPollWithBackoff_TimeoutMessageIncludesConfiguredValue(t *testing.T) {
	// Status never finalizes; we expect the configured timeout in the error.
	withFastPoll(t)

	fetchFn := func() (*pollResult, error) {
		return &pollResult{Status: "running"}, nil
	}

	c := &Client{}
	err := c.pollWithBackoff(2*time.Millisecond, "Batch", fetchFn)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	want := "timed out waiting for Batch processing after 2ms"
	if got := err.Error(); got != want {
		t.Errorf("error message = %q; want %q", got, want)
	}
}

func TestPollWithBackoff_FailedStatusReturnsError(t *testing.T) {
	withFastPoll(t)

	msg := "schema validation failed"
	fetchFn := func() (*pollResult, error) {
		return &pollResult{Status: "failed", ErrorMessage: &msg}, nil
	}

	c := &Client{}
	err := c.pollWithBackoff(time.Second, "Coverage", fetchFn)
	if err == nil {
		t.Fatal("expected error for failed status, got nil")
	}
	want := "Coverage processing failed: schema validation failed"
	if got := err.Error(); got != want {
		t.Errorf("error = %q; want %q", got, want)
	}
}

func TestPollWithBackoff_FailedStatusWithoutMessage(t *testing.T) {
	withFastPoll(t)

	fetchFn := func() (*pollResult, error) {
		return &pollResult{Status: "failed"}, nil
	}

	c := &Client{}
	err := c.pollWithBackoff(time.Second, "Batch", fetchFn)
	if err == nil {
		t.Fatal("expected error for failed status, got nil")
	}
	if !strings.Contains(err.Error(), "unknown error") {
		t.Errorf("expected fallback 'unknown error' in message, got %q", err.Error())
	}
}

func TestPollWithBackoff_FetchErrorPropagates(t *testing.T) {
	withFastPoll(t)

	boom := errors.New("network is on fire")
	fetchFn := func() (*pollResult, error) {
		return nil, boom
	}

	c := &Client{}
	err := c.pollWithBackoff(time.Second, "Batch", fetchFn)
	if !errors.Is(err, boom) {
		t.Errorf("expected wrapped fetch error %v, got %v", boom, err)
	}
}
