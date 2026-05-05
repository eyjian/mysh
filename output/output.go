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
		_, err := fmt.Fprintf(f.writer, "ERROR: %s\n", result.Error.Error())
		return err
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

	// Convert rows to strings for width calculation
	strRows := make([][]string, len(result.Rows))
	for i, row := range result.Rows {
		strRows[i] = make([]string, len(row))
		for j, val := range row {
			strRows[i][j] = formatValue(val)
		}
	}

	// Calculate column widths
	widths := make([]int, len(result.Columns))
	for i, col := range result.Columns {
		widths[i] = utf8.RuneCountInString(col)
	}
	for _, row := range strRows {
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

	// Print rows
	for _, row := range strRows {
		var line strings.Builder
		line.WriteString("|")
		for i, val := range row {
			if i < len(widths) {
				line.WriteString(" ")
				line.WriteString(padRight(val, widths[i]))
				line.WriteString(" |")
			}
		}
		fmt.Fprintln(f.writer, line.String())
	}

	if len(strRows) > 0 {
		fmt.Fprintln(f.writer, separator)
	}

	// Row count and timing
	count := len(result.Rows)
	if count == 1 {
		fmt.Fprintf(f.writer, "1 row in set (%s)\n", executor.FormatDuration(result.Duration))
	} else {
		fmt.Fprintf(f.writer, "%d rows in set (%s)\n", count, executor.FormatDuration(result.Duration))
	}

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
			val := "NULL"
			if i < len(row) && row[i] != nil {
				val = formatValue(row[i])
			}
			fmt.Fprintf(f.writer, "%s: %s\n", padRight(col, labelWidth), val)
		}
	}

	count := len(result.Rows)
	if count == 1 {
		fmt.Fprintf(f.writer, "1 row in set (%s)\n", executor.FormatDuration(result.Duration))
	} else {
		fmt.Fprintf(f.writer, "%d rows in set (%s)\n", count, executor.FormatDuration(result.Duration))
	}

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
	if count == 1 {
		fmt.Fprintf(f.writer, "1 row in set (%s)\n", executor.FormatDuration(result.Duration))
	} else {
		fmt.Fprintf(f.writer, "%d rows in set (%s)\n", count, executor.FormatDuration(result.Duration))
	}

	return nil
}

// writeMarkdown formats results as a Markdown table.
func (f *Formatter) writeMarkdown(result *executor.QueryResult) error {
	if len(result.Columns) == 0 {
		return nil
	}

	// Convert rows to strings for width calculation
	strRows := make([][]string, len(result.Rows))
	for i, row := range result.Rows {
		strRows[i] = make([]string, len(row))
		for j, val := range row {
			strRows[i][j] = formatValue(val)
		}
	}

	// Calculate column widths
	widths := make([]int, len(result.Columns))
	for i, col := range result.Columns {
		widths[i] = utf8.RuneCountInString(col)
	}
	for _, row := range strRows {
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

	// Print data rows
	for _, row := range strRows {
		var line strings.Builder
		line.WriteString("|")
		for i, val := range row {
			if i < len(widths) {
				line.WriteString(" ")
				line.WriteString(padRight(val, widths[i]))
				line.WriteString(" |")
			}
		}
		fmt.Fprintln(f.writer, line.String())
	}

	// Row count and timing
	count := len(result.Rows)
	if count == 1 {
		fmt.Fprintf(f.writer, "1 row in set (%s)\n", executor.FormatDuration(result.Duration))
	} else {
		fmt.Fprintf(f.writer, "%d rows in set (%s)\n", count, executor.FormatDuration(result.Duration))
	}

	return nil
}

// Helper functions

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
		return fmt.Sprintf("Query OK, %d rows affected (%s)", result.AffectedRows, executor.FormatDuration(result.Duration))
	}
	return fmt.Sprintf("Query OK (%s)", executor.FormatDuration(result.Duration))
}
