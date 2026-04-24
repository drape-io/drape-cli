package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/drape-io/drape-cli/internal/api"
	"github.com/drape-io/drape-cli/internal/cidetect"
	"github.com/drape-io/drape-cli/internal/exitcode"
	"github.com/drape-io/drape-cli/internal/output"
)

var (
	flagCovFormat       string
	flagCovPathPrefix   string
	flagCovTargetBranch string
	flagCovRunDate      string
	flagCovGroups       []string
	flagCovDrapeRunID   string
	flagCovShardKey     string
	flagCovTotalShards  int
)

var uploadCoverageCmd = &cobra.Command{
	Use:   "coverage <file> [file...]",
	Short: "Upload coverage report to Drape",
	Long: `Upload one or more coverage reports (Cobertura XML, LCOV, or Go coverage) to Drape for analysis.

When multiple files are provided, they are uploaded as a batch and merged
server-side into a single coverage report before the regression check runs.
Glob patterns are supported (e.g. "coverage/*.xml").`,
	Args: cobra.MinimumNArgs(1),
	RunE: runUploadCoverage,
}

func init() {
	uploadCoverageCmd.Flags().StringVar(&flagCovFormat, "format", "", "Coverage format: cobertura, lcov, go (required)")
	uploadCoverageCmd.Flags().StringVar(&flagCovPathPrefix, "path-prefix", "", "Path prefix mapping for repo structure")
	uploadCoverageCmd.Flags().StringVar(&flagCovTargetBranch, "target-branch", "", "Target branch for PR diff (auto-detected from CI)")
	uploadCoverageCmd.Flags().StringVar(&flagCovRunDate, "run-date", "", "ISO 8601 date for historical uploads (e.g. 2026-03-15)")
	uploadCoverageCmd.Flags().StringSliceVar(&flagCovGroups, "group", nil, "Group label(s) for this upload (can be specified multiple times)")
	uploadCoverageCmd.Flags().StringVar(&flagCovDrapeRunID, "drape-run-id", "", "Drape run ID to correlate triggered CI runs (env: DRAPE_RUN_ID)")
	uploadCoverageCmd.Flags().StringVar(&flagCovShardKey, "shard-key", "", "Shared identifier across sibling matrix shards (e.g., the CI provider's run ID). Auto-detected from GITHUB_RUN_ID in GitHub Actions.")
	uploadCoverageCmd.Flags().IntVar(&flagCovTotalShards, "total-shards", 0, "Total number of coverage shards across all CI jobs in this run. Enables server-side batch merging for matrix jobs. Must be 2 or greater.")

	_ = uploadCoverageCmd.MarkFlagRequired("format")

	uploadCmd.AddCommand(uploadCoverageCmd)
}

func runUploadCoverage(cmd *cobra.Command, args []string) error {
	files, err := expandGlobs(args)
	if err != nil {
		return &ExitError{Code: exitcode.UsageError, Err: err}
	}
	if len(files) == 0 {
		return &ExitError{Code: exitcode.ParseError, Err: fmt.Errorf("no files matched the given pattern(s)")}
	}

	output.Info("Found %d coverage file(s) to upload", len(files))

	ctx, err := newUploadContext()
	if err != nil {
		return err
	}

	if flagDryRun {
		ctx.dryRunSimple(files, flagCovFormat)
		return nil
	}

	if err := ctx.resolveClient(); err != nil {
		return err
	}

	metadata := buildCoverageMetadata(ctx)

	// Batch-join mode: --total-shards opts into the server's natural-key upsert,
	// letting sibling CI jobs (matrix shards) fan into one coverage batch.
	if flagCovTotalShards > 0 || flagCovShardKey != "" {
		req, err := buildBatchJoinRequest(ctx.ci, batchJoinFlags{
			ShardKey:    flagCovShardKey,
			TotalShards: flagCovTotalShards,
			Groups:      flagCovGroups,
		}, len(files), ctx.branch, ctx.sha, metadata)
		if err != nil {
			return &ExitError{Code: exitcode.UsageError, Err: err}
		}
		group, _ := metadata["group"].(string)
		output.Verbose("Batch natural key: provider_run_id=%q run_attempt=%d upload_type=coverage group=%q",
			req.ProviderRunID, req.RunAttempt, group)
		return runBatchCoverageUpload(ctx, files, metadata, batchOptions{
			expectedCount: flagCovTotalShards,
			naturalKey: &naturalKey{
				ProviderRunID: req.ProviderRunID,
				RunAttempt:    req.RunAttempt,
			},
			timeoutMultiplier:    1,
			failOnLocalUploadErr: false,
		})
	}

	if len(files) == 1 {
		return runSingleCoverageUpload(ctx, files[0], metadata)
	}
	return runBatchCoverageUpload(ctx, files, metadata, batchOptions{
		expectedCount:        len(files),
		timeoutMultiplier:    len(files),
		failOnLocalUploadErr: true,
	})
}

// naturalKey identifies a batch for the server's create-or-get upsert path.
// Group is carried in metadata (metadata["group"]); the server extracts it
// from there during upsert, so there's one source of truth for the group string.
type naturalKey struct {
	ProviderRunID string
	RunAttempt    int
}

// batchOptions parameterizes runBatchCoverageUpload so the same runner serves
// both the local-only multi-file batch path (naturalKey == nil) and the
// cross-job batch-join path (naturalKey populated).
type batchOptions struct {
	expectedCount        int         // server-advertised expected_count (from --total-shards in join mode, len(files) otherwise)
	naturalKey           *naturalKey // nil → legacy "always create fresh" server path; populated → natural-key upsert
	timeoutMultiplier    int         // multiplies flagUploadTimeout for poll wait; legacy uses len(files), join uses 1
	failOnLocalUploadErr bool        // true: fail early if any of our local uploads fail (legacy); false: sibling shards may still finalize the batch (join)
}

func buildCoverageMetadata(ctx *uploadContext) map[string]any {
	targetBranch := resolveGitContext(flagCovTargetBranch, ctx.ci, func(info *cidetect.CIInfo) string { return info.TargetBranch })

	metadata := map[string]any{
		"format": flagCovFormat,
	}
	if flagCovPathPrefix != "" {
		metadata["path_prefix"] = flagCovPathPrefix
	}
	if ctx.prNumber != 0 {
		metadata["pr_number"] = ctx.prNumber
	}
	if targetBranch != "" {
		metadata["target_branch"] = targetBranch
	}
	if flagCovRunDate != "" {
		metadata["run_date"] = flagCovRunDate
	}
	if len(flagCovGroups) > 0 {
		metadata["group"] = strings.Join(flagCovGroups, ",")
	}
	applyDrapeRunIDMetadata(metadata, flagCovDrapeRunID, flagCovGroups)
	return metadata
}

func runSingleCoverageUpload(ctx *uploadContext, filePath string, metadata map[string]any) error {
	filePath = filepath.Clean(filePath)
	filename := filepath.Base(filePath)

	data, err := os.ReadFile(filePath) //nolint:gosec // G304: path is from CLI args, cleaned above
	if err != nil {
		return &ExitError{Code: exitcode.ParseError, Err: fmt.Errorf("reading file %s: %w", filePath, err)}
	}
	output.Info("Read %s (%d bytes)", filename, len(data))

	uploadID, err := ctx.uploadFile("coverage", filename, data, metadata, nil)
	if err != nil {
		return &ExitError{Code: exitcode.UploadError, Err: err}
	}
	output.Info("Coverage upload initiated (ID: %d)", uploadID)

	result := UploadResult{
		FilesMatched:  1,
		FilesUploaded: 1,
		Uploads:       []UploadEntry{{Filename: filename, UploadID: uploadID, DrapeURL: ctx.drapeURL(uploadID)}},
	}

	if !flagUploadWait {
		setResult(result)
		return nil
	}

	output.Info("Waiting for processing (timeout: %ds)...", flagUploadTimeout)
	status, err := ctx.client.PollCoverageStatus(ctx.orgSlug, ctx.repoID, uploadID, ctx.pollTimeout())
	if err != nil {
		if status != nil && status.Status == "failed" {
			return &ExitError{Code: exitcode.UploadError, Err: err}
		}
		return &ExitError{Code: exitcode.Timeout, Err: err}
	}

	result.Uploads[0].Result = status
	setResult(result)

	printCoverageSummary(status)

	if status.CoverageDiff != nil {
		return printCoverageDiff(status.CoverageDiff, ctx.prNumber)
	}

	return nil
}

func runBatchCoverageUpload(ctx *uploadContext, files []string, metadata map[string]any, opts batchOptions) error {
	req := api.CoverageBatchRequest{
		ExpectedCount: opts.expectedCount,
		UploadType:    "coverage",
		Branch:        ctx.branch,
		SHA:           ctx.sha,
		Metadata:      metadata,
	}
	if opts.naturalKey != nil {
		req.ProviderRunID = opts.naturalKey.ProviderRunID
		req.RunAttempt = opts.naturalKey.RunAttempt
	}

	batchResp, err := ctx.client.CreateCoverageBatch(ctx.orgSlug, ctx.repoID, req)
	if err != nil {
		return &ExitError{Code: exitcode.UploadError, Err: fmt.Errorf("creating batch: %w", err)}
	}
	output.Info("Created batch (ID: %d) for %d files", batchResp.BatchID, len(files))

	batchID := batchResp.BatchID
	result, uploadErrors := ctx.uploadFiles("coverage", files, func(_ string) map[string]any { return metadata }, &batchID)
	if err := checkAllFailed(uploadErrors, result.Uploads); err != nil {
		return err
	}

	output.Info("Uploaded %d/%d file(s)", result.FilesUploaded, len(files))

	if uploadErrors > 0 && opts.failOnLocalUploadErr {
		// Local-only batch: without all our files the batch can't reach
		// expected_count, so fail early instead of polling a doomed batch.
		// (In batch-join mode, sibling shards from other CI jobs may still
		// finalize it — don't short-circuit on our local failures.)
		setResult(result)
		return &ExitError{
			Code: exitcode.UploadError,
			Err: fmt.Errorf("%d of %d file(s) failed to upload; batch %d cannot complete",
				uploadErrors, len(files), batchResp.BatchID),
		}
	}

	if !flagUploadWait {
		setResult(result)
		return nil
	}

	// Cap batch timeout at 4 minutes to stay under the server-side 5-minute reaper.
	batchTimeout := time.Duration(flagUploadTimeout) * time.Second * time.Duration(opts.timeoutMultiplier)
	const maxBatchTimeout = 4 * time.Minute
	if batchTimeout > maxBatchTimeout {
		batchTimeout = maxBatchTimeout
	}
	output.Info("Waiting for batch processing (timeout: %ds)...", int(batchTimeout.Seconds()))

	batchStatus, err := ctx.client.PollCoverageBatchStatus(ctx.orgSlug, ctx.repoID, batchResp.BatchID, batchTimeout)
	if err != nil {
		if batchStatus != nil && batchStatus.Status == "failed" {
			return &ExitError{Code: exitcode.UploadError, Err: err}
		}
		return &ExitError{Code: exitcode.Timeout, Err: err}
	}

	return renderBatchResult(result, batchStatus, ctx.prNumber)
}

// renderBatchResult maps the batch status to the CLI's JSON output shape,
// prints the human-readable summary, and returns any exit-worthy error (e.g.
// coverage regression). Surfaces the partial-coverage warning when the
// server-side reaper finalized the batch with fewer than expected_count shards.
func renderBatchResult(result UploadResult, batchStatus *api.CoverageBatchStatusResponse, prNumber int) error {
	batchCovStatus := &api.CoverageStatusResponse{
		Status:         batchStatus.Status,
		ErrorMessage:   batchStatus.ErrorMessage,
		CoverageResult: batchStatus.CoverageResult,
	}

	if len(result.Uploads) > 0 {
		result.Uploads[0].Result = batchCovStatus
	}
	setResult(result)

	if batchCovStatus.Partial != nil && *batchCovStatus.Partial {
		output.Warn("⚠️  Partial coverage: %d of %d shards finalized before the 5-minute batch timeout.",
			batchStatus.CompletedCount, batchStatus.ExpectedCount)
		output.Warn("   The batch was published with partial data. Likely causes:")
		output.Warn("     - A sibling CI job failed before uploading (check the workflow run).")
		output.Warn("     - A sibling job is still running (check for slow tests or infra issues).")
		output.Warn("     - Shards disagreed on --total-shards (soft-merged as max across shards).")
		output.Warn("   To retry with full coverage, re-run the failed jobs in the workflow.")
	}

	printCoverageSummary(batchCovStatus)

	if batchCovStatus.CoverageDiff != nil {
		return printCoverageDiff(batchCovStatus.CoverageDiff, prNumber)
	}
	return nil
}

func printCoverageSummary(status *api.CoverageStatusResponse) {
	output.Info("")
	output.Info("Coverage Summary")
	if status.CoverageRate != nil {
		output.Info("  Coverage rate: %s%%", *status.CoverageRate)
	}
	if status.FileCount != nil {
		output.Info("  Files:         %d", *status.FileCount)
	}
}

func printCoverageDiff(diff *api.CoverageDiffInfo, prNumber int) error {
	output.Info("")
	if prNumber > 0 {
		output.Info("Coverage Diff (PR #%d)", prNumber)
	} else {
		output.Info("Coverage Diff")
	}

	if diff.BaseCoverageRate != nil {
		output.Info("  Base:      %s%%", *diff.BaseCoverageRate)
	} else {
		output.Info("  Base:      (no baseline)")
	}

	if diff.CoverageDelta != nil {
		output.Info("  Head:      %s%%  (%s%%)", diff.HeadCoverageRate, formatDelta(*diff.CoverageDelta))
	} else {
		output.Info("  Head:      %s%%", diff.HeadCoverageRate)
	}

	if diff.NewCodeCoverageRate != nil {
		output.Info("  New code:  %s%%  (%d/%d lines)", *diff.NewCodeCoverageRate, diff.NewLinesCovered, diff.NewLinesTotal)
	} else if diff.NewLinesTotal > 0 {
		output.Info("  New code:  %d/%d lines", diff.NewLinesCovered, diff.NewLinesTotal)
	}

	output.Info("  Regressed: %d lines", diff.RegressedLinesCount)

	if len(diff.RegressedFiles) > 0 {
		output.Info("")
		output.Info("  Regressed files:")
		for _, rf := range diff.RegressedFiles {
			if len(rf.RegressedLineRanges) > 0 {
				output.Info("    %s: %d lines (%s)", rf.FilePath, rf.RegressedLines, formatLineRanges(rf.RegressedLineRanges))
			} else {
				output.Info("    %s: %d lines", rf.FilePath, rf.RegressedLines)
			}
		}
	}

	if diff.Passed {
		output.Info("  Result:    PASSED")
	} else {
		output.Info("  Result:    FAILED")
		for _, reason := range diff.FailureReasons {
			output.Info("    - %s", reason)
		}
		return &ExitError{Code: exitcode.CoverageRegression, Err: fmt.Errorf("coverage check failed")}
	}

	// Print info-level warnings (e.g., "no baseline found") even on pass
	for _, reason := range diff.FailureReasons {
		output.Info("  Note: %s", reason)
	}

	return nil
}

func formatDelta(delta string) string {
	if len(delta) == 0 {
		return delta
	}
	if delta[0] != '-' {
		return "+" + delta
	}
	return delta
}

func formatLineRanges(ranges [][]int) string {
	parts := make([]string, 0, len(ranges))
	for _, r := range ranges {
		if len(r) != 2 {
			continue
		}
		if r[0] == r[1] {
			parts = append(parts, strconv.Itoa(r[0]))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", r[0], r[1]))
		}
	}
	return strings.Join(parts, ", ")
}
