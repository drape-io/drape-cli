package api

import "time"

// ScanStatusResponse is the CLI-facing status for a scan upload.
type ScanStatusResponse struct {
	UploadID                      int           `json:"upload_id"`
	Status                        string        `json:"status"`
	ScanID                        *int          `json:"scan_id,omitempty"`
	ScanName                      *string       `json:"scan_name,omitempty"`
	TotalVulnerabilities          *int          `json:"total_vulnerabilities,omitempty"`
	HighestSeverity               *string       `json:"highest_severity,omitempty"`
	UnsuppressedVulnerabilities   *int          `json:"unsuppressed_vulnerabilities,omitempty"`
	UnsuppressedHighestSeverity   *string       `json:"unsuppressed_highest_severity,omitempty"`
	ErrorMessage                  *string       `json:"error_message,omitempty"`
	ScanDiff                      *ScanDiffInfo `json:"scan_diff,omitempty"`
}

// ScanDiffInfo contains the result of comparing PR scan results against the base branch.
type ScanDiffInfo struct {
	Passed             bool            `json:"passed"`
	NewCriticalCount   int             `json:"new_critical_count"`
	NewHighCount       int             `json:"new_high_count"`
	NewMediumCount     int             `json:"new_medium_count"`
	NewLowCount        int             `json:"new_low_count"`
	SuppressedCVECount int             `json:"suppressed_cves_count"`
	UnchangedCVECount  int             `json:"unchanged_cves_count"`
	NewCVEs            []ScanCVE       `json:"new_cves"`
	ResolvedCVEs       []ScanCVE       `json:"resolved_cves"`
	SLAViolations      []SLAViolation  `json:"sla_violations"`
	FailureReasons     []string        `json:"failure_reasons"`
}

// ScanCVE represents a CVE in diff results.
type ScanCVE struct {
	CVEID          string      `json:"cve_id"`
	Severity       string      `json:"severity"`
	PackageName    string      `json:"package_name"`
	PackageVersion string      `json:"package_version"`
	FixState       string      `json:"fix_state"`
	Suppression    *Suppression `json:"suppression,omitempty"`
}

// Suppression represents suppression metadata attached to a CVE or violation.
type Suppression struct {
	Type          string `json:"suppression_type"`
	Justification string `json:"justification"`
}

// SLAViolation represents a CVE that has exceeded its SLA deadline.
type SLAViolation struct {
	CVEID       string `json:"cve_id"`
	Severity    string `json:"severity"`
	PackageName string `json:"package_name"`
	DaysOverdue int    `json:"days_overdue"`
}

// PollScanStatus polls the upload status until it completes or times out.
func (c *Client) PollScanStatus(orgSlug string, repoID, uploadID int, timeout time.Duration) (*ScanStatusResponse, error) {
	raw, err := c.PollUploadStatus(orgSlug, repoID, uploadID, timeout, "Scan")
	if err != nil {
		if raw != nil {
			return &ScanStatusResponse{
				UploadID:     raw.UploadID,
				Status:       raw.Status,
				ErrorMessage: raw.ErrorMessage,
			}, err
		}
		return nil, err
	}

	return mapScanStatus(raw), nil
}

func mapScanStatus(raw *UploadStatusResponse) *ScanStatusResponse {
	result := &ScanStatusResponse{
		UploadID:     raw.UploadID,
		Status:       raw.Status,
		ErrorMessage: raw.ErrorMessage,
	}

	if raw.Result == nil {
		return result
	}

	result.ScanID = getInt(raw.Result, "scan_id")
	result.ScanName = getString(raw.Result, "scan_name")
	result.TotalVulnerabilities = getInt(raw.Result, "total_vulnerabilities")
	result.HighestSeverity = getString(raw.Result, "highest_severity")
	result.UnsuppressedVulnerabilities = getInt(raw.Result, "unsuppressed_vulnerabilities")
	result.UnsuppressedHighestSeverity = getString(raw.Result, "unsuppressed_highest_severity")
	if diffMap, ok := raw.Result["scan_diff"].(map[string]any); ok {
		result.ScanDiff = mapScanDiff(diffMap)
	}

	return result
}

func mapScanDiff(m map[string]any) *ScanDiffInfo {
	diff := &ScanDiffInfo{
		Passed:             getBool(m, "passed"),
		NewCriticalCount:   getIntVal(m, "new_critical_count"),
		NewHighCount:       getIntVal(m, "new_high_count"),
		NewMediumCount:     getIntVal(m, "new_medium_count"),
		NewLowCount:        getIntVal(m, "new_low_count"),
		SuppressedCVECount: getIntVal(m, "suppressed_cves_count"),
		UnchangedCVECount:  getIntVal(m, "unchanged_cves_count"),
		FailureReasons:     getStringSlice(m, "failure_reasons"),
	}

	if cves, ok := m["new_cves"].([]any); ok {
		for _, c := range cves {
			if cm, ok := c.(map[string]any); ok {
				diff.NewCVEs = append(diff.NewCVEs, mapScanCVE(cm))
			}
		}
	}
	if cves, ok := m["resolved_cves"].([]any); ok {
		for _, c := range cves {
			if cm, ok := c.(map[string]any); ok {
				diff.ResolvedCVEs = append(diff.ResolvedCVEs, mapScanCVE(cm))
			}
		}
	}
	if violations, ok := m["sla_violations"].([]any); ok {
		for _, v := range violations {
			if vm, ok := v.(map[string]any); ok {
				diff.SLAViolations = append(diff.SLAViolations, SLAViolation{
					CVEID:       getStringVal(vm, "cve_id"),
					Severity:    getStringVal(vm, "severity"),
					PackageName: getStringVal(vm, "package_name"),
					DaysOverdue: getIntVal(vm, "days_overdue"),
				})
			}
		}
	}

	return diff
}

func mapScanCVE(m map[string]any) ScanCVE {
	cve := ScanCVE{
		CVEID:          getStringVal(m, "cve_id"),
		Severity:       getStringVal(m, "severity"),
		PackageName:    getStringVal(m, "package_name"),
		PackageVersion: getStringVal(m, "package_version"),
		FixState:       getStringVal(m, "fix_state"),
	}
	if sup, ok := m["suppression"].(map[string]any); ok {
		cve.Suppression = &Suppression{
			Type:          getStringVal(sup, "suppression_type"),
			Justification: getStringVal(sup, "justification"),
		}
	}
	return cve
}
