package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/drape-io/drape-cli/internal/api"
	"github.com/drape-io/drape-cli/internal/exitcode"
	"github.com/drape-io/drape-cli/internal/output"
)

var (
	flagTestFormat   string
	flagTestJobName  string
	flagTestPRNumber int
	flagTestRunDate  string
	flagTestGroups   []string
	flagTestDrapeRunID string
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
	uploadTestsCmd.Flags().StringVar(&flagTestDrapeRunID, "drape-run-id", "", "Drape run ID to correlate triggered CI runs (env: DRAPE_RUN_ID)")

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
	applyDrapeRunIDMetadata(metadata, flagTestDrapeRunID, flagTestGroups)

	// Upload each file
	result, uploadErrors := ctx.uploadFiles("test_results", files, func(_ string) map[string]any { return metadata }, nil)
	if err := checkAllFailed(uploadErrors, result.Uploads); err != nil {
		return err
	}

	output.Info("Uploaded %d/%d file(s)", result.FilesUploaded, len(files))

	if !flagUploadWait {
		for _, u := range result.Uploads {
			output.Info("  %s: processing (ID: %d)", u.Filename, u.UploadID)
		}
		setResult(result)
		return nil
	}

	output.Info("Waiting for processing (timeout: %s)...", flagUploadWaitTimeout)

	var totalIngested int
	var totalSuppressed int
	var totalFailed int
	var totalUnsuppressedFailures int
	var processingErrors int

	for i, u := range result.Uploads {
		status, err := ctx.client.PollTestStatus(ctx.orgSlug, ctx.repoID, u.UploadID, flagUploadWaitTimeout)
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
		if status.SuppressedCount != nil {
			totalSuppressed += *status.SuppressedCount
		}
		if status.FailedCount != nil {
			totalFailed += *status.FailedCount
		}
		if status.UnsuppressedFailureCount != nil {
			totalUnsuppressedFailures += *status.UnsuppressedFailureCount
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
	if totalSuppressed > 0 {
		output.Info("  Suppressed:     %d", totalSuppressed)
	}
	if uploadErrors > 0 {
		output.Info("  Upload errors:  %d", uploadErrors)
	}
	if processingErrors > 0 {
		output.Info("  Process errors: %d", processingErrors)
	}

	// Display new test detection
	var newTests []string
	for _, u := range result.Uploads {
		if status, ok := u.Result.(*api.TestStatusResponse); ok {
			newTests = append(newTests, status.NewTestsDetected...)
		}
	}
	if len(newTests) > 0 {
		output.Info("")
		if len(newTests) == 1 {
			output.Info("1 new test detected:")
		} else {
			output.Info("%d new tests detected:", len(newTests))
		}
		const maxDisplay = 10
		for i, t := range newTests {
			if i >= maxDisplay {
				output.Info("  ...and %d more", len(newTests)-maxDisplay)
				break
			}
			output.Info("  - %s", t)
		}
		output.Info("Consider burn-in via Drape dashboard to verify stability.")
	}

	return testUploadExitError(totalFailed, totalUnsuppressedFailures, processingErrors, totalIngested)
}

// testUploadExitError determines the exit code after test upload processing.
func testUploadExitError(totalFailed, totalUnsuppressedFailures, processingErrors, totalIngested int) error {
	if totalFailed > 0 && totalUnsuppressedFailures == 0 {
		output.Info("")
		output.Info("All %d failure(s) are suppressed — passing CI", totalFailed)
		return nil
	}

	if processingErrors > 0 && totalIngested == 0 {
		return &ExitError{Code: exitcode.UploadError, Err: fmt.Errorf("all processing failed")}
	}

	if totalUnsuppressedFailures > 0 {
		return &ExitError{Code: exitcode.TestFailure, Err: fmt.Errorf("%d unsuppressed test failure(s)", totalUnsuppressedFailures)}
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

	if flagJSON {
		jsonFiles := make([]TestsDryRunFile, 0, len(results))
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

	if parseErrors > 0 {
		return &ExitError{Code: exitcode.ParseError, Err: fmt.Errorf("%d file(s) failed to parse", parseErrors)}
	}

	return nil
}
