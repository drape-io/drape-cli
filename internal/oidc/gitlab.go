package oidc

import (
	"fmt"
	"os"
)

// GitLabProvider reads OIDC tokens from GitLab CI's file-based mechanism.
//
// GitLab CI sets GITLAB_OIDC_TOKEN_FILE (or the job can use `id_tokens`)
// which contains a JWT. Unlike GitHub, no HTTP call is needed — the token
// is written to a file by the runner.
type GitLabProvider struct {
	tokenFile string
}

// NewGitLabProvider creates a GitLab CI OIDC provider from env vars.
func NewGitLabProvider(env EnvFunc) *GitLabProvider {
	return &GitLabProvider{
		tokenFile: env("GITLAB_OIDC_TOKEN_FILE"),
	}
}

func (g *GitLabProvider) Available() bool {
	return g.tokenFile != ""
}

func (g *GitLabProvider) Name() string {
	return "GitLab CI"
}

func (g *GitLabProvider) FetchToken(audience string) (string, error) {
	data, err := os.ReadFile(g.tokenFile) //nolint:gosec // G304: path is from env var set by GitLab runner
	if err != nil {
		return "", fmt.Errorf("reading GitLab OIDC token file: %w", err)
	}
	token := string(data)
	if token == "" {
		return "", fmt.Errorf("GitLab OIDC token file is empty")
	}
	return token, nil
}
