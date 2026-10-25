package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drape-io/drape-cli/internal/exitcode"
	"github.com/drape-io/drape-cli/internal/output"
)

// drapeServer answers a lint upload: the repo lookup, then `upload` for the initiate request.
func drapeServer(t *testing.T, upload http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/orgs/acme/repos/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 7, "name": "acme/web"}})
	})
	mux.HandleFunc("/api/v1/orgs/acme/repos/7/uploads", upload)
	mux.HandleFunc("/put", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("/api/v1/orgs/acme/repos/7/uploads/5/complete", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Drape-Billing-Warning", "trial_expiring")
		w.Header().Set("X-Drape-Billing-Days-Remaining", "2")
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// uploadLint runs `drape upload lint` against server and returns its exit code and stderr.
func uploadLint(t *testing.T, server *httptest.Server) (int, string) {
	t.Helper()
	report := filepath.Join(t.TempDir(), "lint.sarif")
	if err := os.WriteFile(report, []byte(`{"runs": []}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	output.Stderr = &stderr
	output.Stdout = &bytes.Buffer{}
	t.Cleanup(output.Reset)
	t.Setenv("GITHUB_ACTIONS", "true")
	rootCmd.SetArgs([]string{
		"upload", "lint", report, "--org", "acme", "--repo", "acme/web", "--api-key", "key",
		"--api-url", server.URL, "--branch", "main", "--sha", "abc123", "--wait=false",
	})
	return Execute(), stderr.String()
}

func TestARefusedUploadExitsZeroAndSaysWhy(t *testing.T) {
	server := drapeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"code":        "payment_required",
			"detail":      "This organization's Drape access has ended; nothing was saved.",
			"upgrade_url": "https://app.drape.io/acme/upgrade/",
		})
	})

	code, stderr := uploadLint(t, server)

	if code != exitcode.Success {
		t.Errorf("exit code = %d, want %d: billing must never fail CI", code, exitcode.Success)
	}
	for _, want := range []string{
		"results from this run were not saved",
		"An admin can restore it at https://app.drape.io/acme/upgrade/.",
		"::warning title=Drape access has ended::",
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, stderr)
		}
	}
	if strings.Contains(stderr, "Error:") {
		t.Errorf("a billing refusal is a warning, not an error:\n%s", stderr)
	}
}

func TestABillingWarningIsShownOnASuccessfulUpload(t *testing.T) {
	server := drapeServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"upload_id": 5, "upload_url": "http://" + r.Host + "/put"})
	})

	code, stderr := uploadLint(t, server)

	if code != exitcode.Success {
		t.Errorf("exit code = %d, want %d", code, exitcode.Success)
	}
	if !strings.Contains(stderr, "this organization's trial ends in 2 days") {
		t.Errorf("stderr missing the trial warning:\n%s", stderr)
	}
}

func TestOtherUploadFailuresStillFail(t *testing.T) {
	server := drapeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Invalid API key"})
	})

	code, _ := uploadLint(t, server)

	if code != exitcode.UploadError {
		t.Errorf("exit code = %d, want %d", code, exitcode.UploadError)
	}
}
