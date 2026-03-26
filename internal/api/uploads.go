package api

import (
	"bytes"
	"encoding/json"
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
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/orgs/%s/repos/%d/uploads", c.BaseURL, orgSlug, repoID)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	c.setHeaders(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doWithRetries(httpReq)
	if err != nil {
		return nil, fmt.Errorf("initiating upload: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, parseErrorResponse(resp)
	}

	var result UploadInitiateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
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

// seekableReadCloser wraps a bytes.Reader so it satisfies io.ReadCloser and io.Seeker,
// allowing doWithRetries to reset the body on retry.
type seekableReadCloser struct {
	*bytes.Reader
}

func (s *seekableReadCloser) Close() error { return nil }

// CompleteUpload marks an upload as complete and triggers processing.
func (c *Client) CompleteUpload(orgSlug string, repoID, uploadID int) error {
	url := fmt.Sprintf("%s/api/v1/orgs/%s/repos/%d/uploads/%d/complete", c.BaseURL, orgSlug, repoID, uploadID)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.doWithRetries(req)
	if err != nil {
		return fmt.Errorf("completing upload: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return parseErrorResponse(resp)
	}
	return nil
}

// GetUploadStatus fetches the current status of an upload.
func (c *Client) GetUploadStatus(orgSlug string, repoID, uploadID int) (*UploadStatusResponse, error) {
	url := fmt.Sprintf("%s/api/v1/orgs/%s/repos/%d/uploads/%d/status", c.BaseURL, orgSlug, repoID, uploadID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.doWithRetries(req)
	if err != nil {
		return nil, fmt.Errorf("checking upload status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, parseErrorResponse(resp)
	}

	var raw UploadStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &raw, nil
}

// PollUploadStatus polls the upload status until it completes or times out.
func (c *Client) PollUploadStatus(orgSlug string, repoID, uploadID int, timeout time.Duration, label string) (*UploadStatusResponse, error) {
	deadline := time.Now().Add(timeout)
	interval := 1 * time.Second
	maxInterval := 10 * time.Second

	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for %s processing after %v", label, timeout)
		}

		status, err := c.GetUploadStatus(orgSlug, repoID, uploadID)
		if err != nil {
			return nil, err
		}

		switch status.Status {
		case "completed":
			return status, nil
		case "failed":
			msg := "unknown error"
			if status.ErrorMessage != nil {
				msg = *status.ErrorMessage
			}
			return status, fmt.Errorf("%s processing failed: %s", label, msg)
		}

		output.Verbose("%s status: %s, waiting %v...", label, status.Status, interval)
		time.Sleep(interval)

		// Exponential backoff capped at maxInterval
		interval *= 2
		if interval > maxInterval {
			interval = maxInterval
		}
	}
}
