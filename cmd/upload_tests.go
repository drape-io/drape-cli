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
	"github.com/drape-io/drape-cli/internal/junit"
	"github.com/drape-io/drape-cli/internal/output"
)

var (
	flagBranch   string
	flagSHA      string
	flagFormat   string
	flagJobName  string
	flagPRNumber int
	flagWait     bool
	flagTimeout  int
	flagRunDate  string
	flagGroups   []string
)

var uploadTestsCmd = &cobra.Command{
	Use:   "tests <glob>",
	Short: "Upload JUnit XML test results to Drape",
	Long:  "Find test result files matching the glob pattern, upload them to Drape, and print a summary.",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runUploadTests,
}

func init() {
	uploadTestsCmd.Flags().StringVar(&flagBranch, "branch", "", "Git branch (auto-detected from CI)")
	uploadTestsCmd.Flags().StringVar(&flagSHA, "sha", "", "Git commit SHA (auto-detected from CI)")
	uploadTestsCmd.Flags().StringVar(&flagFormat, "format", "", "Force format (junit, ctrf). Default: auto-detect")
	uploadTestsCmd.Flags().StringVar(&flagJobName, "job-name", "", "CI job name (auto-detected from CI)")
	uploadTestsCmd.Flags().IntVar(&flagPRNumber, "pr-number", 0, "PR number (auto-detected from CI)")
	uploadTestsCmd.Flags().BoolVar(&flagWait, "wait", true, "Wait for server-side processing before exiting")
	uploadTestsCmd.Flags().IntVar(&flagTimeout, "timeout", 120, "Max wait time in seconds for processing")
	uploadTestsCmd.Flags().StringVar(&flagRunDate, "run-date", "", "ISO 8601 date for historical uploads (e.g. 2026-03-15)")
	uploadTestsCmd.Flags().StringSliceVar(&flagGroups, "group", nil, "Group label(s) for this upload (can be specified multiple times)")

	uploadCmd.AddCommand(uploadTestsCmd)
}

func runUploadTests(cmd *cobra.Command, args []string) error {
	// Expand glob patterns
	files, err := expandGlobs(args)
	if err != nil {
		return &ExitError{Code: exitcode.UsageError, Err: err}
	}
	if len(files) == 0 {
		return &ExitError{Code: exitcode.ParseError, Err: fmt.Errorf("no files matched the given pattern(s)")}
	}

	output.Info("Found %d file(s) to upload", len(files))

	// Detect CI environment
	ci := cidetect.Detect(os.Getenv)
	if ci == nil {
		ci = cidetect.DetectFromGit()
	}

	branch := resolveGitContext(flagBranch, ci, func(info *cidetect.CIInfo) string { return info.Branch })
	sha := resolveGitContext(flagSHA, ci, func(info *cidetect.CIInfo) string { return info.CommitSHA })

	if branch == "" {
		return &ExitError{Code: exitcode.UsageError, Err: errMissing("--branch (could not auto-detect)")}
	}
	if sha == "" {
		return &ExitError{Code: exitcode.UsageError, Err: errMissing("--sha (could not auto-detect)")}
	}

	if ci != nil {
		output.Verbose("Detected CI: %s", ci.ProviderName)
		output.Verbose("  Branch: %s, SHA: %s", branch, sha)
		if ci.IsPullRequest {
			output.Verbose("  PR #%s → %s", ci.PRNumber, ci.TargetBranch)
		}
	}

	// Dry run: validate locally only
	if flagDryRun {
		return dryRunValidate(files)
	}

	// Resolve org and repo
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

	// Build metadata
	prNumber := flagPRNumber
	if prNumber == 0 && ci != nil && ci.PRNumber != "" {
		prNumber, _ = strconv.Atoi(ci.PRNumber)
	}

	metadata := api.TestUploadMetadata{
		Branch:   branch,
		SHA:      sha,
		Format:   flagFormat,
		JobName:  flagJobName,
		PRNumber: prNumber,
	}
	if ci != nil {
		metadata.ProviderType = ci.Provider
		if metadata.JobName == "" {
			metadata.JobName = ci.JobID
		}
	}
	if flagRunDate != "" {
		metadata.RunDate = flagRunDate
	}
	if len(flagGroups) > 0 {
		metadata.Group = strings.Join(flagGroups, ",")
	}

	// Upload each file via unified upload flow
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

		// Step 1: Initiate upload
		initResp, err := client.InitiateTestUpload(orgSlug, repoID, metadata)
		if err != nil {
			output.Error("Failed to initiate upload for %s: %v", filename, err)
			uploadErrors++
			continue
		}

		// Step 2: Upload to presigned URL
		if err := client.UploadToPresignedURL(initResp.UploadURL, data); err != nil {
			output.Error("Failed to upload %s to storage: %v", filename, err)
			uploadErrors++
			continue
		}

		// Step 3: Complete upload
		if err := client.CompleteTestUpload(orgSlug, repoID, initResp.UploadID); err != nil {
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

	// Step 4: Wait for processing if requested
	if !flagWait {
		for _, u := range uploads {
			output.Info("  %s: processing (ID: %d)", u.filename, u.uploadID)
		}
		return nil
	}

	output.Info("Waiting for processing (timeout: %ds)...", flagTimeout)
	timeout := time.Duration(flagTimeout) * time.Second

	var totalIngested int
	var totalQuarantined int
	var totalFailed int
	var totalUnquarantinedFailures int
	var processingErrors int

	for _, u := range uploads {
		status, err := client.PollTestStatus(orgSlug, repoID, u.uploadID, timeout)
		if err != nil {
			if status != nil && status.Status == "failed" {
				output.Error("  %s: processing failed: %v", u.filename, err)
				processingErrors++
				continue
			}
			return &ExitError{Code: exitcode.Timeout, Err: err}
		}

		ingested := 0
		if status.TestsIngested != nil {
			ingested = *status.TestsIngested
		}
		output.Verbose("  %s: %d tests ingested", u.filename, ingested)

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

	// Summary
	output.Info("")
	output.Info("Upload Summary")
	output.Info("  Files uploaded: %d/%d", len(uploads), len(files))
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

	// Exit code override: if all failures are quarantined, pass CI
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

// resolveGitContext returns the flag value if set, otherwise the CI-detected value.
func resolveGitContext(flagVal string, ci *cidetect.CIInfo, getter func(*cidetect.CIInfo) string) string {
	if flagVal != "" {
		return flagVal
	}
	if ci != nil {
		return getter(ci)
	}
	return ""
}

func dryRunValidate(files []string) error {
	output.Info("[dry-run] Validating files locally, no upload will be performed")

	var allCases []junit.TestCase
	var parseErrors int

	for _, f := range files {
		data, err := os.ReadFile(filepath.Clean(f)) //nolint:gosec // G304: path is from CLI args + glob expansion
		if err != nil {
			output.Error("Failed to read %s: %v", f, err)
			parseErrors++
			continue
		}

		cases, err := junit.Parse(data)
		if err != nil {
			output.Error("Failed to parse %s: %v", f, err)
			parseErrors++
			continue
		}

		summary := junit.Summarize(cases)
		output.Info("  %s: %d tests (%d passed, %d failed, %d skipped, %d errors)",
			filepath.Base(f), summary.Total, summary.Passed, summary.Failed, summary.Skipped, summary.Errored)
		allCases = append(allCases, cases...)
	}

	summary := junit.Summarize(allCases)
	output.Info("")
	output.Info("[dry-run] Total: %d tests (%d passed, %d failed, %d skipped, %d errors)",
		summary.Total, summary.Passed, summary.Failed, summary.Skipped, summary.Errored)

	if parseErrors > 0 {
		return &ExitError{Code: exitcode.ParseError, Err: fmt.Errorf("%d file(s) failed to parse", parseErrors)}
	}
	return nil
}
