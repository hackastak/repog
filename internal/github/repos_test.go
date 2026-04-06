package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchOwnedReposSinglePage(t *testing.T) {
	repos := []Repo{
		{ID: 1, FullName: "user/repo1", Name: "repo1", Owner: Owner{Login: "user"}},
		{ID: 2, FullName: "user/repo2", Name: "repo2", Owner: Owner{Login: "user"}},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(repos)
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	repoCh, errCh := FetchOwnedRepos(context.Background(), client)

	var fetched []Repo
	for repo := range repoCh {
		fetched = append(fetched, repo)
	}

	if err := <-errCh; err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(fetched) != 2 {
		t.Errorf("Expected 2 repos, got %d", len(fetched))
	}
}

func TestFetchOwnedReposPagination(t *testing.T) {
	page1 := make([]Repo, 100)
	for i := 0; i < 100; i++ {
		page1[i] = Repo{ID: int64(i), FullName: "user/repo" + string(rune('a'+i%26))}
	}
	page2 := []Repo{
		{ID: 100, FullName: "user/repo-final"},
	}

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			_ = json.NewEncoder(w).Encode(page1)
		} else {
			_ = json.NewEncoder(w).Encode(page2)
		}
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	repoCh, errCh := FetchOwnedRepos(context.Background(), client)

	count := 0
	for range repoCh {
		count++
	}

	if err := <-errCh; err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if count != 101 {
		t.Errorf("Expected 101 repos, got %d", count)
	}

	if requestCount != 2 {
		t.Errorf("Expected 2 requests for pagination, got %d", requestCount)
	}
}

func TestFetchStarredReposSinglePage(t *testing.T) {
	repos := []Repo{
		{ID: 1, FullName: "other/starred1", Name: "starred1"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "starred") {
			t.Errorf("Expected starred endpoint, got %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(repos)
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	repoCh, errCh := FetchStarredRepos(context.Background(), client)

	var fetched []Repo
	for repo := range repoCh {
		fetched = append(fetched, repo)
	}

	if err := <-errCh; err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(fetched) != 1 {
		t.Errorf("Expected 1 repo, got %d", len(fetched))
	}
}

func TestFetchReadmeReturnsDecodedContent(t *testing.T) {
	content := "# Hello World\n\nThis is a README."
	encoded := base64.StdEncoding.EncodeToString([]byte(content))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"content":  encoded,
			"encoding": "base64",
		})
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	readme := FetchReadme(context.Background(), client, "user", "repo")

	if readme != strings.TrimRight(content, "\n\r") {
		t.Errorf("Expected decoded content, got %q", readme)
	}
}

func TestFetchReadmeReturnsEmptyOnNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	readme := FetchReadme(context.Background(), client, "user", "repo")

	if readme != "" {
		t.Errorf("Expected empty string for 404, got %q", readme)
	}
}

func TestFetchFileTreeReturnsOnlyBlobsAtDepthLessThanTwo(t *testing.T) {
	tree := treeResponse{
		Tree: []treeEntry{
			{Path: "README.md", Type: "blob"},         // depth 0 - include
			{Path: "src", Type: "tree"},               // directory - exclude
			{Path: "src/index.ts", Type: "blob"},      // depth 1 - include
			{Path: "src/lib/helper.ts", Type: "blob"}, // depth 2 - exclude
			{Path: "package.json", Type: "blob"},      // depth 0 - include
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(tree)
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	result := FetchFileTree(context.Background(), client, "user", "repo", "main")

	lines := strings.Split(result, "\n")
	if len(lines) != 3 {
		t.Errorf("Expected 3 files, got %d: %v", len(lines), lines)
	}

	// Verify sorted alphabetically
	expected := []string{"README.md", "package.json", "src/index.ts"}
	for i, path := range expected {
		if i < len(lines) && lines[i] != path {
			t.Errorf("Expected %s at position %d, got %s", path, i, lines[i])
		}
	}
}

func TestFetchFileTreeReturnsEmptyOnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	result := FetchFileTree(context.Background(), client, "user", "repo", "main")

	if result != "" {
		t.Errorf("Expected empty string on error, got %q", result)
	}
}

func TestFetchOwnedReposAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	repoCh, errCh := FetchOwnedRepos(context.Background(), client)

	var count int
	for range repoCh {
		count++
	}

	err := <-errCh
	if err == nil {
		t.Error("Expected error for API failure")
	}

	if count != 0 {
		t.Errorf("Expected 0 repos on error, got %d", count)
	}
}

func TestFetchStarredReposAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	repoCh, errCh := FetchStarredRepos(context.Background(), client)

	for range repoCh {
	}

	err := <-errCh
	if err == nil {
		t.Error("Expected error for API failure")
	}
}

func TestFetchReadmeReturnsEmptyOnOtherError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	readme := FetchReadme(context.Background(), client, "user", "repo")

	if readme != "" {
		t.Errorf("Expected empty string on 403, got %q", readme)
	}
}

func TestFetchReadmeReturnsEmptyOnInvalidEncoding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"content":  "not-base64-encoded",
			"encoding": "utf-8", // Not base64
		})
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	readme := FetchReadme(context.Background(), client, "user", "repo")

	if readme != "" {
		t.Errorf("Expected empty string for non-base64 encoding, got %q", readme)
	}
}

func TestFetchReadmeReturnsEmptyOnEmptyContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"content":  "",
			"encoding": "base64",
		})
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	readme := FetchReadme(context.Background(), client, "user", "repo")

	if readme != "" {
		t.Errorf("Expected empty string for empty content, got %q", readme)
	}
}

func TestFetchFileTreeReturnsEmptyOnEmptyTree(t *testing.T) {
	tree := treeResponse{
		Tree: []treeEntry{},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(tree)
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	result := FetchFileTree(context.Background(), client, "user", "repo", "main")

	if result != "" {
		t.Errorf("Expected empty string for empty tree, got %q", result)
	}
}

func TestFetchStarredReposPagination(t *testing.T) {
	page1 := make([]Repo, 100)
	for i := 0; i < 100; i++ {
		page1[i] = Repo{ID: int64(i), FullName: "other/starred" + string(rune('a'+i%26))}
	}
	page2 := []Repo{
		{ID: 100, FullName: "other/starred-final"},
	}

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			_ = json.NewEncoder(w).Encode(page1)
		} else {
			_ = json.NewEncoder(w).Encode(page2)
		}
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	repoCh, errCh := FetchStarredRepos(context.Background(), client)

	count := 0
	for range repoCh {
		count++
	}

	if err := <-errCh; err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if count != 101 {
		t.Errorf("Expected 101 repos, got %d", count)
	}
}

func TestFetchOwnedReposEmptyPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Repo{})
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	repoCh, errCh := FetchOwnedRepos(context.Background(), client)

	var count int
	for range repoCh {
		count++
	}

	if err := <-errCh; err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if count != 0 {
		t.Errorf("Expected 0 repos for empty page, got %d", count)
	}
}

func TestFetchStarredReposEmptyPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Repo{})
	}))
	defer server.Close()

	client := &Client{
		httpClient: server.Client(),
		pat:        "test-pat",
		baseURL:    server.URL,
	}

	repoCh, errCh := FetchStarredRepos(context.Background(), client)

	var count int
	for range repoCh {
		count++
	}

	if err := <-errCh; err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if count != 0 {
		t.Errorf("Expected 0 repos for empty page, got %d", count)
	}
}
