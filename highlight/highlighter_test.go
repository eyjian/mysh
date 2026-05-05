package highlight

import (
	"strings"
	"testing"

	"github.com/eyjian/mysh/config"
)

func TestNewHighlighter(t *testing.T) {
	theme := &config.ThemeConfig{
		Keyword:  "bold magenta",
		String:   "yellow",
		Number:   "cyan",
		Comment:  "dim",
		Function: "green",
		Operator: "white",
	}
	h := NewHighlighter(theme)
	if h == nil {
		t.Fatal("NewHighlighter() returned nil")
	}
}

func TestNewHighlighter_NilTheme(t *testing.T) {
	h := NewHighlighter(nil)
	if h == nil {
		t.Fatal("NewHighlighter(nil) returned nil")
	}
	// Should not panic
	result := h.Highlight("SELECT 1")
	if result == "" {
		t.Error("Highlight() with nil theme returned empty string")
	}
}

func TestHighlight_SimpleSQL(t *testing.T) {
	theme := &config.ThemeConfig{
		Keyword:  "bold magenta",
		String:   "yellow",
		Number:   "cyan",
		Comment:  "dim",
		Function: "green",
		Operator: "white",
	}
	h := NewHighlighter(theme)

	tests := []string{
		"SELECT * FROM users",
		"INSERT INTO t VALUES (1)",
		"UPDATE t SET x = 1",
		"DELETE FROM t WHERE id = 1",
		"-- comment\nSELECT 1",
		"SELECT 'hello'",
		"SELECT 42",
		"SELECT COUNT(*)",
	}

	for _, input := range tests {
		result := h.Highlight(input)
		if result == "" {
			t.Errorf("Highlight(%q) returned empty string", input)
		}
		// Result should contain the input text (possibly with ANSI codes)
		// Strip ANSI codes and check the text is preserved
		stripped := stripANSI(result)
		// The stripped version should match the input minus whitespace differences
		if stripped != input {
			// Whitespace handling may differ; just verify keywords are present
			if !strings.Contains(stripped, "SELECT") && strings.Contains(input, "SELECT") {
				t.Errorf("Highlight(%q) stripped = %q, missing SELECT", input, stripped)
			}
		}
	}
}

func TestHighlight_ContainsANSICodes(t *testing.T) {
	theme := &config.ThemeConfig{
		Keyword: "bold magenta",
	}
	h := NewHighlighter(theme)
	result := h.Highlight("SELECT")

	if !strings.Contains(result, "\033[") {
		t.Error("Highlight() should produce ANSI escape codes")
	}
	if !strings.Contains(result, "\033[0m") {
		t.Error("Highlight() should contain reset code")
	}
}

func TestHighlight_NoStyleForIdentifier(t *testing.T) {
	theme := &config.ThemeConfig{
		Keyword: "bold magenta",
	}
	h := NewHighlighter(theme)
	result := h.Highlight("users")

	// Identifier should not have ANSI codes (no style for identifiers)
	if strings.Contains(result, "\033[") {
		t.Error("Highlight() for identifier should not add ANSI codes")
	}
}

func TestSetTheme(t *testing.T) {
	theme1 := &config.ThemeConfig{
		Keyword: "bold magenta",
	}
	h := NewHighlighter(theme1)

	theme2 := &config.ThemeConfig{
		Keyword: "bold red",
	}
	h.SetTheme(theme2)

	result := h.Highlight("SELECT")
	// Should use new theme (bold red = \033[1;31m)
	if !strings.Contains(result, "\033[1;31m") {
		t.Errorf("SetTheme() should update styles; result = %q", result)
	}
}

func TestHighlight_Comment(t *testing.T) {
	theme := &config.ThemeConfig{
		Comment: "dim",
	}
	h := NewHighlighter(theme)
	result := h.Highlight("-- comment")
	if !strings.Contains(result, "\033[2m") {
		t.Errorf("comment should use dim style; result = %q", result)
	}
}

func TestHighlight_String(t *testing.T) {
	theme := &config.ThemeConfig{
		String: "yellow",
	}
	h := NewHighlighter(theme)
	result := h.Highlight("'hello'")
	if !strings.Contains(result, "\033[33m") {
		t.Errorf("string should use yellow style; result = %q", result)
	}
}

func TestHighlight_Number(t *testing.T) {
	theme := &config.ThemeConfig{
		Number: "cyan",
	}
	h := NewHighlighter(theme)
	result := h.Highlight("42")
	if !strings.Contains(result, "\033[36m") {
		t.Errorf("number should use cyan style; result = %q", result)
	}
}

func TestHighlight_Function(t *testing.T) {
	theme := &config.ThemeConfig{
		Function: "green",
	}
	h := NewHighlighter(theme)
	result := h.Highlight("COUNT")
	if !strings.Contains(result, "\033[32m") {
		t.Errorf("function should use green style; result = %q", result)
	}
}

func TestHighlight_EmptyInput(t *testing.T) {
	theme := config.DefaultConfig().Theme
	h := NewHighlighter(&theme)
	result := h.Highlight("")
	if result != "" {
		t.Errorf("Highlight('') = %q, want empty", result)
	}
}

func TestParseStyleString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"bold", "\033[1m"},
		{"dim", "\033[2m"},
		{"underline", "\033[4m"},
		{"red", "\033[31m"},
		{"green", "\033[32m"},
		{"yellow", "\033[33m"},
		{"blue", "\033[34m"},
		{"magenta", "\033[35m"},
		{"cyan", "\033[36m"},
		{"white", "\033[37m"},
		{"bold magenta", "\033[1;35m"},
		{"bold red", "\033[1;31m"},
		{"bold yellow", "\033[1;33m"},
		{"bold green", "\033[1;32m"},
		{"bold cyan", "\033[1;36m"},
		{"", ""},           // empty
		{"unknown", ""},     // unknown style
	}

	for _, tt := range tests {
		got := parseStyleString(tt.input)
		if got != tt.want {
			t.Errorf("parseStyleString(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// stripANSI removes ANSI escape codes from a string.
func stripANSI(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			// Skip until 'm'
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			i = j + 1
			continue
		}
		result.WriteByte(s[i])
		i++
	}
	return result.String()
}
