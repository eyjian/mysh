package tui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/eyjian/mysh/completer"
	"github.com/eyjian/mysh/config"
	"github.com/eyjian/mysh/connection"
	"github.com/eyjian/mysh/editor"
	"github.com/eyjian/mysh/executor"
	"github.com/eyjian/mysh/highlight"
	"github.com/eyjian/mysh/history"
	"github.com/eyjian/mysh/metadata"
	"github.com/eyjian/mysh/output"
	sshpkg "github.com/eyjian/mysh/ssh"
)

// Dependencies holds all the shared service instances for the TUI.
type Dependencies struct {
	Config             *config.Config
	Pool               *connection.Pool
	Executor           *executor.Executor
	Meta               *metadata.Cache
	History            *history.History
	Formatter          *output.Formatter
	Highlighter        *highlight.Highlighter
	Completer          *completer.Completer
	AutoVerticalOutput bool
	SSHTunnel          *sshpkg.Tunnel // nil if no SSH tunnel
}

// TunnelCloser is an interface for closing an SSH tunnel.
type TunnelCloser interface {
	Close() error
	LocalAddr() string
}

// Model is the top-level bubbletea model for the mysh TUI.
type Model struct {
	deps     Dependencies
	ed       *editor.Editor
	prompt   string
	mlPrompt string

	// UI state
	width             int
	height            int
	output            []string // accumulated output lines (internal tracking)
	pendingPrintLines []string // lines waiting to be printed via tea.Println
	err               error    // last error
	quitting          bool
	executing         bool // true while a query is running
	multiline         bool // true when in multi-line input mode

	// Completion state
	showComp  bool
	compItems []completer.Suggestion
	compIndex int

	// Cursor blink state
	cursorOn   bool
	cursorTick bool // true when cursor blink tick is active

	// Multi-line accumulation
	multilineParts []string // collected lines during multi-line input

	// Mouse mode
	mouseEnabled bool // true = scroll wheel captures; false = native selection/copy

	// Auto vertical output
	autoVerticalOutput bool // true = switch to vertical if result wider than terminal

	// Connection health state
	connected bool // true = last health check was successful

	// History search state (Ctrl+R)
	historySearch   bool     // true when in incremental history search mode
	searchQuery     string   // current search query
	searchResults   []string // filtered history entries matching query
	searchResultIdx int      // current index in searchResults

	// Async execution state
	execStart    time.Time // when the current query started executing
	spinnerFrame int       // current frame index of the spinner animation

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

	// Favorites state
	persistedFavorites map[string]config.FavoriteConfig // favorites loaded from file
	tempFavorites      map[string]config.FavoriteConfig // session-only favorites (save failed)

	// Pagination state (query results)
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

	// Session variables (for \set / \get)
	sessionVars map[string]string

	// Special session variables (controlled via \set)
	autoCommit  bool      // AUTOCOMMIT: auto-commit each statement (default: on)
	onErrorStop bool      // ON_ERROR_STOP: stop script on error (default: off)
	echoMode    echoModeT // ECHO: echo SQL before execution

	// Result title (for \T)
	resultTitle string

	// Show column header (for \pset header)
	showHeader bool

	// Verbose mode (for \verbose)
	verboseMode bool

	// Show warnings (for \warn)
	showWarnings bool

	// Styles
	promptStyle  lipgloss.Style
	outputStyle  lipgloss.Style
	errorStyle   lipgloss.Style
	compStyle    lipgloss.Style
	compSelStyle lipgloss.Style
}

// NewModel creates a new TUI model with the given dependencies.
func NewModel(deps Dependencies) Model {
	cfg := deps.Config
	prompt := cfg.UI.Prompt
	mlPrompt := cfg.UI.MultilinePrompt

	m := Model{
		deps:               deps,
		ed:                 editor.New(),
		prompt:             prompt,
		mlPrompt:           mlPrompt,
		output:             []string{},
		cursorOn:           true,
		mouseEnabled:       false,
		autoVerticalOutput: deps.AutoVerticalOutput,
		connected:          true,
		safeUpdates:        deps.Config.Safety.SafeUpdates,
		showTiming:         true,
		showHeader:         true,
		sessionVars:        make(map[string]string),
		autoCommit:         true,
		onErrorStop:        false,
		echoMode:           echoOff,
		verboseMode:        false,
		showWarnings:       true,
		workDir:            "", // will be set on first prompt
		promptStyle:        lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true),
		outputStyle:        lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		errorStyle:         lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
		compStyle:          lipgloss.NewStyle().Foreground(lipgloss.Color("246")),
		compSelStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("62")).Bold(true),
	}
	_ = cfg.Theme // theme is used via Highlighter

	// Migrate favorites from main config to standalone file (if needed)
	_ = config.MigrateFavoritesFromConfig(cfg)

	// Load persisted favorites from standalone file
	persistedFavs, _ := config.LoadFavorites()
	if persistedFavs == nil {
		persistedFavs = make(map[string]config.FavoriteConfig)
	}
	m.persistedFavorites = persistedFavs

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

// execMultiResultMsg is sent when multiple async SQL statements complete.
type execMultiResultMsg struct {
	results []*executor.QueryResult
	err     error
	format  output.Format
	stmts   []string // original SQL text of each statement
	// perFormat[i] is the format override for stmts[i] (nil means use global format)
	perFormat []output.Format
	stopped   bool // true if execution was stopped by ON_ERROR_STOP
}

// editFinishedMsg is sent when the external editor (\edit) has exited.
// The bubbletea runtime takes care of releasing/restoring the terminal
// around the editor process, so we just need to read back the temp file
// and execute the SQL here.
type editFinishedMsg struct {
	tmpPath string
	err     error
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
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		var newM tea.Model
		newM, cmd = m.handleKey(msg)
		m = newM.(Model)

	case tea.MouseMsg:
		var newM tea.Model
		newM, cmd = m.handleMouse(msg)
		m = newM.(Model)

	case blinkMsg:
		m.cursorOn = !m.cursorOn
		cmd = blinkCmd()

	case healthCheckMsg:
		if m.deps.Pool != nil {
			m.connected = m.deps.Pool.IsConnected()
		}
		cmd = healthCheckCmd()

	case tickMsg:
		m.executing = false

	case execResultMsg:
		m.executing = false
		m.execStart = time.Time{}
		m.spinnerFrame = 0
		cmd = m.displayQueryResult(msg.result, msg.err, msg.formatOverride)

	case editFinishedMsg:
		var newM tea.Model
		newM, cmd = m.handleEditFinished(msg)
		m = newM.(Model)

	case execMultiResultMsg:
		m.executing = false
		m.execStart = time.Time{}
		m.spinnerFrame = 0
		if msg.err != nil {
			m.addOutput(fmt.Sprintf("%s", msg.err))
			if msg.stopped {
				m.addOutput("Execution stopped (ON_ERROR_STOP is on).")
			}
		} else {
			for i, result := range msg.results {
				if i > 0 {
					m.addOutput("") // blank line between results
				}
				// Use per-statement format override if available
				var stmtFmt output.Format
				if i < len(msg.perFormat) {
					stmtFmt = msg.perFormat[i]
				} else {
					stmtFmt = msg.format
				}
				if lastCmd := m.displayQueryResult(result, nil, stmtFmt); lastCmd != nil {
					cmd = lastCmd
				}
			}
		}

	case execTickMsg:
		if m.executing {
			m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
			cmd = tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
				return execTickMsg(t)
			})
		}

	case watchTickMsg:
		if m.watching {
			m.watchTick++
			cmd = m.executeWatchQuery(m.watchTick)
		}

	case watchResultMsg:
		if m.watching {
			// Clear the terminal screen for watch mode (show only latest result)
			m.pendingPrintLines = append(m.pendingPrintLines, "\033[2J\033[H")
			m.output = nil
			m.addOutput(fmt.Sprintf("\033[2m--- Watch #%d (%s) ---\033[0m", msg.tick, time.Now().Format("15:04:05")))

			if msg.err != nil {
				m.addOutput(fmt.Sprintf("Error: %s", msg.err))
				cmd = watchTickCmd(m.watchInterval)
			} else {
				resultCmd := m.displayQueryResult(msg.result, msg.err, m.watchFormat)
				tickCmd := watchTickCmd(m.watchInterval)
				cmd = tea.Batch(resultCmd, tickCmd)
			}
		}
	}

	// Flush any pending output lines via tea.Println so they scroll naturally
	if flushCmd := m.flushPrintLines(); flushCmd != nil {
		if cmd != nil {
			cmd = tea.Batch(cmd, flushCmd)
		} else {
			cmd = flushCmd
		}
	}

	return m, cmd
}

// handleMouse processes mouse events.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if !m.mouseEnabled {
		return m, nil
	}
	// Scrolling is handled by terminal's scrollback buffer
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
		m.rollbackIfInTransaction()
		m.quitting = true
		return m, tea.Quit

	case tea.KeyCtrlD:
		if m.ed.IsEmpty() {
			m.rollbackIfInTransaction()
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

	case tea.KeyCtrlJ:
		// Ctrl+J (0x0A, \n) arrives when pasting multi-line text without
		// bracketed paste. Insert a newline character so the pasted text
		// preserves its original line structure in the editor.
		m.ed.Insert("\n")
		m.showComp = false
		return m, nil

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
		// Scrolling handled by terminal's scrollback buffer
		return m, nil

	case tea.KeyPgDown:
		// Scrolling handled by terminal's scrollback buffer
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
		// Normalize line endings in pasted text:
		// Many terminals send only \r (CR) as the line break inside a
		// bracketed paste (e.g. macOS Terminal, iTerm2). Convert any
		// \r\n or lone \r into \n so the editor stores a consistent
		// representation.
		runes := msg.Runes
		var b strings.Builder
		b.Grow(len(runes))
		for i := 0; i < len(runes); i++ {
			r := runes[i]
			if r == '\r' {
				// Collapse \r\n into a single \n
				if i+1 < len(runes) && runes[i+1] == '\n' {
					i++
				}
				b.WriteRune('\n')
			} else {
				b.WriteRune(r)
			}
		}
		insertText := b.String()

		// Multi-line paste: process each line through the multiline
		// accumulation mechanism (like MySQL cli). This ensures each
		// line is displayed in the output area with the appropriate
		// prompt, and the last line remains in the editor for editing.
		lines := strings.Split(insertText, "\n")
		if len(lines) > 1 {
			// Trim trailing empty lines from pasted text.
			// When users copy SQL that ends with a trailing newline
			// (e.g., from an editor), Split produces a final empty
			// string element. Leaving this empty string in the editor
			// prevents the SQL from being auto-executed even though
			// the statement is already complete (ends with ;).
			for len(lines) > 1 && strings.TrimSpace(lines[len(lines)-1]) == "" {
				lines = lines[:len(lines)-1]
			}

			// First line uses the primary prompt
			firstLine := lines[0]
			m.addOutput(m.prompt + m.highlightDisplayEntry(firstLine))
			m.multilineParts = append(m.multilineParts, firstLine)
			m.multiline = true

			// Middle lines use the continuation prompt
			for i := 1; i < len(lines)-1; i++ {
				m.addOutput(m.mlPrompt + m.highlightDisplayEntry(lines[i]))
				m.multilineParts = append(m.multilineParts, lines[i])
			}

			// Last line stays in the editor for editing
			lastLine := lines[len(lines)-1]
			m.ed.SetText(lastLine)

			// Auto-execute: if the last line ends with ';' (SQL is
			// complete), execute immediately instead of leaving it
			// in the editor. This is especially important when the
			// pasted text had a trailing newline — the user already
			// considers the SQL finished and expects results.
			lastTrimmed := strings.TrimSpace(lastLine)
			if strings.HasSuffix(lastTrimmed, ";") {
				// Combine all parts and execute
				fullInput := strings.Join(append(m.multilineParts, lastLine), "\n")
				m.multilineParts = nil
				m.multiline = false
				m.ed.Clear()
				return m.executeInput(fullInput, output.FormatTable)
			}
		} else {
			// Single-line paste: just insert into editor
			m.ed.Insert(insertText)
		}
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
	case 10: // \n — insert newline (handles paste without bracketed paste)
		m.ed.Insert("\n")
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
		return strings.TrimSuffix(input, "\\G"), output.FormatVertical
	}
	if strings.HasSuffix(input, "\\j") {
		return strings.TrimSuffix(input, "\\j"), output.FormatJSON
	}
	if strings.HasSuffix(input, "\\m") {
		return strings.TrimSuffix(input, "\\m"), output.FormatMarkdown
	}
	return input, output.FormatTable
}

// splitStatementsWithFormats splits SQL text by semicolons and extracts per-statement format overrides.
// Returns both the statement list and a parallel list of format overrides (nil entries mean "use global").
func (m *Model) splitStatementsWithFormats(sql string) ([]string, []output.Format) {
	rawStmts := executor.SplitStatements(sql)
	var stmts []string
	var perFmt []output.Format

	for _, s := range rawStmts {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		trimmed, fmt := parseFormatSuffix(s)
		stmts = append(stmts, trimmed)
		perFmt = append(perFmt, fmt)
	}
	return stmts, perFmt
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

// rollbackIfInTransaction rolls back any active transaction before exiting.
func (m *Model) rollbackIfInTransaction() {
	if m.deps.Executor != nil && m.deps.Executor.InTransaction() {
		if err := m.deps.Executor.RollbackTransaction(); err != nil {
			m.addOutput(fmt.Sprintf("Warning: rollback on exit failed: %s", err))
		} else {
			m.addOutput("Transaction rolled back on exit.")
		}
	}
}

// highlightDisplayEntry applies syntax highlighting to the SQL portion of a display entry,
// preserving format suffixes (\G, \j, \m) and semicolons outside the highlighted SQL.
func (m Model) highlightDisplayEntry(entry string) string {
	if m.deps.Highlighter == nil {
		return entry
	}

	// Separate format suffix if present
	suffix := ""
	trimmed := entry
	if strings.HasSuffix(entry, "\\G") {
		suffix = "\\G"
		trimmed = strings.TrimSuffix(entry, "\\G")
	} else if strings.HasSuffix(entry, "\\j") {
		suffix = "\\j"
		trimmed = strings.TrimSuffix(entry, "\\j")
	} else if strings.HasSuffix(entry, "\\m") {
		suffix = "\\m"
		trimmed = strings.TrimSuffix(entry, "\\m")
	}

	// Highlight the SQL portion
	highlighted := m.deps.Highlighter.Highlight(trimmed)

	// Re-append the format suffix (unhighlighted)
	if suffix != "" {
		highlighted += suffix
	}

	return highlighted
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
	// Preserve newlines in pasted text to maintain SQL comment semantics.
	// Single-line comments (--) extend to end of line, so \n must be preserved
	// to prevent comments from consuming subsequent SQL text.

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

	// Check for pending prompt capture (from \prompt command)
	if m.sessionVars != nil {
		if varName, ok := m.sessionVars["__prompt_var"]; ok && strings.TrimSpace(input) != "" {
			delete(m.sessionVars, "__prompt_var")
			trimmedInput := strings.TrimSpace(input)
			// Don't capture backslash commands as prompt input
			if !strings.HasPrefix(trimmedInput, "\\") {
				m.sessionVars[varName] = trimmedInput
				m.addOutput(fmt.Sprintf("  %s = %s", varName, trimmedInput))
				m.ed.Clear()
				return m, nil
			}
		}
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
		m.rollbackIfInTransaction()
		m.quitting = true
		return m, tea.Quit
	}
	if lowerCmd == "clear" {
		m.ed.Clear()
		m.output = nil
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
		m.addOutput(m.prompt + m.highlightDisplayEntry(input))
		m.multilineParts = append(m.multilineParts, input)
		m.ed.Clear()
		m.multiline = true
		return m, nil
	}

	// Execute the SQL — combine all multi-line parts with the final line
	// Use newline as separator to preserve SQL comment semantics
	// (e.g., "-- comment" only comments to end of line, which requires \n)
	m.multiline = false
	m.showComp = false
	fullInput := strings.Join(append(m.multilineParts, trimmed), "\n")
	m.multilineParts = nil
	return m.executeInput(fullInput, formatOverride)
}

// handleBackslashCommand processes built-in backslash commands.
func (m Model) handleBackslashCommand(cmd string) (tea.Model, tea.Cmd) {
	// Echo the command to output area before clearing input
	m.addOutput(m.prompt + cmd)

	// Save the current input buffer before clearing, so \copy sql can access it
	inputBuffer := m.ed.Text()
	m.ed.Clear()
	m.multiline = false

	// Save to history so Up/Down can recall backslash commands
	// (done after command processing to avoid current command appearing in \history)
	parts := splitArgs(cmd)
	command := parts[0]

	// Strip trailing semicolons from arguments for most commands
	// (users may type \use mydb; out of habit).
	// Exception: \sys and \pipe where semicolons are shell operators.
	if command != "\\sys" && command != "\\!" && command != "\\pipe" && command != "\\|" {
		for i := 1; i < len(parts); i++ {
			parts[i] = strings.TrimRight(parts[i], ";")
		}
	}

	switch command {
	case "\\quit", "\\q":
		m.rollbackIfInTransaction()
		m.deps.History.Append(strings.TrimSpace(cmd))
		m.quitting = true
		return m, tea.Quit

	case "\\help", "\\h", "\\?":
		m.addOutput(helpText())

	case "\\clear", "\\c":
		m.output = nil
		m.pendingPrintLines = append(m.pendingPrintLines, "\033[2J\033[H")

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

	case "\\rollback":
		if m.deps.Executor == nil || !m.deps.Executor.InTransaction() {
			m.addOutput("Not in a transaction.")
		} else {
			if err := m.deps.Executor.RollbackTransaction(); err != nil {
				m.addOutput(fmt.Sprintf("Rollback failed: %s", err))
			} else {
				m.addOutput("Transaction rolled back.")
			}
		}

	case "\\desc", "\\d":
		if len(parts) < 2 {
			// \d without arguments lists tables (psql behavior)
			m.handleListTables("")
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
			m.deps.History.Append(strings.TrimSpace(cmd))
			return m, nil
		}
		m.deps.History.Append(strings.TrimSpace(cmd))
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

	case "\\fav", "\\favorites":
		if len(parts) < 2 {
			m.listFavorites()
		} else {
			switch parts[1] {
			case "+", "add":
				if len(parts) < 3 {
					m.addOutput("Usage: \\fav + <name> [description]")
					m.addOutput("  Saves the last query as a favorite.")
				} else {
					desc := ""
					if len(parts) >= 4 {
						desc = strings.Join(parts[3:], " ")
					}
					m.addFavorite(parts[2], desc)
				}
			case "-", "del", "rm", "delete":
				if len(parts) < 3 {
					m.addOutput("Usage: \\fav - <name>")
				} else {
					m.deleteFavorite(parts[2])
				}
			case "run", "exec":
				if len(parts) < 3 {
					m.addOutput("Usage: \\fav run <name>")
				} else {
					return m.runFavorite(parts[2])
				}
			case "show":
				if len(parts) < 3 {
					m.addOutput("Usage: \\fav show <name>")
				} else {
					m.showFavorite(parts[2])
				}
			default:
				// Show favorite SQL by name (safe default)
				m.showFavorite(parts[1])
			}
		}

	case "\\edit", "\\e":
		m.deps.History.Append(strings.TrimSpace(cmd))
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
			m.handleCopy(parts[1], inputBuffer)
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
		// Use raw text after the command (preserves shell operators like ;, |, &&)
		sysCmd := strings.TrimSpace(strings.TrimPrefix(cmd, command))
		if sysCmd == "" {
			m.addOutput("Usage: \\sys <command> [args...]")
			m.addOutput("  Execute a system command from within mysh.")
			m.addOutput("  Example: \\sys ls -la")
			m.addOutput("  Example: \\sys ls; pwd")
		} else {
			m.handleSys([]string{sysCmd})
		}

	case "\\l", "\\list", "\\databases":
		m.handleListDatabases()

	case "\\dt", "\\tables":
		var pattern string
		if len(parts) >= 2 {
			pattern = parts[1]
		}
		m.handleListTables(pattern)

	case "\\echo":
		text := strings.TrimSpace(strings.TrimPrefix(cmd, command))
		// Support :varname substitution in \echo (psql-compatible)
		text = m.substituteVarsInText(text)
		m.addOutput(text)

	case "\\conninfo":
		m.addOutput(m.formatConnInfo())

	case "\\x", "\\expanded":
		m.autoVerticalOutput = !m.autoVerticalOutput
		if m.autoVerticalOutput {
			m.addOutput("Expanded display is on.")
		} else {
			m.addOutput("Expanded display is off.")
		}

	case "\\dn", "\\schemas":
		m.handleListSchemas()

	case "\\du", "\\users":
		m.handleListUsers()

	case "\\di", "\\indexes":
		var table string
		if len(parts) >= 2 {
			table = parts[1]
		}
		m.handleListIndexes(table)

	case "\\dv", "\\views":
		var pattern string
		if len(parts) >= 2 {
			pattern = parts[1]
		}
		m.handleListViews(pattern)

	case "\\set":
		m.handleSet(parts[1:])

	case "\\get":
		m.handleGet(parts[1:])

	case "\\unset":
		if len(parts) < 2 {
			m.addOutput("Usage: \\unset <name>")
		} else {
			delete(m.sessionVars, parts[1])
			m.addOutput(fmt.Sprintf("Variable %s unset.", parts[1]))
		}

	case "\\prompt":
		m.handlePrompt(parts[1:])

	case "\\pset":
		m.handlePset(parts[1:])

	case "\\T":
		if len(parts) < 2 {
			if m.resultTitle != "" {
				m.addOutput(fmt.Sprintf("Result title: %s", m.resultTitle))
			} else {
				m.addOutput("No result title set.")
			}
		} else if parts[1] == "off" || parts[1] == "-" {
			m.resultTitle = ""
			m.addOutput("Result title cleared.")
		} else {
			m.resultTitle = strings.Join(parts[1:], " ")
			m.addOutput(fmt.Sprintf("Result title set to: %s", m.resultTitle))
		}

	case "\\g":
		m.handleG(parts[1:])

	case "\\gx":
		// Execute current input or last query with vertical output
		sql := strings.TrimSpace(m.ed.Text())
		if sql == "" && m.lastQuery != "" {
			sql = m.lastQuery
		}
		if sql == "" {
			m.addOutput("No query to execute.")
		} else {
			m.ed.Clear()
			return m.executeInput(sql, output.FormatVertical)
		}

	case "\\encoding":
		m.handleEncoding(parts[1:])

	case "\\sf":
		if len(parts) < 2 {
			m.addOutput("Usage: \\sf <function_name>")
		} else {
			m.handleShowFunction(parts[1])
		}

	case "\\privileges":
		if len(parts) < 2 {
			m.addOutput("Usage: \\privileges <table>")
		} else {
			m.handlePrivileges(parts[1])
		}

	case "\\verbose":
		m.verboseMode = !m.verboseMode
		if m.verboseMode {
			m.addOutput("Verbose mode is on.")
		} else {
			m.addOutput("Verbose mode is off.")
		}

	case "\\warn":
		if len(parts) >= 2 {
			switch strings.ToLower(parts[1]) {
			case "on", "1", "true":
				m.showWarnings = true
				m.addOutput("Warnings are on.")
			case "off", "0", "false":
				m.showWarnings = false
				m.addOutput("Warnings are off.")
			default:
				m.addOutput("Usage: \\warn [on|off]")
			}
		} else {
			m.showWarnings = !m.showWarnings
			if m.showWarnings {
				m.addOutput("Warnings are on.")
			} else {
				m.addOutput("Warnings are off.")
			}
		}

	case "\\explain":
		m.deps.History.Append(strings.TrimSpace(cmd))
		return m.handleExplainCmd(parts[1:])

	default:
		m.addOutput(fmt.Sprintf("Unknown command: %s. Type \\help for available commands.", command))
	}

	// Save to history so Up/Down can recall backslash commands
	m.deps.History.Append(strings.TrimSpace(cmd))

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
	return m, nil
}

// executeInput runs the SQL statement asynchronously and displays the result.
// formatOverride specifies a per-query output format (FormatTable = use global default).
func (m Model) executeInput(input string, formatOverride output.Format) (tea.Model, tea.Cmd) {
	// Save original input (with semicolon and format suffix) for history and display
	originalInput := input
	if suffix := formatSuffixString(formatOverride); suffix != "" {
		originalInput = input + suffix
	}

	// Remove trailing semicolons for execution
	input = strings.TrimRight(input, ";")
	input = strings.TrimSpace(input)

	if input == "" {
		m.ed.Clear()
		return m, nil
	}

	// Save to history (use original input to preserve semicolon and format suffix)
	historyEntry := strings.TrimSpace(originalInput)
	m.deps.History.Append(historyEntry)

	// Echo the input line to output area (like mysql CLI), with syntax highlighting
	displayEntry := strings.TrimSpace(originalInput)
	m.addOutput(m.prompt + m.highlightDisplayEntry(displayEntry))

	// Substitute session variables in SQL (:varname, :'varname', :"varname")
	substituted := m.substituteVars(input)

	// ECHO queries/all: show the SQL after variable substitution
	if m.echoMode == echoQueries || m.echoMode == echoAll {
		if substituted != input {
			m.addOutput(fmt.Sprintf("  Substituted: %s", substituted))
		}
	}

	// Normalize table name casing in SQL before execution
	execSQL := m.normalizeTableNames(substituted + ";")

	// AUTOCOMMIT off: automatically begin a transaction before each statement
	// (unless already in a transaction or the statement itself is BEGIN/COMMIT/ROLLBACK)
	if !m.autoCommit && m.deps.Executor != nil && !m.deps.Executor.InTransaction() {
		upper := strings.ToUpper(strings.TrimSpace(input))
		upper = strings.TrimSuffix(upper, ";")
		if !m.isTransactionControl(upper) {
			if _, err := m.deps.Executor.Execute(context.Background(), "BEGIN"); err != nil {
				m.addOutput(fmt.Sprintf("ERROR: %s", err))
				m.ed.Clear()
				return m, nil
			}
		}
	}

	m.executing = true
	m.execStart = time.Now()
	m.ed.Clear()

	// Save the query for \watch
	m.lastQuery = input

	// Check if input contains multiple statements (has internal semicolons)
	// by attempting to split; if more than one, use ExecuteMulti
	stmts, perFmt := m.splitStatementsWithFormats(execSQL)
	if len(stmts) > 1 {
		// Apply the outer formatOverride to the last statement only
		// (e.g., "select 1; select 2\G" → only the 2nd result uses vertical)
		if len(perFmt) > 0 && formatOverride != output.FormatTable {
			perFmt[len(perFmt)-1] = formatOverride
		}
		// Multi-statement execution
		onErrorStop := m.onErrorStop
		return m, tea.Batch(
			func() tea.Msg {
				var results []*executor.QueryResult
				for i, stmt := range stmts {
					result, err := m.deps.Executor.Execute(context.Background(), stmt)
					if err != nil {
						if onErrorStop {
							return execMultiResultMsg{results, err, output.FormatTable, stmts[:i+1], perFmt[:i+1], true}
						}
						return execMultiResultMsg{results, err, output.FormatTable, stmts[:i+1], perFmt[:i+1], false}
					}
					results = append(results, result)
				}
				return execMultiResultMsg{results, nil, output.FormatTable, stmts, perFmt, false}
			},
			tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
				return execTickMsg(t)
			}),
		)
	}

	// Single statement execution
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
// It returns a tea.Cmd that may be non-nil if the result needs to be printed
// directly to terminal via ExecProcess (for large results exceeding terminal height).
func (m *Model) displayQueryResult(result *executor.QueryResult, err error, formatOverride output.Format) tea.Cmd {
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
		} else if m.verboseMode {
			// Verbose mode: show full error with type info
			m.addOutput(fmt.Sprintf("\033[31mERROR (%T):\033[0m %s", err, errMsg))
		} else {
			m.addOutput(fmt.Sprintf("%s", errMsg))
		}
		return nil
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
		if output.CalcTableWidth(result) > m.width {
			outFmt = output.FormatVertical
		}
	}

	// Check if pagination is needed
	pageSize := m.deps.Config.UI.PageSize
	if result.IsQuery && pageSize > 0 && len(result.Rows) > pageSize {
		m.enterPagination(result, outFmt)
		m.addOutput(m.renderPagedResult())
		return nil
	}

	// Format result (no column truncation — data should never be cut off)
	// Show result title if set (via \T)
	if m.resultTitle != "" {
		m.addOutput(m.resultTitle)
	}
	var buf strings.Builder
	formatter := output.NewFormatter(outFmt, &buf)
	formatter.SetShowTiming(m.showTiming)
	formatter.SetShowHeader(m.showHeader)
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
	return nil
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
			m.pagedPage++
			m.addOutput(m.renderPagedResult())
		} else {
			m.exitPagination(true)
		}
	case tea.KeyUp, tea.KeyPgUp:
		if m.pagedPage > 0 {
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
		var buf strings.Builder
		formatter := output.NewFormatter(m.pagedFormat, &buf)
		formatter.WriteResult(m.pagedResult)
		m.addOutput(strings.TrimRight(buf.String(), "\n"))
	}
	m.pagedResult = nil
	m.pagedFormat = output.FormatTable
	m.pagedPage = 0
	m.pagedTotal = 0
}

// handleEditFinished is invoked after the external editor has exited.
// It reads back the edited content from the temp file (path captured in msg),
// removes the temp file, and dispatches the SQL for execution.
func (m Model) handleEditFinished(msg editFinishedMsg) (tea.Model, tea.Cmd) {
	tmpPath := msg.tmpPath
	if tmpPath != "" {
		defer os.Remove(tmpPath)
	}

	if msg.err != nil {
		m.addOutput(fmt.Sprintf("ERROR: Editor failed: %s", msg.err))
		return m, nil
	}

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

	// Parse format suffix (\G, \j, \m) so that e.g. `SELECT ... \G`
	// edited via \edit is treated the same as if typed directly.
	sql, formatOverride := parseFormatSuffix(sql)
	sql = strings.TrimSpace(sql)
	if sql == "" {
		m.addOutput("Empty content, nothing to execute.")
		m.ed.Clear()
		return m, nil
	}

	// Clear the input line and execute the edited SQL
	m.ed.Clear()
	m.multiline = false
	displayEntry := sql
	if suffix := formatSuffixString(formatOverride); suffix != "" {
		displayEntry = sql + suffix
	}
	m.addOutput(m.prompt + m.highlightDisplayEntry(displayEntry))
	return m.executeInput(sql, formatOverride)
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

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		m.addOutput(fmt.Sprintf("ERROR: Failed to write temp file: %s", err))
		return m, nil
	}
	tmpFile.Close()

	// Determine editor
	editorBin := os.Getenv("EDITOR")
	if editorBin == "" {
		editorBin = os.Getenv("VISUAL")
	}
	if editorBin == "" {
		editorBin = "vi"
	}

	// Use tea.ExecProcess so that bubbletea properly releases the terminal
	// (restores cooked mode, stops its own input reader) before launching
	// the editor, and re-captures the terminal afterwards. Without this,
	// keystrokes typed into vi (e.g. the leading 's' of 'select' when
	// pasting) can be swallowed by bubbletea's still-running input loop.
	cmd := exec.Command(editorBin, tmpPath)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editFinishedMsg{tmpPath: tmpPath, err: err}
	})
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

// substituteVars replaces :varname, :'varname', and :"varname" references
// in the input SQL with the corresponding session variable values.
// This is compatible with psql's variable substitution behavior.
func (m Model) substituteVars(input string) string {
	if len(m.sessionVars) == 0 {
		return input
	}

	var result strings.Builder
	runes := []rune(input)
	i := 0
	for i < len(runes) {
		// Skip single-quoted strings (no substitution inside)
		if runes[i] == '\'' {
			j := i + 1
			for j < len(runes) {
				if runes[j] == '\'' {
					j++
					break
				}
				if runes[j] == '\\' && j+1 < len(runes) {
					j++
				}
				j++
			}
			result.WriteString(string(runes[i:j]))
			i = j
			continue
		}
		// Skip double-quoted strings (no substitution inside)
		if runes[i] == '"' {
			j := i + 1
			for j < len(runes) {
				if runes[j] == '"' {
					j++
					break
				}
				if runes[j] == '\\' && j+1 < len(runes) {
					j++
				}
				j++
			}
			result.WriteString(string(runes[i:j]))
			i = j
			continue
		}
		// Look for :varname, :'varname', :"varname"
		if runes[i] == ':' {
			// :'varname' — quoted with single quotes
			if i+2 < len(runes) && runes[i+1] == '\'' {
				endQ := -1
				for k := i + 2; k < len(runes); k++ {
					if runes[k] == '\'' {
						endQ = k
						break
					}
				}
				if endQ != -1 {
					varName := string(runes[i+2 : endQ])
					if val, ok := m.sessionVars[varName]; ok {
						// Escape single quotes within the value
						escaped := strings.ReplaceAll(val, "'", "''")
						result.WriteString("'" + escaped + "'")
					} else {
						result.WriteString(string(runes[i : endQ+1]))
					}
					i = endQ + 1
					continue
				}
			}
			// :"varname" — quoted with double quotes
			if i+2 < len(runes) && runes[i+1] == '"' {
				endQ := -1
				for k := i + 2; k < len(runes); k++ {
					if runes[k] == '"' {
						endQ = k
						break
					}
				}
				if endQ != -1 {
					varName := string(runes[i+2 : endQ])
					if val, ok := m.sessionVars[varName]; ok {
						// Escape double quotes within the value
						escaped := strings.ReplaceAll(val, `"`, `\"`)
						result.WriteString(`"` + escaped + `"`)
					} else {
						result.WriteString(string(runes[i : endQ+1]))
					}
					i = endQ + 1
					continue
				}
			}
			// :varname — plain variable reference
			if i+1 < len(runes) && isIdentStart(runes[i+1]) {
				j := i + 1
				for j < len(runes) && isIdentRune(runes[j]) {
					j++
				}
				varName := string(runes[i+1 : j])
				if val, ok := m.sessionVars[varName]; ok {
					result.WriteString(val)
				} else {
					result.WriteString(string(runes[i:j]))
				}
				i = j
				continue
			}
		}
		result.WriteRune(runes[i])
		i++
	}
	return result.String()
}

func isIdentRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

// isTransactionControl returns true if the SQL is a transaction control statement.
func (m Model) isTransactionControl(upper string) bool {
	return strings.HasPrefix(upper, "BEGIN") ||
		strings.HasPrefix(upper, "START TRANSACTION") ||
		strings.HasPrefix(upper, "COMMIT") ||
		strings.HasPrefix(upper, "ROLLBACK")
}

// substituteVarsInText substitutes :varname references in plain text
// (used by \echo). Unlike substituteVars, it does not skip quoted strings
// and does not support :'varname' or :"varname" forms.
func (m Model) substituteVarsInText(text string) string {
	if len(m.sessionVars) == 0 {
		return text
	}
	var result strings.Builder
	runes := []rune(text)
	i := 0
	for i < len(runes) {
		if runes[i] == ':' && i+1 < len(runes) && isIdentStart(runes[i+1]) {
			j := i + 1
			for j < len(runes) && isIdentRune(runes[j]) {
				j++
			}
			varName := string(runes[i+1 : j])
			if val, ok := m.sessionVars[varName]; ok {
				result.WriteString(val)
			} else {
				result.WriteString(string(runes[i:j]))
			}
			i = j
			continue
		}
		result.WriteRune(runes[i])
		i++
	}
	return result.String()
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
		return "Goodbye!\n"
	}

	var sb strings.Builder

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
		// Transaction indicator
		txIndicator := ""
		if m.deps.Executor != nil && m.deps.Executor.InTransaction() {
			txIndicator = "\033[1;33mtx>\033[0m "
		}
		currentPrompt := healthIndicator + txIndicator + m.prompt
		sb.WriteString(m.promptStyle.Render(currentPrompt))

		input := m.ed.Text()
		cursorLine := m.ed.CurrentLine()
		cursorCol := m.ed.CursorColumn()
		lines := strings.Split(input, "\n")

		// Cursor: overlay on character at cursor position using reverse video
		cursorStyle := lipgloss.NewStyle().Reverse(true)

		// Determine available width for soft-wrapping the input.
		// The first visual row of each logical line shares space with the prompt;
		// continuation rows are indented to match the prompt's display width.
		firstPromptW := lipgloss.Width(currentPrompt)
		mlPromptW := lipgloss.Width(m.mlPrompt)
		// Indent continuation rows so they line up under the input area
		// (i.e., right after the visible prompt). Use spaces of equal width.
		contIndent := strings.Repeat(" ", firstPromptW)

		if input == "" {
			if m.cursorOn {
				sb.WriteString(cursorStyle.Render(" "))
			} else {
				sb.WriteString(" ")
			}
		} else {
			for i, line := range lines {
				// Pick the prompt width that this logical line starts with
				lineStartW := firstPromptW
				if i > 0 {
					sb.WriteString("\n")
					sb.WriteString(m.promptStyle.Render(m.mlPrompt))
					lineStartW = mlPromptW
				}

				lineRunes := []rune(line)

				// Compute soft-wrap segments for this logical line.
				// segs[k] = (startRuneIdx, endRuneIdx) of segment k.
				// The first segment uses (m.width - lineStartW) columns,
				// subsequent segments use (m.width - firstPromptW) columns.
				type seg struct{ start, end int }
				var segs []seg
				if m.width <= 0 {
					// Width unknown: fall back to a single segment (no wrapping).
					segs = []seg{{0, len(lineRunes)}}
				} else {
					firstAvail := m.width - lineStartW
					contAvail := m.width - firstPromptW
					if firstAvail < 1 {
						firstAvail = 1
					}
					if contAvail < 1 {
						contAvail = 1
					}
					avail := firstAvail
					segStart := 0
					curW := 0
					for r := 0; r < len(lineRunes); r++ {
						w := runewidth.RuneWidth(lineRunes[r])
						if w == 0 {
							w = 1
						}
						if curW+w > avail {
							segs = append(segs, seg{segStart, r})
							segStart = r
							curW = 0
							avail = contAvail
						}
						curW += w
					}
					segs = append(segs, seg{segStart, len(lineRunes)})
				}

				for sIdx, s := range segs {
					if sIdx > 0 {
						// Continuation visual row of this same logical line.
						sb.WriteString("\n")
						sb.WriteString(contIndent)
					}
					segText := string(lineRunes[s.start:s.end])

					// Decide whether the cursor falls in this segment.
					cursorInSeg := false
					localCol := 0
					atSegEnd := false
					if i == cursorLine && m.cursorOn {
						// Cursor at end of last segment of last logical line
						isLastSeg := sIdx == len(segs)-1
						if cursorCol >= s.start && cursorCol < s.end {
							cursorInSeg = true
							localCol = cursorCol - s.start
						} else if cursorCol == s.end && isLastSeg {
							cursorInSeg = true
							localCol = cursorCol - s.start
							atSegEnd = true
						}
					}

					if cursorInSeg {
						segRunes := []rune(segText)
						if atSegEnd {
							// Block cursor after the last character
							if m.deps.Highlighter != nil {
								sb.WriteString(m.deps.Highlighter.Highlight(segText))
							} else {
								sb.WriteString(segText)
							}
							sb.WriteString(cursorStyle.Render(" "))
						} else {
							before := string(segRunes[:localCol])
							ch := string(segRunes[localCol])
							after := string(segRunes[localCol+1:])
							if m.deps.Highlighter != nil {
								sb.WriteString(m.deps.Highlighter.Highlight(before))
								sb.WriteString(cursorStyle.Render(ch))
								sb.WriteString(m.deps.Highlighter.Highlight(after))
							} else {
								sb.WriteString(before)
								sb.WriteString(cursorStyle.Render(ch))
								sb.WriteString(after)
							}
						}
					} else {
						if m.deps.Highlighter != nil {
							sb.WriteString(m.deps.Highlighter.Highlight(segText))
						} else {
							sb.WriteString(segText)
						}
					}
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

// addOutput appends lines to the output buffer and queues them for
// printing via tea.Println. Output is printed directly to the terminal
// (scrolling naturally) rather than being re-rendered by View() each frame.
func (m *Model) addOutput(text string) {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		m.output = append(m.output, line)
		m.pendingPrintLines = append(m.pendingPrintLines, line)
	}
	// Cap the output buffer to prevent excessive memory usage.
	// The terminal's scrollback buffer will preserve older content.
	const maxOutputBufferLines = 10000
	if len(m.output) > maxOutputBufferLines {
		m.output = m.output[len(m.output)-maxOutputBufferLines:]
	}
}

// flushPrintLines returns a tea.Cmd that prints all pending output lines
// directly to the terminal via tea.Println. After flushing, pendingPrintLines
// is cleared. This ensures output scrolls naturally in the terminal.
func (m *Model) flushPrintLines() tea.Cmd {
	if len(m.pendingPrintLines) == 0 {
		return nil
	}
	text := strings.Join(m.pendingPrintLines, "\n")
	m.pendingPrintLines = nil
	return tea.Println(text)
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

// listFavorites displays all saved favorite queries.
func (m *Model) listFavorites() {
	count := 0

	// Persisted favorites
	if m.persistedFavorites != nil {
		for name, fav := range m.persistedFavorites {
			preview := fav.SQL
			if len(preview) > 60 {
				preview = preview[:57] + "..."
			}
			desc := ""
			if fav.Description != "" {
				desc = fmt.Sprintf("  \033[2m(%s)\033[0m", fav.Description)
			}
			m.addOutput(fmt.Sprintf("  \033[1m%-20s\033[0m %s%s", name, preview, desc))
			count++
		}
	}

	// Temp favorites (session-only, saved when persist failed)
	if m.tempFavorites != nil {
		for name, fav := range m.tempFavorites {
			preview := fav.SQL
			if len(preview) > 60 {
				preview = preview[:57] + "..."
			}
			desc := ""
			if fav.Description != "" {
				desc = fmt.Sprintf("  \033[2m(%s)\033[0m", fav.Description)
			}
			m.addOutput(fmt.Sprintf("  \033[1m%-20s\033[0m %s%s \033[33m(temp)\033[0m", name, preview, desc))
			count++
		}
	}

	if count == 0 {
		m.addOutput("No favorites saved. Use \\fav + <name> [desc] to save the last query.")
	} else {
		m.addOutput(fmt.Sprintf("%d favorite(s). \\fav <name> to view, \\fav run <name> to execute.", count))
	}
}

// resolveFavorite finds a favorite by name (temp first, then persisted).
func (m *Model) resolveFavorite(name string) (config.FavoriteConfig, bool) {
	if m.tempFavorites != nil {
		if fav, ok := m.tempFavorites[name]; ok {
			return fav, true
		}
	}
	if m.persistedFavorites != nil {
		if fav, ok := m.persistedFavorites[name]; ok {
			return fav, true
		}
	}
	return config.FavoriteConfig{}, false
}

// addFavorite saves the last query as a favorite.
func (m *Model) addFavorite(name, description string) {
	if m.lastQuery == "" {
		m.addOutput("No last query to save. Run a query first.")
		return
	}

	now := time.Now().Unix()
	fav := config.FavoriteConfig{
		SQL:         m.lastQuery,
		Description: description,
		LastUsed:    now,
	}

	if m.persistedFavorites == nil {
		m.persistedFavorites = make(map[string]config.FavoriteConfig)
	}
	m.persistedFavorites[name] = fav

	if err := config.SaveFavorites(m.persistedFavorites); err != nil {
		// Fallback to temp-only
		if m.tempFavorites == nil {
			m.tempFavorites = make(map[string]config.FavoriteConfig)
		}
		m.tempFavorites[name] = fav
		delete(m.persistedFavorites, name)
		m.addOutput(fmt.Sprintf("Save failed (%s), saved as temp favorite.", err))
	}

	desc := ""
	if description != "" {
		desc = fmt.Sprintf(" (%s)", description)
	}
	m.addOutput(fmt.Sprintf("Favorite '%s' saved%s.", name, desc))
}

// deleteFavorite removes a favorite by name.
func (m *Model) deleteFavorite(name string) {
	deleted := false

	if m.tempFavorites != nil {
		if _, ok := m.tempFavorites[name]; ok {
			delete(m.tempFavorites, name)
			deleted = true
		}
	}

	if m.persistedFavorites != nil {
		if _, ok := m.persistedFavorites[name]; ok {
			delete(m.persistedFavorites, name)
			_ = config.SaveFavorites(m.persistedFavorites)
			deleted = true
		}
	}

	if deleted {
		m.addOutput(fmt.Sprintf("Favorite '%s' deleted.", name))
	} else {
		m.addOutput(fmt.Sprintf("Favorite '%s' not found.", name))
	}
}

// showFavorite displays the favorite name and puts the SQL into the input buffer,
// so the user can press Enter to execute or edit before running.
func (m *Model) showFavorite(name string) {
	fav, ok := m.resolveFavorite(name)
	if !ok {
		m.addOutput(fmt.Sprintf("Favorite '%s' not found.", name))
		return
	}
	header := fmt.Sprintf("Favorite: \033[1m%s\033[0m", name)
	if fav.Description != "" {
		header += fmt.Sprintf("  \033[2m(%s)\033[0m", fav.Description)
	}
	m.addOutput(header)
	m.ed.SetText(fav.SQL)
}

// runFavorite executes a saved favorite query.
func (m Model) runFavorite(name string) (tea.Model, tea.Cmd) {
	fav, ok := m.resolveFavorite(name)
	if !ok {
		m.addOutput(fmt.Sprintf("Favorite '%s' not found. Use \\fav to list.", name))
		return m, nil
	}
	// Update LastUsed timestamp for LRU
	if m.persistedFavorites != nil {
		if _, exists := m.persistedFavorites[name]; exists {
			m.persistedFavorites[name] = config.FavoriteConfig{
				SQL:         fav.SQL,
				Description: fav.Description,
				LastUsed:    time.Now().Unix(),
			}
			_ = config.SaveFavorites(m.persistedFavorites)
		}
	}
	// Show which favorite is being run
	m.addOutput(fmt.Sprintf("\033[2m→ fav %s: %s\033[0m", name, truncateStr(fav.SQL, 60)))
	m.ed.Clear()
	return m.executeInput(fav.SQL, output.FormatTable)
}

// handleListDatabases lists all databases (like psql's \l or MySQL's SHOW DATABASES).
// For PostgreSQL, also shows schemas in the current database.
func (m *Model) handleListDatabases() {
	if m.deps.Pool == nil {
		m.addOutput("No connection available.")
		return
	}
	dbs, err := m.deps.Pool.Databases()
	if err != nil {
		m.addOutput(fmt.Sprintf("ERROR: %s", err))
		return
	}

	if len(dbs) == 0 {
		m.addOutput("No databases found.")
	} else {
		m.addOutput(fmt.Sprintf("  List of databases (%d):", len(dbs)))
		curDB := m.deps.Pool.CurrentDB()
		for _, db := range dbs {
			if db == curDB {
				m.addOutput(fmt.Sprintf("   %s  [current]", db))
			} else {
				m.addOutput(fmt.Sprintf("   %s", db))
			}
		}
	}

	// For PostgreSQL, also show schemas in the current database
	if m.deps.Pool.DriverName() == "postgres" {
		schemas, err := m.deps.Pool.Schemas()
		if err != nil {
			return
		}
		if len(schemas) > 0 {
			currentSchema := m.deps.Pool.CurrentSchema()
			m.addOutput("")
			m.addOutput(fmt.Sprintf("  Schemas in database %q (%d):", m.deps.Pool.CurrentDB(), len(schemas)))
			for _, s := range schemas {
				if s == currentSchema {
					m.addOutput(fmt.Sprintf("   %s  [current]", s))
				} else {
					m.addOutput(fmt.Sprintf("   %s", s))
				}
			}
		}
	}
}

// handleListTables lists tables in the current database, optionally filtered by pattern.
func (m *Model) handleListTables(pattern string) {
	if m.deps.Pool == nil {
		m.addOutput("No connection available.")
		return
	}
	schema := m.deps.Pool.CurrentSchema()
	tables, err := m.deps.Pool.Tables(schema)
	if err != nil {
		m.addOutput(fmt.Sprintf("ERROR: %s", err))
		return
	}

	// Filter by pattern if provided
	if pattern != "" {
		// Convert SQL-style % wildcard to glob * for matching
		globPattern := strings.ReplaceAll(pattern, "%", "*")
		var filtered []string
		for _, t := range tables {
			matched, _ := filepath.Match(globPattern, t)
			if matched {
				filtered = append(filtered, t)
			}
		}
		// If no glob match, try case-insensitive prefix match for usability
		if len(filtered) == 0 {
			prefix := strings.TrimRight(pattern, "*%")
			if prefix != "" {
				for _, t := range tables {
					if strings.HasPrefix(strings.ToLower(t), strings.ToLower(prefix)) {
						filtered = append(filtered, t)
					}
				}
			}
		}
		tables = filtered
	}

	if len(tables) == 0 {
		if pattern != "" {
			m.addOutput(fmt.Sprintf("No tables found matching %q in schema %q.", pattern, schema))
		} else {
			m.addOutput(fmt.Sprintf("No tables found in schema %q.", schema))
		}
		return
	}

	label := fmt.Sprintf("  List of relations (%d) in %s:", len(tables), schema)
	if pattern != "" {
		label = fmt.Sprintf("  List of relations (%d) matching %q in %s:", len(tables), pattern, schema)
	}
	m.addOutput(label)
	for _, t := range tables {
		m.addOutput(fmt.Sprintf("   %s", t))
	}
}

// formatConnInfo returns detailed connection information (like psql's \conninfo).
func (m Model) formatConnInfo() string {
	if m.deps.Pool == nil || m.deps.Config == nil {
		return "No connection."
	}
	cfg := m.deps.Config.Connection
	db := m.deps.Pool.CurrentDB()

	driverLabel := cfg.Driver
	if driverLabel == "" {
		driverLabel = "mysql"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("You are connected to database %q", db))
	sb.WriteString(fmt.Sprintf(" as user %q", cfg.User))
	sb.WriteString(fmt.Sprintf(" on host %q", cfg.Host))
	sb.WriteString(fmt.Sprintf(" at port %q", fmt.Sprintf("%d", cfg.Port)))
	sb.WriteString(fmt.Sprintf(" via driver %q", driverLabel))
	if m.deps.SSHTunnel != nil {
		sb.WriteString(fmt.Sprintf(" (SSH tunnel: %s)", m.deps.SSHTunnel.LocalAddr()))
	}
	sb.WriteString(".")
	return sb.String()
}

// handleListSchemas lists schemas (MySQL: databases, PostgreSQL: schemas).
func (m *Model) handleListSchemas() {
	if m.deps.Pool == nil {
		m.addOutput("No connection available.")
		return
	}
	schemas, err := m.deps.Pool.Schemas()
	if err != nil {
		m.addOutput(fmt.Sprintf("ERROR: %s", err))
		return
	}
	if len(schemas) == 0 {
		m.addOutput("No schemas found.")
		return
	}
	m.addOutput(fmt.Sprintf("  List of schemas (%d):", len(schemas)))
	for _, s := range schemas {
		m.addOutput(fmt.Sprintf("   %s", s))
	}
}

// handleListUsers lists database users.
func (m *Model) handleListUsers() {
	if m.deps.Pool == nil {
		m.addOutput("No connection available.")
		return
	}
	users, err := m.deps.Pool.Users()
	if err != nil {
		m.addOutput(fmt.Sprintf("ERROR: %s", err))
		return
	}
	if len(users) == 0 {
		m.addOutput("No users found.")
		return
	}
	m.addOutput(fmt.Sprintf("  List of users (%d):", len(users)))
	for _, u := range users {
		m.addOutput(fmt.Sprintf("   %s", u))
	}
}

// handleListIndexes lists index information, optionally filtered by table.
func (m *Model) handleListIndexes(table string) {
	if m.deps.Pool == nil {
		m.addOutput("No connection available.")
		return
	}
	indexes, err := m.deps.Pool.ListIndexes(m.deps.Pool.CurrentSchema(), table)
	if err != nil {
		m.addOutput(fmt.Sprintf("ERROR: %s", err))
		return
	}
	if len(indexes) == 0 {
		if table != "" {
			m.addOutput(fmt.Sprintf("No indexes found for table %q.", table))
		} else {
			m.addOutput("No indexes found.")
		}
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
	_ = m.displayQueryResult(result, nil, output.FormatTable)
}

// handleListViews lists views, optionally filtered by pattern.
func (m *Model) handleListViews(pattern string) {
	if m.deps.Pool == nil {
		m.addOutput("No connection available.")
		return
	}
	db := m.deps.Pool.CurrentSchema()
	views, err := m.deps.Pool.Views(db)
	if err != nil {
		m.addOutput(fmt.Sprintf("ERROR: %s", err))
		return
	}

	// Filter by pattern if provided
	if pattern != "" {
		globPattern := strings.ReplaceAll(pattern, "%", "*")
		var filtered []string
		for _, v := range views {
			matched, _ := filepath.Match(globPattern, v)
			if matched {
				filtered = append(filtered, v)
			}
		}
		if len(filtered) == 0 {
			prefix := strings.TrimRight(pattern, "*%")
			if prefix != "" {
				for _, v := range views {
					if strings.HasPrefix(strings.ToLower(v), strings.ToLower(prefix)) {
						filtered = append(filtered, v)
					}
				}
			}
		}
		views = filtered
	}

	if len(views) == 0 {
		if pattern != "" {
			m.addOutput(fmt.Sprintf("No views found matching %q.", pattern))
		} else {
			m.addOutput("No views found.")
		}
		return
	}
	m.addOutput(fmt.Sprintf("  List of views (%d):", len(views)))
	for _, v := range views {
		m.addOutput(fmt.Sprintf("   %s", v))
	}
}

// handleSet sets a session variable.
// Supports special variables that control mysh behavior:
//
//	AUTOCOMMIT   - on/off: auto-commit each statement (default: on)
//	ON_ERROR_STOP - on/off: stop execution on error (default: off)
//	ECHO         - all/queries/off: echo SQL before execution
//
// Also supports :varname substitution in SQL (psql-compatible).
func (m *Model) handleSet(parts []string) {
	if len(parts) == 0 {
		// List all variables, showing special ones first
		m.addOutput("  Special variables:")
		m.addOutput(fmt.Sprintf("   AUTOCOMMIT = %s", boolStr(m.autoCommit)))
		m.addOutput(fmt.Sprintf("   ON_ERROR_STOP = %s", boolStr(m.onErrorStop)))
		echoVal := "off"
		if m.echoMode == echoAll {
			echoVal = "all"
		} else if m.echoMode == echoQueries {
			echoVal = "queries"
		}
		m.addOutput(fmt.Sprintf("   ECHO = %s", echoVal))
		if len(m.sessionVars) > 0 {
			m.addOutput("  User variables:")
			for k, v := range m.sessionVars {
				m.addOutput(fmt.Sprintf("   %s = %s", k, v))
			}
		}
		return
	}
	name := parts[0]
	if len(parts) == 1 {
		// Check special variables first
		switch strings.ToUpper(name) {
		case "AUTOCOMMIT":
			m.addOutput(fmt.Sprintf("  AUTOCOMMIT = %s", boolStr(m.autoCommit)))
			return
		case "ON_ERROR_STOP":
			m.addOutput(fmt.Sprintf("  ON_ERROR_STOP = %s", boolStr(m.onErrorStop)))
			return
		case "ECHO":
			echoVal := "off"
			if m.echoMode == echoAll {
				echoVal = "all"
			} else if m.echoMode == echoQueries {
				echoVal = "queries"
			}
			m.addOutput(fmt.Sprintf("  ECHO = %s", echoVal))
			return
		}
		if val, ok := m.sessionVars[parts[0]]; ok {
			m.addOutput(fmt.Sprintf("  %s = %s", parts[0], val))
		} else {
			m.addOutput(fmt.Sprintf("  Variable %q not set.", parts[0]))
		}
		return
	}
	value := strings.Join(parts[1:], " ")

	// Handle special variables
	switch strings.ToUpper(name) {
	case "AUTOCOMMIT":
		newVal := parseBoolValue(value, m.autoCommit)
		m.autoCommit = newVal
		m.addOutput(fmt.Sprintf("  AUTOCOMMIT = %s", boolStr(newVal)))
		return
	case "ON_ERROR_STOP":
		newVal := parseBoolValue(value, m.onErrorStop)
		m.onErrorStop = newVal
		m.addOutput(fmt.Sprintf("  ON_ERROR_STOP = %s", boolStr(newVal)))
		return
	case "ECHO":
		switch strings.ToLower(value) {
		case "all":
			m.echoMode = echoAll
			m.addOutput("  ECHO = all")
		case "queries":
			m.echoMode = echoQueries
			m.addOutput("  ECHO = queries")
		case "off":
			m.echoMode = echoOff
			m.addOutput("  ECHO = off")
		default:
			m.addOutput(fmt.Sprintf("  Invalid ECHO value %q. Use: all, queries, off", value))
		}
		return
	}

	// Regular user variable
	if m.sessionVars == nil {
		m.sessionVars = make(map[string]string)
	}
	m.sessionVars[name] = value
	m.addOutput(fmt.Sprintf("  Set %s = %s", name, value))
}

// handleGet shows a session variable value.
func (m *Model) handleGet(parts []string) {
	if len(parts) == 0 {
		m.handleSet(nil) // show all
		return
	}
	name := parts[0]
	if val, ok := m.sessionVars[name]; ok {
		m.addOutput(val)
	} else {
		m.addOutput(fmt.Sprintf("  Variable %q not set.", name))
	}
}

// handlePrompt prompts the user for input (stores into a session variable).
// In TUI mode, this creates a prompt message. The actual input is handled
// by prompting the user to type a value after the command.
func (m *Model) handlePrompt(parts []string) {
	if len(parts) == 0 {
		m.addOutput("Usage: \\prompt <varname> [prompt_text]")
		return
	}
	varName := parts[0]
	promptText := "Enter value: "
	if len(parts) >= 2 {
		promptText = strings.Join(parts[1:], " ") + ": "
	}
	// In TUI, we show the prompt and set a state to capture next input
	m.addOutput(fmt.Sprintf("%s(Type the value for %s and press Enter)", promptText, varName))
	// Set pending prompt state
	if m.sessionVars == nil {
		m.sessionVars = make(map[string]string)
	}
	// Store a special marker so next input is captured
	m.sessionVars["__prompt_var"] = varName
}

// handlePset controls output format details.
func (m *Model) handlePset(parts []string) {
	if len(parts) == 0 {
		m.addOutput(fmt.Sprintf("  border: %d", 1))
		m.addOutput(fmt.Sprintf("  expanded: %s", boolStr(m.autoVerticalOutput)))
		m.addOutput(fmt.Sprintf("  header: %s", boolStr(m.showHeader)))
		m.addOutput(fmt.Sprintf("  null: %s", "NULL"))
		m.addOutput(fmt.Sprintf("  pager: %s", "on"))
		m.addOutput(fmt.Sprintf("  title: %s", boolStr(m.resultTitle != "")))
		if m.resultTitle != "" {
			m.addOutput(fmt.Sprintf("  title_text: %s", m.resultTitle))
		}
		return
	}
	option := strings.ToLower(parts[0])
	switch option {
	case "border":
		m.addOutput("Border style: 1 (default)")
	case "expanded", "x":
		if len(parts) >= 2 {
			switch strings.ToLower(parts[1]) {
			case "on", "1", "true", "auto":
				m.autoVerticalOutput = true
				m.addOutput("Expanded display is on.")
			case "off", "0", "false":
				m.autoVerticalOutput = false
				m.addOutput("Expanded display is off.")
			default:
				m.addOutput("Usage: \\pset expanded [on|off|auto]")
			}
		} else {
			m.autoVerticalOutput = !m.autoVerticalOutput
			m.addOutput(fmt.Sprintf("Expanded display is %s.", boolStr(m.autoVerticalOutput)))
		}
	case "null":
		if len(parts) >= 2 {
			m.addOutput(fmt.Sprintf("Null display set to %q.", parts[1]))
		} else {
			m.addOutput("Null display: \"NULL\"")
		}
	case "pager":
		m.addOutput("Pager: on (uses terminal scrollback)")
	case "header":
		if len(parts) >= 2 {
			switch strings.ToLower(parts[1]) {
			case "on", "true", "1":
				m.showHeader = true
				m.addOutput("Header display turned on.")
			case "off", "false", "0":
				m.showHeader = false
				m.addOutput("Header display turned off.")
			default:
				m.addOutput("Usage: \\pset header on|off")
			}
		} else {
			if m.showHeader {
				m.addOutput("Header display is on.")
			} else {
				m.addOutput("Header display is off.")
			}
		}
	case "title":
		if len(parts) >= 2 {
			m.resultTitle = strings.Join(parts[1:], " ")
			m.addOutput(fmt.Sprintf("Title set to: %s", m.resultTitle))
		} else if m.resultTitle != "" {
			m.addOutput(fmt.Sprintf("Title: %s", m.resultTitle))
		} else {
			m.addOutput("No title set.")
		}
	case "format":
		if len(parts) >= 2 {
			f, err := output.ParseFormat(parts[1])
			if err != nil {
				m.addOutput(fmt.Sprintf("ERROR: %s", err))
			} else {
				m.deps.Formatter.SetFormat(f)
				m.addOutput(fmt.Sprintf("Output format set to %s", f))
			}
		} else {
			m.addOutput(fmt.Sprintf("Current format: %s", m.deps.Formatter.CurrentFormat()))
		}
	default:
		m.addOutput(fmt.Sprintf("Unknown pset option: %s", option))
		m.addOutput("Available options: border, expanded, null, pager, title, format")
	}
}

func boolStr(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// echoModeT controls the ECHO special variable behavior.
type echoModeT int

const (
	echoOff     echoModeT = iota // ECHO off (default)
	echoAll                      // ECHO all — echo commands and queries
	echoQueries                  // ECHO queries — echo only query text
)

// parseBoolValue parses a string as a boolean, returning the default value
// if the string is not a recognized boolean keyword.
func parseBoolValue(s string, defaultVal bool) bool {
	switch strings.ToLower(s) {
	case "on", "true", "1", "yes":
		return true
	case "off", "false", "0", "no":
		return false
	default:
		return defaultVal
	}
}

// handleG executes the last query (like \g in psql), optionally saving to file.
func (m *Model) handleG(parts []string) {
	sql := m.lastQuery
	if sql == "" {
		m.addOutput("No previous query to execute. Run a query first.")
		return
	}
	if len(parts) >= 1 {
		// Execute and save to file
		filePath := parts[0]
		if strings.HasPrefix(filePath, "~") {
			home, err := os.UserHomeDir()
			if err == nil {
				filePath = home + filePath[1:]
			}
		}
		m.addOutput(fmt.Sprintf("Executing query and saving to %s ...", filePath))
		// Execute and export
		result, err := m.deps.Executor.Execute(context.Background(), sql+";")
		if err != nil {
			m.addOutput(fmt.Sprintf("ERROR: %s", err))
			return
		}
		if result.Error != nil {
			m.addOutput(fmt.Sprintf("ERROR: %s", result.Error))
			return
		}

		// Determine export format
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
				format = output.ExportCSV // default
			} else {
				format = f
			}
		}

		rowCount, err := output.ExportResult(result, filePath, format)
		if err != nil {
			m.addOutput(fmt.Sprintf("Export failed: %s", err))
			return
		}
		m.addOutput(fmt.Sprintf("Exported %d rows to %s (%s format)", rowCount, filePath, format))
		return
	}
	// Just re-execute the last query
	m.ed.Clear()
	m.executeInput(sql, output.FormatTable)
}

// handleEncoding shows or sets the client character encoding.
func (m *Model) handleEncoding(parts []string) {
	if m.deps.Pool == nil {
		m.addOutput("No connection available.")
		return
	}
	if len(parts) == 0 {
		encoding, err := m.deps.Pool.GetEncoding()
		if err != nil {
			m.addOutput(fmt.Sprintf("ERROR: %s", err))
		} else {
			m.addOutput(fmt.Sprintf("Client encoding: %s", encoding))
		}
		return
	}
	encoding := parts[0]
	if err := m.deps.Pool.SetEncoding(encoding); err != nil {
		m.addOutput(fmt.Sprintf("ERROR: %s", err))
	} else {
		m.addOutput(fmt.Sprintf("Client encoding set to %s", encoding))
	}
}

// handleShowFunction shows a function definition.
func (m *Model) handleShowFunction(functionName string) {
	if m.deps.Pool == nil {
		m.addOutput("No connection available.")
		return
	}
	db := m.deps.Pool.CurrentSchema()
	def, err := m.deps.Pool.ShowFunction(db, functionName)
	if err != nil {
		m.addOutput(fmt.Sprintf("ERROR: %s", err))
		return
	}
	// Highlight the function definition (preserve DB formatting, don't reformat)
	if m.deps.Highlighter != nil {
		m.addOutput(m.deps.Highlighter.Highlight(def))
	} else {
		m.addOutput(def)
	}
}

// handlePrivileges shows table privileges.
func (m *Model) handlePrivileges(tableName string) {
	if m.deps.Pool == nil {
		m.addOutput("No connection available.")
		return
	}
	db := m.deps.Pool.CurrentSchema()
	privileges, err := m.deps.Pool.TablePrivileges(db, tableName)
	if err != nil {
		m.addOutput(fmt.Sprintf("ERROR: %s", err))
		return
	}
	if len(privileges) == 0 {
		m.addOutput(fmt.Sprintf("No privileges found for table %q (or insufficient permissions).", tableName))
		return
	}
	m.addOutput(fmt.Sprintf("  Privileges for %s:", tableName))
	for _, p := range privileges {
		m.addOutput(fmt.Sprintf("   %s", p))
	}
}

// handleExplainCmd executes an EXPLAIN query.
func (m Model) handleExplainCmd(parts []string) (tea.Model, tea.Cmd) {
	if len(parts) == 0 {
		m.addOutput("Usage: \\explain [analyze] <sql>")
		m.addOutput("  Runs EXPLAIN on the given SQL statement.")
		m.addOutput("  Add 'analyze' to actually execute the query and show timing.")
		return m, nil
	}

	analyze := false
	sqlStart := 0
	if strings.EqualFold(parts[0], "analyze") && len(parts) >= 2 {
		analyze = true
		sqlStart = 1
	}

	sql := strings.Join(parts[sqlStart:], " ")
	if sql == "" {
		m.addOutput("Usage: \\explain [analyze] <sql>")
		return m, nil
	}

	explainSQL := "EXPLAIN"
	if analyze {
		explainSQL += " ANALYZE"
	}
	explainSQL += " " + sql

	m.ed.Clear()
	return m.executeInput(explainSQL+";", output.FormatTable)
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
	if m.resultTitle != "" {
		sb.WriteString(fmt.Sprintf("Title: %s\n", m.resultTitle))
	}
	if m.verboseMode {
		sb.WriteString("Verbose: ON\n")
	}
	if !m.showWarnings {
		sb.WriteString("Warnings: OFF\n")
	}
	if len(m.sessionVars) > 0 {
		sb.WriteString(fmt.Sprintf("Session vars: %d\n", len(m.sessionVars)))
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
	if m.deps.SSHTunnel != nil {
		sb.WriteString(fmt.Sprintf("SSH tunnel: via %s\n", m.deps.SSHTunnel.LocalAddr()))
	}
	return sb.String()
}

// handleDesc handles the \desc command with sub-modes.
func (m *Model) handleDesc(parts []string) {
	if m.deps.Pool == nil {
		m.addOutput("No connection available.")
		return
	}

	tableName := strings.TrimRight(parts[0], ";")
	mode := "columns"
	if len(parts) >= 2 {
		mode = strings.ToLower(parts[1])
	}

	db := m.deps.Pool.CurrentSchema()

	switch mode {
	case "create":
		createSQL, err := m.deps.Pool.ShowCreateTable(db, tableName)
		if err != nil {
			m.addOutput(fmt.Sprintf("ERROR: %s", err))
			return
		}
		// Highlight the CREATE TABLE statement (preserve DB formatting, don't reformat)
		if m.deps.Highlighter != nil {
			m.addOutput(m.deps.Highlighter.Highlight(createSQL))
		} else {
			m.addOutput(createSQL)
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
		_ = m.displayQueryResult(result, nil, output.FormatTable)

	case "full":
		// Full column info using adapter's DescribeFullSQL
		result, err := m.deps.Executor.Execute(context.Background(),
			m.deps.Pool.DescribeFullSQL(db, tableName)+";")
		if err != nil {
			m.addOutput(fmt.Sprintf("ERROR: %s", err))
			return
		}
		_ = m.displayQueryResult(result, nil, output.FormatTable)

	default: // "columns"
		result, err := m.deps.Executor.Execute(context.Background(),
			m.deps.Pool.DescribeTableSQL(db, tableName)+";")
		if err != nil {
			m.addOutput(fmt.Sprintf("ERROR: %s", err))
			return
		}
		_ = m.displayQueryResult(result, nil, output.FormatTable)
	}
}

// handleCopy copies data to the system clipboard.
func (m *Model) handleCopy(what string, inputBuffer string) {
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
		sql := strings.TrimSpace(inputBuffer)
		if sql == "" {
			// If input buffer is empty, fall back to last executed query
			if m.lastQuery == "" {
				m.addOutput("No SQL to copy (input buffer is empty and no query executed).")
				return
			}
			sql = m.lastQuery + ";"
		}
		content = sql

	default:
		m.addOutput(fmt.Sprintf("Unknown copy target: %s (use: result, query, sql)", what))
		return
	}

	if err := copyToClipboard(content); err != nil {
		// No clipboard available — output to terminal instead
		m.addOutput(content)
		switch strings.ToLower(what) {
		case "result":
			lines := strings.Count(content, "\n")
			if lines > 0 {
				lines-- // trailing newline
			}
			m.addOutput(fmt.Sprintf("(%d rows — no clipboard, output above)", lines))
		case "query":
			m.addOutput("(No clipboard, query output above)")
		case "sql":
			m.addOutput("(No clipboard, SQL output above)")
		}
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

// interactiveCommands lists commands that require a real TTY (terminal).
var interactiveCommands = map[string]bool{
	"vi": true, "vim": true, "nano": true, "emacs": true,
	"less": true, "more": true,
	"top": true, "htop": true, "btop": true,
	"tmux": true, "screen": true,
	"mysql": true, "psql": true, "sqlite3": true,
	"ssh": true, "telnet": true,
	"man": true,
}

// handleSys executes a system command and displays its output.
// cmdParts is either a single raw command string (from \sys) or split args.
func (m *Model) handleSys(cmdParts []string) {
	// Join parts back into a single command string
	cmdStr := strings.Join(cmdParts, " ")

	// If the command contains shell operators, run via sh -c
	if m.needsShell(cmdStr) {
		m.execShellCommand(cmdStr)
		return
	}

	// Otherwise, split into command and args for direct execution
	fields := strings.Fields(cmdStr)

	// Interactive commands need a real TTY — suspend TUI and connect directly
	if interactiveCommands[fields[0]] {
		m.execInteractiveCommand(fields)
		return
	}

	cmd := exec.Command(fields[0], fields[1:]...)
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

// needsShell checks if the command string contains shell operators that require sh -c.
func (m *Model) needsShell(cmdStr string) bool {
	return strings.ContainsAny(cmdStr, ";|&<>") || strings.Contains(cmdStr, "$(")
}

// execShellCommand runs a command via sh -c for shell operator support.
func (m *Model) execShellCommand(cmdStr string) {
	cmd := exec.Command("sh", "-c", cmdStr)
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
		for _, line := range strings.Split(out, "\n") {
			m.addOutput(line)
		}
	}
}

// execInteractiveCommand runs a command that needs a real TTY.
// It suspends the TUI, connects stdin/stdout/stderr to the real terminal,
// and resumes the TUI after the command exits.
func (m *Model) execInteractiveCommand(fields []string) {
	tea.ExitAltScreen()
	defer tea.EnterAltScreen()

	cmd := exec.Command(fields[0], fields[1:]...)
	if m.workDir != "" {
		cmd.Dir = m.workDir
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		m.addOutput(fmt.Sprintf("ERROR: %s", err))
	}
}

// truncateStr truncates a string to maxLen runes, appending "..." if truncated.
func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen > 3 {
		return string(runes[:maxLen-3]) + "..."
	}
	return string(runes[:maxLen])
}

// splitArgs splits a command string into arguments, respecting single and double quotes.
// This allows arguments containing spaces, e.g.: \pipe grep '2025-03-27 09:31:21'
func splitArgs(input string) []string {
	var args []string
	var current strings.Builder
	inSingle := false
	inDouble := false

	for _, r := range input {
		switch {
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case r == ' ' && !inSingle && !inDouble:
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
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
  \alias [name sql] Show/set command aliases
  \cd [dir]         Change/show working directory (for \source, \sys)
  \clear, \c        Clear screen output
  \connect <dsn>    Connect to a database (user@host:port/db or just db)
  \conninfo         Show detailed connection info
  \copy <what>      Copy to clipboard (result|query|sql)
  \desc <t> [mode]  Describe table (no arg = list tables; columns|full|indexes|create)
  \di [table], \indexes  List indexes (optional table filter)
  \dn, \schemas      List schemas (MySQL: databases, PostgreSQL: schemas)
  \dt [pattern], \tables  List tables (optional pattern: user* or user%)
  \du, \users        List database users
  \dv [pattern], \views   List views (optional pattern)
  \echo <text>      Echo text to output (:var substitution supported)
  \edit, \e         Open editor ($EDITOR or vi) to edit/execute SQL
  \encoding [name]  Show/set client character encoding
  \explain [analyze] <sql>  Run EXPLAIN on SQL (add analyze to execute)
  \export <f> [fmt] Export last result to file (csv/json/markdown)
  \fav, \favorites   List favorite queries (saved in ~/.mysh_favorites.yaml, max 1000, LRU eviction)
  \fav <name>        Show favorite SQL and put in input buffer (Enter to execute)
  \fav + <name> [desc]  Save last query as favorite
  \fav - <name>      Delete a favorite
  \fav run <name>    Execute a saved favorite directly
  \fav show <name>   Show favorite SQL (same as \fav <name>)
  \format [type]    Set/show output format (table|vertical|json|markdown|sql)
  \g [file]         Execute last query, optionally save to file
  \get <name>       Show session variable value
  \gx               Execute last query with vertical output
  \help, \h, \?    Show this help message
  \history [pat]    Search/show command history
  \l, \list, \databases  List all databases
  \mouse            Toggle mouse mode (scroll wheel vs text selection)
  \pipe, \| <cmd>   Pipe last query result to a system command
  \privileges <t>   Show table privileges
  \prompt <var> [text]  Prompt for input (stores into variable)
  \pset [opt [val]] Control output details:
                       expanded [on|off|auto]  Toggle vertical output
                       format [table|vertical|json|markdown]  Set output format
                       header [on|off]         Show/hide column names
                       null [string]           Set NULL display string
                       pager                   Show pager status
                       title [text|off]        Set/clear result title
  \quit, \q         Exit mysh (also: quit, exit)
  \reconnect        Reconnect to the current server
  \refresh, \r      Refresh metadata cache
  \rollback         Rollback current transaction
  \safe-updates [on|off]  Toggle safe-updates mode (block UPDATE/DELETE without WHERE/LIMIT)
  \session          List saved sessions
  \session <name>   Switch to saved session
  \session save <n> Save current connection as session
  \session del <n>  Delete a saved session
  \sf <func>        Show function definition
  \set [name value] Show/set session variables
                       Special: AUTOCOMMIT, ON_ERROR_STOP, ECHO
                       Use :varname in SQL to substitute variable value
                       Use :'varname' for quoted substitution
  \slow [seconds]   Set/show slow query warning threshold (0 = disabled)
  \source <file>    Execute SQL from file
  \status, \s       Show connection status
  \sys, \! <cmd>    Execute a system command
  \T [title|off]    Set/clear result title (shown in markdown output)
  \timing           Toggle query execution time display
  \unalias <name>   Remove temp alias
  \unset <name>     Remove session variable
  \use <db>         Switch to database <db>
  \verbose          Toggle verbose mode (show full error details)
  \warn [on|off]    Toggle warning display
  \watch [sec] [SQL] Watch query at intervals (default 5s, Ctrl+C stop)
  \x, \expanded     Toggle expanded (vertical) output mode

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

// SetShowHeader controls whether column headers are displayed in results.
func (m *Model) SetShowHeader(show bool) {
	m.showHeader = show
}
