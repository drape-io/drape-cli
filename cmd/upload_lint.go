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
	flagLintBranch  string
	flagLintSHA     string
	flagLintFormat  string
	flagLintWait    bool
	flagLintTimeout int
)

var uploadLintCmd = &cobra.Command{
	Use:   "lint <file>",
	Short: "Upload a lint report to Drape",
	Long:  "Upload a SARIF lint report to Drape for analysis.",
	Args:  cobra.ExactArgs(1),
	RunE:  runUploadLint,
}

func init() {
	uploadLintCmd.Flags().StringVar(&flagLintBranch, "branch", "", "Git branch (auto-detected from CI)")
	uploadLintCmd.Flags().StringVar(&flagLintSHA, "sha", "", "Git commit SHA (auto-detected from CI)")
	uploadLintCmd.Flags().StringVar(&flagLintFormat, "format", "sarif", "Lint report format (default: sarif)")
	uploadLintCmd.Flags().BoolVar(&flagLintWait, "wait", true, "Wait for server-side processing")
	uploadLintCmd.Flags().IntVar(&flagLintTimeout, "timeout", 120, "Max wait time in seconds")

	uploadCmd.AddCommand(uploadLintCmd)
}

func runUploadLint(cmd *cobra.Command, args []string) error {
	filePath := filepath.Clean(args[0])

	data, err := os.ReadFile(filePath) //nolint:gosec // G304: path is from CLI args, cleaned above
	if err != nil {
		return &ExitError{Code: exitcode.ParseError, Err: fmt.Errorf("reading file %s: %w", filePath, err)}
	}
	output.Info("Read %s (%d bytes)", filepath.Base(filePath), len(data))

	ci := cidetect.Detect(os.Getenv)
	if ci == nil {
		ci = cidetect.DetectFromGit()
	}

	branch := resolveGitContext(flagLintBranch, ci, func(info *cidetect.CIInfo) string { return info.Branch })
	sha := resolveGitContext(flagLintSHA, ci, func(info *cidetect.CIInfo) string { return info.CommitSHA })

	if branch == "" {
		return &ExitError{Code: exitcode.UsageError, Err: errMissing("--branch (could not auto-detect)")}
	}
	if sha == "" {
		return &ExitError{Code: exitcode.UsageError, Err: errMissing("--sha (could not auto-detect)")}
	}

	if flagDryRun {
		output.Info("[dry-run] Would upload %s (format: %s, branch: %s, sha: %s)", filepath.Base(filePath), flagLintFormat, branch, sha)
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

	filename := filepath.Base(filePath)
	metadata := map[string]any{
		"format": flagLintFormat,
	}
	if prNumber != 0 {
		metadata["pr_number"] = prNumber
	}

	initResp, err := client.InitiateUpload(orgSlug, repoID, api.UploadInitiateRequest{
		UploadType: "lint_report",
		Branch:     branch,
		SHA:        sha,
		Filename:   filename,
		Metadata:   metadata,
	})
	if err != nil {
		return &ExitError{Code: exitcode.UploadError, Err: err}
	}

	if err := client.UploadToPresignedURL(initResp.UploadURL, data); err != nil {
		return &ExitError{Code: exitcode.UploadError, Err: err}
	}

	if err := client.CompleteUpload(orgSlug, repoID, initResp.UploadID); err != nil {
		return &ExitError{Code: exitcode.UploadError, Err: err}
	}
	output.Info("Lint report uploaded (ID: %d)", initResp.UploadID)

	if !flagLintWait {
		return nil
	}

	output.Info("Waiting for processing (timeout: %ds)...", flagLintTimeout)
	timeout := time.Duration(flagLintTimeout) * time.Second
	status, err := client.PollLintStatus(orgSlug, repoID, initResp.UploadID, timeout)
	if err != nil {
		if status != nil && status.Status == "failed" {
			return &ExitError{Code: exitcode.UploadError, Err: err}
		}
		return &ExitError{Code: exitcode.Timeout, Err: err}
	}

	// Display summary
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
		return printLintDiff(status.LintDiff, prNumber)
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
