package api

import (
	"time"
)

// TestUploadMetadata contains test-specific metadata for the upload.
type TestUploadMetadata struct {
	Branch        string `json:"branch"`
	SHA           string `json:"sha"`
	Format        string `json:"format,omitempty"`
	ProviderType  string `json:"provider_type,omitempty"`
	ProviderRunID string `json:"provider_run_id,omitempty"`
	JobName       string `json:"job_name,omitempty"`
	PRNumber      int    `json:"pr_number,omitempty"`
	PRTitle       string `json:"pr_title,omitempty"`
	PRURL         string `json:"pr_url,omitempty"`
	PRAuthor      string `json:"pr_author,omitempty"`
	RunDate       string `json:"run_date,omitempty"`
	Group         string `json:"group,omitempty"`
}

// TestInitiateResponse matches the unified UploadInitiateResponse.
type TestInitiateResponse = UploadInitiateResponse

// TestStatusResponse is the CLI-facing status for a test upload.
// Extracted from the unified UploadStatusResponse result dict.
type TestStatusResponse struct {
	UploadID                  int      `json:"upload_id"`
	Status                    string   `json:"status"`
	TestsIngested             *int     `json:"tests_ingested,omitempty"`
	QuarantinedCount          *int     `json:"quarantined_count,omitempty"`
	QuarantinedTests          []string `json:"quarantined_tests,omitempty"`
	FailedCount               *int     `json:"failed_count,omitempty"`
	UnquarantinedFailureCount *int     `json:"unquarantined_failure_count,omitempty"`
	ErrorMessage              *string  `json:"error_message,omitempty"`
}

// InitiateTestUpload starts the test upload process via the unified upload API.
func (c *Client) InitiateTestUpload(orgSlug string, repoID int, metadata TestUploadMetadata) (*TestInitiateResponse, error) {
	meta := map[string]any{}
	if metadata.Format != "" {
		meta["format"] = metadata.Format
	}
	if metadata.ProviderType != "" {
		meta["provider_type"] = metadata.ProviderType
	}
	if metadata.ProviderRunID != "" {
		meta["provider_run_id"] = metadata.ProviderRunID
	}
	if metadata.JobName != "" {
		meta["job_name"] = metadata.JobName
	}
	if metadata.PRNumber != 0 {
		meta["pr_number"] = metadata.PRNumber
	}
	if metadata.PRTitle != "" {
		meta["pr_title"] = metadata.PRTitle
	}
	if metadata.PRURL != "" {
		meta["pr_url"] = metadata.PRURL
	}
	if metadata.PRAuthor != "" {
		meta["pr_author"] = metadata.PRAuthor
	}
	if metadata.RunDate != "" {
		meta["run_date"] = metadata.RunDate
	}
	if metadata.Group != "" {
		meta["group"] = metadata.Group
	}

	return c.InitiateUpload(orgSlug, repoID, UploadInitiateRequest{
		UploadType: "test_results",
		Branch:     metadata.Branch,
		SHA:        metadata.SHA,
		Filename:   "",
		Metadata:   meta,
	})
}

// CompleteTestUpload marks the upload as complete and triggers processing.
func (c *Client) CompleteTestUpload(orgSlug string, repoID, uploadID int) error {
	return c.CompleteUpload(orgSlug, repoID, uploadID)
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
		if v, ok := raw.Result["tests_ingested"]; ok {
			if f, ok := v.(float64); ok {
				n := int(f)
				result.TestsIngested = &n
			}
		}
		if v, ok := raw.Result["quarantined_count"]; ok {
			if f, ok := v.(float64); ok {
				n := int(f)
				result.QuarantinedCount = &n
			}
		}
		if v, ok := raw.Result["quarantined_tests"]; ok {
			if arr, ok := v.([]any); ok {
				for _, item := range arr {
					if s, ok := item.(string); ok {
						result.QuarantinedTests = append(result.QuarantinedTests, s)
					}
				}
			}
		}
		if v, ok := raw.Result["failed_count"]; ok {
			if f, ok := v.(float64); ok {
				n := int(f)
				result.FailedCount = &n
			}
		}
		if v, ok := raw.Result["unquarantined_failure_count"]; ok {
			if f, ok := v.(float64); ok {
				n := int(f)
				result.UnquarantinedFailureCount = &n
			}
		}
	}

	return result
}
