package highlight

import (
	"fmt"
	"strings"

	"github.com/eyjian/mysh/config"
)

// styleMap maps token types to lipgloss-compatible style strings.
// The style strings come from config.ThemeConfig fields.
var tokenStyleKey = map[TokenType]string{
	TokenKeyword:    "Keyword",
	TokenString:     "String",
	TokenNumber:     "Number",
	TokenComment:    "Comment",
	TokenFunction:   "Function",
	TokenOperator:   "Operator",
	TokenIdentifier: "", // no special styling
	TokenVariable:   "Keyword", // variables styled like keywords
	TokenPunctuation: "", // no special styling
}

// ANSI color/style codes for common lipgloss-compatible style strings.
// This is a simple ANSI-based renderer that doesn't require lipgloss at runtime,
// keeping the highlighter self-contained and testable.
var ansiStyles = map[string]string{
	"bold magenta":  "\033[1;35m",
	"magenta":       "\033[35m",
	"yellow":        "\033[33m",
	"bold yellow":   "\033[1;33m",
	"cyan":          "\033[36m",
	"bold cyan":     "\033[1;36m",
	"dim":           "\033[2m",
	"green":         "\033[32m",
	"bold green":    "\033[1;32m",
	"white":         "\033[37m",
	"bold white":    "\033[1;37m",
	"red":           "\033[31m",
	"bold red":      "\033[1;31m",
	"blue":          "\033[34m",
	"bold blue":     "\033[1;34m",
	"bold":          "\033[1m",
	"underline":     "\033[4m",
}

const resetCode = "\033[0m"

// Highlighter renders SQL with syntax highlighting using ANSI escape codes.
type Highlighter struct {
	tokenizer *Tokenizer
	theme     *config.ThemeConfig
	styles    map[TokenType]string // cached ANSI codes per token type
}

// NewHighlighter creates a new Highlighter with the given theme.
func NewHighlighter(theme *config.ThemeConfig) *Highlighter {
	h := &Highlighter{
		tokenizer: NewTokenizer(),
		theme:     theme,
	}
	h.rebuildStyles()
	return h
}

// Highlight renders the input SQL with ANSI color codes.
func (h *Highlighter) Highlight(input string) string {
	tokens := h.tokenizer.Tokenize(input)
	var sb strings.Builder
	prevEnd := 0
	runes := []rune(input)

	for _, tok := range tokens {
		// Add any whitespace/gap between tokens
		if tok.Start > prevEnd {
			sb.WriteString(string(runes[prevEnd:tok.Start]))
		}

		style, ok := h.styles[tok.Type]
		if ok && style != "" {
			sb.WriteString(style)
			sb.WriteString(tok.Value)
			sb.WriteString(resetCode)
		} else {
			sb.WriteString(tok.Value)
		}

		prevEnd = tok.End
	}

	// Add any trailing text
	if prevEnd < len(runes) {
		sb.WriteString(string(runes[prevEnd:]))
	}

	return sb.String()
}

// SetTheme updates the highlighting theme.
func (h *Highlighter) SetTheme(theme *config.ThemeConfig) {
	h.theme = theme
	h.rebuildStyles()
}

// rebuildStyles rebuilds the ANSI style cache from the theme config.
func (h *Highlighter) rebuildStyles() {
	h.styles = make(map[TokenType]string, len(tokenStyleKey))

	if h.theme == nil {
		return
	}

	// Map theme fields to token types
	themeValues := map[string]string{
		"Keyword":  h.theme.Keyword,
		"String":   h.theme.String,
		"Number":   h.theme.Number,
		"Comment":  h.theme.Comment,
		"Function": h.theme.Function,
		"Operator": h.theme.Operator,
	}

	for tokType, styleKey := range tokenStyleKey {
		if styleKey == "" {
			continue
		}
		if val, ok := themeValues[styleKey]; ok && val != "" {
			if ansi, ok := ansiStyles[val]; ok {
				h.styles[tokType] = ansi
			} else {
				// Try to parse custom style string
				h.styles[tokType] = parseStyleString(val)
			}
		}
	}
}

// parseStyleString attempts to convert a lipgloss-style string to ANSI codes.
// Supports combinations like "bold magenta", "dim", "underline green", etc.
func parseStyleString(s string) string {
	parts := strings.Fields(s)
	var codes []string
	for _, part := range parts {
		switch strings.ToLower(part) {
		case "bold":
			codes = append(codes, "1")
		case "dim":
			codes = append(codes, "2")
		case "underline":
			codes = append(codes, "4")
		case "black":
			codes = append(codes, "30")
		case "red":
			codes = append(codes, "31")
		case "green":
			codes = append(codes, "32")
		case "yellow":
			codes = append(codes, "33")
		case "blue":
			codes = append(codes, "34")
		case "magenta":
			codes = append(codes, "35")
		case "cyan":
			codes = append(codes, "36")
		case "white":
			codes = append(codes, "37")
		}
	}
	if len(codes) == 0 {
		return ""
	}
	return fmt.Sprintf("\033[%sm", strings.Join(codes, ";"))
}
