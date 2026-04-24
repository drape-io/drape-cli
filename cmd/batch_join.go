package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/drape-io/drape-cli/internal/api"
	"github.com/drape-io/drape-cli/internal/cidetect"
)

// batchJoinFlags carries the flag values that drive batch-join mode.
// Packaged as a struct so buildBatchJoinRequest stays pure and testable
// without touching package-level flag globals.
type batchJoinFlags struct {
	ShardKey    string
	TotalShards int
	Groups      []string
}

// buildBatchJoinRequest validates flags, resolves the natural-key components
// from flags + CI env, and returns a fully-populated CoverageBatchRequest ready
// to POST. It mutates metadata["group"] to a stable, sorted joined form so the
// natural key matches across sibling shards regardless of --group order.
//
// Errors returned here carry exitcode.UsageError semantics — callers should
// wrap with ExitError{Code: exitcode.UsageError, Err: ...}.
func buildBatchJoinRequest(ci *cidetect.CIInfo, flags batchJoinFlags, fileCount int, branch, sha string, metadata map[string]any) (api.CoverageBatchRequest, error) {
	var req api.CoverageBatchRequest

	if flags.ShardKey != "" && flags.TotalShards == 0 {
		return req, fmt.Errorf("--shard-key requires --total-shards to indicate how many sibling shards will join this batch (e.g., --total-shards 5)")
	}
	if flags.TotalShards != 0 && flags.TotalShards < 2 {
		return req, fmt.Errorf("--total-shards must be >= 2 for batch mode; omit --total-shards for single-file uploads")
	}
	if flags.TotalShards > 0 && fileCount > flags.TotalShards {
		return req, fmt.Errorf("--total-shards=%d is less than the %d files provided locally. --total-shards must count all shards across all CI jobs, including the %d in this invocation. Did you mean --total-shards=%d or higher?",
			flags.TotalShards, fileCount, fileCount, fileCount)
	}

	shardKey := flags.ShardKey
	if shardKey == "" && ci != nil {
		shardKey = ci.ProviderRunID
	}
	if shardKey == "" {
		return req, fmt.Errorf("--total-shards requires a shard key shared across sibling jobs. In GitHub Actions this is auto-detected from GITHUB_RUN_ID. For other CI providers or local testing, pass --shard-key <your-ci-run-identifier>")
	}

	if ci != nil && ci.RunAttemptErr != nil {
		return req, fmt.Errorf("%w — batch mode requires a valid run attempt; unset GITHUB_RUN_ATTEMPT or set it to a positive integer", ci.RunAttemptErr)
	}

	// Sort groups for commutativity — shard A with "--group a --group b" must
	// match shard B with "--group b --group a". The server reads group from
	// metadata, so overwrite metadata["group"] with the sorted join.
	//
	// Note: applyDrapeRunIDMetadata (called upstream in buildCoverageMetadata)
	// may have already set metadata["group"] to "drape:{drape-run-id}" when
	// --drape-run-id was passed without --group. In that case flags.Groups is
	// empty here; we leave the drape-prefixed value alone (still stable across
	// shards that all pass the same --drape-run-id).
	if len(flags.Groups) > 0 {
		groups := append([]string(nil), flags.Groups...)
		sort.Strings(groups)
		metadata["group"] = strings.Join(groups, ",")
	}

	runAttempt := 1
	if ci != nil {
		runAttempt = ci.RunAttempt
	}

	req = api.CoverageBatchRequest{
		ExpectedCount: flags.TotalShards,
		UploadType:    "coverage",
		Branch:        branch,
		SHA:           sha,
		Metadata:      metadata,
		ProviderRunID: shardKey,
		RunAttempt:    runAttempt,
	}
	return req, nil
}
