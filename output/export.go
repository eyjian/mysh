package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/eyjian/mysh/executor"
)

// ExportFormat represents the file format for exporting query results.
type ExportFormat int

const (
	ExportCSV ExportFormat = iota
	ExportJSON
	ExportMarkdown
)

// String returns the string representation of an ExportFormat.
func (f ExportFormat) String() string {
	switch f {
	case ExportCSV:
		return "csv"
	case ExportJSON:
		return "json"
	case ExportMarkdown:
		return "markdown"
	default:
		return "csv"
	}
}

// ParseExportFormat parses a format string and returns the ExportFormat.
func ParseExportFormat(s string) (ExportFormat, error) {
	switch strings.ToLower(s) {
	case "csv":
		return ExportCSV, nil
	case "json":
		return ExportJSON, nil
	case "markdown", "md":
		return ExportMarkdown, nil
	default:
		return ExportCSV, fmt.Errorf("unknown export format: %s (supported: csv, json, markdown)", s)
	}
}

// InferExportFormat infers the export format from a file extension.
func InferExportFormat(path string) (ExportFormat, error) {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".csv") {
		return ExportCSV, nil
	}
	if strings.HasSuffix(lower, ".json") {
		return ExportJSON, nil
	}
	if strings.HasSuffix(lower, ".md") {
		return ExportMarkdown, nil
	}
	return ExportCSV, fmt.Errorf("cannot infer format from file extension, please specify format: %s", path)
}

// ExportResult writes a query result to a file in the specified format.
// Returns the number of rows exported.
func ExportResult(result *executor.QueryResult, path string, format ExportFormat) (int, error) {
	if result == nil || !result.IsQuery || len(result.Columns) == 0 {
		return 0, fmt.Errorf("no query result to export")
	}

	f, err := os.Create(path)
	if err != nil {
		return 0, fmt.Errorf("cannot create file: %w", err)
	}
	defer f.Close()

	switch format {
	case ExportCSV:
		return exportCSV(result, f)
	case ExportJSON:
		return exportJSON(result, f)
	case ExportMarkdown:
		return exportMarkdown(result, f)
	default:
		return exportCSV(result, f)
	}
}

// exportCSV writes the result as CSV (no ANSI codes).
func exportCSV(result *executor.QueryResult, w io.Writer) (int, error) {
	writer := csv.NewWriter(w)
	defer writer.Flush()

	if err := writer.Write(result.Columns); err != nil {
		return 0, err
	}

	for _, row := range result.Rows {
		record := make([]string, len(row))
		for i, val := range row {
			record[i] = formatValue(val)
		}
		if err := writer.Write(record); err != nil {
			return 0, err
		}
	}

	writer.Flush()
	return len(result.Rows), writer.Error()
}

// exportJSON writes the result as JSON (no ANSI codes).
func exportJSON(result *executor.QueryResult, w io.Writer) (int, error) {
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

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(records); err != nil {
		return 0, fmt.Errorf("JSON encode failed: %w", err)
	}

	return len(result.Rows), nil
}

// exportMarkdown writes the result as a Markdown table (no ANSI codes).
func exportMarkdown(result *executor.QueryResult, w io.Writer) (int, error) {
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

	// Header
	fmt.Fprint(w, "|")
	for i, col := range result.Columns {
		fmt.Fprintf(w, " %s |", padRight(col, widths[i]))
	}
	fmt.Fprintln(w)

	// Separator
	fmt.Fprint(w, "|")
	for _, cw := range widths {
		fmt.Fprintf(w, " %s |", strings.Repeat("-", cw))
	}
	fmt.Fprintln(w)

	// Data rows
	for _, row := range result.Rows {
		fmt.Fprint(w, "|")
		for i, val := range row {
			if i < len(widths) {
				fmt.Fprintf(w, " %s |", padRight(formatValue(val), widths[i]))
			}
		}
		fmt.Fprintln(w)
	}

	return len(result.Rows), nil
}
