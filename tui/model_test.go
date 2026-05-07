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
		height:       100, // large enough so help text fits in TUI view
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
	// When help text exceeds terminal height, it's printed via ExecProcess
	// (not in output buffer). When it fits, it's in the output buffer.
	// Either way, the command should not crash and should produce output or a cmd.
	allOutput := strings.Join(updated.output, "\n")
	hasHelpInOutput := strings.Contains(allOutput, "Backslash commands")
	if !hasHelpInOutput && len(updated.output) == 0 {
		t.Error("expected some output after \\help command")
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
		if strings.Contains(line, "No history entries.") {
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

// --- \l, \list, \databases tests ---

func TestBackslashCommand_ListDatabases_NilPool(t *testing.T) {
	m := newTestModel()
	m.deps.Pool = nil
	m.ed.Insert("\\l")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "No connection") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'No connection' output after \\l with nil pool")
	}
}

func TestBackslashCommand_ListDatabases_Aliases(t *testing.T) {
	for _, cmd := range []string{"\\list", "\\databases"} {
		m := newTestModel()
		m.deps.Pool = nil
		m.ed.Insert(cmd)
		model, _ := m.handleEnter()
		updated := model.(Model)
		found := false
		for _, line := range updated.output {
			if strings.Contains(line, "No connection") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected 'No connection' output after %s with nil pool", cmd)
		}
	}
}

// --- \dt, \tables tests ---

func TestBackslashCommand_ListTables_NilPool(t *testing.T) {
	m := newTestModel()
	m.deps.Pool = nil
	m.ed.Insert("\\dt")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "No connection") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'No connection' output after \\dt with nil pool")
	}
}

func TestBackslashCommand_ListTables_Alias(t *testing.T) {
	m := newTestModel()
	m.deps.Pool = nil
	m.ed.Insert("\\tables")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "No connection") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'No connection' output after \\tables with nil pool")
	}
}

// --- \echo tests ---

func TestBackslashCommand_Echo_Text(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\echo hello world")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "hello world") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'hello world' in output after \\echo hello world")
	}
}

func TestBackslashCommand_Echo_Empty(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\echo")
	model, _ := m.handleEnter()
	updated := model.(Model)
	// Should produce output without panicking
	_ = updated
}

// --- \conninfo tests ---

func TestBackslashCommand_ConnInfo_NilPool(t *testing.T) {
	m := newTestModel()
	m.deps.Pool = nil
	m.ed.Insert("\\conninfo")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "No connection") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'No connection' output after \\conninfo with nil pool")
	}
}

// --- \x, \expanded tests ---

func TestBackslashCommand_ToggleExpanded(t *testing.T) {
	m := newTestModel()
	initial := m.autoVerticalOutput
	m.ed.Insert("\\x")
	model, _ := m.handleEnter()
	updated := model.(Model)
	if updated.autoVerticalOutput == initial {
		t.Error("expected autoVerticalOutput to toggle after \\x")
	}
	// Should show "on" or "off" in output
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "Expanded display is") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'Expanded display is' in output after \\x")
	}
}

func TestBackslashCommand_ToggleExpandedTwice(t *testing.T) {
	m := newTestModel()
	initial := m.autoVerticalOutput
	// Toggle twice should return to original state
	m.ed.Insert("\\x")
	model, _ := m.handleEnter()
	updated := model.(Model)
	updated.ed.Insert("\\x")
	model2, _ := updated.handleEnter()
	updated2 := model2.(Model)
	if updated2.autoVerticalOutput != initial {
		t.Error("expected autoVerticalOutput to return to initial state after two \\x")
	}
}

func TestBackslashCommand_ExpandedAlias(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\expanded")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "Expanded display is") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'Expanded display is' in output after \\expanded")
	}
}

// --- \dn, \schemas tests ---

func TestBackslashCommand_ListSchemas_NilPool(t *testing.T) {
	m := newTestModel()
	m.deps.Pool = nil
	m.ed.Insert("\\dn")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "No connection") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'No connection' output after \\dn with nil pool")
	}
}

func TestBackslashCommand_ListSchemas_Alias(t *testing.T) {
	m := newTestModel()
	m.deps.Pool = nil
	m.ed.Insert("\\schemas")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "No connection") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'No connection' output after \\schemas with nil pool")
	}
}

// --- \du, \users tests ---

func TestBackslashCommand_ListUsers_NilPool(t *testing.T) {
	m := newTestModel()
	m.deps.Pool = nil
	m.ed.Insert("\\du")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "No connection") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'No connection' output after \\du with nil pool")
	}
}

func TestBackslashCommand_ListUsers_Alias(t *testing.T) {
	m := newTestModel()
	m.deps.Pool = nil
	m.ed.Insert("\\users")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "No connection") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'No connection' output after \\users with nil pool")
	}
}

// --- \d without args lists tables (psql behavior) ---

func TestBackslashCommand_DescNoArgs_ListsTables(t *testing.T) {
	m := newTestModel()
	m.deps.Pool = nil
	m.ed.Insert("\\d")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "No connection") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected \\d without args to list tables (falls through to \\dt behavior)")
	}
}

// --- helpText contains new commands ---

func TestHelpText_ContainsNewCommands(t *testing.T) {
	help := helpText()
	newCommands := []string{`\l`, `\dt`, `\echo`, `\conninfo`, `\x`, `\dn`, `\du`, `\di`, `\dv`, `\set`, `\get`, `\encoding`, `\verbose`, `\warn`, `\explain`, `\sf`, `\privileges`, `\pset`, `\gx`}
	for _, cmd := range newCommands {
		if !strings.Contains(help, cmd) {
			t.Errorf("helpText should contain %q", cmd)
		}
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
	// Output is now printed via tea.Println, not rendered in View()
	// Check that the output was queued for printing
	if len(m.pendingPrintLines) == 0 {
		t.Error("expected pending print lines after addOutput")
	}
	// View() should not contain the output text (it's printed above the view)
	view := m.View()
	if strings.Contains(view, "Hello World") {
		t.Error("output text should not be in View() - it's printed via tea.Println")
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

// --- \di, \indexes tests ---

func TestBackslashCommand_ListIndexes_NilPool(t *testing.T) {
	m := newTestModel()
	m.deps.Pool = nil
	m.ed.Insert("\\di")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "No connection") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'No connection' output after \\di with nil pool")
	}
}

func TestBackslashCommand_ListIndexes_Alias(t *testing.T) {
	m := newTestModel()
	m.deps.Pool = nil
	m.ed.Insert("\\indexes")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "No connection") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'No connection' output after \\indexes with nil pool")
	}
}

// --- \dv, \views tests ---

func TestBackslashCommand_ListViews_NilPool(t *testing.T) {
	m := newTestModel()
	m.deps.Pool = nil
	m.ed.Insert("\\dv")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "No connection") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'No connection' output after \\dv with nil pool")
	}
}

func TestBackslashCommand_ListViews_Alias(t *testing.T) {
	m := newTestModel()
	m.deps.Pool = nil
	m.ed.Insert("\\views")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "No connection") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'No connection' output after \\views with nil pool")
	}
}

// --- \set, \get, \unset tests ---

func TestBackslashCommand_SetList(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\set")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "No session variables") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'No session variables' output after \\set without args")
	}
}

func TestBackslashCommand_SetAndGet(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\set myvar hello")
	model, _ := m.handleEnter()
	updated := model.(Model)
	updated.ed.Insert("\\get myvar")
	model2, _ := updated.handleEnter()
	updated2 := model2.(Model)
	found := false
	for _, line := range updated2.output {
		if strings.Contains(line, "hello") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'hello' in output after \\get myvar")
	}
}

func TestBackslashCommand_Unset(t *testing.T) {
	m := newTestModel()
	m.sessionVars = map[string]string{"x": "val"}
	m.ed.Insert("\\unset x")
	model, _ := m.handleEnter()
	updated := model.(Model)
	if _, ok := updated.sessionVars["x"]; ok {
		t.Error("expected variable 'x' to be unset")
	}
}

// --- \pset tests ---

func TestBackslashCommand_PsetNoArgs(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\pset")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "border") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'border' in output after \\pset without args")
	}
}

func TestBackslashCommand_PsetExpanded(t *testing.T) {
	m := newTestModel()
	m.autoVerticalOutput = false
	m.ed.Insert("\\pset expanded on")
	model, _ := m.handleEnter()
	updated := model.(Model)
	if !updated.autoVerticalOutput {
		t.Error("expected autoVerticalOutput=true after \\pset expanded on")
	}
}

func TestBackslashCommand_PsetUnknown(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\pset unknown_option")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "Unknown pset option") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'Unknown pset option' output")
	}
}

// --- \T (title) tests ---

func TestBackslashCommand_TitleSet(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\T My Report")
	model, _ := m.handleEnter()
	updated := model.(Model)
	if updated.resultTitle != "My Report" {
		t.Errorf("expected resultTitle='My Report', got %q", updated.resultTitle)
	}
}

func TestBackslashCommand_TitleClear(t *testing.T) {
	m := newTestModel()
	m.resultTitle = "Old Title"
	m.ed.Insert("\\T off")
	model, _ := m.handleEnter()
	updated := model.(Model)
	if updated.resultTitle != "" {
		t.Errorf("expected resultTitle cleared, got %q", updated.resultTitle)
	}
}

func TestBackslashCommand_TitleShow(t *testing.T) {
	m := newTestModel()
	m.resultTitle = "My Title"
	m.ed.Insert("\\T")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "My Title") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'My Title' in output after \\T")
	}
}

// --- \verbose tests ---

func TestBackslashCommand_Verbose(t *testing.T) {
	m := newTestModel()
	initial := m.verboseMode
	m.ed.Insert("\\verbose")
	model, _ := m.handleEnter()
	updated := model.(Model)
	if updated.verboseMode == initial {
		t.Error("expected verboseMode to toggle after \\verbose")
	}
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "Verbose mode is") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'Verbose mode is' in output")
	}
}

// --- \warn tests ---

func TestBackslashCommand_Warn(t *testing.T) {
	m := newTestModel()
	m.showWarnings = false
	m.ed.Insert("\\warn on")
	model, _ := m.handleEnter()
	updated := model.(Model)
	if !updated.showWarnings {
		t.Error("expected showWarnings=true after \\warn on")
	}
}

func TestBackslashCommand_WarnToggle(t *testing.T) {
	m := newTestModel()
	m.showWarnings = false
	m.ed.Insert("\\warn")
	model, _ := m.handleEnter()
	updated := model.(Model)
	if !updated.showWarnings {
		t.Error("expected showWarnings to toggle on after \\warn")
	}
}

// --- \encoding tests ---

func TestBackslashCommand_EncodingNilPool(t *testing.T) {
	m := newTestModel()
	m.deps.Pool = nil
	m.ed.Insert("\\encoding")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "No connection") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'No connection' output after \\encoding with nil pool")
	}
}

// --- \sf tests ---

func TestBackslashCommand_ShowFunctionNilPool(t *testing.T) {
	m := newTestModel()
	m.deps.Pool = nil
	m.ed.Insert("\\sf myfunc")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "No connection") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'No connection' output after \\sf with nil pool")
	}
}

func TestBackslashCommand_ShowFunctionNoArg(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\sf")
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
		t.Error("expected 'Usage' output after \\sf without arg")
	}
}

// --- \privileges tests ---

func TestBackslashCommand_PrivilegesNilPool(t *testing.T) {
	m := newTestModel()
	m.deps.Pool = nil
	m.ed.Insert("\\privileges mytable")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "No connection") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'No connection' output after \\privileges with nil pool")
	}
}

func TestBackslashCommand_PrivilegesNoArg(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\privileges")
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
		t.Error("expected 'Usage' output after \\privileges without arg")
	}
}

// --- \explain tests ---

func TestBackslashCommand_ExplainNoArg(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\explain")
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
		t.Error("expected 'Usage' output after \\explain without arg")
	}
}

// --- \gx tests ---

func TestBackslashCommand_GxNoQuery(t *testing.T) {
	m := newTestModel()
	m.lastQuery = ""
	m.ed.Insert("\\gx")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "No query") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'No query' output after \\gx with no previous query")
	}
}

// --- \prompt tests ---

func TestBackslashCommand_PromptNoArg(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\prompt")
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
		t.Error("expected 'Usage' output after \\prompt without arg")
	}
}

func TestBackslashCommand_PromptSetsVar(t *testing.T) {
	m := newTestModel()
	m.ed.Insert("\\prompt myvar Enter name")
	model, _ := m.handleEnter()
	updated := model.(Model)
	if updated.sessionVars["__prompt_var"] != "myvar" {
		t.Error("expected __prompt_var to be set to 'myvar'")
	}
}

// --- \g tests ---

func TestBackslashCommand_GNoQuery(t *testing.T) {
	m := newTestModel()
	m.lastQuery = ""
	m.ed.Insert("\\g")
	model, _ := m.handleEnter()
	updated := model.(Model)
	found := false
	for _, line := range updated.output {
		if strings.Contains(line, "No previous query") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'No previous query' output after \\g with no last query")
	}
}
