package cmd

import (
	"testing"

	"github.com/drape-io/drape-cli/internal/exitcode"
)

func TestTestUploadExitError(t *testing.T) {
	tests := []struct {
		name                      string
		totalFailed               int
		totalUnsuppressedFailures int
		processingErrors          int
		totalIngested             int
		wantNil                   bool
		wantCode                  int
	}{
		{
			name:    "no failures returns nil",
			wantNil: true,
		},
		{
			name:                      "all failures suppressed returns nil",
			totalFailed:               5,
			totalUnsuppressedFailures: 0,
			totalIngested:             100,
			wantNil:                   true,
		},
		{
			name:                      "unsuppressed failures returns TestFailure",
			totalFailed:               5,
			totalUnsuppressedFailures: 3,
			totalIngested:             100,
			wantCode:                  exitcode.TestFailure,
		},
		{
			name:             "all processing failed returns UploadError",
			processingErrors: 2,
			totalIngested:    0,
			wantCode:         exitcode.UploadError,
		},
		{
			name:             "partial processing failure with ingested tests returns nil",
			processingErrors: 1,
			totalIngested:    50,
			wantNil:          true,
		},
		{
			name:                      "unsuppressed failures take precedence over partial processing errors",
			totalFailed:               3,
			totalUnsuppressedFailures: 2,
			processingErrors:          1,
			totalIngested:             50,
			wantCode:                  exitcode.TestFailure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := testUploadExitError(tt.totalFailed, tt.totalUnsuppressedFailures, tt.processingErrors, tt.totalIngested)
			if tt.wantNil {
				if err != nil {
					t.Errorf("expected nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error with code %d, got nil", tt.wantCode)
			}
			exitErr, ok := err.(*ExitError)
			if !ok {
				t.Fatalf("expected *ExitError, got %T", err)
			}
			if exitErr.Code != tt.wantCode {
				t.Errorf("expected exit code %d, got %d", tt.wantCode, exitErr.Code)
			}
		})
	}
}
