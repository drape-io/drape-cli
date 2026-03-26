// Package api provides an HTTP client for the Drape API.
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/drape-io/drape-cli/internal/output"
)

// seekableReadCloser wraps a bytes.Reader so it satisfies io.ReadCloser and io.Seeker,
// allowing doWithRetries to reset the body on retry.
type seekableReadCloser struct {
	*bytes.Reader
}

func (s *seekableReadCloser) Close() error { return nil }

// Client communicates with the Drape API.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
	UserAgent  string
}

// NewClient creates a new API client. Returns an error if baseURL is not a valid URL.
func NewClient(baseURL, token string) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL %q: %w", baseURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("invalid base URL scheme %q: must be http or https", u.Scheme)
	}
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		UserAgent: "drape-cli/dev",
	}, nil
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
	defer func() { _ = resp.Body.Close() }()

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
	const maxRetries = 3
	var lastErr error
	for attempt := range maxRetries {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s for retries 1 and 2
			delay := time.Second << (attempt - 1)
			output.Verbose("Retrying request after %v (attempt %d/%d)", delay, attempt+1, maxRetries)
			time.Sleep(delay)

			// Reset body for retries if body supports seeking (e.g. bytes.Reader)
			if req.Body != nil {
				if seeker, ok := req.Body.(io.Seeker); ok {
					if _, err := seeker.Seek(0, io.SeekStart); err != nil {
						return nil, fmt.Errorf("resetting request body for retry: %w", err)
					}
				}
			}
		}

		resp, err := c.HTTPClient.Do(req) //nolint:gosec // G704: BaseURL is validated in NewClient
		if err != nil {
			lastErr = err
			continue
		}

		// Only retry on 5xx errors
		if resp.StatusCode >= 500 {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("server error %d: %s", resp.StatusCode, string(body))
			continue
		}

		return resp, nil
	}
	return nil, fmt.Errorf("request failed after %d attempts: %w", maxRetries, lastErr)
}

// ErrorResponse represents an API error.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Detail  string `json:"detail"`
}

// postJSON sends a POST request with a JSON body and decodes the response.
// If reqBody is nil, no body is sent. If result is nil, the response body is discarded.
func (c *Client) postJSON(url string, reqBody any, result any) error {
	var body io.Reader
	if reqBody != nil {
		encoded, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshaling request: %w", err)
		}
		sr := &seekableReadCloser{bytes.NewReader(encoded)}
		body = sr
	}

	httpReq, err := http.NewRequest("POST", url, body)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	// We wrap the body in seekableReadCloser so doWithRetries can Seek(0) on
	// retry. http.NewRequest only auto-detects ContentLength for *bytes.Reader
	// and *bytes.Buffer; our wrapper hides that type, so we must set both
	// Body and ContentLength explicitly. Without ContentLength the server may
	// reject the request (e.g. Django returns 422 "Field required" for a
	// chunked-encoded JSON body).
	if sr, ok := body.(*seekableReadCloser); ok {
		httpReq.Body = sr
		httpReq.ContentLength = int64(sr.Len())
		httpReq.Header.Set("Content-Type", "application/json")
	}
	c.setHeaders(httpReq)

	resp, err := c.doWithRetries(httpReq)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseErrorResponse(resp)
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}

// getJSON sends a GET request and decodes the JSON response.
func (c *Client) getJSON(url string, result any) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.doWithRetries(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseErrorResponse(resp)
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
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
