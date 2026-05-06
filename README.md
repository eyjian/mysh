# mysh

[中文文档](README_zh.md)

MySQL and PostgreSQL CLI with syntax highlighting and intelligent auto-completion.

An enhanced database command-line client that supports both MySQL and PostgreSQL, providing real-time SQL syntax highlighting, context-aware auto-completion, and interactive editing on top of the standard CLI experience.

## Features

- **Syntax Highlighting** — Real-time coloring of SQL keywords, strings, numbers, comments, functions, and operators
- **Smart Auto-Completion** — Context-aware suggestions for keywords, table names, column names, and database names
- **SQL Snippets** — Built-in templates for CREATE TABLE, ALTER, INSERT, etc., a\helputo-expand on Tab
- **Interactive Editor** — Multi-line editing with cursor movement, history navigation, and selection
- **External Editor** — Open `$EDITOR` (vim/vi) to edit SQL with `\edit`, auto-execute on save
- **Command History** — Persistent history with search (`Ctrl+R`), navigation (Up/Down), and deduplication
- **Multiple Output Formats** — Table (default), vertical (`\G`), JSON (`\j`), and Markdown (`\m`) result formatting
- **Result Export** — Export query results to CSV, JSON, or Markdown files via `\export`
- **Pipe to Commands** — Pipe query results to system commands via `\pipe` (e.g., `\pipe grep pattern`)
- **Live Watch** — Periodically re-execute queries with `\watch` for real-time monitoring
- **Query Timing** — Toggle execution time display with `\timing`
- **NULL Display** — NULL values rendered in dim italic for clear visual distinction
- **SQL Aliases** — Define shortcuts for frequently used queries in config or interactively
- **Session Management** — Save, switch, and delete database connection profiles via `\session`
- **Favorite Queries** — Bookmark SQL queries with `\fav`, list/run/delete interactively
- **Transaction Support** — Explicit BEGIN/COMMIT/ROLLBACK with dedicated connection binding, `tx>` prompt indicator, and auto-rollback on exit
- **SSH Tunnel** — Connect to remote MySQL through an SSH jump host via local port forwarding
- **PostgreSQL Support** — Connect to PostgreSQL databases with `--driver postgres` or `postgres://` DSN
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
# Connect to MySQL (default)
mysh -h 127.0.0.1 -P 3306 -u root -p mydb

# Connect to PostgreSQL
mysh --driver postgres -h 127.0.0.1 -P 5432 -u postgres -p mydb

# Or use a DSN string
mysh root:password@tcp(127.0.0.1:3306)/mydb           # MySQL
mysh postgres://postgres:password@127.0.0.1:5432/mydb  # PostgreSQL
```

### Command Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--driver` | `mysql` | Database driver (`mysql` or `postgres`) |
| `-h` | `127.0.0.1` | Database host |
| `-P` | `3306`/`5432` | Database port (MySQL default: 3306, PostgreSQL default: 5432) |
| `-u` | `root` | Database user |
| `-p` | (empty) | Database password |
| `-d` | (empty) | Default database |
| `--config` | `~/.mysh.yaml` | Config file path |
| `--ssh-host` | — | SSH tunnel host (jump server) |
| `--ssh-port` | `22` | SSH tunnel port |
| `--ssh-user` | — | SSH tunnel user |
| `--ssh-key` | `~/.ssh/id_rsa` | SSH private key path |
| `--ssh-password` | — | SSH password (prefer key auth) |
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

### Transaction Support

mysh supports explicit transactions with dedicated connection binding:

```sql
mysh> BEGIN;
tx> INSERT INTO orders (user_id, amount) VALUES (1, 99.9);
tx> INSERT INTO order_items (order_id, product_id) VALUES (LAST_INSERT_ID(), 42);
tx> COMMIT;
mysh>
```

- After `BEGIN`, the prompt changes to `tx>` indicating an active transaction
- All statements within the transaction are executed on the same dedicated connection
- Use `\rollback` or `ROLLBACK` to abort and discard changes
- Auto-rollback on exit (`\q`, `Ctrl+D`, `Ctrl+C`) if a transaction is still open

### Key Bindings

| Key | Action |
|-----|--------|
| `Tab` / `Ctrl+Space` | Trigger auto-completion (snippets expand on single match) |
| `Up` / `Down` | Navigate history |
| `Ctrl+R` | Search history |
| `Ctrl+C` | Cancel query/watch or clear input |
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
| `\m` | Markdown table | `SELECT * FROM users\m` |

### Export Query Results

Export the last query result to a file:

```sql
mysh> SELECT * FROM users;
mysh> \export ~/users.csv
mysh> \export ~/users.json json
mysh> \export ~/users.md markdown
```

File format is auto-inferred from the extension (`.csv`, `.json`, `.md`), or specified explicitly.

### Live Watch Mode

Repeatedly execute a query at intervals for real-time monitoring:

```sql
mysh> \watch 2 SELECT COUNT(*) FROM processes;
mysh> \watch 5                    -- re-execute last query every 5 seconds
```

Press `Ctrl+C` to stop watching.

### SQL Aliases

Define shortcuts for frequently used queries:

```sql
mysh> \alias top10 SELECT * FROM users ORDER BY score DESC LIMIT 10
mysh> top10                        -- expands to the full SQL
```

Or configure in `~/.mysh.yaml`:

```yaml
aliases:
  top10: "SELECT * FROM users ORDER BY score DESC LIMIT 10"
  active: "SELECT * FROM users WHERE status = 'active'"
```

### Session Management

Save and switch between multiple database connections:

```sql
mysh> \session save prod           -- save current connection as "prod"
mysh> \session save staging        -- save another as "staging"
mysh> \session prod                -- switch to "prod"
mysh> \session                     -- list all saved sessions
mysh> \session del staging         -- delete a session
```

### Favorite Queries

Bookmark frequently used SQL queries:

```sql
mysh> SELECT * FROM users ORDER BY score DESC LIMIT 10;
mysh> \fav + top10 Top 10 users   -- save last query as "top10"
mysh> \fav top10                   -- run the saved favorite
mysh> \fav                         -- list all favorites
mysh> \fav show top10              -- view favorite SQL
mysh> \fav - top10                 -- delete a favorite
```

Or configure in `~/.mysh.yaml`:

```yaml
favorites:
  top_users:
    sql: "SELECT * FROM users ORDER BY score DESC LIMIT 10"
    description: "Top 10 users by score"
```

### SSH Tunnel

Connect to MySQL servers through an SSH jump host:

```bash
# CLI flags
mysh -h 10.0.0.5 --ssh-host jump.example.com --ssh-user deploy

# With SSH key
mysh -h db.internal --ssh-host bastion --ssh-user admin --ssh-key ~/.ssh/id_ed25519
```

Or configure in `~/.mysh.yaml`:

```yaml
connection:
  host: "10.0.0.5"          # MySQL host (as seen from the SSH server)
  port: 3306
  ssh:
    host: "jump.example.com"
    port: 22
    user: "deploy"
    key: "~/.ssh/id_ed25519"
```

## Configuration

Configuration file: `~/.mysh.yaml`

```yaml
# Connection defaults
connection:
  driver: "mysql"          # "mysql" (default) or "postgres"
  host: "127.0.0.1"
  port: 3306
  user: "root"
  password: ""
  database: ""

# UI settings
ui:
  prompt: "mysh> "
  multiline_prompt: "    -> "
  page_size: 0           # Result pagination rows (0 = no pagination, like mysql CLI)

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

# SQL aliases
aliases:
  top10: "SELECT * FROM users ORDER BY score DESC LIMIT 10"
  active: "SELECT * FROM users WHERE status = 'active'"

# Saved sessions
sessions:
  prod:
    host: "db.prod.example.com"
    port: 3306
    user: "admin"
    database: "myapp"
  staging:
    host: "db.staging.example.com"
    port: 3306
    user: "dev"
    database: "myapp_dev"

# Favorite queries
favorites:
  top_users:
    sql: "SELECT * FROM users ORDER BY score DESC LIMIT 10"
    description: "Top 10 users by score"
  active_sessions:
    sql: "SELECT * FROM information_schema.PROCESSLIST WHERE TIME > 5"
    description: "Long-running sessions"
```

Theme values use [lipgloss](https://github.com/charmbracelet/lipgloss) style syntax: `bold`, `italic`, `underline`, `dim`, plus color names (`red`, `green`, `yellow`, `blue`, `magenta`, `cyan`, `white`) or hex codes (`#ff0000`).

## Built-in Commands

| Command | Description |
|---------|-------------|
| `\help`, `\h`, `\?` | Show help |
| `\quit`, `\q` | Exit mysh |
| `\clear`, `\c` | Clear screen output |
| `\status`, `\s` | Show connection status |
| `\use <db>` | Switch database |
| `\refresh`, `\r` | Refresh metadata cache |
| `\format [type]` | Set/show output format (table, vertical, json, markdown) |
| `\history [pattern]` | Search/show command history |
| `\connect <dsn>` | Connect to a database (user@host:port/db or just db) |
| `\reconnect` | Reconnect to the current server |
| `\desc <t> [mode]` | Describe table (columns, full, indexes, create) |
| `\source <file>` | Execute SQL from file |
| `\edit`, `\e` | Open external editor to edit/execute SQL |
| `\pipe`, `\| <cmd>` | Pipe last query result to a system command |
| `\copy <what>` | Copy to clipboard (result, query, sql) |
| `\timing` | Toggle query execution time display |
| `\safe-updates [on\|off]` | Toggle safe-updates mode (block UPDATE/DELETE without WHERE/LIMIT) |
| `\slow [seconds]` | Set/show slow query warning threshold (0 = disabled) |
| `\export <file> [fmt]` | Export last query result to file (csv, json, markdown) |
| `\watch [sec] [SQL]` | Re-execute query at intervals (default 5s, Ctrl+C stop) |
| `\alias [name sql]` | Show/set command aliases |
| `\unalias <name>` | Remove temporary alias |
| `\session` | List saved sessions |
| `\session <name>` | Switch to saved session |
| `\session save <name>` | Save current connection as session |
| `\session del <name>` | Delete a saved session |
| `\fav`, `\favorites` | List favorite queries |
| `\fav <name>` | Execute a saved favorite |
| `\fav + <name> [desc]` | Save last query as favorite |
| `\fav - <name>` | Delete a favorite |
| `\fav show <name>` | Show favorite SQL |
| `\mouse` | Toggle mouse mode |
| `\cd [dir]` | Change/show working directory (for \source, \sys) |
| `\sys`, `\! <cmd>` | Execute a system command |
| `\rollback` | Rollback current transaction |

### Format Suffixes

Append to any SQL statement to override the output format for that query:

| Suffix | Format | Example |
|--------|--------|---------|
| `\G` | Vertical (one column per line) | `SELECT * FROM users\G` |
| `\j` | JSON array | `SELECT * FROM users\j` |
| `\m` | Markdown table | `SELECT * FROM users\m` |

## Auto-Completion Context

mysh provides context-aware suggestions based on your cursor position:

| Context | Suggestions |
|---------|-------------|
| Statement start | SQL keywords, built-in commands, SQL snippets |
| After `SELECT` | Column names, functions, `DISTINCT`, `*` |
| After `FROM` | Table names, database names, `WHERE`/`JOIN` keywords |
| After `WHERE` / `AND` / `OR` | Column names, operators, functions |
| After `JOIN` | Table names, `ON`/`USING` |
| After `table.` | Column names of that table, `*` |
| After `SET` | Database names, `NAMES`/`AUTOCOMMIT` |
| After `ORDER BY` / `GROUP BY` | Column names, `ASC`/`DESC` |

### Built-in SQL Snippets

When auto-completion has few matches, snippet templates are suggested. Press `Tab` on a single match to expand:

| Trigger | Template |
|---------|----------|
| `CREATE TABLE` | `CREATE TABLE ... (id INT PRIMARY KEY, ...)` |
| `ALTER TABLE` | `ALTER TABLE ... ADD COLUMN ...` |
| `INSERT INTO` | `INSERT INTO ... (...) VALUES (...)` |
| `UPDATE` | `UPDATE ... SET ... WHERE ...` |
| `DELETE FROM` | `DELETE FROM ... WHERE ...` |
| `CREATE INDEX` | `CREATE INDEX ... ON ... (...)` |
| `CREATE USER` | `CREATE USER ... IDENTIFIED BY ...` |
| `GRANT` | `GRANT ... ON ... TO ...` |
| `SELECT INTO` | `SELECT ... INTO OUTFILE ...` |

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
│  │ MySQL/PG │ │ Schema    │ │ File   │ │
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
├── connection/          # Database connection pool & adapter
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

[Apache License 2.0](LICENSE)
