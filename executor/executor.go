package executor

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"mysh/connection"
	"mysh/metadata"
)

// QueryResult holds the result of a SQL query execution.
type QueryResult struct {
	Columns      []string      // Column names
	Rows         [][]any       // Row data as interface{} slice
	RowCount     int64         // Number of rows returned (for SELECT)
	AffectedRows int64         // Number of rows affected (for DML)
	Duration     time.Duration // Query execution time
	IsQuery      bool          // true for SELECT/SHOW/DESCRIBE, false for DML/DDL
	Error        error         // Error if any
}

// Executor handles SQL statement execution.
type Executor struct {
	pool    *connection.Pool
	meta    *metadata.Cache
	cancel  context.CancelFunc
}

// New creates a new SQL executor.
func New(pool *connection.Pool, meta *metadata.Cache) *Executor {
	return &Executor{
		pool: pool,
		meta: meta,
	}
}

// Execute runs a single SQL statement and returns the result.
func (e *Executor) Execute(ctx context.Context, query string) (*QueryResult, error) {
	start := time.Now()

	// Create a cancellable context
	_, cancel := context.WithTimeout(ctx, 30*time.Second)
	e.cancel = cancel
	defer cancel()

	query = strings.TrimSpace(query)

	// Remove trailing semicolon for execution
	query = strings.TrimSuffix(query, ";")

	if query == "" {
		return &QueryResult{
			Duration: time.Since(start),
			IsQuery:  false,
		}, nil
	}

	// Check if this is a query (SELECT/SHOW/DESCRIBE/EXPLAIN)
	isQuery := e.isQueryStatement(query)

	result := &QueryResult{
		IsQuery:  isQuery,
		Duration: time.Since(start), // will be updated
	}

	if isQuery {
		rows, err := e.pool.Query(query)
		if err != nil {
			result.Error = err
			result.Duration = time.Since(start)
			return result, fmt.Errorf("query error: %w", err)
		}
		defer rows.Close()

		// Read column names
		cols, err := rows.Columns()
		if err != nil {
			result.Error = err
			result.Duration = time.Since(start)
			return result, fmt.Errorf("column error: %w", err)
		}
		result.Columns = cols

		// Read rows
		for rows.Next() {
			row := make([]any, len(cols))
			rowPtrs := make([]any, len(cols))
			for i := range row {
				rowPtrs[i] = &row[i]
			}
			if err := rows.Scan(rowPtrs...); err != nil {
				result.Error = err
				result.Duration = time.Since(start)
				return result, fmt.Errorf("scan error: %w", err)
			}
			// Convert []byte to string for display
			converted := make([]any, len(row))
			for i, v := range row {
				if b, ok := v.([]byte); ok {
					converted[i] = string(b)
				} else {
					converted[i] = v
				}
			}
			result.Rows = append(result.Rows, converted)
		}
		if err := rows.Err(); err != nil {
			result.Error = err
			result.Duration = time.Since(start)
			return result, fmt.Errorf("rows error: %w", err)
		}
		result.RowCount = int64(len(result.Rows))
	} else {
		// DML/DDL execution
		res, err := e.pool.Exec(query)
		if err != nil {
			result.Error = err
			result.Duration = time.Since(start)
			return result, fmt.Errorf("exec error: %w", err)
		}

		affected, _ := res.RowsAffected()
		result.AffectedRows = affected

		// Mark metadata dirty if this is a DDL statement
		if e.IsDDL(query) && e.meta != nil {
			e.meta.MarkDirty()
		}
	}

	result.Duration = time.Since(start)
	return result, nil
}

// ExecuteMulti executes multiple SQL statements separated by semicolons.
func (e *Executor) ExecuteMulti(ctx context.Context, sql string) ([]*QueryResult, error) {
	statements := splitStatements(sql)
	var results []*QueryResult

	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		result, err := e.Execute(ctx, stmt)
		results = append(results, result)
		if err != nil {
			return results, err
		}
	}

	return results, nil
}

// IsDDL checks if the SQL statement is a DDL statement.
func (e *Executor) IsDDL(query string) bool {
	upper := strings.ToUpper(strings.TrimSpace(query))

	// Remove leading comments
	for {
		if strings.HasPrefix(upper, "--") {
			if idx := strings.Index(upper, "\n"); idx >= 0 {
				upper = strings.TrimSpace(upper[idx+1:])
				continue
			}
			return false
		}
		if strings.HasPrefix(upper, "/*") {
			if idx := strings.Index(upper, "*/"); idx >= 0 {
				upper = strings.TrimSpace(upper[idx+2:])
				continue
			}
			return false
		}
		break
	}

	ddlPrefixes := []string{"CREATE", "ALTER", "DROP", "TRUNCATE", "RENAME", "GRANT", "REVOKE"}
	for _, prefix := range ddlPrefixes {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	return false
}

// Cancel cancels the currently running query.
func (e *Executor) Cancel() {
	if e.cancel != nil {
		e.cancel()
	}
}

// isQueryStatement determines if the SQL is a query (returns rows).
func (e *Executor) isQueryStatement(query string) bool {
	upper := strings.ToUpper(strings.TrimSpace(query))

	// Remove leading comments
	for {
		if strings.HasPrefix(upper, "--") {
			if idx := strings.Index(upper, "\n"); idx >= 0 {
				upper = strings.TrimSpace(upper[idx+1:])
				continue
			}
			return false
		}
		if strings.HasPrefix(upper, "/*") {
			if idx := strings.Index(upper, "*/"); idx >= 0 {
				upper = strings.TrimSpace(upper[idx+2:])
				continue
			}
			return false
		}
		break
	}

	queryPrefixes := []string{"SELECT", "SHOW", "DESCRIBE", "DESC", "EXPLAIN", "WITH"}
	for _, prefix := range queryPrefixes {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	return false
}

// splitStatements splits SQL text into individual statements by semicolons,
// respecting string literals and comments.
func splitStatements(sql string) []string {
	var statements []string
	var current strings.Builder
	inString := false
	var stringChar byte

	for i := 0; i < len(sql); i++ {
		c := sql[i]

		// Handle string literals
		if inString {
			current.WriteByte(c)
			if c == '\\' && i+1 < len(sql) {
				i++
				current.WriteByte(sql[i])
				continue
			}
			if c == stringChar {
				inString = false
			}
			continue
		}

		// Handle single-line comments
		if c == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			for i < len(sql) && sql[i] != '\n' {
				current.WriteByte(sql[i])
				i++
			}
			if i < len(sql) {
				current.WriteByte(sql[i])
			}
			continue
		}

		// Handle multi-line comments
		if c == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			current.WriteString("/*")
			i += 2
			for i < len(sql) {
				if i+1 < len(sql) && sql[i] == '*' && sql[i+1] == '/' {
					current.WriteString("*/")
					i += 2
					break
				}
				current.WriteByte(sql[i])
				i++
			}
			i-- // compensate for the for-loop's i++
			continue
		}

		if c == '\'' || c == '"' {
			inString = true
			stringChar = c
			current.WriteByte(c)
			continue
		}

		if c == ';' {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
			continue
		}

		current.WriteByte(c)
	}

	// Don't forget the last statement (without semicolon)
	stmt := strings.TrimSpace(current.String())
	if stmt != "" {
		statements = append(statements, stmt)
	}

	return statements
}

// FormatDuration formats a duration for display.
func FormatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%.2fms", float64(d.Microseconds())/1000.0)
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

// NullString is a helper to convert nullable sql.NullString.
func NullString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}
