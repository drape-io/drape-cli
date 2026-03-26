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

	if len(files) == 1 {
		return runSingleCoverageUpload(ctx, files[0], metadata)
	}
	return runBatchCoverageUpload(ctx, files, metadata)
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

	uploadID, err := ctx.uploadFile("coverage", filename, data, metadata)
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

func runBatchCoverageUpload(ctx *uploadContext, files []string, metadata map[string]any) error {
	// Create the batch
	batchResp, err := ctx.client.CreateCoverageBatch(ctx.orgSlug, ctx.repoID, api.CoverageBatchRequest{
		ExpectedCount: len(files),
		UploadType:    "coverage",
		Branch:        ctx.branch,
		SHA:           ctx.sha,
		Metadata:      metadata,
	})
	if err != nil {
		return &ExitError{Code: exitcode.UploadError, Err: fmt.Errorf("creating batch: %w", err)}
	}
	output.Info("Created batch (ID: %d) for %d files", batchResp.BatchID, len(files))

	// Upload each file with the batch ID
	result := UploadResult{FilesMatched: len(files)}
	var uploadErrors int

	for _, f := range files {
		data, err := os.ReadFile(filepath.Clean(f)) //nolint:gosec // G304: path is from CLI args + glob expansion
		if err != nil {
			output.Error("Failed to read %s: %v", f, err)
			uploadErrors++
			continue
		}

		filename := filepath.Base(f)
		output.Verbose("Uploading %s (%d bytes)...", filename, len(data))

		uploadID, err := ctx.uploadFileWithBatch("coverage", filename, data, metadata, batchResp.BatchID)
		if err != nil {
			output.Error("Failed to upload %s: %v", filename, err)
			uploadErrors++
			continue
		}

		output.Verbose("  %s: upload initiated (ID: %d)", filename, uploadID)
		result.Uploads = append(result.Uploads, UploadEntry{Filename: filename, UploadID: uploadID, DrapeURL: ctx.drapeURL(uploadID)})
	}

	result.FilesUploaded = len(result.Uploads)
	if err := checkAllFailed(uploadErrors, result.Uploads); err != nil {
		return err
	}

	output.Info("Uploaded %d/%d file(s)", result.FilesUploaded, len(files))

	if !flagUploadWait {
		setResult(result)
		return nil
	}

	// Poll batch status with scaled timeout: 120s per file
	batchTimeout := time.Duration(flagUploadTimeout) * time.Second * time.Duration(len(files))
	output.Info("Waiting for batch processing (timeout: %ds)...", int(batchTimeout.Seconds()))

	batchStatus, err := ctx.client.PollCoverageBatchStatus(ctx.orgSlug, ctx.repoID, batchResp.BatchID, batchTimeout)
	if err != nil {
		if batchStatus != nil && batchStatus.Status == "failed" {
			return &ExitError{Code: exitcode.UploadError, Err: err}
		}
		return &ExitError{Code: exitcode.Timeout, Err: err}
	}

	// Map batch result to a CoverageStatusResponse for consistent JSON output
	batchCovStatus := &api.CoverageStatusResponse{
		Status:             batchStatus.Status,
		CoverageSnapshotID: batchStatus.CoverageSnapshotID,
		CoverageRate:       batchStatus.CoverageRate,
		FileCount:          batchStatus.FileCount,
		ErrorMessage:       batchStatus.ErrorMessage,
		CoverageDiff:       batchStatus.CoverageDiff,
	}

	// Attach the merged result to the first upload entry for JSON output
	if len(result.Uploads) > 0 {
		result.Uploads[0].Result = batchCovStatus
	}
	setResult(result)

	printCoverageSummary(batchCovStatus)

	if batchCovStatus.CoverageDiff != nil {
		return printCoverageDiff(batchCovStatus.CoverageDiff, ctx.prNumber)
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
