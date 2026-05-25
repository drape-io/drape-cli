// Package oidc provides OIDC token auto-detection for CI/CD environments.
//
// When running in a supported CI provider (GitHub Actions, GitLab CI, etc.)
// with OIDC enabled, this package fetches a short-lived JWT that the Drape API
// accepts in place of a static API key.
package oidc

import "strings"

// EnvFunc looks up an environment variable by name.
type EnvFunc func(string) string

// Provider can fetch an OIDC token for a given audience.
type Provider interface {
	// Available reports whether this provider's OIDC credentials are present.
	Available() bool
	// Name returns a human-readable provider name (e.g. "GitHub Actions").
	Name() string
	// FetchToken requests a JWT for the given audience.
	FetchToken(audience string) (string, error)
}

// providers returns the ordered list of OIDC providers to try.
func providers(env EnvFunc) []Provider {
	return []Provider{
		NewGitHubProvider(env),
		NewGitLabProvider(env),
	}
}

// DetectAndFetchToken tries each known OIDC provider in order.
// Returns the JWT on success, or ("", nil) if no provider is available.
func DetectAndFetchToken(env EnvFunc, apiURL, orgSlug string) (string, error) {
	audience := strings.TrimRight(apiURL, "/") + "/oidc/" + orgSlug

	for _, p := range providers(env) {
		if p.Available() {
			return p.FetchToken(audience)
		}
	}
	return "", nil
}
