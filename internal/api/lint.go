package api

import "time"

// LintStatusResponse is the CLI-facing status for a lint upload.
type LintStatusResponse struct {
	UploadID        int           `json:"upload_id"`
	Status          string        `json:"status"`
	SnapshotID      *int          `json:"snapshot_id,omitempty"`
	TotalViolations *int          `json:"total_violations,omitempty"`
	ErrorCount      *int          `json:"error_count,omitempty"`
	WarningCount    *int          `json:"warning_count,omitempty"`
	ErrorMessage    *string       `json:"error_message,omitempty"`
	LintDiff        *LintDiffInfo `json:"lint_diff,omitempty"`
}

// LintDiffInfo contains the result of comparing PR lint results against the base branch.
type LintDiffInfo struct {
	Passed                   bool              `json:"passed"`
	BaseViolationCount       int               `json:"base_violation_count"`
	HeadViolationCount       int               `json:"head_violation_count"`
	NewViolationCount        int               `json:"new_violation_count"`
	ResolvedViolationCount   int               `json:"resolved_violation_count"`
	SuppressedViolationCount int               `json:"suppressed_violation_count"`
	NewViolations            []LintViolation   `json:"new_violations"`
	FailureReasons           []string          `json:"failure_reasons"`
}

// LintViolation represents a single lint violation in diff results.
type LintViolation struct {
	RuleID   string `json:"rule_id"`
	FilePath string `json:"file_path"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// PollLintStatus polls the upload status until it completes or times out.
func (c *Client) PollLintStatus(orgSlug string, repoID, uploadID int, timeout time.Duration) (*LintStatusResponse, error) {
	raw, err := c.PollUploadStatus(orgSlug, repoID, uploadID, timeout, "Lint")
	if err != nil {
		if raw != nil {
			return &LintStatusResponse{
				UploadID:     raw.UploadID,
				Status:       raw.Status,
				ErrorMessage: raw.ErrorMessage,
			}, err
		}
		return nil, err
	}

	return mapLintStatus(raw), nil
}

func mapLintStatus(raw *UploadStatusResponse) *LintStatusResponse {
	result := &LintStatusResponse{
		UploadID:     raw.UploadID,
		Status:       raw.Status,
		ErrorMessage: raw.ErrorMessage,
	}

	if raw.Result == nil {
		return result
	}

	if v, ok := raw.Result["snapshot_id"].(float64); ok {
		id := int(v)
		result.SnapshotID = &id
	}
	if v, ok := raw.Result["total_violations"].(float64); ok {
		count := int(v)
		result.TotalViolations = &count
	}
	if v, ok := raw.Result["error_count"].(float64); ok {
		count := int(v)
		result.ErrorCount = &count
	}
	if v, ok := raw.Result["warning_count"].(float64); ok {
		count := int(v)
		result.WarningCount = &count
	}
	if diffMap, ok := raw.Result["lint_diff"].(map[string]any); ok {
		result.LintDiff = mapLintDiff(diffMap)
	}

	return result
}

func mapLintDiff(m map[string]any) *LintDiffInfo {
	diff := &LintDiffInfo{}

	if v, ok := m["passed"].(bool); ok {
		diff.Passed = v
	}
	if v, ok := m["base_violation_count"].(float64); ok {
		diff.BaseViolationCount = int(v)
	}
	if v, ok := m["head_violation_count"].(float64); ok {
		diff.HeadViolationCount = int(v)
	}
	if v, ok := m["new_violation_count"].(float64); ok {
		diff.NewViolationCount = int(v)
	}
	if v, ok := m["resolved_violation_count"].(float64); ok {
		diff.ResolvedViolationCount = int(v)
	}
	if v, ok := m["suppressed_violation_count"].(float64); ok {
		diff.SuppressedViolationCount = int(v)
	}
	if violations, ok := m["new_violations"].([]any); ok {
		for _, v := range violations {
			if vm, ok := v.(map[string]any); ok {
				lv := LintViolation{}
				if s, ok := vm["rule_id"].(string); ok {
					lv.RuleID = s
				}
				if s, ok := vm["file_path"].(string); ok {
					lv.FilePath = s
				}
				if f, ok := vm["line"].(float64); ok {
					lv.Line = int(f)
				}
				if s, ok := vm["severity"].(string); ok {
					lv.Severity = s
				}
				if s, ok := vm["message"].(string); ok {
					lv.Message = s
				}
				diff.NewViolations = append(diff.NewViolations, lv)
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
