package api

import (
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
