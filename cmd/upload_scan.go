package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/drape-io/drape-cli/internal/api"
	"github.com/drape-io/drape-cli/internal/exitcode"
	"github.com/drape-io/drape-cli/internal/output"
)

var (
	flagScanFormat         string
	flagScanName           string
	flagScanTag            string
	flagScanType           string
	flagScanFailOnVulns    bool
	flagScanFailOnSeverity string
)

// severityRank maps severity names to a numeric rank for comparison.
var severityRank = map[string]int{
	"critical": 4,
	"high":     3,
	"medium":   2,
	"low":      1,
	"unknown":  0,
}

var validSeverities = []string{"critical", "high", "medium", "low", "any"}

func severityMeetsThreshold(severity, threshold string) bool {
	if threshold == "any" {
		return true
	}
	return severityRank[severity] >= severityRank[threshold]
}

var uploadScanCmd = &cobra.Command{
	Use:   "scan <glob>",
	Short: "Upload security scan results to Drape",
	Long:  "Upload SARIF or CycloneDX security scan results to Drape for analysis.",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runUploadScan,
}

func init() {
	uploadScanCmd.Flags().StringVar(&flagScanFormat, "format", "sarif", "Scan report format: sarif, cyclonedx (default: sarif)")
	uploadScanCmd.Flags().StringVar(&flagScanName, "scan-name", "", "Scan name (e.g. docker image name, tool name)")
	uploadScanCmd.Flags().StringVar(&flagScanTag, "scan-tag", "", "Scan tag (e.g. image tag, version)")
	uploadScanCmd.Flags().StringVar(&flagScanType, "scan-type", "", "Scan type: image, dependency (default: auto-detect from format)")
	uploadScanCmd.Flags().BoolVar(&flagScanFailOnVulns, "fail-on-vulnerabilities", false, "Exit non-zero if unsuppressed vulnerabilities are found")
	uploadScanCmd.Flags().StringVar(&flagScanFailOnSeverity, "fail-on-severity", "medium", "Minimum severity to fail on: critical, high, medium, low, any (default: medium)")

	uploadCmd.AddCommand(uploadScanCmd)
}

func runUploadScan(cmd *cobra.Command, args []string) error {
	// Validate --fail-on-severity value
	validSev := false
	for _, s := range validSeverities {
		if flagScanFailOnSeverity == s {
			validSev = true
			break
		}
	}
	if !validSev {
		return &ExitError{Code: exitcode.UsageError, Err: fmt.Errorf("invalid --fail-on-severity value %q, must be one of: critical, high, medium, low, any", flagScanFailOnSeverity)}
	}

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
		ctx.dryRunSimple(files, flagScanFormat)
		return nil
	}

	if err := ctx.resolveClient(); err != nil {
		return err
	}

	// Upload each file
	result, uploadErrors := ctx.uploadFiles("scan_results", files, func(_ string) map[string]any {
		metadata := map[string]any{
			"format": flagScanFormat,
		}
		if flagScanName != "" {
			metadata["scan_name"] = flagScanName
		}
		if flagScanTag != "" {
			metadata["scan_tag"] = flagScanTag
		}
		if flagScanType != "" {
			metadata["scan_type"] = flagScanType
		}
		if ctx.prNumber != 0 {
			metadata["pr_number"] = ctx.prNumber
		}
		return metadata
	})
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

	output.Info("Waiting for processing (timeout: %ds)...", flagUploadTimeout)

	var lastDiffErr error
	var processingErrors int
	var hasUnsuppressedVulns bool

	for i, u := range result.Uploads {
		status, err := ctx.client.PollScanStatus(ctx.orgSlug, ctx.repoID, u.UploadID, ctx.pollTimeout())
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

		printScanSummary(u.Filename, status)

		if flagScanFailOnVulns {
			hasUnsuppressedVulns = hasUnsuppressedVulns || checkScanVulns(status)
		}

		if status.ScanDiff != nil {
			if err := printScanDiff(status.ScanDiff, ctx.prNumber); err != nil {
				lastDiffErr = err
			}
		}
	}

	if uploadErrors > 0 {
		output.Info("")
		output.Info("Upload errors: %d", uploadErrors)
	}
	if processingErrors > 0 {
		output.Info("Process errors: %d", processingErrors)
	}

	if processingErrors > 0 && len(result.Uploads) == processingErrors {
		return &ExitError{Code: exitcode.UploadError, Err: fmt.Errorf("all processing failed")}
	}

	// Always set result so JSON is emitted even on policy failure
	setResult(result)

	if lastDiffErr != nil {
		return lastDiffErr
	}

	if flagScanFailOnVulns && hasUnsuppressedVulns {
		return &ExitError{Code: exitcode.ScanFailure, Err: fmt.Errorf("unsuppressed vulnerabilities found")}
	}

	return nil
}

func printScanSummary(filename string, status *api.ScanStatusResponse) {
	output.Info("")
	output.Info("Scan Summary (%s)", filename)
	if status.ScanName != nil {
		output.Info("  Scan:            %s", *status.ScanName)
	}
	if status.TotalVulnerabilities != nil {
		output.Info("  Vulnerabilities: %d", *status.TotalVulnerabilities)
	}
	if status.UnsuppressedVulnerabilities != nil {
		suppressed := 0
		if status.TotalVulnerabilities != nil {
			suppressed = *status.TotalVulnerabilities - *status.UnsuppressedVulnerabilities
		}
		if suppressed > 0 {
			output.Info("  Suppressed:      %d", suppressed)
			output.Info("  Unsuppressed:    %d", *status.UnsuppressedVulnerabilities)
		}
	}
	if status.UnsuppressedHighestSeverity != nil {
		output.Info("  Highest:         %s", *status.UnsuppressedHighestSeverity)
	} else if status.HighestSeverity != nil {
		output.Info("  Highest:         %s", *status.HighestSeverity)
	}
}

func checkScanVulns(status *api.ScanStatusResponse) bool {
	vulnCount := 0
	if status.UnsuppressedVulnerabilities != nil {
		vulnCount = *status.UnsuppressedVulnerabilities
	} else if status.TotalVulnerabilities != nil {
		vulnCount = *status.TotalVulnerabilities
	}
	if vulnCount == 0 {
		return false
	}
	highestSev := ""
	if status.UnsuppressedHighestSeverity != nil {
		highestSev = *status.UnsuppressedHighestSeverity
	} else if status.HighestSeverity != nil {
		highestSev = *status.HighestSeverity
	}
	return severityMeetsThreshold(highestSev, flagScanFailOnSeverity)
}

func printScanDiff(diff *api.ScanDiffInfo, prNumber int) error {
	output.Info("")
	if prNumber > 0 {
		output.Info("Scan Diff (PR #%d)", prNumber)
	} else {
		output.Info("Scan Diff")
	}

	totalNew := diff.NewCriticalCount + diff.NewHighCount + diff.NewMediumCount + diff.NewLowCount
	output.Info("  New CVEs:    %d (critical: %d, high: %d, medium: %d, low: %d)",
		totalNew, diff.NewCriticalCount, diff.NewHighCount, diff.NewMediumCount, diff.NewLowCount)
	if diff.SuppressedCVECount > 0 {
		output.Info("  Suppressed:  %d", diff.SuppressedCVECount)
	}
	if len(diff.ResolvedCVEs) > 0 {
		output.Info("  Resolved:    %d", len(diff.ResolvedCVEs))
	}
	output.Info("  Unchanged:   %d", diff.UnchangedCVECount)

	if len(diff.NewCVEs) > 0 {
		output.Info("")
		output.Info("  New CVEs:")
		for _, cve := range diff.NewCVEs {
			suppressed := ""
			if cve.Suppression != nil {
				suppressed = fmt.Sprintf(" [suppressed: %s]", cve.Suppression.Type)
			}
			output.Info("    %s [%s] %s@%s (%s)%s",
				cve.CVEID, cve.Severity, cve.PackageName, cve.PackageVersion, cve.FixState, suppressed)
		}
	}

	if len(diff.SLAViolations) > 0 {
		output.Info("")
		output.Info("  SLA Violations:")
		for _, v := range diff.SLAViolations {
			output.Info("    %s [%s] %s — %d days overdue", v.CVEID, v.Severity, v.PackageName, v.DaysOverdue)
		}
	}

	if diff.Passed {
		output.Info("  Result:    PASSED")
		if diff.SuppressedCVECount > 0 && totalNew == 0 {
			output.Info("")
			output.Info("All %d new CVE(s) are suppressed — passing CI", diff.SuppressedCVECount)
		}
	} else {
		output.Info("  Result:    FAILED")
		for _, reason := range diff.FailureReasons {
			output.Info("    - %s", reason)
		}
		return &ExitError{Code: exitcode.ScanFailure, Err: fmt.Errorf("scan check failed")}
	}

	// Print info-level notes even on pass
	for _, reason := range diff.FailureReasons {
		output.Info("  Note: %s", reason)
	}

	return nil
}
