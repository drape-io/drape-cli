package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/drape-io/drape-cli/internal/cidetect"
)

func TestBuildBatchJoinRequest_RequiresTotalShardsWhenShardKeySet(t *testing.T) {
	_, err := buildBatchJoinRequest(
		&cidetect.CIInfo{RunAttempt: 1},
		batchJoinFlags{ShardKey: "ci-42", TotalShards: 0},
		1, "main", "abc", map[string]any{},
	)
	if err == nil || !strings.Contains(err.Error(), "--shard-key requires --total-shards") {
		t.Fatalf("expected --shard-key/--total-shards error, got: %v", err)
	}
}

func TestBuildBatchJoinRequest_RejectsTotalShardsOne(t *testing.T) {
	_, err := buildBatchJoinRequest(
		&cidetect.CIInfo{ProviderRunID: "42", RunAttempt: 1},
		batchJoinFlags{TotalShards: 1},
		1, "main", "abc", map[string]any{},
	)
	if err == nil || !strings.Contains(err.Error(), "--total-shards must be >= 2") {
		t.Fatalf("expected --total-shards >= 2 error, got: %v", err)
	}
}

func TestBuildBatchJoinRequest_RejectsNegativeTotalShards(t *testing.T) {
	_, err := buildBatchJoinRequest(
		&cidetect.CIInfo{ProviderRunID: "42", RunAttempt: 1},
		batchJoinFlags{TotalShards: -3},
		1, "main", "abc", map[string]any{},
	)
	if err == nil || !strings.Contains(err.Error(), "--total-shards must be >= 2") {
		t.Fatalf("expected --total-shards >= 2 error for negative value, got: %v", err)
	}
}

func TestBuildBatchJoinRequest_RejectsFilesGreaterThanShards(t *testing.T) {
	_, err := buildBatchJoinRequest(
		&cidetect.CIInfo{ProviderRunID: "42", RunAttempt: 1},
		batchJoinFlags{TotalShards: 2},
		3, "main", "abc", map[string]any{},
	)
	if err == nil || !strings.Contains(err.Error(), "Did you mean --total-shards=3") {
		t.Fatalf("expected files>shards error with suggestion, got: %v", err)
	}
}

func TestBuildBatchJoinRequest_RequiresShardKeyFromFlagOrEnv(t *testing.T) {
	_, err := buildBatchJoinRequest(
		&cidetect.CIInfo{RunAttempt: 1}, // no ProviderRunID
		batchJoinFlags{TotalShards: 3},
		1, "main", "abc", map[string]any{},
	)
	if err == nil || !strings.Contains(err.Error(), "--total-shards requires a shard key") {
		t.Fatalf("expected missing-shard-key error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "GITHUB_RUN_ID") || !strings.Contains(err.Error(), "--shard-key") {
		t.Fatalf("error should mention both auto-detect source and flag fallback: %v", err)
	}
}

func TestBuildBatchJoinRequest_AutoDerivesFromGitHubEnv(t *testing.T) {
	ci := &cidetect.CIInfo{
		Provider:      "github-actions",
		ProviderRunID: "42",
		RunAttempt:    1,
	}
	metadata := map[string]any{"group": "python"}

	req, err := buildBatchJoinRequest(ci, batchJoinFlags{TotalShards: 3, Groups: []string{"python"}}, 1, "main", "abc", metadata)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.ProviderRunID != "42" {
		t.Errorf("ProviderRunID = %q; want %q", req.ProviderRunID, "42")
	}
	// Wire shape: RunAttempt must be an int so JSON encodes it as an integer, not a string.
	if req.RunAttempt != 1 {
		t.Errorf("RunAttempt = %d; want 1", req.RunAttempt)
	}
	if g, _ := metadata["group"].(string); g != "python" {
		t.Errorf("metadata[group] = %q; want %q", g, "python")
	}
	if req.ExpectedCount != 3 {
		t.Errorf("ExpectedCount = %d; want 3", req.ExpectedCount)
	}
	if req.UploadType != "coverage" {
		t.Errorf("UploadType = %q; want %q", req.UploadType, "coverage")
	}
}

func TestBuildBatchJoinRequest_ShardKeyOverridesEnv(t *testing.T) {
	ci := &cidetect.CIInfo{ProviderRunID: "env-value", RunAttempt: 1}
	req, err := buildBatchJoinRequest(
		ci,
		batchJoinFlags{ShardKey: "flag-value", TotalShards: 3},
		1, "main", "abc", map[string]any{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.ProviderRunID != "flag-value" {
		t.Errorf("ProviderRunID = %q; want %q (flag should override env)", req.ProviderRunID, "flag-value")
	}
}

func TestBuildBatchJoinRequest_GroupsSortedForCommutativity(t *testing.T) {
	ci := &cidetect.CIInfo{ProviderRunID: "42", RunAttempt: 1}

	m1 := map[string]any{}
	_, err := buildBatchJoinRequest(ci, batchJoinFlags{TotalShards: 3, Groups: []string{"b", "a"}}, 1, "main", "abc", m1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m2 := map[string]any{}
	_, err = buildBatchJoinRequest(ci, batchJoinFlags{TotalShards: 3, Groups: []string{"a", "b"}}, 1, "main", "abc", m2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m1["group"] != m2["group"] {
		t.Errorf("group key differs across orderings: %q vs %q (must be commutative)", m1["group"], m2["group"])
	}
	if m1["group"] != "a,b" {
		t.Errorf("group = %q; want %q (sorted+joined)", m1["group"], "a,b")
	}
}

func TestBuildBatchJoinRequest_FailsOnRunAttemptParseError(t *testing.T) {
	ci := &cidetect.CIInfo{
		ProviderRunID: "42",
		RunAttempt:    1, // defaulted because env was garbage
		RunAttemptErr: errors.New(`GITHUB_RUN_ATTEMPT="garbage" is not a positive integer`),
	}
	_, err := buildBatchJoinRequest(
		ci,
		batchJoinFlags{TotalShards: 3},
		1, "main", "abc", map[string]any{},
	)
	if err == nil {
		t.Fatal("expected error for garbage GITHUB_RUN_ATTEMPT, got nil")
	}
	if !strings.Contains(err.Error(), "GITHUB_RUN_ATTEMPT") || !strings.Contains(err.Error(), "positive integer") {
		t.Errorf("error should mention the env var and fix-it guidance: %v", err)
	}
}

func TestBuildBatchJoinRequest_CoexistsWithDrapeRunID(t *testing.T) {
	// Simulate what buildCoverageMetadata would have written: applyDrapeRunIDMetadata
	// wrote run_id and a drape-prefixed group default since --group was absent.
	metadata := map[string]any{
		"run_id": "drape-foo",
		"group":  "drape:drape-foo",
	}
	ci := &cidetect.CIInfo{RunAttempt: 1}

	req, err := buildBatchJoinRequest(
		ci,
		batchJoinFlags{ShardKey: "ci-42", TotalShards: 3 /* Groups is empty — user only set --drape-run-id */},
		1, "main", "abc", metadata,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.ProviderRunID != "ci-42" {
		t.Errorf("top-level provider_run_id = %q; want %q (--shard-key should win)", req.ProviderRunID, "ci-42")
	}
	if metadata["run_id"] != "drape-foo" {
		t.Errorf("metadata[run_id] was overwritten: %q", metadata["run_id"])
	}
	// The drape-prefixed group stays — join doesn't clobber applyDrapeRunIDMetadata's default.
	if metadata["group"] != "drape:drape-foo" {
		t.Errorf("metadata[group] changed unexpectedly: %q", metadata["group"])
	}
}

func TestBuildBatchJoinRequest_NilCIInfoWithShardKey(t *testing.T) {
	// Non-CI invocation (e.g. local testing): ci can be nil.
	// --shard-key must still work as the explicit override.
	req, err := buildBatchJoinRequest(
		nil,
		batchJoinFlags{ShardKey: "local-1", TotalShards: 3},
		1, "main", "abc", map[string]any{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.ProviderRunID != "local-1" {
		t.Errorf("ProviderRunID = %q; want %q", req.ProviderRunID, "local-1")
	}
	if req.RunAttempt != 1 {
		t.Errorf("RunAttempt = %d; want 1 (default when CI absent)", req.RunAttempt)
	}
}
