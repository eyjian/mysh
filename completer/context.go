package completer

import (
	"strings"
	"unicode"

	"github.com/eyjian/mysh/highlight"
)

// ContextType represents the type of completion context at the cursor position.
type ContextType string

const (
	ContextStatementStart ContextType = "StatementStart"
	ContextAfterFrom      ContextType = "AfterFrom"
	ContextAfterSelect    ContextType = "AfterSelect"
	ContextAfterWhere     ContextType = "AfterWhere"
	ContextAfterJoin      ContextType = "AfterJoin"
	ContextAfterDot       ContextType = "AfterDot"
	ContextAfterSet       ContextType = "AfterSet"
	ContextAfterOrderBy   ContextType = "AfterOrderBy"
	ContextAfterGroupBy   ContextType = "AfterGroupBy"
	ContextUnknown        ContextType = "Unknown"
)

// keywords that signal specific contexts
var fromKeywords = map[string]bool{
	"FROM": true,
}

var joinKeywords = map[string]bool{
	"JOIN": true, "INNER": true, "LEFT": true, "RIGHT": true,
	"OUTER": true, "CROSS": true, "FULL": true, "NATURAL": true,
}

var whereKeywords = map[string]bool{
	"WHERE": true,
}

var selectKeywords = map[string]bool{
	"SELECT": true, "DISTINCT": true,
}

var setKeywords = map[string]bool{
	"SET": true,
}

// ContextAnalyzer determines the completion context at a cursor position.
type ContextAnalyzer struct {
	tokenizer *highlight.Tokenizer
}

// NewContextAnalyzer creates a new ContextAnalyzer.
func NewContextAnalyzer() *ContextAnalyzer {
	return &ContextAnalyzer{
		tokenizer: highlight.NewTokenizer(),
	}
}

// Analyze determines the completion context type based on input and cursor position.
// It uses token-based analysis first, then falls back to heuristic regex matching.
func (ca *ContextAnalyzer) Analyze(input string, cursorPos int) ContextType {
	if cursorPos <= 0 {
		return ContextStatementStart
	}

	// Get the text up to the cursor
	prefix := input
	runes := []rune(input)
	if cursorPos <= len(runes) {
		prefix = string(runes[:cursorPos])
	}

	// Trim trailing whitespace for analysis
	trimmed := strings.TrimRight(prefix, " \t")

	if trimmed == "" {
		return ContextStatementStart
	}

	// Tokenize the prefix
	tokens := ca.tokenizer.Tokenize(trimmed)
	if len(tokens) == 0 {
		return ContextStatementStart
	}

	// Check for dot context (table.column)
	if ca.isAfterDot(trimmed, tokens) {
		return ContextAfterDot
	}

	// Check for compound keyword patterns (ORDER BY, GROUP BY) before single-token analysis
	// by looking at the last two keyword tokens
	if lastCompound := ca.findLastCompoundKeyword(tokens); lastCompound != "" {
		switch lastCompound {
		case "ORDER BY":
			return ContextAfterOrderBy
		case "GROUP BY":
			return ContextAfterGroupBy
		}
	}

	// Check if we're at statement start (after a semicolon)
	if ca.isAfterSemicolon(trimmed) {
		return ContextStatementStart
	}

	// Analyze based on the last significant token
	lastToken := tokens[len(tokens)-1]
	lastKeyword := ca.findLastKeyword(tokens)

	// If the last token is a keyword, use it to determine context
	if lastToken.Type == highlight.TokenKeyword {
		upper := strings.ToUpper(lastToken.Value)
		if fromKeywords[upper] {
			return ContextAfterFrom
		}
		if joinKeywords[upper] {
			return ContextAfterJoin
		}
		if whereKeywords[upper] {
			return ContextAfterWhere
		}
		if selectKeywords[upper] {
			return ContextAfterSelect
		}
		if setKeywords[upper] {
			return ContextAfterSet
		}
		// After AND/OR, we're likely in a WHERE-like context
		if upper == "AND" || upper == "OR" {
			return ContextAfterWhere
		}
	}

	// If the last token is an operator or punctuation, check the previous keyword
	if lastToken.Type == highlight.TokenOperator || lastToken.Type == highlight.TokenPunctuation {
		if lastKeyword != "" {
			return ca.contextFromKeyword(lastKeyword, tokens)
		}
	}

	// If we're in the middle of typing an identifier, check context
	if lastToken.Type == highlight.TokenIdentifier || lastToken.Type == highlight.TokenFunction {
		if lastKeyword != "" {
			return ca.contextFromKeyword(lastKeyword, tokens)
		}
	}

	// Heuristic fallback
	return ca.heuristicAnalyze(trimmed)
}

// isAfterDot checks if the cursor is right after a dot (table.column completion).
func (ca *ContextAnalyzer) isAfterDot(trimmed string, tokens []highlight.Token) bool {
	if len(tokens) == 0 {
		return false
	}
	lastToken := tokens[len(tokens)-1]
	if lastToken.Type == highlight.TokenPunctuation && lastToken.Value == "." {
		return true
	}
	if strings.HasSuffix(trimmed, ".") {
		return true
	}
	return false
}

// findLastKeyword finds the last keyword token before the current position.
func (ca *ContextAnalyzer) findLastKeyword(tokens []highlight.Token) string {
	for i := len(tokens) - 1; i >= 0; i-- {
		if tokens[i].Type == highlight.TokenKeyword {
			return strings.ToUpper(tokens[i].Value)
		}
	}
	return ""
}

// findLastCompoundKeyword finds the last compound keyword pattern (e.g., "ORDER BY", "GROUP BY").
func (ca *ContextAnalyzer) findLastCompoundKeyword(tokens []highlight.Token) string {
	// Scan backwards for the last two consecutive keyword tokens
	var lastKeywords []string
	for i := len(tokens) - 1; i >= 0 && len(lastKeywords) < 2; i-- {
		if tokens[i].Type == highlight.TokenKeyword {
			lastKeywords = append(lastKeywords, strings.ToUpper(tokens[i].Value))
		}
	}
	if len(lastKeywords) == 2 {
		compound := lastKeywords[1] + " " + lastKeywords[0] // reversed order
		switch compound {
		case "ORDER BY":
			return "ORDER BY"
		case "GROUP BY":
			return "GROUP BY"
		}
	}
	return ""
}

// contextFromKeyword maps a keyword to a completion context.
func (ca *ContextAnalyzer) contextFromKeyword(keyword string, tokens []highlight.Token) ContextType {
	upper := strings.ToUpper(keyword)

	if fromKeywords[upper] {
		return ContextAfterFrom
	}
	if joinKeywords[upper] {
		return ContextAfterJoin
	}
	if whereKeywords[upper] {
		return ContextAfterWhere
	}
	if selectKeywords[upper] {
		return ContextAfterSelect
	}
	if setKeywords[upper] {
		return ContextAfterSet
	}
	if upper == "ORDER" {
		return ContextAfterOrderBy
	}
	if upper == "GROUP" {
		return ContextAfterGroupBy
	}

	// Check compound keywords (ORDER BY, GROUP BY)
	if upper == "BY" && len(tokens) >= 2 {
		for i := len(tokens) - 2; i >= 0; i-- {
			if tokens[i].Type == highlight.TokenKeyword {
				prev := strings.ToUpper(tokens[i].Value)
				if prev == "ORDER" {
					return ContextAfterOrderBy
				}
				if prev == "GROUP" {
					return ContextAfterGroupBy
				}
				break
			}
		}
	}

	return ContextUnknown
}

// isAfterSemicolon checks if the last non-whitespace character is a semicolon.
func (ca *ContextAnalyzer) isAfterSemicolon(trimmed string) bool {
	if len(trimmed) == 0 {
		return false
	}
	return trimmed[len(trimmed)-1] == ';'
}

// heuristicAnalyze falls back to regex-based context inference.
func (ca *ContextAnalyzer) heuristicAnalyze(trimmed string) ContextType {
	upper := strings.ToUpper(trimmed)

	// Remove trailing partial word for matching
	upper = strings.TrimRightFunc(upper, func(r rune) bool {
		return unicode.IsLetter(r) || r == '_'
	})
	upper = strings.TrimRight(upper, " \t")

	if strings.HasSuffix(upper, "FROM") {
		return ContextAfterFrom
	}
	if strings.HasSuffix(upper, "JOIN") || strings.HasSuffix(upper, "INNER JOIN") ||
		strings.HasSuffix(upper, "LEFT JOIN") || strings.HasSuffix(upper, "RIGHT JOIN") ||
		strings.HasSuffix(upper, "CROSS JOIN") {
		return ContextAfterJoin
	}
	if strings.HasSuffix(upper, "WHERE") {
		return ContextAfterWhere
	}
	if strings.HasSuffix(upper, "SELECT") || strings.HasSuffix(upper, "SELECT DISTINCT") {
		return ContextAfterSelect
	}
	if strings.HasSuffix(upper, "SET") {
		return ContextAfterSet
	}
	if strings.HasSuffix(upper, "ORDER BY") {
		return ContextAfterOrderBy
	}
	if strings.HasSuffix(upper, "GROUP BY") {
		return ContextAfterGroupBy
	}
	if strings.HasSuffix(upper, ".") {
		return ContextAfterDot
	}

	return ContextUnknown
}
