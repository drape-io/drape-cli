package api

import (
	"time"
)

// CoverageUploadRequest is the CLI-facing request for uploading coverage.
// It maps to the unified UploadInitiateRequest on the server.
type CoverageUploadRequest struct {
	Branch       string `json:"branch"`
	SHA          string `json:"sha"`
	Format       string `json:"format"`
	Filename     string `json:"filename"`
	PathPrefix   string `json:"path_prefix,omitempty"`
	PRNumber     int    `json:"pr_number,omitempty"`
	PRTitle      string `json:"pr_title,omitempty"`
	PRURL        string `json:"pr_url,omitempty"`
	PRAuthor     string `json:"pr_author,omitempty"`
	TargetBranch string `json:"target_branch,omitempty"`
	RunDate      string `json:"run_date,omitempty"`
	Group        string `json:"group,omitempty"`
}

// CoverageInitiateResponse matches the unified UploadInitiateResponse.
type CoverageInitiateResponse = UploadInitiateResponse

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

// InitiateCoverageUpload starts the coverage upload process via the unified upload API.
func (c *Client) InitiateCoverageUpload(orgSlug string, repoID int, req CoverageUploadRequest) (*CoverageInitiateResponse, error) {
	metadata := map[string]any{
		"format": req.Format,
	}
	if req.PathPrefix != "" {
		metadata["path_prefix"] = req.PathPrefix
	}
	if req.PRNumber != 0 {
		metadata["pr_number"] = req.PRNumber
	}
	if req.PRTitle != "" {
		metadata["pr_title"] = req.PRTitle
	}
	if req.PRURL != "" {
		metadata["pr_url"] = req.PRURL
	}
	if req.PRAuthor != "" {
		metadata["pr_author"] = req.PRAuthor
	}
	if req.TargetBranch != "" {
		metadata["target_branch"] = req.TargetBranch
	}
	if req.RunDate != "" {
		metadata["run_date"] = req.RunDate
	}
	if req.Group != "" {
		metadata["group"] = req.Group
	}

	return c.InitiateUpload(orgSlug, repoID, UploadInitiateRequest{
		UploadType: "coverage",
		Branch:     req.Branch,
		SHA:        req.SHA,
		Filename:   req.Filename,
		Metadata:   metadata,
	})
}

// CompleteCoverageUpload marks the upload as complete and triggers processing.
func (c *Client) CompleteCoverageUpload(orgSlug string, repoID, uploadID int) error {
	return c.CompleteUpload(orgSlug, repoID, uploadID)
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
		if v, ok := raw.Result["snapshot_id"]; ok {
			if f, ok := v.(float64); ok {
				id := int(f)
				result.CoverageSnapshotID = &id
			}
		}
		if v, ok := raw.Result["coverage_rate"]; ok {
			if s, ok := v.(string); ok {
				result.CoverageRate = &s
			}
		}
		if v, ok := raw.Result["file_count"]; ok {
			if f, ok := v.(float64); ok {
				count := int(f)
				result.FileCount = &count
			}
		}
		if v, ok := raw.Result["coverage_diff"]; ok {
			if diffMap, ok := v.(map[string]any); ok {
				result.CoverageDiff = mapCoverageDiff(diffMap)
			}
		}
	}

	return result
}

func mapCoverageDiff(m map[string]any) *CoverageDiffInfo {
	diff := &CoverageDiffInfo{}

	if v, ok := m["passed"].(bool); ok {
		diff.Passed = v
	}
	if v, ok := m["base_coverage_rate"].(string); ok {
		diff.BaseCoverageRate = &v
	}
	if v, ok := m["head_coverage_rate"].(string); ok {
		diff.HeadCoverageRate = v
	}
	if v, ok := m["coverage_delta"].(string); ok {
		diff.CoverageDelta = &v
	}
	if v, ok := m["new_lines_total"].(float64); ok {
		diff.NewLinesTotal = int(v)
	}
	if v, ok := m["new_lines_covered"].(float64); ok {
		diff.NewLinesCovered = int(v)
	}
	if v, ok := m["new_code_coverage_rate"].(string); ok {
		diff.NewCodeCoverageRate = &v
	}
	if v, ok := m["regressed_lines_count"].(float64); ok {
		diff.RegressedLinesCount = int(v)
	}
	if files, ok := m["regressed_files"].([]any); ok {
		for _, f := range files {
			if fm, ok := f.(map[string]any); ok {
				rf := RegressedFileInfo{}
				if v, ok := fm["file_path"].(string); ok {
					rf.FilePath = v
				}
				if v, ok := fm["regressed_lines"].(float64); ok {
					rf.RegressedLines = int(v)
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
	if reasons, ok := m["failure_reasons"].([]any); ok {
		for _, r := range reasons {
			if s, ok := r.(string); ok {
				diff.FailureReasons = append(diff.FailureReasons, s)
			}
		}
	}

	return diff
}
