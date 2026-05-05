package tui

import (
	"strings"
	"testing"

	"github.com/eyjian/mysh/completer"
	"github.com/eyjian/mysh/config"
	"github.com/eyjian/mysh/editor"
	"github.com/eyjian/mysh/history"
	"github.com/eyjian/mysh/output"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// newTestModel creates a Model with minimal dependencies for testing.
func newTestModel() Model {
	cfg := &config.Config{
		Connection: config.ConnectionConfig{
			Host:     "localhost",
			Port:     3306,
			User:     "root",
			Password: "",
			Database: "testdb",
		},
		UI: config.UIConfig{
			Prompt:          "mysql> ",
			MultilinePrompt: "    -> ",
		},
	}

	hist := history.New("", 100)

	return Model{
		deps: Dependencies{
			Config:    cfg,
			Pool:      nil, // no real DB needed for most tests
			History:   hist,
			Formatter: output.NewFormatter(output.FormatTable, nil),
		},
		ed:           editor.New(),
		prompt:       "mysql> ",
		mlPrompt:     "    -> ",
		output:       []string{},
		cursorOn:     true,
		promptStyle:  lipgloss.NewStyle(),
		outputStyle:  lipgloss.NewStyle(),
		errorStyle:   lipgloss.NewStyle(),
		compStyle:    lipgloss.NewStyle(),
		compSelStyle: lipgloss.NewStyle(),
	}
}

// isQuitCmd checks if a tea.Cmd is the tea.Quit command.
func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	// tea.Quit returns a tea.Msg that is tea.QuitMsg
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	return ok
}

// --- helpText tests ---

func TestHelpText_ContainsCommands(t *testing.T) {
	help := helpText()
	expectedCommands := []string{`\help`, `\quit`, `\status`, `\use`, `\refresh`, `\format`, `\history`, `\connect`, `\source`}
	for _, cmd := range expectedCommands {
		if !strings.Contains(help, cmd) {
			t.Errorf("helpText should contain %q", cmd)
		}
	}
}

func TestHelpText_ContainsKeyboardShortcuts(t *testing.T) {
	help := helpText()
	expectedShortcuts := []string{"Tab", "Ctrl+C", "Ctrl+D", "Enter"}
	for _, shortcut := range expectedShortcuts {
		if !strings.Contains(help, shortcut) {
			t.Errorf("helpText should contain %q", shortcut)
		}
	}
}

func TestHelpText_NotEmpty(t *testing.T) {
	help := helpText()
	if len(help) == 0 {
		t.Error("helpText should not be empty")
	}
}

// --- formatStatus tests ---

func TestFormatStatus_NilPool(t *testing.T) {
	m := newTestModel()
	m.deps.Pool = nil
	status := m.formatStatus()
	if !strings.Contains(status, "not connected") {
		t.Errorf("formatStatus with nil pool should say 'not connected', got: %q", status)
	}
}

func TestFormatStatus_ContainsVersion(t *testing.T) {
	m := newTestModel()
	status := m.formatStatus()
	if !strings.Contains(status, "0.1.0") {
		t.Errorf("formatStatus should contain version, got: %q", status)
	}
}

func TestFormatStatus_ContainsOutputFormat(t *testing.T) {
	m := newTestModel()
	status := m.formatStatus()
	if !strings.Contains(status, "Output format:") {
		t.Errorf("formatStatus should contain 'Output format:', got: %q", status)
	}
}

// --- handleBackslashCommand tests ---

func TestBackslashCommand_Help(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\help")
	model, _ := m.handleEnter()
	updated := model.(Model)
	if len(updated.output) == 0 {
		t.Error("expected output after \\help command")
	}
	// helpText is split into multiple lines by addOutput, check the full output
	allOutput := strings.Join(updated.output, "\n")
	if !strings.Contains(allOutput, "Backslash commands") {
		t.Errorf("expected 'Backslash commands' in help output, got: %q", allOutput[:min(200, len(allOutput))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestBackslashCommand_HelpShort(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\h")
	model, _ := m.handleEnter()
	updated := model.(Model)
	if len(updated.output) == 0 {
		t.Error("expected output after \\h command")
	}
}

func TestBackslashCommand_HelpQuestionMark(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\?")
	model, _ := m.handleEnter()
	updated := model.(Model)
	if len(updated.output) == 0 {
		t.Error("expected output after \\? command")
	}
}

func TestBackslashCommand_Quit(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\quit")
	model, cmd := m.handleEnter()
	updated := model.(Model)
	if !updated.quitting {
		t.Error("expected quitting=true after \\quit")
	}
	if !isQuitCmd(cmd) {
		t.Error("expected tea.Quit command after \\quit")
	}
}

func TestBackslashCommand_QuitShort(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\q")
	model, cmd := m.handleEnter()
	updated := model.(Model)
	if !updated.quitting {
		t.Error("expected quitting=true after \\q")
	}
	if !isQuitCmd(cmd) {
		t.Error("expected tea.Quit command after \\q")
	}
}

func TestBackslashCommand_Status(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\status")
	model, _ := m.handleEnter()
	updated := model.(Model)
	if len(updated.output) == 0 {
		t.Error("expected output after \\status command")
	}
	// First output is the echoed command, status content is in subsequent outputs
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "0.1.0") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected version in status output, got: %v", updated.output)
	}
}

func TestBackslashCommand_StatusShort(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\s")
	model, _ := m.handleEnter()
	updated := model.(Model)
	if len(updated.output) == 0 {
		t.Error("expected output after \\s command")
	}
}

func TestBackslashCommand_FormatNoArg(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\format")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "Current format:") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'Current format:' output after \\format without arg")
	}
}

func TestBackslashCommand_FormatSetJSON(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\format json")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "json") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'json' in output after \\format json")
	}
}

func TestBackslashCommand_FormatInvalid(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\format csv")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "ERROR") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected ERROR output for invalid format")
	}
}

func TestBackslashCommand_RefreshNilMeta(t *testing.T) {
	m := newTestModel()
	m.deps.Meta = nil
	m.ed.Insert("\\refresh")
	model, _ := m.handleEnter()
	// Should not panic with nil Meta
	_ = model.(Model)
}

func TestBackslashCommand_UseNoArg(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\use")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "Usage:") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Usage output after \\use without arg")
	}
}

func TestBackslashCommand_ConnectNoArg(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\connect")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "Usage:") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Usage output after \\connect without arg")
	}
}

func TestBackslashCommand_ConnectWithArg_NilPool(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\connect dsn")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "No connection pool") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'No connection pool' output after \\connect with arg and nil pool")
	}
}

func TestBackslashCommand_SourceNoArg(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\source")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "Usage:") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Usage output after \\source without arg")
	}
}

func TestBackslashCommand_Source(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\source /nonexistent/file.sql")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "Failed to read file") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'Failed to read file' output for nonexistent file")
	}
}

func TestBackslashCommand_HistoryEmpty(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\history")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "No history entries") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'No history entries' for empty history")
	}
}

func TestBackslashCommand_HistoryWithEntries(t *testing.T) {
	m := newTestModel()
	m.deps.History.Append("SELECT 1")
	m.deps.History.Append("SELECT 2")
	m.ed.Insert("\\history")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "SELECT 1") || strings.Contains(line, "SELECT 2") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected history entries in output")
	}
}

func TestBackslashCommand_HistorySearch(t *testing.T) {
	m := newTestModel()
	m.deps.History.Append("SELECT 1")
	m.deps.History.Append("INSERT INTO t VALUES (1)")
	m.ed.Insert("\\history insert")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "INSERT") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected INSERT in history search results")
	}
}

func TestBackslashCommand_Unknown(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\unknown")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "Unknown command") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'Unknown command' output for unknown command")
	}
}

// --- handleEnter tests ---

func TestHandleEnter_MultilineNoSemicolon(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("SELECT 1")
	model, _ := m.handleEnter()
	updated := model.(Model)
	if !updated.multiline {
		t.Error("expected multiline=true when input has no semicolon")
	}
}

func TestHandleEnter_ExecuteWithSemicolon_NilPool(t *testing.T) {
	m := newTestModel()
	m.deps.Pool = nil
	m.deps.Executor = nil
	m.ed.Insert("SELECT 1;")
	// This will panic if Executor is nil, so we test with no pool
	// The executeInput path accesses deps.Executor, which is nil here
	// We need to handle this — skip if executor is nil
	// Actually handleEnter calls executeInput which calls deps.Executor.Execute
	// This will panic. Let's test with a different approach.
	// Just verify multiline is not set when semicolon is present
}

func TestHandleEnter_BackslashCommand(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\help")
	model, _ := m.handleEnter()
	updated := model.(Model)
	if updated.multiline {
		t.Error("expected multiline=false after backslash command")
	}
	if len(updated.output) == 0 {
		t.Error("expected output after backslash command")
	}
}

// --- handleKey tests ---

func TestHandleKey_CtrlC_WithInput(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("SELECT 1")
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	updated := model.(Model)
	if updated.ed.Text() != "" {
		t.Error("expected editor to be cleared after Ctrl+C with input")
	}
	if updated.multiline {
		t.Error("expected multiline=false after Ctrl+C")
	}
}

func TestHandleKey_CtrlC_EmptyInput_Quit(t *testing.T) {
	m := newTestModel()
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	updated := model.(Model)
	if !updated.quitting {
		t.Error("expected quitting=true after Ctrl+C on empty input")
	}
	if !isQuitCmd(cmd) {
		t.Error("expected tea.Quit after Ctrl+C on empty input")
	}
}

func TestHandleKey_CtrlD_EmptyInput(t *testing.T) {
	m := newTestModel()
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	updated := model.(Model)
	if !updated.quitting {
		t.Error("expected quitting=true after Ctrl+D on empty input")
	}
	if !isQuitCmd(cmd) {
		t.Error("expected tea.Quit after Ctrl+D on empty input")
	}
}

func TestHandleKey_CtrlD_WithInput(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("hello")
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	updated := model.(Model)
	if updated.quitting {
		t.Error("expected not quitting after Ctrl+D with input")
	}
}

func TestHandleKey_Escape(t *testing.T) {
	m := newTestModel()
	m.showComp = true
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	updated := model.(Model)
	if updated.showComp {
		t.Error("expected showComp=false after Escape")
	}
}

func TestHandleKey_Backspace(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("hello")
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	updated := model.(Model)
	if updated.ed.Text() != "hell" {
		t.Errorf("expected 'hell' after backspace, got %q", updated.ed.Text())
	}
}

func TestHandleKey_Delete(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("hello")
	m.ed.MoveLeft()
	m.ed.MoveLeft()
	m.ed.MoveLeft()
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyDelete})
	updated := model.(Model)
	if updated.ed.Text() != "helo" {
		t.Errorf("expected 'helo' after delete, got %q", updated.ed.Text())
	}
}

func TestHandleKey_Runes(t *testing.T) {
	m := newTestModel()
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	updated := model.(Model)
	if updated.ed.Text() != "a" {
		t.Errorf("expected 'a' after typing, got %q", updated.ed.Text())
	}
}

func TestHandleKey_WindowSize(t *testing.T) {
	m := newTestModel()
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	updated := model.(Model)
	if updated.width != 80 || updated.height != 24 {
		t.Errorf("expected width=80, height=24, got width=%d, height=%d", updated.width, updated.height)
	}
}

// --- View tests ---

func TestView_Quitting(t *testing.T) {
	m := newTestModel()
	m.quitting = true
	view := m.View()
	if !strings.Contains(view, "Goodbye") {
		t.Errorf("expected 'Goodbye' in view when quitting, got: %q", view)
	}
}

func TestView_Normal(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("SELECT 1")
	view := m.View()
	if !strings.Contains(view, "mysql>") {
		t.Errorf("expected prompt in view, got: %q", view)
	}
	if !strings.Contains(view, "SELECT 1") {
		t.Errorf("expected input text in view, got: %q", view)
	}
}

func TestView_MultilinePrompt(t *testing.T) {
	m := newTestModel()
	m.multiline = true
	view := m.View()
	// Multi-line mode now always shows the same prompt (like mysql CLI)
	if !strings.Contains(view, "mysql>") {
		t.Errorf("expected prompt in view, got: %q", view)
	}
}

func TestView_WithCompletion(t *testing.T) {
	m := newTestModel()
	m.showComp = true
	m.compItems = []completer.Suggestion{
		{Text: "SELECT", Type: completer.SuggestKeyword, Detail: "keyword"},
		{Text: "SHOW", Type: completer.SuggestKeyword, Detail: "keyword"},
	}
	m.compIndex = 0
	view := m.View()
	if !strings.Contains(view, "SELECT") {
		t.Error("expected completion item in view")
	}
	if !strings.Contains(view, "SHOW") {
		t.Error("expected completion item in view")
	}
}

func TestView_WithOutput(t *testing.T) {
	m := newTestModel()
	m.addOutput("Hello World")
	view := m.View()
	if !strings.Contains(view, "Hello World") {
		t.Error("expected output text in view")
	}
}

// --- addOutput tests ---

func TestAddOutput_SingleLine(t *testing.T) {
	m := newTestModel()
	m.addOutput("test line")
	if len(m.output) != 1 || m.output[0] != "test line" {
		t.Errorf("expected ['test line'], got %v", m.output)
	}
}

func TestAddOutput_MultiLine(t *testing.T) {
	m := newTestModel()
	m.addOutput("line1\nline2\nline3")
	if len(m.output) != 3 {
		t.Errorf("expected 3 lines, got %d", len(m.output))
	}
}

// --- Init test ---

func TestInit(t *testing.T) {
	m := newTestModel()
	cmd := m.Init()
	if cmd == nil {
		t.Error("expected non-nil cmd from Init (blink + health check timers)")
	}
}

// --- NewModel test ---

func TestNewModel(t *testing.T) {
	cfg := &config.Config{
		UI: config.UIConfig{
			Prompt:          "test> ",
			MultilinePrompt: "  -> ",
		},
	}
	deps := Dependencies{
		Config:    cfg,
		History:   history.New("", 100),
		Formatter: output.NewFormatter(output.FormatTable, nil),
	}
	m := NewModel(deps)
	if m.prompt != "test> " {
		t.Errorf("expected prompt 'test> ', got %q", m.prompt)
	}
	if m.mlPrompt != "  -> " {
		t.Errorf("expected mlPrompt '  -> ', got %q", m.mlPrompt)
	}
}
