# OpenSpec: Phase 2 - Output Experience Enhancement

## Metadata
- **ID**: mysh-phase2-output-experience
- **Version**: 1.0
- **Status**: draft
- **Author**: AI Assistant
- **Created**: 2026-05-05

## Overview
Phase 2 of mysh UX enhancement, focusing on 5 features that improve output readability and query execution feedback:
1. NULL Value Coloring
2. DML Result Highlighting
3. Result Pagination (using existing `page_size` config)
4. Query Timer (real-time elapsed counter)
5. Query Progress Animation ("Executing..." spinner)

## Tasks

---

### Task 1: NULL Value Coloring

**Goal**: Display NULL values in a distinct color (dim/italic) across all output formats, making them visually distinguishable from the string "NULL" and empty strings.

**Design**:

#### New Helper Function (output/output.go)
Add ANSI-colored NULL rendering:
```go
// nullStyle renders NULL with ANSI styling for visual distinction.
const nullANSI = "\033[3;90m" // dim italic
const resetANSI = "\033[0m"

func formatValueStyled(v interface{}) string {
    if v == nil {
        return nullANSI + "NULL" + resetANSI
    }
    switch val := v.(type) {
    case string:
        return val
    case []byte:
        return string(val)
    default:
        return fmt.Sprintf("%v", val)
    }
}
```

#### Changes to writeTable() (output/output.go)
Replace `formatValue(val)` with `formatValueStyled(val)` for row data cells.
Keep `formatValue()` for width calculation (unstyled, to measure correctly).

```go
// Width calculation still uses formatValue() (no ANSI codes)
strRows[i][j] = formatValue(val)

// Display uses formatValueStyled()
displayRows[i][j] = formatValueStyled(val)
```

Note: Need separate string slices — one for width calculation (plain), one for display (styled).

#### Changes to writeVertical() (output/output.go)
Replace the inline NULL check with `formatValueStyled()`:
```go
// Before:
val := "NULL"
if i < len(row) && row[i] != nil {
    val = formatValue(row[i])
}

// After:
var val string
if i < len(row) {
    val = formatValueStyled(row[i])
} else {
    val = formatValueStyled(nil)
}
```

#### Changes to writeJSON() (output/output.go)
No change needed — JSON output should represent NULL as `null` (unstyled), which `convertForJSON` already handles.

#### Changes to writeMarkdown() (output/output.go)
Same as `writeTable()` — use `formatValueStyled()` for display rows, `formatValue()` for width calculation.

#### Implementation Files
- `output/output.go`: Add `formatValueStyled()`, update `writeTable()`, `writeVertical()`, `writeMarkdown()`

---

### Task 2: DML Result Highlighting

**Goal**: Colorize DML result messages to make affected row counts and status more prominent. Apply green for success, yellow for warnings.

**Design**:

#### New Color Constants (output/output.go)
```go
const (
    greenANSI  = "\033[32m"  // success / affected rows
    yellowANSI = "\033[33m"  // warning
    boldANSI   = "\033[1m"   // emphasis
    cyanANSI   = "\033[36m"  // timing info
)
```

#### Changes to formatDMLResult() (output/output.go)
```go
func formatDMLResult(result *executor.QueryResult) string {
    if result.AffectedRows >= 0 {
        return fmt.Sprintf("%sQuery OK%s, %s%d rows affected%s %s(%s)%s",
            greenANSI+boldANSI, resetANSI,
            greenANSI, result.AffectedRows, resetANSI,
            cyanANSI, executor.FormatDuration(result.Duration), resetANSI)
    }
    return fmt.Sprintf("%sQuery OK%s %s(%s)%s",
        greenANSI+boldANSI, resetANSI,
        cyanANSI, executor.FormatDuration(result.Duration), resetANSI)
}
```

#### Changes to Row Count Lines (output/output.go)
Update the "N rows in set (time)" line in all format writers:
```go
// In writeTable(), writeVertical(), writeJSON(), writeMarkdown():
count := len(result.Rows)
rowWord := "row"
if count != 1 {
    rowWord = "rows"
}
fmt.Fprintf(f.writer, "%s%d %s in set%s %s(%s)%s\n",
    greenANSI, count, rowWord, resetANSI,
    cyanANSI, executor.FormatDuration(result.Duration), resetANSI)
```

#### Changes to Error Output (output/output.go)
Update `WriteResult()` error handling:
```go
if result.Error != nil {
    _, err := fmt.Fprintf(f.writer, "%sERROR:%s %s\n",
        "\033[31;1m", resetANSI, result.Error.Error())
    return err
}
```

#### Implementation Files
- `output/output.go`: Update `formatDMLResult()`, all format writers' row count lines, error output

---

### Task 3: Result Pagination

**Goal**: When a SELECT result has more rows than `page_size`, display results one page at a time with a `-- More --` prompt. User can press Space/Enter for next page, 'q' to discard remaining rows. The `page_size` config (default 20) is already defined but unused.

**Design**:

#### New Model State Fields (tui/model.go)
```go
// Pagination state
pagedResult    *executor.QueryResult  // result being paginated
pagedFormat    output.Format          // format for paged result
pagedPage      int                    // current page number (0-based)
pagedTotal     int                    // total pages
```

#### Pagination Flow
1. After query execution, check if result row count > `m.deps.Config.UI.PageSize`
2. If yes, enter pagination mode instead of displaying all rows at once
3. Render only the current page's rows
4. Show `-- More (page X/Y, Enter=next, q=show all) --` at the bottom
5. Key handling in pagination mode:
   - Space / Enter / Down / PgDn: next page
   - Up / PgUp: previous page
   - 'q' / Escape: dump all remaining rows to output, exit pagination
   - 'a': show all remaining rows

#### New Method: enterPagination() (tui/model.go)
```go
func (m *Model) enterPagination(result *executor.QueryResult, format output.Format) {
    pageSize := m.deps.Config.UI.PageSize
    if pageSize <= 0 {
        pageSize = 20
    }
    totalRows := len(result.Rows)
    totalPages := (totalRows + pageSize - 1) / pageSize
    if totalPages <= 1 {
        return // no pagination needed
    }
    m.pagedResult = result
    m.pagedFormat = format
    m.pagedPage = 0
    m.pagedTotal = totalPages
}
```

#### New Method: renderPagedResult() (tui/model.go)
```go
func (m *Model) renderPagedResult() string {
    pageSize := m.deps.Config.UI.PageSize
    if pageSize <= 0 {
        pageSize = 20
    }
    result := m.pagedResult
    startRow := m.pagedPage * pageSize
    endRow := startRow + pageSize
    if endRow > len(result.Rows) {
        endRow = len(result.Rows)
    }

    // Create a partial result with only the current page's rows
    partial := &executor.QueryResult{
        Columns:  result.Columns,
        Rows:     result.Rows[startRow:endRow],
        Duration: result.Duration,
        IsQuery:  true,
    }

    var buf strings.Builder
    formatter := output.NewFormatter(m.pagedFormat, &buf)
    formatter.WriteResult(partial)

    // Append pager prompt
    prompt := fmt.Sprintf("-- More (page %d/%d, Space=next, q=show all) --",
        m.pagedPage+1, m.pagedTotal)
    return buf.String() + "\n" + prompt
}
```

#### Key Handling in Pagination Mode
In `handleKey()`, add pagination mode check before normal key processing:
```go
if m.pagedResult != nil {
    return m.handlePagedKey(msg)
}
```

```go
func (m Model) handlePagedKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    switch msg.Type {
    case tea.KeyEnter, tea.KeySpace, tea.KeyDown, tea.KeyPgDown:
        if m.pagedPage < m.pagedTotal-1 {
            m.pagedPage++
            m.output = m.output[:len(m.output)-1] // remove old prompt line
            m.addOutput(m.renderPagedResult())
        } else {
            m.exitPagination(true)
        }
    case tea.KeyUp, tea.KeyPgUp:
        if m.pagedPage > 0 {
            m.pagedPage--
            m.output = m.output[:len(m.output)-1]
            m.addOutput(m.renderPagedResult())
        }
    case tea.KeyEscape:
        m.exitPagination(false)
    default:
        if msg.Type == tea.KeyRunes && string(msg.Runes) == "q" {
            m.exitPagination(false)
        } else if msg.Type == tea.KeyRunes && string(msg.Runes) == "a" {
            m.exitPagination(true)
        }
    }
    return m, nil
}

func (m *Model) exitPagination(showAll bool) {
    if showAll && m.pagedResult != nil {
        // Render full result without pagination
        var buf strings.Builder
        formatter := output.NewFormatter(m.pagedFormat, &buf)
        formatter.WriteResult(m.pagedResult)
        // Replace last paged output with full output
        m.addOutput(strings.TrimRight(buf.String(), "\n"))
    }
    m.pagedResult = nil
    m.pagedFormat = output.FormatTable
    m.pagedPage = 0
    m.pagedTotal = 0
}
```

#### Integration in executeInput()
After formatting the result, check if pagination is needed:
```go
// After determining outFmt and creating the result...
pageSize := m.deps.Config.UI.PageSize
if result.IsQuery && len(result.Rows) > pageSize && pageSize > 0 {
    m.enterPagination(result, outFmt)
    m.addOutput(m.renderPagedResult())
} else {
    // Normal (non-paged) output
    var buf strings.Builder
    formatter := output.NewFormatter(outFmt, &buf)
    formatter.WriteResult(result)
    m.addOutput(strings.TrimRight(buf.String(), "\n"))
}
```

#### View() Changes
When in pagination mode, the `-- More --` prompt should be styled distinctly:
```go
// In renderPagedResult():
promptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
prompt = promptStyle.Render(prompt)
```

Note: Since the prompt is embedded in the output string, we need to use ANSI codes instead of lipgloss for the pager prompt within `renderPagedResult()`.

#### Implementation Files
- `tui/model.go`: Add pagination state, `enterPagination()`, `renderPagedResult()`, `handlePagedKey()`, `exitPagination()`, integrate in `executeInput()`
- `output/output.go`: No changes needed

---

### Task 4: Async Query Execution + Query Timer

**Goal**: Make query execution asynchronous so the TUI remains responsive during long-running queries. Display a real-time elapsed timer while the query is running.

**Design**:

This is a significant refactor of `executeInput()`. Currently it's synchronous — `m.deps.Executor.Execute()` blocks the TUI update loop. We need to move execution to a `tea.Cmd` (goroutine) and handle the result via a message.

#### New Message Types (tui/model.go)
```go
// execResultMsg is sent when a query execution completes.
type execResultMsg struct {
    input          string
    formatOverride output.Format
    result         *executor.QueryResult
    err            error
}

// execTickMsg is sent periodically during query execution to update the timer.
type execTickMsg time.Time
```

#### New Model State Field (tui/model.go)
```go
execStart time.Time // when the current query started executing
```

#### Refactored executeInput() (tui/model.go)
```go
func (m Model) executeInput(input string, formatOverride output.Format) (tea.Model, tea.Cmd) {
    // ... (input validation, history, echo remain the same)

    execSQL := m.normalizeTableNames(input + ";")
    m.executing = true
    m.execStart = time.Now()
    m.ed.Clear()

    // Return async execution command + timer tick
    return m, tea.Batch(
        func() tea.Msg {
            result, err := m.deps.Executor.Execute(context.Background(), execSQL)
            return execResultMsg{input, formatOverride, result, err}
        },
        tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
            return execTickMsg(t)
        }),
    )
}
```

#### Update() Message Handling (tui/model.go)
```go
case execResultMsg:
    m.executing = false
    m.execStart = time.Time{}
    m.displayQueryResult(msg.input, msg.formatOverride, msg.result, msg.err)
    return m, nil

case execTickMsg:
    if m.executing {
        // Continue ticking to update timer display
        return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
            return execTickMsg(t)
        })
    }
    return m, nil
```

#### New Method: displayQueryResult() (tui/model.go)
Extract the result formatting and display logic from the old synchronous `executeInput()`:
```go
func (m *Model) displayQueryResult(input string, formatOverride output.Format, result *executor.QueryResult, err error) {
    if err != nil {
        m.addOutput(fmt.Sprintf("%s", err))
        return
    }

    // Choose format
    outFmt := m.deps.Formatter.CurrentFormat()
    if formatOverride != output.FormatTable {
        outFmt = formatOverride
    } else if m.autoVerticalOutput && outFmt == output.FormatTable && m.width > 0 {
        if output.CalcTableWidth(result) > m.width {
            outFmt = output.FormatVertical
        }
    }

    // Check pagination
    pageSize := m.deps.Config.UI.PageSize
    if result.IsQuery && pageSize > 0 && len(result.Rows) > pageSize {
        m.enterPagination(result, outFmt)
        m.addOutput(m.renderPagedResult())
        return
    }

    // Normal output
    var buf strings.Builder
    formatter := output.NewFormatter(outFmt, &buf)
    if writeErr := formatter.WriteResult(result); writeErr != nil {
        m.addOutput(fmt.Sprintf("Output error: %s", writeErr))
    }
    if buf.Len() > 0 {
        m.addOutput(strings.TrimRight(buf.String(), "\n"))
    }
}
```

#### View() Timer Display
When `m.executing` is true, show elapsed time in the prompt area:
```go
if m.executing {
    elapsed := time.Since(m.execStart)
    timerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
    sb.WriteString(timerStyle.Render(fmt.Sprintf("Executing... (%s)", executor.FormatDuration(elapsed))))
}
```

#### Ctrl+C Handling
Already works — `m.deps.Executor.Cancel()` cancels the context. Need to also reset `execStart`:
```go
case tea.KeyCtrlC:
    if m.executing {
        m.deps.Executor.Cancel()
        m.executing = false
        m.execStart = time.Time{}
        m.addOutput("Query cancelled")
        return m, nil
    }
```

#### execSourceFile() Handling
`execSourceFile()` is simpler — it can remain synchronous since it's processing a file (expected to be batch). But for consistency, we could add a simple progress indicator. For now, keep it synchronous.

#### Implementation Files
- `tui/model.go`: Refactor `executeInput()` to async, add `execResultMsg`, `execTickMsg`, `execStart`, `displayQueryResult()`, update `Update()`, `View()`, Ctrl+C handling

---

### Task 5: Query Progress Animation

**Goal**: Show an animated spinner ("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏") while a query is executing, making it clear that mysh is working and not frozen.

**Design**:

This builds on Task 4's async execution. The spinner is shown alongside the elapsed timer.

#### New Model State Field (tui/model.go)
```go
spinnerFrame int    // current frame index of the spinner animation
```

#### Spinner Frames
```go
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
```

#### Update execTickMsg Handler
Advance the spinner frame on each tick:
```go
case execTickMsg:
    if m.executing {
        m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
        return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
            return execTickMsg(t)
        })
    }
    return m, nil
```

#### View() Combined Spinner + Timer
When executing, replace the prompt line with:
```go
if m.executing {
    elapsed := time.Since(m.execStart)
    spinner := spinnerFrames[m.spinnerFrame]
    spinnerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
    timerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
    sb.WriteString(spinnerStyle.Render(spinner))
    sb.WriteString(" ")
    sb.WriteString(timerStyle.Render(fmt.Sprintf("Executing... (%s)", executor.FormatDuration(elapsed))))
}
```

#### Reset Spinner on Completion
In `execResultMsg` handler:
```go
case execResultMsg:
    m.executing = false
    m.execStart = time.Time{}
    m.spinnerFrame = 0
    // ...
```

And in Ctrl+C handler:
```go
m.spinnerFrame = 0
```

#### Implementation Files
- `tui/model.go`: Add `spinnerFrame`, spinner frames, update `execTickMsg` handler and `View()`

---

## Implementation Order
1. **Task 1: NULL Value Coloring** — Small, isolated to output package
2. **Task 2: DML Result Highlighting** — Small, isolated to output package
3. **Task 4 + 5: Async Execution + Timer + Spinner** — Large refactor, foundation for pagination
4. **Task 3: Result Pagination** — Depends on async execution for responsive paging

## Testing Strategy
- Task 1: `SELECT NULL, 1, 'text'` — verify NULL is dimmed in table/vertical/markdown formats
- Task 2: `UPDATE t SET x=1` — verify green "Query OK" and cyan timing. `SELECT 1` — verify colored row count
- Task 3: `SELECT * FROM large_table` — verify pagination appears, Space advances, 'q' shows all
- Task 4: Run a slow query (`SELECT SLEEP(3)`) — verify timer updates in real-time, Ctrl+C cancels
- Task 5: Same slow query — verify spinner animation
- All tasks: `go build ./... && go test ./...` must pass

## Risk Assessment
- **Task 1**: Low risk — ANSI codes in strings, no logic changes
- **Task 2**: Low risk — ANSI codes in strings, no logic changes
- **Task 3**: Medium risk — new UI mode with state management. Mitigation: pagination is isolated to a separate mode, only active when `pagedResult != nil`
- **Task 4**: High risk — refactoring core execution flow from sync to async. Mitigation: carefully preserve all existing behavior, test with various query types (fast, slow, error, cancel)
- **Task 5**: Low risk — builds on Task 4's infrastructure, purely visual
