package cmd

import "testing"

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
