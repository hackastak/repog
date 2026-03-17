// Package github provides HTTP client functionality for the GitHub API.
package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	originalDefaultBaseURL = "https://api.github.com"
	userAgent              = "repog-cli/2.0"
)

// defaultBaseURL can be overridden for testing.
var defaultBaseURL = originalDefaultBaseURL

// SetDefaultBaseURL sets the default base URL for new clients (for testing).
func SetDefaultBaseURL(url string) {
	defaultBaseURL = url
}

// ResetDefaultBaseURL resets the default base URL to the original value.
func ResetDefaultBaseURL() {
	defaultBaseURL = originalDefaultBaseURL
}

// Client is an HTTP client for the GitHub API.
type Client struct {
	httpClient *http.Client
	pat        string
	baseURL    string
}

// NewClient creates a new GitHub API client with the given PAT.
func NewClient(pat string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		pat:        pat,
		baseURL:    defaultBaseURL,
	}
}

// RateLimitError is returned when the GitHub API rate limit is exceeded
// and the retry also failed.
type RateLimitError struct {
	ResetAt time.Time
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("GitHub API rate limit exceeded, resets at %s", e.ResetAt.Format(time.RFC3339))
}

// Do executes an HTTP request with the GitHub PAT auth header, User-Agent,
// and Accept headers set. On 429 or 403 with x-ratelimit-remaining=0,
// waits until x-ratelimit-reset and retries once. Returns RateLimitError
// if the retry also fails.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	// Set required headers
	req.Header.Set("Authorization", "Bearer "+c.pat)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	// Check for rate limit
	if resp.StatusCode == 429 || resp.StatusCode == 403 {
		remaining := resp.Header.Get("x-ratelimit-remaining")
		if remaining == "0" {
			resetStr := resp.Header.Get("x-ratelimit-reset")
			if resetStr != "" {
				resetUnix, _ := strconv.ParseInt(resetStr, 10, 64)
				resetAt := time.Unix(resetUnix, 0)

				// Wait until reset
				waitDuration := time.Until(resetAt)
				if waitDuration > 0 {
					// Read and discard body before closing
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()

					time.Sleep(waitDuration + time.Second) // Add 1 second buffer

					// Retry once
					resp, err = c.httpClient.Do(req)
					if err != nil {
						return nil, err
					}

					// If still rate limited, return error
					if resp.StatusCode == 429 || resp.StatusCode == 403 {
						_, _ = io.Copy(io.Discard, resp.Body)
						_ = resp.Body.Close()
						return nil, &RateLimitError{ResetAt: resetAt}
					}
				}
			}
		}
	}

	return resp, nil
}

// Get is a convenience wrapper around Do for GET requests.
func (c *Client) Get(ctx context.Context, path string) (*http.Response, error) {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}
