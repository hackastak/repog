package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Repo represents a GitHub repository from the API.
type Repo struct {
	ID            int64    `json:"id"`
	NodeID        string   `json:"node_id"`
	Name          string   `json:"name"`
	FullName      string   `json:"full_name"`
	Description   *string  `json:"description"`
	Private       bool     `json:"private"`
	Owner         Owner    `json:"owner"`
	HTMLURL       string   `json:"html_url"`
	CloneURL      string   `json:"clone_url"`
	Language      *string  `json:"language"`
	StargazersCount int    `json:"stargazers_count"`
	ForksCount    int      `json:"forks_count"`
	DefaultBranch string   `json:"default_branch"`
	Topics        []string `json:"topics"`
	PushedAt      string   `json:"pushed_at"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
	Archived      bool     `json:"archived"`
	Fork          bool     `json:"fork"`
	Size          int      `json:"size"`
}

// Owner represents a GitHub repository owner.
type Owner struct {
	Login string `json:"login"`
	ID    int64  `json:"id"`
}

// readmeResponse represents the API response for README requests.
type readmeResponse struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// treeResponse represents the API response for tree requests.
type treeResponse struct {
	Tree []treeEntry `json:"tree"`
}

type treeEntry struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"` // "blob" or "tree"
	SHA  string `json:"sha"`
	Size int    `json:"size,omitempty"`
}

// FetchOwnedRepos returns a channel that yields owned repos one at a time.
// Pagination is handled internally. The channel is closed when all pages
// are exhausted or an error occurs. Errors are sent on the errCh channel.
func FetchOwnedRepos(ctx context.Context, client *Client) (<-chan Repo, <-chan error) {
	repoCh := make(chan Repo, 100)
	errCh := make(chan error, 1)

	go func() {
		defer close(repoCh)
		defer close(errCh)

		page := 1
		for {
			path := fmt.Sprintf("/user/repos?type=owner&sort=pushed&direction=desc&per_page=100&page=%d", page)
			resp, err := client.Get(ctx, path)
			if err != nil {
				errCh <- err
				return
			}

			body, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				errCh <- err
				return
			}

			if resp.StatusCode != 200 {
				errCh <- fmt.Errorf("GitHub API error: %s", resp.Status)
				return
			}

			var repos []Repo
			if err := json.Unmarshal(body, &repos); err != nil {
				errCh <- err
				return
			}

			if len(repos) == 0 {
				return
			}

			for _, repo := range repos {
				select {
				case repoCh <- repo:
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				}
			}

			if len(repos) < 100 {
				return
			}

			page++
		}
	}()

	return repoCh, errCh
}

// FetchStarredRepos returns a channel that yields starred repos one at a time.
func FetchStarredRepos(ctx context.Context, client *Client) (<-chan Repo, <-chan error) {
	repoCh := make(chan Repo, 100)
	errCh := make(chan error, 1)

	go func() {
		defer close(repoCh)
		defer close(errCh)

		page := 1
		for {
			path := fmt.Sprintf("/user/starred?per_page=100&page=%d", page)
			resp, err := client.Get(ctx, path)
			if err != nil {
				errCh <- err
				return
			}

			body, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				errCh <- err
				return
			}

			if resp.StatusCode != 200 {
				errCh <- fmt.Errorf("GitHub API error: %s", resp.Status)
				return
			}

			var repos []Repo
			if err := json.Unmarshal(body, &repos); err != nil {
				errCh <- err
				return
			}

			if len(repos) == 0 {
				return
			}

			for _, repo := range repos {
				select {
				case repoCh <- repo:
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				}
			}

			if len(repos) < 100 {
				return
			}

			page++
		}
	}()

	return repoCh, errCh
}

// FetchReadme fetches and base64-decodes the README for the given repo.
// Returns ("", nil) if the repo has no README (404).
// Never returns an error — logs and returns empty string on any failure.
func FetchReadme(ctx context.Context, client *Client, owner, repo string) string {
	path := fmt.Sprintf("/repos/%s/%s/readme", owner, repo)
	resp, err := client.Get(ctx, path)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 404 {
		return ""
	}

	if resp.StatusCode != 200 {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	var readme readmeResponse
	if err := json.Unmarshal(body, &readme); err != nil {
		return ""
	}

	if readme.Encoding != "base64" || readme.Content == "" {
		return ""
	}

	// Strip newlines before decoding
	content := strings.ReplaceAll(readme.Content, "\n", "")
	decoded, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return ""
	}

	return strings.TrimRight(string(decoded), "\n\r")
}

// FetchFileTree fetches the git tree for the given repo and branch.
// Returns only blob entries at depth <= 2 (paths with at most one '/'),
// sorted alphabetically, joined by newlines.
// Returns "" on any error — never returns an error.
func FetchFileTree(ctx context.Context, client *Client, owner, repo, branch string) string {
	path := fmt.Sprintf("/repos/%s/%s/git/trees/%s?recursive=1", owner, repo, branch)
	resp, err := client.Get(ctx, path)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	var tree treeResponse
	if err := json.Unmarshal(body, &tree); err != nil {
		return ""
	}

	var paths []string
	for _, entry := range tree.Tree {
		// Only include blobs (files)
		if entry.Type != "blob" {
			continue
		}

		// Filter depth <= 2 (at most one '/' in the path)
		slashCount := strings.Count(entry.Path, "/")
		if slashCount > 1 {
			continue
		}

		paths = append(paths, entry.Path)
	}

	if len(paths) == 0 {
		return ""
	}

	// Sort alphabetically
	sort.Strings(paths)

	return strings.Join(paths, "\n")
}
