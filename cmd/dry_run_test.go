package cmd

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/drape-io/drape-cli/internal/exitcode"
	"github.com/drape-io/drape-cli/internal/output"
)

// writeReport writes content to a file named name in a fresh temp dir and returns its path.
func writeReport(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// dryRun runs `drape upload <args> --dry-run` offline and returns its exit code and stderr.
func dryRun(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var stderr bytes.Buffer
	output.Stderr = &stderr
	output.Stdout = &bytes.Buffer{}
	t.Cleanup(output.Reset)
	// Cobra keeps flag values between Execute calls; put back the defaults.
	t.Cleanup(func() {
		flagDryRun, flagJSON, flagQuiet = false, false, false
		flagTestFormat, flagCovFormat = "", ""
		flagLintFormat, flagScanFormat = "sarif", "sarif"
	})
	args = append([]string{"upload"}, args...)
	args = append(args, "--dry-run", "--branch", "main", "--sha", "abc123")
	rootCmd.SetArgs(args)
	return Execute(), stderr.String()
}

const (
	validJUnit     = `<testsuites><testsuite name="s"><testcase name="t"/></testsuite></testsuites>`
	validCobertura = `<?xml version="1.0"?><coverage line-rate="1.0"><packages/></coverage>`
	validLCOV      = "TN:\nSF:main.go\nDA:1,1\nend_of_record\n"
	validGo        = "mode: set\nexample.com/m/main.go:1.1,2.2 1 1\n"
	validSARIF     = `{"version": "2.1.0", "runs": []}`
	validCycloneDX = `{"bomFormat": "CycloneDX", "specVersion": "1.5", "components": []}`
	validGrype     = `{"matches": [], "descriptor": {"name": "grype"}}`
	validTrivy     = `{"SchemaVersion": 2, "ArtifactType": "container_image", "Results": []}`
	validPipAudit  = `{"dependencies": [], "fixes": []}`
)

func TestDryRunRejectsUnknownFormat(t *testing.T) {
	cases := map[string][]string{
		"tests":    {"tests", writeReport(t, "junit.xml", validJUnit)},
		"coverage": {"coverage", writeReport(t, "coverage.xml", validCobertura)},
		"lint":     {"lint", writeReport(t, "lint.sarif", validSARIF)},
		"scan":     {"scan", writeReport(t, "scan.sarif", validSARIF)},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			code, stderr := dryRun(t, append(args, "--format", "not-a-format")...)
			if code != exitcode.UsageError {
				t.Fatalf("exit = %d, want %d (usage error); stderr:\n%s", code, exitcode.UsageError, stderr)
			}
			if !strings.Contains(stderr, `"not-a-format"`) {
				t.Errorf("stderr should name the bad format; got:\n%s", stderr)
			}
		})
	}
}

// Drape has no CTRF parser, so the CLI must not accept or advertise it.
func TestTestsRejectCTRFFormat(t *testing.T) {
	code, stderr := dryRun(t, "tests", writeReport(t, "ctrf.json", `{"results": {"tests": []}}`), "--format", "ctrf")
	if code != exitcode.UsageError {
		t.Fatalf("exit = %d, want %d; stderr:\n%s", code, exitcode.UsageError, stderr)
	}
	if help := uploadTestsCmd.Flags().Lookup("format").Usage; strings.Contains(strings.ToLower(help), "ctrf") {
		t.Errorf("--format help still advertises CTRF: %q", help)
	}
}

func TestDryRunAcceptsEveryValidReport(t *testing.T) {
	cases := map[string][]string{
		"junit default":  {"tests", writeReport(t, "junit.xml", validJUnit)},
		"junit explicit": {"tests", writeReport(t, "junit.xml", validJUnit), "--format", "junit"},
		"cobertura":      {"coverage", writeReport(t, "coverage.xml", validCobertura), "--format", "cobertura"},
		"lcov":           {"coverage", writeReport(t, "lcov.info", validLCOV), "--format", "lcov"},
		"go":             {"coverage", writeReport(t, "cover.out", validGo), "--format", "go"},
		"lint sarif":     {"lint", writeReport(t, "lint.sarif", validSARIF)},
		"scan sarif":     {"scan", writeReport(t, "scan.sarif", validSARIF)},
		"cyclonedx":      {"scan", writeReport(t, "bom.json", validCycloneDX), "--format", "cyclonedx"},
		"grype":          {"scan", writeReport(t, "grype.json", validGrype), "--format", "grype-json"},
		"trivy":          {"scan", writeReport(t, "trivy.json", validTrivy), "--format", "trivy-json"},
		"pip-audit":      {"scan", writeReport(t, "audit.json", validPipAudit), "--format", "pip-audit-json"},
		"coverage batch": {"coverage", writeReport(t, "a.xml", validCobertura), writeReport(t, "b.xml", validCobertura), "--format", "cobertura"},
		// Windows tools write these; the server's parsers read them.
		"sarif with utf-8 bom": {"lint", writeReport(t, "lint.sarif", "\xef\xbb\xbf"+validSARIF)},
		"sarif in utf-16":      {"scan", writeReport(t, "scan.sarif", utf16LE(validSARIF))},
		"latin-1 cobertura": {"coverage", writeReport(t, "c.xml",
			"<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><coverage line-rate=\"1.0\"><sources><source>caf\xe9</source></sources></coverage>"),
			"--format", "cobertura"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			if code, stderr := dryRun(t, args...); code != exitcode.Success {
				t.Fatalf("exit = %d, want 0; stderr:\n%s", code, stderr)
			}
		})
	}
}

func TestDryRunRejectsReportsThatDoNotMatchTheirFormat(t *testing.T) {
	broken := writeReport(t, "broken.json", `{"runs": [`)
	cases := map[string][]string{
		"malformed cobertura":     {"coverage", writeReport(t, "c.xml", `<coverage line-rate="1"`), "--format", "cobertura"},
		"lcov given as cobertura": {"coverage", writeReport(t, "lcov.info", validLCOV), "--format", "cobertura"},
		"cobertura given as lcov": {"coverage", writeReport(t, "c.xml", validCobertura), "--format", "lcov"},
		"not go coverage":         {"coverage", writeReport(t, "cover.out", validLCOV), "--format", "go"},
		"directory":               {"coverage", t.TempDir(), "--format", "cobertura"},
		"mixed batch": {"coverage", writeReport(t, "a.xml", validCobertura), writeReport(t, "lcov.info", validLCOV),
			"--format", "cobertura"},
		"malformed lint":              {"lint", broken},
		"malformed scan":              {"scan", broken},
		"sarif not cyclonedx":         {"scan", writeReport(t, "bom.json", validSARIF), "--format", "cyclonedx"},
		"sarif not grype":             {"scan", writeReport(t, "grype.json", validSARIF), "--format", "grype-json"},
		"sarif not trivy":             {"scan", writeReport(t, "trivy.json", validSARIF), "--format", "trivy-json"},
		"sarif not pip-audit":         {"scan", writeReport(t, "audit.json", validSARIF), "--format", "pip-audit-json"},
		"json array not sarif":        {"lint", writeReport(t, "lint.sarif", `[]`)},
		"odd-length utf-16":           {"lint", writeReport(t, "lint.sarif", utf16LE(validSARIF)+"x")},
		"sarif 1.x":                   {"lint", writeReport(t, "lint.sarif", `{"version": "1.0.0", "runs": []}`)},
		"cobertura without line-rate": {"coverage", writeReport(t, "c.xml", `<coverage/>`), "--format", "cobertura"},
		"lcov without end_of_record":  {"coverage", writeReport(t, "lcov.info", "SF:main.go\nDA:1,1\n"), "--format", "lcov"},
		"go mode line only":           {"coverage", writeReport(t, "cover.out", "mode: set\n"), "--format", "go"},
		"grype-shaped syft output": {"scan", writeReport(t, "grype.json", `{"matches": [], "descriptor": {"name": "syft"}}`),
			"--format", "grype-json"},
		"trivy without Results": {"scan", writeReport(t, "trivy.json", `{"SchemaVersion": 2, "ArtifactType": "x"}`),
			"--format", "trivy-json"},
		"cyclonedx given as pip-audit": {"scan", writeReport(t, "bom.json",
			`{"bomFormat": "CycloneDX", "dependencies": [], "fixes": []}`), "--format", "pip-audit-json"},
		"cyclonedx given as trivy": {"scan", writeReport(t, "bom.json",
			`{"bomFormat": "CycloneDX", "SchemaVersion": 2, "ArtifactType": "x", "Results": []}`), "--format", "trivy-json"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			if code, stderr := dryRun(t, args...); code != exitcode.ParseError {
				t.Fatalf("exit = %d, want %d (parse error); stderr:\n%s", code, exitcode.ParseError, stderr)
			}
		})
	}
}

// utf16LE encodes s as UTF-16 little-endian with a byte-order mark, as Windows PowerShell writes files.
func utf16LE(s string) string {
	units := utf16.Encode([]rune(s))
	b := make([]byte, 0, 2+2*len(units))
	b = append(b, 0xff, 0xfe)
	for _, u := range units {
		b = binary.LittleEndian.AppendUint16(b, u)
	}
	return string(b)
}

// A batch's aggregate result isn't any one upload, so it must not carry an upload_id of 0.
func TestBatchResultHasNoZeroUploadID(t *testing.T) {
	var stdout bytes.Buffer
	output.Stdout = &stdout
	output.Stderr = &bytes.Buffer{}
	t.Cleanup(output.Reset)
	t.Cleanup(func() { pendingJSON = nil })

	result := UploadResult{FilesMatched: 2, FilesUploaded: 2, Uploads: []UploadEntry{
		{Filename: "a.xml", UploadID: 8},
		{Filename: "b.xml", UploadID: 9},
	}}
	if err := renderBatchResult(result, completedBatchStatus(nil), 0); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(pendingJSON)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"upload_id":0`) {
		t.Errorf("batch JSON carries upload_id 0: %s", raw)
	}
}
