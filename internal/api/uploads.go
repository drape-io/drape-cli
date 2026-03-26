package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/drape-io/drape-cli/internal/output"
)

// UploadInitiateRequest is the unified upload API request body.
type UploadInitiateRequest struct {
	UploadType  string         `json:"upload_type"`
	Branch      string         `json:"branch"`
	SHA         string         `json:"sha"`
	Filename    string         `json:"filename,omitempty"`
	ContentType string         `json:"content_type,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	BatchID     *int           `json:"batch_id,omitempty"`
}

// UploadInitiateResponse is the unified upload API initiate response.
type UploadInitiateResponse struct {
	UploadID  int    `json:"upload_id"`
	UploadURL string `json:"upload_url"`
	ExpiresIn int    `json:"expires_in"`
}

// UploadStatusResponse is the raw unified upload status response.
type UploadStatusResponse struct {
	UploadID     int            `json:"upload_id"`
	UploadType   string         `json:"upload_type"`
	Status       string         `json:"status"`
	Result       map[string]any `json:"result"`
	ErrorMessage *string        `json:"error_message"`
}

// InitiateUpload starts an upload via the unified upload API.
func (c *Client) InitiateUpload(orgSlug string, repoID int, req UploadInitiateRequest) (*UploadInitiateResponse, error) {
	url := fmt.Sprintf("%s/api/v1/orgs/%s/repos/%d/uploads", c.BaseURL, orgSlug, repoID)
	var result UploadInitiateResponse
	if err := c.postJSON(url, req, &result); err != nil {
		return nil, fmt.Errorf("initiating upload: %w", err)
	}
	return &result, nil
}

// UploadToPresignedURL uploads file content to a presigned URL.
func (c *Client) UploadToPresignedURL(presignedURL string, content []byte) error {
	body := &seekableReadCloser{bytes.NewReader(content)}
	req, err := http.NewRequest("PUT", presignedURL, body)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	// Preserve the seekable body — NewRequest may wrap it with NopCloser.
	req.Body = body
	req.ContentLength = int64(len(content))
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.doWithRetries(req)
	if err != nil {
		return fmt.Errorf("uploading to presigned URL: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("presigned upload failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// CompleteUpload marks an upload as complete and triggers processing.
func (c *Client) CompleteUpload(orgSlug string, repoID, uploadID int) error {
	url := fmt.Sprintf("%s/api/v1/orgs/%s/repos/%d/uploads/%d/complete", c.BaseURL, orgSlug, repoID, uploadID)
	if err := c.postJSON(url, nil, nil); err != nil {
		return fmt.Errorf("completing upload: %w", err)
	}
	return nil
}

// GetUploadStatus fetches the current status of an upload.
func (c *Client) GetUploadStatus(orgSlug string, repoID, uploadID int) (*UploadStatusResponse, error) {
	url := fmt.Sprintf("%s/api/v1/orgs/%s/repos/%d/uploads/%d/status", c.BaseURL, orgSlug, repoID, uploadID)
	var raw UploadStatusResponse
	if err := c.getJSON(url, &raw); err != nil {
		return nil, fmt.Errorf("checking upload status: %w", err)
	}
	return &raw, nil
}

// pollResult captures the terminal state fields returned by a polling function.
type pollResult struct {
	Status       string
	ErrorMessage *string
}

// pollWithBackoff runs fetchFn in a loop with exponential backoff until it returns
// a terminal status ("completed" or "failed") or the timeout expires.
func (c *Client) pollWithBackoff(timeout time.Duration, label string, fetchFn func() (*pollResult, error)) error {
	deadline := time.Now().Add(timeout)
	interval := 1 * time.Second
	maxInterval := 10 * time.Second

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s processing after %v", label, timeout)
		}

		result, err := fetchFn()
		if err != nil {
			return err
		}

		switch result.Status {
		case "completed":
			return nil
		case "failed":
			msg := "unknown error"
			if result.ErrorMessage != nil {
				msg = *result.ErrorMessage
			}
			return fmt.Errorf("%s processing failed: %s", label, msg)
		}

		output.Verbose("%s status: %s, waiting %v...", label, result.Status, interval)
		time.Sleep(interval)

		interval *= 2
		if interval > maxInterval {
			interval = maxInterval
		}
	}
}

// PollUploadStatus polls the upload status until it completes or times out.
func (c *Client) PollUploadStatus(orgSlug string, repoID, uploadID int, timeout time.Duration, label string) (*UploadStatusResponse, error) {
	var last *UploadStatusResponse
	err := c.pollWithBackoff(timeout, label, func() (*pollResult, error) {
		status, err := c.GetUploadStatus(orgSlug, repoID, uploadID)
		if err != nil {
			return nil, err
		}
		last = status
		return &pollResult{Status: status.Status, ErrorMessage: status.ErrorMessage}, nil
	})
	return last, err
}
