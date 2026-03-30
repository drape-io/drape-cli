package oidc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectAndFetchToken_GitHub(t *testing.T) {
	wantAudience := "https://app.drape.io/oidc/my-org"
	wantToken := "eyJhbGciOiJSUzI1NiJ9.test-token"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("audience"); got != wantAudience {
			t.Errorf("audience = %q, want %q", got, wantAudience)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer req-token" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer req-token")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"value": wantToken})
	}))
	defer srv.Close()

	env := func(key string) string {
		switch key {
		case "ACTIONS_ID_TOKEN_REQUEST_URL":
			return srv.URL + "?param=existing"
		case "ACTIONS_ID_TOKEN_REQUEST_TOKEN":
			return "req-token"
		}
		return ""
	}

	token, err := DetectAndFetchToken(env, "https://app.drape.io", "my-org")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != wantToken {
		t.Errorf("token = %q, want %q", token, wantToken)
	}
}

func TestDetectAndFetchToken_TrailingSlashInAPIURL(t *testing.T) {
	wantAudience := "https://app.drape.io/oidc/my-org"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("audience"); got != wantAudience {
			t.Errorf("audience = %q, want %q (trailing slash should be stripped)", got, wantAudience)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"value": "eyJ.token"})
	}))
	defer srv.Close()

	env := func(key string) string {
		switch key {
		case "ACTIONS_ID_TOKEN_REQUEST_URL":
			return srv.URL + "?"
		case "ACTIONS_ID_TOKEN_REQUEST_TOKEN":
			return "tok"
		}
		return ""
	}

	// API URL with trailing slash should not produce double-slash in audience
	_, err := DetectAndFetchToken(env, "https://app.drape.io/", "my-org")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDetectAndFetchToken_NoProvider(t *testing.T) {
	env := func(string) string { return "" }

	token, err := DetectAndFetchToken(env, "https://app.drape.io", "my-org")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "" {
		t.Errorf("token = %q, want empty", token)
	}
}

func TestDetectAndFetchToken_GitLabInlineToken(t *testing.T) {
	wantToken := "eyJhbGciOiJSUzI1NiJ9.gitlab-token"

	env := func(key string) string {
		if key == "DRAPE_OIDC_TOKEN" {
			return wantToken
		}
		return ""
	}

	token, err := DetectAndFetchToken(env, "https://app.drape.io", "my-org")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != wantToken {
		t.Errorf("token = %q, want %q", token, wantToken)
	}
}

func TestDetectAndFetchToken_GitLabTokenFile(t *testing.T) {
	wantToken := "eyJhbGciOiJSUzI1NiJ9.gitlab-token"

	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenFile, []byte(wantToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	env := func(key string) string {
		if key == "DRAPE_OIDC_TOKEN_FILE" {
			return tokenFile
		}
		return ""
	}

	token, err := DetectAndFetchToken(env, "https://app.drape.io", "my-org")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != wantToken {
		t.Errorf("token = %q, want %q (trailing newline should be trimmed)", token, wantToken)
	}
}

func TestDetectAndFetchToken_GitLabInlineTokenOverridesFile(t *testing.T) {
	// When both DRAPE_OIDC_TOKEN and DRAPE_OIDC_TOKEN_FILE are set, inline wins.
	env := func(key string) string {
		switch key {
		case "DRAPE_OIDC_TOKEN":
			return "eyJ.inline"
		case "DRAPE_OIDC_TOKEN_FILE":
			return "/should/not/be/read"
		}
		return ""
	}

	token, err := DetectAndFetchToken(env, "https://app.drape.io", "my-org")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "eyJ.inline" {
		t.Errorf("token = %q, want %q", token, "eyJ.inline")
	}
}

func TestGitHubProvider_NotAvailable(t *testing.T) {
	p := NewGitHubProvider(func(string) string { return "" })
	if p.Available() {
		t.Error("expected Available() = false")
	}
}

func TestGitHubProvider_FetchToken_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden"))
	}))
	defer srv.Close()

	p := NewGitHubProvider(func(key string) string {
		switch key {
		case "ACTIONS_ID_TOKEN_REQUEST_URL":
			return srv.URL + "?"
		case "ACTIONS_ID_TOKEN_REQUEST_TOKEN":
			return "tok"
		}
		return ""
	})

	_, err := p.FetchToken("aud")
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
}

func TestGitHubProvider_FetchToken_EmptyValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"value": ""})
	}))
	defer srv.Close()

	p := NewGitHubProvider(func(key string) string {
		switch key {
		case "ACTIONS_ID_TOKEN_REQUEST_URL":
			return srv.URL + "?"
		case "ACTIONS_ID_TOKEN_REQUEST_TOKEN":
			return "tok"
		}
		return ""
	})

	_, err := p.FetchToken("aud")
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestGitLabProvider_NotAvailable(t *testing.T) {
	p := NewGitLabProvider(func(string) string { return "" })
	if p.Available() {
		t.Error("expected Available() = false")
	}
}

func TestGitLabProvider_MissingFile(t *testing.T) {
	p := NewGitLabProvider(func(key string) string {
		if key == "DRAPE_OIDC_TOKEN_FILE" {
			return "/nonexistent/path"
		}
		return ""
	})

	_, err := p.FetchToken("aud")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestGitHubProvider_PriorityOverGitLab(t *testing.T) {
	// When both GitHub and GitLab OIDC are available, GitHub should win.
	env := func(key string) string {
		switch key {
		case "ACTIONS_ID_TOKEN_REQUEST_URL":
			return "https://github-oidc.example.com?"
		case "ACTIONS_ID_TOKEN_REQUEST_TOKEN":
			return "gh-tok"
		case "DRAPE_OIDC_TOKEN":
			return "eyJ.gitlab-token"
		}
		return ""
	}

	ps := providers(env)
	for _, p := range ps {
		if p.Available() {
			if p.Name() != "GitHub Actions" {
				t.Errorf("first available provider = %q, want %q", p.Name(), "GitHub Actions")
			}
			return
		}
	}
	t.Fatal("expected at least one available provider")
}
