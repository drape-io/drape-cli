package oidc

import (
	"fmt"
	"os"
	"strings"
)

// GitLabProvider reads OIDC tokens from GitLab CI.
//
// GitLab's modern id_tokens feature requires users to define the token variable
// name in .gitlab-ci.yml. Users should set DRAPE_OIDC_TOKEN to that variable's
// value, or point DRAPE_OIDC_TOKEN_FILE at a file containing the JWT.
//
// Example .gitlab-ci.yml:
//
//	upload:
//	  id_tokens:
//	    DRAPE_OIDC_TOKEN:
//	      aud: https://app.drape.io/oidc/my-org
//	  script:
//	    - drape upload coverage coverage.xml --format cobertura
//
// Note: The audience is configured in .gitlab-ci.yml at token creation time,
// not at fetch time. The audience parameter to FetchToken is not used — the
// token already has its audience baked in. Ensure the audience in your CI config
// matches the trust policy configured in Drape (typically {api-url}/oidc/{org}).
type GitLabProvider struct {
	token     string
	tokenFile string
}

// NewGitLabProvider creates a GitLab CI OIDC provider from env vars.
// Checks DRAPE_OIDC_TOKEN (inline value) first, then DRAPE_OIDC_TOKEN_FILE (path).
func NewGitLabProvider(env EnvFunc) *GitLabProvider {
	return &GitLabProvider{
		token:     env("DRAPE_OIDC_TOKEN"),
		tokenFile: env("DRAPE_OIDC_TOKEN_FILE"),
	}
}

func (g *GitLabProvider) Available() bool {
	return g.token != "" || g.tokenFile != ""
}

func (g *GitLabProvider) Name() string {
	return "GitLab CI"
}

// FetchToken returns the OIDC JWT. The audience parameter is ignored because
// GitLab bakes the audience into the token at creation time (configured in
// .gitlab-ci.yml id_tokens). See the type-level doc comment for details.
func (g *GitLabProvider) FetchToken(_ string) (string, error) {
	// Prefer inline token over file-based token.
	if g.token != "" {
		return strings.TrimSpace(g.token), nil
	}

	data, err := os.ReadFile(g.tokenFile) //nolint:gosec // G304: path is from env var set by CI runner
	if err != nil {
		return "", fmt.Errorf("reading OIDC token file %s: %w", g.tokenFile, err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("OIDC token file %s is empty", g.tokenFile)
	}
	return token, nil
}
