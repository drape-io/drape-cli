package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/drape-io/drape-cli/internal/api"
	"github.com/drape-io/drape-cli/internal/cidetect"
	"github.com/drape-io/drape-cli/internal/exitcode"
	"github.com/drape-io/drape-cli/internal/output"
)

var (
	flagScanBranch          string
	flagScanSHA             string
	flagScanFormat          string
	flagScanName            string
	flagScanTag             string
	flagScanType            string
	flagScanWait            bool
	flagScanTimeout         int
	flagScanFailOnVulns     bool
	flagScanFailOnSeverity  string
)

// severityRank maps severity names to a numeric rank for comparison.
// Higher rank = more severe. Unknown/unrecognized severities get rank 0.
var severityRank = map[string]int{
	"critical": 4,
	"high":     3,
	"medium":   2,
	"low":      1,
	"unknown":  0,
}

var validSeverities = []string{"critical", "high", "medium", "low", "any"}

// severityMeetsThreshold returns true if the given severity is at or above the threshold.
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
	uploadScanCmd.Flags().StringVar(&flagScanBranch, "branch", "", "Git branch (auto-detected from CI)")
	uploadScanCmd.Flags().StringVar(&flagScanSHA, "sha", "", "Git commit SHA (auto-detected from CI)")
	uploadScanCmd.Flags().StringVar(&flagScanFormat, "format", "sarif", "Scan report format: sarif, cyclonedx (default: sarif)")
	uploadScanCmd.Flags().StringVar(&flagScanName, "scan-name", "", "Scan name (e.g. docker image name, tool name)")
	uploadScanCmd.Flags().StringVar(&flagScanTag, "scan-tag", "", "Scan tag (e.g. image tag, version)")
	uploadScanCmd.Flags().StringVar(&flagScanType, "scan-type", "", "Scan type: image, dependency (default: auto-detect from format)")
	uploadScanCmd.Flags().BoolVar(&flagScanWait, "wait", true, "Wait for server-side processing")
	uploadScanCmd.Flags().IntVar(&flagScanTimeout, "timeout", 120, "Max wait time in seconds")
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

	ci := cidetect.Detect(os.Getenv)
	if ci == nil {
		ci = cidetect.DetectFromGit()
	}

	branch := resolveGitContext(flagScanBranch, ci, func(info *cidetect.CIInfo) string { return info.Branch })
	sha := resolveGitContext(flagScanSHA, ci, func(info *cidetect.CIInfo) string { return info.CommitSHA })

	if branch == "" {
		return &ExitError{Code: exitcode.UsageError, Err: errMissing("--branch (could not auto-detect)")}
	}
	if sha == "" {
		return &ExitError{Code: exitcode.UsageError, Err: errMissing("--sha (could not auto-detect)")}
	}

	if flagDryRun {
		for _, f := range files {
			output.Info("[dry-run] Would upload %s (format: %s, branch: %s, sha: %s)", filepath.Base(f), flagScanFormat, branch, sha)
		}
		return nil
	}

	orgSlug, err := resolveOrg()
	if err != nil {
		return err
	}

	client, err := newClient()
	if err != nil {
		return err
	}

	repoID, err := resolveRepoID(client, orgSlug)
	if err != nil {
		return err
	}

	prNumber := 0
	if ci != nil && ci.PRNumber != "" {
		prNumber, _ = strconv.Atoi(ci.PRNumber)
	}

	type uploadResult struct {
		uploadID int
		filename string
	}

	var uploads []uploadResult
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
		if prNumber != 0 {
			metadata["pr_number"] = prNumber
		}

		initResp, err := client.InitiateUpload(orgSlug, repoID, api.UploadInitiateRequest{
			UploadType: "scan_results",
			Branch:     branch,
			SHA:        sha,
			Filename:   filename,
			Metadata:   metadata,
		})
		if err != nil {
			output.Error("Failed to initiate upload for %s: %v", filename, err)
			uploadErrors++
			continue
		}

		if err := client.UploadToPresignedURL(initResp.UploadURL, data); err != nil {
			output.Error("Failed to upload %s to storage: %v", filename, err)
			uploadErrors++
			continue
		}

		if err := client.CompleteUpload(orgSlug, repoID, initResp.UploadID); err != nil {
			output.Error("Failed to complete upload for %s: %v", filename, err)
			uploadErrors++
			continue
		}

		output.Verbose("  %s: upload initiated (ID: %d)", filename, initResp.UploadID)
		uploads = append(uploads, uploadResult{uploadID: initResp.UploadID, filename: filename})
	}

	if uploadErrors > 0 && len(uploads) == 0 {
		return &ExitError{Code: exitcode.UploadError, Err: fmt.Errorf("all uploads failed")}
	}

	output.Info("Uploaded %d/%d file(s)", len(uploads), len(files))

	if !flagScanWait {
		for _, u := range uploads {
			output.Info("  %s: processing (ID: %d)", u.filename, u.uploadID)
		}
		return nil
	}

	output.Info("Waiting for processing (timeout: %ds)...", flagScanTimeout)
	timeout := time.Duration(flagScanTimeout) * time.Second

	var lastDiffErr error
	var processingErrors int
	var hasUnsuppressedVulns bool

	for _, u := range uploads {
		status, err := client.PollScanStatus(orgSlug, repoID, u.uploadID, timeout)
		if err != nil {
			if status != nil && status.Status == "failed" {
				output.Error("  %s: processing failed: %v", u.filename, err)
				processingErrors++
				continue
			}
			return &ExitError{Code: exitcode.Timeout, Err: err}
		}

		// Display summary per file
		output.Info("")
		output.Info("Scan Summary (%s)", u.filename)
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

		// Track whether any scan has unsuppressed vulnerabilities that meet the severity threshold
		if flagScanFailOnVulns {
			vulnCount := 0
			if status.UnsuppressedVulnerabilities != nil {
				vulnCount = *status.UnsuppressedVulnerabilities
			} else if status.TotalVulnerabilities != nil {
				// Fall back to total if server doesn't return unsuppressed count
				vulnCount = *status.TotalVulnerabilities
			}
			if vulnCount > 0 {
				// Check if the highest unsuppressed severity meets the threshold
				highestSev := ""
				if status.UnsuppressedHighestSeverity != nil {
					highestSev = *status.UnsuppressedHighestSeverity
				} else if status.HighestSeverity != nil {
					highestSev = *status.HighestSeverity
				}
				if severityMeetsThreshold(highestSev, flagScanFailOnSeverity) {
					hasUnsuppressedVulns = true
				}
			}
		}

		if status.ScanDiff != nil {
			if err := printScanDiff(status.ScanDiff, prNumber); err != nil {
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

	if processingErrors > 0 && len(uploads) == processingErrors {
		return &ExitError{Code: exitcode.UploadError, Err: fmt.Errorf("all processing failed")}
	}

	// Return the last diff failure if any scan diff failed policy
	if lastDiffErr != nil {
		return lastDiffErr
	}

	// Fail if --fail-on-vulnerabilities is set and unsuppressed vulnerabilities exist
	if flagScanFailOnVulns && hasUnsuppressedVulns {
		return &ExitError{Code: exitcode.ScanFailure, Err: fmt.Errorf("unsuppressed vulnerabilities found")}
	}

	return nil
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
