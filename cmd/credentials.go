package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/drape-io/drape-cli/internal/api"
	"github.com/drape-io/drape-cli/internal/cidetect"
	"github.com/drape-io/drape-cli/internal/exitcode"
	"github.com/drape-io/drape-cli/internal/oidc"
)

const defaultAPIURL = "https://app.drape.io"

var (
	errForkPullRequest = errors.New(
		"pull requests from forks get no GitHub token or secrets, so Drape can't accept uploads from them")
	errNoIDTokenPermission = errors.New(
		"add `id-token: write` under `permissions:` in this workflow or job " +
			"(keep your other permissions), or set DRAPE_API_KEY")
)

// newClient creates an API client from global flags, resolving env var defaults. An API key
// wins; without one, a GitHub Actions job allowed to mint OIDC tokens uses those.
func newClient() (*api.Client, error) {
	apiURL := resolveFlag(flagAPIURL, "DRAPE_API_URL")
	if apiURL == "" {
		apiURL = defaultAPIURL
	}

	apiKey := resolveFlag(flagAPIKey, "DRAPE_API_KEY")
	var tokens api.TokenSource
	if apiKey == "" {
		audience, err := audienceFor(apiURL)
		if err != nil {
			return nil, &ExitError{Code: exitcode.UsageError, Err: err}
		}
		source, ok := oidc.GitHubActionsFromEnv(os.Getenv, audience)
		if !ok {
			return nil, &ExitError{Code: exitcode.UsageError, Err: missingCredential(os.Getenv)}
		}
		tokens = source
	}

	client, err := api.NewClient(apiURL, apiKey)
	if err != nil {
		return nil, &ExitError{Code: exitcode.UsageError, Err: err}
	}
	client.Tokens = tokens
	return client, nil
}

// audienceFor is the OIDC audience Drape expects: its origin, without a path or trailing slash.
func audienceFor(apiURL string) (string, error) {
	u, err := url.Parse(apiURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid API URL %q", apiURL)
	}
	return u.Scheme + "://" + u.Host, nil
}

// missingCredential says what to do when there's neither an API key nor a GitHub token to use.
func missingCredential(getenv cidetect.EnvFunc) error {
	if getenv("GITHUB_ACTIONS") != "true" {
		return errMissing("--api-key or DRAPE_API_KEY")
	}
	if isForkPullRequest(getenv) {
		return errForkPullRequest
	}
	return errNoIDTokenPermission
}

// isForkPullRequest reports whether this GitHub Actions run is for a pull request from a fork,
// which GitHub gives neither secrets nor an id-token.
func isForkPullRequest(getenv cidetect.EnvFunc) bool {
	eventName := getenv("GITHUB_EVENT_NAME")
	if !strings.HasPrefix(eventName, "pull_request") || eventName == "pull_request_target" {
		return false
	}
	data, err := os.ReadFile(getenv("GITHUB_EVENT_PATH"))
	if err != nil {
		return false
	}
	type side struct {
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	}
	var event struct {
		PullRequest struct {
			Head side `json:"head"`
			Base side `json:"base"`
		} `json:"pull_request"`
	}
	if json.Unmarshal(data, &event) != nil {
		return false
	}
	// A deleted fork leaves the head repository null.
	head, base := event.PullRequest.Head.Repo.FullName, event.PullRequest.Base.Repo.FullName
	return base != "" && !strings.EqualFold(head, base)
}

// sameRepository reports whether a --repo value names the token's repository: the full
// owner/name or just the name, ignoring case.
func sameRepository(given, tokenRepo string) bool {
	if strings.EqualFold(given, tokenRepo) {
		return true
	}
	_, name, found := strings.Cut(tokenRepo, "/")
	return found && strings.EqualFold(given, name)
}
