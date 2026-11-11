package oidc

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func jwtExpiring(exp time.Time) string {
	seg := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	return seg(map[string]string{"alg": "RS256"}) + "." + seg(map[string]int64{"exp": exp.Unix()}) + ".sig"
}

type fakeGitHub struct {
	server    *httptest.Server
	requests  []*http.Request
	expiresIn time.Duration
	now       time.Time
	status    int
	failures  int // requests answered 503 before the first token
	sleeps    []time.Duration
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	f := &fakeGitHub{expiresIn: 5 * time.Minute, now: time.Unix(1_700_000_000, 0), status: 200}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests = append(f.requests, r)
		if len(f.requests) <= f.failures {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if f.status != 200 {
			w.WriteHeader(f.status)
			_, _ = w.Write([]byte(`{"message":"no id-token permission"}`))
			return
		}
		token := jwtExpiring(f.now.Add(f.expiresIn))
		_, _ = fmt.Fprintf(w, `{"value":%q}`, token)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeGitHub) source() *GitHubActions {
	src, ok := GitHubActionsFromEnv(func(key string) string {
		return map[string]string{
			"ACTIONS_ID_TOKEN_REQUEST_URL":   f.server.URL + "/token?api-version=2.0",
			"ACTIONS_ID_TOKEN_REQUEST_TOKEN": "runner-secret",
		}[key]
	}, "https://app.drape.io")
	if !ok {
		panic("expected a source")
	}
	src.Now = func() time.Time { return f.now }
	src.Sleep = func(d time.Duration) { f.sleeps = append(f.sleeps, d) }
	return src
}

func TestNoSourceWithoutTheIDTokenPermission(t *testing.T) {
	if _, ok := GitHubActionsFromEnv(func(string) string { return "" }, "aud"); ok {
		t.Fatal("expected no token source")
	}
}

func TestFetchesWithTheAudienceAddedToTheRequestURL(t *testing.T) {
	gh := newFakeGitHub(t)

	if _, err := gh.source().Token(); err != nil {
		t.Fatal(err)
	}

	r := gh.requests[0]
	// Escaped, not pasted: the audience is itself a URL.
	if !strings.Contains(r.URL.RawQuery, "audience=https%3A%2F%2Fapp.drape.io") {
		t.Errorf("raw query = %q", r.URL.RawQuery)
	}
	if got := r.URL.Query().Get("api-version"); got != "2.0" {
		t.Errorf("existing query lost: api-version = %q", got)
	}
	if got := r.Header.Get("Authorization"); got != "bearer runner-secret" {
		t.Errorf("Authorization = %q", got)
	}
}

func TestReusesATokenUntilAMinuteBeforeItExpires(t *testing.T) {
	gh := newFakeGitHub(t)
	src := gh.source()

	first, _ := src.Token()
	gh.now = gh.now.Add(3 * time.Minute) // two minutes left
	second, _ := src.Token()
	gh.now = gh.now.Add(90 * time.Second) // thirty seconds left
	third, _ := src.Token()

	if first != second {
		t.Error("expected the cached token while it has time left")
	}
	if third == second || len(gh.requests) != 2 {
		t.Errorf("expected a fresh token near expiry; %d requests", len(gh.requests))
	}
}

func TestGitHubRefusingIsAnErrorWithItsMessage(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.status = 403

	_, err := gh.source().Token()

	if err == nil || !strings.Contains(err.Error(), "no id-token permission") {
		t.Fatalf("err = %v", err)
	}
	if len(gh.requests) != 1 {
		t.Errorf("a refusal was retried: %d requests", len(gh.requests))
	}
}

func TestRetriesAGitHubServerError(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.failures = 1

	token, err := gh.source().Token()

	if err != nil || token == "" {
		t.Fatalf("token = %q, err = %v", token, err)
	}
	if len(gh.requests) != 2 || len(gh.sleeps) != 1 || gh.sleeps[0] != time.Second {
		t.Errorf("%d requests, sleeps %v", len(gh.requests), gh.sleeps)
	}
}

func TestGivesUpAfterThreeAttempts(t *testing.T) {
	gh := newFakeGitHub(t)
	gh.failures = 3

	_, err := gh.source().Token()

	if err == nil || !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("err = %v", err)
	}
	if len(gh.requests) != 3 || len(gh.sleeps) != 2 || gh.sleeps[1] != 2*time.Second {
		t.Errorf("%d requests, sleeps %v", len(gh.requests), gh.sleeps)
	}
}

func TestACutOffResponseIsAReadError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte(`{"value":`))
	}))
	t.Cleanup(server.Close)
	src, _ := GitHubActionsFromEnv(func(key string) string {
		return map[string]string{
			"ACTIONS_ID_TOKEN_REQUEST_URL":   server.URL,
			"ACTIONS_ID_TOKEN_REQUEST_TOKEN": "runner-secret",
		}[key]
	}, "https://app.drape.io")
	src.Sleep = func(time.Duration) {}

	_, err := src.Token()

	if err == nil || !strings.Contains(err.Error(), "reading GitHub's token response") {
		t.Fatalf("err = %v", err)
	}
}
