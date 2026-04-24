package api

import (
	"time"
)

// TestStatusResponse is the CLI-facing status for a test upload.
// Extracted from the unified UploadStatusResponse result dict.
type TestStatusResponse struct {
	UploadID                  int      `json:"upload_id"`
	Status                    string   `json:"status"`
	TestsIngested             *int     `json:"tests_ingested,omitempty"`
	SuppressedCount          *int     `json:"suppressed_count,omitempty"`
	SuppressedTests          []string `json:"suppressed_tests,omitempty"`
	FailedCount              *int     `json:"failed_count,omitempty"`
	UnsuppressedFailureCount *int     `json:"unsuppressed_failure_count,omitempty"`
	NewTestsDetected         []string `json:"new_tests_detected,omitempty"`
	ErrorMessage              *string  `json:"error_message,omitempty"`
}

// PollTestStatus polls the upload status until it completes or times out.
func (c *Client) PollTestStatus(orgSlug string, repoID, uploadID int, timeout time.Duration) (*TestStatusResponse, error) {
	raw, err := c.PollUploadStatus(orgSlug, repoID, uploadID, timeout, "Test results")
	if err != nil {
		if raw != nil {
			return &TestStatusResponse{
				UploadID:     raw.UploadID,
				Status:       raw.Status,
				ErrorMessage: raw.ErrorMessage,
			}, err
		}
		return nil, err
	}

	return mapTestStatus(raw), nil
}

func mapTestStatus(raw *UploadStatusResponse) *TestStatusResponse {
	result := &TestStatusResponse{
		UploadID:     raw.UploadID,
		Status:       raw.Status,
		ErrorMessage: raw.ErrorMessage,
	}

	if raw.Result != nil {
		result.TestsIngested = getInt(raw.Result, "tests_ingested")
		result.SuppressedCount = getInt(raw.Result, "suppressed_count")
		result.SuppressedTests = getStringSlice(raw.Result, "suppressed_tests")
		result.FailedCount = getInt(raw.Result, "failed_count")
		result.UnsuppressedFailureCount = getInt(raw.Result, "unsuppressed_failure_count")
		result.NewTestsDetected = getStringSlice(raw.Result, "new_tests_detected")
	}

	return result
}
