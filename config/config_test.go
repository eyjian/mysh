package config

import (
	"os"
	"path/filepath"
	"testing"
)

// --- DefaultConfig tests ---

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Connection defaults
	if cfg.Connection.Host != "127.0.0.1" {
		t.Errorf("default host = %q, want %q", cfg.Connection.Host, "127.0.0.1")
	}
	if cfg.Connection.Port != 3306 {
		t.Errorf("default port = %d, want %d", cfg.Connection.Port, 3306)
	}
	if cfg.Connection.User != "root" {
		t.Errorf("default user = %q, want %q", cfg.Connection.User, "root")
	}
	if cfg.Connection.Password != "" {
		t.Errorf("default password = %q, want empty", cfg.Connection.Password)
	}
	if cfg.Connection.Database != "" {
		t.Errorf("default database = %q, want empty", cfg.Connection.Database)
	}

	// UI defaults
	if cfg.UI.Prompt != "mysh> " {
		t.Errorf("default prompt = %q, want %q", cfg.UI.Prompt, "mysh> ")
	}
	if cfg.UI.MultilinePrompt != "    -> " {
		t.Errorf("default multiline_prompt = %q, want %q", cfg.UI.MultilinePrompt, "    -> ")
	}
	if cfg.UI.PageSize != 20 {
		t.Errorf("default page_size = %d, want %d", cfg.UI.PageSize, 20)
	}

	// Theme defaults
	if cfg.Theme.Keyword != "bold magenta" {
		t.Errorf("default keyword = %q, want %q", cfg.Theme.Keyword, "bold magenta")
	}
	if cfg.Theme.String != "yellow" {
		t.Errorf("default string = %q, want %q", cfg.Theme.String, "yellow")
	}

	// History defaults
	if cfg.History.MaxEntries != 10000 {
		t.Errorf("default max_entries = %d, want %d", cfg.History.MaxEntries, 10000)
	}

	// Completion defaults
	if cfg.Completion.MinChars != 2 {
		t.Errorf("default min_chars = %d, want %d", cfg.Completion.MinChars, 2)
	}
	if cfg.Completion.MaxSuggestions != 15 {
		t.Errorf("default max_suggestions = %d, want %d", cfg.Completion.MaxSuggestions, 15)
	}
}

// --- DSN tests ---

func TestDSN(t *testing.T) {
	tests := []struct {
		name string
		cfg  ConnectionConfig
		want string
	}{
		{
			name: "basic connection",
			cfg:  ConnectionConfig{Host: "127.0.0.1", Port: 3306, User: "root", Password: "", Database: "testdb"},
			want: "root:@tcp(127.0.0.1:3306)/testdb",
		},
		{
			name: "with password",
			cfg:  ConnectionConfig{Host: "10.0.0.1", Port: 3307, User: "admin", Password: "secret", Database: "mydb"},
			want: "admin:secret@tcp(10.0.0.1:3307)/mydb",
		},
		{
			name: "empty database",
			cfg:  ConnectionConfig{Host: "localhost", Port: 3306, User: "root", Password: "", Database: ""},
			want: "root:@tcp(localhost:3306)/",
		},
		{
			name: "non-standard port",
			cfg:  ConnectionConfig{Host: "db.example.com", Port: 33060, User: "app", Password: "p@ss", Database: "prod"},
			want: "app:p@ss@tcp(db.example.com:33060)/prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.DSN()
			if got != tt.want {
				t.Errorf("DSN() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- itoa tests ---

func TestItoa(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{3306, "3306"},
		{-1, "-1"},
		{-3306, "-3306"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := itoa(tt.input)
			if got != tt.want {
				t.Errorf("itoa(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- expandHome tests ---

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "tilde expansion",
			path: "~/test",
			want: filepath.Join(home, "test"),
		},
		{
			name: "absolute path unchanged",
			path: "/etc/mysh/config.yaml",
			want: "/etc/mysh/config.yaml",
		},
		{
			name: "relative path unchanged",
			path: "relative/path",
			want: "relative/path",
		},
		{
			name: "just tilde",
			path: "~",
			want: "~", // only ~/ is expanded, not bare ~
		},
		{
			name: "empty path",
			path: "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandHome(tt.path)
			if got != tt.want {
				t.Errorf("expandHome(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// --- Load tests ---

func TestLoad_NoConfigFile(t *testing.T) {
	// When no config file exists, defaults should be returned
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Connection.Host != "127.0.0.1" {
		t.Errorf("host = %q, want %q", cfg.Connection.Host, "127.0.0.1")
	}
	if cfg.Connection.Port != 3306 {
		t.Errorf("port = %d, want %d", cfg.Connection.Port, 3306)
	}
}

func TestLoad_WithConfigFile(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configContent := `
connection:
  host: "192.168.1.100"
  port: 3307
  user: "testuser"
  password: "testpass"
  database: "testdb"
ui:
  prompt: "test> "
  page_size: 50
theme:
  keyword: "bold red"
history:
  max_entries: 5000
completion:
  min_chars: 3
  max_suggestions: 10
`
	configPath := filepath.Join(tmpDir, "mysh.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Set viper config path via environment variable
	// Since Load() uses viper with hardcoded paths, we test by placing config in home dir
	// Instead, test the config parsing directly
	t.Setenv("HOME", tmpDir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Connection.Host != "192.168.1.100" {
		t.Errorf("host = %q, want %q", cfg.Connection.Host, "192.168.1.100")
	}
	if cfg.Connection.Port != 3307 {
		t.Errorf("port = %d, want %d", cfg.Connection.Port, 3307)
	}
	if cfg.Connection.User != "testuser" {
		t.Errorf("user = %q, want %q", cfg.Connection.User, "testuser")
	}
	if cfg.Connection.Password != "testpass" {
		t.Errorf("password = %q, want %q", cfg.Connection.Password, "testpass")
	}
	if cfg.Connection.Database != "testdb" {
		t.Errorf("database = %q, want %q", cfg.Connection.Database, "testdb")
	}
	if cfg.UI.Prompt != "test> " {
		t.Errorf("prompt = %q, want %q", cfg.UI.Prompt, "test> ")
	}
	if cfg.UI.PageSize != 50 {
		t.Errorf("page_size = %d, want %d", cfg.UI.PageSize, 50)
	}
	if cfg.Theme.Keyword != "bold red" {
		t.Errorf("keyword = %q, want %q", cfg.Theme.Keyword, "bold red")
	}
	if cfg.History.MaxEntries != 5000 {
		t.Errorf("max_entries = %d, want %d", cfg.History.MaxEntries, 5000)
	}
	if cfg.Completion.MinChars != 3 {
		t.Errorf("min_chars = %d, want %d", cfg.Completion.MinChars, 3)
	}
	if cfg.Completion.MaxSuggestions != 10 {
		t.Errorf("max_suggestions = %d, want %d", cfg.Completion.MaxSuggestions, 10)
	}
}

func TestLoad_PartialConfig(t *testing.T) {
	// Config file with only some fields set - others should use defaults
	tmpDir := t.TempDir()
	configContent := `
connection:
  host: "10.0.0.1"
`
	configPath := filepath.Join(tmpDir, "mysh.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("HOME", tmpDir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Overridden value
	if cfg.Connection.Host != "10.0.0.1" {
		t.Errorf("host = %q, want %q", cfg.Connection.Host, "10.0.0.1")
	}
	// Default values should remain
	if cfg.Connection.Port != 3306 {
		t.Errorf("port = %d, want %d (default)", cfg.Connection.Port, 3306)
	}
	if cfg.UI.Prompt != "mysh> " {
		t.Errorf("prompt = %q, want %q (default)", cfg.UI.Prompt, "mysh> ")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	// Invalid YAML content should return an error
	tmpDir := t.TempDir()
	configContent := `
connection:
  host: "value
  invalid yaml: [
`
	configPath := filepath.Join(tmpDir, "mysh.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("HOME", tmpDir)

	_, err := Load()
	if err == nil {
		t.Error("Load() with invalid YAML should return error, got nil")
	}
}

// --- LoadFromArgs tests ---

func TestLoadFromArgs_AllOverrides(t *testing.T) {
	cfg, err := LoadFromArgs("10.0.0.1", 3307, "admin", "secret", "mydb")
	if err != nil {
		t.Fatalf("LoadFromArgs() error = %v", err)
	}
	if cfg.Connection.Host != "10.0.0.1" {
		t.Errorf("host = %q, want %q", cfg.Connection.Host, "10.0.0.1")
	}
	if cfg.Connection.Port != 3307 {
		t.Errorf("port = %d, want %d", cfg.Connection.Port, 3307)
	}
	if cfg.Connection.User != "admin" {
		t.Errorf("user = %q, want %q", cfg.Connection.User, "admin")
	}
	if cfg.Connection.Password != "secret" {
		t.Errorf("password = %q, want %q", cfg.Connection.Password, "secret")
	}
	if cfg.Connection.Database != "mydb" {
		t.Errorf("database = %q, want %q", cfg.Connection.Database, "mydb")
	}
}

func TestLoadFromArgs_PartialOverrides(t *testing.T) {
	// Only override host; rest should be defaults
	cfg, err := LoadFromArgs("custom-host", 0, "", "", "")
	if err != nil {
		t.Fatalf("LoadFromArgs() error = %v", err)
	}
	if cfg.Connection.Host != "custom-host" {
		t.Errorf("host = %q, want %q", cfg.Connection.Host, "custom-host")
	}
	// port=0 should NOT override (only port > 0 overrides)
	if cfg.Connection.Port != 3306 {
		t.Errorf("port = %d, want %d (default)", cfg.Connection.Port, 3306)
	}
	if cfg.Connection.User != "root" {
		t.Errorf("user = %q, want %q (default)", cfg.Connection.User, "root")
	}
}

func TestLoadFromArgs_EmptyStringsNoOverride(t *testing.T) {
	cfg, err := LoadFromArgs("", 0, "", "", "")
	if err != nil {
		t.Fatalf("LoadFromArgs() error = %v", err)
	}
	// All empty/zero values should leave defaults unchanged
	if cfg.Connection.Host != "127.0.0.1" {
		t.Errorf("host = %q, want default %q", cfg.Connection.Host, "127.0.0.1")
	}
	if cfg.Connection.User != "root" {
		t.Errorf("user = %q, want default %q", cfg.Connection.User, "root")
	}
}
