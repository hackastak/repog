// Package config handles configuration loading, saving, and credential management.
package config

import (
	"errors"
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
	// ConfigVersion is the current config file version.
	ConfigVersion = 2
)

// ErrNotConfigured is returned when repog has not been initialised.
var ErrNotConfigured = errors.New("repog is not configured — run `repog init` first")

// Config represents the on-disk configuration.
type Config struct {
	DBPath        string `yaml:"db_path"`
	ConfigVersion int    `yaml:"config_version"`
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
// does not exist or either secret is missing from the keyring.
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

	// Verify both secrets exist in keyring
	_, err = defaultKeyring.Get(KeyringService, KeyringGitHubPAT)
	if err != nil {
		return nil, ErrNotConfigured
	}

	_, err = defaultKeyring.Get(KeyringService, KeyringGeminiAPIKey)
	if err != nil {
		return nil, ErrNotConfigured
	}

	return &cfg, nil
}

// SaveConfig writes config to disk and stores secrets in the keyring.
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
