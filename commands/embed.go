package commands

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/hackastak/repog/internal/config"
	"github.com/hackastak/repog/internal/db"
	"github.com/hackastak/repog/internal/embed"
)

var embedCmd = &cobra.Command{
	Use:   "embed",
	Short: "Generate embeddings for synced repositories",
	Long:  "Generate vector embeddings for repository chunks using Google Gemini.",
	RunE:  runEmbed,
}

var (
	embedIncludeFileTree bool
	embedVerbose         bool
	embedBatchSize       int
)

func init() {
	embedCmd.Flags().BoolVar(&embedIncludeFileTree, "include-file-tree", true, "Include file_tree chunks in embeddings (default true)")
	embedCmd.Flags().BoolVar(&embedVerbose, "verbose", false, "Show detailed progress")
	embedCmd.Flags().IntVar(&embedBatchSize, "batch-size", 20, "Embedding batch size (max 100)")
}

func runEmbed(cmd *cobra.Command, args []string) error {
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()

	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, red("Run `repog init` first."))
		os.Exit(1)
	}

	geminiKey, err := config.GetGeminiAPIKey()
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

	// Start embedding pipeline
	var s *spinner.Spinner
	if !embedVerbose {
		s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = " Generating embeddings..."
		s.Start()
	}

	eventCh := embed.RunEmbedPipeline(context.Background(), embed.EmbedOptions{
		IncludeFileTree: embedIncludeFileTree,
		BatchSize:       embedBatchSize,
		DB:              database,
		GeminiAPIKey:    geminiKey,
	})

	for event := range eventCh {
		switch event.Type {
		case "batch":
			if embedVerbose {
				fmt.Printf("Batch %d/%d: %d embedded, %d skipped, %d errors\n",
					event.BatchIndex, event.BatchTotal, event.ChunksEmbedded, event.ChunksSkipped, event.ChunksErrored)
				// Show first unique error from this batch
				if len(event.Errors) > 0 {
					fmt.Println(red("  Error:"), event.Errors[0])
				}
			} else if s != nil {
				progress := float64(event.BatchIndex) / float64(event.BatchTotal) * 100
				s.Suffix = fmt.Sprintf(" Generating embeddings... %.0f%% (%d/%d chunks)",
					progress, event.ChunksEmbedded, event.TotalChunks)
			}

		case "repo_skip":
			if embedVerbose {
				fmt.Println(yellow("~"), "skipped", event.RepoFullName)
			}

		case "error":
			if embedVerbose {
				fmt.Println(red("✗"), "error:", event.RepoFullName)
			}

		case "done":
			if s != nil {
				s.Stop()
			}

			if event.ChunksErrored > 0 {
				fmt.Println(yellow("✓"), fmt.Sprintf("Embedding complete — %d embedded, %d skipped, %d errors",
					event.ChunksEmbedded, event.ChunksSkipped, event.ChunksErrored))
			} else {
				fmt.Println(green("✓"), fmt.Sprintf("Embedding complete — %d embedded, %d skipped, %d errors",
					event.ChunksEmbedded, event.ChunksSkipped, event.ChunksErrored))
			}
		}
	}

	return nil
}
