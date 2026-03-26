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

	results, total, parseErrors := validateJUnitFiles(files)

	for _, r := range results {
		output.Verbose("  %s: %d tests (%d passed, %d failed, %d skipped, %d errors)",
			r.Filename, r.Summary.Total, r.Summary.Passed, r.Summary.Failed, r.Summary.Skipped, r.Summary.Errored)
	}

	if parseErrors > 0 && len(results) == 0 {
		return &ExitError{Code: exitcode.ParseError, Err: fmt.Errorf("failed to parse any test result files")}
	}

	output.Info("")
	output.Info("Validation Summary")
	output.Info("  Files:   %d valid, %d failed to parse", len(results), parseErrors)
	output.Info("  Tests:   %d total", total.Total)
	output.Info("  Passed:  %d", total.Passed)
	output.Info("  Failed:  %d", total.Failed)
	output.Info("  Skipped: %d", total.Skipped)
	output.Info("  Errors:  %d", total.Errored)

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

	if parseErrors > 0 {
		return &ExitError{Code: exitcode.ParseError, Err: fmt.Errorf("%d file(s) failed to parse", parseErrors)}
	}

	return nil
}

// validatedFile holds the result of parsing a single JUnit XML file.
type validatedFile struct {
	Filename string
	Summary  junit.Summary
}

// validateJUnitFiles parses each file and returns per-file results, an overall
// summary, and the number of files that failed to parse.
func validateJUnitFiles(files []string) ([]validatedFile, junit.Summary, int) {
	var results []validatedFile
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
		results = append(results, validatedFile{
			Filename: filepath.Base(f),
			Summary:  summary,
		})
		allCases = append(allCases, cases...)
	}

	total := junit.Summarize(allCases)
	return results, total, parseErrors
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
