package output

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eyjian/mysh/executor"
)

func TestParseFormat(t *testing.T) {
	tests := []struct {
		input string
		want  Format
	}{
		{"table", FormatTable},
		{"vertical", FormatVertical},
		{"v", FormatVertical},
		{"json", FormatJSON},
		{"j", FormatJSON},
		{"", FormatTable},
		{"unknown", FormatTable},
	}

	for _, tt := range tests {
		got, err := ParseFormat(tt.input)
		if tt.input == "unknown" {
			if err == nil {
				t.Errorf("ParseFormat(%q) expected error, got nil", tt.input)
			}
		} else {
			if err != nil {
				t.Errorf("ParseFormat(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseFormat(%q) = %v, want %v", tt.input, got, tt.want)
			}
		}
	}
}

func TestFormatString(t *testing.T) {
	tests := []struct {
		f    Format
		want string
	}{
		{FormatTable, "table"},
		{FormatVertical, "vertical"},
		{FormatJSON, "json"},
	}

	for _, tt := range tests {
		got := tt.f.String()
		if got != tt.want {
			t.Errorf("Format(%d).String() = %q, want %q", tt.f, got, tt.want)
		}
	}
}

func TestWriteResult_Table(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatTable, &buf)

	result := &executor.QueryResult{
		Columns:  []string{"id", "name"},
		Rows:     [][]any{{int64(1), "Alice"}, {int64(2), "Bob"}},
		RowCount: 2,
		IsQuery:  true,
		Duration: 10 * time.Millisecond,
	}

	err := f.WriteResult(result)
	if err != nil {
		t.Fatalf("WriteResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "id") || !strings.Contains(output, "name") {
		t.Errorf("Table output missing column headers: %q", output)
	}
	if !strings.Contains(output, "Alice") || !strings.Contains(output, "Bob") {
		t.Errorf("Table output missing row data: %q", output)
	}
	if !strings.Contains(output, "2 rows in set") {
		t.Errorf("Table output missing row count: %q", output)
	}
}

func TestWriteResult_Vertical(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatVertical, &buf)

	result := &executor.QueryResult{
		Columns:  []string{"id", "name"},
		Rows:     [][]any{{int64(1), "Alice"}},
		RowCount: 1,
		IsQuery:  true,
		Duration: 5 * time.Millisecond,
	}

	err := f.WriteResult(result)
	if err != nil {
		t.Fatalf("WriteResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "1. row") {
		t.Errorf("Vertical output missing row marker: %q", output)
	}
	if !strings.Contains(output, "id") || !strings.Contains(output, "name") {
		t.Errorf("Vertical output missing column labels: %q", output)
	}
}

func TestWriteResult_JSON(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatJSON, &buf)

	result := &executor.QueryResult{
		Columns:  []string{"id", "name"},
		Rows:     [][]any{{int64(1), "Alice"}},
		RowCount: 1,
		IsQuery:  true,
		Duration: 3 * time.Millisecond,
	}

	err := f.WriteResult(result)
	if err != nil {
		t.Fatalf("WriteResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"id"`) || !strings.Contains(output, `"name"`) {
		t.Errorf("JSON output missing keys: %q", output)
	}
	if !strings.Contains(output, "Alice") {
		t.Errorf("JSON output missing data: %q", output)
	}
}

func TestWriteResult_DML(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatTable, &buf)

	result := &executor.QueryResult{
		AffectedRows: 3,
		IsQuery:      false,
		Duration:     1 * time.Millisecond,
	}

	err := f.WriteResult(result)
	if err != nil {
		t.Fatalf("WriteResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "3 rows affected") {
		t.Errorf("DML output missing affected rows: %q", output)
	}
}

func TestWriteResult_Error(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatTable, &buf)

	result := &executor.QueryResult{
		Error:    fmt.Errorf("syntax error"),
		IsQuery:  false,
		Duration: time.Millisecond,
	}

	err := f.WriteResult(result)
	if err != nil {
		t.Fatalf("WriteResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "ERROR:") {
		t.Errorf("Error output missing ERROR prefix: %q", output)
	}
}

func TestWriteResult_Empty(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatTable, &buf)

	result := &executor.QueryResult{
		Columns:  []string{"id"},
		Rows:     [][]any{},
		RowCount: 0,
		IsQuery:  true,
		Duration: time.Millisecond,
	}

	err := f.WriteResult(result)
	if err != nil {
		t.Fatalf("WriteResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "0 rows in set") {
		t.Errorf("Empty result output missing count: %q", output)
	}
}

func TestSetFormat(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatTable, &buf)

	if f.CurrentFormat() != FormatTable {
		t.Errorf("initial format should be table")
	}

	f.SetFormat(FormatJSON)
	if f.CurrentFormat() != FormatJSON {
		t.Errorf("format should be JSON after SetFormat")
	}
}

// ---- Additional output tests ----

func TestWriteResult_NilResult(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatTable, &buf)
	err := f.WriteResult(nil)
	if err != nil {
		t.Errorf("WriteResult(nil) error: %v", err)
	}
	if buf.String() != "" {
		t.Errorf("WriteResult(nil) should produce no output, got %q", buf.String())
	}
}

func TestWriteResult_TableWithNULL(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatTable, &buf)

	result := &executor.QueryResult{
		Columns:  []string{"id", "name"},
		Rows:     [][]any{{int64(1), nil}},
		RowCount: 1,
		IsQuery:  true,
		Duration: time.Millisecond,
	}

	err := f.WriteResult(result)
	if err != nil {
		t.Fatalf("WriteResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "NULL") {
		t.Errorf("Table output should show NULL for nil values: %q", output)
	}
	if !strings.Contains(output, "1 row in set") {
		t.Errorf("Table output missing row count: %q", output)
	}
}

func TestWriteResult_TableWithChineseChars(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatTable, &buf)

	result := &executor.QueryResult{
		Columns:  []string{"名字", "年龄"},
		Rows:     [][]any{{"张三", int64(25)}},
		RowCount: 1,
		IsQuery:  true,
		Duration: time.Millisecond,
	}

	err := f.WriteResult(result)
	if err != nil {
		t.Fatalf("WriteResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "张三") {
		t.Errorf("Table output missing Chinese data: %q", output)
	}
}

func TestWriteResult_TableWithByteSlice(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatTable, &buf)

	result := &executor.QueryResult{
		Columns:  []string{"data"},
		Rows:     [][]any{{[]byte("hello")}},
		RowCount: 1,
		IsQuery:  true,
		Duration: time.Millisecond,
	}

	err := f.WriteResult(result)
	if err != nil {
		t.Fatalf("WriteResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "hello") {
		t.Errorf("Table output missing byte data: %q", output)
	}
}

func TestWriteResult_DMLNoAffectedRows(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatTable, &buf)

	result := &executor.QueryResult{
		AffectedRows: -1,
		IsQuery:      false,
		Duration:     time.Millisecond,
	}

	err := f.WriteResult(result)
	if err != nil {
		t.Fatalf("WriteResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Query OK") {
		t.Errorf("DML output should contain Query OK: %q", output)
	}
	if strings.Contains(output, "rows affected") {
		t.Errorf("DML output with -1 affected should not show rows affected: %q", output)
	}
}

func TestWriteResult_EmptyColumns(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatTable, &buf)

	result := &executor.QueryResult{
		Columns:  []string{},
		Rows:     [][]any{},
		IsQuery:  true,
		Duration: time.Millisecond,
	}

	err := f.WriteResult(result)
	if err != nil {
		t.Fatalf("WriteResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Empty set") {
		t.Errorf("Empty columns should show Empty set: %q", output)
	}
}

func TestWriteResult_VerticalWithNULL(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatVertical, &buf)

	result := &executor.QueryResult{
		Columns:  []string{"id", "name"},
		Rows:     [][]any{{nil, "Alice"}},
		RowCount: 1,
		IsQuery:  true,
		Duration: time.Millisecond,
	}

	err := f.WriteResult(result)
	if err != nil {
		t.Fatalf("WriteResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "NULL") {
		t.Errorf("Vertical output should show NULL: %q", output)
	}
}

func TestWriteResult_VerticalMultipleRows(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatVertical, &buf)

	result := &executor.QueryResult{
		Columns:  []string{"id"},
		Rows:     [][]any{{int64(1)}, {int64(2)}},
		RowCount: 2,
		IsQuery:  true,
		Duration: time.Millisecond,
	}

	err := f.WriteResult(result)
	if err != nil {
		t.Fatalf("WriteResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "1. row") || !strings.Contains(output, "2. row") {
		t.Errorf("Vertical output missing row markers: %q", output)
	}
	if !strings.Contains(output, "2 rows in set") {
		t.Errorf("Vertical output missing row count: %q", output)
	}
}

func TestWriteResult_JSONWithNULL(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatJSON, &buf)

	result := &executor.QueryResult{
		Columns:  []string{"id", "name"},
		Rows:     [][]any{{int64(1), nil}},
		RowCount: 1,
		IsQuery:  true,
		Duration: time.Millisecond,
	}

	err := f.WriteResult(result)
	if err != nil {
		t.Fatalf("WriteResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "null") {
		t.Errorf("JSON output should contain null for nil values: %q", output)
	}
}

func TestWriteResult_JSONWithByteSlice(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatJSON, &buf)

	result := &executor.QueryResult{
		Columns:  []string{"data"},
		Rows:     [][]any{{[]byte("hello")}},
		RowCount: 1,
		IsQuery:  true,
		Duration: time.Millisecond,
	}

	err := f.WriteResult(result)
	if err != nil {
		t.Fatalf("WriteResult error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "hello") {
		t.Errorf("JSON output missing byte data: %q", output)
	}
}

func TestWriteResult_EmptyResultZeroRows(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatTable, &buf)

	result := &executor.QueryResult{
		Columns:  []string{"id"},
		Rows:     [][]any{},
		RowCount: 0,
		IsQuery:  true,
		Duration: time.Millisecond,
	}

	err := f.WriteResult(result)
	if err != nil {
		t.Fatalf("WriteResult error: %v", err)
	}

	output := buf.String()
	// Should have separator + header + separator + "0 rows in set"
	if !strings.Contains(output, "0 rows in set") {
		t.Errorf("Empty result should show 0 rows: %q", output)
	}
}

func TestFormatDefault(t *testing.T) {
	// Test Format(99) which hits the default case in String()
	f := Format(99)
	if f.String() != "table" {
		t.Errorf("Format(99).String() = %q, want %q", f.String(), "table")
	}
}

// ---- Smart column width tests ----

func TestAdjustColumnWidths_NoLimit(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatTable, &buf)
	// No max width set — should not truncate
	widths := []int{10, 20, 30}
	result := f.adjustColumnWidths(widths, 3)
	if result[0] != 10 || result[1] != 20 || result[2] != 30 {
		t.Errorf("Expected widths unchanged, got %v", result)
	}
}

func TestAdjustColumnWidths_FitsExactly(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatTable, &buf)
	// Total: 1 + (10+3) + (20+3) = 37
	f.SetMaxWidth(37)
	widths := []int{10, 20}
	result := f.adjustColumnWidths(widths, 2)
	if result[0] != 10 || result[1] != 20 {
		t.Errorf("Expected widths unchanged when fits, got %v", result)
	}
}

func TestAdjustColumnWidths_NeedsTruncation(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatTable, &buf)
	// Total: 1 + (100+3) + (10+3) = 117, but maxWidth is 30
	f.SetMaxWidth(30)
	widths := []int{100, 10}
	result := f.adjustColumnWidths(widths, 2)
	// Widest column should be truncated
	total := 1
	for _, w := range result {
		total += w + 3
	}
	if total > 30 {
		t.Errorf("Expected total width <= 30, got %d (widths=%v)", total, result)
	}
}

func TestAdjustColumnWidths_MinimumWidth(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatTable, &buf)
	// Very narrow terminal
	f.SetMaxWidth(15)
	widths := []int{50, 40, 30}
	result := f.adjustColumnWidths(widths, 3)
	// Each column should be at least minWidth (4)
	for _, w := range result {
		if w < 4 {
			t.Errorf("Column width should be >= 4, got %d", w)
		}
	}
}

func TestTruncateRunes(t *testing.T) {
	tests := []struct {
		input    string
		maxRunes int
		want     string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 4, "hel…"},
		{"hello", 1, "…"},
		{"你好世界", 3, "你好…"},
	}
	for _, tt := range tests {
		got := truncateRunes(tt.input, tt.maxRunes)
		if got != tt.want {
			t.Errorf("truncateRunes(%q, %d) = %q, want %q", tt.input, tt.maxRunes, got, tt.want)
		}
	}
}

func TestSetMaxWidth(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatTable, &buf)
	if f.MaxWidth() != 0 {
		t.Errorf("Default maxWidth should be 0")
	}
	f.SetMaxWidth(80)
	if f.MaxWidth() != 80 {
		t.Errorf("MaxWidth should be 80 after SetMaxWidth")
	}
}

func TestWriteResult_TableWithMaxWidth(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter(FormatTable, &buf)
	f.SetMaxWidth(30)

	result := &executor.QueryResult{
		Columns:  []string{"id", "very_long_column_name"},
		Rows:     [][]any{{int64(1), "a very long value that should be truncated"}},
		RowCount: 1,
		IsQuery:  true,
		Duration: time.Millisecond,
	}

	err := f.WriteResult(result)
	if err != nil {
		t.Fatalf("WriteResult error: %v", err)
	}

	output := buf.String()
	// Should contain id column
	if !strings.Contains(output, "id") {
		t.Errorf("Output should contain 'id': %q", output)
	}
	// Lines should not exceed maxWidth significantly
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		stripped := stripANSI(line)
		if len([]rune(stripped)) > 35 { // allow some margin
			t.Errorf("Line too long (%d chars): %q", len([]rune(stripped)), stripped)
		}
	}
}

func TestCalcTableWidthWithMax(t *testing.T) {
	result := &executor.QueryResult{
		Columns: []string{"id", "name"},
		Rows:    [][]any{{int64(1), "Alice"}},
		IsQuery: true,
	}

	// Without max
	w1 := CalcTableWidth(result)
	if w1 <= 0 {
		t.Errorf("CalcTableWidth should be > 0, got %d", w1)
	}

	// With max that forces truncation
	w2 := CalcTableWidthWithMax(result, 20)
	if w2 > 20 {
		t.Errorf("CalcTableWidthWithMax should be <= 20, got %d", w2)
	}
}
