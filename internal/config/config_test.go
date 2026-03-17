package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// mockKeyring implements KeyringBackend for testing.
type mockKeyring struct {
	store map[string]string
}

func newMockKeyring() *mockKeyring {
	return &mockKeyring{store: make(map[string]string)}
}

func (m *mockKeyring) Set(service, key, value string) error {
	m.store[service+":"+key] = value
	return nil
}

func (m *mockKeyring) Get(service, key string) (string, error) {
	v, ok := m.store[service+":"+key]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func (m *mockKeyring) Delete(service, key string) error {
	delete(m.store, service+":"+key)
	return nil
}

// Override the configDir function for testing
var testConfigDir string

func init() {
	// Override configDir to use test directory
	origConfigDir := configDir
	configDir = func() (string, error) {
		if testConfigDir != "" {
			return testConfigDir, nil
		}
		return origConfigDir()
	}
}

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testConfigDir = dir
	return dir
}

func teardownTestDir() {
	testConfigDir = ""
}

func TestSaveConfigWritesValidYAML(t *testing.T) {
	dir := setupTestDir(t)
	defer teardownTestDir()

	mock := newMockKeyring()
	SetKeyringBackend(mock)

	cfg := &Config{
		DBPath: filepath.Join(dir, "test.db"),
	}

	err := SaveConfig(cfg, "test-pat", "test-api-key")
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Verify config file was created
	configFile := filepath.Join(dir, "config.yaml")
	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	if len(data) == 0 {
		t.Error("Config file is empty")
	}

	// Verify DB path is in the file
	if !contains(string(data), cfg.DBPath) {
		t.Errorf("Config file does not contain DB path: %s", string(data))
	}
}

func TestLoadConfigAfterSave(t *testing.T) {
	dir := setupTestDir(t)
	defer teardownTestDir()

	mock := newMockKeyring()
	SetKeyringBackend(mock)

	originalCfg := &Config{
		DBPath: filepath.Join(dir, "test.db"),
	}

	err := SaveConfig(originalCfg, "test-pat", "test-api-key")
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loadedCfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loadedCfg.DBPath != originalCfg.DBPath {
		t.Errorf("DBPath mismatch: got %s, want %s", loadedCfg.DBPath, originalCfg.DBPath)
	}

	if loadedCfg.ConfigVersion != ConfigVersion {
		t.Errorf("ConfigVersion mismatch: got %d, want %d", loadedCfg.ConfigVersion, ConfigVersion)
	}
}

func TestLoadConfigReturnsErrNotConfiguredWhenFileMissing(t *testing.T) {
	_ = setupTestDir(t)
	defer teardownTestDir()

	mock := newMockKeyring()
	SetKeyringBackend(mock)

	// Don't create any config file
	_, err := LoadConfig()
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Expected ErrNotConfigured, got: %v", err)
	}
}

func TestIsConfiguredReturnsFalseWhenMissing(t *testing.T) {
	dir := setupTestDir(t)
	defer teardownTestDir()
	_ = dir

	mock := newMockKeyring()
	SetKeyringBackend(mock)

	if IsConfigured() {
		t.Error("IsConfigured should return false when config is missing")
	}
}

func TestConfigFilePermissions(t *testing.T) {
	dir := setupTestDir(t)
	defer teardownTestDir()

	mock := newMockKeyring()
	SetKeyringBackend(mock)

	cfg := &Config{
		DBPath: filepath.Join(dir, "test.db"),
	}

	err := SaveConfig(cfg, "test-pat", "test-api-key")
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	configFile := filepath.Join(dir, "config.yaml")
	info, err := os.Stat(configFile)
	if err != nil {
		t.Fatalf("Failed to stat config file: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("Config file permissions wrong: got %o, want 0600", perm)
	}
}

func TestClearConfigRemovesFile(t *testing.T) {
	dir := setupTestDir(t)
	defer teardownTestDir()

	mock := newMockKeyring()
	SetKeyringBackend(mock)

	cfg := &Config{
		DBPath: filepath.Join(dir, "test.db"),
	}

	err := SaveConfig(cfg, "test-pat", "test-api-key")
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	configFile := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Fatal("Config file should exist before clear")
	}

	err = ClearConfig()
	if err != nil {
		t.Fatalf("ClearConfig failed: %v", err)
	}

	// After clear, LoadConfig should fail
	if IsConfigured() {
		t.Error("IsConfigured should return false after ClearConfig")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestDefaultDBPath(t *testing.T) {
	result := DefaultDBPath()

	// Assert: result contains ".repog/repog.db"
	if !contains(result, ".repog/repog.db") {
		t.Errorf("DefaultDBPath should contain '.repog/repog.db', got %q", result)
	}

	// Assert: result starts with the user's home directory
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("Could not get user home dir: %v", err)
	}

	if !contains(result, home) {
		t.Errorf("DefaultDBPath should start with home directory %q, got %q", home, result)
	}

	// Assert: result does not contain "~" (must be fully expanded)
	if contains(result, "~") {
		t.Errorf("DefaultDBPath should not contain '~', got %q", result)
	}
}

func TestGetGitHubPAT_ReturnsStoredValue(t *testing.T) {
	_ = setupTestDir(t)
	defer teardownTestDir()

	mock := newMockKeyring()
	SetKeyringBackend(mock)

	// Store a test token
	_ = mock.Set(KeyringService, KeyringGitHubPAT, "ghp_testtoken123")

	pat, err := GetGitHubPAT()
	if err != nil {
		t.Errorf("GetGitHubPAT returned unexpected error: %v", err)
	}
	if pat != "ghp_testtoken123" {
		t.Errorf("GetGitHubPAT = %q, want %q", pat, "ghp_testtoken123")
	}
}

func TestGetGitHubPAT_ReturnsErrorWhenNotSet(t *testing.T) {
	_ = setupTestDir(t)
	defer teardownTestDir()

	mock := newMockKeyring() // Empty keyring
	SetKeyringBackend(mock)

	pat, err := GetGitHubPAT()
	if err == nil {
		t.Error("GetGitHubPAT should return error when not set")
	}
	if pat != "" {
		t.Errorf("GetGitHubPAT should return empty string when not set, got %q", pat)
	}
}

func TestGetGeminiAPIKey_ReturnsStoredValue(t *testing.T) {
	_ = setupTestDir(t)
	defer teardownTestDir()

	mock := newMockKeyring()
	SetKeyringBackend(mock)

	// Store a test API key
	_ = mock.Set(KeyringService, KeyringGeminiAPIKey, "AIzaTestKey")

	key, err := GetGeminiAPIKey()
	if err != nil {
		t.Errorf("GetGeminiAPIKey returned unexpected error: %v", err)
	}
	if key != "AIzaTestKey" {
		t.Errorf("GetGeminiAPIKey = %q, want %q", key, "AIzaTestKey")
	}
}

func TestGetGeminiAPIKey_ReturnsErrorWhenNotSet(t *testing.T) {
	_ = setupTestDir(t)
	defer teardownTestDir()

	mock := newMockKeyring() // Empty keyring
	SetKeyringBackend(mock)

	key, err := GetGeminiAPIKey()
	if err == nil {
		t.Error("GetGeminiAPIKey should return error when not set")
	}
	if key != "" {
		t.Errorf("GetGeminiAPIKey should return empty string when not set, got %q", key)
	}
}

func TestSaveConfig_StoresSecretsInKeyring(t *testing.T) {
	dir := setupTestDir(t)
	defer teardownTestDir()

	mock := newMockKeyring()
	SetKeyringBackend(mock)

	cfg := &Config{
		DBPath: filepath.Join(dir, "test.db"),
	}

	err := SaveConfig(cfg, "ghp_mysecretpat", "AIzaMySecretKey")
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Assert: mock keyring Get("repog", "github_pat") returns the PAT
	storedPAT, err := mock.Get(KeyringService, KeyringGitHubPAT)
	if err != nil {
		t.Errorf("Failed to get PAT from keyring: %v", err)
	}
	if storedPAT != "ghp_mysecretpat" {
		t.Errorf("Stored PAT = %q, want %q", storedPAT, "ghp_mysecretpat")
	}

	// Assert: mock keyring Get("repog", "gemini_api_key") returns the Gemini key
	storedKey, err := mock.Get(KeyringService, KeyringGeminiAPIKey)
	if err != nil {
		t.Errorf("Failed to get Gemini key from keyring: %v", err)
	}
	if storedKey != "AIzaMySecretKey" {
		t.Errorf("Stored Gemini key = %q, want %q", storedKey, "AIzaMySecretKey")
	}
}

func TestSaveConfig_WritesDBPathToFile(t *testing.T) {
	dir := setupTestDir(t)
	defer teardownTestDir()

	mock := newMockKeyring()
	SetKeyringBackend(mock)

	customPath := filepath.Join(dir, "custom", "path", "mydb.db")
	cfg := &Config{
		DBPath: customPath,
	}

	err := SaveConfig(cfg, "test-pat", "test-key")
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Load the written YAML file directly
	configFile := filepath.Join(dir, "config.yaml")
	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	// Assert: the file contains the custom db_path value
	if !contains(string(data), customPath) {
		t.Errorf("Config file should contain custom db_path %q, got:\n%s", customPath, string(data))
	}
}

func TestClearConfig_DeletesSecretsFromKeyring(t *testing.T) {
	dir := setupTestDir(t)
	defer teardownTestDir()

	mock := newMockKeyring()
	SetKeyringBackend(mock)

	// Store both secrets
	_ = mock.Set(KeyringService, KeyringGitHubPAT, "test-pat")
	_ = mock.Set(KeyringService, KeyringGeminiAPIKey, "test-key")

	// Create a config file so ClearConfig has something to remove
	cfg := &Config{
		DBPath: filepath.Join(dir, "test.db"),
	}
	err := SaveConfig(cfg, "test-pat", "test-key")
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Call ClearConfig
	err = ClearConfig()
	if err != nil {
		t.Fatalf("ClearConfig failed: %v", err)
	}

	// Assert: mock keyring Get for both keys now returns an error
	_, err = mock.Get(KeyringService, KeyringGitHubPAT)
	if err == nil {
		t.Error("GitHub PAT should be deleted from keyring")
	}

	_, err = mock.Get(KeyringService, KeyringGeminiAPIKey)
	if err == nil {
		t.Error("Gemini API key should be deleted from keyring")
	}

	// Assert: config file is removed
	configFile := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(configFile); !os.IsNotExist(err) {
		t.Error("Config file should be removed after ClearConfig")
	}
}

func TestLoadConfig_ReturnsErrNotConfiguredWhenKeyringEmpty(t *testing.T) {
	dir := setupTestDir(t)
	defer teardownTestDir()

	mock := newMockKeyring()
	SetKeyringBackend(mock)

	// Create a valid config file manually (without secrets in keyring)
	configFile := filepath.Join(dir, "config.yaml")
	configContent := "db_path: /some/path/test.db\nconfig_version: 2\n"
	err := os.WriteFile(configFile, []byte(configContent), 0600)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Don't store any secrets in keyring - leave it empty

	// Call LoadConfig
	_, err = LoadConfig()
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("LoadConfig should return ErrNotConfigured when keyring is empty, got: %v", err)
	}
}

func TestIsConfigured_ReturnsTrueWhenBothSecretsPresent(t *testing.T) {
	dir := setupTestDir(t)
	defer teardownTestDir()

	mock := newMockKeyring()
	SetKeyringBackend(mock)

	// Save a complete config
	cfg := &Config{
		DBPath: filepath.Join(dir, "test.db"),
	}
	err := SaveConfig(cfg, "test-pat", "test-key")
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Assert: IsConfigured() returns true
	if !IsConfigured() {
		t.Error("IsConfigured should return true when both secrets are present")
	}
}

func TestIsConfigured_ReturnsFalseWhenOnlyOneSecretPresent(t *testing.T) {
	dir := setupTestDir(t)
	defer teardownTestDir()

	mock := newMockKeyring()
	SetKeyringBackend(mock)

	// Create a valid config file
	configFile := filepath.Join(dir, "config.yaml")
	configContent := "db_path: /some/path/test.db\nconfig_version: 2\n"
	err := os.WriteFile(configFile, []byte(configContent), 0600)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Store only the GitHub PAT (no Gemini key)
	_ = mock.Set(KeyringService, KeyringGitHubPAT, "test-pat")

	// Assert: IsConfigured() returns false
	if IsConfigured() {
		t.Error("IsConfigured should return false when only one secret is present")
	}
}
