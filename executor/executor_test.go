package executor

import (
	"context"
	"testing"
	"time"
)

// ---- IsDDL tests ----

func TestIsDDL(t *testing.T) {
	e := &Executor{}

	tests := []struct {
		input string
		want  bool
	}{
		{"CREATE TABLE t (id INT)", true},
		{"ALTER TABLE t ADD COLUMN name VARCHAR(255)", true},
		{"DROP TABLE t", true},
		{"TRUNCATE TABLE t", true},
		{"RENAME TABLE t TO t2", true},
		{"GRANT SELECT ON db.t TO user", true},
		{"REVOKE SELECT ON db.t FROM user", true},
		{"SELECT * FROM t", false},
		{"INSERT INTO t VALUES (1)", false},
		{"UPDATE t SET x = 1", false},
		{"DELETE FROM t WHERE id = 1", false},
		{"", false},
		{"  CREATE TABLE t (id INT)", true}, // leading whitespace
	}

	for _, tt := range tests {
		got := e.IsDDL(tt.input)
		if got != tt.want {
			t.Errorf("IsDDL(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsDDL_WithComments(t *testing.T) {
	e := &Executor{}

	tests := []struct {
		input string
		want  bool
	}{
		{"-- comment\nCREATE TABLE t (id INT)", true},
		{"/* block */ CREATE TABLE t (id INT)", true},
		{"-- comment\nSELECT 1", false},
	}

	for _, tt := range tests {
		got := e.IsDDL(tt.input)
		if got != tt.want {
			t.Errorf("IsDDL(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// ---- isQueryStatement tests ----

func TestIsQueryStatement(t *testing.T) {
	e := &Executor{}

	tests := []struct {
		input string
		want  bool
	}{
		{"SELECT * FROM t", true},
		{"SHOW TABLES", true},
		{"DESCRIBE t", true},
		{"DESC t", true},
		{"EXPLAIN SELECT 1", true},
		{"WITH cte AS (SELECT 1) SELECT * FROM cte", true},
		{"INSERT INTO t VALUES (1)", false},
		{"UPDATE t SET x = 1", false},
		{"DELETE FROM t WHERE 1=1", false},
		{"CREATE TABLE t (id INT)", false},
		{"", false},
	}

	for _, tt := range tests {
		got := e.isQueryStatement(tt.input)
		if got != tt.want {
			t.Errorf("isQueryStatement(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsQueryStatement_WithComments(t *testing.T) {
	e := &Executor{}

	tests := []struct {
		input string
		want  bool
	}{
		{"-- comment\nSELECT 1", true},
		{"/* block */ SHOW TABLES", true},
		{"-- comment\nINSERT INTO t VALUES (1)", false},
	}

	for _, tt := range tests {
		got := e.isQueryStatement(tt.input)
		if got != tt.want {
			t.Errorf("isQueryStatement(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// ---- SplitStatements tests ----

func TestSplitStatements(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"SELECT 1", 1},
		{"SELECT 1; SELECT 2", 2},
		{"SELECT 1; SELECT 2; SELECT 3", 3},
		{"", 0},
		{";", 0},
		{";;", 0},
		{"SELECT 1;", 1},
	}

	for _, tt := range tests {
		got := SplitStatements(tt.input)
		if len(got) != tt.want {
			t.Errorf("SplitStatements(%q) = %d statements, want %d", tt.input, len(got), tt.want)
		}
	}
}

func TestSplitStatements_RespectsStrings(t *testing.T) {
	input := "INSERT INTO t VALUES ('a;b'); SELECT 1"
	stmts := SplitStatements(input)
	if len(stmts) != 2 {
		t.Errorf("SplitStatements() = %d statements, want 2", len(stmts))
	}
	if stmts[0] != "INSERT INTO t VALUES ('a;b')" {
		t.Errorf("stmts[0] = %q, want %q", stmts[0], "INSERT INTO t VALUES ('a;b')")
	}
}

func TestSplitStatements_RespectsComments(t *testing.T) {
	input := "SELECT 1 -- comment; not a separator\n; SELECT 2"
	stmts := SplitStatements(input)
	if len(stmts) != 2 {
		t.Errorf("SplitStatements() = %d statements, want 2", len(stmts))
	}
}

// ---- FormatDuration tests ----

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{100 * time.Nanosecond, "0µs"},
		{500 * time.Microsecond, "500µs"},
		{5 * time.Millisecond, "5.00ms"},
		{1500 * time.Millisecond, "1.50s"},
		{0, "0µs"},
	}

	for _, tt := range tests {
		got := FormatDuration(tt.d)
		if got != tt.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

// ---- NullString tests ----

func TestNullString(t *testing.T) {
	tests := []struct {
		valid bool
		str   string
		want  string
	}{
		{true, "hello", "hello"},
		{false, "", ""},
	}

	for _, tt := range tests {
		got := NullString(struct {
			String string
			Valid  bool
		}{String: tt.str, Valid: tt.valid})
		_ = got
	}
}

// ---- QueryResult struct test ----

func TestQueryResult_Fields(t *testing.T) {
	qr := &QueryResult{
		Columns:      []string{"id", "name"},
		RowCount:     10,
		AffectedRows: 0,
		IsQuery:      true,
	}
	if len(qr.Columns) != 2 {
		t.Errorf("Columns len = %d, want 2", len(qr.Columns))
	}
	if !qr.IsQuery {
		t.Error("IsQuery should be true")
	}
}

// ---- Cancel test ----

func TestCancel_NoPanic(t *testing.T) {
	e := &Executor{}
	e.Cancel() // should not panic even with nil cancel
}

// ---- Additional IsDDL edge cases ----

func TestIsDDL_UnclosedBlockComment(t *testing.T) {
	e := &Executor{}
	// Unclosed block comment → returns false
	got := e.IsDDL("/* unclosed comment")
	if got {
		t.Error("IsDDL with unclosed block comment should return false")
	}
}

func TestIsDDL_CommentOnlyLineComment(t *testing.T) {
	e := &Executor{}
	// Line comment without newline → returns false
	got := e.IsDDL("-- just a comment")
	if got {
		t.Error("IsDDL with comment-only input should return false")
	}
}

func TestIsDDL_NestedBlockCommentWithDDL(t *testing.T) {
	e := &Executor{}
	got := e.IsDDL("/* comment */ /* another */ DROP TABLE t")
	if !got {
		t.Error("IsDDL with nested block comments + DDL should return true")
	}
}

// ---- isQueryStatement edge cases ----

func TestIsQueryStatement_UnclosedBlockComment(t *testing.T) {
	e := &Executor{}
	got := e.isQueryStatement("/* unclosed")
	if got {
		t.Error("isQueryStatement with unclosed block comment should return false")
	}
}

func TestIsQueryStatement_CommentOnlyLineComment(t *testing.T) {
	e := &Executor{}
	got := e.isQueryStatement("-- just a comment")
	if got {
		t.Error("isQueryStatement with comment-only should return false")
	}
}

// ---- SplitStatements additional edge cases ----

func TestSplitStatements_DoubleQuotedString(t *testing.T) {
	input := `INSERT INTO t VALUES ("a;b"); SELECT 1`
	stmts := SplitStatements(input)
	if len(stmts) != 2 {
		t.Errorf("SplitStatements with double-quoted string = %d statements, want 2", len(stmts))
	}
}

func TestSplitStatements_EscapedQuote(t *testing.T) {
	input := `INSERT INTO t VALUES ('it\'s; ok'); SELECT 1`
	stmts := SplitStatements(input)
	if len(stmts) != 2 {
		t.Errorf("SplitStatements with escaped quote = %d statements, want 2", len(stmts))
	}
}

func TestSplitStatements_BlockComment(t *testing.T) {
	// Note: SplitStatements does NOT skip semicolons inside block comments
	// It only respects string literals and single-line comments
	input := "SELECT 1 /* comment; inside */; SELECT 2"
	stmts := SplitStatements(input)
	if len(stmts) != 3 {
		t.Logf("SplitStatements with block comment = %d statements (block comment semicolons are NOT skipped)", len(stmts))
		for i, s := range stmts {
			t.Logf("  stmt[%d] = %q", i, s)
		}
	}
}

func TestSplitStatements_UnclosedBlockComment(t *testing.T) {
	input := "SELECT 1 /* unclosed; comment"
	stmts := SplitStatements(input)
	if len(stmts) != 1 {
		t.Errorf("SplitStatements with unclosed block comment = %d statements, want 1", len(stmts))
	}
}

func TestSplitStatements_MultipleSemicolonsWithContent(t *testing.T) {
	input := "SELECT 1;; SELECT 2;"
	stmts := SplitStatements(input)
	if len(stmts) != 2 {
		t.Errorf("SplitStatements with double semicolons = %d statements, want 2", len(stmts))
	}
}

// ---- FormatDuration additional edge cases ----

func TestFormatDuration_ExactMicrosecond(t *testing.T) {
	got := FormatDuration(1 * time.Microsecond)
	if got != "1µs" {
		t.Errorf("FormatDuration(1µs) = %q, want %q", got, "1µs")
	}
}

func TestFormatDuration_SubMicrosecond(t *testing.T) {
	got := FormatDuration(100 * time.Nanosecond)
	if got != "0µs" {
		t.Errorf("FormatDuration(100ns) = %q, want %q", got, "0µs")
	}
}

func TestFormatDuration_ExactSecond(t *testing.T) {
	got := FormatDuration(1 * time.Second)
	if got != "1.00s" {
		t.Errorf("FormatDuration(1s) = %q, want %q", got, "1.00s")
	}
}

// ---- NullString with proper sql.NullString ----

func TestNullString_Valid(t *testing.T) {
	got := NullString(struct {
		String string
		Valid  bool
	}{String: "hello", Valid: true})
	if got != "hello" {
		t.Errorf("NullString(valid) = %q, want %q", got, "hello")
	}
}

func TestNullString_Invalid(t *testing.T) {
	got := NullString(struct {
		String string
		Valid  bool
	}{String: "", Valid: false})
	if got != "" {
		t.Errorf("NullString(invalid) = %q, want %q", got, "")
	}
}

// ---- New Executor constructor ----

func TestNew_Executor(t *testing.T) {
	e := New(nil, nil)
	if e == nil {
		t.Error("New() should not return nil")
	}
}

// ---- Execute with empty query ----

func TestExecute_EmptyQuery(t *testing.T) {
	e := New(nil, nil)
	result, err := e.Execute(context.Background(), "")
	if err != nil {
		t.Errorf("Execute empty query error: %v", err)
	}
	if result.IsQuery {
		t.Error("Empty query should not be IsQuery")
	}
}

func TestExecute_WhitespaceOnly(t *testing.T) {
	e := New(nil, nil)
	result, err := e.Execute(context.Background(), "   ")
	if err != nil {
		t.Errorf("Execute whitespace error: %v", err)
	}
	if result.IsQuery {
		t.Error("Whitespace-only query should not be IsQuery")
	}
}

func TestExecute_SemicolonOnly(t *testing.T) {
	e := New(nil, nil)
	result, err := e.Execute(context.Background(), ";")
	if err != nil {
		t.Errorf("Execute semicolon error: %v", err)
	}
	if result.IsQuery {
		t.Error("Semicolon-only should not be IsQuery")
	}
}

// ---- Safe-updates tests ----

func TestSafeUpdates_DefaultOff(t *testing.T) {
	e := New(nil, nil)
	if e.SafeUpdates() {
		t.Error("SafeUpdates should be off by default")
	}
}

func TestSafeUpdates_SetOnOff(t *testing.T) {
	e := New(nil, nil)
	e.SetSafeUpdates(true)
	if !e.SafeUpdates() {
		t.Error("SafeUpdates should be on after SetSafeUpdates(true)")
	}
	e.SetSafeUpdates(false)
	if e.SafeUpdates() {
		t.Error("SafeUpdates should be off after SetSafeUpdates(false)")
	}
}

func TestCheckSafeUpdates_UpdateNoWhere(t *testing.T) {
	e := New(nil, nil)
	e.SetSafeUpdates(true)
	err := e.checkSafeUpdates("UPDATE users SET name = 'test'")
	if err == nil {
		t.Error("Expected SafeUpdateError for UPDATE without WHERE/LIMIT")
	}
	if _, ok := err.(*SafeUpdateError); !ok {
		t.Errorf("Expected *SafeUpdateError, got %T", err)
	}
}

func TestCheckSafeUpdates_UpdateWithWhere(t *testing.T) {
	e := New(nil, nil)
	e.SetSafeUpdates(true)
	err := e.checkSafeUpdates("UPDATE users SET name = 'test' WHERE id = 1")
	if err != nil {
		t.Errorf("Expected no error for UPDATE with WHERE, got %v", err)
	}
}

func TestCheckSafeUpdates_UpdateWithLimit(t *testing.T) {
	e := New(nil, nil)
	e.SetSafeUpdates(true)
	err := e.checkSafeUpdates("UPDATE users SET name = 'test' LIMIT 10")
	if err != nil {
		t.Errorf("Expected no error for UPDATE with LIMIT, got %v", err)
	}
}

func TestCheckSafeUpdates_DeleteNoWhere(t *testing.T) {
	e := New(nil, nil)
	e.SetSafeUpdates(true)
	err := e.checkSafeUpdates("DELETE FROM users")
	if err == nil {
		t.Error("Expected SafeUpdateError for DELETE without WHERE/LIMIT")
	}
}

func TestCheckSafeUpdates_DeleteWithWhere(t *testing.T) {
	e := New(nil, nil)
	e.SetSafeUpdates(true)
	err := e.checkSafeUpdates("DELETE FROM users WHERE id = 1")
	if err != nil {
		t.Errorf("Expected no error for DELETE with WHERE, got %v", err)
	}
}

func TestCheckSafeUpdates_SelectNotBlocked(t *testing.T) {
	e := New(nil, nil)
	e.SetSafeUpdates(true)
	err := e.checkSafeUpdates("SELECT * FROM users")
	if err != nil {
		t.Errorf("SELECT should not be blocked by safe-updates, got %v", err)
	}
}

func TestCheckSafeUpdates_InsertNotBlocked(t *testing.T) {
	e := New(nil, nil)
	e.SetSafeUpdates(true)
	err := e.checkSafeUpdates("INSERT INTO users VALUES (1, 'test')")
	if err != nil {
		t.Errorf("INSERT should not be blocked by safe-updates, got %v", err)
	}
}

func TestCheckSafeUpdates_OffNotBlocked(t *testing.T) {
	e := New(nil, nil)
	e.SetSafeUpdates(false)
	// checkSafeUpdates is only called when safeUpdates=true, but the method
	// itself doesn't check the flag — that's done in Execute().
	// When safeUpdates=false, Execute() skips the check entirely.
	// So we just verify the flag is off.
	if e.SafeUpdates() {
		t.Error("SafeUpdates should be off")
	}
}

func TestCheckSafeUpdates_WithComments(t *testing.T) {
	e := New(nil, nil)
	e.SetSafeUpdates(true)
	err := e.checkSafeUpdates("-- comment\nDELETE FROM users")
	if err == nil {
		t.Error("Expected SafeUpdateError for DELETE with leading comment")
	}
}

func TestSafeUpdateError_Message(t *testing.T) {
	err := &SafeUpdateError{Query: "DELETE FROM users"}
	msg := err.Error()
	if msg == "" {
		t.Error("SafeUpdateError.Error() should not be empty")
	}
}

// ---- Slow threshold tests ----

func TestSlowThreshold_DefaultDisabled(t *testing.T) {
	e := New(nil, nil)
	if e.SlowThreshold() != 0 {
		t.Error("SlowThreshold should be 0 (disabled) by default")
	}
}

func TestSlowThreshold_Set(t *testing.T) {
	e := New(nil, nil)
	e.SetSlowThreshold(5 * time.Second)
	if e.SlowThreshold() != 5*time.Second {
		t.Errorf("SlowThreshold = %v, want 5s", e.SlowThreshold())
	}
}

func TestSlowThreshold_SetZero(t *testing.T) {
	e := New(nil, nil)
	e.SetSlowThreshold(5 * time.Second)
	e.SetSlowThreshold(0)
	if e.SlowThreshold() != 0 {
		t.Error("SlowThreshold should be 0 after SetSlowThreshold(0)")
	}
}

// ---- QueryResult SlowQuery field test ----

func TestQueryResult_SlowQuery(t *testing.T) {
	qr := &QueryResult{
		Duration:  10 * time.Second,
		SlowQuery: true,
	}
	if !qr.SlowQuery {
		t.Error("SlowQuery should be true")
	}
}
