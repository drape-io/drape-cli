package api

import (
	"encoding/json"
	"testing"
)

func TestMapCoverageResult(t *testing.T) {
	m := map[string]any{
		"snapshot_id":   float64(42),
		"coverage_rate": "85.50",
		"file_count":    float64(10),
		"coverage_diff": map[string]any{
			"passed":            true,
			"head_coverage_rate": "85.50",
			"new_lines_total":   float64(20),
			"new_lines_covered": float64(18),
			"failure_reasons":   []any{},
		},
	}

	result := mapCoverageResult(m)

	if result.CoverageSnapshotID == nil || *result.CoverageSnapshotID != 42 {
		t.Errorf("expected snapshot_id=42, got %v", result.CoverageSnapshotID)
	}
	if result.CoverageRate == nil || *result.CoverageRate != "85.50" {
		t.Errorf("expected coverage_rate=85.50, got %v", result.CoverageRate)
	}
	if result.FileCount == nil || *result.FileCount != 10 {
		t.Errorf("expected file_count=10, got %v", result.FileCount)
	}
	if result.CoverageDiff == nil {
		t.Fatal("expected coverage_diff to be set")
	}
	if !result.CoverageDiff.Passed {
		t.Error("expected coverage_diff.passed=true")
	}
	if result.CoverageDiff.NewLinesTotal != 20 {
		t.Errorf("expected new_lines_total=20, got %d", result.CoverageDiff.NewLinesTotal)
	}
}

func TestMapCoverageResult_NilFields(t *testing.T) {
	m := map[string]any{}

	result := mapCoverageResult(m)

	if result.CoverageSnapshotID != nil {
		t.Errorf("expected nil snapshot_id, got %v", result.CoverageSnapshotID)
	}
	if result.CoverageRate != nil {
		t.Errorf("expected nil coverage_rate, got %v", result.CoverageRate)
	}
	if result.CoverageDiff != nil {
		t.Errorf("expected nil coverage_diff, got %v", result.CoverageDiff)
	}
}

func TestMapBatchStatus(t *testing.T) {
	m := map[string]any{
		"batch_id":        float64(7),
		"status":          "completed",
		"expected_count":  float64(3),
		"completed_count": float64(3),
		"result": map[string]any{
			"snapshot_id":   float64(99),
			"coverage_rate": "91.20",
			"file_count":    float64(25),
			"coverage_diff": map[string]any{
				"passed":             true,
				"head_coverage_rate": "91.20",
				"base_coverage_rate": "88.00",
				"coverage_delta":     "+3.20",
				"new_lines_total":    float64(50),
				"new_lines_covered":  float64(48),
				"failure_reasons":    []any{},
			},
		},
	}

	status := mapBatchStatus(m)

	if status.BatchID != 7 {
		t.Errorf("expected batch_id=7, got %d", status.BatchID)
	}
	if status.Status != "completed" {
		t.Errorf("expected status=completed, got %s", status.Status)
	}
	if status.ExpectedCount != 3 {
		t.Errorf("expected expected_count=3, got %d", status.ExpectedCount)
	}
	if status.CompletedCount != 3 {
		t.Errorf("expected completed_count=3, got %d", status.CompletedCount)
	}
	// CoverageResult fields (from embedded struct)
	if status.CoverageSnapshotID == nil || *status.CoverageSnapshotID != 99 {
		t.Errorf("expected snapshot_id=99, got %v", status.CoverageSnapshotID)
	}
	if status.CoverageRate == nil || *status.CoverageRate != "91.20" {
		t.Errorf("expected coverage_rate=91.20, got %v", status.CoverageRate)
	}
	if status.FileCount == nil || *status.FileCount != 25 {
		t.Errorf("expected file_count=25, got %v", status.FileCount)
	}
	if status.CoverageDiff == nil {
		t.Fatal("expected coverage_diff to be set")
	}
	if !status.CoverageDiff.Passed {
		t.Error("expected coverage_diff.passed=true")
	}
	if status.CoverageDiff.BaseCoverageRate == nil || *status.CoverageDiff.BaseCoverageRate != "88.00" {
		t.Errorf("expected base_coverage_rate=88.00, got %v", status.CoverageDiff.BaseCoverageRate)
	}
}

func TestMapBatchStatus_Pending(t *testing.T) {
	m := map[string]any{
		"batch_id":        float64(5),
		"status":          "pending",
		"expected_count":  float64(3),
		"completed_count": float64(1),
	}

	status := mapBatchStatus(m)

	if status.Status != "pending" {
		t.Errorf("expected status=pending, got %s", status.Status)
	}
	if status.CoverageRate != nil {
		t.Errorf("expected nil coverage_rate for pending batch, got %v", status.CoverageRate)
	}
	if status.CoverageDiff != nil {
		t.Errorf("expected nil coverage_diff for pending batch")
	}
}

func TestMapBatchStatus_Failed(t *testing.T) {
	errMsg := "Batch timed out: expected 3 uploads, received 2"
	m := map[string]any{
		"batch_id":        float64(5),
		"status":          "failed",
		"expected_count":  float64(3),
		"completed_count": float64(2),
		"error_message":   errMsg,
	}

	status := mapBatchStatus(m)

	if status.Status != "failed" {
		t.Errorf("expected status=failed, got %s", status.Status)
	}
	if status.ErrorMessage == nil || *status.ErrorMessage != errMsg {
		t.Errorf("expected error_message=%q, got %v", errMsg, status.ErrorMessage)
	}
}

func TestCoverageResultEmbedding_JSONShape(t *testing.T) {
	// Verify that embedded CoverageResult fields are promoted to top level in JSON,
	// not nested under a "CoverageResult" key.
	rate := "85.00"
	count := 10
	resp := CoverageStatusResponse{
		UploadID: 1,
		Status:   "completed",
		CoverageResult: CoverageResult{
			CoverageRate: &rate,
			FileCount:    &count,
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// Fields should be at top level, not nested
	if parsed["coverage_rate"] != "85.00" {
		t.Errorf("expected top-level coverage_rate=85.00, got %v", parsed["coverage_rate"])
	}
	if parsed["file_count"].(float64) != 10 {
		t.Errorf("expected top-level file_count=10, got %v", parsed["file_count"])
	}
	// Should NOT have a "CoverageResult" key
	if _, ok := parsed["CoverageResult"]; ok {
		t.Error("embedded CoverageResult should not appear as a named key in JSON")
	}
}

func TestBatchStatusResponse_JSONShape(t *testing.T) {
	rate := "91.20"
	resp := CoverageBatchStatusResponse{
		BatchID:        7,
		Status:         "completed",
		ExpectedCount:  3,
		CompletedCount: 3,
		CoverageResult: CoverageResult{
			CoverageRate: &rate,
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if parsed["batch_id"].(float64) != 7 {
		t.Errorf("expected batch_id=7, got %v", parsed["batch_id"])
	}
	if parsed["coverage_rate"] != "91.20" {
		t.Errorf("expected top-level coverage_rate=91.20, got %v", parsed["coverage_rate"])
	}
	if parsed["expected_count"].(float64) != 3 {
		t.Errorf("expected expected_count=3, got %v", parsed["expected_count"])
	}
}
