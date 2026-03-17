package commands

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/hackastak/repog/internal/config"
	"github.com/hackastak/repog/internal/db"
	"github.com/hackastak/repog/internal/gemini"
	"github.com/hackastak/repog/internal/github"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize RepoG with your GitHub and Gemini API credentials",
	Long:  "Initialize RepoG by providing your GitHub Personal Access Token and Gemini API key.",
	RunE:  runInit,
}

var (
	initGitHubToken string
	initGeminiKey   string
	initDBPath      string
	initForce       bool
)

func init() {
	initCmd.Flags().StringVar(&initGitHubToken, "github-token", "", "GitHub Personal Access Token")
	initCmd.Flags().StringVar(&initGeminiKey, "gemini-key", "", "Google Gemini API Key")
	initCmd.Flags().StringVar(&initDBPath, "db-path", "", "Custom database path")
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "Overwrite existing configuration")
}

func maskSecret(secret string) string {
	if len(secret) <= 8 {
		return "****"
	}
	return secret[:4] + "********" + secret[len(secret)-4:]
}

func runInit(cmd *cobra.Command, args []string) error {
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	bold := color.New(color.Bold).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()

	fmt.Println()
	fmt.Println(bold("RepoG Setup"))
	fmt.Println()

	// Check if already configured
	if config.IsConfigured() && !initForce {
		cfg, _ := config.LoadConfig()
		pat, _ := config.GetGitHubPAT()

		fmt.Println(yellow("RepoG is already configured."))
		if pat != "" {
			fmt.Println("  GitHub:", green("configured"))
		} else {
			fmt.Println("  GitHub:", red("not set"))
		}

		apiKey, _ := config.GetGeminiAPIKey()
		if apiKey != "" {
			fmt.Println("  Gemini:", green("configured"))
		} else {
			fmt.Println("  Gemini:", red("not set"))
		}

		if cfg != nil {
			fmt.Println("  Database:", cfg.DBPath)
		}
		fmt.Println()

		var overwrite bool
		prompt := &survey.Confirm{
			Message: "Do you want to reconfigure?",
			Default: false,
		}
		if err := survey.AskOne(prompt, &overwrite); err != nil {
			fmt.Println(dim("Setup cancelled."))
			return nil
		}

		if !overwrite {
			fmt.Println(dim("Setup cancelled."))
			return nil
		}
	}

	// Get GitHub token
	githubToken := initGitHubToken
	if githubToken == "" {
		fmt.Println(bold("RepoG requires a fine-grained GitHub Personal Access Token."))
		fmt.Println()
		fmt.Println(dim("Create one at: https://github.com/settings/personal-access-tokens/new"))
		fmt.Println()
		fmt.Println(dim("Required settings:"))
		fmt.Println(dim("  Resource owner:     Your GitHub account"))
		fmt.Println(dim("  Repository access:  All repositories (or select specific repos)"))
		fmt.Println(dim("  Permissions:"))
		fmt.Println(dim("    Contents:         Read-only"))
		fmt.Println(dim("    Metadata:         Read-only"))
		fmt.Println()

		prompt := &survey.Password{
			Message: "GitHub Personal Access Token:",
		}
		if err := survey.AskOne(prompt, &githubToken, survey.WithValidator(survey.Required)); err != nil {
			fmt.Println(red("✗"), "Failed to read input:", err)
			os.Exit(1)
		}
	}

	// Validate GitHub token
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Validating GitHub token..."
	s.Start()

	client := github.NewClient(githubToken)
	patResult := github.ValidatePAT(context.Background(), client)
	s.Stop()

	if !patResult.Valid {
		fmt.Println(red("✗"), "GitHub token invalid:", patResult.Error)
		os.Exit(1)
	}

	fmt.Println(green("✓"), "Fine-grained PAT validated - logged in as @"+patResult.Login)

	// Get Gemini API key
	geminiKey := initGeminiKey
	if geminiKey == "" {
		fmt.Println()
		fmt.Println(dim("Get a Gemini API key at: https://aistudio.google.com/apikey"))
		fmt.Println()

		prompt := &survey.Password{
			Message: "Gemini API Key:",
		}
		if err := survey.AskOne(prompt, &geminiKey, survey.WithValidator(survey.Required)); err != nil {
			fmt.Println(red("✗"), "Failed to read input:", err)
			os.Exit(1)
		}
	}

	// Validate Gemini API key
	s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Validating Gemini API key..."
	s.Start()

	geminiResult := gemini.ValidateAPIKey(context.Background(), geminiKey)
	s.Stop()

	if !geminiResult.Valid {
		fmt.Println(red("✗"), "Gemini API key invalid:", geminiResult.Error)
		os.Exit(1)
	}

	fmt.Println(green("✓"), "Gemini API key valid")

	// Get database path
	dbPath := initDBPath
	if dbPath == "" {
		defaultPath := config.DefaultDBPath()

		prompt := &survey.Input{
			Message: "Database path:",
			Default: defaultPath,
		}
		if err := survey.AskOne(prompt, &dbPath); err != nil {
			// User cancelled, use default
			dbPath = defaultPath
		}

		if dbPath == "" {
			dbPath = defaultPath
		}
	}

	// Initialize database
	s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Initializing database..."
	s.Start()

	database, err := db.Open(dbPath)
	if err != nil {
		s.Stop()
		fmt.Println(red("✗"), "Database init failed:", err)
		os.Exit(1)
	}
	_ = database.Close()
	s.Stop()

	fmt.Println(green("✓"), "Database initialized")

	// Save configuration
	s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Saving configuration..."
	s.Start()

	cfg := &config.Config{
		DBPath: dbPath,
	}

	if err := config.SaveConfig(cfg, githubToken, geminiKey); err != nil {
		s.Stop()
		fmt.Println(red("✗"), "Failed to save config:", err)
		os.Exit(1)
	}
	s.Stop()

	fmt.Println(green("✓"), "Configuration saved")

	// Summary
	fmt.Println()
	fmt.Println(bold(green("Setup complete!")))
	fmt.Println()
	fmt.Println("Configuration:")
	fmt.Println("  GitHub:", cyan(maskSecret(githubToken)), "("+patResult.Login+")")
	fmt.Println("  Gemini:", cyan(maskSecret(geminiKey)))
	fmt.Println("  Database:", cyan(dbPath))
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println(" ", cyan("repog sync"), "    - Sync your GitHub repositories")
	fmt.Println(" ", cyan("repog status"), "  - Check sync status")
	fmt.Println()

	return nil
}
