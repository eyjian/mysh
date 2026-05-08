package connection

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"

	"github.com/eyjian/mysh/config"
)

// Pool manages database connections.
type Pool struct {
	db           *sql.DB
	cfg          *config.ConnectionConfig
	adapter      DBAdapter
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

	adapter := NewAdapter(cfg.Driver)

	dsn := adapter.DSN(
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, cfg.Charset, cfg.SSLMode)
	// Password-free DSN for error messages / logs
	safeDSN := adapter.DSN(
		cfg.Host, cfg.Port, cfg.User, "", cfg.Database, cfg.Charset, cfg.SSLMode)
	db, err := sql.Open(adapter.DriverName(), dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w (dsn: %s)", err, safeDSN)
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
				adapter:      adapter,
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
	return nil, fmt.Errorf("failed to connect after %d retries: %w (dsn: %s)", maxRetries, lastErr, safeDSN)
}

// NewWithDB creates a Pool from an existing *sql.DB (for testing).
func NewWithDB(db *sql.DB, cfg *config.ConnectionConfig) *Pool {
	return &Pool{db: db, cfg: cfg, adapter: NewAdapter(cfg.Driver)}
}

// DB returns the underlying *sql.DB instance.
func (p *Pool) DB() *sql.DB {
	return p.db
}

// Adapter returns the DBAdapter in use.
func (p *Pool) Adapter() DBAdapter {
	return p.adapter
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
	if p.adapter.IsConnectionError(err) {
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
	if p.adapter.IsConnectionError(err) {
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
// This delegates to the adapter's implementation.
func IsConnectionError(err error) bool {
	if err == nil {
		return false
	}
	// Use a generic check that covers both MySQL and PG
	msg := err.Error()
	return strings.Contains(msg, "invalid connection") ||
		strings.Contains(msg, "bad connection") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "server has gone away") ||
		strings.Contains(msg, "connect: connection refused") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "driver: bad conn") ||
		strings.Contains(msg, "conn closed") ||
		strings.Contains(msg, "connection unexpectedly closed")
}

// CurrentDB returns the currently selected database name.
func (p *Pool) CurrentDB() string {
	name, err := p.adapter.CurrentDB(p.db)
	if err != nil {
		return ""
	}
	return name
}

// CurrentSchema returns the currently active schema name.
// For MySQL this is the same as CurrentDB; for PostgreSQL it is typically "public".
func (p *Pool) CurrentSchema() string {
	name, err := p.adapter.CurrentSchema(p.db)
	if err != nil {
		return ""
	}
	return name
}

// DriverName returns the driver name of the current adapter (e.g., "mysql", "postgres").
func (p *Pool) DriverName() string {
	return p.adapter.DriverName()
}

// UseDB switches the current database.
func (p *Pool) UseDB(ctx context.Context, dbName string) error {
	return p.adapter.UseDB(ctx, p.db, dbName)
}

// Reset re-establishes the connection with a new config.
func (p *Pool) Reset(cfg *config.ConnectionConfig) error {
	if p.db != nil {
		p.db.Close()
	}

	p.cfg = cfg
	p.closed = false
	p.adapter = NewAdapter(cfg.Driver)

	db, err := sql.Open(p.adapter.DriverName(), p.adapter.DSN(
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, cfg.Charset, cfg.SSLMode))
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
	return p.adapter.Databases(p.db)
}

// Tables returns a list of tables in the current or specified database.
func (p *Pool) Tables(database string) ([]string, error) {
	return p.adapter.Tables(p.db, database)
}

// Columns returns column names for a given table.
func (p *Pool) Columns(database, table string) ([]ColumnInfo, error) {
	return p.adapter.Columns(p.db, database, table)
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
	return p.adapter.ShowCreateTable(p.db, database, table)
}

// ShowIndexes returns index information for a table.
func (p *Pool) ShowIndexes(database, table string) ([]TableIndexInfo, error) {
	return p.adapter.ShowIndexes(p.db, database, table)
}

// DescribeTableSQL returns the SQL for describing a table.
func (p *Pool) DescribeTableSQL(database, table string) string {
	return p.adapter.DescribeTableSQL(database, table)
}

// DescribeFullSQL returns the SQL for full column info.
func (p *Pool) DescribeFullSQL(database, table string) string {
	return p.adapter.DescribeFullSQL(database, table)
}

// Schemas returns a list of schemas (MySQL: databases, PostgreSQL: schemas).
func (p *Pool) Schemas() ([]string, error) {
	return p.adapter.Schemas(p.db)
}

// Users returns a list of database users.
func (p *Pool) Users() ([]string, error) {
	return p.adapter.Users(p.db)
}

// Views returns a list of views in the given database/schema.
func (p *Pool) Views(database string) ([]string, error) {
	return p.adapter.Views(p.db, database)
}

// ShowFunction returns the definition of a function.
func (p *Pool) ShowFunction(database, function string) (string, error) {
	return p.adapter.ShowFunction(p.db, database, function)
}

// TablePrivileges returns privilege information for a table.
func (p *Pool) TablePrivileges(database, table string) ([]string, error) {
	return p.adapter.TablePrivileges(p.db, database, table)
}

// ListIndexes returns index information for a table (or all tables if table is empty).
func (p *Pool) ListIndexes(database, table string) ([]TableIndexInfo, error) {
	return p.adapter.ListIndexes(p.db, database, table)
}

// SetEncoding sets the client character encoding.
func (p *Pool) SetEncoding(encoding string) error {
	return p.adapter.SetEncoding(p.db, encoding)
}

// GetEncoding returns the current client character encoding.
func (p *Pool) GetEncoding() (string, error) {
	return p.adapter.GetEncoding(p.db)
}
