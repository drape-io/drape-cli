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

	result.SnapshotID = getInt(raw.Result, "snapshot_id")
	result.TotalViolations = getInt(raw.Result, "total_violations")
	result.ErrorCount = getInt(raw.Result, "error_count")
	result.WarningCount = getInt(raw.Result, "warning_count")
	if diffMap, ok := raw.Result["lint_diff"].(map[string]any); ok {
		result.LintDiff = mapLintDiff(diffMap)
	}

	return result
}

func mapLintDiff(m map[string]any) *LintDiffInfo {
	diff := &LintDiffInfo{
		Passed:                   getBool(m, "passed"),
		BaseViolationCount:       getIntVal(m, "base_violation_count"),
		HeadViolationCount:       getIntVal(m, "head_violation_count"),
		NewViolationCount:        getIntVal(m, "new_violation_count"),
		ResolvedViolationCount:   getIntVal(m, "resolved_violation_count"),
		SuppressedViolationCount: getIntVal(m, "suppressed_violation_count"),
		FailureReasons:           getStringSlice(m, "failure_reasons"),
	}

	if violations, ok := m["new_violations"].([]any); ok {
		for _, v := range violations {
			if vm, ok := v.(map[string]any); ok {
				diff.NewViolations = append(diff.NewViolations, LintViolation{
					RuleID:   getStringVal(vm, "rule_id"),
					FilePath: getStringVal(vm, "file_path"),
					Line:     getIntVal(vm, "line"),
					Severity: getStringVal(vm, "severity"),
					Message:  getStringVal(vm, "message"),
				})
			}
		}
	}

	return diff
}
