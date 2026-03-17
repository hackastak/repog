package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewClient_SetsUserAgent(t *testing.T) {
	var capturedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"login":"hackastak"}`))
	}))
	defer server.Close()

	SetDefaultBaseURL(server.URL)
	t.Cleanup(ResetDefaultBaseURL)

	client := NewClient("ghp_testtoken")
	_, err := client.Get(context.Background(), "/user")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert: User-Agent header == "repog-cli/2.0"
	if got := capturedHeaders.Get("User-Agent"); got != "repog-cli/2.0" {
		t.Errorf("User-Agent = %q, want %q", got, "repog-cli/2.0")
	}

	// Assert: Authorization header == "Bearer ghp_testtoken"
	if got := capturedHeaders.Get("Authorization"); got != "Bearer ghp_testtoken" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer ghp_testtoken")
	}

	// Assert: Accept header == "application/vnd.github+json"
	if got := capturedHeaders.Get("Accept"); got != "application/vnd.github+json" {
		t.Errorf("Accept = %q, want %q", got, "application/vnd.github+json")
	}
}

func TestNewClient_SetsAPIVersionHeader(t *testing.T) {
	var capturedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"login":"hackastak"}`))
	}))
	defer server.Close()

	SetDefaultBaseURL(server.URL)
	t.Cleanup(ResetDefaultBaseURL)

	client := NewClient("ghp_testtoken")
	_, err := client.Get(context.Background(), "/user")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Assert: X-GitHub-Api-Version header == "2022-11-28"
	if got := capturedHeaders.Get("X-GitHub-Api-Version"); got != "2022-11-28" {
		t.Errorf("X-GitHub-Api-Version = %q, want %q", got, "2022-11-28")
	}
}

func TestRateLimitError_ErrorMethod(t *testing.T) {
	resetTime := time.Now().Add(5 * time.Minute)
	err := &RateLimitError{ResetAt: resetTime}

	errorMsg := err.Error()

	// Assert: returned string is non-empty
	if errorMsg == "" {
		t.Error("RateLimitError.Error() should return non-empty string")
	}

	// Assert: returned string contains some indication of the reset time
	if !strings.Contains(errorMsg, "rate limit") {
		t.Errorf("Error message should mention rate limit, got %q", errorMsg)
	}
}

func TestClientDo_RetriesOn429(t *testing.T) {
	var mu sync.Mutex
	var reqCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		reqCount++
		count := reqCount
		mu.Unlock()

		if count == 1 {
			// First request: return 429 with reset time 2 seconds from now
			// Must be at least 1 second to trigger retry (Unix timestamp is in seconds)
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(2*time.Second).Unix()))
			w.Header().Set("x-ratelimit-remaining", "0")
			w.WriteHeader(429)
			return
		}
		// Second request: return 200
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"login":"hackastak"}`))
	}))
	defer server.Close()

	client := &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	resp, err := client.Get(context.Background(), "/user")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Assert: eventual response is 200
	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Assert: server received exactly 2 requests
	mu.Lock()
	finalCount := reqCount
	mu.Unlock()
	if finalCount != 2 {
		t.Errorf("Expected 2 requests, got %d", finalCount)
	}
}

func TestClientDo_ReturnsRateLimitErrorIfRetryAlsoFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return 429 with reset time 2 seconds from now
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(2*time.Second).Unix()))
		w.Header().Set("x-ratelimit-remaining", "0")
		w.WriteHeader(429)
	}))
	defer server.Close()

	client := &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	_, err := client.Get(context.Background(), "/user")

	// Assert: returned error is *RateLimitError
	if err == nil {
		t.Fatal("Expected error for persistent rate limit")
	}

	_, ok := err.(*RateLimitError)
	if !ok {
		t.Errorf("Expected *RateLimitError, got %T: %v", err, err)
	}
}

func TestClientDo_RetriesOn403WithRateLimitHeader(t *testing.T) {
	var mu sync.Mutex
	var reqCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		reqCount++
		count := reqCount
		mu.Unlock()

		if count == 1 {
			// First request: return 403 with rate limit headers
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(2*time.Second).Unix()))
			w.Header().Set("x-ratelimit-remaining", "0")
			w.WriteHeader(403)
			return
		}
		// Second request: return 200
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"login":"hackastak"}`))
	}))
	defer server.Close()

	client := &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	resp, err := client.Get(context.Background(), "/user")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Assert: eventual response is 200
	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestSetDefaultBaseURL_And_ResetDefaultBaseURL(t *testing.T) {
	// Store original
	original := defaultBaseURL

	t.Cleanup(ResetDefaultBaseURL)

	// Set custom URL
	customURL := "http://custom-url.example.com"
	SetDefaultBaseURL(customURL)

	// Create client and verify it uses custom URL
	client := NewClient("test-pat")
	if client.baseURL != customURL {
		t.Errorf("Client baseURL = %q, want %q", client.baseURL, customURL)
	}

	// Reset to default
	ResetDefaultBaseURL()

	// Verify it's restored to original
	client2 := NewClient("test-pat")
	if client2.baseURL != original {
		t.Errorf("Client baseURL after reset = %q, want %q", client2.baseURL, original)
	}
}

func TestValidatePAT_ValidToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "repo, read:user")
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"login": "hackastak",
			"id":    12345,
		})
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	result := ValidatePAT(context.Background(), client)

	// Assert: result.Valid == true
	if !result.Valid {
		t.Errorf("ValidatePAT should return Valid=true, got false with error: %s", result.Error)
	}

	// Assert: result.Login == "hackastak"
	if result.Login != "hackastak" {
		t.Errorf("ValidatePAT Login = %q, want %q", result.Login, "hackastak")
	}

	// Assert: result.Scopes contains "repo"
	hasRepo := false
	for _, scope := range result.Scopes {
		if scope == "repo" {
			hasRepo = true
			break
		}
	}
	if !hasRepo {
		t.Errorf("ValidatePAT Scopes should contain 'repo', got %v", result.Scopes)
	}
}

func TestValidatePAT_InvalidToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"message": "Bad credentials"}`))
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "invalid-pat",
		baseURL:    server.URL,
	}

	result := ValidatePAT(context.Background(), client)

	// Assert: result.Valid == false
	if result.Valid {
		t.Error("ValidatePAT should return Valid=false for invalid token")
	}
}

func TestValidatePAT_NetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	// Close server immediately to simulate network error
	server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	result := ValidatePAT(context.Background(), client)

	// Assert: result.Valid == false
	if result.Valid {
		t.Error("ValidatePAT should return Valid=false for network error")
	}
}

func TestGetRateLimitInfo_ReturnsCorrectValues(t *testing.T) {
	resetTime := time.Now().Add(1 * time.Hour)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"resources": map[string]interface{}{
				"core": map[string]interface{}{
					"limit":     5000,
					"remaining": 4832,
					"reset":     resetTime.Unix(),
				},
			},
		})
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	result := GetRateLimitInfo(context.Background(), client)

	// Assert: result is not nil
	if result == nil {
		t.Fatal("GetRateLimitInfo should not return nil")
	}

	// Assert: result.Limit == 5000
	if result.Limit != 5000 {
		t.Errorf("RateLimitInfo.Limit = %d, want %d", result.Limit, 5000)
	}

	// Assert: result.Remaining == 4832
	if result.Remaining != 4832 {
		t.Errorf("RateLimitInfo.Remaining = %d, want %d", result.Remaining, 4832)
	}

	// Assert: result.ResetAt is approximately 1 hour from now
	parsedReset, err := time.Parse(time.RFC3339, result.ResetAt)
	if err != nil {
		t.Errorf("Failed to parse ResetAt: %v", err)
	} else {
		diff := parsedReset.Sub(resetTime)
		if diff < -time.Second || diff > time.Second {
			t.Errorf("ResetAt time differs from expected by %v", diff)
		}
	}
}

func TestGetRateLimitInfo_ReturnsNilOnAPIFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	result := GetRateLimitInfo(context.Background(), client)

	// Assert: returns nil on error
	if result != nil {
		t.Errorf("GetRateLimitInfo should return nil on API failure, got %+v", result)
	}
}

func TestClient_DoSetsAllRequiredHeaders(t *testing.T) {
	var capturedReq *http.Request

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read and discard body to avoid issues
		_, _ = io.ReadAll(r.Body)
		capturedReq = r.Clone(context.Background())
		w.WriteHeader(200)
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "test-token-123",
		baseURL:    server.URL,
	}

	req, _ := http.NewRequest("GET", server.URL+"/test", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Verify all required headers were set
	expectedHeaders := map[string]string{
		"Authorization":        "Bearer test-token-123",
		"Accept":               "application/vnd.github+json",
		"X-GitHub-Api-Version": "2022-11-28",
		"User-Agent":           "repog-cli/2.0",
	}

	for key, expected := range expectedHeaders {
		if got := capturedReq.Header.Get(key); got != expected {
			t.Errorf("Header %s = %q, want %q", key, got, expected)
		}
	}
}
