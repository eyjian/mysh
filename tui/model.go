package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"mysh/completer"
	"mysh/config"
	"mysh/connection"
	"mysh/editor"
	"mysh/executor"
	"mysh/highlight"
	"mysh/history"
	"mysh/metadata"
	"mysh/output"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Dependencies holds all the shared service instances for the TUI.
type Dependencies struct {
	Config     *config.Config
	Pool       *connection.Pool
	Executor   *executor.Executor
	Meta       *metadata.Cache
	History    *history.History
	Formatter  *output.Formatter
	Highlighter *highlight.Highlighter
	Completer  *completer.Completer
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
		mouseEnabled: false,
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
	return blinkCmd()
}

// blinkMsg is sent by the cursor blink timer.
type blinkMsg time.Time

// blinkCmd returns a command that waits for the cursor blink interval.
func blinkCmd() tea.Cmd {
	return tea.Tick(530*time.Millisecond, func(t time.Time) tea.Msg {
		return blinkMsg(t)
	})
}

// tickMsg is sent after a query execution completes.
type tickMsg time.Time

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

	case tickMsg:
		m.executing = false
		return m, nil
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

	switch msg.Type {
	case tea.KeyCtrlC:
		if m.executing {
			m.deps.Executor.Cancel()
			m.executing = false
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
	case 21: // Ctrl+U — kill to beginning of line
		pos := m.ed.CursorPos()
		text := []rune(m.ed.Text())
		if pos > 0 {
			m.ed = editor.New()
			m.ed.SetText(string(text[pos:]))
		}
		m.showComp = false
	case 23: // Ctrl+W — delete word backward
		word := m.ed.WordBeforeCursor()
		if word != "" {
			m.ed.Backspace(len([]rune(word)))
		}
		m.showComp = false
	default:
		// Ignore other control characters
	}
	return m, nil
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

	// Check for backslash commands
	trimmed := strings.TrimSpace(input)
	if strings.HasPrefix(trimmed, "\\") {
		return m.handleBackslashCommand(trimmed)
	}

	// Check for built-in SQL-style commands (quit/exit/clear/source, with or without trailing ;)
	cmd := strings.TrimRight(trimmed, ";")
	lowerCmd := strings.ToLower(cmd)
	if lowerCmd == "quit" || lowerCmd == "exit" {
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

	// Check for \G (vertical output) suffix
	useVertical := strings.HasSuffix(trimmed, "\\G")
	if useVertical {
		trimmed = strings.TrimSuffix(trimmed, "\\G")
		trimmed = strings.TrimSpace(trimmed)
	}

	// Multi-line: if no semicolon at end and no \G, continue input
	if !strings.HasSuffix(trimmed, ";") && !useVertical {
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
	return m.executeInput(fullInput, useVertical)
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
		m.addOutput(helpText())

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
			m.addOutput("Usage: \\connect <dsn>")
		} else {
			m.addOutput("\\connect is not yet supported in this version.")
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

		// Handle \G (vertical output) suffix — it's a client-side format
		// indicator, not valid SQL syntax, so strip it before execution.
		useVertical := false
		if strings.HasSuffix(stmt, "\\G") {
			useVertical = true
			stmt = strings.TrimSuffix(stmt, "\\G")
			stmt = strings.TrimSpace(stmt)
		}
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
		if useVertical {
			outFmt = output.FormatVertical
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

// executeInput runs the SQL statement and displays the result.
// When useVertical is true, output is displayed in vertical format (like \G in mysql CLI).
func (m Model) executeInput(input string, useVertical bool) (tea.Model, tea.Cmd) {
	// Remove trailing semicolons
	input = strings.TrimRight(input, ";")
	input = strings.TrimSpace(input)

	if input == "" {
		m.ed.Clear()
		return m, nil
	}

	// Save to history (append \G if vertical output was requested)
	historyEntry := input
	if useVertical {
		historyEntry = input + " \\G"
	}
	m.deps.History.Append(historyEntry)

	// Echo the input line to output area (like mysql CLI)
	displayEntry := input + ";"
	if useVertical {
		displayEntry = input + " \\G"
	}
	m.addOutput(m.prompt + displayEntry)

	// Normalize table name casing in SQL before execution
	execSQL := m.normalizeTableNames(input+";")

	m.executing = true

	// Execute
	ctx := context.Background()
	result, err := m.deps.Executor.Execute(ctx, execSQL)
	if err != nil {
		m.addOutput(fmt.Sprintf("%s", err))
		m.executing = false
		m.ed.Clear()
		return m, nil
	}

	// Choose format: use vertical if \G was specified
	outFmt := m.deps.Formatter.CurrentFormat()
	if useVertical {
		outFmt = output.FormatVertical
	}

	// Format and display result
	var buf strings.Builder
	formatter := output.NewFormatter(outFmt, &buf)
	if writeErr := formatter.WriteResult(result); writeErr != nil {
		m.addOutput(fmt.Sprintf("Output error: %s", writeErr))
	}
	if buf.Len() > 0 {
		m.addOutput(strings.TrimRight(buf.String(), "\n"))
	}

	m.executing = false
	m.ed.Clear()

	return m, nil
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

	// Prompt + highlighted input with cursor
	currentPrompt := m.prompt
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
	if m.deps.Meta != nil {
		dbs := m.deps.Meta.Databases()
		sb.WriteString(fmt.Sprintf("Cached databases: %d\n", len(dbs)))
	}
	return sb.String()
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
  \format [type]    Set/show output format (table|vertical|json)
  \history [pat]    Search/show command history
  \connect <dsn>    Connect to a database
  \source <file>    Execute SQL from file
  \mouse            Toggle mouse mode (scroll wheel vs text selection)

Keyboard shortcuts:
  Tab               Auto-complete
  Up/Down           Navigate history / completion list
  Ctrl+C            Cancel current query or clear input
  Ctrl+D            Exit (when input is empty)
  Enter             Execute SQL (ends with ;) or start multi-line
`
}
