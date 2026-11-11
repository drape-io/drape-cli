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
		failedFiles               int
		totalFiles                int
		wantNil                   bool
		wantCode                  int
	}{
		{
			name:       "no failures returns nil",
			totalFiles: 2,
			wantNil:    true,
		},
		{
			name:                      "all failures suppressed returns nil",
			totalFailed:               5,
			totalUnsuppressedFailures: 0,
			totalFiles:                2,
			wantNil:                   true,
		},
		{
			name:                      "unsuppressed failures returns TestFailure",
			totalFailed:               5,
			totalUnsuppressedFailures: 3,
			totalFiles:                2,
			wantCode:                  exitcode.TestFailure,
		},
		{
			name:        "every file failing returns UploadError",
			failedFiles: 2,
			totalFiles:  2,
			wantCode:    exitcode.UploadError,
		},
		{
			name:        "one file failing returns UploadError though others ingested",
			failedFiles: 1,
			totalFiles:  2,
			wantCode:    exitcode.UploadError,
		},
		{
			name:        "a failed file with all test failures suppressed returns UploadError",
			totalFailed: 3,
			failedFiles: 1,
			totalFiles:  2,
			wantCode:    exitcode.UploadError,
		},
		{
			name:                      "unsuppressed failures take precedence over failed files",
			totalFailed:               3,
			totalUnsuppressedFailures: 2,
			failedFiles:               1,
			totalFiles:                2,
			wantCode:                  exitcode.TestFailure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := testUploadExitError(tt.totalFailed, tt.totalUnsuppressedFailures, tt.failedFiles, tt.totalFiles)
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
