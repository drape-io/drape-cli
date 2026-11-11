package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/drape-io/drape-cli/internal/exitcode"
	"github.com/drape-io/drape-cli/internal/output"
)

func env(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func eventFile(t *testing.T, head, base string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "event.json")
	event := fmt.Sprintf(`{"pull_request":{"head":{"repo":{"full_name":%q}},"base":{"repo":{"full_name":%q}}}}`, head, base)
	if err := os.WriteFile(path, []byte(event), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMissingCredentialOutsideGitHubActionsAsksForAKey(t *testing.T) {
	err := missingCredential(env(nil))
	if !strings.Contains(err.Error(), "--api-key or DRAPE_API_KEY") {
		t.Fatalf("err = %v", err)
	}
}

func TestMissingCredentialInGitHubActionsAsksForTheIDTokenPermission(t *testing.T) {
	err := missingCredential(env(map[string]string{
		"GITHUB_ACTIONS":    "true",
		"GITHUB_EVENT_NAME": "pull_request",
		"GITHUB_EVENT_PATH": eventFile(t, "acme/web", "acme/web"),
	}))
	if !errors.Is(err, errNoIDTokenPermission) {
		t.Fatalf("err = %v", err)
	}
}

func TestMissingCredentialOnAForkPullRequestSaysForksCantUpload(t *testing.T) {
	err := missingCredential(env(map[string]string{
		"GITHUB_ACTIONS":    "true",
		"GITHUB_EVENT_NAME": "pull_request",
		"GITHUB_EVENT_PATH": eventFile(t, "someone/web", "acme/web"),
	}))
	if !errors.Is(err, errForkPullRequest) {
		t.Fatalf("err = %v", err)
	}
}

func TestMissingCredentialOnAPullRequestFromADeletedForkSaysForksCantUpload(t *testing.T) {
	// GitHub sends a null head repository once the fork is deleted.
	path := filepath.Join(t.TempDir(), "event.json")
	event := `{"pull_request":{"head":{"repo":null},"base":{"repo":{"full_name":"acme/web"}}}}`
	if err := os.WriteFile(path, []byte(event), 0o600); err != nil {
		t.Fatal(err)
	}
	err := missingCredential(env(map[string]string{
		"GITHUB_ACTIONS":    "true",
		"GITHUB_EVENT_NAME": "pull_request",
		"GITHUB_EVENT_PATH": path,
	}))
	if !errors.Is(err, errForkPullRequest) {
		t.Fatalf("err = %v", err)
	}
}

func TestSameRepositoryAcceptsTheFullOrBareNameInAnyCase(t *testing.T) {
	for given, want := range map[string]bool{
		"acme/web": true, "ACME/Web": true, "web": true, "Web": true, "acme/api": false, "api": false,
	} {
		if got := sameRepository(given, "acme/web"); got != want {
			t.Errorf("sameRepository(%q) = %v, want %v", given, got, want)
		}
	}
}

func TestTheAudienceIsTheAPIOrigin(t *testing.T) {
	for _, apiURL := range []string{"https://app.drape.io/", "https://app.drape.io/some/path"} {
		got, err := audienceFor(apiURL)
		if err != nil || got != "https://app.drape.io" {
			t.Errorf("audienceFor(%q) = %q, %v", apiURL, got, err)
		}
	}
}

func fakeJWT() string {
	seg := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	return seg(map[string]string{"alg": "RS256"}) + "." +
		seg(map[string]int64{"exp": time.Now().Add(5 * time.Minute).Unix()}) + ".sig"
}

// githubActionsServer plays both GitHub's token endpoint and Drape, recording the bearer of
// each Drape request.
func githubActionsServer(t *testing.T, bearers *[]string) *httptest.Server {
	t.Helper()
	token := fakeJWT()
	mux := http.NewServeMux()
	mux.HandleFunc("/github-token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"value": token})
	})
	record := func(r *http.Request) {
		*bearers = append(*bearers, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	}
	mux.HandleFunc("/api/v1/ci/identity", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		_ = json.NewEncoder(w).Encode(map[string]any{"org_slug": "acme", "repo_id": 7, "repo_name": "acme/web"})
	})
	mux.HandleFunc("/api/v1/orgs/acme/repos/", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 7, "name": "acme/web"}})
	})
	mux.HandleFunc("/api/v1/orgs/acme/repos/7/uploads", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		_ = json.NewEncoder(w).Encode(map[string]any{"upload_id": 5, "upload_url": "http://" + r.Host + "/put"})
	})
	mux.HandleFunc("/put", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("/api/v1/orgs/acme/repos/7/uploads/5/complete", func(w http.ResponseWriter, r *http.Request) {
		record(r)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", server.URL+"/github-token?api-version=2.0")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "runner-secret")
	t.Setenv("DRAPE_API_KEY", "")
	t.Setenv("DRAPE_ORG", "")
	t.Setenv("DRAPE_REPO", "")
	return server
}

// uploadLintAs runs `drape upload lint` with the given extra flags and returns the exit code
// and everything it printed.
func uploadLintAs(t *testing.T, server *httptest.Server, extra ...string) (int, string) {
	t.Helper()
	report := filepath.Join(t.TempDir(), "report.sarif")
	if err := os.WriteFile(report, []byte(`{"runs": []}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var printed bytes.Buffer
	output.Stderr, output.Stdout = &printed, &printed
	t.Cleanup(output.Reset)
	t.Cleanup(func() { flagAPIKey, flagOrg, flagRepo, flagVerbose = "", "", "", false })
	flagAPIKey, flagOrg, flagRepo = "", "", ""
	rootCmd.SetArgs(append([]string{"upload", "lint", report, "--api-url", server.URL,
		"--branch", "main", "--sha", "abc123", "--wait=false"}, extra...))
	return Execute(), printed.String()
}

func TestAGitHubActionsJobUploadsWithItsTokenAndNoConfiguredRepository(t *testing.T) {
	var bearers []string
	server := githubActionsServer(t, &bearers)

	code, printed := uploadLintAs(t, server, "--verbose")

	if code != exitcode.Success {
		t.Fatalf("exit %d: %s", code, printed)
	}
	if !strings.Contains(printed, "Authenticated as GitHub Actions for acme/web") {
		t.Errorf("no credential line in:\n%s", printed)
	}
	for _, bearer := range bearers {
		if bearer == "" || bearer == "runner-secret" || !strings.HasPrefix(bearer, "eyJ") {
			t.Fatalf("Drape got bearer %q", bearer)
		}
	}
	if strings.Contains(printed, bearers[0]) {
		t.Error("the token was printed")
	}
}

func TestAnAPIKeyWinsOverTheGitHubToken(t *testing.T) {
	var bearers []string
	server := githubActionsServer(t, &bearers)

	key := "drp_org_" + strings.Repeat("0", 8)
	code, printed := uploadLintAs(t, server, "--api-key", key, "--org", "acme", "--repo", "acme/web")

	if code != exitcode.Success {
		t.Fatalf("exit %d: %s", code, printed)
	}
	if !strings.Contains(printed, "Authenticated with an API key") {
		t.Errorf("no credential line in:\n%s", printed)
	}
	for _, bearer := range bearers {
		if bearer != key {
			t.Fatalf("Drape got bearer %q", bearer)
		}
	}
}

func TestARepositoryOtherThanTheTokensIsAUsageError(t *testing.T) {
	var bearers []string
	server := githubActionsServer(t, &bearers)

	code, printed := uploadLintAs(t, server, "--repo", "acme/api")

	if code != exitcode.UsageError {
		t.Fatalf("exit %d: %s", code, printed)
	}
	if !strings.Contains(printed, "`repo` is acme/api, but this workflow's GitHub token is for acme/web") {
		t.Errorf("unexpected message:\n%s", printed)
	}
}
