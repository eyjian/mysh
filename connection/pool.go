package connection

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/eyjian/mysh/config"
)

// Pool manages MySQL database connections.
type Pool struct {
	db           *sql.DB
	cfg          *config.ConnectionConfig
	closed       bool
	connTimeout  time.Duration // per-query connection timeout (default 30s)
	lastActivity time.Time     // last successful query timestamp
}

// New creates a new connection pool and verifies connectivity.
// It retries up to maxRetries times with a timeout.
func New(cfg *config.ConnectionConfig) (*Pool, error) {
	if cfg == nil {
		return nil, fmt.Errorf("connection config is nil")
	}

	db, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	// Verify connectivity with retries
	maxRetries := 3
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := db.PingContext(ctx)
		cancel()
		if err == nil {
			return &Pool{
				db:           db,
				cfg:          cfg,
				connTimeout:  30 * time.Second,
				lastActivity: time.Now(),
			}, nil
		}
		lastErr = err
		if i < maxRetries-1 {
			time.Sleep(time.Duration(i+1) * 500*time.Millisecond)
		}
	}

	db.Close()
	return nil, fmt.Errorf("failed to connect after %d retries: %w", maxRetries, lastErr)
}

// NewWithDB creates a Pool from an existing *sql.DB (for testing).
func NewWithDB(db *sql.DB, cfg *config.ConnectionConfig) *Pool {
	return &Pool{db: db, cfg: cfg}
}

// DB returns the underlying *sql.DB instance.
func (p *Pool) DB() *sql.DB {
	return p.db
}

// Close closes the connection pool.
func (p *Pool) Close() error {
	if p.closed {
		return nil
	}
	p.closed = true
	return p.db.Close()
}

// Ping verifies the connection is still alive.
func (p *Pool) Ping(ctx context.Context) error {
	return p.db.PingContext(ctx)
}

// IsConnected returns whether the pool is still connected.
func (p *Pool) IsConnected() bool {
	if p.closed || p.db == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	err := p.db.PingContext(ctx)
	cancel()
	return err == nil
}

// Query executes a query that returns rows.
// If the connection has been idle too long and a connection error occurs,
// it attempts to reconnect once and retry.
func (p *Pool) Query(query string, args ...interface{}) (*sql.Rows, error) {
	rows, err := p.db.Query(query, args...)
	if err == nil {
		p.lastActivity = time.Now()
		return rows, nil
	}
	// Connection error — try reconnect and retry
	if IsConnectionError(err) {
		if reconnErr := p.Reconnect(); reconnErr != nil {
			return nil, fmt.Errorf("connection lost and reconnect failed: %w", reconnErr)
		}
		rows, err = p.db.Query(query, args...)
		if err == nil {
			p.lastActivity = time.Now()
		}
	}
	return rows, err
}

// QueryContext executes a query with context that returns rows.
func (p *Pool) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	rows, err := p.db.QueryContext(ctx, query, args...)
	if err == nil {
		p.lastActivity = time.Now()
	}
	return rows, err
}

// Exec executes a query without returning rows.
// If the connection has been idle too long and a connection error occurs,
// it attempts to reconnect once and retry.
func (p *Pool) Exec(query string, args ...interface{}) (sql.Result, error) {
	res, err := p.db.Exec(query, args...)
	if err == nil {
		p.lastActivity = time.Now()
		return res, nil
	}
	// Connection error — try reconnect and retry
	if IsConnectionError(err) {
		if reconnErr := p.Reconnect(); reconnErr != nil {
			return nil, fmt.Errorf("connection lost and reconnect failed: %w", reconnErr)
		}
		res, err = p.db.Exec(query, args...)
		if err == nil {
			p.lastActivity = time.Now()
		}
	}
	return res, err
}

// ExecContext executes a query with context without returning rows.
func (p *Pool) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	res, err := p.db.ExecContext(ctx, query, args...)
	if err == nil {
		p.lastActivity = time.Now()
	}
	return res, err
}

// Conn returns a single dedicated connection from the pool.
// The caller is responsible for closing the connection when done.
// This is used for transaction mode where all statements must run on the same connection.
func (p *Pool) Conn(ctx context.Context) (*sql.Conn, error) {
	return p.db.Conn(ctx)
}

// SetConnTimeout sets the per-query connection timeout.
func (p *Pool) SetConnTimeout(d time.Duration) {
	p.connTimeout = d
}

// ConnTimeout returns the current connection timeout.
func (p *Pool) ConnTimeout() time.Duration {
	return p.connTimeout
}

// LastActivity returns the time of the last successful query.
func (p *Pool) LastActivity() time.Time {
	return p.lastActivity
}

// IsIdleTooLong checks if the connection has been idle longer than the given duration.
func (p *Pool) IsIdleTooLong(idleThreshold time.Duration) bool {
	return time.Since(p.lastActivity) > idleThreshold
}

// IsConnectionError checks if an error is caused by a lost connection.
func IsConnectionError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "invalid connection") ||
		strings.Contains(msg, "bad connection") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "server has gone away") ||
		strings.Contains(msg, "connect: connection refused") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "driver: bad conn")
}

// CurrentDB returns the currently selected database name.
func (p *Pool) CurrentDB() string {
	var db string
	err := p.db.QueryRow("SELECT DATABASE()").Scan(&db)
	if err != nil {
		return ""
	}
	return db
}

// UseDB switches the current database.
func (p *Pool) UseDB(ctx context.Context, dbName string) error {
	_, err := p.db.ExecContext(ctx, "USE `"+dbName+"`")
	return err
}

// Reset re-establishes the connection with a new config.
func (p *Pool) Reset(cfg *config.ConnectionConfig) error {
	if p.db != nil {
		p.db.Close()
	}

	p.cfg = cfg
	p.closed = false

	db, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return fmt.Errorf("failed to reset connection: %w", err)
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = db.PingContext(ctx)
	cancel()
	if err != nil {
		db.Close()
		return fmt.Errorf("failed to ping after reset: %w", err)
	}

	p.db = db
	p.lastActivity = time.Now()
	return nil
}

// IsClosed returns whether the pool has been closed.
func (p *Pool) IsClosed() bool {
	return p.closed
}

// Reconnect attempts to re-establish the connection.
func (p *Pool) Reconnect() error {
	return p.Reset(p.cfg)
}

// Databases returns a list of all accessible databases.
func (p *Pool) Databases() ([]string, error) {
	rows, err := p.db.Query("SHOW DATABASES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dbs []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		dbs = append(dbs, name)
	}
	return dbs, rows.Err()
}

// Tables returns a list of tables in the current or specified database.
func (p *Pool) Tables(database string) ([]string, error) {
	query := "SHOW TABLES"
	if database != "" {
		query = fmt.Sprintf("SHOW TABLES FROM `%s`", database)
	}

	rows, err := p.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

// Columns returns column names for a given table.
func (p *Pool) Columns(database, table string) ([]ColumnInfo, error) {
	query := fmt.Sprintf("SHOW COLUMNS FROM `%s`", table)
	if database != "" {
		query = fmt.Sprintf("SHOW COLUMNS FROM `%s`.`%s`", database, table)
	}

	rows, err := p.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []ColumnInfo
	for rows.Next() {
		var field, typ, null, key string
		var def, extra sql.NullString
		if err := rows.Scan(&field, &typ, &null, &key, &def, &extra); err != nil {
			return nil, err
		}
		cols = append(cols, ColumnInfo{
			Name:     field,
			Type:     typ,
			Nullable: null == "YES",
			Key:      key,
		})
	}
	return cols, rows.Err()
}

// ColumnInfo holds metadata about a database column.
type ColumnInfo struct {
	Name     string
	Type     string
	Nullable bool
	Key      string
}

// TableIndexInfo holds metadata about a table index.
type TableIndexInfo struct {
	Name      string // Index name
	Columns   string // Column names in the index
	NonUnique bool   // Whether the index allows duplicates
	Type      string // Index type (BTREE, HASH, etc.)
}

// ShowCreateTable returns the CREATE TABLE statement for a table.
func (p *Pool) ShowCreateTable(database, table string) (string, error) {
	query := fmt.Sprintf("SHOW CREATE TABLE `%s`", table)
	if database != "" {
		query = fmt.Sprintf("SHOW CREATE TABLE `%s`.`%s`", database, table)
	}

	rows, err := p.db.Query(query)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	// MySQL returns: Table, Create Table, ...
	// MariaDB may return: Table, Create Table, ...
	// Allocate scanners for all columns
	values := make([]sql.NullString, len(cols))
	scanArgs := make([]interface{}, len(cols))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	if rows.Next() {
		if err := rows.Scan(scanArgs...); err != nil {
			return "", err
		}
		// The CREATE TABLE statement is in the second column
		if len(values) >= 2 && values[1].Valid {
			return values[1].String, nil
		}
	}
	return "", rows.Err()
}

// ShowIndexes returns index information for a table.
func (p *Pool) ShowIndexes(database, table string) ([]TableIndexInfo, error) {
	query := fmt.Sprintf("SHOW INDEX FROM `%s`", table)
	if database != "" {
		query = fmt.Sprintf("SHOW INDEX FROM `%s`.`%s`", database, table)
	}

	rows, err := p.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexes []TableIndexInfo
	for rows.Next() {
		var table1, nonUnique, keyName, seqInIndex, colName, collation,
			cardinality, subPart, packed, nullable, indexType, comment,
			indexComment, visible string
		var nullablePtr, subPartPtr, packedPtr, commentPtr, indexCommentPtr, visiblePtr sql.NullString

		if err := rows.Scan(&table1, &nonUnique, &keyName, &seqInIndex,
			&colName, &collation, &cardinality, &subPartPtr, &packedPtr,
			&nullablePtr, &indexType, &commentPtr, &indexCommentPtr, &visiblePtr); err != nil {
			// Try simpler scan for older MySQL versions
			rows.Close()
			return p.showIndexesSimple(database, table)
		}

		_ = nullable
		_ = subPart
		_ = packed
		_ = comment
		_ = indexComment
		_ = visible

		indexes = append(indexes, TableIndexInfo{
			Name:      keyName,
			Columns:   colName,
			NonUnique: nonUnique == "1",
			Type:      indexType,
		})
	}
	return indexes, rows.Err()
}

// showIndexesSimple is a fallback for MySQL versions with fewer SHOW INDEX columns.
func (p *Pool) showIndexesSimple(database, table string) ([]TableIndexInfo, error) {
	query := fmt.Sprintf("SHOW INDEX FROM `%s`", table)
	if database != "" {
		query = fmt.Sprintf("SHOW INDEX FROM `%s`.`%s`", database, table)
	}

	rows, err := p.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexes []TableIndexInfo
	for rows.Next() {
		// Use a dynamic scan approach
		cols, _ := rows.Columns()
		values := make([]interface{}, len(cols))
		for i := range values {
			values[i] = new(sql.NullString)
		}
		if err := rows.Scan(values...); err != nil {
			return nil, err
		}

		keyName := ""
		colName := ""
		nonUnique := "0"
		indexType := ""

		// Standard column positions in SHOW INDEX
		if len(cols) > 1 {
			if v, ok := values[1].(*sql.NullString); ok && v.Valid {
				nonUnique = v.String
			}
		}
		if len(cols) > 2 {
			if v, ok := values[2].(*sql.NullString); ok && v.Valid {
				keyName = v.String
			}
		}
		if len(cols) > 4 {
			if v, ok := values[4].(*sql.NullString); ok && v.Valid {
				colName = v.String
			}
		}
		if len(cols) > 10 {
			if v, ok := values[10].(*sql.NullString); ok && v.Valid {
				indexType = v.String
			}
		}

		indexes = append(indexes, TableIndexInfo{
			Name:      keyName,
			Columns:   colName,
			NonUnique: nonUnique == "1",
			Type:      indexType,
		})
	}
	return indexes, rows.Err()
}
