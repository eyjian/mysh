# OpenSpec: Phase 1 - UX Enhancement Design

## Metadata
- **ID**: mysh-phase1-ux-enhancement
- **Version**: 1.0
- **Status**: implemented
- **Author**: AI Assistant
- **Created**: 2026-05-05

## Overview
Phase 1 of mysh UX enhancement, focusing on 4 high-value features that significantly improve daily usage efficiency:
1. Ctrl+R Incremental History Search
2. Secure Password Input (terminal.ReadPassword)
3. Panic Recovery (terminal safety)
4. Format Suffix Shortcuts (\G, \j, \m)

## Tasks

---

### Task 1: Ctrl+R Incremental History Search

**Goal**: Allow users to interactively search command history by typing a substring, matching entries in real-time, similar to bash's Ctrl+R behavior.

**Design**:

#### New Model State Fields (tui/model.go)
```go
// History search state
historySearch  bool   // true when in Ctrl+R search mode
searchQuery    string // current search query string
searchResult   string // currently matched history entry
searchResultIdx int   // index in filtered results
```

#### State Machine
- **Normal mode** → Ctrl+R → **Search mode**
- **Search mode**:
  - Typing characters: append to `searchQuery`, auto-filter history, show best match
  - Ctrl+R again: cycle to next match (older)
  - Ctrl+S / Shift+Ctrl+R: cycle to previous match (newer)
  - Enter: accept match, populate editor, return to normal mode
  - Escape: cancel search, return to normal mode
  - Backspace: remove last char from query, re-filter
  - Ctrl+C: cancel search, return to normal mode

#### UI Rendering (Search Mode)
In search mode, the prompt line changes to:
```
(reverse-i-search)`query': matched command text
```
The matched portion of the command is highlighted (bold/cyan).

#### History Package Changes (history/history.go)
Add a new method:
```go
// SearchIncremental returns all entries matching the pattern (case-insensitive),
// ordered from most recent to oldest. Used by Ctrl+R incremental search.
func (h *History) SearchIncremental(pattern string) []string
```
This is similar to the existing `Search()` but returns results in reverse chronological order (most recent first) for Ctrl+R cycling.

#### Implementation Files
- `tui/model.go`: Add search state fields, handle Ctrl+R in handleKey/handleCtrlRune, render search UI in View()
- `history/history.go`: Add SearchIncremental() method

---

### Task 2: Secure Password Input

**Goal**: When `-p` is specified without an argument, use `golang.org/x/term` to read the password without echo, instead of showing it in plaintext via `fmt.Fscanln`.

**Design**:

#### Dependency Addition
Add `golang.org/x/term` to go.mod.

#### main.go Changes
Replace the current `-p` no-argument handling:
```go
// Before:
fmt.Fprintf(os.Stderr, "Enter password: ")
var pw string
fmt.Fscanln(os.Stdin, &pw)

// After:
fmt.Fprintf(os.Stderr, "Enter password: ")
pw, err := term.ReadPassword(int(os.Stdin.Fd()))
if err != nil {
    fmt.Fprintf(os.Stderr, "\nError reading password: %s\n", err)
    os.Exit(1)
}
fmt.Fprintln(os.Stderr) // newline after hidden input
flagPassword = string(pw)
```

#### Implementation Files
- `main.go`: Import `golang.org/x/term`, replace password reading logic
- `go.mod`: Add `golang.org/x/term` dependency

---

### Task 3: Panic Recovery

**Goal**: If the TUI panics, restore the terminal to a usable state instead of leaving it in raw/broken mode.

**Design**:

#### Approach
Wrap the TUI program entry point with a deferred recover that:
1. Restores terminal state (disable raw mode, show cursor, reset colors)
2. Prints the panic stack trace
3. Ensures cleanup (history save, connection close) still runs

#### main.go Changes
```go
// Before running the TUI, set up panic recovery
defer func() {
    if r := recover(); r != nil {
        // Restore terminal state
        fmt.Fprintf(os.Stderr, "\n\033[?25h\033[0m") // show cursor, reset attributes
        fmt.Fprintf(os.Stderr, "mysh crashed: %v\n", r)
        // Still run cleanup
        cleanup(deps)
        os.Exit(2)
    }
}()
```

Note: The `tea.Program.Run()` method already handles restoring the terminal on normal exit. The panic recovery is a safety net for unexpected crashes within the Update/View methods.

#### Implementation Files
- `main.go`: Add deferred panic recovery before `program.Run()`

---

### Task 4: Format Suffix Shortcuts (\G, \j, \m)

**Goal**: Extend the existing `\G` (vertical output) suffix to also support `\j` (JSON) and `\m` (markdown), allowing per-query format override without changing the global format setting.

**Design**:

#### New Constants (output/output.go)
No new constants needed; existing Format enum already has FormatJSON and FormatMarkdown.

#### Suffix Detection Logic (tui/model.go)
Replace the current `\G`-only detection in `handleEnter()` with a generalized format suffix parser:
```go
// Check for format suffixes: \G (vertical), \j (json), \m (markdown)
var formatOverride output.Format
trimmed, formatOverride = parseFormatSuffix(trimmed)
```

New helper function:
```go
// parseFormatSuffix checks if the input ends with a format suffix
// and returns the trimmed input and the format override.
// Returns the original input and FormatTable (no override) if no suffix found.
func parseFormatSuffix(input string) (string, output.Format) {
    if strings.HasSuffix(input, "\\G") {
        return strings.TrimSpace(strings.TrimSuffix(input, "\\G")), output.FormatVertical
    }
    if strings.HasSuffix(input, "\\j") {
        return strings.TrimSpace(strings.TrimSuffix(input, "\\j")), output.FormatJSON
    }
    if strings.HasSuffix(input, "\\m") {
        return strings.TrimSpace(strings.TrimSuffix(input, "\\m")), output.FormatMarkdown
    }
    return input, output.FormatTable
}
```

#### Format Override Application
In `executeInput()`, replace the `useVertical bool` parameter with `formatOverride output.Format`:
```go
func (m Model) executeInput(input string, formatOverride output.Format) (tea.Model, tea.Cmd) {
    // ...
    outFmt := m.deps.Formatter.CurrentFormat()
    if formatOverride != output.FormatTable {
        outFmt = formatOverride
    } else if m.autoVerticalOutput && outFmt == output.FormatTable && m.width > 0 {
        if output.CalcTableWidth(result) > m.width {
            outFmt = output.FormatVertical
        }
    }
    // ...
}
```

Same change in `execSourceFile()` for `\G`, `\j`, `\m` suffix handling.

#### History Entry
When a format suffix is used, preserve it in the history entry:
```go
historyEntry := input
suffix := formatSuffixString(formatOverride) // "\G", "\j", "\m", or ""
if suffix != "" {
    historyEntry = input + " " + suffix
}
```

#### Help Text Update
Add format suffix documentation to `helpText()`:
```
Format suffixes (append to SQL):
  \G    Display result in vertical format
  \j    Display result in JSON format
  \m    Display result in Markdown format
```

#### Implementation Files
- `tui/model.go`: Replace `useVertical bool` with `formatOverride output.Format`, add `parseFormatSuffix()`, update `handleEnter()`, `executeInput()`, `execSourceFile()`, and `helpText()`
- `output/output.go`: No changes needed

---

## Implementation Order
1. **Task 2: Secure Password Input** - Smallest change, isolated to main.go
2. **Task 3: Panic Recovery** - Small change, isolated to main.go
3. **Task 4: Format Suffix Shortcuts** - Refactors existing \G logic, medium complexity
4. **Task 1: Ctrl+R History Search** - Most complex, new UI mode

## Testing Strategy
- Task 1: Manual testing - Ctrl+R, type query, cycle matches, accept/cancel
- Task 2: Manual testing - `mysh -u root -p` without password arg, verify no echo
- Task 3: Add a test panic trigger, verify terminal is restored
- Task 4: Manual testing - `SELECT 1\G`, `SELECT 1\j`, `SELECT 1\m`
- All tasks: `go build ./... && go test ./...` must pass

## Risk Assessment
- **Task 1**: Medium risk - new UI mode could interfere with existing key handling. Mitigation: search mode is isolated, only active when `historySearch == true`.
- **Task 2**: Low risk - well-tested stdlib extension, minimal code change.
- **Task 3**: Low risk - deferred recover is standard Go pattern.
- **Task 4**: Low risk - extending existing \G pattern, well-understood.
