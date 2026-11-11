package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/drape-io/drape-cli/internal/exitcode"
	"github.com/drape-io/drape-cli/internal/output"
)

// outcomeServer fakes Drape's upload API. Each upload's processing outcome
// depends on its file name: "good*" completes, "bad*" fails to parse, and
// "slow*" never finishes.
func outcomeServer(t *testing.T) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	names := map[int]string{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/orgs/acme/repos/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 7, "name": "acme/web"}})
	})
	mux.HandleFunc("POST /api/v1/orgs/acme/repos/7/uploads", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Filename string `json:"filename"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		id := len(names) + 1
		names[id] = req.Filename
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"upload_id": id, "upload_url": "http://" + r.Host + "/put"})
	})
	mux.HandleFunc("/put", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("POST /api/v1/orgs/acme/repos/7/upload-batches", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"batch_id": 9})
	})
	mux.HandleFunc("POST /api/v1/orgs/acme/repos/7/uploads/{id}/complete", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("GET /api/v1/orgs/acme/repos/7/uploads/{id}/status", func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.Atoi(r.PathValue("id"))
		mu.Lock()
		name := names[id]
		mu.Unlock()
		status := map[string]any{"upload_id": id, "status": "completed", "result": map[string]any{"tests_ingested": 1}}
		switch {
		case strings.HasPrefix(name, "bad"):
			status = map[string]any{"upload_id": id, "status": "failed", "error_message": "Failed to parse " + name}
		case strings.HasPrefix(name, "slow"):
			status = map[string]any{"upload_id": id, "status": "processing"}
		}
		_ = json.NewEncoder(w).Encode(status)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// reports writes an empty report for each name and returns their paths. A name ending in "/" becomes a directory, which can't be read.
func reports(t *testing.T, names ...string) []string {
	t.Helper()
	paths := make([]string, 0, len(names))
	for _, name := range names {
		if dirName, ok := strings.CutSuffix(name, "/"); ok {
			path := filepath.Join(t.TempDir(), dirName)
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			paths = append(paths, path)
			continue
		}
		paths = append(paths, writeReport(t, name, `{"runs": []}`))
	}
	return paths
}

// upload runs `drape upload <command> <paths> --json` against server and
// returns the exit code and the JSON result the command printed, or "" if none.
func upload(t *testing.T, server *httptest.Server, command string, paths []string, extra ...string) (int, string) {
	t.Helper()
	output.Stdout = &bytes.Buffer{}
	output.Stderr = &bytes.Buffer{}
	t.Cleanup(output.Reset)
	// Cobra keeps flag values between Execute calls, and other tests leave
	// some set (e.g. --wait=false), so start from the defaults.
	resetFlags := func() {
		pendingJSON = nil
		flagJSON, flagQuiet = false, false
		flagUploadWait, flagUploadWaitTimeout = true, 3*time.Minute
		flagTestFormat, flagCovFormat = "", ""
		flagLintFormat, flagScanFormat = "sarif", "sarif"
	}
	resetFlags()
	t.Cleanup(resetFlags)
	args := append([]string{"upload", command}, paths...)
	args = append(args, "--org", "acme", "--repo", "acme/web", "--api-key", "key", "--api-url", server.URL,
		"--branch", "main", "--sha", "abc123", "--json")
	rootCmd.SetArgs(append(args, extra...))
	code := Execute()
	if pendingJSON == nil {
		return code, ""
	}
	printed, err := json.Marshal(pendingJSON)
	if err != nil {
		t.Fatal(err)
	}
	return code, string(printed)
}

// uploadJSON parses the --json result.
func uploadJSON(t *testing.T, printed string) UploadResult {
	t.Helper()
	var result UploadResult
	if err := json.Unmarshal([]byte(printed), &result); err != nil {
		t.Fatalf("not a JSON result: %v\n%s", err, printed)
	}
	return result
}

// A report that fails to process must fail the run, even when another report in it succeeded.
func TestOneFailedReportFailsTheRun(t *testing.T) {
	cases := map[string]struct {
		command string
		names   []string
		extra   []string
	}{
		"tests, bad last":           {"tests", []string{"good.xml", "bad.xml"}, nil},
		"tests, bad first":          {"tests", []string{"bad.xml", "good.xml"}, nil},
		"scan, bad last":            {"scan", []string{"good.json", "bad.json"}, nil},
		"scan, bad first":           {"scan", []string{"bad.json", "good.json"}, nil},
		"tests, unreadable":         {"tests", []string{"good.xml", "dir/"}, nil},
		"scan, unreadable":          {"scan", []string{"good.json", "dir/"}, nil},
		"tests no wait, unreadable": {"tests", []string{"good.xml", "dir/"}, []string{"--wait=false"}},
		"scan no wait, unreadable":  {"scan", []string{"good.json", "dir/"}, []string{"--wait=false"}},
		"coverage, all unreadable":  {"coverage", []string{"dir1/", "dir2/"}, []string{"--format", "cobertura"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			code, printed := upload(t, outcomeServer(t), tc.command, reports(t, tc.names...), tc.extra...)
			if code != exitcode.UploadError {
				t.Fatalf("exit = %d, want %d\n%s", code, exitcode.UploadError, printed)
			}
			result := uploadJSON(t, printed)
			if len(result.Uploads)+len(result.Failures) != len(tc.names) {
				t.Errorf("JSON reports %d uploads and %d failures, want one entry per file (%d):\n%s",
					len(result.Uploads), len(result.Failures), len(tc.names), printed)
			}
			// Every file that didn't make it says why.
			for _, f := range result.Failures {
				if f.Error == "" {
					t.Errorf("failure for %s has no error:\n%s", f.Filename, printed)
				}
			}
			for _, u := range result.Uploads {
				if strings.HasPrefix(u.Filename, "bad") && u.Error == "" {
					t.Errorf("upload %s failed processing but has no error:\n%s", u.Filename, printed)
				}
			}
		})
	}
}

func TestAllReportsSucceedingStillPasses(t *testing.T) {
	for _, command := range []string{"tests", "scan"} {
		t.Run(command, func(t *testing.T) {
			code, printed := upload(t, outcomeServer(t), command, reports(t, "good1.json", "good2.json"))
			if code != exitcode.Success {
				t.Fatalf("exit = %d, want 0\n%s", code, printed)
			}
		})
	}
}

// Once an upload exists, every way the command can fail still prints its
// JSON result, so automation can find the upload and why it failed.
func TestFailuresAfterUploadStillPrintTheJSONResult(t *testing.T) {
	cases := map[string]struct {
		command  string
		name     string
		extra    []string
		wantCode int
	}{
		"coverage parse failure": {"coverage", "bad.xml", []string{"--format", "cobertura"}, exitcode.UploadError},
		"lint parse failure":     {"lint", "bad.sarif", nil, exitcode.UploadError},
		"scan parse failure":     {"scan", "bad.json", nil, exitcode.UploadError},
		"tests timeout":          {"tests", "slow.xml", []string{"--wait-timeout", "1ns"}, exitcode.Timeout},
		"coverage timeout":       {"coverage", "slow.xml", []string{"--format", "cobertura", "--wait-timeout", "1ns"}, exitcode.Timeout},
		"lint timeout":           {"lint", "slow.sarif", []string{"--wait-timeout", "1ns"}, exitcode.Timeout},
		"scan timeout":           {"scan", "slow.json", []string{"--wait-timeout", "1ns"}, exitcode.Timeout},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			code, printed := upload(t, outcomeServer(t), tc.command, reports(t, tc.name), tc.extra...)
			if code != tc.wantCode {
				t.Fatalf("exit = %d, want %d\n%s", code, tc.wantCode, printed)
			}
			result := uploadJSON(t, printed)
			if len(result.Uploads) != 1 || result.Uploads[0].UploadID != 1 {
				t.Fatalf("JSON should name the upload (ID 1):\n%s", printed)
			}
			if result.Uploads[0].Error == "" {
				t.Errorf("JSON should say why the upload failed:\n%s", printed)
			}
		})
	}
}
