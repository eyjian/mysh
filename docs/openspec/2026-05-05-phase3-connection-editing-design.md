# OpenSpec: Phase 3 - Connection & Editing Enhancement

## Metadata
- **ID**: mysh-phase3-connection-editing
- **Version**: 1.0
- **Status**: implemented
- **Author**: AI Assistant
- **Created**: 2026-05-05

## Overview
Phase 3 of mysh UX enhancement, focusing on 5 features that improve connection management and text editing experience:
1. Reconnection on Connection Loss (auto-retry + manual `\reconnect`)
2. Editor Word Navigation (Ctrl+Left/Right, Alt+Left/Right)
3. Editor Kill Word (Ctrl+W improvement, Alt+D kill-word-forward)
4. `\connect` Command Implementation (switch connection without restart)
5. Connection Health Indicator (prompt shows connection status)

## Tasks

---

### Task 1: Reconnection on Connection Loss

**Goal**: Automatically detect and recover from lost MySQL connections. When a query fails due to connection loss, auto-retry with reconnection instead of showing a cryptic error.

**Design**:

#### Executor Enhancement (executor/executor.go)

Add retry logic in `Execute()` for connection errors:

```go
// IsConnectionError checks if an error is caused by a lost connection.
func IsConnectionError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "invalid connection") ||
		strings.Contains(msg, "bad connection") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "server has gone away") ||
		strings.Contains(msg, "connect: connection refused") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "driver: bad conn")
}
```

In `Executor.Execute()`, wrap the execution with retry-on-connection-error:

```go
func (e *Executor) Execute(ctx context.Context, query string) (*QueryResult, error) {
	result, err := e.executeOnce(ctx, query)
	if err == nil || !IsConnectionError(err) {
		return result, err
	}

	// Connection error — try to reconnect once
	if e.pool != nil {
		if reconnErr := e.pool.Reconnect(); reconnErr != nil {
			return nil, fmt.Errorf("connection lost and reconnect failed: %w", reconnErr)
		}
		// Retry the query after reconnection
		result, err = e.executeOnce(ctx, query)
		if err == nil {
			result.Warning = "Reconnected to server"
		}
	}
	return result, err
}
```

#### QueryResult Enhancement (executor/executor.go)

Add a `Warning` field to `QueryResult`:

```go
type QueryResult struct {
	Columns      []string
	Rows         [][]interface{}
	Duration     time.Duration
	AffectedRows int64
	IsQuery      bool
	Error        error
	Warning      string // non-critical message (e.g., "Reconnected to server")
}
```

#### Formatter Warning Output (output/output.go)

Display the warning after the result:

```go
// After the normal result output
if result.Warning != "" {
	fmt.Fprintf(f.writer, "\033[33mWarning: %s\033[0m\n", result.Warning)
}
```

#### Manual Reconnect Command (tui/model.go)

Add `\reconnect` backslash command:

```go
case "\\reconnect":
	if m.deps.Pool != nil {
		m.addOutput("Reconnecting...")
		if err := m.deps.Pool.Reconnect(); err != nil {
			m.addOutput(fmt.Sprintf("Reconnect failed: %s", err))
		} else {
			m.addOutput("Reconnected successfully.")
		}
	}
```

**Complexity**: Medium  
**Files Changed**: `executor/executor.go`, `output/output.go`, `tui/model.go`

---

### Task 2: Editor Word Navigation

**Goal**: Support Ctrl+Left/Right (and Alt+Left/Right) to jump by word boundaries, like standard shell editors.

**Design**:

#### New Editor Methods (editor/editor.go)

```go
// MoveWordLeft moves the cursor to the beginning of the previous word.
func (e *Editor) MoveWordLeft() {
	if e.cursor == 0 {
		return
	}
	pos := e.cursor - 1
	// Skip whitespace
	for pos > 0 && isWhitespace(e.text[pos]) {
		pos--
	}
	// Skip word characters
	for pos > 0 && !isWhitespace(e.text[pos-1]) {
		pos--
	}
	e.cursor = pos
}

// MoveWordRight moves the cursor to the beginning of the next word.
func (e *Editor) MoveWordRight() {
	if e.cursor >= len(e.text) {
		return
	}
	pos := e.cursor
	// Skip current word characters
	for pos < len(e.text) && !isWhitespace(e.text[pos]) {
		pos++
	}
	// Skip whitespace
	for pos < len(e.text) && isWhitespace(e.text[pos]) {
		pos++
	}
	e.cursor = pos
}

func isWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n'
}
```

#### Key Binding (tui/model.go)

Bubble Tea doesn't have `KeyCtrlLeft`/`KeyCtrlRight` built-in, so we need to handle raw escape sequences. Most terminals send `Esc[1;5D` for Ctrl+Left and `Esc[1;5C` for Ctrl+Right. We'll handle Alt+Left/Right similarly (`Esc[1;3D` / `Esc[1;3C`).

However, since Bubble Tea abstracts key events, the simpler approach is to handle `Alt+b` (word-back) and `Alt+f` (word-forward) which are more universally supported:

```go
// In handleCtrlRune or handleKey:
case 2: // Ctrl+B — move word left (Alt+B often sends Esc+b)
	// Actually, we need to detect Alt+key sequences
```

Better approach: Add Ctrl+Left/Right via escape sequence detection in the key handling:

```go
// In handleKey, add these key types from bubbletea:
// tea.KeyCtrlLeft and tea.KeyCtrlRight if available,
// otherwise use Alt+b and Alt+f as alternatives.

// Alt+b = word back, Alt+f = word forward
// These come through as tea.KeyRunes with the Alt modifier
```

Since Bubble Tea v0.25+ supports `tea.KeyCtrlLeft` and `tea.KeyCtrlRight`, we'll use those:

```go
case tea.KeyCtrlLeft:
	m.ed.MoveWordLeft()
	m.showComp = false
	return m, nil

case tea.KeyCtrlRight:
	m.ed.MoveWordRight()
	m.showComp = false
	return m, nil
```

If the tea.KeyCtrlLeft/Right constants don't exist in the version being used, fallback to handling Alt+b (0x02 in some terminals) and Alt+f.

**Complexity**: Low  
**Files Changed**: `editor/editor.go`, `tui/model.go`

---

### Task 3: Editor Kill Word

**Goal**: Add Ctrl+W kill-word-backward (improved from current basic implementation) and Alt+D kill-word-forward, matching readline/emacs conventions.

**Design**:

#### New Editor Methods (editor/editor.go)

```go
// KillWordBackward deletes the word before the cursor.
// Unlike Backspace which deletes one character, this deletes
// the entire word (whitespace + word) before the cursor.
func (e *Editor) KillWordBackward() {
	if e.cursor == 0 {
		return
	}
	pos := e.cursor - 1
	// Skip whitespace
	for pos > 0 && isWhitespace(e.text[pos]) {
		pos--
	}
	// Skip word characters
	for pos > 0 && !isWhitespace(e.text[pos-1]) {
		pos--
	}
	e.text = append(e.text[:pos], e.text[e.cursor:]...)
	e.cursor = pos
}

// KillWordForward deletes the word after the cursor.
func (e *Editor) KillWordForward() {
	if e.cursor >= len(e.text) {
		return
	}
	pos := e.cursor
	// Skip word characters
	for pos < len(e.text) && !isWhitespace(e.text[pos]) {
		pos++
	}
	// Skip whitespace
	for pos < len(e.text) && isWhitespace(e.text[pos]) {
		pos++
	}
	e.text = append(e.text[:e.cursor], e.text[pos:]...)
}
```

#### Key Binding (tui/model.go)

Replace the current Ctrl+W implementation with the new `KillWordBackward`:

```go
case 23: // Ctrl+W — kill word backward
	m.ed.KillWordBackward()
	m.showComp = false
```

For Alt+D (kill-word-forward), we can handle it via the escape sequence. Alt+D sends `Esc+d` which appears as `\x1bd`. In Bubble Tea this may come through as a KeyRunes with specific encoding. We'll handle it as a control character:

```go
case 4: // Ctrl+D — currently exits on empty, but also used as Alt+D trigger
	// Actually, Alt+D comes through differently depending on terminal
	// We'll add it as a special case in handleKey
```

The cleanest approach: Detect Alt+D in the `handleCtrlRune` or add a new handler for Alt sequences. Since Bubble Tea sends Alt+key as `KeyMsg` with `Type: tea.KeyRunes` and `Alt: true` (in newer versions), we check the `Alt` field:

```go
// In handleKey, for KeyRunes:
case tea.KeyRunes:
	if len(msg.Runes) == 1 && msg.Runes[0] < 32 {
		return m.handleCtrlRune(msg.Runes[0])
	}
	// Check for Alt+d (kill word forward)
	if msg.Alt && len(msg.Runes) == 1 && msg.Runes[0] == 'd' {
		m.ed.KillWordForward()
		m.showComp = false
		return m, nil
	}
	m.ed.Insert(string(msg.Runes))
	// ...
```

**Note**: Bubble Tea's `KeyMsg` has an `Alt` field in recent versions. If not available, we'll need to handle the raw escape sequence manually.

**Complexity**: Low  
**Files Changed**: `editor/editor.go`, `tui/model.go`

---

### Task 4: `\connect` Command Implementation

**Goal**: Implement the `\connect` command to switch to a different MySQL server/database without restarting mysh. Currently it shows "not yet supported".

**Design**:

#### Command Syntax

```
\connect <host> <port> <user> <database>    # prompt for password
\connect <user>@<host>:<port>/<database>     # DSN format
\connect <user>@<host>/<database>            # default port
\connect <database>                          # reconnect to same server, different db
```

For simplicity, we'll support:

```
\connect <host> <port> <user> <database>     # 4 positional args
\connect <user>@<host>:<port>/<database>     # DSN format
\connect <database>                          # switch database on same server
```

#### Implementation (tui/model.go)

Replace the current `\connect` stub:

```go
case "\\connect":
	if len(parts) < 2 {
		m.addOutput("Usage: \\connect <dsn> | \\connect <host> <port> <user> <database>")
		m.addOutput("  DSN format: user@host:port/database")
		m.addOutput("  Shorthand:  database (reconnect to same server)")
		return m, nil
	}

	// Parse arguments
	var newCfg config.ConnectionConfig
	if len(parts) == 2 && strings.Contains(parts[1], "@") {
		// DSN format: user@host:port/database
		parsed, parseErr := parseConnectDSN(parts[1], m.deps.Config.Connection)
		if parseErr != nil {
			m.addOutput(fmt.Sprintf("Invalid DSN: %s", parseErr))
			return m, nil
		}
		newCfg = parsed
	} else if len(parts) == 2 {
		// Just a database name — reconnect to same server
		newCfg = m.deps.Config.Connection
		newCfg.Database = parts[1]
	} else if len(parts) >= 5 {
		// host port user database
		newCfg = m.deps.Config.Connection // inherit charset etc.
		newCfg.Host = parts[1]
		fmt.Sscanf(parts[2], "%d", &newCfg.Port)
		newCfg.User = parts[3]
		newCfg.Database = parts[4]
		// Password: prompt securely
		// (In TUI mode, we can't use term.ReadPassword easily,
		//  so we'll use a special input mode or accept it from config)
	} else {
		m.addOutput("Usage: \\connect <dsn> | \\connect <host> <port> <user> <database>")
		return m, nil
	}

	// Attempt reconnection
	m.addOutput(fmt.Sprintf("Connecting to %s@%s:%d/%s ...", newCfg.User, newCfg.Host, newCfg.Port, newCfg.Database))
	if err := m.deps.Pool.Reset(&newCfg); err != nil {
		m.addOutput(fmt.Sprintf("Connection failed: %s", err))
		return m, nil
	}

	// Update config
	m.deps.Config.Connection = newCfg

	// Refresh metadata
	if m.deps.Meta != nil {
		m.deps.Meta.MarkDirty()
		go m.deps.Meta.Refresh()
	}

	m.addOutput("Connection established.")
```

#### DSN Parser (tui/model.go)

```go
// parseConnectDSN parses a DSN string in the format user@host:port/database.
func parseConnectDSN(dsn string, current config.ConnectionConfig) (config.ConnectionConfig, error) {
	cfg := current // inherit current settings (charset, password)

	// Split user@hostpart
	atIdx := strings.Index(dsn, "@")
	if atIdx < 0 {
		return cfg, fmt.Errorf("missing '@' in DSN")
	}
	cfg.User = dsn[:atIdx]
	hostPart := dsn[atIdx+1:]

	// Split host:port/database
	slashIdx := strings.Index(hostPart, "/")
	if slashIdx >= 0 {
		cfg.Database = hostPart[slashIdx+1:]
		hostPart = hostPart[:slashIdx]
	}

	// Split host:port
	colonIdx := strings.LastIndex(hostPart, ":")
	if colonIdx >= 0 {
		cfg.Host = hostPart[:colonIdx]
		fmt.Sscanf(hostPart[colonIdx+1:], "%d", &cfg.Port)
	} else {
		cfg.Host = hostPart
		cfg.Port = 3306
	}

	return cfg, nil
}
```

**Complexity**: Medium  
**Files Changed**: `tui/model.go`

---

### Task 5: Connection Health Indicator

**Goal**: Show connection status in the prompt or status bar, so users can immediately see if the connection is alive or lost.

**Design**:

#### Prompt Enhancement (tui/model.go)

Add a small colored indicator before the prompt:
- 🟢 (green dot) = connected
- 🔴 (red dot) = disconnected
- 🟡 (yellow dot) = reconnecting

Since emoji may not render well in all terminals, use colored text instead:

```
● mysh>      # green ● = connected
● mysh>      # red ● = disconnected
● mysh>      # yellow ● = reconnecting
```

Using ANSI colors:
- Connected: `\033[32m●\033[0m` (green)
- Disconnected: `\033[31m●\033[0m` (red)

#### Periodic Health Check

Add a background health check that runs every 30 seconds:

```go
// healthCheckMsg is sent periodically to check connection status.
type healthCheckMsg time.Time

// In Init():
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		blinkCmd(),
		healthCheckCmd(),
	)
}

func healthCheckCmd() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg {
		return healthCheckMsg(t)
	})
}
```

In `Update()`:

```go
case healthCheckMsg:
	if m.deps.Pool != nil {
		connected := m.deps.Pool.IsConnected()
		m.connected = connected
	}
	return m, healthCheckCmd()
```

#### New Model State (tui/model.go)

```go
connected bool // true = last health check was successful
```

Initialize to `true` in `NewModel()`.

#### Prompt Rendering (tui/model.go)

In the `View()` method, prepend the health indicator to the prompt:

```go
// Build prompt prefix with connection indicator
var promptPrefix string
if m.connected {
	promptPrefix = "\033[32m●\033[0m " // green dot
} else {
	promptPrefix = "\033[31m●\033[0m " // red dot
}

currentPrompt := promptPrefix + m.prompt
sb.WriteString(m.promptStyle.Render(currentPrompt))
```

Also update the health status on query execution results — if a query succeeds, mark as connected; if it fails with a connection error, mark as disconnected.

#### Update on Query Result (tui/model.go)

In `displayQueryResult`:

```go
func (m *Model) displayQueryResult(result *executor.QueryResult, err error, formatOverride output.Format) {
	// Update connection health based on result
	if err != nil && executor.IsConnectionError(err) {
		m.connected = false
	} else if err == nil {
		m.connected = true
	}
	// ... rest of the method
}
```

**Complexity**: Low  
**Files Changed**: `tui/model.go`, `executor/executor.go` (add `IsConnectionError`)

---

## Implementation Order

1. **Task 1** — Reconnection (most impactful, improves reliability)
2. **Task 5** — Health Indicator (depends on Task 1's `IsConnectionError`)
3. **Task 2** — Word Navigation (pure editor enhancement)
4. **Task 3** — Kill Word (pure editor enhancement)
5. **Task 4** — `\connect` Command (most complex, depends on pool.Reset)

## Summary

| Task | Feature | Complexity | Files Changed |
|------|---------|-----------|---------------|
| 1 | Auto-reconnection on connection loss | Medium | executor/executor.go, output/output.go, tui/model.go |
| 2 | Editor word navigation (Ctrl+Left/Right) | Low | editor/editor.go, tui/model.go |
| 3 | Editor kill word (Ctrl+W, Alt+D) | Low | editor/editor.go, tui/model.go |
| 4 | `\connect` command | Medium | tui/model.go |
| 5 | Connection health indicator | Low | tui/model.go, executor/executor.go |
