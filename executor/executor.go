package executor

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/eyjian/mysh/connection"
	"github.com/eyjian/mysh/metadata"
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
	Warning      string        // Non-critical message (e.g., "Reconnected to server")
	SlowQuery    bool          // true if query exceeded slow query threshold
}

// SafeUpdateError is returned when a DML statement is blocked by safe-updates mode.
type SafeUpdateError struct {
	Query string
}

func (e *SafeUpdateError) Error() string {
	return fmt.Sprintf("Safe update mode: statement blocked (no WHERE/LIMIT clause): %s", e.Query)
}

// Executor handles SQL statement execution.
type Executor struct {
	pool           *connection.Pool
	meta           *metadata.Cache
	cancel         context.CancelFunc
	safeUpdates    bool          // when true, block UPDATE/DELETE without WHERE/LIMIT
	slowThreshold  time.Duration // queries slower than this are flagged (0 = disabled)
	txConn         *sql.Conn     // dedicated connection for transaction mode (nil when not in tx)
	inTransaction  bool          // true when inside a BEGIN..COMMIT/ROLLBACK block
}

// New creates a new SQL executor.
func New(pool *connection.Pool, meta *metadata.Cache) *Executor {
	return &Executor{
		pool:          pool,
		meta:          meta,
		slowThreshold: 0, // disabled by default
	}
}

// SetSafeUpdates enables or disables safe-updates mode.
func (e *Executor) SetSafeUpdates(enabled bool) {
	e.safeUpdates = enabled
}

// SafeUpdates returns whether safe-updates mode is enabled.
func (e *Executor) SafeUpdates() bool {
	return e.safeUpdates
}

// SetSlowThreshold sets the slow query warning threshold.
// A duration of 0 disables slow query warnings.
func (e *Executor) SetSlowThreshold(d time.Duration) {
	e.slowThreshold = d
}

// SlowThreshold returns the current slow query threshold.
func (e *Executor) SlowThreshold() time.Duration {
	return e.slowThreshold
}

// Execute runs a single SQL statement and returns the result.
// If a connection error occurs, it attempts to reconnect and retry once.
// It also detects BEGIN/COMMIT/ROLLBACK to manage transaction mode.
func (e *Executor) Execute(ctx context.Context, query string) (*QueryResult, error) {
	// Check safe-updates before execution
	if e.safeUpdates {
		if err := e.checkSafeUpdates(query); err != nil {
			return nil, err
		}
	}

	// Detect transaction control statements
	upper := strings.ToUpper(strings.TrimSpace(query))
	upper = stripComments(upper)
	upper = strings.TrimSuffix(upper, ";")

	if e.isBeginStatement(upper) {
		return e.beginTransaction(ctx)
	}
	if e.isCommitStatement(upper) {
		return e.commitTransaction(ctx)
	}
	if e.isRollbackStatement(upper) {
		return e.rollbackTransaction(ctx)
	}

	result, err := e.executeOnce(ctx, query)
	if err == nil || !connection.IsConnectionError(err) {
		if err == nil && e.slowThreshold > 0 && result.Duration > e.slowThreshold {
			result.SlowQuery = true
		}
		return result, err
	}

	// Connection error — try to reconnect once
	if e.pool != nil && !e.inTransaction {
		if reconnErr := e.pool.Reconnect(); reconnErr != nil {
			return nil, fmt.Errorf("connection lost and reconnect failed: %w", reconnErr)
		}
		// Retry the query after reconnection
		result, err = e.executeOnce(ctx, query)
		if err == nil {
			result.Warning = "Reconnected to server"
			if e.slowThreshold > 0 && result.Duration > e.slowThreshold {
				result.SlowQuery = true
			}
		}
	}
	return result, err
}

// checkSafeUpdates validates that UPDATE/DELETE statements have WHERE or LIMIT clauses.
func (e *Executor) checkSafeUpdates(query string) error {
	upper := strings.ToUpper(strings.TrimSpace(query))

	// Remove leading comments
	upper = stripComments(upper)

	// Only check UPDATE and DELETE statements
	if !strings.HasPrefix(upper, "UPDATE") && !strings.HasPrefix(upper, "DELETE") {
		return nil
	}

	// Check for WHERE or LIMIT clause
	hasWhere := strings.Contains(upper, " WHERE ")
	hasLimit := strings.Contains(upper, " LIMIT ")

	// Also check for case variations at boundaries
	if !hasWhere {
		hasWhere = strings.Contains(upper, "\nWHERE ") || strings.Contains(upper, "\tWHERE ")
	}
	if !hasLimit {
		hasLimit = strings.Contains(upper, "\nLIMIT ") || strings.Contains(upper, "\tLIMIT ")
	}

	if !hasWhere && !hasLimit {
		return &SafeUpdateError{Query: truncate(query, 80)}
	}

	return nil
}

// stripComments removes SQL comments from the beginning of a query.
func stripComments(query string) string {
	for {
		query = strings.TrimSpace(query)
		if strings.HasPrefix(query, "--") {
			if idx := strings.Index(query, "\n"); idx >= 0 {
				query = query[idx+1:]
				continue
			}
			return ""
		}
		if strings.HasPrefix(query, "/*") {
			if idx := strings.Index(query, "*/"); idx >= 0 {
				query = query[idx+2:]
				continue
			}
			return ""
		}
		return query
	}
}

// truncate shortens a string to maxLen, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// executeOnce runs a single SQL statement without retry logic.
func (e *Executor) executeOnce(ctx context.Context, query string) (*QueryResult, error) {
	start := time.Now()

	// Create a cancellable context with connection timeout
	timeout := 30 * time.Second
	if e.pool != nil && e.pool.ConnTimeout() > 0 {
		timeout = e.pool.ConnTimeout()
	}
	_, cancel := context.WithTimeout(ctx, timeout)
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
		var rows *sql.Rows
		var err error
		if e.inTransaction && e.txConn != nil {
			rows, err = e.txConn.QueryContext(ctx, query)
		} else {
			rows, err = e.pool.Query(query)
		}
		if err != nil {
			result.Error = err
			result.Duration = time.Since(start)
			return result, err
		}
		defer rows.Close()

		// Read column names
		cols, err := rows.Columns()
		if err != nil {
			result.Error = err
			result.Duration = time.Since(start)
			return result, err
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
				return result, err
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
			return result, err
		}
		result.RowCount = int64(len(result.Rows))
	} else {
		// DML/DDL execution
		var res sql.Result
		var err error
		if e.inTransaction && e.txConn != nil {
			res, err = e.txConn.ExecContext(ctx, query)
		} else {
			res, err = e.pool.Exec(query)
		}
		if err != nil {
			result.Error = err
			result.Duration = time.Since(start)
			return result, err
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
	statements := SplitStatements(sql)
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
	upper = stripComments(upper)

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

// InTransaction returns whether the executor is currently in transaction mode.
func (e *Executor) InTransaction() bool {
	return e.inTransaction
}

// RollbackTransaction forces a rollback of the current transaction.
// This is used by the \rollback command or when exiting while in a transaction.
func (e *Executor) RollbackTransaction() error {
	if !e.inTransaction || e.txConn == nil {
		return nil
	}
	_, err := e.txConn.ExecContext(context.Background(), "ROLLBACK")
	e.releaseTxConn()
	return err
}

// beginTransaction starts a transaction by acquiring a dedicated connection.
func (e *Executor) beginTransaction(ctx context.Context) (*QueryResult, error) {
	start := time.Now()

	if e.inTransaction {
		return nil, fmt.Errorf("already in transaction")
	}

	conn, err := e.pool.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire connection for transaction: %w", err)
	}

	// Execute BEGIN on the dedicated connection
	_, err = conn.ExecContext(ctx, "BEGIN")
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	e.txConn = conn
	e.inTransaction = true

	return &QueryResult{
		Duration:     time.Since(start),
		IsQuery:      false,
		AffectedRows: 0,
		Warning:      "Transaction started",
	}, nil
}

// commitTransaction commits the current transaction and releases the dedicated connection.
func (e *Executor) commitTransaction(ctx context.Context) (*QueryResult, error) {
	start := time.Now()

	if !e.inTransaction || e.txConn == nil {
		return nil, fmt.Errorf("not in transaction")
	}

	_, err := e.txConn.ExecContext(ctx, "COMMIT")
	if err != nil {
		return nil, fmt.Errorf("commit failed: %w", err)
	}

	e.releaseTxConn()

	return &QueryResult{
		Duration:     time.Since(start),
		IsQuery:      false,
		AffectedRows: 0,
	}, nil
}

// rollbackTransaction rolls back the current transaction and releases the dedicated connection.
func (e *Executor) rollbackTransaction(ctx context.Context) (*QueryResult, error) {
	start := time.Now()

	if !e.inTransaction || e.txConn == nil {
		return nil, fmt.Errorf("not in transaction")
	}

	_, err := e.txConn.ExecContext(ctx, "ROLLBACK")
	if err != nil {
		return nil, fmt.Errorf("rollback failed: %w", err)
	}

	e.releaseTxConn()

	return &QueryResult{
		Duration:     time.Since(start),
		IsQuery:      false,
		AffectedRows: 0,
	}, nil
}

// releaseTxConn releases the dedicated transaction connection.
func (e *Executor) releaseTxConn() {
	if e.txConn != nil {
		e.txConn.Close()
		e.txConn = nil
	}
	e.inTransaction = false
}

// isBeginStatement checks if the SQL starts a transaction.
func (e *Executor) isBeginStatement(upper string) bool {
	return upper == "BEGIN" || upper == "START TRANSACTION" ||
		strings.HasPrefix(upper, "START TRANSACTION ")
}

// isCommitStatement checks if the SQL commits a transaction.
func (e *Executor) isCommitStatement(upper string) bool {
	return upper == "COMMIT"
}

// isRollbackStatement checks if the SQL rolls back a transaction.
func (e *Executor) isRollbackStatement(upper string) bool {
	return upper == "ROLLBACK"
}

// isQueryStatement determines if the SQL is a query (returns rows).
func (e *Executor) isQueryStatement(query string) bool {
	upper := strings.ToUpper(strings.TrimSpace(query))

	// Remove leading comments
	upper = stripComments(upper)

	queryPrefixes := []string{"SELECT", "SHOW", "DESCRIBE", "DESC", "EXPLAIN", "WITH"}
	for _, prefix := range queryPrefixes {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	return false
}

// SplitStatements splits SQL text into individual statements by semicolons,
// respecting string literals and comments.
func SplitStatements(sql string) []string {
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

// IsConnectionError checks if an error is caused by a lost connection.
// Deprecated: Use connection.IsConnectionError instead.
func IsConnectionError(err error) bool {
	return connection.IsConnectionError(err)
}
