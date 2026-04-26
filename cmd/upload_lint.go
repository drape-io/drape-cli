package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/drape-io/drape-cli/internal/api"
	"github.com/drape-io/drape-cli/internal/exitcode"
	"github.com/drape-io/drape-cli/internal/output"
)

var flagLintFormat string

var uploadLintCmd = &cobra.Command{
	Use:   "lint <file>",
	Short: "Upload a lint report to Drape",
	Long:  "Upload a SARIF lint report to Drape for analysis.",
	Args:  cobra.ExactArgs(1),
	RunE:  runUploadLint,
}

func init() {
	uploadLintCmd.Flags().StringVar(&flagLintFormat, "format", "sarif", "Lint report format (default: sarif)")

	uploadCmd.AddCommand(uploadLintCmd)
}

func runUploadLint(cmd *cobra.Command, args []string) error {
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
		ctx.dryRunSimple([]string{filePath}, flagLintFormat)
		return nil
	}

	if err := ctx.resolveClient(); err != nil {
		return err
	}

	metadata := map[string]any{
		"format": flagLintFormat,
	}
	if ctx.prNumber != 0 {
		metadata["pr_number"] = ctx.prNumber
	}

	uploadID, err := ctx.uploadFile("lint_report", filename, data, metadata, nil)
	if err != nil {
		return &ExitError{Code: exitcode.UploadError, Err: err}
	}
	output.Info("Lint report uploaded (ID: %d)", uploadID)

	result := UploadResult{
		FilesMatched:  1,
		FilesUploaded: 1,
		Uploads:       []UploadEntry{{Filename: filename, UploadID: uploadID, DrapeURL: ctx.drapeURL(uploadID)}},
	}

	if !flagUploadWait {
		setResult(result)
		return nil
	}

	output.Info("Waiting for processing (timeout: %s)...", flagUploadWaitTimeout)
	status, err := ctx.client.PollLintStatus(ctx.orgSlug, ctx.repoID, uploadID, flagUploadWaitTimeout)
	if err != nil {
		if status != nil && status.Status == "failed" {
			return &ExitError{Code: exitcode.UploadError, Err: err}
		}
		return &ExitError{Code: exitcode.Timeout, Err: err}
	}

	result.Uploads[0].Result = status
	setResult(result)

	output.Info("")
	output.Info("Lint Summary")
	if status.TotalViolations != nil {
		output.Info("  Total violations: %d", *status.TotalViolations)
	}
	if status.ErrorCount != nil {
		output.Info("  Errors:           %d", *status.ErrorCount)
	}
	if status.WarningCount != nil {
		output.Info("  Warnings:         %d", *status.WarningCount)
	}

	if status.LintDiff != nil {
		return printLintDiff(status.LintDiff, ctx.prNumber)
	}

	return nil
}

func printLintDiff(diff *api.LintDiffInfo, prNumber int) error {
	output.Info("")
	if prNumber > 0 {
		output.Info("Lint Diff (PR #%d)", prNumber)
	} else {
		output.Info("Lint Diff")
	}

	output.Info("  Base violations: %d", diff.BaseViolationCount)
	output.Info("  Head violations: %d", diff.HeadViolationCount)
	output.Info("  New:             %d", diff.NewViolationCount)
	output.Info("  Resolved:        %d", diff.ResolvedViolationCount)
	if diff.SuppressedViolationCount > 0 {
		output.Info("  Suppressed:      %d", diff.SuppressedViolationCount)
	}

	if len(diff.NewViolations) > 0 {
		output.Info("")
		output.Info("  New violations:")
		for _, v := range diff.NewViolations {
			output.Info("    %s:%d [%s] %s: %s", v.FilePath, v.Line, v.Severity, v.RuleID, v.Message)
		}
	}

	if diff.Passed {
		output.Info("  Result:    PASSED")
		if diff.SuppressedViolationCount > 0 && diff.NewViolationCount == 0 {
			output.Info("")
			output.Info("All %d new violation(s) are suppressed — passing CI", diff.SuppressedViolationCount)
		}
	} else {
		output.Info("  Result:    FAILED")
		for _, reason := range diff.FailureReasons {
			output.Info("    - %s", reason)
		}
		return &ExitError{Code: exitcode.LintFailure, Err: fmt.Errorf("lint check failed")}
	}

	// Print info-level notes even on pass
	for _, reason := range diff.FailureReasons {
		output.Info("  Note: %s", reason)
	}

	return nil
}
