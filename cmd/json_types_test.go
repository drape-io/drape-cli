package cmd

import (
	"encoding/json"
	"testing"

	"github.com/drape-io/drape-cli/internal/api"
)

func TestUploadResultSingleFile(t *testing.T) {
	rate := "85.5"
	count := 42
	result := UploadResult{
		FilesMatched:  1,
		FilesUploaded: 1,
		Uploads: []UploadEntry{
			{
				Filename: "coverage.xml",
				UploadID: 123,
				Result: &api.CoverageStatusResponse{
					UploadID: 123,
					Status:   "completed",
					CoverageResult: api.CoverageResult{
						CoverageRate: &rate,
						FileCount:    &count,
					},
				},
			},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if parsed["files_matched"].(float64) != 1 {
		t.Errorf("files_matched = %v, want 1", parsed["files_matched"])
	}
	if parsed["files_uploaded"].(float64) != 1 {
		t.Errorf("files_uploaded = %v, want 1", parsed["files_uploaded"])
	}

	uploads := parsed["uploads"].([]any)
	if len(uploads) != 1 {
		t.Fatalf("uploads length = %d, want 1", len(uploads))
	}

	entry := uploads[0].(map[string]any)
	if entry["filename"] != "coverage.xml" {
		t.Errorf("filename = %v, want coverage.xml", entry["filename"])
	}
	if entry["upload_id"].(float64) != 123 {
		t.Errorf("upload_id = %v, want 123", entry["upload_id"])
	}

	resultMap := entry["result"].(map[string]any)
	if resultMap["status"] != "completed" {
		t.Errorf("result.status = %v, want completed", resultMap["status"])
	}
	if resultMap["coverage_rate"] != "85.5" {
		t.Errorf("result.coverage_rate = %v, want 85.5", resultMap["coverage_rate"])
	}
}

func TestUploadResultNilResult(t *testing.T) {
	result := UploadResult{
		FilesMatched:  1,
		FilesUploaded: 1,
		Uploads: []UploadEntry{
			{Filename: "cov.xml", UploadID: 456},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	entry := parsed["uploads"].([]any)[0].(map[string]any)
	if _, ok := entry["result"]; ok {
		t.Error("expected result to be omitted when nil")
	}
}

func TestUploadResultMultiFile(t *testing.T) {
	ingested := 100
	failed := 2
	result := UploadResult{
		FilesMatched:  3,
		FilesUploaded: 2,
		Uploads: []UploadEntry{
			{
				Filename: "tests-1.xml",
				UploadID: 10,
				Result: &api.TestStatusResponse{
					UploadID:      10,
					Status:        "completed",
					TestsIngested: &ingested,
					FailedCount:   &failed,
				},
			},
			{
				Filename: "tests-2.xml",
				UploadID: 11,
			},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if parsed["files_matched"].(float64) != 3 {
		t.Errorf("files_matched = %v, want 3", parsed["files_matched"])
	}
	if parsed["files_uploaded"].(float64) != 2 {
		t.Errorf("files_uploaded = %v, want 2", parsed["files_uploaded"])
	}

	uploads := parsed["uploads"].([]any)
	if len(uploads) != 2 {
		t.Fatalf("uploads length = %d, want 2", len(uploads))
	}

	first := uploads[0].(map[string]any)
	if first["filename"] != "tests-1.xml" {
		t.Errorf("uploads[0].filename = %v, want tests-1.xml", first["filename"])
	}
	if first["result"] == nil {
		t.Error("uploads[0].result should not be nil")
	}

	second := uploads[1].(map[string]any)
	if _, ok := second["result"]; ok {
		t.Error("uploads[1].result should be omitted when nil")
	}
}

func TestUploadResultLintStatus(t *testing.T) {
	violations := 5
	result := UploadResult{
		FilesMatched:  1,
		FilesUploaded: 1,
		Uploads: []UploadEntry{
			{
				Filename: "lint.sarif",
				UploadID: 789,
				Result: &api.LintStatusResponse{
					UploadID:        789,
					Status:          "completed",
					TotalViolations: &violations,
				},
			},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	entry := parsed["uploads"].([]any)[0].(map[string]any)
	resultMap := entry["result"].(map[string]any)
	if resultMap["total_violations"].(float64) != 5 {
		t.Errorf("total_violations = %v, want 5", resultMap["total_violations"])
	}
}

func TestUploadResultScanStatus(t *testing.T) {
	vulns := 10
	result := UploadResult{
		FilesMatched:  1,
		FilesUploaded: 1,
		Uploads: []UploadEntry{
			{
				Filename: "scan.sarif",
				UploadID: 50,
				Result: &api.ScanStatusResponse{
					UploadID:             50,
					Status:               "completed",
					TotalVulnerabilities: &vulns,
				},
			},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	entry := parsed["uploads"].([]any)[0].(map[string]any)
	resultMap := entry["result"].(map[string]any)
	if resultMap["total_vulnerabilities"].(float64) != 10 {
		t.Errorf("total_vulnerabilities = %v, want 10", resultMap["total_vulnerabilities"])
	}
}

func TestDryRunResultJSON(t *testing.T) {
	result := DryRunResult{
		DryRun: true,
		Files:  []string{"a.xml", "b.xml"},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if parsed["dry_run"] != true {
		t.Errorf("dry_run = %v, want true", parsed["dry_run"])
	}

	files := parsed["files"].([]any)
	if len(files) != 2 {
		t.Errorf("files length = %d, want 2", len(files))
	}
}

func TestTestsDryRunResultJSON(t *testing.T) {
	result := TestsDryRunResult{
		DryRun: true,
		Files: []TestsDryRunFile{
			{
				Filename: "test.xml",
				Total:    10,
				Passed:   8,
				Failed:   1,
				Skipped:  1,
				Errored:  0,
			},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	files := parsed["files"].([]any)
	first := files[0].(map[string]any)
	if first["total"].(float64) != 10 {
		t.Errorf("files[0].total = %v, want 10", first["total"])
	}
	if first["passed"].(float64) != 8 {
		t.Errorf("files[0].passed = %v, want 8", first["passed"])
	}
}

func TestUploadResultWithCoverageDiff(t *testing.T) {
	rate := "90.0"
	baseRate := "88.0"
	delta := "+2.0"
	result := UploadResult{
		FilesMatched:  1,
		FilesUploaded: 1,
		Uploads: []UploadEntry{
			{
				Filename: "cov.xml",
				UploadID: 100,
				Result: &api.CoverageStatusResponse{
					UploadID: 100,
					Status:   "completed",
					CoverageResult: api.CoverageResult{
						CoverageRate: &rate,
						CoverageDiff: &api.CoverageDiffInfo{
							Passed:           true,
							BaseCoverageRate: &baseRate,
							HeadCoverageRate: "90.0",
							CoverageDelta:    &delta,
							NewLinesTotal:    50,
							NewLinesCovered:  45,
							FailureReasons:   []string{},
						},
					},
				},
			},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	entry := parsed["uploads"].([]any)[0].(map[string]any)
	resultMap := entry["result"].(map[string]any)
	diff := resultMap["coverage_diff"].(map[string]any)
	if diff["passed"] != true {
		t.Errorf("coverage_diff.passed = %v, want true", diff["passed"])
	}
	if diff["base_coverage_rate"] != "88.0" {
		t.Errorf("base_coverage_rate = %v, want 88.0", diff["base_coverage_rate"])
	}
	if diff["new_lines_total"].(float64) != 50 {
		t.Errorf("new_lines_total = %v, want 50", diff["new_lines_total"])
	}
}

func TestUploadResultBatchCoverage(t *testing.T) {
	// Batch coverage attaches merged result to first upload entry.
	rate := "91.20"
	count := 25
	result := UploadResult{
		FilesMatched:  3,
		FilesUploaded: 3,
		Uploads: []UploadEntry{
			{
				Filename: "unit.xml",
				UploadID: 10,
				Result: &api.CoverageStatusResponse{
					Status: "completed",
					CoverageResult: api.CoverageResult{
						CoverageRate: &rate,
						FileCount:    &count,
					},
				},
			},
			{Filename: "integration.xml", UploadID: 11},
			{Filename: "e2e.xml", UploadID: 12},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if parsed["files_matched"].(float64) != 3 {
		t.Errorf("files_matched = %v, want 3", parsed["files_matched"])
	}

	uploads := parsed["uploads"].([]any)
	if len(uploads) != 3 {
		t.Fatalf("uploads length = %d, want 3", len(uploads))
	}

	// First entry has the merged result
	first := uploads[0].(map[string]any)
	resultMap := first["result"].(map[string]any)
	if resultMap["coverage_rate"] != "91.20" {
		t.Errorf("result.coverage_rate = %v, want 91.20", resultMap["coverage_rate"])
	}
	if resultMap["file_count"].(float64) != 25 {
		t.Errorf("result.file_count = %v, want 25", resultMap["file_count"])
	}

	// Other entries have no result
	second := uploads[1].(map[string]any)
	if _, ok := second["result"]; ok {
		t.Error("uploads[1].result should be omitted")
	}
	third := uploads[2].(map[string]any)
	if _, ok := third["result"]; ok {
		t.Error("uploads[2].result should be omitted")
	}
}

func TestUnifiedShapeConsistency(t *testing.T) {
	// Verify that all upload types produce the same top-level JSON shape.
	// This is what the GitHub Action consumer depends on.
	types := []UploadResult{
		{FilesMatched: 1, FilesUploaded: 1, Uploads: []UploadEntry{{Filename: "cov.xml", UploadID: 1}}},
		{FilesMatched: 3, FilesUploaded: 2, Uploads: []UploadEntry{{Filename: "a.xml", UploadID: 2}, {Filename: "b.xml", UploadID: 3}}},
	}

	for i, r := range types {
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("case %d: Marshal error: %v", i, err)
		}

		var parsed map[string]any
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("case %d: Unmarshal error: %v", i, err)
		}

		// All results must have these top-level keys
		for _, key := range []string{"uploads", "files_matched", "files_uploaded"} {
			if _, ok := parsed[key]; !ok {
				t.Errorf("case %d: missing top-level key %q", i, key)
			}
		}
	}
}
