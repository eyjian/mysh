package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

// Config holds all configuration for mysh.
type Config struct {
	Connection ConnectionConfig         `mapstructure:"connection"`
	UI         UIConfig                 `mapstructure:"ui"`
	Theme      ThemeConfig              `mapstructure:"theme"`
	History    HistoryConfig            `mapstructure:"history"`
	Completion CompletionConfig         `mapstructure:"completion"`
	Safety     SafetyConfig             `mapstructure:"safety"`
	Aliases    map[string]string              `mapstructure:"aliases"`
	Sessions   map[string]SessionConfig       `mapstructure:"sessions"`
	Favorites  map[string]FavoriteConfig      `mapstructure:"favorites"`

	// Internal: path to config file (for Save)
	configPath string `mapstructure:"-"`
}

// ConnectionConfig holds database connection parameters.
type ConnectionConfig struct {
	Driver   string `mapstructure:"driver"` // "mysql" (default) or "postgres"/"pg"
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Database string `mapstructure:"database"`
	Charset  string `mapstructure:"charset"`
	SSLMode  string `mapstructure:"ssl_mode"` // PostgreSQL SSL mode: disable, allow, prefer, require, verify-ca, verify-full (default: disable)
	Timeout  int    `mapstructure:"timeout"`   // connection timeout in seconds (0 = default 30s)
	SSH      SSHConfig `mapstructure:"ssh"`
}

// SSHConfig holds SSH tunnel parameters for connecting through a jump host.
type SSHConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Key      string `mapstructure:"key"`
	Password string `mapstructure:"password"`
}

// Enabled returns true if SSH tunnel is configured.
func (s SSHConfig) Enabled() bool {
	return s.Host != ""
}

// SafetyConfig holds safety-related configuration.
type SafetyConfig struct {
	SafeUpdates   bool `mapstructure:"safe_updates"`   // block UPDATE/DELETE without WHERE/LIMIT
	SlowThreshold int  `mapstructure:"slow_threshold"` // slow query threshold in seconds (0 = disabled)
}

// UIConfig holds UI-related configuration.
type UIConfig struct {
	Prompt          string `mapstructure:"prompt"`
	MultilinePrompt string `mapstructure:"multiline_prompt"`
	PageSize        int    `mapstructure:"page_size"`
}

// ThemeConfig holds syntax highlighting theme colors.
type ThemeConfig struct {
	Keyword  string `mapstructure:"keyword"`
	String   string `mapstructure:"string"`
	Number   string `mapstructure:"number"`
	Comment  string `mapstructure:"comment"`
	Function string `mapstructure:"function"`
	Operator string `mapstructure:"operator"`
}

// HistoryConfig holds history settings.
type HistoryConfig struct {
	File       string `mapstructure:"file"`
	MaxEntries int    `mapstructure:"max_entries"`
}

// CompletionConfig holds auto-completion settings.
type CompletionConfig struct {
	MinChars       int `mapstructure:"min_chars"`
	MaxSuggestions int `mapstructure:"max_suggestions"`
}

// FavoriteConfig holds a saved favorite SQL query.
type FavoriteConfig struct {
	SQL         string `mapstructure:"sql"`
	Description string `mapstructure:"description"`
}

// SessionConfig holds a saved database session connection config.
type SessionConfig struct {
	Driver   string `mapstructure:"driver"`
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Database string `mapstructure:"database"`
	Charset  string `mapstructure:"charset"`
	SSLMode  string `mapstructure:"ssl_mode"`
}

// ToConnectionConfig converts a SessionConfig to a ConnectionConfig.
func (s SessionConfig) ToConnectionConfig() ConnectionConfig {
	port := s.Port
	driver := s.Driver
	if driver == "" {
		driver = "mysql"
	}
	if port == 0 {
		switch driver {
		case "postgres", "pg":
			port = 5432
		default:
			port = 3306
		}
	}
	return ConnectionConfig{
		Driver:   driver,
		Host:     s.Host,
		Port:     port,
		User:     s.User,
		Password: s.Password,
		Database: s.Database,
		Charset:  s.Charset,
		SSLMode:  s.SSLMode,
	}
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Connection: ConnectionConfig{
			Host:     "127.0.0.1",
			Port:     3306,
			User:     "root",
			Password: "",
			Database: "",
		},
		UI: UIConfig{
			Prompt:          "mysh> ",
			MultilinePrompt: "    -> ",
			PageSize:        0,
		},
		Theme: ThemeConfig{
			Keyword:  "bold magenta",
			String:   "yellow",
			Number:   "cyan",
			Comment:  "dim",
			Function: "green",
			Operator: "white",
		},
		History: HistoryConfig{
			File:       "~/.mysh_history",
			MaxEntries: 10000,
		},
		Completion: CompletionConfig{
			MinChars:       2,
			MaxSuggestions: 15,
		},
		Safety: SafetyConfig{
			SafeUpdates:   false,
			SlowThreshold: 0,
		},
	}
}

// DSN returns the Data Source Name from connection config.
// The format depends on the driver type.
func (c *ConnectionConfig) DSN() string {
	driver := c.Driver
	if driver == "" {
		driver = "mysql"
	}
	switch driver {
	case "postgres", "pg":
		sslMode := c.SSLMode
		if sslMode == "" {
			sslMode = "disable"
		}
		// Build DSN with only non-empty fields. An empty value like `dbname=`
		// can confuse lib/pq's key-value parser and cause subsequent keys
		// (e.g. sslmode) to be silently dropped.
		parts := []string{}
		if c.Host != "" {
			parts = append(parts, fmt.Sprintf("host=%s", c.Host))
		}
		if c.Port > 0 {
			parts = append(parts, fmt.Sprintf("port=%d", c.Port))
		}
		if c.User != "" {
			parts = append(parts, fmt.Sprintf("user=%s", c.User))
		}
		if c.Password != "" {
			parts = append(parts, fmt.Sprintf("password=%s", c.Password))
		}
		if c.Database != "" {
			parts = append(parts, fmt.Sprintf("dbname=%s", c.Database))
		}
		parts = append(parts, fmt.Sprintf("sslmode=%s", sslMode))
		if c.Charset != "" {
			parts = append(parts, "client_encoding="+c.Charset)
		}
		return strings.Join(parts, " ")
	default:
		dsn := c.User + ":" + c.Password + "@tcp(" + c.Host + ":" + itoa(c.Port) + ")/" + c.Database
		if c.Charset != "" {
			dsn += "?charset=" + c.Charset
		}
		return dsn
	}
}

// DefaultPort returns the default port based on the driver.
func (c *ConnectionConfig) DefaultPort() int {
	switch c.Driver {
	case "postgres", "pg":
		return 5432
	default:
		return 3306
	}
}

// Load reads configuration from file and applies defaults.
// It looks for ~/.mysh.yaml, then /etc/mysh/config.yaml.
// Missing config file is not an error; defaults are used.
func Load() (*Config, error) {
	cfg := DefaultConfig()

	v := viper.New()
	v.SetConfigName("mysh")
	v.SetConfigType("yaml")

	// Add config search paths
	home, err := os.UserHomeDir()
	if err == nil {
		v.AddConfigPath(home)
	}
	v.AddConfigPath("/etc/mysh")

	// Set defaults from our default config
	v.SetDefault("connection.driver", cfg.Connection.Driver)
	v.SetDefault("connection.host", cfg.Connection.Host)
	v.SetDefault("connection.port", cfg.Connection.Port)
	v.SetDefault("connection.user", cfg.Connection.User)
	v.SetDefault("connection.password", cfg.Connection.Password)
	v.SetDefault("connection.database", cfg.Connection.Database)
	v.SetDefault("ui.prompt", cfg.UI.Prompt)
	v.SetDefault("ui.multiline_prompt", cfg.UI.MultilinePrompt)
	v.SetDefault("ui.page_size", cfg.UI.PageSize)
	v.SetDefault("theme.keyword", cfg.Theme.Keyword)
	v.SetDefault("theme.string", cfg.Theme.String)
	v.SetDefault("theme.number", cfg.Theme.Number)
	v.SetDefault("theme.comment", cfg.Theme.Comment)
	v.SetDefault("theme.function", cfg.Theme.Function)
	v.SetDefault("theme.operator", cfg.Theme.Operator)
	v.SetDefault("history.file", cfg.History.File)
	v.SetDefault("history.max_entries", cfg.History.MaxEntries)
	v.SetDefault("completion.min_chars", cfg.Completion.MinChars)
	v.SetDefault("completion.max_suggestions", cfg.Completion.MaxSuggestions)
	v.SetDefault("safety.safe_updates", cfg.Safety.SafeUpdates)
	v.SetDefault("safety.slow_threshold", cfg.Safety.SlowThreshold)

	// Read config file (optional)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
		// Config file not found; use defaults
	}

	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}

	// Expand ~ in file paths
	cfg.History.File = expandHome(cfg.History.File)

	// Record config file path for Save
	if v.ConfigFileUsed() != "" {
		cfg.configPath = v.ConfigFileUsed()
	} else {
		home, _ := os.UserHomeDir()
		if home != "" {
			cfg.configPath = filepath.Join(home, ".mysh.yaml")
		}
	}

	return cfg, nil
}

// LoadFromArgs overrides config with command-line arguments.
func LoadFromArgs(host string, port int, user, password, database string) (*Config, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}

	if host != "" {
		cfg.Connection.Host = host
	}
	if port > 0 {
		cfg.Connection.Port = port
	}
	if user != "" {
		cfg.Connection.User = user
	}
	if password != "" {
		cfg.Connection.Password = password
	}
	if database != "" {
		cfg.Connection.Database = database
	}

	return cfg, nil
}

// expandHome replaces ~ with the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// itoa converts int to string without importing strconv at the package level.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

// Save writes the config back to the YAML file.
func Save(cfg *Config) error {
	if cfg == nil || cfg.configPath == "" {
		return fmt.Errorf("cannot save: no config file path")
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(cfg.configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
