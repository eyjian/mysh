package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"mysh/completer"
	"mysh/config"
	"mysh/connection"
	"mysh/executor"
	"mysh/highlight"
	"mysh/history"
	"mysh/metadata"
	"mysh/output"
	"mysh/tui"

	tea "github.com/charmbracelet/bubbletea"
)

// version is set via -ldflags at build time
var version = "dev"

// CLI flag defaults
var (
	flagHost     string
	flagPort     int
	flagUser     string
	flagPassword string
	flagDatabase string
	flagExecute  string
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

	// Step 2: Connect to MySQL
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

	// Step 6: Initialize output formatter (writing to stdout)
	formatter := output.NewFormatter(output.FormatTable, os.Stdout)

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
		Config:      cfg,
		Pool:        pool,
		Executor:    exec,
		Meta:        meta,
		History:     hist,
		Formatter:   formatter,
		Highlighter: highlighter,
		Completer:   comp,
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
	formatter := output.NewFormatter(output.FormatTable, os.Stdout)
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
				// -p without argument = prompt for password (simplified: read from stdin)
				fmt.Fprintf(os.Stderr, "Enter password: ")
				// In a real implementation, we'd use terminal.ReadPassword
				// For simplicity, read from stdin
				var pw string
				fmt.Fscanln(os.Stdin, &pw)
				flagPassword = pw
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
      --help              Show this help message
      --version           Show version

Configuration:
  Config file: ~/.mysh.yaml

Examples:
  mysh -h localhost -u root -p secret -D mydb
  mysh --host db.example.com --port 3307 --user admin
  mysh -e "SHOW DATABASES"
  mysh -u root -p secret -e "SELECT * FROM users LIMIT 10"`)
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

	fmt.Fprintln(os.Stderr, "Goodbye!")
}
