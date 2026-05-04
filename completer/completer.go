package completer

import (
	"context"
	"sort"
	"strings"
	"unicode"

	"mysh/config"
	"mysh/highlight"
)

// SuggestionType represents the type of a completion suggestion.
type SuggestionType string

const (
	SuggestKeyword  SuggestionType = "Keyword"
	SuggestTable    SuggestionType = "Table"
	SuggestColumn   SuggestionType = "Column"
	SuggestDatabase SuggestionType = "Database"
	SuggestFunction SuggestionType = "Function"
	SuggestVariable SuggestionType = "Variable"
	SuggestAlias    SuggestionType = "Alias"
	SuggestOperator SuggestionType = "Operator"
)

// Suggestion represents a single completion candidate.
type Suggestion struct {
	Text   string
	Type   SuggestionType
	Detail string
}

// MetadataCache defines the interface for accessing database schema metadata.
// This is satisfied by metadata.Cache (implemented by developer_1).
type MetadataCache interface {
	Databases() []string
	Tables(db string) []string
	Columns(db, table string) []ColumnInfo
	Functions() []string
	IsDirty() bool
}

// ColumnInfo describes a database column (mirrors metadata.ColumnInfo).
type ColumnInfo struct {
	Name     string
	Type     string
	Nullable bool
	Key      string
	Default  string
	Extra    string
}

// Completer provides context-aware SQL auto-completion.
type Completer struct {
	analyzer   *ContextAnalyzer
	tokenizer  *highlight.Tokenizer
	metadata   MetadataCache
	cfg        *config.CompletionConfig
	keywords   []Suggestion
	functions  []Suggestion
}

// NewCompleter creates a new Completer with the given config.
func NewCompleter(cfg *config.CompletionConfig) *Completer {
	c := &Completer{
		analyzer:  NewContextAnalyzer(),
		tokenizer: highlight.NewTokenizer(),
		cfg:       cfg,
	}

	// Build keyword suggestions from the tokenizer's keyword set
	c.keywords = buildKeywordSuggestions()
	c.functions = buildFunctionSuggestions()

	return c
}

// SetMetadata injects the metadata cache for table/column/database completion.
func (c *Completer) SetMetadata(cache MetadataCache) {
	c.metadata = cache
}

// Complete returns a list of completion suggestions for the given input and cursor position.
func (c *Completer) Complete(ctx context.Context, input string, cursorPos int) ([]Suggestion, error) {
	// Determine the prefix to complete
	prefix := c.extractPrefix(input, cursorPos)

	// Check minimum characters threshold
	if c.cfg != nil && len(prefix) < c.cfg.MinChars && prefix == "" {
		return nil, nil
	}

	// Determine context
	contextType := c.analyzer.Analyze(input, cursorPos)

	// Gather candidates based on context
	var candidates []Suggestion
	switch contextType {
	case ContextStatementStart:
		candidates = c.completeStatementStart(prefix)
	case ContextAfterSelect:
		candidates = c.completeAfterSelect(prefix)
	case ContextAfterFrom:
		candidates = c.completeAfterFrom(prefix)
	case ContextAfterWhere:
		candidates = c.completeAfterWhere(prefix)
	case ContextAfterJoin:
		candidates = c.completeAfterJoin(prefix)
	case ContextAfterDot:
		candidates = c.completeAfterDot(input, cursorPos, prefix)
	case ContextAfterSet:
		candidates = c.completeAfterSet(prefix)
	case ContextAfterOrderBy:
		candidates = c.completeAfterOrderBy(prefix)
	case ContextAfterGroupBy:
		candidates = c.completeAfterGroupBy(prefix)
	default:
		candidates = c.completeFallback(prefix)
	}

	// Filter by prefix
	candidates = filterByPrefix(candidates, prefix)

	// Sort: exact matches first, then alphabetical
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Text == prefix && candidates[j].Text != prefix {
			return true
		}
		if candidates[i].Text != prefix && candidates[j].Text == prefix {
			return false
		}
		return candidates[i].Text < candidates[j].Text
	})

	// Limit results
	if c.cfg != nil && len(candidates) > c.cfg.MaxSuggestions {
		candidates = candidates[:c.cfg.MaxSuggestions]
	}

	return candidates, nil
}

// extractPrefix extracts the word being typed immediately before the cursor.
func (c *Completer) extractPrefix(input string, cursorPos int) string {
	if cursorPos <= 0 {
		return ""
	}
	runes := []rune(input)
	if cursorPos > len(runes) {
		cursorPos = len(runes)
	}

	end := cursorPos
	start := cursorPos - 1
	for start >= 0 && isCompletionChar(runes[start]) {
		start--
	}
	start++

	if start >= end {
		return ""
	}
	return string(runes[start:end])
}

// ---- Context-specific completion methods ----

func (c *Completer) completeStatementStart(prefix string) []Suggestion {
	// At statement start, suggest SQL keywords and built-in commands
	var suggestions []Suggestion
	suggestions = append(suggestions, c.keywords...)
	suggestions = append(suggestions, backtickCommands()...)
	return suggestions
}

func (c *Completer) completeAfterSelect(prefix string) []Suggestion {
	var suggestions []Suggestion
	// Suggest column names and functions
	if c.metadata != nil {
		suggestions = append(suggestions, c.columnSuggestions("")...)
	}
	suggestions = append(suggestions, c.functions...)
	// Also some relevant keywords
	suggestions = append(suggestions, keywordsFiltered("FROM", "AS", "DISTINCT", "COUNT", "SUM", "AVG", "MAX", "MIN", "*")...)
	return suggestions
}

func (c *Completer) completeAfterFrom(prefix string) []Suggestion {
	var suggestions []Suggestion
	// Suggest table names and database names
	if c.metadata != nil {
		suggestions = append(suggestions, c.tableSuggestions()...)
		suggestions = append(suggestions, c.databaseSuggestions()...)
	}
	// Some keywords that can follow FROM
	suggestions = append(suggestions, keywordsFiltered("WHERE", "JOIN", "INNER", "LEFT", "RIGHT", "CROSS", "ON", "AS", "ORDER", "GROUP", "HAVING", "LIMIT")...)
	return suggestions
}

func (c *Completer) completeAfterWhere(prefix string) []Suggestion {
	var suggestions []Suggestion
	// Suggest column names
	if c.metadata != nil {
		suggestions = append(suggestions, c.columnSuggestions("")...)
	}
	suggestions = append(suggestions, c.functions...)
	suggestions = append(suggestions, keywordsFiltered("AND", "OR", "NOT", "IN", "LIKE", "BETWEEN", "IS", "NULL", "EXISTS", "BETWEEN")...)
	return suggestions
}

func (c *Completer) completeAfterJoin(prefix string) []Suggestion {
	var suggestions []Suggestion
	// Suggest table names
	if c.metadata != nil {
		suggestions = append(suggestions, c.tableSuggestions()...)
	}
	suggestions = append(suggestions, keywordsFiltered("ON", "USING", "AS")...)
	return suggestions
}

func (c *Completer) completeAfterDot(input string, cursorPos int, prefix string) []Suggestion {
	// Extract the table name before the dot
	tableName := c.extractTableBeforeDot(input, cursorPos)
	if tableName == "" {
		return nil
	}

	var suggestions []Suggestion
	if c.metadata != nil {
		suggestions = append(suggestions, c.columnSuggestions(tableName)...)
	}
	// Suggest * after table.
	if prefix == "" || strings.HasPrefix("*", strings.ToUpper(prefix)) {
		suggestions = append(suggestions, Suggestion{Text: "*", Type: SuggestKeyword, Detail: "all columns"})
	}
	return suggestions
}

func (c *Completer) completeAfterSet(prefix string) []Suggestion {
	var suggestions []Suggestion
	if c.metadata != nil {
		suggestions = append(suggestions, c.databaseSuggestions()...)
	}
	suggestions = append(suggestions, keywordsFiltered("NAMES", "CHARACTER", "AUTOCOMMIT", "GLOBAL", "SESSION")...)
	return suggestions
}

func (c *Completer) completeAfterOrderBy(prefix string) []Suggestion {
	var suggestions []Suggestion
	if c.metadata != nil {
		suggestions = append(suggestions, c.columnSuggestions("")...)
	}
	suggestions = append(suggestions, keywordsFiltered("ASC", "DESC", "LIMIT")...)
	return suggestions
}

func (c *Completer) completeAfterGroupBy(prefix string) []Suggestion {
	var suggestions []Suggestion
	if c.metadata != nil {
		suggestions = append(suggestions, c.columnSuggestions("")...)
	}
	suggestions = append(suggestions, keywordsFiltered("HAVING", "ORDER", "LIMIT")...)
	return suggestions
}

func (c *Completer) completeFallback(prefix string) []Suggestion {
	// Fallback: suggest keywords, tables, columns
	var suggestions []Suggestion
	suggestions = append(suggestions, c.keywords...)
	if c.metadata != nil {
		suggestions = append(suggestions, c.tableSuggestions()...)
		suggestions = append(suggestions, c.columnSuggestions("")...)
	}
	return suggestions
}

// ---- Metadata-based suggestion helpers ----

func (c *Completer) tableSuggestions() []Suggestion {
	if c.metadata == nil {
		return nil
	}
	tables := c.metadata.Tables("")
	suggestions := make([]Suggestion, len(tables))
	for i, t := range tables {
		suggestions[i] = Suggestion{Text: t, Type: SuggestTable, Detail: "table"}
	}
	return suggestions
}

func (c *Completer) databaseSuggestions() []Suggestion {
	if c.metadata == nil {
		return nil
	}
	dbs := c.metadata.Databases()
	suggestions := make([]Suggestion, len(dbs))
	for i, d := range dbs {
		suggestions[i] = Suggestion{Text: d, Type: SuggestDatabase, Detail: "database"}
	}
	return suggestions
}

func (c *Completer) columnSuggestions(table string) []Suggestion {
	if c.metadata == nil {
		return nil
	}
	var columns []ColumnInfo
	if table != "" {
		columns = c.metadata.Columns("", table)
	} else {
		// Get columns from all tables (may be slow, but useful for WHERE context)
		tables := c.metadata.Tables("")
		for _, t := range tables {
			cols := c.metadata.Columns("", t)
			columns = append(columns, cols...)
		}
	}
	suggestions := make([]Suggestion, len(columns))
	for i, col := range columns {
		suggestions[i] = Suggestion{
			Text:   col.Name,
			Type:   SuggestColumn,
			Detail: col.Type,
		}
	}
	return suggestions
}

// extractTableBeforeDot extracts the table identifier before a dot at cursor position.
func (c *Completer) extractTableBeforeDot(input string, cursorPos int) string {
	runes := []rune(input)
	if cursorPos > len(runes) {
		cursorPos = len(runes)
	}

	// Find the dot before cursor
	pos := cursorPos - 1
	for pos >= 0 && runes[pos] != '.' {
		pos--
	}
	if pos < 0 {
		return ""
	}

	// Scan backwards for the identifier before the dot
	end := pos
	start := pos - 1
	// Handle backtick-quoted identifiers
	if start >= 0 && runes[start] == '`' {
		innerEnd := start
		start--
		for start >= 0 && runes[start] != '`' {
			start--
		}
		return string(runes[start+1 : innerEnd])
	}
	// Regular identifier
	for start >= 0 && isCompletionChar(runes[start]) {
		start--
	}
	start++

	return string(runes[start:end])
}

// ---- Helper functions ----

func isCompletionChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '@'
}

func filterByPrefix(suggestions []Suggestion, prefix string) []Suggestion {
	if prefix == "" {
		return suggestions
	}
	upper := strings.ToUpper(prefix)
	var filtered []Suggestion
	for _, s := range suggestions {
		if strings.HasPrefix(strings.ToUpper(s.Text), upper) {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

func keywordsFiltered(keys ...string) []Suggestion {
	suggestions := make([]Suggestion, len(keys))
	for i, k := range keys {
		suggestions[i] = Suggestion{Text: k, Type: SuggestKeyword}
	}
	return suggestions
}

func backtickCommands() []Suggestion {
	return []Suggestion{
		{Text: "\\help", Type: SuggestKeyword, Detail: "show help"},
		{Text: "\\connect", Type: SuggestKeyword, Detail: "connect to database"},
		{Text: "\\use", Type: SuggestKeyword, Detail: "switch database"},
		{Text: "\\refresh", Type: SuggestKeyword, Detail: "refresh metadata cache"},
		{Text: "\\status", Type: SuggestKeyword, Detail: "show connection status"},
		{Text: "\\format", Type: SuggestKeyword, Detail: "change output format"},
		{Text: "\\history", Type: SuggestKeyword, Detail: "search history"},
		{Text: "\\source", Type: SuggestKeyword, Detail: "execute SQL file"},
		{Text: "\\quit", Type: SuggestKeyword, Detail: "exit mysh"},
	}
}

// buildKeywordSuggestions builds the keyword suggestion list from the tokenizer's keyword set.
func buildKeywordSuggestions() []Suggestion {
	// Use a curated list of the most useful keywords for completion
	keywords := []string{
		"SELECT", "FROM", "WHERE", "INSERT", "INTO", "VALUES", "UPDATE", "DELETE",
		"CREATE", "ALTER", "DROP", "TRUNCATE", "TABLE", "INDEX", "VIEW", "DATABASE",
		"JOIN", "INNER", "LEFT", "RIGHT", "OUTER", "CROSS", "FULL", "ON", "USING",
		"AND", "OR", "NOT", "IN", "IS", "NULL", "LIKE", "BETWEEN", "EXISTS",
		"AS", "DISTINCT", "GROUP", "BY", "ORDER", "ASC", "DESC", "HAVING",
		"LIMIT", "OFFSET", "UNION", "ALL", "SET", "SHOW", "DESCRIBE", "EXPLAIN",
		"USE", "IF", "CASE", "WHEN", "THEN", "ELSE", "END", "BEGIN",
		"COMMIT", "ROLLBACK", "PRIMARY", "KEY", "UNIQUE", "FOREIGN",
		"DEFAULT", "AUTO_INCREMENT", "CONSTRAINT", "REFERENCES",
		"GRANT", "REVOKE", "FLUSH", "WITH", "RECURSIVE",
	}
	suggestions := make([]Suggestion, len(keywords))
	for i, k := range keywords {
		suggestions[i] = Suggestion{Text: k, Type: SuggestKeyword}
	}
	return suggestions
}

// buildFunctionSuggestions builds the function suggestion list.
func buildFunctionSuggestions() []Suggestion {
	functions := []string{
		"COUNT", "SUM", "AVG", "MIN", "MAX", "GROUP_CONCAT",
		"CONCAT", "CONCAT_WS", "LENGTH", "SUBSTRING", "TRIM",
		"UPPER", "LOWER", "REPLACE", "LEFT", "RIGHT",
		"ABS", "CEIL", "FLOOR", "ROUND", "MOD",
		"NOW", "CURDATE", "CURTIME", "DATE_ADD", "DATE_SUB",
		"DATEDIFF", "DATE_FORMAT", "YEAR", "MONTH", "DAY",
		"IFNULL", "NULLIF", "COALESCE", "CAST", "CONVERT",
		"DATABASE", "USER", "VERSION", "LAST_INSERT_ID",
		"ROW_NUMBER", "RANK", "DENSE_RANK",
	}
	suggestions := make([]Suggestion, len(functions))
	for i, f := range functions {
		suggestions[i] = Suggestion{Text: f, Type: SuggestFunction, Detail: "function"}
	}
	return suggestions
}
