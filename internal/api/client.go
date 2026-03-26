// Package api provides an HTTP client for the Drape API.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/drape-io/drape-cli/internal/output"
)

// Client communicates with the Drape API.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
	UserAgent  string
}

// NewClient creates a new API client.
func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		UserAgent: "drape-cli/dev",
	}
}

// RepoInfo is returned by the repo lookup endpoint.
type RepoInfo struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// LookupRepo resolves a repository name to its ID.
func (c *Client) LookupRepo(orgSlug, repoName string) (*RepoInfo, error) {
	url := fmt.Sprintf("%s/api/v1/orgs/%s/repos/?name=%s", c.BaseURL, orgSlug, repoName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.doWithRetries(req)
	if err != nil {
		return nil, fmt.Errorf("looking up repo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("repository %q not found in org %q", repoName, orgSlug)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseErrorResponse(resp)
	}

	var repos []RepoInfo
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("repository %q not found in org %q", repoName, orgSlug)
	}
	return &repos[0], nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("User-Agent", c.UserAgent)
}

func (c *Client) doWithRetries(req *http.Request) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<uint(attempt-1)) * time.Second
			output.Verbose("Retrying request after %v (attempt %d/3)", delay, attempt+1)
			time.Sleep(delay)
		}

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		// Only retry on 5xx errors
		if resp.StatusCode >= 500 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("server error %d: %s", resp.StatusCode, string(body))
			continue
		}

		return resp, nil
	}
	return nil, fmt.Errorf("request failed after 3 attempts: %w", lastErr)
}

// ErrorResponse represents an API error.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Detail  string `json:"detail"`
}

func parseErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	var errResp ErrorResponse
	if json.Unmarshal(body, &errResp) == nil {
		msg := errResp.Error
		if msg == "" {
			msg = errResp.Message
		}
		if msg == "" {
			msg = errResp.Detail
		}
		if msg != "" {
			return fmt.Errorf("API error %d: %s", resp.StatusCode, msg)
		}
	}
	return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
}
