// Package config handles configuration loading, saving, and credential management.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"
)

const (
	// KeyringService is the service name used for keyring storage.
	KeyringService = "repog"
	// KeyringGitHubPAT is the keyring key for the GitHub PAT.
	KeyringGitHubPAT = "github_pat"
	// KeyringGeminiAPIKey is the keyring key for the Gemini API key.
	KeyringGeminiAPIKey = "gemini_api_key"
	// KeyringOpenAIAPIKey is the keyring key for the OpenAI API key.
	KeyringOpenAIAPIKey = "openai_api_key"
	// KeyringOpenRouterAPIKey is the keyring key for the OpenRouter API key.
	KeyringOpenRouterAPIKey = "openrouter_api_key"
	// KeyringAnthropicAPIKey is the keyring key for the Anthropic API key.
	KeyringAnthropicAPIKey = "anthropic_api_key"
	// KeyringVoyageAIAPIKey is the keyring key for the Voyage AI API key.
	KeyringVoyageAIAPIKey = "voyageai_api_key"
	// ConfigVersion is the current config file version.
	ConfigVersion = 3
)

// ErrNotConfigured is returned when repog has not been initialised.
var ErrNotConfigured = errors.New("repog is not configured — run `repog init` first")

// ProviderConfig represents configuration for an AI provider
type ProviderConfig struct {
	Provider   string `yaml:"provider"`              // gemini, ollama, openrouter
	Model      string `yaml:"model"`                 // model name
	Dimensions int    `yaml:"dimensions,omitempty"`  // embedding dimensions (embedding only)
	MaxTokens  int    `yaml:"max_tokens,omitempty"`  // custom max token limit (embedding only, 0 = use model default)
	BaseURL    string `yaml:"base_url,omitempty"`    // custom base URL (ollama/openrouter)
	Fallback   string `yaml:"fallback,omitempty"`    // fallback model (LLM only)
}

// Config represents the on-disk configuration.
type Config struct {
	DBPath        string         `yaml:"db_path"`
	ConfigVersion int            `yaml:"config_version"`
	Embedding     ProviderConfig `yaml:"embedding"`
	Generation    ProviderConfig `yaml:"generation"`
}

// KeyringBackend defines the interface for keyring operations.
// This allows mocking in tests.
type KeyringBackend interface {
	Set(service, key, value string) error
	Get(service, key string) (string, error)
	Delete(service, key string) error
}

// realKeyring implements KeyringBackend using the actual system keyring.
type realKeyring struct{}

func (r *realKeyring) Set(service, key, value string) error {
	return keyring.Set(service, key, value)
}

func (r *realKeyring) Get(service, key string) (string, error) {
	return keyring.Get(service, key)
}

func (r *realKeyring) Delete(service, key string) error {
	return keyring.Delete(service, key)
}

// defaultKeyring is the keyring backend used by default.
var defaultKeyring KeyringBackend = &realKeyring{}

// SetKeyringBackend sets the keyring backend (for testing).
func SetKeyringBackend(kb KeyringBackend) {
	defaultKeyring = kb
}

// configDir returns the path to the repog config directory.
// Uses ~/.config/repog on macOS/Linux.
// This is a variable so it can be overridden in tests.
var configDir = func() (string, error) {
	configBase, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configBase, "repog"), nil
}

// configPath returns the path to the config file.
func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// DefaultDBPath returns the default database path.
func DefaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".repog", "repog.db")
	}
	return filepath.Join(home, ".repog", "repog.db")
}

// LoadConfig reads config from disk. Returns ErrNotConfigured if config file
// does not exist or required secrets are missing from the keyring.
// Automatically migrates v2 configs to v3 with Gemini defaults.
func LoadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, ErrNotConfigured
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotConfigured
		}
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Auto-migrate v2 to v3
	if cfg.ConfigVersion < 3 {
		cfg.Embedding = DefaultEmbeddingConfig()
		cfg.Generation = DefaultGenerationConfig()
		cfg.ConfigVersion = 3
		// Don't save here - let the user continue with defaults in memory
	}

	// Verify GitHub PAT exists
	_, err = defaultKeyring.Get(KeyringService, KeyringGitHubPAT)
	if err != nil {
		return nil, ErrNotConfigured
	}

	// Verify provider-specific API keys exist based on configuration
	if cfg.Embedding.Provider != "ollama" {
		_, err := GetAPIKeyForProvider(cfg.Embedding.Provider)
		if err != nil {
			return nil, ErrNotConfigured
		}
	}

	if cfg.Generation.Provider != "ollama" {
		_, err := GetAPIKeyForProvider(cfg.Generation.Provider)
		if err != nil {
			return nil, ErrNotConfigured
		}
	}

	return &cfg, nil
}

// SaveConfig writes config to disk and stores secrets in the keyring.
// Deprecated: Use SaveConfigFile and SetAPIKeyForProvider for new multi-provider setup.
func SaveConfig(cfg *Config, githubPAT, geminiAPIKey string) error {
	// Ensure config directory exists
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Ensure DB directory exists
	dbDir := filepath.Dir(cfg.DBPath)
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return err
	}

	// Set config version
	cfg.ConfigVersion = ConfigVersion

	// Ensure provider configs are set (for backward compatibility)
	if cfg.Embedding.Provider == "" {
		cfg.Embedding = DefaultEmbeddingConfig()
	}
	if cfg.Generation.Provider == "" {
		cfg.Generation = DefaultGenerationConfig()
	}

	// Write config file
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	path, err := configPath()
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}

	// Set permissions explicitly
	if err := os.Chmod(path, 0600); err != nil {
		return err
	}

	// Store secrets in keyring
	if err := defaultKeyring.Set(KeyringService, KeyringGitHubPAT, githubPAT); err != nil {
		return err
	}

	if err := defaultKeyring.Set(KeyringService, KeyringGeminiAPIKey, geminiAPIKey); err != nil {
		return err
	}

	return nil
}

// SaveConfigFile writes the config file to disk without storing API keys.
// Use SetAPIKeyForProvider to store API keys in the keyring separately.
func SaveConfigFile(cfg *Config) error {
	// Ensure config directory exists
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Ensure DB directory exists
	dbDir := filepath.Dir(cfg.DBPath)
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return err
	}

	// Set config version
	cfg.ConfigVersion = ConfigVersion

	// Write config file
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	path, err := configPath()
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}

	// Set permissions explicitly
	return os.Chmod(path, 0600)
}

// ClearConfig removes the config file and deletes keyring entries.
func ClearConfig() error {
	// Delete config file
	path, err := configPath()
	if err == nil {
		_ = os.Remove(path) // Ignore error if file doesn't exist
	}

	// Delete keyring entries (ignore errors)
	_ = defaultKeyring.Delete(KeyringService, KeyringGitHubPAT)
	_ = defaultKeyring.Delete(KeyringService, KeyringGeminiAPIKey)
	_ = defaultKeyring.Delete(KeyringService, KeyringOpenAIAPIKey)
	_ = defaultKeyring.Delete(KeyringService, KeyringOpenRouterAPIKey)
	_ = defaultKeyring.Delete(KeyringService, KeyringAnthropicAPIKey)
	_ = defaultKeyring.Delete(KeyringService, KeyringVoyageAIAPIKey)

	return nil
}

// IsConfigured returns true if config file exists and both secrets are present
// in the keyring.
func IsConfigured() bool {
	_, err := LoadConfig()
	return err == nil
}

// GetGitHubPAT retrieves the GitHub PAT from the keyring.
func GetGitHubPAT() (string, error) {
	pat, err := defaultKeyring.Get(KeyringService, KeyringGitHubPAT)
	if err != nil {
		return "", ErrNotConfigured
	}
	return pat, nil
}

// GetGeminiAPIKey retrieves the Gemini API key from the keyring.
func GetGeminiAPIKey() (string, error) {
	key, err := defaultKeyring.Get(KeyringService, KeyringGeminiAPIKey)
	if err != nil {
		return "", ErrNotConfigured
	}
	return key, nil
}

// GetAPIKeyForProvider retrieves the API key for a specific provider from the keyring.
func GetAPIKeyForProvider(provider string) (string, error) {
	switch provider {
	case "gemini":
		return GetGeminiAPIKey()
	case "openai":
		key, err := defaultKeyring.Get(KeyringService, KeyringOpenAIAPIKey)
		if err != nil {
			return "", ErrNotConfigured
		}
		return key, nil
	case "openrouter":
		key, err := defaultKeyring.Get(KeyringService, KeyringOpenRouterAPIKey)
		if err != nil {
			return "", ErrNotConfigured
		}
		return key, nil
	case "anthropic":
		key, err := defaultKeyring.Get(KeyringService, KeyringAnthropicAPIKey)
		if err != nil {
			return "", ErrNotConfigured
		}
		return key, nil
	case "voyageai":
		key, err := defaultKeyring.Get(KeyringService, KeyringVoyageAIAPIKey)
		if err != nil {
			return "", ErrNotConfigured
		}
		return key, nil
	case "ollama":
		// Ollama doesn't need an API key (local)
		return "", nil
	default:
		return "", fmt.Errorf("unknown provider: %s", provider)
	}
}

// SetAPIKeyForProvider stores the API key for a specific provider in the keyring.
func SetAPIKeyForProvider(provider, key string) error {
	switch provider {
	case "github":
		return defaultKeyring.Set(KeyringService, KeyringGitHubPAT, key)
	case "gemini":
		return defaultKeyring.Set(KeyringService, KeyringGeminiAPIKey, key)
	case "openai":
		return defaultKeyring.Set(KeyringService, KeyringOpenAIAPIKey, key)
	case "openrouter":
		return defaultKeyring.Set(KeyringService, KeyringOpenRouterAPIKey, key)
	case "anthropic":
		return defaultKeyring.Set(KeyringService, KeyringAnthropicAPIKey, key)
	case "voyageai":
		return defaultKeyring.Set(KeyringService, KeyringVoyageAIAPIKey, key)
	case "ollama":
		// Ollama doesn't need an API key
		return nil
	default:
		return fmt.Errorf("unknown provider: %s", provider)
	}
}

// DefaultEmbeddingConfig returns the default Gemini embedding configuration.
func DefaultEmbeddingConfig() ProviderConfig {
	return ProviderConfig{
		Provider:   "gemini",
		Model:      "gemini-embedding-2-preview",
		Dimensions: 768,
	}
}

// DefaultGenerationConfig returns the default Gemini generation configuration.
func DefaultGenerationConfig() ProviderConfig {
	return ProviderConfig{
		Provider: "gemini",
		Model:    "gemini-2.5-flash",
		Fallback: "gemini-3.0-flash",
	}
}
