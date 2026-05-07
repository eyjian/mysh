# OpenSpec: Phase 5 - Favorites & SSH Tunnel

## Metadata
- **ID**: mysh-phase5-favorites-ssh
- **Version**: 1.0
- **Status**: implemented
- **Author**: AI Assistant
- **Created**: 2026-05-05

## Overview
Phase 5 adds two independent feature modules:
1. **`\favorites`** — Bookmark SQL queries with names/descriptions, list/run/manage interactively
2. **SSH Tunnel** — Connect to MySQL through an SSH jump host via local port forwarding

## Tasks

---

### Task 1: `\favorites` Command

**Goal**: Allow users to save, list, run, and delete favorite SQL queries. Unlike aliases (short substitutions), favorites store full queries with descriptions and support interactive browsing.

**Design**:

#### Command Syntax
```
\fav, \favorites                    List all favorites
\fav <name>                         Execute a saved favorite
\fav + <name> [description]         Save last query as favorite
\fav - <name>                       Delete a favorite
\fav show <name>                    Show the SQL of a favorite
```

#### Config Format (~/.mysh.yaml)
```yaml
favorites:
  top_users:
    sql: "SELECT * FROM users ORDER BY score DESC LIMIT 10"
    description: "Top 10 users by score"
  active_sessions:
    sql: "SELECT * FROM information_schema.PROCESSLIST WHERE TIME > 5"
    description: "Long-running sessions"
```

#### Config Structure (config/config.go)
```go
type FavoriteConfig struct {
    SQL         string `mapstructure:"sql"`
    Description string `mapstructure:"description"`
}

// Add to Config struct:
Favorites map[string]FavoriteConfig `mapstructure:"favorites"`
```

#### Model State (tui/model.go)
```go
tempFavorites map[string]config.FavoriteConfig // session-only favorites
```

#### Implementation
- Add `\fav` / `\favorites` command handling in `handleBackslashCommand()`
- `listFavorites()` — display all favorites with name, description, and SQL preview
- `addFavorite(name, description)` — save `m.lastQuery` as favorite
- `runFavorite(name)` — execute the saved SQL
- `deleteFavorite(name)` — remove a favorite
- `showFavorite(name)` — display full SQL
- Persist additions/deletions via `config.Save()`

---

### Task 2: SSH Tunnel

**Goal**: Support connecting to MySQL servers that are only reachable through an SSH jump host. Establish a local port forward so the MySQL connection goes through the SSH tunnel transparently.

**Design**:

#### CLI Flags
```
--ssh-host <host>       SSH jump host
--ssh-port <port>       SSH port (default 22)
--ssh-user <user>       SSH username
--ssh-key <path>        SSH private key (default ~/.ssh/id_rsa)
--ssh-password <pass>   SSH password (less secure, prefer key)
```

#### Config Format (~/.mysh.yaml)
```yaml
connection:
  host: "10.0.0.5"        # MySQL host as seen from SSH server
  port: 3306
  ssh:
    host: "jump.example.com"
    port: 22
    user: "deploy"
    key: "~/.ssh/id_rsa"
    password: ""           # optional, prefer key auth
```

#### Config Structure (config/config.go)
```go
type SSHConfig struct {
    Host     string `mapstructure:"host"`
    Port     int    `mapstructure:"port"`
    User     string `mapstructure:"user"`
    Key      string `mapstructure:"key"`
    Password string `mapstructure:"password"`
}

// Add to ConnectionConfig:
SSH SSHConfig `mapstructure:"ssh"`
```

#### Tunnel Package (ssh/tunnel.go)
New package `ssh/` with:
- `Tunnel` struct wrapping an SSH client + local listener
- `NewTunnel(cfg *config.SSHConfig, remoteHost string, remotePort int) (*Tunnel, error)`
- Uses `golang.org/x/crypto/ssh` for SSH connection
- Picks a random available local port for forwarding
- `LocalAddr() string` returns `127.0.0.1:localPort`
- `Close()` shuts down the tunnel

#### Integration (main.go)
1. If SSH config is present, establish tunnel before MySQL connection
2. Replace MySQL host/port with tunnel's local address
3. Close tunnel on cleanup

---

## Implementation Order
1. **Task 1**: `\favorites` — independent, no new dependencies
2. **Task 2**: SSH Tunnel — needs `golang.org/x/crypto` dependency

## Summary

| Task | Feature | Complexity | Files Changed |
|------|---------|-----------|---------------|
| 1 | `\favorites` command | Medium | config/config.go, tui/model.go |
| 2 | SSH Tunnel | Medium-High | ssh/tunnel.go (new), config/config.go, main.go |
