package commands

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/hackastak/repog/internal/config"
	"github.com/hackastak/repog/internal/db"
	"github.com/hackastak/repog/internal/format"
	"github.com/hackastak/repog/internal/github"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync and embedding status",
	Long:  "Display the current status of your RepoG knowledge base.",
	RunE:  runStatus,
}

var statusJSON bool

func init() {
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output as JSON")
}

// statusResult contains all status information.
type statusResult struct {
	Repos struct {
		Total        int `json:"total"`
		Owned        int `json:"owned"`
		Starred      int `json:"starred"`
		EmbeddedCount int `json:"embeddedCount"`
		PendingEmbed int `json:"pendingEmbed"`
	} `json:"repos"`
	Sync struct {
		LastSyncedAt   *string `json:"lastSyncedAt"`
		LastSyncStatus *string `json:"lastSyncStatus"`
	} `json:"sync"`
	Embed struct {
		LastEmbeddedAt  *string `json:"lastEmbeddedAt"`
		TotalChunks     int     `json:"totalChunks"`
		TotalEmbeddings int     `json:"totalEmbeddings"`
	} `json:"embed"`
	RateLimit *github.RateLimitInfo `json:"rateLimit"`
	DB        struct {
		Path      string `json:"path"`
		SizeBytes int64  `json:"sizeBytes"`
		SizeMB    string `json:"sizeMb"`
	} `json:"db"`
	GeneratedAt string `json:"generatedAt"`
}

func runStatus(cmd *cobra.Command, args []string) error {
	red := color.New(color.FgRed).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()
	bold := color.New(color.Bold).SprintFunc()

	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, red("Run `repog init` first."))
		os.Exit(1)
	}

	// Open database
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, red("Database error:"), err)
		os.Exit(1)
	}
	defer func() { _ = database.Close() }()

	// Show spinner for plain text mode
	var s *spinner.Spinner
	if !statusJSON {
		s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = " Fetching status..."
		s.Start()
	}

	// Collect status in parallel
	result := statusResult{}
	result.GeneratedAt = time.Now().UTC().Format(time.RFC3339)

	var wg sync.WaitGroup
	var mu sync.Mutex

	// Repo stats
	wg.Add(1)
	go func() {
		defer wg.Done()
		var total, owned, starred, embedded, pending int

		row := database.QueryRow(`
			SELECT
				COUNT(*) as total,
				SUM(CASE WHEN is_owned = 1 THEN 1 ELSE 0 END) as owned,
				SUM(CASE WHEN is_starred = 1 THEN 1 ELSE 0 END) as starred,
				SUM(CASE WHEN embedded_hash IS NOT NULL AND embedded_hash = pushed_at_hash THEN 1 ELSE 0 END) as embedded,
				SUM(CASE WHEN embedded_hash IS NULL OR embedded_hash != pushed_at_hash THEN 1 ELSE 0 END) as pending
			FROM repos
		`)
		_ = row.Scan(&total, &owned, &starred, &embedded, &pending)

		mu.Lock()
		result.Repos.Total = total
		result.Repos.Owned = owned
		result.Repos.Starred = starred
		result.Repos.EmbeddedCount = embedded
		result.Repos.PendingEmbed = pending
		mu.Unlock()
	}()

	// Sync state
	wg.Add(1)
	go func() {
		defer wg.Done()
		var status, lastSynced sql.NullString

		// Try sync_state table first
		_ = database.QueryRow(`
			SELECT status, last_synced_at
			FROM sync_state
			ORDER BY last_synced_at DESC
			LIMIT 1
		`).Scan(&status, &lastSynced)

		// Fallback to repos table
		if !lastSynced.Valid {
			_ = database.QueryRow("SELECT MAX(synced_at) FROM repos").Scan(&lastSynced)
			if lastSynced.Valid {
				status.String = "completed"
				status.Valid = true
			}
		}

		mu.Lock()
		if lastSynced.Valid {
			result.Sync.LastSyncedAt = &lastSynced.String
		}
		if status.Valid {
			result.Sync.LastSyncStatus = &status.String
		}
		mu.Unlock()
	}()

	// Chunk counts
	wg.Add(1)
	go func() {
		defer wg.Done()
		var chunks int
		_ = database.QueryRow("SELECT COUNT(*) FROM chunks").Scan(&chunks)

		mu.Lock()
		result.Embed.TotalChunks = chunks
		mu.Unlock()
	}()

	// Embedding counts
	wg.Add(1)
	go func() {
		defer wg.Done()
		var embeddings int
		_ = database.QueryRow("SELECT COUNT(*) FROM chunk_embeddings").Scan(&embeddings)

		mu.Lock()
		result.Embed.TotalEmbeddings = embeddings
		mu.Unlock()
	}()

	// Last embedded
	wg.Add(1)
	go func() {
		defer wg.Done()
		var lastEmbedded sql.NullString
		_ = database.QueryRow("SELECT MAX(embedded_at) FROM repos WHERE embedded_at IS NOT NULL").Scan(&lastEmbedded)

		mu.Lock()
		if lastEmbedded.Valid {
			result.Embed.LastEmbeddedAt = &lastEmbedded.String
		}
		mu.Unlock()
	}()

	// GitHub rate limit
	wg.Add(1)
	go func() {
		defer wg.Done()
		pat, err := config.GetGitHubPAT()
		if err != nil {
			return
		}

		client := github.NewClient(pat)
		rateLimit := github.GetRateLimitInfo(context.Background(), client)

		mu.Lock()
		result.RateLimit = rateLimit
		mu.Unlock()
	}()

	// DB stats
	wg.Add(1)
	go func() {
		defer wg.Done()
		info, err := os.Stat(cfg.DBPath)

		mu.Lock()
		result.DB.Path = cfg.DBPath
		if err == nil {
			result.DB.SizeBytes = info.Size()
			result.DB.SizeMB = fmt.Sprintf("%.2f MB", float64(info.Size())/(1024*1024))
		}
		mu.Unlock()
	}()

	wg.Wait()

	if s != nil {
		s.Stop()
	}

	// Output
	if statusJSON {
		output, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(output))
		return nil
	}

	// Plain text output
	labelWidth := 18
	padLabel := func(label string) string {
		return fmt.Sprintf("%-*s", labelWidth, label)
	}

	fmt.Println(bold("RepoG Status"))
	fmt.Println(dim("─────────────────────────────────────────────"))
	fmt.Println()

	// Repositories
	fmt.Println(bold("  Repositories"))
	fmt.Printf("    %s%15d\n", padLabel("Total:"), result.Repos.Total)
	fmt.Printf("    %s%15d\n", padLabel("Owned:"), result.Repos.Owned)
	fmt.Printf("    %s%15d\n", padLabel("Starred:"), result.Repos.Starred)
	fmt.Printf("    %s%15d\n", padLabel("Embedded:"), result.Repos.EmbeddedCount)
	fmt.Printf("    %s%15d\n", padLabel("Pending embed:"), result.Repos.PendingEmbed)
	fmt.Println()

	// Knowledge Base
	fmt.Println(bold("  Knowledge Base"))
	fmt.Printf("    %s%15d\n", padLabel("Chunks:"), result.Embed.TotalChunks)
	fmt.Printf("    %s%15d\n", padLabel("Embeddings:"), result.Embed.TotalEmbeddings)
	fmt.Println()

	// Last Sync
	fmt.Println(bold("  Last Sync"))
	syncStatus := "Never synced"
	if result.Sync.LastSyncStatus != nil {
		syncStatus = *result.Sync.LastSyncStatus
	}

	statusColor := syncStatus
	switch syncStatus {
	case "completed":
		statusColor = green(syncStatus)
	case "failed":
		statusColor = red(syncStatus)
	case "in_progress":
		statusColor = yellow(syncStatus)
	}
	fmt.Printf("    %s%15s\n", padLabel("Status:"), statusColor)

	if result.Sync.LastSyncedAt != nil {
		fmt.Printf("    %s%15s\n", padLabel("Date:"), format.FormatRelativeTime(*result.Sync.LastSyncedAt))
	}
	fmt.Println()

	// Last Embed
	fmt.Println(bold("  Last Embed"))
	if result.Embed.LastEmbeddedAt != nil {
		fmt.Printf("    %s%15s\n", padLabel("Date:"), format.FormatRelativeTime(*result.Embed.LastEmbeddedAt))
	} else {
		fmt.Printf("    %s%15s\n", padLabel("Date:"), "Never embedded")
	}
	fmt.Println()

	// GitHub API
	fmt.Println(bold("  GitHub API"))
	if result.RateLimit != nil {
		remainingStr := fmt.Sprintf("%d / %d", result.RateLimit.Remaining, result.RateLimit.Limit)
		fmt.Printf("    %s%15s\n", padLabel("Remaining:"), remainingStr)
		fmt.Printf("    %s%15s\n", padLabel("Resets:"), format.FormatRelativeTime(result.RateLimit.ResetAt))
	} else {
		fmt.Printf("    %s%15s\n", padLabel("Status:"), red("unavailable"))
	}
	fmt.Println()

	// Database
	fmt.Println(bold("  Database"))
	// Shorten path if it starts with home directory
	displayPath := result.DB.Path
	if home, err := os.UserHomeDir(); err == nil && len(displayPath) > len(home) {
		if displayPath[:len(home)] == home {
			displayPath = "~" + displayPath[len(home):]
		}
	}
	fmt.Printf("    %s%15s\n", padLabel("Path:"), displayPath)
	fmt.Printf("    %s%15s\n", padLabel("Size:"), result.DB.SizeMB)
	fmt.Println()

	fmt.Println(dim("─────────────────────────────────────────────"))
	timeStr := time.Now().Format("15:04:05")
	fmt.Println(dim("Generated at " + timeStr))

	return nil
}
