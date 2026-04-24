package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/drape-io/drape-cli/internal/api"
	"github.com/drape-io/drape-cli/internal/output"
)

// captureStderr routes output.Warn / output.Error to a buffer for the test's
// duration, then restores the default stderr writer.
func captureStderr(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	output.Stderr = &buf
	t.Cleanup(output.Reset)
	return &buf
}

// completedBatchStatus builds a minimal *api.CoverageBatchStatusResponse
// with the common fields populated. Tests override only what they exercise.
func completedBatchStatus(partial *bool) *api.CoverageBatchStatusResponse {
	rate := "85.50"
	count := 42
	return &api.CoverageBatchStatusResponse{
		BatchID:        1,
		Status:         "completed",
		ExpectedCount:  5,
		CompletedCount: 3,
		CoverageResult: api.CoverageResult{
			CoverageRate: &rate,
			FileCount:    &count,
			Partial:      partial,
		},
	}
}

func TestRenderBatchResult_PartialFlagWarnsButExitsZero(t *testing.T) {
	stderr := captureStderr(t)
	truePtr := true
	err := renderBatchResult(UploadResult{FilesMatched: 3}, completedBatchStatus(&truePtr), 0)

	if err != nil {
		t.Fatalf("expected nil error (partial is informational), got: %v", err)
	}

	got := stderr.String()
	wantFragments := []string{
		"Partial coverage: 3 of 5 shards finalized",
		"The batch was published with partial data",
		"sibling CI job failed",
		"sibling job is still running",
		"Shards disagreed on --total-shards",
		"re-run the failed jobs",
	}
	for _, want := range wantFragments {
		if !strings.Contains(got, want) {
			t.Errorf("stderr missing %q\nfull stderr:\n%s", want, got)
		}
	}
}

func TestRenderBatchResult_NonPartialDoesNotWarn(t *testing.T) {
	stderr := captureStderr(t)
	falsePtr := false
	err := renderBatchResult(UploadResult{FilesMatched: 3}, completedBatchStatus(&falsePtr), 0)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(stderr.String(), "Partial coverage") {
		t.Errorf("partial=false should not print partial warning; got:\n%s", stderr.String())
	}
}

func TestRenderBatchResult_AbsentPartialDoesNotWarn(t *testing.T) {
	// Older server responses (or non-batch paths) omit the field entirely.
	stderr := captureStderr(t)
	err := renderBatchResult(UploadResult{FilesMatched: 3}, completedBatchStatus(nil), 0)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(stderr.String(), "Partial coverage") {
		t.Errorf("absent partial field should not print warning; got:\n%s", stderr.String())
	}
}

