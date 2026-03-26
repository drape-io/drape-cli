package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLookupRepo_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			t.Errorf("expected Bearer test-token, got %q", auth)
		}

		// Verify path and query
		if r.URL.Path != "/api/v1/orgs/acme/repos/" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("name") != "my-service" {
			t.Errorf("unexpected name query param: %s", r.URL.Query().Get("name"))
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode([]RepoInfo{{ID: 42, Name: "my-service"}}); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token")
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}
	repo, err := client.LookupRepo("acme", "my-service")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.ID != 42 {
		t.Errorf("expected repo ID 42, got %d", repo.ID)
	}
	if repo.Name != "my-service" {
		t.Errorf("expected repo name my-service, got %s", repo.Name)
	}
}

func TestLookupRepo_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode([]RepoInfo{}); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token")
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}
	_, err = client.LookupRepo("acme", "nonexistent")
	if err == nil {
		t.Fatal("expected error for not found repo")
	}
}

func TestLookupRepo_AuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		if err := json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid token"}); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "bad-token")
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}
	_, err = client.LookupRepo("acme", "my-service")
	if err == nil {
		t.Fatal("expected error for auth failure")
	}
}

func TestRetries_ServerError(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			if _, err := w.Write([]byte("server error")); err != nil {
				t.Fatalf("failed to write response: %v", err)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode([]RepoInfo{{ID: 1, Name: "repo"}}); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token")
	if err != nil {
		t.Fatalf("unexpected error creating client: %v", err)
	}
	repo, err := client.LookupRepo("acme", "repo")
	if err != nil {
		t.Fatalf("unexpected error after retries: %v", err)
	}
	if repo.ID != 1 {
		t.Errorf("expected repo ID 1, got %d", repo.ID)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}
