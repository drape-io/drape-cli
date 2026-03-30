package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveTokenWith_APIKeyTakesPriority(t *testing.T) {
	// Set the global flag to simulate --api-key being passed.
	old := flagAPIKey
	flagAPIKey = "drp_test_key"
	defer func() { flagAPIKey = old }()

	// Even with OIDC env vars present, the API key should win.
	env := func(key string) string {
		switch key {
		case "ACTIONS_ID_TOKEN_REQUEST_URL":
			return "https://should-not-be-called.example.com?"
		case "ACTIONS_ID_TOKEN_REQUEST_TOKEN":
			return "tok"
		}
		return ""
	}

	token, err := resolveTokenWith(env, "my-org")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "drp_test_key" {
		t.Errorf("token = %q, want %q", token, "drp_test_key")
	}
}

func TestResolveTokenWith_FallsBackToOIDC(t *testing.T) {
	wantToken := "eyJhbGciOiJSUzI1NiJ9.oidc-token"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"value": wantToken})
	}))
	defer srv.Close()

	// No API key set.
	old := flagAPIKey
	flagAPIKey = ""
	defer func() { flagAPIKey = old }()

	env := func(key string) string {
		switch key {
		case "ACTIONS_ID_TOKEN_REQUEST_URL":
			return srv.URL + "?"
		case "ACTIONS_ID_TOKEN_REQUEST_TOKEN":
			return "req-token"
		}
		return ""
	}

	token, err := resolveTokenWith(env, "my-org")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != wantToken {
		t.Errorf("token = %q, want %q", token, wantToken)
	}
}

func TestResolveTokenWith_NoAuthMethodAvailable(t *testing.T) {
	old := flagAPIKey
	flagAPIKey = ""
	defer func() { flagAPIKey = old }()

	env := func(string) string { return "" }

	_, err := resolveTokenWith(env, "my-org")
	if err == nil {
		t.Fatal("expected error when no auth method is available")
	}
}

func TestResolveTokenWith_OIDCFailureIsNonFatal(t *testing.T) {
	// OIDC server returns an error, but we should still get a clean
	// "missing auth" error, not a panic or OIDC-specific error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer srv.Close()

	old := flagAPIKey
	flagAPIKey = ""
	defer func() { flagAPIKey = old }()

	env := func(key string) string {
		switch key {
		case "ACTIONS_ID_TOKEN_REQUEST_URL":
			return srv.URL + "?"
		case "ACTIONS_ID_TOKEN_REQUEST_TOKEN":
			return "tok"
		}
		return ""
	}

	_, err := resolveTokenWith(env, "my-org")
	if err == nil {
		t.Fatal("expected error when OIDC fails and no API key")
	}
	// The error should be about missing auth, not about OIDC failure details.
	// OIDC failure is logged via output.Verbose but doesn't propagate.
}
