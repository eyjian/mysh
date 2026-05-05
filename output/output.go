package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"mysh/executor"
)

// Format represents the output format type.
type Format int

const (
	FormatTable Format = iota
	FormatVertical
	FormatJSON
	FormatMarkdown
)

// ParseFormat parses a format string and returns the Format enum.
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(s) {
	case "table", "":
		return FormatTable, nil
	case "vertical", "v":
		return FormatVertical, nil
	case "json", "j":
		return FormatJSON, nil
	case "markdown", "md":
		return FormatMarkdown, nil
	default:
		return FormatTable, fmt.Errorf("unknown output format: %s", s)
	}
}

// String returns the string representation of a Format.
func (f Format) String() string {
	switch f {
	case FormatTable:
		return "table"
	case FormatVertical:
		return "vertical"
	case FormatJSON:
		return "json"
	case FormatMarkdown:
		return "markdown"
	default:
		return "table"
	}
}

// Formatter formats and writes query results.
type Formatter struct {
	format Format
	writer io.Writer
}

// NewFormatter creates a new Formatter with the given format and writer.
func NewFormatter(format Format, writer io.Writer) *Formatter {
	return &Formatter{
		format: format,
		writer: writer,
	}
}

// SetFormat changes the output format.
func (f *Formatter) SetFormat(format Format) {
	f.format = format
}

// CurrentFormat returns the current output format.
func (f *Formatter) CurrentFormat() Format {
	return f.format
}

// WriteResult writes a QueryResult to the underlying writer using the current format.
func (f *Formatter) WriteResult(result *executor.QueryResult) error {
	if result == nil {
		return nil
	}

	// Handle errors
	if result.Error != nil {
		_, err := fmt.Fprintf(f.writer, "%sERROR:%s %s\n",
			ansiRed+ansiBold, ansiReset, result.Error.Error())
		return err
	}

	// Display warning if present (e.g., reconnection notice)
	if result.Warning != "" {
		fmt.Fprintf(f.writer, "%sWarning: %s%s\n", ansiYellow, result.Warning, ansiReset)
	}

	// Handle DML/DDL results
	if !result.IsQuery {
		msg := formatDMLResult(result)
		_, err := fmt.Fprintln(f.writer, msg)
		return err
	}

	// Handle empty result sets
	if len(result.Columns) == 0 {
		_, err := fmt.Fprintln(f.writer, "Empty set")
		return err
	}

	switch f.format {
	case FormatTable:
		return f.writeTable(result)
	case FormatVertical:
		return f.writeVertical(result)
	case FormatJSON:
		return f.writeJSON(result)
	case FormatMarkdown:
		return f.writeMarkdown(result)
	default:
		return f.writeTable(result)
	}
}

// writeTable formats results as an aligned table.
func (f *Formatter) writeTable(result *executor.QueryResult) error {
	if len(result.Columns) == 0 {
		return nil
	}

	// Convert rows to plain strings for width calculation
	plainRows := make([][]string, len(result.Rows))
	for i, row := range result.Rows {
		plainRows[i] = make([]string, len(row))
		for j, val := range row {
			plainRows[i][j] = formatValue(val)
		}
	}

	// Convert rows to styled strings for display
	styledRows := make([][]string, len(result.Rows))
	for i, row := range result.Rows {
		styledRows[i] = make([]string, len(row))
		for j, val := range row {
			styledRows[i][j] = formatValueStyled(val)
		}
	}

	// Calculate column widths (using plain text, no ANSI codes)
	widths := make([]int, len(result.Columns))
	for i, col := range result.Columns {
		widths[i] = utf8.RuneCountInString(col)
	}
	for _, row := range plainRows {
		for i, val := range row {
			if i < len(widths) {
				w := utf8.RuneCountInString(val)
				if w > widths[i] {
					widths[i] = w
				}
			}
		}
	}

	// Build separator line
	separator := buildSeparator(widths)

	// Print header
	fmt.Fprintln(f.writer, separator)
	var header strings.Builder
	header.WriteString("|")
	for i, col := range result.Columns {
		header.WriteString(" ")
		header.WriteString(padRight(col, widths[i]))
		header.WriteString(" |")
	}
	fmt.Fprintln(f.writer, header.String())
	fmt.Fprintln(f.writer, separator)

	// Print rows (using styled values)
	for _, row := range styledRows {
		var line strings.Builder
		line.WriteString("|")
		for i, val := range row {
			if i < len(widths) {
				line.WriteString(" ")
				line.WriteString(padRightStyled(val, widths[i]))
				line.WriteString(" |")
			}
		}
		fmt.Fprintln(f.writer, line.String())
	}

	if len(styledRows) > 0 {
		fmt.Fprintln(f.writer, separator)
	}

	// Row count and timing
	fmt.Fprintf(f.writer, "%s%d %s in set%s %s(%s)%s\n",
		ansiGreen, len(result.Rows), pluralRow(len(result.Rows)), ansiReset,
		ansiCyan, executor.FormatDuration(result.Duration), ansiReset)

	return nil
}

// writeVertical formats results in vertical layout (one line per column).
func (f *Formatter) writeVertical(result *executor.QueryResult) error {
	if len(result.Columns) == 0 {
		return nil
	}

	// Calculate label width (column name width)
	labelWidth := 0
	for _, col := range result.Columns {
		w := utf8.RuneCountInString(col)
		if w > labelWidth {
			labelWidth = w
		}
	}

	for rowIdx, row := range result.Rows {
		if rowIdx > 0 {
			fmt.Fprintln(f.writer)
		}
		fmt.Fprintf(f.writer, "*************************** %d. row ***************************\n", rowIdx+1)
		for i, col := range result.Columns {
			var val string
			if i < len(row) {
				val = formatValueStyled(row[i])
			} else {
				val = formatValueStyled(nil)
			}
			fmt.Fprintf(f.writer, "%s: %s\n", padRight(col, labelWidth), val)
		}
	}

	fmt.Fprintf(f.writer, "%s%d %s in set%s %s(%s)%s\n",
		ansiGreen, len(result.Rows), pluralRow(len(result.Rows)), ansiReset,
		ansiCyan, executor.FormatDuration(result.Duration), ansiReset)

	return nil
}

// writeJSON formats results as a JSON array.
func (f *Formatter) writeJSON(result *executor.QueryResult) error {
	if len(result.Columns) == 0 {
		return nil
	}

	// Build slice of maps
	records := make([]map[string]interface{}, 0, len(result.Rows))
	for _, row := range result.Rows {
		record := make(map[string]interface{}, len(result.Columns))
		for i, col := range result.Columns {
			if i < len(row) {
				record[col] = convertForJSON(row[i])
			} else {
				record[col] = nil
			}
		}
		records = append(records, record)
	}

	encoder := json.NewEncoder(f.writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(records); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}

	count := len(result.Rows)
	fmt.Fprintf(f.writer, "%s%d %s in set%s %s(%s)%s\n",
		ansiGreen, count, pluralRow(count), ansiReset,
		ansiCyan, executor.FormatDuration(result.Duration), ansiReset)

	return nil
}

// writeMarkdown formats results as a Markdown table.
func (f *Formatter) writeMarkdown(result *executor.QueryResult) error {
	if len(result.Columns) == 0 {
		return nil
	}

	// Convert rows to plain strings for width calculation
	plainRows := make([][]string, len(result.Rows))
	for i, row := range result.Rows {
		plainRows[i] = make([]string, len(row))
		for j, val := range row {
			plainRows[i][j] = formatValue(val)
		}
	}

	// Convert rows to styled strings for display
	styledRows := make([][]string, len(result.Rows))
	for i, row := range result.Rows {
		styledRows[i] = make([]string, len(row))
		for j, val := range row {
			styledRows[i][j] = formatValueStyled(val)
		}
	}

	// Calculate column widths (using plain text)
	widths := make([]int, len(result.Columns))
	for i, col := range result.Columns {
		widths[i] = utf8.RuneCountInString(col)
	}
	for _, row := range plainRows {
		for i, val := range row {
			if i < len(widths) {
				w := utf8.RuneCountInString(val)
				if w > widths[i] {
					widths[i] = w
				}
			}
		}
	}

	// Print header row
	var header strings.Builder
	header.WriteString("|")
	for i, col := range result.Columns {
		header.WriteString(" ")
		header.WriteString(padRight(col, widths[i]))
		header.WriteString(" |")
	}
	fmt.Fprintln(f.writer, header.String())

	// Print separator row
	var sep strings.Builder
	sep.WriteString("|")
	for _, w := range widths {
		sep.WriteString(" ")
		sep.WriteString(strings.Repeat("-", w))
		sep.WriteString(" |")
	}
	fmt.Fprintln(f.writer, sep.String())

	// Print data rows (using styled values)
	for _, row := range styledRows {
		var line strings.Builder
		line.WriteString("|")
		for i, val := range row {
			if i < len(widths) {
				line.WriteString(" ")
				line.WriteString(padRightStyled(val, widths[i]))
				line.WriteString(" |")
			}
		}
		fmt.Fprintln(f.writer, line.String())
	}

	// Row count and timing
	fmt.Fprintf(f.writer, "%s%d %s in set%s %s(%s)%s\n",
		ansiGreen, len(result.Rows), pluralRow(len(result.Rows)), ansiReset,
		ansiCyan, executor.FormatDuration(result.Duration), ansiReset)

	return nil
}

// ANSI color constants for styled output.
const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiDim     = "\033[2m"
	ansiItalic  = "\033[3m"
	ansiRed     = "\033[31m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiCyan    = "\033[36m"
	ansiDimFg   = "\033[90m" // bright black / gray
)

// Helper functions

// formatValue returns the plain text representation of a value (no ANSI codes).
// Used for width calculation.
func formatValue(v interface{}) string {
	if v == nil {
		return "NULL"
	}
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// formatValueStyled returns the styled representation of a value (with ANSI codes).
// NULL values are rendered in dim italic for visual distinction.
func formatValueStyled(v interface{}) string {
	if v == nil {
		return ansiDim + ansiItalic + "NULL" + ansiReset
	}
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				i = j + 1
				continue
			}
		}
		result.WriteByte(s[i])
		i++
	}
	return result.String()
}

// padRightStyled pads a potentially ANSI-styled string to the given display width.
func padRightStyled(s string, width int) string {
	visibleWidth := utf8.RuneCountInString(stripANSI(s))
	if visibleWidth >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visibleWidth)
}

func convertForJSON(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case []byte:
		return string(val)
	default:
		return val
	}
}

func buildSeparator(widths []int) string {
	var sb strings.Builder
	sb.WriteString("+")
	for _, w := range widths {
		sb.WriteString(strings.Repeat("-", w+2))
		sb.WriteString("+")
	}
	return sb.String()
}

func padRight(s string, width int) string {
	runeCount := utf8.RuneCountInString(s)
	if runeCount >= width {
		return s
	}
	return s + strings.Repeat(" ", width-runeCount)
}

// CalcTableWidth returns the display width (in runes) that a table-formatted
// result would require. Returns 0 if the result has no columns.
func CalcTableWidth(result *executor.QueryResult) int {
	if result == nil || len(result.Columns) == 0 {
		return 0
	}

	// Calculate column widths (same logic as writeTable)
	widths := make([]int, len(result.Columns))
	for i, col := range result.Columns {
		widths[i] = utf8.RuneCountInString(col)
	}
	for _, row := range result.Rows {
		for i, val := range row {
			if i < len(widths) {
				w := utf8.RuneCountInString(formatValue(val))
				if w > widths[i] {
					widths[i] = w
				}
			}
		}
	}

	// Total width = sum of (column_width + 2 padding) + separators
	// Format: | col1 | col2 | ... |  =>  1 + sum(width+2+1) = 1 + sum(width+3)
	total := 1 // leading "|"
	for _, w := range widths {
		total += w + 3 // " value |" = 1 space + value + 1 space + "|"
	}
	return total
}

func formatDMLResult(result *executor.QueryResult) string {
	if result.AffectedRows >= 0 {
		return fmt.Sprintf("%sQuery OK%s, %s%d rows affected%s %s(%s)%s",
			ansiGreen+ansiBold, ansiReset,
			ansiGreen, result.AffectedRows, ansiReset,
			ansiCyan, executor.FormatDuration(result.Duration), ansiReset)
	}
	return fmt.Sprintf("%sQuery OK%s %s(%s)%s",
		ansiGreen+ansiBold, ansiReset,
		ansiCyan, executor.FormatDuration(result.Duration), ansiReset)
}

// pluralRow returns "row" or "rows" based on count.
func pluralRow(count int) string {
	if count == 1 {
		return "row"
	}
	return "rows"
}
