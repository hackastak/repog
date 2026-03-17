// Package commands provides CLI command implementations for repog.
package commands

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "repog",
	Short: "AI-powered knowledge base for your GitHub repositories",
	Version: "0.1.0",
	Long: `RepoG is an AI-powered CLI tool that lets developers build a searchable
knowledge base from their GitHub repositories. It ingests repo metadata,
READMEs, and file trees; generates vector embeddings via Google Gemini;
and supports natural language search, Q&A, recommendations, and summarization.`,
}


// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(embedCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(recommendCmd)
	rootCmd.AddCommand(askCmd)
	rootCmd.AddCommand(summarizeCmd)
	rootCmd.AddCommand(statusCmd)
}
