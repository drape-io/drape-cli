package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/drape-io/drape-cli/internal/api"
	"github.com/drape-io/drape-cli/internal/output"
)

func TestSeverityMeetsThreshold(t *testing.T) {
	tests := []struct {
		severity  string
		threshold string
		want      bool
	}{
		// "any" threshold matches everything
		{"critical", "any", true},
		{"low", "any", true},
		{"unknown", "any", true},

		// "medium" threshold (default) — fails on medium, high, critical
		{"critical", "medium", true},
		{"high", "medium", true},
		{"medium", "medium", true},
		{"low", "medium", false},
		{"unknown", "medium", false},

		// "critical" threshold — only fails on critical
		{"critical", "critical", true},
		{"high", "critical", false},
		{"medium", "critical", false},
		{"low", "critical", false},
		{"unknown", "critical", false},

		// "low" threshold — fails on everything except unknown
		{"critical", "low", true},
		{"high", "low", true},
		{"medium", "low", true},
		{"low", "low", true},
		{"unknown", "low", false},

		// "high" threshold
		{"critical", "high", true},
		{"high", "high", true},
		{"medium", "high", false},
		{"low", "high", false},
	}

	for _, tt := range tests {
		t.Run(tt.severity+"_"+tt.threshold, func(t *testing.T) {
			got := severityMeetsThreshold(tt.severity, tt.threshold)
			if got != tt.want {
				t.Errorf("severityMeetsThreshold(%q, %q) = %v, want %v", tt.severity, tt.threshold, got, tt.want)
			}
		})
	}
}

// The pass message must cover won't-fix CVEs too, not just suppressions.
func TestPrintScanDiffPassMessageCoversAllNonBlockingCVEs(t *testing.T) {
	tests := []struct {
		name          string
		suppressed    int
		nonActionable int
	}{
		{"suppressed and won't-fix", 1, 97},
		{"won't-fix only", 0, 97},
		{"suppressed only", 4, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			output.Stdout = &buf
			defer output.Reset()

			total := tt.suppressed + tt.nonActionable
			diff := &api.ScanDiffInfo{
				Passed:             true,
				SuppressedCVECount: tt.suppressed,
				NonActionableCount: tt.nonActionable,
			}
			if err := printScanDiff(diff, 0); err != nil {
				t.Fatalf("printScanDiff() error: %v", err)
			}

			got := buf.String()
			if !strings.Contains(got, "passing CI") {
				t.Fatalf("no pass explanation printed for %d non-blocking CVE(s)\nGot:\n%s", total, got)
			}
			if want := fmt.Sprintf("All %d new CVE(s)", total); !strings.Contains(got, want) {
				t.Errorf("pass explanation should name all %d non-blocking CVE(s)\nGot:\n%s", total, got)
			}
		})
	}
}

// With a diff, the gate counts only the CVEs the diff calls new. A PR whose
// only remaining findings are unchanged from the baseline must pass, even
// though the whole-scan unsuppressed count is non-zero.
func TestScanFailsOnVulnsGatesOnNewCVEsWhenDiffPresent(t *testing.T) {
	high := "high"
	unsuppressed := 3

	tests := []struct {
		name      string
		diff      *api.ScanDiffInfo
		threshold string
		want      bool
	}{
		{"no new CVEs, existing ones unchanged", &api.ScanDiffInfo{UnchangedCVECount: 55}, "medium", false},
		{"new medium at medium threshold", &api.ScanDiffInfo{NewMediumCount: 1}, "medium", true},
		{"new low below medium threshold", &api.ScanDiffInfo{NewLowCount: 2}, "medium", false},
		{"new high at critical threshold", &api.ScanDiffInfo{NewHighCount: 1}, "critical", false},
		{"new critical at critical threshold", &api.ScanDiffInfo{NewCriticalCount: 1}, "critical", true},
		{"new unknown only counts for any", &api.ScanDiffInfo{NewUnknownCount: 1}, "low", false},
		{"new unknown with any", &api.ScanDiffInfo{NewUnknownCount: 1}, "any", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &api.ScanStatusResponse{
				UnsuppressedVulnerabilities: &unsuppressed,
				UnsuppressedHighestSeverity: &high,
				ScanDiff:                    tt.diff,
			}
			if got := scanFailsOnVulns(status, tt.threshold); got != tt.want {
				t.Errorf("scanFailsOnVulns() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Without a diff (an older server), fall back to the whole-scan count.
func TestScanFailsOnVulnsFallsBackToWholeScanWithoutDiff(t *testing.T) {
	high := "high"
	unsuppressed := 3
	status := &api.ScanStatusResponse{
		UnsuppressedVulnerabilities: &unsuppressed,
		UnsuppressedHighestSeverity: &high,
	}
	if !scanFailsOnVulns(status, "medium") {
		t.Error("scanFailsOnVulns() = false, want true for a high finding with no diff")
	}
}

// Existing CVEs that still need action don't block the run, but the output
// must name them rather than leave only a count.
func TestPrintScanDiffListsExistingActionableCVEs(t *testing.T) {
	var buf bytes.Buffer
	output.Stdout = &buf
	defer output.Reset()

	diff := &api.ScanDiffInfo{
		Passed:            true,
		UnchangedCVECount: 55,
		ExistingActionableCVEs: []api.ScanCVE{
			{CVEID: "CVE-2026-0001", Severity: "high", PackageName: "zlib1g", PackageVersion: "1.3", FixState: "not-fixed"},
		},
	}
	if err := printScanDiff(diff, 0); err != nil {
		t.Fatalf("printScanDiff() error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "Need action: 1") {
		t.Errorf("missing existing-needs-action count\nGot:\n%s", got)
	}
	if !strings.Contains(got, "CVE-2026-0001 [high] zlib1g@1.3 (not-fixed)") {
		t.Errorf("missing existing CVE line\nGot:\n%s", got)
	}
}

// With per-CVE counts from the server, the summary counts each CVE once so it
// matches the scan page, while the per-package lists still print in full.
func TestPrintScanDiffPrefersPerCVECounts(t *testing.T) {
	var buf bytes.Buffer
	output.Stdout = &buf
	defer output.Reset()

	diff := &api.ScanDiffInfo{
		Passed:             false,
		NewHighCount:       2,
		NonActionableCount: 4,
		UnchangedCVECount:  40,
		CVECounts: &api.CVECounts{
			New:      map[string]int{"high": 1},
			Existing: 3,
			NoFix:    2,
		},
	}
	_ = printScanDiff(diff, 0)

	got := buf.String()
	for _, want := range []string{
		"New CVEs:    1 (critical: 0, high: 1, medium: 0, low: 0)",
		"Won't fix:   2",
		"Need action: 3",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q\nGot:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Unchanged") {
		t.Errorf("per-package unchanged count printed next to per-CVE counts\nGot:\n%s", got)
	}
}

// Until the server returns a report_url, link to a page the webapp serves.
// The old fallback used /repos/ and 404'd.
func TestDrapeURLFallbackUsesTheRepositoriesRoute(t *testing.T) {
	ctx := &uploadContext{
		client:   &api.Client{BaseURL: "https://drape.example"},
		orgSlug:  "acme",
		repoName: "acme/app",
	}
	want := "https://drape.example/orgs/acme/repositories/acme/app/uploads/"
	if got := ctx.drapeURL(); got != want {
		t.Errorf("drapeURL() = %q, want %q", got, want)
	}
}
