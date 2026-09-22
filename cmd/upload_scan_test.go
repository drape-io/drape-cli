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
