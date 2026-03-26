package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/drape-io/drape-cli/internal/exitcode"
	"github.com/drape-io/drape-cli/internal/output"
)

var (
	flagTestFormat   string
	flagTestJobName  string
	flagTestPRNumber int
	flagTestRunDate  string
	flagTestGroups   []string
)

var uploadTestsCmd = &cobra.Command{
	Use:   "tests <glob>",
	Short: "Upload JUnit XML test results to Drape",
	Long:  "Find test result files matching the glob pattern, upload them to Drape, and print a summary.",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runUploadTests,
}

func init() {
	uploadTestsCmd.Flags().StringVar(&flagTestFormat, "format", "", "Force format (junit, ctrf). Default: auto-detect")
	uploadTestsCmd.Flags().StringVar(&flagTestJobName, "job-name", "", "CI job name (auto-detected from CI)")
	uploadTestsCmd.Flags().IntVar(&flagTestPRNumber, "pr-number", 0, "PR number (auto-detected from CI)")
	uploadTestsCmd.Flags().StringVar(&flagTestRunDate, "run-date", "", "ISO 8601 date for historical uploads (e.g. 2026-03-15)")
	uploadTestsCmd.Flags().StringSliceVar(&flagTestGroups, "group", nil, "Group label(s) for this upload (can be specified multiple times)")

	uploadCmd.AddCommand(uploadTestsCmd)
}

func runUploadTests(cmd *cobra.Command, args []string) error {
	files, err := expandGlobs(args)
	if err != nil {
		return &ExitError{Code: exitcode.UsageError, Err: err}
	}
	if len(files) == 0 {
		return &ExitError{Code: exitcode.ParseError, Err: fmt.Errorf("no files matched the given pattern(s)")}
	}

	output.Info("Found %d file(s) to upload", len(files))

	ctx, err := newUploadContext()
	if err != nil {
		return err
	}

	if flagDryRun {
		return dryRunValidate(files)
	}

	if err := ctx.resolveClient(); err != nil {
		return err
	}

	// Build metadata
	prNumber := flagTestPRNumber
	if prNumber == 0 {
		prNumber = ctx.prNumber
	}

	metadata := map[string]any{}
	if flagTestFormat != "" {
		metadata["format"] = flagTestFormat
	}
	if ctx.ci != nil {
		metadata["provider_type"] = ctx.ci.Provider
	}
	jobName := flagTestJobName
	if jobName == "" && ctx.ci != nil {
		jobName = ctx.ci.JobID
	}
	if jobName != "" {
		metadata["job_name"] = jobName
	}
	if prNumber != 0 {
		metadata["pr_number"] = prNumber
	}
	if flagTestRunDate != "" {
		metadata["run_date"] = flagTestRunDate
	}
	if len(flagTestGroups) > 0 {
		metadata["group"] = strings.Join(flagTestGroups, ",")
	}

	// Upload each file
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

		uploadID, err := ctx.uploadFile("test_results", filename, data, metadata)
		if err != nil {
			output.Error("Failed to upload %s: %v", filename, err)
			uploadErrors++
			continue
		}

		output.Verbose("  %s: upload initiated (ID: %d)", filename, uploadID)
		result.Uploads = append(result.Uploads, UploadEntry{Filename: filename, UploadID: uploadID, DrapeURL: ctx.drapeURL(uploadID)})
	}

	result.FilesUploaded = len(result.Uploads)

	if uploadErrors > 0 && len(result.Uploads) == 0 {
		return &ExitError{Code: exitcode.UploadError, Err: fmt.Errorf("all uploads failed")}
	}

	output.Info("Uploaded %d/%d file(s)", result.FilesUploaded, len(files))

	if !flagUploadWait {
		for _, u := range result.Uploads {
			output.Info("  %s: processing (ID: %d)", u.Filename, u.UploadID)
		}
		setResult(result)
		return nil
	}

	output.Info("Waiting for processing (timeout: %ds)...", flagUploadTimeout)

	var totalIngested int
	var totalQuarantined int
	var totalFailed int
	var totalUnquarantinedFailures int
	var processingErrors int

	for i, u := range result.Uploads {
		status, err := ctx.client.PollTestStatus(ctx.orgSlug, ctx.repoID, u.UploadID, ctx.pollTimeout())
		if err != nil {
			if status != nil && status.Status == "failed" {
				output.Error("  %s: processing failed: %v", u.Filename, err)
				result.Uploads[i].Result = status
				processingErrors++
				continue
			}
			return &ExitError{Code: exitcode.Timeout, Err: err}
		}

		result.Uploads[i].Result = status

		ingested := 0
		if status.TestsIngested != nil {
			ingested = *status.TestsIngested
		}
		output.Verbose("  %s: %d tests ingested", u.Filename, ingested)

		totalIngested += ingested
		if status.QuarantinedCount != nil {
			totalQuarantined += *status.QuarantinedCount
		}
		if status.FailedCount != nil {
			totalFailed += *status.FailedCount
		}
		if status.UnquarantinedFailureCount != nil {
			totalUnquarantinedFailures += *status.UnquarantinedFailureCount
		}
	}

	setResult(result)

	// Summary
	output.Info("")
	output.Info("Upload Summary")
	output.Info("  Files uploaded: %d/%d", result.FilesUploaded, len(files))
	output.Info("  Tests ingested: %d", totalIngested)
	if totalFailed > 0 {
		output.Info("  Failed tests:   %d", totalFailed)
	}
	if totalQuarantined > 0 {
		output.Info("  Quarantined:    %d", totalQuarantined)
	}
	if uploadErrors > 0 {
		output.Info("  Upload errors:  %d", uploadErrors)
	}
	if processingErrors > 0 {
		output.Info("  Process errors: %d", processingErrors)
	}

	if totalFailed > 0 && totalUnquarantinedFailures == 0 {
		output.Info("")
		output.Info("All %d failure(s) are quarantined — passing CI", totalFailed)
		return nil
	}

	if processingErrors > 0 && totalIngested == 0 {
		return &ExitError{Code: exitcode.UploadError, Err: fmt.Errorf("all processing failed")}
	}

	return nil
}

func dryRunValidate(files []string) error {
	output.Info("[dry-run] Validating files locally, no upload will be performed")

	results, total, parseErrors := validateJUnitFiles(files)

	for _, r := range results {
		output.Info("  %s: %d tests (%d passed, %d failed, %d skipped, %d errors)",
			r.Filename, r.Summary.Total, r.Summary.Passed, r.Summary.Failed, r.Summary.Skipped, r.Summary.Errored)
	}

	output.Info("")
	output.Info("[dry-run] Total: %d tests (%d passed, %d failed, %d skipped, %d errors)",
		total.Total, total.Passed, total.Failed, total.Skipped, total.Errored)

	if parseErrors > 0 {
		return &ExitError{Code: exitcode.ParseError, Err: fmt.Errorf("%d file(s) failed to parse", parseErrors)}
	}

	if flagJSON {
		var jsonFiles []TestsDryRunFile
		for _, r := range results {
			jsonFiles = append(jsonFiles, TestsDryRunFile{
				Filename: r.Filename,
				Total:    r.Summary.Total,
				Passed:   r.Summary.Passed,
				Failed:   r.Summary.Failed,
				Skipped:  r.Summary.Skipped,
				Errored:  r.Summary.Errored,
			})
		}
		setResult(TestsDryRunResult{DryRun: true, Files: jsonFiles})
	}
	return nil
}
