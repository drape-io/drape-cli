package oidc

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// GitHubProvider fetches OIDC tokens from GitHub Actions.
//
// Requires the workflow to have `permissions: id-token: write` so that
// ACTIONS_ID_TOKEN_REQUEST_URL and ACTIONS_ID_TOKEN_REQUEST_TOKEN are set.
type GitHubProvider struct {
	requestURL   string
	requestToken string
	httpClient   *http.Client
}

// NewGitHubProvider creates a GitHub Actions OIDC provider from env vars.
func NewGitHubProvider(env EnvFunc) *GitHubProvider {
	return &GitHubProvider{
		requestURL:   env("ACTIONS_ID_TOKEN_REQUEST_URL"),
		requestToken: env("ACTIONS_ID_TOKEN_REQUEST_TOKEN"),
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (g *GitHubProvider) Available() bool {
	return g.requestURL != "" && g.requestToken != ""
}

func (g *GitHubProvider) Name() string {
	return "GitHub Actions"
}

func (g *GitHubProvider) FetchToken(audience string) (string, error) {
	url := g.requestURL + "&audience=" + audience

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("creating OIDC token request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.requestToken)
	req.Header.Set("Accept", "application/json; api-version=2.0")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching OIDC token from GitHub: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading OIDC token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub OIDC token request failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decoding OIDC token response: %w", err)
	}
	if result.Value == "" {
		return "", fmt.Errorf("GitHub OIDC response contained empty token")
	}
	return result.Value, nil
}
