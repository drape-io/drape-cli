// Package oidc gets short-lived OIDC tokens from the CI provider, so uploads need no API key.
package oidc

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// refreshBefore is how long before a token expires it is replaced. A request's retries reuse its
// token for up to about 90 seconds; Drape's 60 seconds of leeway on expiry covers the rest.
const refreshBefore = 60 * time.Second

// maxAttempts is how many times a token is requested before giving up, waiting 1s, then 2s.
const maxAttempts = 3

// retryable marks a failure worth another attempt: a network error, a 5xx or a 429.
type retryable struct{ error }

func (r retryable) Unwrap() error { return r.error }

// GitHubActions fetches GitHub Actions OIDC tokens for one audience, caching each until shortly
// before it expires. A job gets the env vars it needs only with `permissions: id-token: write`.
type GitHubActions struct {
	RequestURL   string
	RequestToken string
	Audience     string
	HTTPClient   *http.Client
	Now          func() time.Time
	Sleep        func(time.Duration)

	mu      sync.Mutex
	token   string
	expires time.Time
}

// GitHubActionsFromEnv returns a token source when this is a GitHub Actions job allowed to mint
// OIDC tokens, and false otherwise.
func GitHubActionsFromEnv(getenv func(string) string, audience string) (*GitHubActions, bool) {
	requestURL := getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	requestToken := getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	if requestURL == "" || requestToken == "" {
		return nil, false
	}
	return &GitHubActions{
		RequestURL:   requestURL,
		RequestToken: requestToken,
		Audience:     audience,
		HTTPClient:   &http.Client{Timeout: 30 * time.Second},
		Now:          time.Now,
		Sleep:        time.Sleep,
	}, true
}

// Token returns a token valid for at least another minute, fetching a new one when needed.
func (g *GitHubActions) Token() (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.token != "" && g.Now().Add(refreshBefore).Before(g.expires) {
		return g.token, nil
	}
	var (
		token   string
		expires time.Time
		err     error
	)
	for attempt := range maxAttempts {
		if attempt > 0 {
			g.Sleep(time.Second << (attempt - 1))
		}
		token, expires, err = g.fetch()
		var r retryable
		if !errors.As(err, &r) {
			break
		}
	}
	if err != nil {
		return "", err
	}
	g.token, g.expires = token, expires
	return token, nil
}

func (g *GitHubActions) fetch() (string, time.Time, error) {
	// The request URL already carries a query string; add the audience to it.
	u, err := url.Parse(g.RequestURL)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("invalid ACTIONS_ID_TOKEN_REQUEST_URL: %w", err)
	}
	query := u.Query()
	query.Set("audience", g.Audience)
	u.RawQuery = query.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("creating GitHub token request: %w", err)
	}
	req.Header.Set("Authorization", "bearer "+g.RequestToken)
	req.Header.Set("Accept", "application/json")

	resp, err := g.HTTPClient.Do(req)
	if err != nil {
		return "", time.Time{}, retryable{fmt.Errorf("requesting a GitHub Actions token: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		// The body is GitHub's error message, never a token.
		err := fmt.Errorf("GitHub didn't issue a token (HTTP %d): %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
			return "", time.Time{}, retryable{err}
		}
		return "", time.Time{}, err
	}
	if err != nil {
		return "", time.Time{}, retryable{fmt.Errorf("reading GitHub's token response: %w", err)}
	}

	var issued struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &issued); err != nil || issued.Value == "" {
		return "", time.Time{}, errors.New("GitHub's token response had no token")
	}
	expires, err := expiry(issued.Value)
	if err != nil {
		return "", time.Time{}, err
	}
	return issued.Value, expires, nil
}

// expiry reads a JWT's `exp` claim. The signature isn't checked: Drape does that; this only
// decides when to fetch a fresh token.
func expiry(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, errors.New("GitHub's token isn't a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, errors.New("GitHub's token payload isn't base64url")
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}, errors.New("GitHub's token has no expiry")
	}
	return time.Unix(claims.Exp, 0), nil
}
