package commands

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/hackastak/repog/internal/config"
	"github.com/hackastak/repog/internal/db"
	"github.com/hackastak/repog/internal/provider"
	_ "github.com/hackastak/repog/internal/provider/anthropic"
	_ "github.com/hackastak/repog/internal/provider/gemini"
	_ "github.com/hackastak/repog/internal/provider/ollama"
	_ "github.com/hackastak/repog/internal/provider/openai"
	_ "github.com/hackastak/repog/internal/provider/openrouter"
	_ "github.com/hackastak/repog/internal/provider/voyageai"
	"github.com/hackastak/repog/internal/sync"
)

var reconfigCmd = &cobra.Command{
	Use:   "reconfig [embedding|generation]",
	Short: "Reconfigure RepoG settings",
	Long:  "Reconfigure embedding provider, generation provider, or both. Use without arguments to reconfigure all settings.",
	RunE:  runReconfig,
}

var (
	reconfigProvider   string
	reconfigModel      string
	reconfigDimensions int
	reconfigBaseURL    string
	reconfigFallback   string
	reconfigAPIKey     string
)

func init() {
	reconfigCmd.Flags().StringVar(&reconfigProvider, "provider", "", "Provider name (gemini, openai, openrouter, voyageai, anthropic, or ollama)")
	reconfigCmd.Flags().StringVar(&reconfigModel, "model", "", "Model name")
	reconfigCmd.Flags().IntVar(&reconfigDimensions, "dimensions", 0, "Embedding dimensions (embedding only)")
	reconfigCmd.Flags().StringVar(&reconfigBaseURL, "base-url", "", "Custom base URL (ollama only)")
	reconfigCmd.Flags().StringVar(&reconfigFallback, "fallback", "", "Fallback model (generation only)")
	reconfigCmd.Flags().StringVar(&reconfigAPIKey, "api-key", "", "API key for the provider")
}

func runReconfig(cmd *cobra.Command, args []string) error {
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	bold := color.New(color.Bold).SprintFunc()
	dim := color.New(color.Faint).SprintFunc()

	// Load current config
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, red("Run `repog init` first."))
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println(bold("RepoG Reconfiguration"))
	fmt.Println()

	// Determine what to reconfigure
	var reconfigureEmbedding, reconfigureGeneration bool

	if len(args) == 0 {
		// No arguments - reconfigure everything
		reconfigureEmbedding = true
		reconfigureGeneration = true
		fmt.Println(dim("Reconfiguring all settings. Current configuration will be prefilled."))
		fmt.Println()
	} else {
		switch args[0] {
		case "embedding":
			reconfigureEmbedding = true
		case "generation":
			reconfigureGeneration = true
		default:
			fmt.Fprintln(os.Stderr, red("Unknown target:", args[0]))
			fmt.Fprintln(os.Stderr, "Usage: repog reconfig [embedding|generation]")
			os.Exit(1)
		}
	}

	// Store original embedding config to detect changes
	originalEmbedding := cfg.Embedding

	// Reconfigure embedding
	if reconfigureEmbedding {
		newEmbedCfg, newEmbedAPIKey := reconfigureEmbeddingProvider(cfg.Embedding, reconfigProvider, reconfigModel, reconfigDimensions, reconfigBaseURL, reconfigAPIKey, red, dim, green, yellow)

		// Check if embedding config changed
		embeddingChanged := originalEmbedding.Provider != newEmbedCfg.Provider ||
			originalEmbedding.Model != newEmbedCfg.Model ||
			originalEmbedding.Dimensions != newEmbedCfg.Dimensions

		// Calculate chunk sizes to detect if they changed
		var oldChunkSize, newChunkSize int
		var chunkSizeChanged bool

		if embeddingChanged {
			// Get old provider's token limit
			oldAPIKey, _ := config.GetAPIKeyForProvider(originalEmbedding.Provider)
			if oldProvider, err := provider.NewEmbeddingProvider(originalEmbedding, oldAPIKey); err == nil {
				oldChunkSize = sync.CalculateMaxCharsFromTokens(oldProvider.MaxTokens())
			}

			// Get new provider's token limit
			if newProvider, err := provider.NewEmbeddingProvider(newEmbedCfg, newEmbedAPIKey); err == nil {
				newChunkSize = sync.CalculateMaxCharsFromTokens(newProvider.MaxTokens())
			}

			chunkSizeChanged = oldChunkSize != newChunkSize && oldChunkSize > 0 && newChunkSize > 0
		}

		if embeddingChanged {
			// Warn about clearing embeddings and potentially chunks
			fmt.Println()
			fmt.Println(yellow("⚠️  Warning: Embedding configuration has changed"))
			fmt.Println()
			fmt.Println("  Previous:", fmt.Sprintf("%s (%s, %dd)", originalEmbedding.Provider, originalEmbedding.Model, originalEmbedding.Dimensions))
			fmt.Println("  New:     ", fmt.Sprintf("%s (%s, %dd)", newEmbedCfg.Provider, newEmbedCfg.Model, newEmbedCfg.Dimensions))
			fmt.Println()

			if chunkSizeChanged {
				fmt.Println(yellow("  ⚠️  Chunk size will change:"))
				fmt.Println(fmt.Sprintf("     Previous: %d characters", oldChunkSize))
				fmt.Println(fmt.Sprintf("     New:      %d characters", newChunkSize))
				fmt.Println()
				fmt.Println(yellow("  This will delete ALL existing embeddings AND chunks."))
				fmt.Println(dim("  You'll need to run:"))
				fmt.Println(dim("    1. repog sync --owned --starred  (to re-chunk with new size)"))
				fmt.Println(dim("    2. repog embed                   (to generate new embeddings)"))
			} else {
				fmt.Println(yellow("  This will delete ALL existing embeddings."))
				fmt.Println(dim("  You'll need to run `repog embed` to regenerate them."))
			}
			fmt.Println()

			var confirmed bool
			prompt := &survey.Confirm{
				Message: "Continue with reconfiguration?",
				Default: false,
			}
			if err := survey.AskOne(prompt, &confirmed); err != nil || !confirmed {
				fmt.Println(dim("Reconfiguration cancelled."))
				return nil
			}

			// Open database
			database, err := db.Open(cfg.DBPath, originalEmbedding.Dimensions)
			if err != nil {
				fmt.Fprintln(os.Stderr, red("Failed to open database:", err))
				os.Exit(1)
			}

			s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)

			// Clear embeddings
			s.Suffix = " Clearing embeddings..."
			s.Start()

			if err := clearEmbeddings(database, newEmbedCfg.Dimensions); err != nil {
				s.Stop()
				fmt.Fprintln(os.Stderr, red("Failed to clear embeddings:", err))
				_ = database.Close()
				os.Exit(1)
			}

			s.Stop()
			fmt.Println(green("✓"), "Embeddings cleared")

			// Clear chunks if chunk size changed
			if chunkSizeChanged {
				s.Suffix = " Clearing chunks..."
				s.Start()

				if err := clearChunks(database); err != nil {
					s.Stop()
					fmt.Fprintln(os.Stderr, red("Failed to clear chunks:", err))
					_ = database.Close()
					os.Exit(1)
				}

				s.Stop()
				fmt.Println(green("✓"), "Chunks cleared")
			}

			_ = database.Close()
		}

		// Update config
		cfg.Embedding = newEmbedCfg

		// Store API key
		if err := config.SetAPIKeyForProvider(newEmbedCfg.Provider, newEmbedAPIKey); err != nil {
			fmt.Fprintln(os.Stderr, red("Failed to save embedding API key:", err))
			os.Exit(1)
		}
	}

	// Reconfigure generation
	if reconfigureGeneration {
		newGenCfg, newGenAPIKey := reconfigureGenerationProvider(cfg.Generation, reconfigProvider, reconfigModel, reconfigFallback, reconfigBaseURL, reconfigAPIKey, red, dim, green)

		// Update config
		cfg.Generation = newGenCfg

		// Store API key
		if err := config.SetAPIKeyForProvider(newGenCfg.Provider, newGenAPIKey); err != nil {
			fmt.Fprintln(os.Stderr, red("Failed to save generation API key:", err))
			os.Exit(1)
		}
	}

	// Save config file
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Saving configuration..."
	s.Start()

	if err := config.SaveConfigFile(cfg); err != nil {
		s.Stop()
		fmt.Fprintln(os.Stderr, red("Failed to save config:", err))
		os.Exit(1)
	}

	s.Stop()
	fmt.Println(green("✓"), "Configuration saved")

	// Summary
	fmt.Println()
	fmt.Println(bold(green("Reconfiguration complete!")))
	fmt.Println()
	if reconfigureEmbedding {
		fmt.Println("Embedding:", cfg.Embedding.Provider, fmt.Sprintf("(%s, %dd)", cfg.Embedding.Model, cfg.Embedding.Dimensions))
	}
	if reconfigureGeneration {
		fmt.Println("Generation:", cfg.Generation.Provider, fmt.Sprintf("(%s)", cfg.Generation.Model))
	}
	fmt.Println()

	return nil
}

// reconfigureEmbeddingProvider handles embedding provider reconfiguration
func reconfigureEmbeddingProvider(current config.ProviderConfig, providerFlag, modelFlag string, dimensionsFlag int, baseURLFlag, apiKeyFlag string, red, dim, green, yellow func(...interface{}) string) (config.ProviderConfig, string) {
	var selectedProvider string
	var apiKey string
	var model string
	var dimensions int
	var baseURL string

	// Select provider (prefill with current)
	if providerFlag != "" {
		selectedProvider = providerFlag
	} else {
		fmt.Println()
		fmt.Println(dim("Select an embedding provider:"))
		options := []string{"gemini", "openai", "openrouter", "voyageai", "ollama"}
		prompt := &survey.Select{
			Message: "Embedding Provider:",
			Options: options,
			Default: current.Provider,
		}
		if err := survey.AskOne(prompt, &selectedProvider); err != nil {
			fmt.Println(red("✗"), "Failed to read input:", err)
			os.Exit(1)
		}
	}

	// Get model
	if modelFlag != "" {
		model = modelFlag
	} else {
		// Determine default based on provider
		var defaultModel string
		if selectedProvider == current.Provider {
			defaultModel = current.Model
		} else {
			switch selectedProvider {
			case "gemini":
				defaultModel = "gemini-embedding-2-preview"
			case "openai":
				defaultModel = "text-embedding-3-small"
			case "openrouter":
				defaultModel = "openai/text-embedding-3-small"
			case "voyageai":
				defaultModel = "voyage-code-3"
			case "ollama":
				defaultModel = "nomic-embed-text"
			}
		}

		fmt.Println()
		prompt := &survey.Input{
			Message: "Model:",
			Default: defaultModel,
		}
		if err := survey.AskOne(prompt, &model); err != nil {
			fmt.Println(red("✗"), "Failed to read input:", err)
			os.Exit(1)
		}
	}

	// Get dimensions
	if dimensionsFlag != 0 {
		dimensions = dimensionsFlag
	} else {
		// Determine default based on provider
		var defaultDimensions int
		if selectedProvider == current.Provider {
			defaultDimensions = current.Dimensions
		} else {
			switch selectedProvider {
			case "gemini":
				defaultDimensions = 768
			case "openai", "openrouter":
				defaultDimensions = 1536
			case "voyageai":
				defaultDimensions = 1024
			case "ollama":
				defaultDimensions = 768
			}
		}

		fmt.Println()
		var dimStr string
		prompt := &survey.Input{
			Message: "Dimensions:",
			Default: fmt.Sprintf("%d", defaultDimensions),
		}
		if err := survey.AskOne(prompt, &dimStr); err != nil {
			fmt.Println(red("✗"), "Failed to read input:", err)
			os.Exit(1)
		}
		fmt.Sscanf(dimStr, "%d", &dimensions)
	}

	// Get base URL for Ollama
	if selectedProvider == "ollama" {
		if baseURLFlag != "" {
			baseURL = baseURLFlag
		} else {
			fmt.Println()
			defaultURL := "http://localhost:11434"
			if current.Provider == "ollama" && current.BaseURL != "" {
				defaultURL = current.BaseURL
			}
			prompt := &survey.Input{
				Message: "Ollama Base URL:",
				Default: defaultURL,
			}
			if err := survey.AskOne(prompt, &baseURL); err != nil {
				fmt.Println(red("✗"), "Failed to read input:", err)
				os.Exit(1)
			}
		}
	}

	// Get API key
	if selectedProvider == "ollama" {
		apiKey = "" // Ollama doesn't need an API key
	} else if apiKeyFlag != "" {
		apiKey = apiKeyFlag
	} else {
		// Try to get existing key from keyring
		existingKey, err := config.GetAPIKeyForProvider(selectedProvider)
		if err == nil && existingKey != "" {
			fmt.Println()
			var reuseKey bool
			prompt := &survey.Confirm{
				Message: fmt.Sprintf("Use existing %s API key?", selectedProvider),
				Default: true,
			}
			if err := survey.AskOne(prompt, &reuseKey); err == nil && reuseKey {
				apiKey = existingKey
			}
		}

		if apiKey == "" {
			fmt.Println()
			switch selectedProvider {
			case "gemini":
				fmt.Println(dim("Get a Gemini API key at: https://aistudio.google.com/apikey"))
			case "openai":
				fmt.Println(dim("Get an OpenAI API key at: https://platform.openai.com/api-keys"))
			case "openrouter":
				fmt.Println(dim("Get an OpenRouter API key at: https://openrouter.ai/keys"))
			case "voyageai":
				fmt.Println(dim("Get a Voyage AI API key at: https://dash.voyageai.com"))
			case "anthropic":
				fmt.Println(dim("Get an Anthropic API key at: https://console.anthropic.com"))
			}
			fmt.Println()

			prompt := &survey.Password{
				Message: fmt.Sprintf("%s API Key:", selectedProvider),
			}
			if err := survey.AskOne(prompt, &apiKey, survey.WithValidator(survey.Required)); err != nil {
				fmt.Println(red("✗"), "Failed to read input:", err)
				os.Exit(1)
			}
		}
	}

	// Build config
	cfg := config.ProviderConfig{
		Provider:   selectedProvider,
		Model:      model,
		Dimensions: dimensions,
		BaseURL:    baseURL,
	}

	// Validate provider
	fmt.Println()
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Validating %s embedding...", selectedProvider)
	s.Start()

	embedProvider, err := provider.NewEmbeddingProvider(cfg, apiKey)
	if err != nil {
		s.Stop()
		fmt.Println(red("✗"), "Failed to create embedding provider:", err)
		os.Exit(1)
	}

	if err := embedProvider.Validate(context.Background()); err != nil {
		s.Stop()
		fmt.Println(red("✗"), fmt.Sprintf("%s embedding validation failed:", selectedProvider), err)
		os.Exit(1)
	}

	s.Stop()
	fmt.Println(green("✓"), fmt.Sprintf("%s embedding validated", selectedProvider))

	return cfg, apiKey
}

// reconfigureGenerationProvider handles generation provider reconfiguration
func reconfigureGenerationProvider(current config.ProviderConfig, providerFlag, modelFlag, fallbackFlag, baseURLFlag, apiKeyFlag string, red, dim, green func(...interface{}) string) (config.ProviderConfig, string) {
	var selectedProvider string
	var apiKey string
	var model string
	var fallback string
	var baseURL string

	// Select provider (prefill with current)
	if providerFlag != "" {
		selectedProvider = providerFlag
	} else {
		fmt.Println()
		fmt.Println(dim("Select a generation provider:"))
		options := []string{"gemini", "openai", "openrouter", "ollama"}
		prompt := &survey.Select{
			Message: "Generation Provider:",
			Options: options,
			Default: current.Provider,
		}
		if err := survey.AskOne(prompt, &selectedProvider); err != nil {
			fmt.Println(red("✗"), "Failed to read input:", err)
			os.Exit(1)
		}
	}

	// Get model
	if modelFlag != "" {
		model = modelFlag
	} else {
		// Determine default based on provider
		var defaultModel string
		if selectedProvider == current.Provider {
			defaultModel = current.Model
		} else {
			switch selectedProvider {
			case "gemini":
				defaultModel = "gemini-2.5-flash"
			case "openai":
				defaultModel = "gpt-4o"
			case "openrouter":
				defaultModel = "openai/gpt-4o"
			case "ollama":
				defaultModel = "llama3.2"
			}
		}

		fmt.Println()
		prompt := &survey.Input{
			Message: "Model:",
			Default: defaultModel,
		}
		if err := survey.AskOne(prompt, &model); err != nil {
			fmt.Println(red("✗"), "Failed to read input:", err)
			os.Exit(1)
		}
	}

	// Get fallback model
	if fallbackFlag != "" {
		fallback = fallbackFlag
	} else {
		// Determine default based on provider
		var defaultFallback string
		if selectedProvider == current.Provider {
			defaultFallback = current.Fallback
		} else {
			switch selectedProvider {
			case "gemini":
				defaultFallback = "gemini-3.0-flash"
			case "openai":
				defaultFallback = "gpt-3.5-turbo"
			case "openrouter":
				defaultFallback = "openai/gpt-3.5-turbo"
			case "ollama":
				defaultFallback = "llama2"
			}
		}

		fmt.Println()
		prompt := &survey.Input{
			Message: "Fallback Model:",
			Default: defaultFallback,
		}
		if err := survey.AskOne(prompt, &fallback); err != nil {
			fmt.Println(red("✗"), "Failed to read input:", err)
			os.Exit(1)
		}
	}

	// Get base URL for Ollama
	if selectedProvider == "ollama" {
		if baseURLFlag != "" {
			baseURL = baseURLFlag
		} else {
			fmt.Println()
			defaultURL := "http://localhost:11434"
			if current.Provider == "ollama" && current.BaseURL != "" {
				defaultURL = current.BaseURL
			}
			prompt := &survey.Input{
				Message: "Ollama Base URL:",
				Default: defaultURL,
			}
			if err := survey.AskOne(prompt, &baseURL); err != nil {
				fmt.Println(red("✗"), "Failed to read input:", err)
				os.Exit(1)
			}
		}
	}

	// Get API key
	if selectedProvider == "ollama" {
		apiKey = "" // Ollama doesn't need an API key
	} else if apiKeyFlag != "" {
		apiKey = apiKeyFlag
	} else {
		// Try to get existing key from keyring
		existingKey, err := config.GetAPIKeyForProvider(selectedProvider)
		if err == nil && existingKey != "" {
			fmt.Println()
			var reuseKey bool
			prompt := &survey.Confirm{
				Message: fmt.Sprintf("Use existing %s API key?", selectedProvider),
				Default: true,
			}
			if err := survey.AskOne(prompt, &reuseKey); err == nil && reuseKey {
				apiKey = existingKey
			}
		}

		if apiKey == "" {
			fmt.Println()
			switch selectedProvider {
			case "gemini":
				fmt.Println(dim("Get a Gemini API key at: https://aistudio.google.com/apikey"))
			case "openai":
				fmt.Println(dim("Get an OpenAI API key at: https://platform.openai.com/api-keys"))
			case "openrouter":
				fmt.Println(dim("Get an OpenRouter API key at: https://openrouter.ai/keys"))
			case "voyageai":
				fmt.Println(dim("Get a Voyage AI API key at: https://dash.voyageai.com"))
			case "anthropic":
				fmt.Println(dim("Get an Anthropic API key at: https://console.anthropic.com"))
			}
			fmt.Println()

			prompt := &survey.Password{
				Message: fmt.Sprintf("%s API Key:", selectedProvider),
			}
			if err := survey.AskOne(prompt, &apiKey, survey.WithValidator(survey.Required)); err != nil {
				fmt.Println(red("✗"), "Failed to read input:", err)
				os.Exit(1)
			}
		}
	}

	// Build config
	cfg := config.ProviderConfig{
		Provider: selectedProvider,
		Model:    model,
		Fallback: fallback,
		BaseURL:  baseURL,
	}

	// Validate provider
	fmt.Println()
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" Validating %s generation...", selectedProvider)
	s.Start()

	llmProvider, err := provider.NewLLMProvider(cfg, apiKey)
	if err != nil {
		s.Stop()
		fmt.Println(red("✗"), "Failed to create LLM provider:", err)
		os.Exit(1)
	}

	if err := llmProvider.Validate(context.Background()); err != nil {
		s.Stop()
		fmt.Println(red("✗"), fmt.Sprintf("%s generation validation failed:", selectedProvider), err)
		os.Exit(1)
	}

	s.Stop()
	fmt.Println(green("✓"), fmt.Sprintf("%s generation validated", selectedProvider))

	return cfg, apiKey
}

// clearEmbeddings drops and recreates the embeddings table with new dimensions
func clearEmbeddings(database *sql.DB, newDimensions int) error {
	// Drop existing embeddings table
	if _, err := database.Exec("DROP TABLE IF EXISTS chunk_embeddings"); err != nil {
		return err
	}

	// Recreate with new dimensions
	createSQL := db.CreateChunkEmbeddingsTableSQL(newDimensions)
	if _, err := database.Exec(createSQL); err != nil {
		return err
	}

	// Mark all repos as needing re-embedding
	if _, err := database.Exec("UPDATE repos SET embedded_hash = NULL, embedded_at = NULL"); err != nil {
		return err
	}

	// Update stored dimensions in meta table
	if err := db.SetEmbeddingDimensions(database, newDimensions); err != nil {
		return err
	}

	return nil
}

// clearChunks deletes all chunks and resets repo sync state
func clearChunks(database *sql.DB) error {
	// Delete all chunks
	if _, err := database.Exec("DELETE FROM chunks"); err != nil {
		return err
	}

	// Reset repo sync state to force re-sync
	if _, err := database.Exec("UPDATE repos SET pushed_at_hash = NULL, embedded_hash = NULL, embedded_at = NULL"); err != nil {
		return err
	}

	// Clear sync state
	if _, err := database.Exec("DELETE FROM sync_state"); err != nil {
		return err
	}

	return nil
}
