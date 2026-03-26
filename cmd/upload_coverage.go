package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
	Use:   "coverage <file>",
	Short: "Upload coverage report to Drape",
	Long:  "Upload a coverage report (Cobertura XML, LCOV, or Go coverage) to Drape for analysis.",
	Args:  cobra.ExactArgs(1),
	RunE:  runUploadCoverage,
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
	filePath := filepath.Clean(args[0])
	filename := filepath.Base(filePath)

	data, err := os.ReadFile(filePath) //nolint:gosec // G304: path is from CLI args, cleaned above
	if err != nil {
		return &ExitError{Code: exitcode.ParseError, Err: fmt.Errorf("reading file %s: %w", filePath, err)}
	}
	output.Info("Read %s (%d bytes)", filename, len(data))

	ctx, err := newUploadContext()
	if err != nil {
		return err
	}

	if flagDryRun {
		output.Info("[dry-run] Would upload %s (format: %s, branch: %s, sha: %s)", filename, flagCovFormat, ctx.branch, ctx.sha)
		if flagJSON {
			setResult(DryRunResult{DryRun: true, Files: []string{filename}})
		}
		return nil
	}

	if err := ctx.resolveClient(); err != nil {
		return err
	}

	// Build metadata
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

	output.Info("")
	output.Info("Coverage Summary")
	if status.CoverageRate != nil {
		output.Info("  Coverage rate: %s%%", *status.CoverageRate)
	}
	if status.FileCount != nil {
		output.Info("  Files:         %d", *status.FileCount)
	}

	if status.CoverageDiff != nil {
		return printCoverageDiff(status.CoverageDiff, ctx.prNumber)
	}

	return nil
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
