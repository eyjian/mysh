package tui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/eyjian/mysh/completer"
	"github.com/eyjian/mysh/config"
	"github.com/eyjian/mysh/connection"
	"github.com/eyjian/mysh/editor"
	"github.com/eyjian/mysh/executor"
	"github.com/eyjian/mysh/highlight"
	"github.com/eyjian/mysh/history"
	"github.com/eyjian/mysh/metadata"
	"github.com/eyjian/mysh/output"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Dependencies holds all the shared service instances for the TUI.
type Dependencies struct {
	Config              *config.Config
	Pool                *connection.Pool
	Executor            *executor.Executor
	Meta                *metadata.Cache
	History             *history.History
	Formatter           *output.Formatter
	Highlighter         *highlight.Highlighter
	Completer           *completer.Completer
	AutoVerticalOutput  bool
}

// Model is the top-level bubbletea model for the mysh TUI.
type Model struct {
	deps   Dependencies
	ed     *editor.Editor
	prompt string
	mlPrompt string

	// UI state
	width       int
	height      int
	output      []string   // accumulated output lines
	err         error      // last error
	quitting    bool
	executing   bool       // true while a query is running
	multiline   bool       // true when in multi-line input mode

	// Completion state
	showComp    bool
	compItems   []completer.Suggestion
	compIndex   int

	// Cursor blink state
	cursorOn    bool
	cursorTick  bool // true when cursor blink tick is active

	// Output scroll state
	scrollOffset int // 0 = bottom (latest), >0 = scrolled up

	// Multi-line accumulation
	multilineParts []string // collected lines during multi-line input

	// Mouse mode
	mouseEnabled bool // true = scroll wheel captures; false = native selection/copy

	// Auto vertical output
	autoVerticalOutput bool // true = switch to vertical if result wider than terminal

	// Connection health state
	connected bool // true = last health check was successful

	// History search state (Ctrl+R)
	historySearch    bool     // true when in incremental history search mode
	searchQuery      string   // current search query
	searchResults    []string // filtered history entries matching query
	searchResultIdx  int      // current index in searchResults

	// Async execution state
	execStart     time.Time // when the current query started executing
	spinnerFrame  int       // current frame index of the spinner animation

	// Last query result (for \export and \watch)
	lastResult *executor.QueryResult
	lastQuery  string
	lastFormat output.Format

	// Watch mode state
	watching      bool
	watchInterval time.Duration
	watchQuery    string
	watchFormat   output.Format
	watchTick     int

	// Alias state
	tempAliases map[string]string // session-only aliases

	// Pagination state
	pagedResult *executor.QueryResult // result being paginated (nil = not paginating)
	pagedFormat output.Format         // format for paged result
	pagedPage   int                   // current page number (0-based)
	pagedTotal  int                   // total pages

	// Timing display
	showTiming bool // true = display query execution time

	// Safe updates mode
	safeUpdates bool // true = block UPDATE/DELETE without WHERE/LIMIT

	// Working directory (for \cd and \sys)
	workDir string

	// Styles
	promptStyle lipgloss.Style
	outputStyle lipgloss.Style
	errorStyle  lipgloss.Style
	compStyle   lipgloss.Style
	compSelStyle lipgloss.Style
}

// NewModel creates a new TUI model with the given dependencies.
func NewModel(deps Dependencies) Model {
	cfg := deps.Config
	prompt := cfg.UI.Prompt
	mlPrompt := cfg.UI.MultilinePrompt

	m := Model{
		deps:         deps,
		ed:           editor.New(),
		prompt:       prompt,
		mlPrompt:     mlPrompt,
		output:       []string{},
		cursorOn:     true,
		mouseEnabled:        false,
		autoVerticalOutput:  deps.AutoVerticalOutput,
		connected:           true,
		safeUpdates:         deps.Config.Safety.SafeUpdates,
		workDir:             "", // will be set on first prompt
		promptStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true),
		outputStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		errorStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
		compStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("246")),
		compSelStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("62")).Bold(true),
	}
	_ = cfg.Theme // theme is used via Highlighter
	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		blinkCmd(),
		healthCheckCmd(),
	)
}

// blinkMsg is sent by the cursor blink timer.
type blinkMsg time.Time

// blinkCmd returns a command that waits for the cursor blink interval.
func blinkCmd() tea.Cmd {
	return tea.Tick(530*time.Millisecond, func(t time.Time) tea.Msg {
		return blinkMsg(t)
	})
}

// healthCheckMsg is sent periodically to check connection status.
type healthCheckMsg time.Time

// healthCheckCmd returns a command that checks connection health every 30 seconds.
func healthCheckCmd() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg {
		return healthCheckMsg(t)
	})
}

// tickMsg is sent after a query execution completes.
type tickMsg time.Time

// execResultMsg is sent when an async query execution completes.
type execResultMsg struct {
	input          string
	formatOverride output.Format
	result         *executor.QueryResult
	err            error
}

// execTickMsg is sent periodically during query execution to update the timer/spinner.
type execTickMsg time.Time

// watchTickMsg is sent at each watch interval to trigger the next query.
type watchTickMsg time.Time

// watchResultMsg is sent when a watch query execution completes.
type watchResultMsg struct {
	result *executor.QueryResult
	err    error
	tick   int
}

// spinnerFrames holds the braille spinner animation frames.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case blinkMsg:
		m.cursorOn = !m.cursorOn
		return m, blinkCmd()

	case healthCheckMsg:
		if m.deps.Pool != nil {
			m.connected = m.deps.Pool.IsConnected()
		}
		return m, healthCheckCmd()

	case tickMsg:
		m.executing = false
		return m, nil

	case execResultMsg:
		m.executing = false
		m.execStart = time.Time{}
		m.spinnerFrame = 0
		m.displayQueryResult(msg.result, msg.err, msg.formatOverride)
		return m, nil

	case execTickMsg:
		if m.executing {
			m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
			return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
				return execTickMsg(t)
			})
		}
		return m, nil

	case watchTickMsg:
		if m.watching {
			m.watchTick++
			return m, m.executeWatchQuery(m.watchTick)
		}
		return m, nil

	case watchResultMsg:
		if !m.watching {
			return m, nil
		}
		// Clear output and show only the latest watch result
		m.output = nil
		m.scrollOffset = 0
		m.addOutput(fmt.Sprintf("\033[2m--- Watch #%d (%s) ---\033[0m", msg.tick, time.Now().Format("15:04:05")))

		if msg.err != nil {
			m.addOutput(fmt.Sprintf("Error: %s", msg.err))
		} else {
			m.displayQueryResult(msg.result, msg.err, m.watchFormat)
		}

		// Schedule next tick
		return m, watchTickCmd(m.watchInterval)
	}

	return m, nil
}

// handleMouse processes mouse events — scroll wheel controls output area scrolling.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if !m.mouseEnabled {
		return m, nil
	}
	switch msg.Type {
	case tea.MouseWheelUp:
		if m.scrollOffset < len(m.output)-1 {
			m.scrollOffset += 3 // scroll 3 lines at a time
			if m.scrollOffset > len(m.output)-1 {
				m.scrollOffset = len(m.output) - 1
			}
			if m.scrollOffset < 0 {
				m.scrollOffset = 0
			}
		}
		return m, nil
	case tea.MouseWheelDown:
		if m.scrollOffset > 0 {
			m.scrollOffset -= 3
			if m.scrollOffset < 0 {
				m.scrollOffset = 0
			}
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Any key press makes cursor visible and resets blink
	m.cursorOn = true

	// If in pagination mode, route keys to pagination handler
	if m.pagedResult != nil {
		return m.handlePagedKey(msg)
	}

	// If in history search mode, route keys to search handler
	if m.historySearch {
		return m.handleHistorySearchKey(msg)
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		if m.watching {
			m.watching = false
			m.watchQuery = ""
			m.addOutput("Watch stopped.")
			return m, nil
		}
		if m.executing {
			m.deps.Executor.Cancel()
			m.executing = false
			m.execStart = time.Time{}
			m.spinnerFrame = 0
			m.addOutput("Query cancelled")
			return m, nil
		}
		// If there's text, clear the input (like mysql CLI)
		if !m.ed.IsEmpty() || m.multiline {
			m.ed.Clear()
			m.multiline = false
			m.multilineParts = nil
			m.showComp = false
			return m, nil
		}
		// Empty input + Ctrl-C = quit
		m.quitting = true
		return m, tea.Quit

	case tea.KeyCtrlD:
		if m.ed.IsEmpty() {
			m.quitting = true
			return m, tea.Quit
		}

	case tea.KeyCtrlR:
		return m.startHistorySearch()

	case tea.KeyCtrlA:
		m.ed.MoveHome()
		m.showComp = false
		return m, nil

	case tea.KeyCtrlE:
		m.ed.MoveEnd()
		m.showComp = false
		return m, nil

	case tea.KeyEnter:
		return m.handleEnter()

	case tea.KeyTab:
		return m.handleTab()

	case tea.KeyShiftTab:
		// Shift+Tab: cycle backwards in completion
		if m.showComp && len(m.compItems) > 0 {
			if m.compIndex > 0 {
				m.compIndex--
			} else {
				m.compIndex = len(m.compItems) - 1
			}
			return m, nil
		}
		return m, nil

	case tea.KeyUp:
		if m.showComp {
			if m.compIndex > 0 {
				m.compIndex--
			}
			return m, nil
		}
		// History navigation
		entry := m.deps.History.Prev()
		if entry != "" {
			m.ed.SetText(entry)
		}
		return m, nil

	case tea.KeyDown:
		if m.showComp {
			if m.compIndex < len(m.compItems)-1 {
				m.compIndex++
			}
			return m, nil
		}
		// History navigation
		entry := m.deps.History.Next()
		if entry != "" {
			m.ed.SetText(entry)
		} else {
			m.ed.Clear()
		}
		return m, nil

	case tea.KeyLeft:
		m.ed.MoveLeft()
		m.showComp = false
		return m, nil

	case tea.KeyRight:
		m.ed.MoveRight()
		m.showComp = false
		return m, nil

	case tea.KeyCtrlLeft:
		m.ed.MoveWordLeft()
		m.showComp = false
		return m, nil

	case tea.KeyCtrlRight:
		m.ed.MoveWordRight()
		m.showComp = false
		return m, nil

	case tea.KeyHome:
		m.ed.MoveHome()
		return m, nil

	case tea.KeyEnd:
		m.ed.MoveEnd()
		return m, nil

	case tea.KeyPgUp:
		// Scroll output up by one page
		maxOutputLines := m.height - 3
		if maxOutputLines < 1 {
			maxOutputLines = 10
		}
		m.scrollOffset += maxOutputLines
		if m.scrollOffset > len(m.output)-1 {
			m.scrollOffset = len(m.output) - 1
		}
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}
		return m, nil

	case tea.KeyPgDown:
		// Scroll output down by one page
		maxOutputLines := m.height - 3
		if maxOutputLines < 1 {
			maxOutputLines = 10
		}
		m.scrollOffset -= maxOutputLines
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}
		return m, nil

	case tea.KeyBackspace:
		m.ed.Backspace(1)
		m.showComp = false
		return m, nil

	case tea.KeyDelete:
		m.ed.Delete(1)
		m.showComp = false
		return m, nil

	case tea.KeySpace:
		m.ed.Insert(" ")
		m.showComp = false
		return m, nil

	case tea.KeyEscape:
		m.showComp = false
		return m, nil

	case tea.KeyRunes:
		// Use raw runes instead of msg.String() to avoid [ ] brackets
		// that Bubble Tea adds around pasted content for shortcut safety.
		// Also handle control characters that might leak through as runes.
		if len(msg.Runes) == 1 && msg.Runes[0] < 32 {
			return m.handleCtrlRune(msg.Runes[0])
		}
		// Handle Alt+key sequences
		if msg.Alt && len(msg.Runes) == 1 {
			switch msg.Runes[0] {
			case 'd':
				m.ed.KillWordForward()
				m.showComp = false
				return m, nil
			case 'b':
				m.ed.MoveWordLeft()
				m.showComp = false
				return m, nil
			case 'f':
				m.ed.MoveWordRight()
				m.showComp = false
				return m, nil
			}
		}
		m.ed.Insert(string(msg.Runes))
		m.showComp = false
		return m, nil
	}

	return m, nil
}

// handleCtrlRune handles control characters (0-31) that leak through as KeyRunes.
func (m Model) handleCtrlRune(r rune) (tea.Model, tea.Cmd) {
	switch r {
	case 1: // Ctrl+A
		m.ed.MoveHome()
		m.showComp = false
	case 5: // Ctrl+E
		m.ed.MoveEnd()
		m.showComp = false
	case 3: // Ctrl+C — same as KeyCtrlC
		return m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	case 4: // Ctrl+D — same as KeyCtrlD
		return m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	case 8: // Ctrl+H — backspace
		m.ed.Backspace(1)
		m.showComp = false
	case 11: // Ctrl+K — kill to end of line
		pos := m.ed.CursorPos()
		text := []rune(m.ed.Text())
		if pos < len(text) {
			m.ed = editor.New()
			m.ed.SetText(string(text[:pos]))
			m.ed.MoveEnd()
		}
		m.showComp = false
	case 18: // Ctrl+R — incremental history search
		return m.startHistorySearch()
	case 21: // Ctrl+U — kill to beginning of line
		pos := m.ed.CursorPos()
		text := []rune(m.ed.Text())
		if pos > 0 {
			m.ed = editor.New()
			m.ed.SetText(string(text[pos:]))
		}
		m.showComp = false
	case 23: // Ctrl+W — kill word backward
		m.ed.KillWordBackward()
		m.showComp = false
	default:
		// Ignore other control characters
	}
	return m, nil
}

// startHistorySearch enters the Ctrl+R incremental history search mode.
func (m Model) startHistorySearch() (tea.Model, tea.Cmd) {
	m.historySearch = true
	m.searchQuery = ""
	m.searchResults = nil
	m.searchResultIdx = 0
	return m, nil
}

// exitHistorySearch exits the history search mode, optionally accepting the match.
func (m *Model) exitHistorySearch(accept bool) {
	if accept && len(m.searchResults) > 0 && m.searchResultIdx < len(m.searchResults) {
		m.ed.SetText(m.searchResults[m.searchResultIdx])
	}
	m.historySearch = false
	m.searchQuery = ""
	m.searchResults = nil
	m.searchResultIdx = 0
}

// updateHistorySearch filters history entries matching the current query
// and resets the result index.
func (m *Model) updateHistorySearch() {
	if m.searchQuery == "" {
		m.searchResults = nil
		m.searchResultIdx = 0
		return
	}
	m.searchResults = m.deps.History.SearchIncremental(m.searchQuery)
	m.searchResultIdx = 0
}

// handleHistorySearchKey handles key events while in Ctrl+R search mode.
func (m Model) handleHistorySearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlR:
		// Cycle to next match
		if len(m.searchResults) > 0 {
			m.searchResultIdx = (m.searchResultIdx + 1) % len(m.searchResults)
		}
		return m, nil

	case tea.KeyCtrlC, tea.KeyEscape:
		m.exitHistorySearch(false)
		return m, nil

	case tea.KeyEnter:
		m.exitHistorySearch(true)
		return m, nil

	case tea.KeyBackspace:
		if len(m.searchQuery) > 0 {
			runes := []rune(m.searchQuery)
			m.searchQuery = string(runes[:len(runes)-1])
			m.updateHistorySearch()
		} else {
			m.exitHistorySearch(false)
		}
		return m, nil

	case tea.KeyRunes:
		m.searchQuery += string(msg.Runes)
		m.updateHistorySearch()
		return m, nil

	default:
		// Any other key cancels the search
		m.exitHistorySearch(false)
		return m, nil
	}
}

// parseFormatSuffix checks if the input ends with a format suffix
// (\G for vertical, \j for JSON, \m for markdown) and returns the
// trimmed input and the format override. Returns the original input
// and FormatTable (no override) if no suffix is found.
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

// formatSuffixString returns the display suffix for a format override.
func formatSuffixString(f output.Format) string {
	switch f {
	case output.FormatVertical:
		return "\\G"
	case output.FormatJSON:
		return "\\j"
	case output.FormatMarkdown:
		return "\\m"
	default:
		return ""
	}
}

// hasFormatSuffix returns true if the input ends with a recognized format suffix.
func hasFormatSuffix(input string) bool {
	return strings.HasSuffix(input, "\\G") ||
		strings.HasSuffix(input, "\\j") ||
		strings.HasSuffix(input, "\\m")
}

// handleEnter processes the Enter key: accepts completion if showing,
// otherwise executes SQL if input ends with ';' or starts multi-line mode.
func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	// If completion menu is shown, accept the selected item
	if m.showComp && len(m.compItems) > 0 {
		sel := m.compItems[m.compIndex]
		m.ed.ReplaceWordBeforeCursor(sel.Text)
		m.showComp = false
		return m, nil
	}

	input := m.ed.Text()

	// Empty input + disconnected: auto-reconnect (like MySQL CLI)
	if strings.TrimSpace(input) == "" && !m.connected {
		m.addOutput("Reconnecting...")
		if m.deps.Pool != nil {
			if err := m.deps.Pool.Reconnect(); err != nil {
				m.addOutput(fmt.Sprintf("Reconnect failed: %s", err))
			} else {
				m.connected = true
				m.addOutput("Reconnected successfully.")
				if m.deps.Meta != nil {
					m.deps.Meta.MarkDirty()
					go m.deps.Meta.Refresh()
				}
			}
		}
		m.ed.Clear()
		return m, nil
	}

	// Check for backslash commands
	trimmed := strings.TrimSpace(input)
	if strings.HasPrefix(trimmed, "\\") {
		return m.handleBackslashCommand(trimmed)
	}

	// Check for aliases (before SQL execution)
	fields := strings.Fields(trimmed)
	if len(fields) > 0 {
		if sql, ok := m.resolveAlias(fields[0]); ok {
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
			expanded := sql
			if rest != "" {
				expanded = sql + " " + rest
			}
			m.addOutput(fmt.Sprintf("\033[2m→ %s\033[0m", expanded))
			m.ed.Clear()
			return m.executeInput(expanded, output.FormatTable)
		}
	}

	// Check for built-in SQL-style commands (quit/exit/clear/source, with or without trailing ;)
	cmd := strings.TrimRight(trimmed, ";")
	lowerCmd := strings.ToLower(cmd)
	if lowerCmd == "quit" || lowerCmd == "exit" {
		m.addOutput(m.prompt + trimmed)
		m.ed.Clear()
		m.quitting = true
		return m, tea.Quit
	}
	if lowerCmd == "clear" {
		m.ed.Clear()
		m.output = nil
		m.scrollOffset = 0
		return m, nil
	}
	// Handle "source <file>" (MySQL-style command)
	if strings.HasPrefix(lowerCmd, "source ") {
		filePath := strings.TrimSpace(cmd[len("source "):])
		m.addOutput(m.prompt + trimmed)
		m.ed.Clear()
		if filePath == "" {
			m.addOutput("Usage: source <file>")
			return m, nil
		}
		return m.execSourceFile(filePath)
	}

	// Check for format suffixes: \G (vertical), \j (json), \m (markdown)
	trimmed, formatOverride := parseFormatSuffix(trimmed)

	// Multi-line: if no semicolon at end and no format suffix, continue input
	if !strings.HasSuffix(trimmed, ";") && formatOverride == output.FormatTable {
		// Move current input line to output area, start fresh line
		m.addOutput(m.prompt + input)
		m.multilineParts = append(m.multilineParts, input)
		m.ed.Clear()
		m.multiline = true
		return m, nil
	}

	// Execute the SQL — combine all multi-line parts with the final line
	m.multiline = false
	m.showComp = false
	fullInput := strings.Join(append(m.multilineParts, trimmed), " ")
	m.multilineParts = nil
	return m.executeInput(fullInput, formatOverride)
}

// handleBackslashCommand processes built-in backslash commands.
func (m Model) handleBackslashCommand(cmd string) (tea.Model, tea.Cmd) {
	// Echo the command to output area before clearing input
	m.addOutput(m.prompt + cmd)
	m.ed.Clear()
	m.multiline = false
	parts := strings.Fields(cmd)
	command := parts[0]

	switch command {
	case "\\quit", "\\q":
		m.quitting = true
		return m, tea.Quit

	case "\\help", "\\h", "\\?":
		echoIdx := len(m.output) - 1 // index of the echo line just added
		m.addOutput(helpText())
		// Auto-scroll so the echo line ("mysh> \help") is visible at the top
		maxLines := m.height - 3
		if maxLines < 1 {
			maxLines = 10
		}
		totalAfterHelp := len(m.output)
		if totalAfterHelp > maxLines {
			m.scrollOffset = totalAfterHelp - maxLines - echoIdx
			if m.scrollOffset < 0 {
				m.scrollOffset = 0
			}
		}

	case "\\clear", "\\c":
		m.output = nil
		m.scrollOffset = 0

	case "\\status", "\\s":
		m.addOutput(m.formatStatus())

	case "\\refresh", "\\r":
		if m.deps.Meta != nil {
			if err := m.deps.Meta.Refresh(); err != nil {
				m.addOutput(fmt.Sprintf("Refresh failed: %s", err))
			} else {
				m.addOutput("Metadata cache refreshed.")
			}
		}

	case "\\use":
		if len(parts) < 2 {
			m.addOutput("Usage: \\use <database>")
		} else {
			dbName := parts[1]
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := m.deps.Pool.UseDB(ctx, dbName); err != nil {
				m.addOutput(fmt.Sprintf("ERROR: %s", err))
			} else {
				m.deps.Meta.MarkDirty()
				m.deps.Meta.Refresh()
				m.addOutput(fmt.Sprintf("Database changed to %s", dbName))
			}
		}

	case "\\format":
		if len(parts) < 2 {
			m.addOutput(fmt.Sprintf("Current format: %s", m.deps.Formatter.CurrentFormat()))
		} else if strings.ToLower(parts[1]) == "sql" {
			// SQL formatting: format the last query or current input
			sql := strings.TrimSpace(m.ed.Text())
			if sql == "" && m.lastQuery != "" {
				sql = m.lastQuery + ";"
			}
			if sql == "" {
				m.addOutput("No SQL to format. Type a SQL statement first.")
			} else {
				formatted := highlight.FormatSQL(sql)
				// Apply syntax highlighting to the formatted SQL
				if m.deps.Highlighter != nil {
					m.addOutput(m.deps.Highlighter.Highlight(formatted))
				} else {
					m.addOutput(formatted)
				}
			}
		} else {
			f, err := output.ParseFormat(parts[1])
			if err != nil {
				m.addOutput(fmt.Sprintf("ERROR: %s", err))
			} else {
				m.deps.Formatter.SetFormat(f)
				m.addOutput(fmt.Sprintf("Output format set to %s", f))
			}
		}

	case "\\history":
		pattern := ""
		if len(parts) >= 2 {
			pattern = parts[1]
		}
		var entries []string
		if pattern != "" {
			entries = m.deps.History.Search(pattern)
		} else {
			entries = m.deps.History.Entries()
		}
		if len(entries) == 0 {
			m.addOutput("No history entries.")
		} else {
			for i, e := range entries {
				m.addOutput(fmt.Sprintf("%5d  %s", i+1, e))
			}
		}

	case "\\connect":
		if len(parts) < 2 {
			m.addOutput("Usage: \\connect <dsn> | \\connect <host> <port> <user> <database>")
			m.addOutput("  DSN format: user@host:port/database")
			m.addOutput("  Shorthand:  database (reconnect to same server)")
		} else {
			m.handleConnect(parts[1:])
		}

	case "\\reconnect":
		if m.deps.Pool != nil {
			m.addOutput("Reconnecting...")
			if err := m.deps.Pool.Reconnect(); err != nil {
				m.addOutput(fmt.Sprintf("Reconnect failed: %s", err))
			} else {
				m.addOutput("Reconnected successfully.")
				m.connected = true
			}
		}

	case "\\desc", "\\d":
		if len(parts) < 2 {
			m.addOutput("Usage: \\desc <table> [columns|indexes|create|full]")
			m.addOutput("  columns  Column list (default)")
			m.addOutput("  full     Full column info (type, nullable, key, default, extra)")
			m.addOutput("  indexes  Index information")
			m.addOutput("  create   SHOW CREATE TABLE")
		} else {
			m.handleDesc(parts[1:])
		}

	case "\\source":
		if len(parts) < 2 {
			m.addOutput("Usage: \\source <file>")
		} else {
			return m.execSourceFile(parts[1])
		}

	case "\\mouse":
		m.mouseEnabled = !m.mouseEnabled
		if m.mouseEnabled {
			m.addOutput("Mouse mode enabled (scroll wheel active)")
			return m, func() tea.Msg { return tea.EnableMouseCellMotion() }
		}
		m.addOutput("Mouse mode disabled (text selection/copy active)")
		return m, func() tea.Msg { return tea.DisableMouse() }

	case "\\export":
		if len(parts) < 2 {
			m.addOutput("Usage: \\export <file> [csv|json|markdown]")
			m.addOutput("  Auto-infer from extension: .csv, .json, .md")
			m.addOutput("  Export the last query result")
		} else {
			m.handleExport(parts[1:])
		}

	case "\\watch":
		if m.executing || m.watching {
			m.addOutput("A query or watch is already running.")
			return m, nil
		}
		return m.handleWatch(parts[1:])

	case "\\alias":
		if len(parts) < 2 {
			m.listAliases()
		} else if len(parts) == 2 {
			if sql, ok := m.resolveAlias(parts[1]); ok {
				m.addOutput(fmt.Sprintf("  %s = %s", parts[1], sql))
			} else {
				m.addOutput(fmt.Sprintf("Alias '%s' not defined", parts[1]))
			}
		} else {
			name := parts[1]
			sql := strings.Join(parts[2:], " ")
			if m.tempAliases == nil {
				m.tempAliases = make(map[string]string)
			}
			m.tempAliases[name] = sql
			m.addOutput(fmt.Sprintf("Alias set: %s = %s", name, sql))
		}

	case "\\unalias":
		if len(parts) < 2 {
			m.addOutput("Usage: \\unalias <name>")
		} else {
			name := parts[1]
			if m.tempAliases != nil {
				delete(m.tempAliases, name)
			}
			m.addOutput(fmt.Sprintf("Temp alias removed: %s", name))
		}

	case "\\session":
		if len(parts) < 2 {
			m.listSessions()
		} else {
			switch parts[1] {
			case "save":
				if len(parts) < 3 {
					m.addOutput("Usage: \\session save <name>")
				} else {
					m.saveSession(parts[2])
				}
			case "delete", "rm", "del":
				if len(parts) < 3 {
					m.addOutput("Usage: \\session delete <name>")
				} else {
					m.deleteSession(parts[2])
				}
			default:
				m.switchSession(parts[1])
			}
		}

	case "\\edit", "\\e":
		return m.handleEdit()

	case "\\pipe", "\\|":
		if len(parts) < 2 {
			m.addOutput("Usage: \\pipe <command> [args...]")
			m.addOutput("  Pipes the last query result to a system command.")
			m.addOutput("  Example: \\pipe grep pattern")
		} else {
			m.handlePipe(parts[1:])
		}

	case "\\timing":
		m.showTiming = !m.showTiming
		m.deps.Formatter.SetShowTiming(m.showTiming)
		if m.showTiming {
			m.addOutput("Timing is on.")
		} else {
			m.addOutput("Timing is off.")
		}

	case "\\safe-updates":
		if len(parts) >= 2 {
			switch strings.ToLower(parts[1]) {
			case "on", "1", "true":
				m.safeUpdates = true
				m.deps.Executor.SetSafeUpdates(true)
				m.addOutput("Safe updates mode enabled (UPDATE/DELETE require WHERE or LIMIT).")
			case "off", "0", "false":
				m.safeUpdates = false
				m.deps.Executor.SetSafeUpdates(false)
				m.addOutput("Safe updates mode disabled.")
			default:
				m.addOutput("Usage: \\safe-updates [on|off]")
			}
		} else {
			m.safeUpdates = !m.safeUpdates
			m.deps.Executor.SetSafeUpdates(m.safeUpdates)
			if m.safeUpdates {
				m.addOutput("Safe updates mode enabled (UPDATE/DELETE require WHERE or LIMIT).")
			} else {
				m.addOutput("Safe updates mode disabled.")
			}
		}

	case "\\slow":
		if len(parts) >= 2 {
			var seconds int
			if _, err := fmt.Sscanf(parts[1], "%d", &seconds); err == nil {
				if seconds <= 0 {
					m.deps.Executor.SetSlowThreshold(0)
					m.addOutput("Slow query warning disabled.")
				} else {
					m.deps.Executor.SetSlowThreshold(time.Duration(seconds) * time.Second)
					m.addOutput(fmt.Sprintf("Slow query threshold set to %d second(s).", seconds))
				}
			} else {
				m.addOutput("Usage: \\slow <seconds>  (0 = disabled)")
			}
		} else {
			threshold := m.deps.Executor.SlowThreshold()
			if threshold <= 0 {
				m.addOutput("Slow query warning is disabled. Use \\slow <seconds> to enable.")
			} else {
				m.addOutput(fmt.Sprintf("Slow query threshold: %s", executor.FormatDuration(threshold)))
			}
		}

	case "\\copy":
		if len(parts) < 2 {
			m.addOutput("Usage: \\copy <result|query|sql>")
			m.addOutput("  result  Copy last query result as TSV (tab-separated)")
			m.addOutput("  query   Copy last executed SQL statement")
			m.addOutput("  sql     Copy current input buffer")
		} else {
			m.handleCopy(parts[1])
		}

	case "\\cd":
		if len(parts) < 2 {
			// Show current directory
			dir, _ := os.Getwd()
			m.addOutput(dir)
		} else {
			target := parts[1]
			if strings.HasPrefix(target, "~") {
				home, err := os.UserHomeDir()
				if err == nil {
					target = home + target[1:]
				}
			}
			if err := os.Chdir(target); err != nil {
				m.addOutput(fmt.Sprintf("ERROR: %s", err))
			} else {
				dir, _ := os.Getwd()
				m.workDir = dir
				m.addOutput(dir)
			}
		}

	case "\\sys", "\\!":
		if len(parts) < 2 {
			m.addOutput("Usage: \\sys <command> [args...]")
			m.addOutput("  Execute a system command from within mysh.")
			m.addOutput("  Example: \\sys ls -la")
		} else {
			m.handleSys(parts[1:])
		}

	default:
		m.addOutput(fmt.Sprintf("Unknown command: %s. Type \\help for available commands.", command))
	}

	return m, nil
}

// execSourceFile reads and executes SQL statements from a file (like MySQL's source command).
func (m Model) execSourceFile(filePath string) (tea.Model, tea.Cmd) {
	// Expand ~ to home directory
	if strings.HasPrefix(filePath, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			filePath = home + filePath[1:]
		}
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		m.addOutput(fmt.Sprintf("Failed to read file: %s", err))
		return m, nil
	}

	content := string(data)
	ctx := context.Background()

	// Split by semicolons and execute each statement
	statements := strings.Split(content, ";")
	successCount := 0
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		// Skip empty statements and comments-only
		if stmt == "" {
			continue
		}

		// Handle format suffixes (\G, \j, \m) — client-side format indicators,
		// not valid SQL syntax, so strip before execution.
		var stmtFormatOverride output.Format
		stmt, stmtFormatOverride = parseFormatSuffix(stmt)
		if stmt == "" {
			continue
		}

		// Skip comment-only lines
		lines := strings.Split(stmt, "\n")
		hasSQL := false
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "--") && !strings.HasPrefix(line, "#") {
				hasSQL = true
				break
			}
		}
		if !hasSQL {
			continue
		}

		result, execErr := m.deps.Executor.Execute(ctx, stmt+";")
		if execErr != nil {
			m.addOutput(fmt.Sprintf("%s", execErr))
			continue
		}
		if result.Error != nil {
			m.addOutput(fmt.Sprintf("%s", result.Error))
			continue
		}
		successCount++

		// Format and display the result
		outFmt := m.deps.Formatter.CurrentFormat()
		if stmtFormatOverride != output.FormatTable {
			outFmt = stmtFormatOverride
		} else if m.autoVerticalOutput && outFmt == output.FormatTable && m.width > 0 {
			if output.CalcTableWidth(result) > m.width {
				outFmt = output.FormatVertical
			}
		}
		var buf strings.Builder
		tmpFormatter := output.NewFormatter(outFmt, &buf)
		if err := tmpFormatter.WriteResult(result); err == nil {
			m.addOutput(strings.TrimRight(buf.String(), "\n"))
		}
	}

	m.addOutput(fmt.Sprintf("Source: %s (%d statements executed)", filePath, successCount))
	m.scrollOffset = 0
	return m, nil
}

// executeInput runs the SQL statement asynchronously and displays the result.
// formatOverride specifies a per-query output format (FormatTable = use global default).
func (m Model) executeInput(input string, formatOverride output.Format) (tea.Model, tea.Cmd) {
	// Remove trailing semicolons
	input = strings.TrimRight(input, ";")
	input = strings.TrimSpace(input)

	if input == "" {
		m.ed.Clear()
		return m, nil
	}

	// Save to history (append format suffix if override was specified)
	historyEntry := input
	if suffix := formatSuffixString(formatOverride); suffix != "" {
		historyEntry = input + " " + suffix
	}
	m.deps.History.Append(historyEntry)

	// Echo the input line to output area (like mysql CLI)
	displayEntry := input + ";"
	if suffix := formatSuffixString(formatOverride); suffix != "" {
		displayEntry = input + " " + suffix
	}
	m.addOutput(m.prompt + displayEntry)

	// Normalize table name casing in SQL before execution
	execSQL := m.normalizeTableNames(input + ";")

	m.executing = true
	m.execStart = time.Now()
	m.ed.Clear()

	// Save the query for \watch
	m.lastQuery = input

	// Execute asynchronously and start timer tick
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

// displayQueryResult handles formatting and displaying a completed query result.
func (m *Model) displayQueryResult(result *executor.QueryResult, err error, formatOverride output.Format) {
	m.ed.Clear()

	// Update connection health based on result
	if err != nil && connection.IsConnectionError(err) {
		m.connected = false
	} else if err == nil {
		m.connected = true
	}

	if err != nil {
		errMsg := err.Error()
		// Check if it's a safe-update error and style it specially
		if _, ok := err.(*executor.SafeUpdateError); ok {
			m.addOutput(fmt.Sprintf("\033[33m⚠ %s\033[0m", errMsg))
			m.addOutput(fmt.Sprintf("\033[2mHint: Use \\safe-updates to toggle, or add WHERE/LIMIT clause.\033[0m"))
		} else {
			m.addOutput(fmt.Sprintf("%s", errMsg))
		}
		return
	}

	// Show reconnection warning if auto-reconnected
	if result != nil && result.Warning != "" {
		m.addOutput(fmt.Sprintf("\033[33m%s\033[0m", result.Warning))
		m.connected = true
	}

	// Show slow query warning
	if result != nil && result.SlowQuery {
		m.addOutput(fmt.Sprintf("\033[33m⚠ Slow query: %s exceeds threshold\033[0m",
			executor.FormatDuration(result.Duration)))
	}

	// Choose format: use override if specified, else auto-vertical or global default
	outFmt := m.deps.Formatter.CurrentFormat()
	if formatOverride != output.FormatTable {
		outFmt = formatOverride
	} else if m.autoVerticalOutput && outFmt == output.FormatTable && m.width > 0 {
		// With smart column truncation, use table format if it fits after truncation
		if m.deps.Formatter.MaxWidth() > 0 {
			// Smart truncation is enabled, use table format
		} else if output.CalcTableWidth(result) > m.width {
			outFmt = output.FormatVertical
		}
	}

	// Check if pagination is needed
	pageSize := m.deps.Config.UI.PageSize
	if result.IsQuery && pageSize > 0 && len(result.Rows) > pageSize {
		m.enterPagination(result, outFmt)
		m.addOutput(m.renderPagedResult())
		return
	}

	// Format and display result
	var buf strings.Builder
	formatter := output.NewFormatter(outFmt, &buf)
	formatter.SetShowTiming(m.showTiming)
	if m.width > 0 && outFmt == output.FormatTable {
		formatter.SetMaxWidth(m.width)
	}
	if writeErr := formatter.WriteResult(result); writeErr != nil {
		m.addOutput(fmt.Sprintf("Output error: %s", writeErr))
	}
	if buf.Len() > 0 {
		m.addOutput(strings.TrimRight(buf.String(), "\n"))
	}

	// Save last successful query result for \export and \watch
	if err == nil && result != nil && result.IsQuery {
		m.lastResult = result
		m.lastFormat = outFmt
	}
}

// enterPagination starts pagination mode for a large result set.
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

// renderPagedResult renders the current page of the paged result.
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
	if m.width > 0 && m.pagedFormat == output.FormatTable {
		formatter.SetMaxWidth(m.width)
	}
	formatter.WriteResult(partial)

	// Append pager prompt with ANSI styling
	prompt := fmt.Sprintf("\033[1;33m-- More (page %d/%d, Space=next, q=show all) --\033[0m",
		m.pagedPage+1, m.pagedTotal)
	return strings.TrimRight(buf.String(), "\n") + "\n" + prompt
}

// handlePagedKey handles key events while in pagination mode.
func (m Model) handlePagedKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter, tea.KeySpace, tea.KeyDown, tea.KeyPgDown:
		if m.pagedPage < m.pagedTotal-1 {
			// Remove previous page output and prompt (last rendered page lines + prompt)
			m.removeLastPagedOutput()
			m.pagedPage++
			m.addOutput(m.renderPagedResult())
		} else {
			m.exitPagination(true)
		}
	case tea.KeyUp, tea.KeyPgUp:
		if m.pagedPage > 0 {
			m.removeLastPagedOutput()
			m.pagedPage--
			m.addOutput(m.renderPagedResult())
		}
	case tea.KeyEscape:
		m.exitPagination(false)
	default:
		if msg.Type == tea.KeyRunes {
			switch string(msg.Runes) {
			case "q":
				m.exitPagination(false)
			case "a":
				m.exitPagination(true)
			default:
				// Ignore other keys in pagination mode
			}
		}
	}
	return m, nil
}

// exitPagination exits pagination mode, optionally showing all remaining rows.
func (m *Model) exitPagination(showAll bool) {
	if showAll && m.pagedResult != nil {
		// Remove the last paged page and show full result
		m.removeLastPagedOutput()
		var buf strings.Builder
		formatter := output.NewFormatter(m.pagedFormat, &buf)
		if m.width > 0 && m.pagedFormat == output.FormatTable {
			formatter.SetMaxWidth(m.width)
		}
		formatter.WriteResult(m.pagedResult)
		m.addOutput(strings.TrimRight(buf.String(), "\n"))
	}
	m.pagedResult = nil
	m.pagedFormat = output.FormatTable
	m.pagedPage = 0
	m.pagedTotal = 0
}

// removeLastPagedOutput removes the lines from the last rendered paged result
// from the output buffer. It removes lines until it finds the pager prompt marker.
func (m *Model) removeLastPagedOutput() {
	// Remove lines from the end until we've removed the pager prompt line
	// The pager prompt contains "-- More --"
	for len(m.output) > 0 {
		lastIdx := len(m.output) - 1
		lastLine := m.output[lastIdx]
		m.output = m.output[:lastIdx]
		if strings.Contains(lastLine, "-- More") {
			break
		}
	}
}

// handleEdit opens the user's editor ($EDITOR or vi) with the current input buffer
// or the last executed query. After the editor closes, the content is executed as SQL.
func (m Model) handleEdit() (tea.Model, tea.Cmd) {
	// Determine initial content: current input, or last query
	content := m.ed.Text()
	if strings.TrimSpace(content) == "" && m.lastQuery != "" {
		content = m.lastQuery
	}

	// Write content to a temp file
	tmpFile, err := os.CreateTemp("", "mysh-edit-*.sql")
	if err != nil {
		m.addOutput(fmt.Sprintf("ERROR: Failed to create temp file: %s", err))
		return m, nil
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		m.addOutput(fmt.Sprintf("ERROR: Failed to write temp file: %s", err))
		return m, nil
	}
	tmpFile.Close()

	// Determine editor
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "vi"
	}

	// Suspend the TUI, run the editor, then resume
	tea.ExitAltScreen()
	defer tea.EnterAltScreen()

	cmd := exec.Command(editor, tmpPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		m.addOutput(fmt.Sprintf("ERROR: Editor failed: %s", err))
		return m, nil
	}

	// Read back the edited content
	edited, err := os.ReadFile(tmpPath)
	if err != nil {
		m.addOutput(fmt.Sprintf("ERROR: Failed to read temp file: %s", err))
		return m, nil
	}

	sql := strings.TrimSpace(string(edited))
	if sql == "" {
		m.addOutput("Empty content, nothing to execute.")
		m.ed.Clear()
		return m, nil
	}

	// Clear the input line and execute the edited SQL
	m.ed.Clear()
	m.multiline = false
	m.addOutput(m.prompt + sql)
	return m.executeInput(sql, output.FormatTable)
}

// handlePipe pipes the last query result to a system command.
func (m *Model) handlePipe(cmdParts []string) {
	if m.lastResult == nil || !m.lastResult.IsQuery {
		m.addOutput("No query result to pipe. Execute a SELECT query first.")
		return
	}

	// Format the result as tab-separated plain text (no ANSI codes) for piping
	var buf strings.Builder
	// Header
	buf.WriteString(strings.Join(m.lastResult.Columns, "\t"))
	buf.WriteString("\n")
	// Rows
	for _, row := range m.lastResult.Rows {
		vals := make([]string, len(row))
		for i, val := range row {
			vals[i] = output.FormatValuePlain(val)
		}
		buf.WriteString(strings.Join(vals, "\t"))
		buf.WriteString("\n")
	}

	// Execute the system command with piped input
	cmd := exec.Command(cmdParts[0], cmdParts[1:]...)
	cmd.Stdin = strings.NewReader(buf.String())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		m.addOutput(fmt.Sprintf("ERROR: Command failed: %s\n%s", err, stderr.String()))
		return
	}

	out := strings.TrimRight(stdout.String(), "\n")
	if out != "" {
		m.addOutput(out)
	}
}

// normalizeTableNames replaces table name identifiers in SQL with the correct
// casing from the metadata cache, making table name references case-insensitive.
func (m Model) normalizeTableNames(sql string) string {
	if m.deps.Meta == nil {
		return sql
	}
	// Extract all identifier-like tokens from SQL and replace with correct casing
	// We scan for word boundaries and check if each word matches a known table name
	var result strings.Builder
	runes := []rune(sql)
	i := 0
	for i < len(runes) {
		// Skip backtick-quoted identifiers (they're explicit, leave as-is)
		if runes[i] == '`' {
			j := i + 1
			for j < len(runes) && runes[j] != '`' {
				j++
			}
			if j < len(runes) {
				j++ // include closing backtick
			}
			result.WriteString(string(runes[i:j]))
			i = j
			continue
		}
		// Skip single-quoted strings
		if runes[i] == '\'' {
			j := i + 1
			for j < len(runes) && runes[j] != '\'' {
				if runes[j] == '\\' && j+1 < len(runes) {
					j++ // skip escaped char
				}
				j++
			}
			if j < len(runes) {
				j++ // include closing quote
			}
			result.WriteString(string(runes[i:j]))
			i = j
			continue
		}
		// Skip double-quoted strings
		if runes[i] == '"' {
			j := i + 1
			for j < len(runes) && runes[j] != '"' {
				if runes[j] == '\\' && j+1 < len(runes) {
					j++
				}
				j++
			}
			if j < len(runes) {
				j++
			}
			result.WriteString(string(runes[i:j]))
			i = j
			continue
		}
		// Collect identifier (letter/digit/underscore)
		if isIdentStart(runes[i]) {
			j := i + 1
			for j < len(runes) && isIdentChar(runes[j]) {
				j++
			}
			word := string(runes[i:j])
			resolved := m.deps.Meta.ResolveTableName(word)
			result.WriteString(resolved)
			i = j
			continue
		}
		result.WriteRune(runes[i])
		i++
	}
	return result.String()
}

func isIdentStart(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_'
}

func isIdentChar(r rune) bool {
	return isIdentStart(r) || (r >= '0' && r <= '9')
}

// handleTab triggers auto-completion or cycles through suggestions.
func (m Model) handleTab() (tea.Model, tea.Cmd) {
	if m.deps.Completer == nil {
		return m, nil
	}

	// If completion is already shown, cycle to next item
	if m.showComp && len(m.compItems) > 0 {
		m.compIndex = (m.compIndex + 1) % len(m.compItems)
		return m, nil
	}

	input := m.ed.Text()
	cursorPos := m.ed.CursorPos()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	suggestions, err := m.deps.Completer.Complete(ctx, input, cursorPos)
	if err != nil || len(suggestions) == 0 {
		m.showComp = false
		return m, nil
	}

	// If only one suggestion, accept it immediately
	if len(suggestions) == 1 {
		// Check if this is a snippet trigger — expand the template
		if snippet := completer.FindSnippetByTrigger(suggestions[0].Text); snippet != nil {
			// Replace the current word with the full template
			m.ed.ReplaceWordBeforeCursor(snippet.Template)
			m.showComp = false
			return m, nil
		}
		m.ed.ReplaceWordBeforeCursor(suggestions[0].Text)
		m.showComp = false
		return m, nil
	}

	// Show completion menu
	m.compItems = suggestions
	m.compIndex = 0
	m.showComp = true
	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	if m.quitting {
		var sb strings.Builder
		// Preserve existing output
		for _, line := range m.output {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		sb.WriteString("Goodbye!\n")
		return sb.String()
	}

	var sb strings.Builder

	// Output area (scrollable region)
	maxOutputLines := m.height - 3 // leave room for prompt + completion
	if maxOutputLines < 1 {
		maxOutputLines = 10
	}
	totalLines := len(m.output)
	// Default: show the latest lines (scrollOffset = 0 means bottom)
	start := 0
	if totalLines > maxOutputLines {
		start = totalLines - maxOutputLines - m.scrollOffset
		if start < 0 {
			start = 0
		}
	}
	end := start + maxOutputLines
	if end > totalLines {
		end = totalLines
	}
	for i := start; i < end; i++ {
		sb.WriteString(m.output[i])
		sb.WriteString("\n")
	}

	// Prompt line
	if m.executing {
		// Show spinner + elapsed timer while query is running
		elapsed := time.Since(m.execStart)
		spinner := spinnerFrames[m.spinnerFrame]
		spinnerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
		timerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
		sb.WriteString(spinnerStyle.Render(spinner))
		sb.WriteString(" ")
		sb.WriteString(timerStyle.Render(fmt.Sprintf("Executing... (%s)", executor.FormatDuration(elapsed))))
	} else if m.historySearch {
		// Render Ctrl+R search prompt
		searchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
		queryStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
		matchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))

		sb.WriteString(searchStyle.Render("(reverse-i-search)"))
		sb.WriteString(queryStyle.Render("`" + m.searchQuery + "': "))

		if len(m.searchResults) > 0 && m.searchResultIdx < len(m.searchResults) {
			sb.WriteString(matchStyle.Render(m.searchResults[m.searchResultIdx]))
		} else if m.searchQuery != "" {
			sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("no matches"))
		}
	} else {
		// Normal prompt + highlighted input with cursor
		// Connection health indicator
		var healthIndicator string
		if m.connected {
			healthIndicator = "\033[32m●\033[0m " // green dot
		} else {
			healthIndicator = "\033[31m●\033[0m " // red dot
		}
		currentPrompt := healthIndicator + m.prompt
		sb.WriteString(m.promptStyle.Render(currentPrompt))

		input := m.ed.Text()
		cursorPos := m.ed.CursorPos()

		// Cursor: overlay on character at cursor position using reverse video
		cursorStyle := lipgloss.NewStyle().Reverse(true)

		if input == "" {
			if m.cursorOn {
				sb.WriteString(cursorStyle.Render(" "))
			} else {
				sb.WriteString(" ")
			}
		} else {
			runes := []rune(input)
			beforeText := string(runes[:cursorPos])

			if m.cursorOn && cursorPos < len(runes) {
			// Cursor overlays the character at cursorPos
			cursorChar := string(runes[cursorPos])
			afterText := string(runes[cursorPos+1:])
			if m.deps.Highlighter != nil {
				sb.WriteString(m.deps.Highlighter.Highlight(beforeText))
				sb.WriteString(cursorStyle.Render(cursorChar))
				sb.WriteString(m.deps.Highlighter.Highlight(afterText))
			} else {
				sb.WriteString(beforeText)
				sb.WriteString(cursorStyle.Render(cursorChar))
				sb.WriteString(afterText)
			}
		} else if m.cursorOn && cursorPos == len(runes) {
			// Cursor at end of text — block cursor on empty space
			if m.deps.Highlighter != nil {
				sb.WriteString(m.deps.Highlighter.Highlight(beforeText))
			} else {
				sb.WriteString(beforeText)
			}
			sb.WriteString(cursorStyle.Render(" "))
		} else {
			// Cursor hidden (blink off)
			afterText := string(runes[cursorPos:])
			if m.deps.Highlighter != nil {
				sb.WriteString(m.deps.Highlighter.Highlight(beforeText))
				sb.WriteString(m.deps.Highlighter.Highlight(afterText))
			} else {
				sb.WriteString(beforeText)
				sb.WriteString(afterText)
			}
		}
		}
	}

	// Completion menu
	if m.showComp && len(m.compItems) > 0 {
		sb.WriteString("\n")
		maxShow := 8
		if len(m.compItems) < maxShow {
			maxShow = len(m.compItems)
		}
		for i := 0; i < maxShow; i++ {
			item := m.compItems[i]
			line := fmt.Sprintf("  %-20s %s", item.Text, item.Detail)
			if i == m.compIndex {
				sb.WriteString(m.compSelStyle.Render(line))
			} else {
				sb.WriteString(m.compStyle.Render(line))
			}
			sb.WriteString("\n")
		}
		if len(m.compItems) > maxShow {
			sb.WriteString(m.compStyle.Render(fmt.Sprintf("  ... and %d more", len(m.compItems)-maxShow)))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// addOutput appends lines to the output buffer.
func (m *Model) addOutput(text string) {
	lines := strings.Split(text, "\n")
	m.output = append(m.output, lines...)
}

// handleConnect handles the \connect command to switch database connections.
func (m *Model) handleConnect(parts []string) {
	if m.deps.Pool == nil {
		m.addOutput("No connection pool available.")
		return
	}

	var newCfg config.ConnectionConfig
	if len(parts) == 1 && strings.Contains(parts[0], "@") {
		// DSN format: user@host:port/database
		parsed, parseErr := parseConnectDSN(parts[0], m.deps.Config.Connection)
		if parseErr != nil {
			m.addOutput(fmt.Sprintf("Invalid DSN: %s", parseErr))
			return
		}
		newCfg = parsed
	} else if len(parts) == 1 {
		// Just a database name — reconnect to same server
		newCfg = m.deps.Config.Connection
		newCfg.Database = parts[0]
	} else if len(parts) >= 4 {
		// host port user database
		newCfg = m.deps.Config.Connection // inherit charset, password
		newCfg.Host = parts[0]
		fmt.Sscanf(parts[1], "%d", &newCfg.Port)
		newCfg.User = parts[2]
		newCfg.Database = parts[3]
	} else {
		m.addOutput("Usage: \\connect <dsn> | \\connect <host> <port> <user> <database>")
		return
	}

	// Attempt reconnection
	m.addOutput(fmt.Sprintf("Connecting to %s@%s:%d/%s ...", newCfg.User, newCfg.Host, newCfg.Port, newCfg.Database))
	if err := m.deps.Pool.Reset(&newCfg); err != nil {
		m.addOutput(fmt.Sprintf("Connection failed: %s", err))
		return
	}

	// Update config
	m.deps.Config.Connection = newCfg

	// Refresh metadata
	if m.deps.Meta != nil {
		m.deps.Meta.MarkDirty()
		go m.deps.Meta.Refresh()
	}

	m.connected = true
	m.addOutput("Connection established.")
}

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

// handleExport handles the \export command.
func (m *Model) handleExport(parts []string) {
	if m.lastResult == nil {
		m.addOutput("No query result to export. Run a SELECT query first.")
		return
	}

	filePath := parts[0]
	// Expand ~ to home directory
	if strings.HasPrefix(filePath, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			filePath = home + filePath[1:]
		}
	}

	var format output.ExportFormat
	if len(parts) >= 2 {
		f, err := output.ParseExportFormat(parts[1])
		if err != nil {
			m.addOutput(fmt.Sprintf("Error: %s", err))
			return
		}
		format = f
	} else {
		f, err := output.InferExportFormat(filePath)
		if err != nil {
			m.addOutput(fmt.Sprintf("Error: %s", err))
			return
		}
		format = f
	}

	rowCount, err := output.ExportResult(m.lastResult, filePath, format)
	if err != nil {
		m.addOutput(fmt.Sprintf("Export failed: %s", err))
		return
	}

	m.addOutput(fmt.Sprintf("Exported %d rows to %s (%s format)", rowCount, filePath, format))
}

// handleWatch handles the \watch command.
func (m Model) handleWatch(args []string) (tea.Model, tea.Cmd) {
	// Parse interval (default 5 seconds)
	interval := 5 * time.Second
	if len(args) >= 1 {
		var seconds int
		if _, err := fmt.Sscanf(args[0], "%d", &seconds); err == nil && seconds > 0 {
			interval = time.Duration(seconds) * time.Second
		}
	}

	// Determine SQL: specified in args or use last query
	query := ""
	if len(args) >= 2 {
		query = strings.Join(args[1:], " ")
	} else if m.lastQuery != "" {
		query = m.lastQuery
	}

	if query == "" {
		m.addOutput("Usage: \\watch [seconds] [SQL]")
		m.addOutput("  No previous query to watch. Please specify a SQL statement.")
		return m, nil
	}

	m.watching = true
	m.watchInterval = interval
	m.watchQuery = query
	m.watchFormat = m.deps.Formatter.CurrentFormat()
	m.watchTick = 1

	m.addOutput(fmt.Sprintf("Watching (every %s, Ctrl+C to stop)...", interval))

	// Execute first query immediately
	return m, tea.Batch(
		m.executeWatchQuery(1),
		watchTickCmd(interval),
	)
}

// watchTickCmd returns a command that waits for the watch interval.
func watchTickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return watchTickMsg(t)
	})
}

// executeWatchQuery returns a command that executes the watch query.
func (m Model) executeWatchQuery(tick int) tea.Cmd {
	query := m.watchQuery
	return func() tea.Msg {
		result, err := m.deps.Executor.Execute(context.Background(), query+";")
		return watchResultMsg{result, err, tick}
	}
}

// resolveAlias looks up an alias, checking temp aliases first, then config aliases.
func (m Model) resolveAlias(name string) (string, bool) {
	if m.tempAliases != nil {
		if sql, ok := m.tempAliases[name]; ok {
			return sql, ok
		}
	}
	if m.deps.Config.Aliases != nil {
		if sql, ok := m.deps.Config.Aliases[name]; ok {
			return sql, ok
		}
	}
	return "", false
}

// listAliases lists all defined aliases.
func (m Model) listAliases() {
	count := 0

	if m.deps.Config.Aliases != nil {
		for name, sql := range m.deps.Config.Aliases {
			m.addOutput(fmt.Sprintf("  %-15s → %s", name, sql))
			count++
		}
	}

	if m.tempAliases != nil {
		for name, sql := range m.tempAliases {
			m.addOutput(fmt.Sprintf("  %-15s → %s  \033[33m(temp)\033[0m", name, sql))
			count++
		}
	}

	if count == 0 {
		m.addOutput("No aliases defined. Add aliases in ~/.mysh.yaml or use \\alias <name> <sql>.")
	} else {
		m.addOutput(fmt.Sprintf("%d alias(es) (config + temp)", count))
	}
}

// listSessions lists all saved sessions.
func (m Model) listSessions() {
	sessions := m.deps.Config.Sessions
	if len(sessions) == 0 {
		m.addOutput("No saved sessions. Use \\session save <name> to save current connection.")
		return
	}

	currentCfg := m.deps.Config.Connection
	for name, sess := range sessions {
		marker := " "
		if sess.Host == currentCfg.Host && sess.Port == currentCfg.Port &&
			sess.User == currentCfg.User && sess.Database == currentCfg.Database {
			marker = "\033[32m*\033[0m"
		}
		port := sess.Port
		if port == 0 {
			port = 3306
		}
		m.addOutput(fmt.Sprintf("  %s %-12s %s@%s:%d/%s", marker, name, sess.User, sess.Host, port, sess.Database))
	}
}

// switchSession switches to a named session.
func (m *Model) switchSession(name string) {
	sessions := m.deps.Config.Sessions
	if sessions == nil {
		m.addOutput(fmt.Sprintf("Session '%s' not found.", name))
		return
	}
	sess, ok := sessions[name]
	if !ok {
		m.addOutput(fmt.Sprintf("Session '%s' not found. Use \\session to list all sessions.", name))
		return
	}

	newCfg := sess.ToConnectionConfig()

	if m.deps.Pool == nil {
		m.addOutput("Cannot switch: no connection pool available.")
		return
	}

	m.addOutput(fmt.Sprintf("Switching to session '%s': %s@%s:%d/%s ...", name, newCfg.User, newCfg.Host, newCfg.Port, newCfg.Database))

	if err := m.deps.Pool.Reset(&newCfg); err != nil {
		m.addOutput(fmt.Sprintf("Connection failed: %s", err))
		m.connected = false
		return
	}

	m.deps.Config.Connection = newCfg

	if m.deps.Meta != nil {
		m.deps.Meta.MarkDirty()
		go m.deps.Meta.Refresh()
	}

	m.connected = true
	m.addOutput("Session switched successfully.")
}

// saveSession saves the current connection as a named session.
func (m *Model) saveSession(name string) {
	cfg := m.deps.Config.Connection

	if m.deps.Config.Sessions == nil {
		m.deps.Config.Sessions = make(map[string]config.SessionConfig)
	}

	m.deps.Config.Sessions[name] = config.SessionConfig{
		Host:     cfg.Host,
		Port:     cfg.Port,
		User:     cfg.User,
		Password: "", // Don't save password for security
		Database: cfg.Database,
		Charset:  cfg.Charset,
	}

	if err := config.Save(m.deps.Config); err != nil {
		m.addOutput(fmt.Sprintf("Save failed: %s (session is valid for this session only)", err))
	} else {
		m.addOutput(fmt.Sprintf("Session '%s' saved to config file.", name))
	}
}

// deleteSession deletes a named session.
func (m *Model) deleteSession(name string) {
	if m.deps.Config.Sessions == nil {
		m.addOutput("No saved sessions.")
		return
	}

	if _, ok := m.deps.Config.Sessions[name]; !ok {
		m.addOutput(fmt.Sprintf("Session '%s' not found.", name))
		return
	}

	delete(m.deps.Config.Sessions, name)

	if err := config.Save(m.deps.Config); err != nil {
		m.addOutput(fmt.Sprintf("Delete save failed: %s", err))
	} else {
		m.addOutput(fmt.Sprintf("Session '%s' deleted.", name))
	}
}

// formatStatus returns a human-readable connection status string.
func (m Model) formatStatus() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("mysh version 0.1.0\n"))
	sb.WriteString(fmt.Sprintf("Connection: "))
	if m.deps.Pool != nil {
		cfg := m.deps.Config.Connection
		sb.WriteString(fmt.Sprintf("%s@%s:%d", cfg.User, cfg.Host, cfg.Port))
		db := m.deps.Pool.CurrentDB()
		if db != "" {
			sb.WriteString(fmt.Sprintf("/%s", db))
		}
		if m.deps.Pool.IsConnected() {
			sb.WriteString(" (connected)")
		} else {
			sb.WriteString(" (disconnected)")
		}
	} else {
		sb.WriteString("not connected")
	}
	sb.WriteString(fmt.Sprintf("\nOutput format: %s\n", m.deps.Formatter.CurrentFormat()))
	if m.safeUpdates {
		sb.WriteString("Safe updates: ON\n")
	}
	if m.deps.Executor != nil {
		if threshold := m.deps.Executor.SlowThreshold(); threshold > 0 {
			sb.WriteString(fmt.Sprintf("Slow query threshold: %s\n", executor.FormatDuration(threshold)))
		}
	}
	if m.deps.Meta != nil {
		dbs := m.deps.Meta.Databases()
		sb.WriteString(fmt.Sprintf("Cached databases: %d\n", len(dbs)))
	}
	return sb.String()
}

// handleDesc handles the \desc command with sub-modes.
func (m *Model) handleDesc(parts []string) {
	if m.deps.Pool == nil {
		m.addOutput("No connection available.")
		return
	}

	tableName := parts[0]
	mode := "columns"
	if len(parts) >= 2 {
		mode = strings.ToLower(parts[1])
	}

	db := m.deps.Pool.CurrentDB()

	switch mode {
	case "create":
		createSQL, err := m.deps.Pool.ShowCreateTable(db, tableName)
		if err != nil {
			m.addOutput(fmt.Sprintf("ERROR: %s", err))
			return
		}
		// Format and highlight the CREATE TABLE statement
		formatted := highlight.FormatSQL(createSQL)
		if m.deps.Highlighter != nil {
			m.addOutput(m.deps.Highlighter.Highlight(formatted))
		} else {
			m.addOutput(formatted)
		}

	case "indexes", "index", "keys":
		indexes, err := m.deps.Pool.ShowIndexes(db, tableName)
		if err != nil {
			m.addOutput(fmt.Sprintf("ERROR: %s", err))
			return
		}
		if len(indexes) == 0 {
			m.addOutput(fmt.Sprintf("No indexes found for table '%s'.", tableName))
			return
		}
		// Group by index name
		type idxGroup struct {
			name      string
			columns   []string
			nonUnique bool
			idxType   string
		}
		groups := make(map[string]*idxGroup)
		var order []string
		for _, idx := range indexes {
			if _, ok := groups[idx.Name]; !ok {
				groups[idx.Name] = &idxGroup{
					name:      idx.Name,
					nonUnique: idx.NonUnique,
					idxType:   idx.Type,
				}
				order = append(order, idx.Name)
			}
			groups[idx.Name].columns = append(groups[idx.Name].columns, idx.Columns)
		}

		// Display as table
		result := &executor.QueryResult{
			Columns: []string{"Index", "Type", "Unique", "Columns"},
			IsQuery: true,
		}
		for _, name := range order {
			g := groups[name]
			unique := "YES"
			if g.nonUnique {
				unique = "NO"
			}
			result.Rows = append(result.Rows, []any{name, g.idxType, unique, strings.Join(g.columns, ", ")})
		}
		m.displayQueryResult(result, nil, output.FormatTable)

	case "full":
		// Full column info using SHOW FULL COLUMNS
		result, err := m.deps.Executor.Execute(context.Background(),
			fmt.Sprintf("SHOW FULL COLUMNS FROM `%s`", tableName)+";")
		if err != nil {
			m.addOutput(fmt.Sprintf("ERROR: %s", err))
			return
		}
		m.displayQueryResult(result, nil, output.FormatTable)

	default: // "columns"
		result, err := m.deps.Executor.Execute(context.Background(),
			fmt.Sprintf("DESCRIBE `%s`", tableName)+";")
		if err != nil {
			m.addOutput(fmt.Sprintf("ERROR: %s", err))
			return
		}
		m.displayQueryResult(result, nil, output.FormatTable)
	}
}

// handleCopy copies data to the system clipboard.
func (m *Model) handleCopy(what string) {
	var content string

	switch strings.ToLower(what) {
	case "result":
		if m.lastResult == nil || !m.lastResult.IsQuery {
			m.addOutput("No query result to copy. Execute a SELECT query first.")
			return
		}
		// Format as TSV (tab-separated values)
		var buf strings.Builder
		buf.WriteString(strings.Join(m.lastResult.Columns, "\t"))
		buf.WriteString("\n")
		for _, row := range m.lastResult.Rows {
			vals := make([]string, len(row))
			for i, val := range row {
				vals[i] = output.FormatValuePlain(val)
			}
			buf.WriteString(strings.Join(vals, "\t"))
			buf.WriteString("\n")
		}
		content = buf.String()

	case "query":
		if m.lastQuery == "" {
			m.addOutput("No last query to copy.")
			return
		}
		content = m.lastQuery + ";"

	case "sql":
		sql := strings.TrimSpace(m.ed.Text())
		if sql == "" {
			m.addOutput("Input buffer is empty.")
			return
		}
		content = sql

	default:
		m.addOutput(fmt.Sprintf("Unknown copy target: %s (use: result, query, sql)", what))
		return
	}

	if err := copyToClipboard(content); err != nil {
		m.addOutput(fmt.Sprintf("Copy failed: %s", err))
		m.addOutput("Tip: install xclip (Linux) or xsel for clipboard support.")
		return
	}

	lines := strings.Count(content, "\n")
	if lines > 0 {
		lines-- // trailing newline
	}
	switch strings.ToLower(what) {
	case "result":
		m.addOutput(fmt.Sprintf("Copied %d rows to clipboard.", lines))
	case "query":
		m.addOutput("Copied last query to clipboard.")
	case "sql":
		m.addOutput("Copied current SQL to clipboard.")
	}
}

// handleSys executes a system command and displays its output.
func (m *Model) handleSys(cmdParts []string) {
	cmd := exec.Command(cmdParts[0], cmdParts[1:]...)
	if m.workDir != "" {
		cmd.Dir = m.workDir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimRight(stderr.String(), "\n")
		if errMsg != "" {
			m.addOutput(fmt.Sprintf("ERROR: %s\n%s", err, errMsg))
		} else {
			m.addOutput(fmt.Sprintf("ERROR: %s", err))
		}
		return
	}

	out := strings.TrimRight(stdout.String(), "\n")
	if out != "" {
		// Split multi-line output and add each line
		for _, line := range strings.Split(out, "\n") {
			m.addOutput(line)
		}
	}
}

// copyToClipboard copies text to the system clipboard using available tools.
func copyToClipboard(text string) error {
	// Try xclip first (most common on Linux)
	if path, err := exec.LookPath("xclip"); err == nil {
		cmd := exec.Command(path, "-selection", "clipboard")
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Try xsel
	if path, err := exec.LookPath("xsel"); err == nil {
		cmd := exec.Command(path, "--clipboard", "--input")
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Try pbcopy (macOS)
	if path, err := exec.LookPath("pbcopy"); err == nil {
		cmd := exec.Command(path)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Try termux-clipboard-set (Termux on Android)
	if path, err := exec.LookPath("termux-clipboard-set"); err == nil {
		cmd := exec.Command(path)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Try wl-copy (Wayland)
	if path, err := exec.LookPath("wl-copy"); err == nil {
		cmd := exec.Command(path)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	return fmt.Errorf("no clipboard tool found (install xclip, xsel, pbcopy, or wl-copy)")
}

// helpText returns the help message for backslash commands.
func helpText() string {
	return `mysh - MySQL CLI with syntax highlighting and auto-completion

Backslash commands:
  \help, \h, \?    Show this help message
  \quit, \q         Exit mysh
  \clear, \c        Clear screen output
  \status, \s       Show connection status
  \use <db>         Switch to database <db>
  \refresh, \r      Refresh metadata cache
  \format [type]    Set/show output format (table|vertical|json|markdown|sql)
  \history [pat]    Search/show command history
  \connect <dsn>    Connect to a database (user@host:port/db or just db)
  \reconnect        Reconnect to the current server
  \desc <t> [mode]  Describe table (columns|full|indexes|create)
  \source <file>    Execute SQL from file
  \edit, \e         Open editor ($EDITOR or vi) to edit/execute SQL
  \pipe, \| <cmd>   Pipe last query result to a system command
  \copy <what>      Copy to clipboard (result|query|sql)
  \timing           Toggle query execution time display
  \safe-updates [on|off]  Toggle safe-updates mode (block UPDATE/DELETE without WHERE/LIMIT)
  \slow [seconds]   Set/show slow query warning threshold (0 = disabled)
  \mouse            Toggle mouse mode (scroll wheel vs text selection)
  \export <f> [fmt] Export last result to file (csv/json/markdown)
  \watch [sec] [SQL] Watch query at intervals (default 5s, Ctrl+C stop)
  \alias [name sql] Show/set command aliases
  \unalias <name>   Remove temp alias
  \session          List saved sessions
  \session <name>   Switch to saved session
  \session save <n> Save current connection as session
  \session del <n>  Delete a saved session
  \cd [dir]         Change/show working directory (for \source, \sys)
  \sys, \! <cmd>    Execute a system command

Format suffixes (append to SQL):
  \G                Display result in vertical format
  \j                Display result in JSON format
  \m                Display result in Markdown format

Keyboard shortcuts:
  Tab               Auto-complete (snippets expand on single match)
  Up/Down           Navigate history / completion list
  Ctrl+C            Cancel query/watch or clear input
  Ctrl+D            Exit (when input is empty)
  Ctrl+R            Incremental history search
  Ctrl+Left/Right   Move by word
  Ctrl+W            Kill word backward
  Alt+D             Kill word forward
  Alt+B/F           Move word backward/forward
  Enter             Execute SQL (ends with ;) or start multi-line
`
}
