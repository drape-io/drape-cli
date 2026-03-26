package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/spf13/cobra"

	"github.com/drape-io/drape-cli/internal/exitcode"
	"github.com/drape-io/drape-cli/internal/junit"
	"github.com/drape-io/drape-cli/internal/output"
)

var validateTestsCmd = &cobra.Command{
	Use:   "tests <glob>",
	Short: "Validate JUnit XML test result files locally",
	Long:  "Parse and validate JUnit XML files without uploading. Useful for checking file format before CI integration.",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runValidateTests,
}

func init() {
	validateCmd.AddCommand(validateTestsCmd)
}

func runValidateTests(cmd *cobra.Command, args []string) error {
	files, err := expandGlobs(args)
	if err != nil {
		return &ExitError{Code: exitcode.UsageError, Err: err}
	}

	if len(files) == 0 {
		return &ExitError{Code: exitcode.ParseError, Err: fmt.Errorf("no files matched the given pattern(s)")}
	}

	output.Info("Found %d file(s) to validate", len(files))

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
		output.Verbose("  %s: %d tests (%d passed, %d failed, %d skipped, %d errors)",
			filepath.Base(f), summary.Total, summary.Passed, summary.Failed, summary.Skipped, summary.Errored)
		allCases = append(allCases, cases...)
	}

	if parseErrors > 0 && len(allCases) == 0 {
		return &ExitError{Code: exitcode.ParseError, Err: fmt.Errorf("failed to parse any test result files")}
	}

	summary := junit.Summarize(allCases)
	output.Info("")
	output.Info("Validation Summary")
	output.Info("  Files:   %d valid, %d failed to parse", len(files)-parseErrors, parseErrors)
	output.Info("  Tests:   %d total", summary.Total)
	output.Info("  Passed:  %d", summary.Passed)
	output.Info("  Failed:  %d", summary.Failed)
	output.Info("  Skipped: %d", summary.Skipped)
	output.Info("  Errors:  %d", summary.Errored)

	if parseErrors > 0 {
		return &ExitError{Code: exitcode.ParseError, Err: fmt.Errorf("%d file(s) failed to parse", parseErrors)}
	}

	return nil
}

// expandGlobs expands glob patterns (including ** recursive patterns) into file paths.
func expandGlobs(patterns []string) ([]string, error) {
	seen := make(map[string]bool)
	var files []string

	for _, pattern := range patterns {
		matches, err := doublestar.FilepathGlob(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
		}
		for _, m := range matches {
			abs, err := filepath.Abs(m)
			if err != nil {
				abs = m
			}
			if seen[abs] {
				continue
			}
			seen[abs] = true
			files = append(files, m)
		}
	}

	return files, nil
}
