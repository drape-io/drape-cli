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

	if v, ok := raw.Result["scan_id"].(float64); ok {
		id := int(v)
		result.ScanID = &id
	}
	if v, ok := raw.Result["scan_name"].(string); ok {
		result.ScanName = &v
	}
	if v, ok := raw.Result["total_vulnerabilities"].(float64); ok {
		count := int(v)
		result.TotalVulnerabilities = &count
	}
	if v, ok := raw.Result["highest_severity"].(string); ok {
		result.HighestSeverity = &v
	}
	if v, ok := raw.Result["unsuppressed_vulnerabilities"].(float64); ok {
		count := int(v)
		result.UnsuppressedVulnerabilities = &count
	}
	if v, ok := raw.Result["unsuppressed_highest_severity"].(string); ok {
		result.UnsuppressedHighestSeverity = &v
	}
	if diffMap, ok := raw.Result["scan_diff"].(map[string]any); ok {
		result.ScanDiff = mapScanDiff(diffMap)
	}

	return result
}

func mapScanDiff(m map[string]any) *ScanDiffInfo {
	diff := &ScanDiffInfo{}

	if v, ok := m["passed"].(bool); ok {
		diff.Passed = v
	}
	if v, ok := m["new_critical_count"].(float64); ok {
		diff.NewCriticalCount = int(v)
	}
	if v, ok := m["new_high_count"].(float64); ok {
		diff.NewHighCount = int(v)
	}
	if v, ok := m["new_medium_count"].(float64); ok {
		diff.NewMediumCount = int(v)
	}
	if v, ok := m["new_low_count"].(float64); ok {
		diff.NewLowCount = int(v)
	}
	if v, ok := m["suppressed_cves_count"].(float64); ok {
		diff.SuppressedCVECount = int(v)
	}
	if v, ok := m["unchanged_cves_count"].(float64); ok {
		diff.UnchangedCVECount = int(v)
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
				sv := SLAViolation{}
				if s, ok := vm["cve_id"].(string); ok {
					sv.CVEID = s
				}
				if s, ok := vm["severity"].(string); ok {
					sv.Severity = s
				}
				if s, ok := vm["package_name"].(string); ok {
					sv.PackageName = s
				}
				if f, ok := vm["days_overdue"].(float64); ok {
					sv.DaysOverdue = int(f)
				}
				diff.SLAViolations = append(diff.SLAViolations, sv)
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

func mapScanCVE(m map[string]any) ScanCVE {
	cve := ScanCVE{}
	if s, ok := m["cve_id"].(string); ok {
		cve.CVEID = s
	}
	if s, ok := m["severity"].(string); ok {
		cve.Severity = s
	}
	if s, ok := m["package_name"].(string); ok {
		cve.PackageName = s
	}
	if s, ok := m["package_version"].(string); ok {
		cve.PackageVersion = s
	}
	if s, ok := m["fix_state"].(string); ok {
		cve.FixState = s
	}
	if sup, ok := m["suppression"].(map[string]any); ok {
		s := &Suppression{}
		if v, ok := sup["suppression_type"].(string); ok {
			s.Type = v
		}
		if v, ok := sup["justification"].(string); ok {
			s.Justification = v
		}
		cve.Suppression = s
	}
	return cve
}
