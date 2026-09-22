package api

import "testing"

// A won't-fix CVE is reported in new_cves but excluded from the severity
// counts, so the headline only adds up once the non-actionable bucket is
// counted back in. Dropping the field would silently break that.
func TestMapScanDiffReconcilesNonActionable(t *testing.T) {
	m := map[string]any{
		"passed":                    true,
		"new_critical_count":        float64(0),
		"new_high_count":            float64(1),
		"new_medium_count":          float64(0),
		"new_low_count":             float64(0),
		"suppressed_cves_count":     float64(1),
		"non_actionable_cves_count": float64(2),
		"unchanged_cves_count":      float64(0),
		"new_cves": []any{
			map[string]any{"cve_id": "CVE-1", "severity": "high", "fix_state": "fixed"},
			map[string]any{"cve_id": "CVE-2", "severity": "high", "fix_state": "wont-fix"},
			map[string]any{"cve_id": "CVE-3", "severity": "high", "fix_state": "wont-fix"},
			map[string]any{"cve_id": "CVE-4", "severity": "high", "fix_state": "fixed"},
		},
	}

	diff := mapScanDiff(m)

	if diff.NonActionableCount != 2 {
		t.Errorf("expected non_actionable_cves_count=2, got %d", diff.NonActionableCount)
	}

	headline := diff.NewCriticalCount + diff.NewHighCount + diff.NewMediumCount + diff.NewLowCount
	total := headline + diff.SuppressedCVECount + diff.NonActionableCount
	if total != len(diff.NewCVEs) {
		t.Errorf(
			"headline %d + suppressed %d + non-actionable %d = %d, want %d listed CVEs",
			headline, diff.SuppressedCVECount, diff.NonActionableCount, total, len(diff.NewCVEs),
		)
	}
}

// An older server omits the key entirely; the count must read as zero rather
// than breaking the mapping.
func TestMapScanDiffWithoutNonActionableKey(t *testing.T) {
	diff := mapScanDiff(map[string]any{
		"passed":         true,
		"new_high_count": float64(1),
	})

	if diff.NonActionableCount != 0 {
		t.Errorf("expected 0 when the key is absent, got %d", diff.NonActionableCount)
	}
	if diff.NewHighCount != 1 {
		t.Errorf("expected new_high_count=1, got %d", diff.NewHighCount)
	}
}
