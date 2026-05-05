package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/eyjian/mysh/completer"
	"github.com/eyjian/mysh/config"
	"github.com/eyjian/mysh/connection"
	"github.com/eyjian/mysh/executor"
	"github.com/eyjian/mysh/highlight"
	"github.com/eyjian/mysh/history"
	"github.com/eyjian/mysh/metadata"
	"github.com/eyjian/mysh/output"
	sshpkg "github.com/eyjian/mysh/ssh"
	"github.com/eyjian/mysh/tui"

	tea "github.com/charmbracelet/bubbletea"
)

// version is set via -ldflags at build time
var version = "dev"

// CLI flag defaults
var (
	flagHost               string
	flagPort               int
	flagUser               string
	flagPassword           string
	flagDatabase           string
	flagExecute            string
	flagAutoVerticalOutput bool
	flagFormat             string
	flagCharset            string
	flagPageSize           = -1 // -1 means not set, 0 means no pagination
	flagSafeUpdates        bool
	flagSlowThreshold      = -1 // -1 means not set, 0 means disabled
	flagConnTimeout        = -1 // -1 means not set, 0 means default 30s
	flagSSHHost            string
	flagSSHPort            int
	flagSSHUser            string
	flagSSHKey             string
	flagSSHPassword        string
)

func main() {
	// Parse command-line arguments (simple flag parsing, no external dependency)
	if err := parseArgs(); err != nil {
		os.Exit(1)
	}

	// Step 1: Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %s\n", err)
		os.Exit(1)
	}

	// Override config with CLI arguments
	if flagHost != "" {
		cfg.Connection.Host = flagHost
	}
	if flagPort > 0 {
		cfg.Connection.Port = flagPort
	}
	if flagUser != "" {
		cfg.Connection.User = flagUser
	}
	if flagPassword != "" {
		cfg.Connection.Password = flagPassword
	}
	if flagDatabase != "" {
		cfg.Connection.Database = flagDatabase
	}
	if flagCharset != "" {
		cfg.Connection.Charset = flagCharset
	}
	if flagPageSize >= 0 {
		cfg.UI.PageSize = flagPageSize
	}

	// Override SSH config with CLI arguments
	if flagSSHHost != "" {
		cfg.Connection.SSH.Host = flagSSHHost
	}
	if flagSSHPort > 0 {
		cfg.Connection.SSH.Port = flagSSHPort
	}
	if flagSSHUser != "" {
		cfg.Connection.SSH.User = flagSSHUser
	}
	if flagSSHKey != "" {
		cfg.Connection.SSH.Key = flagSSHKey
	}
	if flagSSHPassword != "" {
		cfg.Connection.SSH.Password = flagSSHPassword
	}

	// Step 2: Establish SSH tunnel if configured
	var tunnel *sshpkg.Tunnel
	if cfg.Connection.SSH.Enabled() {
		fmt.Fprintf(os.Stderr, "Establishing SSH tunnel to %s", cfg.Connection.SSH.Host)
		if cfg.Connection.SSH.Port > 0 && cfg.Connection.SSH.Port != 22 {
			fmt.Fprintf(os.Stderr, ":%d", cfg.Connection.SSH.Port)
		}
		fmt.Fprintf(os.Stderr, " ... ")

		var tunnelErr error
		tunnel, tunnelErr = sshpkg.NewTunnel(&cfg.Connection.SSH, cfg.Connection.Host, cfg.Connection.Port)
		if tunnelErr != nil {
			fmt.Fprintf(os.Stderr, "FAILED\nError: %s\n", tunnelErr)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "OK (local port %d)\n", tunnel.LocalPort())

		// Override MySQL connection to go through the tunnel
		cfg.Connection.Host = "127.0.0.1"
		cfg.Connection.Port = tunnel.LocalPort()
	}

	// Step 3: Connect to MySQL
	fmt.Fprintf(os.Stderr, "Connecting to %s@%s:%d", cfg.Connection.User, cfg.Connection.Host, cfg.Connection.Port)
	if cfg.Connection.Database != "" {
		fmt.Fprintf(os.Stderr, "/%s", cfg.Connection.Database)
	}
	fmt.Fprintf(os.Stderr, " ... ")

	pool, err := connection.New(&cfg.Connection)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAILED\nError: %s\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "OK\n")

	// If -e flag is provided, execute the statement and exit (non-interactive mode)
	if flagExecute != "" {
		os.Exit(execBatch(pool, flagExecute))
	}

	// Step 3: Initialize metadata cache (empty initially, will load async)
	meta, err := metadata.NewCache(pool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: metadata cache init failed: %s\n", err)
	}

	// Step 4: Initialize history
	hist := history.New(cfg.History.File, cfg.History.MaxEntries)
	if err := hist.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load history: %s\n", err)
	}

	// Step 5: Initialize executor
	exec := executor.New(pool, meta)

	// Apply safety settings from config and CLI flags
	if cfg.Safety.SafeUpdates || flagSafeUpdates {
		exec.SetSafeUpdates(true)
	}
	if flagSlowThreshold >= 0 {
		exec.SetSlowThreshold(time.Duration(flagSlowThreshold) * time.Second)
	} else if cfg.Safety.SlowThreshold > 0 {
		exec.SetSlowThreshold(time.Duration(cfg.Safety.SlowThreshold) * time.Second)
	}

	// Apply connection timeout from config and CLI flags
	if flagConnTimeout >= 0 {
		pool.SetConnTimeout(time.Duration(flagConnTimeout) * time.Second)
	} else if cfg.Connection.Timeout > 0 {
		pool.SetConnTimeout(time.Duration(cfg.Connection.Timeout) * time.Second)
	}

	// Step 6: Initialize output formatter (writing to stdout)
	outFmt := output.FormatTable
	if flagFormat != "" {
		if f, err := output.ParseFormat(flagFormat); err == nil {
			outFmt = f
		}
	}
	formatter := output.NewFormatter(outFmt, os.Stdout)

	// Step 7: Initialize highlighter
	highlighter := highlight.NewHighlighter(&cfg.Theme)

	// Step 8: Initialize completer with metadata adapter
	comp := completer.NewCompleter(&cfg.Completion)
	if meta != nil {
		adapter := metadata.NewCacheAdapter(meta)
		comp.SetMetadata(adapter)
	}

	// Step 9: Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		fmt.Fprintf(os.Stderr, "\nReceived signal %s, shutting down...\n", sig)
		cancel()
		// Force exit after timeout
		time.AfterFunc(5*time.Second, func() {
			os.Exit(1)
		})
	}()

	// Step 10: Assemble dependencies and start TUI
	deps := tui.Dependencies{
		Config:             cfg,
		Pool:               pool,
		Executor:           exec,
		Meta:               meta,
		History:            hist,
		Formatter:          formatter,
		Highlighter:        highlighter,
		Completer:          comp,
		AutoVerticalOutput: flagAutoVerticalOutput,
		SSHTunnel:          tunnel,
	}

	model := tui.NewModel(deps)
	program := tea.NewProgram(model, tea.WithContext(ctx))

	// Async: load metadata cache in background so TUI starts immediately
	if meta != nil {
		go func() {
			if err := meta.Refresh(); err != nil {
				// Silently ignore — cache will be empty but TUI works fine
				_ = err
			}
		}()
	}

	// Panic recovery: restore terminal state if TUI crashes
	defer func() {
		if r := recover(); r != nil {
			// Restore terminal to usable state
			fmt.Fprintf(os.Stderr, "\033[?25h\033[0m") // show cursor, reset attributes
			fmt.Fprintf(os.Stderr, "\nmysh crashed: %v\n", r)
			cleanup(deps)
			os.Exit(2)
		}
	}()

	// Run the TUI main loop
	_, runErr := program.Run()

	// Step 11: Graceful cleanup
	cleanup(deps)

	if runErr != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %s\n", runErr)
		os.Exit(1)
	}
}

// execBatch executes SQL statements in non-interactive mode (like mysql -e).
// It supports multiple statements separated by semicolons.
// Returns 0 on success, 1 on error.
func execBatch(pool *connection.Pool, statements string) int {
	exec := executor.New(pool, nil)
	// Use format from --format flag, default to table
	outFmt := output.FormatTable
	if flagFormat != "" {
		if f, err := output.ParseFormat(flagFormat); err == nil {
			outFmt = f
		}
	}
	formatter := output.NewFormatter(outFmt, os.Stdout)
	ctx := context.Background()

	// Split by semicolons, filter empty
	parts := strings.Split(statements, ";")
	hasError := false
	for _, stmt := range parts {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		result, err := exec.Execute(ctx, stmt+";")
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			hasError = true
			continue
		}
		if result.Error != nil {
			fmt.Fprintf(os.Stderr, "%s\n", result.Error)
			hasError = true
			continue
		}
		if err := formatter.WriteResult(result); err != nil {
			fmt.Fprintf(os.Stderr, "Output error: %s\n", err)
			hasError = true
		}
	}

	pool.Close()
	if hasError {
		return 1
	}
	return 0
}

// parseArgs parses simple command-line flags.
// Supported: -h host, -P port, -u user, -p password, -D database, -e statement
func parseArgs() error {
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--host":
			if i+1 < len(args) {
				flagHost = args[i+1]
				i++
			}
		case "-P", "--port":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &flagPort)
				i++
			}
		case "-u", "--user":
			if i+1 < len(args) {
				flagUser = args[i+1]
				i++
			}
		case "-p", "--password":
			if i+1 < len(args) {
				flagPassword = args[i+1]
				i++
			} else {
				// -p without argument = prompt for password securely
				fmt.Fprintf(os.Stderr, "Enter password: ")
				pw, err := term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Fprintln(os.Stderr)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error reading password: %s\n", err)
					os.Exit(1)
				}
				flagPassword = string(pw)
			}
		case "-D", "--database":
			if i+1 < len(args) {
				flagDatabase = args[i+1]
				i++
			}
		case "-e", "--execute":
			if i+1 < len(args) {
				flagExecute = args[i+1]
				i++
			}
		case "--auto-vertical-output":
			flagAutoVerticalOutput = true
		case "--no-auto-vertical-output":
			flagAutoVerticalOutput = false
		case "--format":
			if i+1 < len(args) {
				flagFormat = args[i+1]
				i++
			}
		case "--default-character-set":
			if i+1 < len(args) {
				flagCharset = args[i+1]
				i++
			}
		case "--page-size":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &flagPageSize)
				i++
			}
		case "-U", "--safe-updates":
			flagSafeUpdates = true
		case "--slow-threshold":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &flagSlowThreshold)
				i++
			}
		case "--connect-timeout":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &flagConnTimeout)
				i++
			}
		case "--ssh-host":
			if i+1 < len(args) {
				flagSSHHost = args[i+1]
				i++
			}
		case "--ssh-port":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &flagSSHPort)
				i++
			}
		case "--ssh-user":
			if i+1 < len(args) {
				flagSSHUser = args[i+1]
				i++
			}
		case "--ssh-key":
			if i+1 < len(args) {
				flagSSHKey = args[i+1]
				i++
			}
		case "--ssh-password":
			if i+1 < len(args) {
				flagSSHPassword = args[i+1]
				i++
			}
		case "--help", "-help":
			printUsage()
			os.Exit(0)
		case "--version", "-version":
			fmt.Printf("mysh version %s\n", version)
			os.Exit(0)
		default:
			// Check for DSN-style argument (user:pass@host:port/db)
			if strings.Contains(args[i], "@") || strings.Contains(args[i], ":") && !strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(os.Stderr, "Warning: DSN-style argument not supported, use flags instead\n")
			} else if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(os.Stderr, "Unknown flag: %s\n\n", args[i])
				printUsage()
				return fmt.Errorf("unknown flag: %s", args[i])
			}
		}
	}
	return nil
}

// printUsage displays CLI usage information.
func printUsage() {
	fmt.Println(`mysh - MySQL CLI with syntax highlighting and auto-completion

Usage:
  mysh [options]
  mysh [options] -e "SQL_STATEMENT"

Options:
  -h, --host <host>       MySQL host (default: 127.0.0.1)
  -P, --port <port>       MySQL port (default: 3306)
  -u, --user <user>       MySQL user (default: root)
  -p, --password <pass>   MySQL password
  -D, --database <db>     Default database
  -e, --execute <stmt>    Execute SQL statement and exit
      --format <type>     Output format: table|vertical|json|markdown
      --default-character-set <name> Set the default character set
      --auto-vertical-output   Auto switch to vertical if result wider than terminal
      --no-auto-vertical-output  Disable auto vertical output (default)
      --page-size <n>     Result pagination rows (0 = no pagination, default: 0)
  -U, --safe-updates      Block UPDATE/DELETE without WHERE or LIMIT
      --slow-threshold <s>  Slow query warning threshold in seconds (0 = disabled)
      --connect-timeout <s>  Connection/query timeout in seconds (default: 30)
      --ssh-host <host>  SSH tunnel host (jump server)
      --ssh-port <port>  SSH tunnel port (default: 22)
      --ssh-user <user>  SSH tunnel user
      --ssh-key <path>   SSH private key path (default: ~/.ssh/id_rsa)
      --ssh-password <p> SSH password (prefer key auth)
      --help              Show this help message
      --version           Show version

Configuration:
  Config file: ~/.mysh.yaml

Examples:
  mysh -h localhost -u root -p secret -D mydb
  mysh --host db.example.com --port 3307 --user admin
  mysh -e "SHOW DATABASES"
  mysh -u root -p secret -e "SELECT * FROM users LIMIT 10"
  mysh -e "SELECT * FROM users" --format markdown
  mysh -e "SHOW TABLES" --format json
  mysh --default-character-set utf8mb4
  mysh -U --slow-threshold 5
  mysh -h 10.0.0.5 --ssh-host jump.example.com --ssh-user deploy
  mysh -h db.internal --ssh-host bastion --ssh-key ~/.ssh/id_ed25519`)
}

// cleanup performs graceful shutdown of all resources.
func cleanup(deps tui.Dependencies) {
	// Save history
	if deps.History != nil {
		if err := deps.History.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save history: %s\n", err)
		}
	}

	// Close database connection
	if deps.Pool != nil {
		if err := deps.Pool.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close connection: %s\n", err)
		}
	}

	// Close SSH tunnel
	if deps.SSHTunnel != nil {
		if err := deps.SSHTunnel.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close SSH tunnel: %s\n", err)
		}
	}

	fmt.Fprintln(os.Stderr, "")
}
