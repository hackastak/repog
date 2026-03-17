package github

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"
)

// PATValidationResult contains the result of validating a GitHub PAT.
type PATValidationResult struct {
	Valid  bool
	Login  string
	Scopes []string
	Error  string
}

// userResponse represents the API response for /user.
type userResponse struct {
	Login string `json:"login"`
	ID    int64  `json:"id"`
	Name  string `json:"name"`
}

// ValidatePAT calls GET /user and checks the x-oauth-scopes header.
// Returns a result even on failure — never returns an error directly.
func ValidatePAT(ctx context.Context, client *Client) PATValidationResult {
	resp, err := client.Get(ctx, "/user")
	if err != nil {
		return PATValidationResult{
			Valid: false,
			Error: err.Error(),
		}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return PATValidationResult{
			Valid: false,
			Error: string(body),
		}
	}

	// Read user info
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return PATValidationResult{
			Valid: false,
			Error: err.Error(),
		}
	}

	var user userResponse
	if err := json.Unmarshal(body, &user); err != nil {
		return PATValidationResult{
			Valid: false,
			Error: err.Error(),
		}
	}

	// Parse scopes from header
	scopeHeader := resp.Header.Get("x-oauth-scopes")
	var scopes []string
	if scopeHeader != "" {
		for _, s := range strings.Split(scopeHeader, ",") {
			scopes = append(scopes, strings.TrimSpace(s))
		}
	}

	return PATValidationResult{
		Valid:  true,
		Login:  user.Login,
		Scopes: scopes,
	}
}

// RateLimitInfo contains GitHub API rate limit information.
type RateLimitInfo struct {
	Limit     int
	Remaining int
	ResetAt   string // ISO datetime string
	Available bool
}

// rateLimitResponse represents the API response for /rate_limit.
type rateLimitResponse struct {
	Resources struct {
		Core struct {
			Limit     int   `json:"limit"`
			Remaining int   `json:"remaining"`
			Reset     int64 `json:"reset"`
		} `json:"core"`
	} `json:"resources"`
}

// GetRateLimitInfo fetches the current rate limit status.
// Returns nil on any error.
func GetRateLimitInfo(ctx context.Context, client *Client) *RateLimitInfo {
	resp, err := client.Get(ctx, "/rate_limit")
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var rateLimit rateLimitResponse
	if err := json.Unmarshal(body, &rateLimit); err != nil {
		return nil
	}

	core := rateLimit.Resources.Core
	return &RateLimitInfo{
		Limit:     core.Limit,
		Remaining: core.Remaining,
		ResetAt:   time.Unix(core.Reset, 0).Format(time.RFC3339),
		Available: core.Remaining > 0,
	}
}
