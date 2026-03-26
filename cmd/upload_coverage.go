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
	flagCovBranch       string
	flagCovSHA          string
	flagCovWait         bool
	flagCovTimeout      int
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
	uploadCoverageCmd.Flags().StringVar(&flagCovBranch, "branch", "", "Git branch (auto-detected from CI)")
	uploadCoverageCmd.Flags().StringVar(&flagCovSHA, "sha", "", "Git commit SHA (auto-detected from CI)")
	uploadCoverageCmd.Flags().BoolVar(&flagCovWait, "wait", true, "Wait for server-side processing")
	uploadCoverageCmd.Flags().IntVar(&flagCovTimeout, "timeout", 120, "Max wait time in seconds")
	uploadCoverageCmd.Flags().StringVar(&flagCovTargetBranch, "target-branch", "", "Target branch for PR diff (auto-detected from CI)")
	uploadCoverageCmd.Flags().StringVar(&flagCovRunDate, "run-date", "", "ISO 8601 date for historical uploads (e.g. 2026-03-15)")
	uploadCoverageCmd.Flags().StringSliceVar(&flagCovGroups, "group", nil, "Group label(s) for this upload (can be specified multiple times)")

	_ = uploadCoverageCmd.MarkFlagRequired("format")

	uploadCmd.AddCommand(uploadCoverageCmd)
}

func runUploadCoverage(cmd *cobra.Command, args []string) error {
	filePath := filepath.Clean(args[0])

	// Read file
	data, err := os.ReadFile(filePath) //nolint:gosec // G304: path is from CLI args, cleaned above
	if err != nil {
		return &ExitError{Code: exitcode.ParseError, Err: fmt.Errorf("reading file %s: %w", filePath, err)}
	}
	output.Info("Read %s (%d bytes)", filepath.Base(filePath), len(data))

	// Detect CI
	ci := cidetect.Detect(os.Getenv)
	if ci == nil {
		ci = cidetect.DetectFromGit()
	}

	branch := resolveGitContext(flagCovBranch, ci, func(info *cidetect.CIInfo) string { return info.Branch })
	sha := resolveGitContext(flagCovSHA, ci, func(info *cidetect.CIInfo) string { return info.CommitSHA })

	if branch == "" {
		return &ExitError{Code: exitcode.UsageError, Err: errMissing("--branch (could not auto-detect)")}
	}
	if sha == "" {
		return &ExitError{Code: exitcode.UsageError, Err: errMissing("--sha (could not auto-detect)")}
	}

	if flagDryRun {
		output.Info("[dry-run] Would upload %s (format: %s, branch: %s, sha: %s)", filepath.Base(filePath), flagCovFormat, branch, sha)
		return nil
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

	// Build request
	prNumber := 0
	if ci != nil && ci.PRNumber != "" {
		prNumber, _ = strconv.Atoi(ci.PRNumber)
	}

	targetBranch := resolveGitContext(flagCovTargetBranch, ci, func(info *cidetect.CIInfo) string { return info.TargetBranch })

	req := api.CoverageUploadRequest{
		Branch:       branch,
		SHA:          sha,
		Format:       flagCovFormat,
		Filename:     filepath.Base(filePath),
		PathPrefix:   flagCovPathPrefix,
		PRNumber:     prNumber,
		TargetBranch: targetBranch,
		RunDate:      flagCovRunDate,
		Group:        strings.Join(flagCovGroups, ","),
	}

	// Step 1: Initiate upload
	output.Verbose("Initiating coverage upload...")
	initResp, err := client.InitiateCoverageUpload(orgSlug, repoID, req)
	if err != nil {
		return &ExitError{Code: exitcode.UploadError, Err: err}
	}
	output.Verbose("Upload ID: %d, uploading to presigned URL...", initResp.UploadID)

	// Step 2: Upload to presigned URL
	if err := client.UploadToPresignedURL(initResp.UploadURL, data); err != nil {
		return &ExitError{Code: exitcode.UploadError, Err: err}
	}
	output.Verbose("File uploaded to object storage")

	// Step 3: Complete upload
	if err := client.CompleteCoverageUpload(orgSlug, repoID, initResp.UploadID); err != nil {
		return &ExitError{Code: exitcode.UploadError, Err: err}
	}
	output.Info("Coverage upload initiated (ID: %d)", initResp.UploadID)

	// Step 4: Wait for processing
	if flagCovWait {
		output.Info("Waiting for processing (timeout: %ds)...", flagCovTimeout)
		timeout := time.Duration(flagCovTimeout) * time.Second
		status, err := client.PollCoverageStatus(orgSlug, repoID, initResp.UploadID, timeout)
		if err != nil {
			if status != nil && status.Status == "failed" {
				return &ExitError{Code: exitcode.UploadError, Err: err}
			}
			return &ExitError{Code: exitcode.Timeout, Err: err}
		}

		output.Info("")
		output.Info("Coverage Summary")
		if status.CoverageRate != nil {
			output.Info("  Coverage rate: %s%%", *status.CoverageRate)
		}
		if status.FileCount != nil {
			output.Info("  Files:         %d", *status.FileCount)
		}

		// Display coverage diff results if present
		if status.CoverageDiff != nil {
			return printCoverageDiff(status.CoverageDiff, prNumber)
		}
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
