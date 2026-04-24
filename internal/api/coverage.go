package api

import (
	"fmt"
	"time"
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

// CoverageResult contains the coverage-specific fields shared between
// individual upload status and batch status responses.
type CoverageResult struct {
	CoverageSnapshotID *int              `json:"coverage_snapshot_id,omitempty"`
	CoverageRate       *string           `json:"coverage_rate,omitempty"`
	FileCount          *int              `json:"file_count,omitempty"`
	CoverageDiff       *CoverageDiffInfo `json:"coverage_diff,omitempty"`
	// Partial is set on batch status responses when the server reaper finalized
	// the batch with fewer than expected_count shards. Always nil for
	// individual-upload responses.
	Partial *bool `json:"partial,omitempty"`
}

// CoverageStatusResponse is the CLI-facing status for a coverage upload.
// Extracted from the unified UploadStatusResponse result dict.
type CoverageStatusResponse struct {
	UploadID     int     `json:"upload_id"`
	Status       string  `json:"status"`
	ErrorMessage *string `json:"error_message,omitempty"`
	CoverageResult
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
		result.CoverageResult = mapCoverageResult(raw.Result)
	}
	return result
}

// mapCoverageResult extracts coverage-specific fields from a result dict.
// Used by both individual upload and batch status mappers.
func mapCoverageResult(m map[string]any) CoverageResult {
	result := CoverageResult{
		CoverageSnapshotID: getInt(m, "snapshot_id"),
		CoverageRate:       getString(m, "coverage_rate"),
		FileCount:          getInt(m, "file_count"),
		Partial:            getBoolPtr(m, "partial"),
	}
	if diffMap, ok := m["coverage_diff"].(map[string]any); ok {
		result.CoverageDiff = mapCoverageDiff(diffMap)
	}
	return result
}

// CoverageBatchRequest is the request body for creating a coverage upload batch.
//
// ProviderRunID + RunAttempt engage the server's natural-key upsert path when
// both are set (matrix-shard fan-in). Leaving them zero/empty keeps the server
// on its legacy "always create fresh batch" path. group is carried inside
// Metadata (server reads it via metadata.get("group")), not as a top-level field.
type CoverageBatchRequest struct {
	ExpectedCount int            `json:"expected_count"`
	UploadType    string         `json:"upload_type"`
	Branch        string         `json:"branch"`
	SHA           string         `json:"sha"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	ProviderRunID string         `json:"provider_run_id,omitempty"`
	RunAttempt    int            `json:"run_attempt,omitempty"`
}

// CoverageBatchResponse is the response from creating a coverage upload batch.
type CoverageBatchResponse struct {
	BatchID int `json:"batch_id"`
}

// CoverageBatchStatusResponse is the response from polling a coverage batch status.
type CoverageBatchStatusResponse struct {
	BatchID        int     `json:"batch_id"`
	Status         string  `json:"status"` // pending, processing, completed, failed
	ExpectedCount  int     `json:"expected_count"`
	CompletedCount int     `json:"completed_count"`
	ErrorMessage   *string `json:"error_message,omitempty"`
	CoverageResult
}

// CreateCoverageBatch creates a new upload batch for multi-file coverage uploads.
func (c *Client) CreateCoverageBatch(orgSlug string, repoID int, req CoverageBatchRequest) (*CoverageBatchResponse, error) {
	url := fmt.Sprintf("%s/api/v1/orgs/%s/repos/%d/upload-batches", c.BaseURL, orgSlug, repoID)
	var result CoverageBatchResponse
	if err := c.postJSON(url, req, &result); err != nil {
		return nil, fmt.Errorf("creating batch: %w", err)
	}
	return &result, nil
}

// GetBatchStatus fetches the current status of a coverage upload batch.
func (c *Client) GetBatchStatus(orgSlug string, repoID, batchID int) (*CoverageBatchStatusResponse, error) {
	url := fmt.Sprintf("%s/api/v1/orgs/%s/repos/%d/upload-batches/%d/status", c.BaseURL, orgSlug, repoID, batchID)
	var raw map[string]any
	if err := c.getJSON(url, &raw); err != nil {
		return nil, fmt.Errorf("checking batch status: %w", err)
	}
	return mapBatchStatus(raw), nil
}

// PollCoverageBatchStatus polls a batch status until it completes or times out.
func (c *Client) PollCoverageBatchStatus(orgSlug string, repoID, batchID int, timeout time.Duration) (*CoverageBatchStatusResponse, error) {
	var last *CoverageBatchStatusResponse
	err := c.pollWithBackoff(timeout, "Batch", func() (*pollResult, error) {
		status, err := c.GetBatchStatus(orgSlug, repoID, batchID)
		if err != nil {
			return nil, err
		}
		last = status
		return &pollResult{Status: status.Status, ErrorMessage: status.ErrorMessage}, nil
	})
	return last, err
}

func mapBatchStatus(m map[string]any) *CoverageBatchStatusResponse {
	status := &CoverageBatchStatusResponse{
		BatchID:        getIntVal(m, "batch_id"),
		Status:         getStringVal(m, "status"),
		ExpectedCount:  getIntVal(m, "expected_count"),
		CompletedCount: getIntVal(m, "completed_count"),
		ErrorMessage:   getString(m, "error_message"),
	}
	// Coverage fields are nested under "result", matching the individual upload shape.
	if result, ok := m["result"].(map[string]any); ok {
		status.CoverageResult = mapCoverageResult(result)
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
