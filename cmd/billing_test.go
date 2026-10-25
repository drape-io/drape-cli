package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	return run(t, server, "lint", 1, false)
}

// run runs `drape upload <command>` with `files` report files against server and returns its exit
// code and stderr.
func run(t *testing.T, server *httptest.Server, command string, files int, json bool) (int, string) {
	t.Helper()
	args := make([]string, 0, files+16)
	args = append(args, "upload", command)
	for i := range files {
		report := filepath.Join(t.TempDir(), fmt.Sprintf("report%d.xml", i))
		if err := os.WriteFile(report, []byte(`{"runs": []}`), 0o600); err != nil {
			t.Fatal(err)
		}
		args = append(args, report)
	}
	var stderr bytes.Buffer
	output.Stderr = &stderr
	output.Stdout = &bytes.Buffer{}
	t.Cleanup(output.Reset)
	t.Cleanup(func() { flagJSON, flagQuiet = false, false })
	t.Setenv("GITHUB_ACTIONS", "true")
	args = append(args, "--org", "acme", "--repo", "acme/web", "--api-key", "key",
		"--api-url", server.URL, "--branch", "main", "--sha", "abc123", "--wait=false",
		fmt.Sprintf("--json=%t", json))
	rootCmd.SetArgs(args)
	return Execute(), stderr.String()
}

func refuse(w http.ResponseWriter, upgradeURL string) {
	w.WriteHeader(http.StatusPaymentRequired)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"code":        "payment_required",
		"detail":      "This organization's Drape access has ended; nothing was saved.",
		"upgrade_url": upgradeURL,
	})
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

func TestA402ThatIsNotDrapesRefusalStillFails(t *testing.T) {
	server := drapeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte("<html>Payment required by proxy</html>"))
	})

	code, _ := uploadLint(t, server)

	if code != exitcode.UploadError {
		t.Errorf("exit code = %d, want %d: only Drape's own refusal passes", code, exitcode.UploadError)
	}
}

func TestAnUpgradeURLThatCouldInjectAWorkflowCommandIsDropped(t *testing.T) {
	server := drapeServer(t, func(w http.ResponseWriter, r *http.Request) {
		refuse(w, "https://x\n::add-mask::secret")
	})

	code, stderr := uploadLint(t, server)

	if code != exitcode.Success {
		t.Errorf("exit code = %d, want %d", code, exitcode.Success)
	}
	for _, line := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(line, "::add-mask") {
			t.Errorf("stderr starts a line with a workflow command:\n%s", stderr)
		}
	}
	if strings.Contains(stderr, "restore it at") {
		t.Errorf("an unsafe upgrade URL was printed:\n%s", stderr)
	}
}

func TestARefusalStopsAMultiFileUploadAndSaysSoInJSON(t *testing.T) {
	var initiated int
	server := drapeServer(t, func(w http.ResponseWriter, r *http.Request) {
		initiated++
		refuse(w, "https://app.drape.io/acme/upgrade/")
	})

	code, _ := run(t, server, "tests", 3, true)

	if code != exitcode.Success {
		t.Errorf("exit code = %d, want %d", code, exitcode.Success)
	}
	if initiated != 1 {
		t.Errorf("initiated %d uploads, want 1: the others would be refused too", initiated)
	}
	// What Execute wrote as JSON to the real stdout.
	result, ok := pendingJSON.(UploadResult)
	if !ok {
		t.Fatalf("JSON result = %#v, want an UploadResult", pendingJSON)
	}
	if result.Billing == nil || result.Billing.Status != "payment_required" {
		t.Errorf("billing = %+v, want payment_required", result.Billing)
	}
	if result.Uploads == nil {
		t.Errorf("uploads is null, want an empty list")
	}
}

func TestARefusalAfterSomeUploadsSaysSomeWereSaved(t *testing.T) {
	var initiated int
	server := drapeServer(t, func(w http.ResponseWriter, r *http.Request) {
		initiated++
		if initiated > 1 {
			refuse(w, "https://app.drape.io/acme/upgrade/")
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"upload_id": 5, "upload_url": "http://" + r.Host + "/put"})
	})

	code, stderr := run(t, server, "tests", 2, false)

	if code != exitcode.Success {
		t.Errorf("exit code = %d, want %d", code, exitcode.Success)
	}
	if !strings.Contains(stderr, "some results from this run were not saved") {
		t.Errorf("stderr should say only some results were lost:\n%s", stderr)
	}
}
