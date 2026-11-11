package cmd

import (
	"bytes"
	"encoding/binary"
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
	"unicode/utf16"

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

// checkChoice rejects a flag value outside allowed, before anything is read or uploaded.
func checkChoice(flag, value string, allowed []string) error {
	if slices.Contains(allowed, value) {
		return nil
	}
	return &ExitError{Code: exitcode.UsageError, Err: fmt.Errorf(
		"invalid %s value %q, must be one of: %s", flag, value, strings.Join(allowed, ", "))}
}

// reportCheckers check, before upload, that a report has the shape the
// server's parser for that format reads, without parsing it in full.
var reportCheckers = map[string]func([]byte) error{
	"cobertura": checkCobertura,
	"lcov":      checkLCOV,
	"go":        checkGoCoverage,
	"sarif": jsonObjectWith(func(o map[string]any) bool {
		version, _ := o["version"].(string)
		_, ok := o["runs"].([]any)
		return ok && strings.HasPrefix(version, "2.")
	}, `SARIF 2.x "version" and a "runs" list`),
	"cyclonedx": jsonObjectWith(isCycloneDX, `"bomFormat": "CycloneDX"`),
	"grype-json": jsonObjectWith(func(o map[string]any) bool {
		descriptor, _ := o["descriptor"].(map[string]any)
		_, hasMatches := o["matches"]
		return hasMatches && descriptor["name"] == "grype"
	}, `"matches" and a "descriptor" named grype`),
	"trivy-json": jsonObjectWith(func(o map[string]any) bool {
		_, isNumber := o["SchemaVersion"].(float64)
		_, hasResults := o["Results"].([]any)
		_, hasArtifactType := o["ArtifactType"]
		return isNumber && hasResults && hasArtifactType && !isCycloneDX(o)
	}, `"SchemaVersion", "ArtifactType" and a "Results" list`),
	"pip-audit-json": jsonObjectWith(func(o map[string]any) bool {
		_, hasDependencies := o["dependencies"].([]any)
		_, hasFixes := o["fixes"]
		return hasDependencies && hasFixes && !isCycloneDX(o)
	}, `a "dependencies" list and "fixes"`),
}

func checkCobertura(data []byte) error {
	// Read every token so a truncated or malformed document fails, not just a wrong root.
	dec := xml.NewDecoder(bytes.NewReader(data))
	// Go decodes only UTF-8, but the server reads any declared encoding. Read
	// each byte as its own rune: element names are ASCII, so the root check
	// holds for any single-byte charset.
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) {
		raw, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		runes := make([]rune, len(raw))
		for i, b := range raw {
			runes[i] = rune(b)
		}
		return strings.NewReader(string(runes)), nil
	}
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
			if !slices.ContainsFunc(start.Attr, func(a xml.Attr) bool { return a.Name.Local == "line-rate" }) {
				return errors.New(`<coverage> has no "line-rate" attribute`)
			}
			sawRoot = true
		}
	}
}

func checkLCOV(data []byte) error {
	text := string(data)
	hasSourceFile := false
	for line := range strings.Lines(text) {
		if strings.HasPrefix(line, "SF:") {
			hasSourceFile = true
			break
		}
	}
	if !hasSourceFile || !strings.Contains(text, "end_of_record") {
		return errors.New(`no "SF:" ... "end_of_record" source file records, so not LCOV`)
	}
	return nil
}

var goCoverageMode = regexp.MustCompile(`^mode:\s*(set|count|atomic)$`)

func checkGoCoverage(data []byte) error {
	first, rest, _ := strings.Cut(string(data), "\n")
	if !goCoverageMode.MatchString(strings.TrimSpace(first)) {
		return errors.New(`first line isn't "mode: set|count|atomic", so not Go coverage`)
	}
	if strings.TrimSpace(rest) == "" {
		return errors.New("no coverage blocks after the mode line")
	}
	return nil
}

func isCycloneDX(o map[string]any) bool {
	return o["bomFormat"] == "CycloneDX"
}

// jsonObjectWith returns a checker for a JSON object that has(o) accepts; want describes it.
func jsonObjectWith(has func(map[string]any) bool, want string) func([]byte) error {
	return func(data []byte) error {
		data, err := utf8JSON(data)
		if err != nil {
			return err
		}
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

// utf8JSON drops a UTF-8 byte-order mark and decodes UTF-16 marked by one,
// which Windows tools write and the server reads; Go's JSON decoder reads neither.
func utf8JSON(data []byte) ([]byte, error) {
	var order binary.ByteOrder
	switch {
	case bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}):
		return data[3:], nil
	case bytes.HasPrefix(data, []byte{0xff, 0xfe}):
		order = binary.LittleEndian
	case bytes.HasPrefix(data, []byte{0xfe, 0xff}):
		order = binary.BigEndian
	default:
		return data, nil
	}
	if len(data)%2 != 0 {
		return nil, errors.New("UTF-16 file has an odd number of bytes")
	}
	units := make([]uint16, 0, len(data)/2)
	for i := 2; i < len(data); i += 2 {
		units = append(units, order.Uint16(data[i:]))
	}
	return []byte(string(utf16.Decode(units))), nil
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
