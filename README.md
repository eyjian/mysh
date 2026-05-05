# mysh

[中文文档](README_zh.md)

MySQL CLI with syntax highlighting and intelligent auto-completion.

An enhanced MySQL command-line client that provides real-time SQL syntax highlighting, context-aware auto-completion, and interactive editing on top of the standard MySQL CLI experience.

## Features

- **Syntax Highlighting** — Real-time coloring of SQL keywords, strings, numbers, comments, functions, and operators
- **Smart Auto-Completion** — Context-aware suggestions for keywords, table names, column names, and database names
- **Interactive Editor** — Multi-line editing with cursor movement, history navigation, and selection
- **Command History** — Persistent history with search (`Ctrl+R`), navigation (Up/Down), and deduplication
- **Multiple Output Formats** — Table (default), vertical (`\G`), and JSON (`\j`) result formatting
- **Schema Metadata Cache** — Auto-cached table/column info with lazy refresh after DDL statements
- **Configurable Themes** — Customizable color schemes via `~/.mysh.yaml`
- **Zero Runtime Dependencies** — Single static binary, no CGO required

## Installation

### One-click Install (Recommended)

```bash
curl -sSL https://raw.githubusercontent.com/eyjian/mysh/main/install.sh | bash
```

Or with custom install directory:

```bash
INSTALL_DIR=~/.local/bin curl -sSL https://raw.githubusercontent.com/eyjian/mysh/main/install.sh | bash
```

### go install

```bash
go install github.com/eyjian/mysh@latest
```

### Manual Download

Download the latest binary from [GitHub Releases](https://github.com/eyjian/mysh/releases):

| OS | Arch | File |
|----|------|------|
| Linux | amd64 | `mysh-linux-amd64` |
| Linux | arm64 | `mysh-linux-arm64` |
| macOS | amd64 | `mysh-darwin-amd64` |
| macOS | arm64 | `mysh-darwin-arm64` |
| Windows | amd64 | `mysh-windows-amd64.exe` |
| Windows | arm64 | `mysh-windows-arm64.exe` |

```bash
# Example for Linux amd64
curl -sfL -o /usr/local/bin/mysh https://github.com/eyjian/mysh/releases/latest/download/mysh-linux-amd64
chmod +x /usr/local/bin/mysh
```

### Build from Source

```bash
git clone https://github.com/eyjian/mysh.git
cd mysh
make build
```

## Usage

### Basic Connection

```bash
# Connect with DSN
mysh -h 127.0.0.1 -P 3306 -u root -p mydb

# Or use a DSN string
mysh root:password@tcp(127.0.0.1:3306)/mydb
```

### Command Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h` | `127.0.0.1` | MySQL host |
| `-P` | `3306` | MySQL port |
| `-u` | `root` | MySQL user |
| `-p` | (empty) | MySQL password |
| `-d` | (empty) | Default database |
| `--config` | `~/.mysh.yaml` | Config file path |
| `--version` | — | Print version |
| `--help` | — | Print help |

### Interactive Usage

Once connected, type SQL statements and press `Enter` to execute:

```sql
mysh> SELECT * FROM users WHERE id = 1;
mysh> SHOW TABLES;
mysh> DESCRIBE users;
```

Multi-line input is supported — press `Enter` after an unclosed statement to continue:

```sql
mysh> SELECT id, name
    -> FROM users
    -> WHERE age > 18;
```

### Key Bindings

| Key | Action |
|-----|--------|
| `Tab` / `Ctrl+Space` | Trigger auto-completion |
| `Up` / `Down` | Navigate history |
| `Ctrl+R` | Search history |
| `Ctrl+C` | Cancel current input |
| `Ctrl+D` | Exit mysh |
| `Home` / `Ctrl+A` | Move cursor to beginning |
| `End` / `Ctrl+E` | Move cursor to end |
| `Left` / `Right` | Move cursor |

### Output Formats

| Suffix | Format | Example |
|--------|--------|---------|
| (default) | Aligned table | `SELECT * FROM users;` |
| `\G` | Vertical (one column per line) | `SELECT * FROM users\G` |
| `\j` | JSON array | `SELECT * FROM users\j` |

## Configuration

Configuration file: `~/.mysh.yaml`

```yaml
# Connection defaults
connection:
  host: "127.0.0.1"
  port: 3306
  user: "root"
  password: ""
  database: ""

# UI settings
ui:
  prompt: "mysh> "
  multiline_prompt: "    -> "
  page_size: 20          # Result pagination rows

# Syntax highlighting theme
theme:
  keyword: "bold magenta"
  string: "yellow"
  number: "cyan"
  comment: "dim"
  function: "green"
  operator: "white"

# History settings
history:
  file: "~/.mysh_history"
  max_entries: 10000

# Auto-completion settings
completion:
  min_chars: 2           # Minimum characters to trigger
  max_suggestions: 15
```

Theme values use [lipgloss](https://github.com/charmbracelet/lipgloss) style syntax: `bold`, `italic`, `underline`, `dim`, plus color names (`red`, `green`, `yellow`, `blue`, `magenta`, `cyan`, `white`) or hex codes (`#ff0000`).

## Built-in Commands

| Command | Description |
|---------|-------------|
| `\help` | Show help |
| `\connect <dsn>` | Connect to a database |
| `\use <db>` | Switch database |
| `\refresh` | Refresh metadata cache |
| `\status` | Show connection status |
| `\format table\|vertical\|json` | Change output format |
| `\history [pattern]` | Search command history |
| `\source <file>` | Execute SQL from file |
| `\quit` | Exit mysh |

## Auto-Completion Context

mysh provides context-aware suggestions based on your cursor position:

| Context | Suggestions |
|---------|-------------|
| Statement start | SQL keywords, built-in commands |
| After `SELECT` | Column names, functions, `DISTINCT`, `*` |
| After `FROM` | Table names, database names, `WHERE`/`JOIN` keywords |
| After `WHERE` / `AND` / `OR` | Column names, operators, functions |
| After `JOIN` | Table names, `ON`/`USING` |
| After `table.` | Column names of that table, `*` |
| After `SET` | Database names, `NAMES`/`AUTOCOMMIT` |
| After `ORDER BY` / `GROUP BY` | Column names, `ASC`/`DESC` |

## Architecture

```
┌─────────────────────────────────────────┐
│              CLI Entry (main)            │
├─────────────────────────────────────────┤
│           TUI Layer (bubbletea)         │
│  ┌──────────┐ ┌───────────┐ ┌────────┐ │
│  │ Editor   │ │ Completer │ │ Syntax │ │
│  │          │ │           │ │Highlight│ │
│  └──────────┘ └───────────┘ └────────┘ │
├─────────────────────────────────────────┤
│           Service Layer                  │
│  ┌──────────┐ ┌───────────┐ ┌────────┐ │
│  │ SQL Exec │ │ Metadata  │ │ History│ │
│  └──────────┘ └───────────┘ └────────┘ │
├─────────────────────────────────────────┤
│           Data Layer                     │
│  ┌──────────┐ ┌───────────┐ ┌────────┐ │
│  │ MySQL    │ │ Schema    │ │ File   │ │
│  │ Conn Pool│ │ Cache     │ │ Store  │ │
│  └──────────┘ └───────────┘ └────────┘ │
└─────────────────────────────────────────┘
```

See [docs/design/ARCHITECTURE.md](docs/design/ARCHITECTURE.md) for full design details.

## Development

### Prerequisites

- Go 1.21+

### Build & Test

```bash
# Build
make build

# Run tests
make test

# Verbose tests
make test-verbose

# Lint
make lint

# Cross-compile all platforms
make cross-compile
```

### Project Structure

```
mysh/
├── main.go              # Entry point
├── config/              # Configuration loading
├── connection/          # MySQL connection pool
├── tui/                 # bubbletea TUI model
├── editor/              # Line editor
├── highlight/           # SQL tokenizer & highlighter
├── completer/           # Auto-completion engine
├── executor/            # SQL execution
├── metadata/            # Schema metadata cache
├── history/             # Command history
├── output/              # Result formatting
└── docs/design/         # Architecture & interface docs
```

### Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Commit your changes (`git commit -m 'Add my feature'`)
4. Ensure tests pass (`make test`)
5. Push to the branch (`git push origin feature/my-feature`)
6. Open a Pull Request

## License

[MIT License](LICENSE)
