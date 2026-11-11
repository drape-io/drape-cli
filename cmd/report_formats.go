package cmd

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/drape-io/drape-cli/internal/exitcode"
	"github.com/drape-io/drape-cli/internal/output"
)

// The report formats Drape has a parser for, per upload type. Keep these in
// step with the server: a format listed here and missing there fails only
// after upload.
var (
	testFormats     = []string{"junit"}
	coverageFormats = []string{"cobertura", "lcov", "go"}
	lintFormats     = []string{"sarif"}
	scanFormats     = []string{"sarif", "cyclonedx", "grype-json", "trivy-json", "pip-audit-json"}
)

// checkFormat rejects a --format value Drape can't parse, before anything is read or uploaded.
func checkFormat(format string, allowed []string) error {
	if slices.Contains(allowed, format) {
		return nil
	}
	return &ExitError{Code: exitcode.UsageError, Err: fmt.Errorf(
		"unsupported --format %q, must be one of: %s", format, strings.Join(allowed, ", "))}
}

// reportCheckers check that a report has the shape the server's parser for
// that format reads. They catch a broken file or the wrong format before
// upload; they don't parse the report in full.
var reportCheckers = map[string]func([]byte) error{
	"cobertura": checkCobertura,
	"lcov":      checkLCOV,
	"go":        checkGoCoverage,
	"sarif": jsonObjectWith(func(o map[string]any) bool {
		_, ok := o["runs"].([]any)
		return ok
	}, `a "runs" list`),
	"cyclonedx": jsonObjectWith(func(o map[string]any) bool {
		return o["bomFormat"] == "CycloneDX"
	}, `"bomFormat": "CycloneDX"`),
	"grype-json": jsonObjectWith(func(o map[string]any) bool {
		_, ok := o["matches"]
		return ok
	}, `a "matches" field`),
	"trivy-json": jsonObjectWith(func(o map[string]any) bool {
		_, ok := o["SchemaVersion"]
		return ok
	}, `a "SchemaVersion" field`),
	"pip-audit-json": jsonObjectWith(func(o map[string]any) bool {
		_, ok := o["dependencies"].([]any)
		return ok
	}, `a "dependencies" list`),
}

func checkCobertura(data []byte) error {
	// Read every token so a truncated or malformed document fails, not just a wrong root.
	dec := xml.NewDecoder(bytes.NewReader(data))
	sawRoot := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) && sawRoot {
			return nil
		}
		if err != nil {
			return fmt.Errorf("not valid XML: %w", err)
		}
		if start, ok := tok.(xml.StartElement); ok && !sawRoot {
			if start.Name.Local != "coverage" {
				return fmt.Errorf("root element is <%s>, Cobertura XML starts with <coverage>", start.Name.Local)
			}
			sawRoot = true
		}
	}
}

func checkLCOV(data []byte) error {
	for line := range strings.Lines(string(data)) {
		if strings.HasPrefix(line, "SF:") {
			return nil
		}
	}
	return errors.New(`no "SF:" source file records, so not LCOV`)
}

var goCoverageMode = regexp.MustCompile(`^mode:\s*(set|count|atomic)$`)

func checkGoCoverage(data []byte) error {
	first, _, _ := strings.Cut(string(data), "\n")
	if goCoverageMode.MatchString(strings.TrimSpace(first)) {
		return nil
	}
	return errors.New(`first line isn't "mode: set|count|atomic", so not Go coverage`)
}

// jsonObjectWith returns a checker for a JSON object that has(o) accepts; want describes it.
func jsonObjectWith(has func(map[string]any) bool, want string) func([]byte) error {
	return func(data []byte) error {
		var o map[string]any
		if err := json.Unmarshal(data, &o); err != nil {
			return fmt.Errorf("not a JSON object: %w", err)
		}
		if !has(o) {
			return fmt.Errorf("JSON object has no %s", want)
		}
		return nil
	}
}

// dryRunCheck checks each report against its format without uploading and
// reports what an upload would send. It fails with a parse error if any file
// can't be read or doesn't match the format.
func (ctx *uploadContext) dryRunCheck(files []string, format string) error {
	output.Info("[dry-run] Checking files locally, no upload will be performed")
	check := reportCheckers[format]
	names := make([]string, 0, len(files))
	var failed int
	for _, f := range files {
		name := filepath.Base(f)
		data, err := os.ReadFile(filepath.Clean(f)) //nolint:gosec // G304: path is from CLI args + glob expansion
		if err == nil {
			err = check(data)
		}
		if err != nil {
			output.Error("  %s: not a valid %s report: %v", name, format, err)
			failed++
			continue
		}
		output.Info("  %s: ok, would upload (format: %s, branch: %s, sha: %s)", name, format, ctx.branch, ctx.sha)
		names = append(names, name)
	}
	if flagJSON {
		setResult(DryRunResult{DryRun: true, Files: names})
	}
	if failed > 0 {
		return &ExitError{Code: exitcode.ParseError, Err: fmt.Errorf("%d file(s) failed validation", failed)}
	}
	return nil
}
