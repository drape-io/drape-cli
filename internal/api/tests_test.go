package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInitiateUpload_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/orgs/acme/repos/42/uploads" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var req UploadInitiateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.UploadType != "test_results" {
			t.Errorf("expected upload_type=test_results, got %s", req.UploadType)
		}
		if req.Branch != "main" {
			t.Errorf("expected branch=main, got %s", req.Branch)
		}
		if req.SHA != "abc123" {
			t.Errorf("expected sha=abc123, got %s", req.SHA)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(UploadInitiateResponse{
			UploadID:  1,
			UploadURL: "https://s3.example.com/presigned",
			ExpiresIn: 3600,
		}); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token")
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}
	resp, err := client.InitiateUpload("acme", 42, UploadInitiateRequest{
		UploadType: "test_results",
		Branch:     "main",
		SHA:        "abc123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.UploadID != 1 {
		t.Errorf("expected upload_id=1, got %d", resp.UploadID)
	}
	if resp.UploadURL != "https://s3.example.com/presigned" {
		t.Errorf("unexpected upload URL: %s", resp.UploadURL)
	}
}

func TestInitiateUpload_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if _, err := w.Write([]byte("internal error")); err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token")
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}
	_, err = client.InitiateUpload("acme", 42, UploadInitiateRequest{
		UploadType: "test_results",
		Branch:     "main",
		SHA:        "abc123",
	})
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

func TestMapTestStatus(t *testing.T) {
	raw := &UploadStatusResponse{
		UploadID:   1,
		UploadType: "test_results",
		Status:     "completed",
		Result: map[string]any{
			"tests_ingested":             float64(10),
			"suppressed_count":           float64(2),
			"suppressed_tests":           []any{"test_a", "test_b"},
			"failed_count":               float64(3),
			"unsuppressed_failure_count": float64(1),
		},
	}

	result := mapTestStatus(raw)

	if result.UploadID != 1 {
		t.Errorf("expected upload_id=1, got %d", result.UploadID)
	}
	if result.Status != "completed" {
		t.Errorf("expected status=completed, got %s", result.Status)
	}
	if result.TestsIngested == nil || *result.TestsIngested != 10 {
		t.Errorf("expected tests_ingested=10, got %v", result.TestsIngested)
	}
	if result.SuppressedCount == nil || *result.SuppressedCount != 2 {
		t.Errorf("expected suppressed_count=2, got %v", result.SuppressedCount)
	}
	if len(result.SuppressedTests) != 2 {
		t.Errorf("expected 2 suppressed tests, got %d", len(result.SuppressedTests))
	}
	if result.FailedCount == nil || *result.FailedCount != 3 {
		t.Errorf("expected failed_count=3, got %v", result.FailedCount)
	}
	if result.UnsuppressedFailureCount == nil || *result.UnsuppressedFailureCount != 1 {
		t.Errorf("expected unsuppressed_failure_count=1, got %v", result.UnsuppressedFailureCount)
	}
}
