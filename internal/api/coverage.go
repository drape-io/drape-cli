package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/drape-io/drape-cli/internal/output"
)

// RegressedFileInfo contains per-file regression details.
type RegressedFileInfo struct {
	FilePath            string  `json:"file_path"`
	RegressedLines      int     `json:"regressed_lines"`
	RegressedLineRanges [][]int `json:"regressed_line_ranges"`
}

// CoverageDiffInfo contains the result of comparing PR coverage against the base branch.
type CoverageDiffInfo struct {
	Passed              bool               `json:"passed"`
	BaseCoverageRate    *string            `json:"base_coverage_rate,omitempty"`
	HeadCoverageRate    string             `json:"head_coverage_rate"`
	CoverageDelta       *string            `json:"coverage_delta,omitempty"`
	NewLinesTotal       int                `json:"new_lines_total"`
	NewLinesCovered     int                `json:"new_lines_covered"`
	NewCodeCoverageRate *string            `json:"new_code_coverage_rate,omitempty"`
	RegressedLinesCount int                `json:"regressed_lines_count"`
	RegressedFiles      []RegressedFileInfo `json:"regressed_files,omitempty"`
	FailureReasons      []string           `json:"failure_reasons"`
}

// CoverageStatusResponse is the CLI-facing status for a coverage upload.
// Extracted from the unified UploadStatusResponse result dict.
type CoverageStatusResponse struct {
	UploadID           int               `json:"upload_id"`
	Status             string            `json:"status"`
	CoverageSnapshotID *int              `json:"coverage_snapshot_id,omitempty"`
	CoverageRate       *string           `json:"coverage_rate,omitempty"`
	FileCount          *int              `json:"file_count,omitempty"`
	ErrorMessage       *string           `json:"error_message,omitempty"`
	CoverageDiff       *CoverageDiffInfo `json:"coverage_diff,omitempty"`
}

// PollCoverageStatus polls the upload status until it completes or times out.
func (c *Client) PollCoverageStatus(orgSlug string, repoID, uploadID int, timeout time.Duration) (*CoverageStatusResponse, error) {
	raw, err := c.PollUploadStatus(orgSlug, repoID, uploadID, timeout, "Coverage")
	if err != nil {
		// Map to CoverageStatusResponse even on failure
		if raw != nil {
			return &CoverageStatusResponse{
				UploadID:     raw.UploadID,
				Status:       raw.Status,
				ErrorMessage: raw.ErrorMessage,
			}, err
		}
		return nil, err
	}

	return mapCoverageStatus(raw), nil
}

func mapCoverageStatus(raw *UploadStatusResponse) *CoverageStatusResponse {
	result := &CoverageStatusResponse{
		UploadID:     raw.UploadID,
		Status:       raw.Status,
		ErrorMessage: raw.ErrorMessage,
	}

	if raw.Result != nil {
		result.CoverageSnapshotID = getInt(raw.Result, "snapshot_id")
		result.CoverageRate = getString(raw.Result, "coverage_rate")
		result.FileCount = getInt(raw.Result, "file_count")
		if diffMap, ok := raw.Result["coverage_diff"].(map[string]any); ok {
			result.CoverageDiff = mapCoverageDiff(diffMap)
		}
	}

	return result
}

// CoverageBatchRequest is the request body for creating a coverage upload batch.
type CoverageBatchRequest struct {
	ExpectedCount int            `json:"expected_count"`
	UploadType    string         `json:"upload_type"`
	Branch        string         `json:"branch"`
	SHA           string         `json:"sha"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// CoverageBatchResponse is the response from creating a coverage upload batch.
type CoverageBatchResponse struct {
	BatchID int `json:"batch_id"`
}

// CoverageBatchStatusResponse is the response from polling a coverage batch status.
type CoverageBatchStatusResponse struct {
	BatchID          int               `json:"batch_id"`
	Status           string            `json:"status"` // pending, processing, completed, failed
	ExpectedCount    int               `json:"expected_count"`
	CompletedCount   int               `json:"completed_count"`
	ErrorMessage     *string           `json:"error_message,omitempty"`
	CoverageRate     *string           `json:"coverage_rate,omitempty"`
	FileCount        *int              `json:"file_count,omitempty"`
	CoverageSnapshotID *int            `json:"coverage_snapshot_id,omitempty"`
	CoverageDiff     *CoverageDiffInfo `json:"coverage_diff,omitempty"`
}

// CreateCoverageBatch creates a new upload batch for multi-file coverage uploads.
func (c *Client) CreateCoverageBatch(orgSlug string, repoID int, req CoverageBatchRequest) (*CoverageBatchResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/orgs/%s/repos/%d/upload-batches", c.BaseURL, orgSlug, repoID)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	c.setHeaders(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doWithRetries(httpReq)
	if err != nil {
		return nil, fmt.Errorf("creating batch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, parseErrorResponse(resp)
	}

	var result CoverageBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &result, nil
}

// PollCoverageBatchStatus polls a batch status until it completes or times out.
func (c *Client) PollCoverageBatchStatus(orgSlug string, repoID, batchID int, timeout time.Duration) (*CoverageBatchStatusResponse, error) {
	deadline := time.Now().Add(timeout)
	interval := 1 * time.Second
	maxInterval := 10 * time.Second
	statusURL := fmt.Sprintf("%s/api/v1/orgs/%s/repos/%d/upload-batches/%d/status", c.BaseURL, orgSlug, repoID, batchID)

	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for batch processing after %v", timeout)
		}

		req, err := http.NewRequest("GET", statusURL, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		c.setHeaders(req)

		resp, err := c.doWithRetries(req)
		if err != nil {
			return nil, fmt.Errorf("checking batch status: %w", err)
		}

		var raw map[string]any
		err = json.NewDecoder(resp.Body).Decode(&raw)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("decoding response: %w", err)
		}

		status := mapBatchStatus(raw)

		switch status.Status {
		case "completed":
			return status, nil
		case "failed":
			msg := "unknown error"
			if status.ErrorMessage != nil {
				msg = *status.ErrorMessage
			}
			return status, fmt.Errorf("batch processing failed: %s", msg)
		}

		output.Verbose("Batch status: %s (%d/%d uploads), waiting %v...", status.Status, status.CompletedCount, status.ExpectedCount, interval)
		time.Sleep(interval)

		interval *= 2
		if interval > maxInterval {
			interval = maxInterval
		}
	}
}

func mapBatchStatus(m map[string]any) *CoverageBatchStatusResponse {
	status := &CoverageBatchStatusResponse{
		BatchID:            getIntVal(m, "batch_id"),
		Status:             getStringVal(m, "status"),
		ExpectedCount:      getIntVal(m, "expected_count"),
		CompletedCount:     getIntVal(m, "completed_count"),
		ErrorMessage:       getString(m, "error_message"),
		CoverageRate:       getString(m, "coverage_rate"),
		FileCount:          getInt(m, "file_count"),
		CoverageSnapshotID: getInt(m, "coverage_snapshot_id"),
	}

	if diffMap, ok := m["coverage_diff"].(map[string]any); ok {
		status.CoverageDiff = mapCoverageDiff(diffMap)
	}

	return status
}

func mapCoverageDiff(m map[string]any) *CoverageDiffInfo {
	diff := &CoverageDiffInfo{
		Passed:              getBool(m, "passed"),
		BaseCoverageRate:    getString(m, "base_coverage_rate"),
		HeadCoverageRate:    getStringVal(m, "head_coverage_rate"),
		CoverageDelta:       getString(m, "coverage_delta"),
		NewLinesTotal:       getIntVal(m, "new_lines_total"),
		NewLinesCovered:     getIntVal(m, "new_lines_covered"),
		NewCodeCoverageRate: getString(m, "new_code_coverage_rate"),
		RegressedLinesCount: getIntVal(m, "regressed_lines_count"),
		FailureReasons:      getStringSlice(m, "failure_reasons"),
	}

	if files, ok := m["regressed_files"].([]any); ok {
		for _, f := range files {
			if fm, ok := f.(map[string]any); ok {
				rf := RegressedFileInfo{
					FilePath:       getStringVal(fm, "file_path"),
					RegressedLines: getIntVal(fm, "regressed_lines"),
				}
				if ranges, ok := fm["regressed_line_ranges"].([]any); ok {
					for _, r := range ranges {
						if pair, ok := r.([]any); ok && len(pair) == 2 {
							start, ok1 := pair[0].(float64)
							end, ok2 := pair[1].(float64)
							if ok1 && ok2 {
								rf.RegressedLineRanges = append(rf.RegressedLineRanges, []int{int(start), int(end)})
							}
						}
					}
				}
				diff.RegressedFiles = append(diff.RegressedFiles, rf)
			}
		}
	}

	return diff
}
