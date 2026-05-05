package completer

import (
	"context"
	"testing"

	"github.com/eyjian/mysh/config"
)

func TestContextAnalyzerStatementStart(t *testing.T) {
	ca := NewContextAnalyzer()
	if ca.Analyze("", 0) != ContextStatementStart {
		t.Error("empty input should be StatementStart")
	}
	if ca.Analyze("  ", 2) != ContextStatementStart {
		t.Error("whitespace input should be StatementStart")
	}
}

func TestContextAnalyzerAfterFrom(t *testing.T) {
	ca := NewContextAnalyzer()
	if ca.Analyze("SELECT * FROM ", 14) != ContextAfterFrom {
		t.Error("'SELECT * FROM ' should be AfterFrom")
	}
}

func TestContextAnalyzerAfterSelect(t *testing.T) {
	ca := NewContextAnalyzer()
	if ca.Analyze("SELECT ", 7) != ContextAfterSelect {
		t.Error("'SELECT ' should be AfterSelect")
	}
}

func TestContextAnalyzerAfterWhere(t *testing.T) {
	ca := NewContextAnalyzer()
	if ca.Analyze("SELECT * FROM t WHERE ", 22) != ContextAfterWhere {
		t.Error("'... WHERE ' should be AfterWhere")
	}
}

func TestContextAnalyzerAfterJoin(t *testing.T) {
	ca := NewContextAnalyzer()
	if ca.Analyze("SELECT * FROM t JOIN ", 21) != ContextAfterJoin {
		t.Error("'... JOIN ' should be AfterJoin")
	}
	if ca.Analyze("SELECT * FROM t LEFT JOIN ", 26) != ContextAfterJoin {
		t.Error("'... LEFT JOIN ' should be AfterJoin")
	}
}

func TestContextAnalyzerAfterDot(t *testing.T) {
	ca := NewContextAnalyzer()
	if ca.Analyze("SELECT t.", 10) != ContextAfterDot {
		t.Error("'SELECT t.' should be AfterDot")
	}
}

func TestContextAnalyzerAfterSet(t *testing.T) {
	ca := NewContextAnalyzer()
	if ca.Analyze("SET ", 4) != ContextAfterSet {
		t.Error("'SET ' should be AfterSet")
	}
}

// mockMetadataCache implements MetadataCache for testing
type mockMetadataCache struct {
	dbs       []string
	tables    []string
	columns   []ColumnInfo
	functions []string
	dirty     bool
}

func (m *mockMetadataCache) Databases() []string                    { return m.dbs }
func (m *mockMetadataCache) Tables(db string) []string              { return m.tables }
func (m *mockMetadataCache) Columns(db, table string) []ColumnInfo  { return m.columns }
func (m *mockMetadataCache) Functions() []string                    { return m.functions }
func (m *mockMetadataCache) IsDirty() bool                          { return m.dirty }
func (m *mockMetadataCache) MarkDirty()                             { m.dirty = true }

func newMockCache() *mockMetadataCache {
	return &mockMetadataCache{
		dbs:    []string{"testdb", "mydb"},
		tables: []string{"users", "orders", "products"},
		columns: []ColumnInfo{
			{Name: "id", Type: "int", Nullable: false, Key: "PRI"},
			{Name: "name", Type: "varchar(255)", Nullable: true},
			{Name: "email", Type: "varchar(255)", Nullable: true},
		},
		functions: []string{"COUNT", "SUM", "AVG", "MAX", "MIN"},
	}
}

func TestCompleterStatementStart(t *testing.T) {
	cfg := &config.CompletionConfig{MinChars: 2, MaxSuggestions: 15}
	c := NewCompleter(cfg)
	c.SetMetadata(newMockCache())

	suggestions, err := c.Complete(context.Background(), "SEL", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, s := range suggestions {
		if s.Text == "SELECT" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected SELECT in suggestions, got %v", suggestions)
	}
}

func TestCompleterAfterFrom(t *testing.T) {
	cfg := &config.CompletionConfig{MinChars: 2, MaxSuggestions: 15}
	c := NewCompleter(cfg)
	c.SetMetadata(newMockCache())

	suggestions, err := c.Complete(context.Background(), "SELECT * FROM u", 15)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, s := range suggestions {
		if s.Text == "users" && s.Type == SuggestTable {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'users' table in suggestions, got %v", suggestions)
	}
}

func TestCompleterAfterDot(t *testing.T) {
	cfg := &config.CompletionConfig{MinChars: 2, MaxSuggestions: 15}
	c := NewCompleter(cfg)
	c.SetMetadata(newMockCache())

	suggestions, err := c.Complete(context.Background(), "SELECT users.i", 14)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, s := range suggestions {
		if s.Text == "id" && s.Type == SuggestColumn {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'id' column in suggestions, got %v", suggestions)
	}
}

func TestCompleterMaxSuggestions(t *testing.T) {
	cfg := &config.CompletionConfig{MinChars: 0, MaxSuggestions: 3}
	c := NewCompleter(cfg)
	c.SetMetadata(newMockCache())

	suggestions, err := c.Complete(context.Background(), "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) > 3 {
		t.Errorf("expected at most 3 suggestions, got %d", len(suggestions))
	}
}

func TestCompleterNoMetadata(t *testing.T) {
	cfg := &config.CompletionConfig{MinChars: 2, MaxSuggestions: 15}
	c := NewCompleter(cfg)

	suggestions, err := c.Complete(context.Background(), "SEL", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, s := range suggestions {
		if s.Text == "SELECT" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected SELECT keyword suggestion even without metadata")
	}
}

func TestCompleterAfterWhere(t *testing.T) {
	cfg := &config.CompletionConfig{MinChars: 2, MaxSuggestions: 15}
	c := NewCompleter(cfg)
	c.SetMetadata(newMockCache())

	suggestions, err := c.Complete(context.Background(), "SELECT * FROM users WHERE n", 28)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, s := range suggestions {
		if s.Text == "name" && s.Type == SuggestColumn {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'name' column in suggestions, got %v", suggestions)
	}
}

// ---- Additional tests for edge paths ----

func TestExtractTableBeforeDot(t *testing.T) {
	c := NewCompleter(nil)

	tests := []struct {
		name       string
		input      string
		cursorPos  int
		wantTable  string
	}{
		{"simple dot", "SELECT users.", 13, "users"},
		{"dot with column prefix", "SELECT users.id", 16, "users"},
		{"backtick identifier", "SELECT `user table`.", 20, "user table"},
		{"no dot", "SELECT users", 12, ""},
		{"dot at start", ".", 1, ""},
		{"empty input", "", 0, ""},
		{"cursor beyond input", "SELECT t.", 100, "t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.extractTableBeforeDot(tt.input, tt.cursorPos)
			if got != tt.wantTable {
				t.Errorf("extractTableBeforeDot(%q, %d) = %q, want %q", tt.input, tt.cursorPos, got, tt.wantTable)
			}
		})
	}
}

func TestCompleterAfterJoin(t *testing.T) {
	cfg := &config.CompletionConfig{MinChars: 0, MaxSuggestions: 50}
	c := NewCompleter(cfg)
	c.SetMetadata(newMockCache())

	suggestions, err := c.Complete(context.Background(), "SELECT * FROM users JOIN o", 27)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, s := range suggestions {
		if s.Text == "orders" && s.Type == SuggestTable {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'orders' table in JOIN suggestions, got %v", suggestions)
	}
}

func TestCompleterAfterSet(t *testing.T) {
	cfg := &config.CompletionConfig{MinChars: 0, MaxSuggestions: 50}
	c := NewCompleter(cfg)
	c.SetMetadata(newMockCache())

	suggestions, err := c.Complete(context.Background(), "SET N", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, s := range suggestions {
		if s.Text == "NAMES" && s.Type == SuggestKeyword {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'NAMES' keyword in SET suggestions, got %v", suggestions)
	}
}

func TestCompleterAfterOrderBy(t *testing.T) {
	ca := NewContextAnalyzer()
	if ca.Analyze("SELECT * FROM users ORDER BY ", 29) != ContextAfterOrderBy {
		t.Error("'... ORDER BY ' should be AfterOrderBy")
	}
}

func TestCompleterAfterGroupBy(t *testing.T) {
	ca := NewContextAnalyzer()
	if ca.Analyze("SELECT * FROM users GROUP BY ", 29) != ContextAfterGroupBy {
		t.Error("'... GROUP BY ' should be AfterGroupBy")
	}
}

func TestCompleterAfterDotWithAsterisk(t *testing.T) {
	cfg := &config.CompletionConfig{MinChars: 0, MaxSuggestions: 50}
	c := NewCompleter(cfg)
	c.SetMetadata(newMockCache())

	suggestions, err := c.Complete(context.Background(), "SELECT users.", 13)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, s := range suggestions {
		if s.Text == "*" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected '*' in dot suggestions, got %v", suggestions)
	}
}

func TestCompleterFallbackUnknown(t *testing.T) {
	cfg := &config.CompletionConfig{MinChars: 0, MaxSuggestions: 50}
	c := NewCompleter(cfg)
	c.SetMetadata(newMockCache())

	suggestions, err := c.Complete(context.Background(), "SOMEUNKNOWN ", 12)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) == 0 {
		t.Error("fallback should return some suggestions")
	}
}

func TestCompleterAfterSemicolon(t *testing.T) {
	ca := NewContextAnalyzer()
	if ca.Analyze("SELECT 1; ", 10) != ContextStatementStart {
		t.Error("after semicolon should be StatementStart")
	}
}

func TestCompleterANDOR(t *testing.T) {
	ca := NewContextAnalyzer()
	if ca.Analyze("SELECT * FROM t WHERE id = 1 AND ", 32) != ContextAfterWhere {
		t.Error("'... AND ' should be AfterWhere")
	}
}

func TestContextAnalyzerAfterDistinct(t *testing.T) {
	ca := NewContextAnalyzer()
	if ca.Analyze("SELECT DISTINCT ", 16) != ContextAfterSelect {
		t.Error("'SELECT DISTINCT ' should be AfterSelect")
	}
}

func TestIsCompletionChar(t *testing.T) {
	tests := []struct {
		r    rune
		want bool
	}{
		{'a', true},
		{'Z', true},
		{'0', true},
		{'_', true},
		{'@', true},
		{' ', false},
		{'.', false},
		{',', false},
		{'(', false},
	}
	for _, tt := range tests {
		if got := isCompletionChar(tt.r); got != tt.want {
			t.Errorf("isCompletionChar(%q) = %v, want %v", tt.r, got, tt.want)
		}
	}
}

func TestFilterByPrefix(t *testing.T) {
	suggestions := []Suggestion{
		{Text: "SELECT", Type: SuggestKeyword},
		{Text: "SET", Type: SuggestKeyword},
		{Text: "SHOW", Type: SuggestKeyword},
		{Text: "INSERT", Type: SuggestKeyword},
	}
	filtered := filterByPrefix(suggestions, "SE")
	if len(filtered) != 2 {
		t.Errorf("expected 2 filtered suggestions, got %d: %v", len(filtered), filtered)
	}
}

func TestCompleterDatabaseSuggestion(t *testing.T) {
	cfg := &config.CompletionConfig{MinChars: 0, MaxSuggestions: 50}
	c := NewCompleter(cfg)
	c.SetMetadata(newMockCache())

	suggestions, err := c.Complete(context.Background(), "SELECT * FROM t", 15)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, s := range suggestions {
		if s.Type == SuggestDatabase {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected database suggestions after FROM, got %v", suggestions)
	}
}

func TestCompleterColumnDetail(t *testing.T) {
	cfg := &config.CompletionConfig{MinChars: 0, MaxSuggestions: 50}
	c := NewCompleter(cfg)
	c.SetMetadata(newMockCache())

	suggestions, err := c.Complete(context.Background(), "SELECT * FROM users WHERE i", 28)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, s := range suggestions {
		if s.Text == "id" && s.Type == SuggestColumn {
			if s.Detail != "int" {
				t.Errorf("expected column detail 'int', got %q", s.Detail)
			}
			return
		}
	}
	t.Error("expected 'id' column suggestion with type detail")
}
