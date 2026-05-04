package connection

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"mysh/config"
)

// Pool manages MySQL database connections.
type Pool struct {
	db     *sql.DB
	cfg    *config.ConnectionConfig
	closed bool
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

	// Verify connectivity with retries
	maxRetries := 3
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := db.PingContext(ctx)
		cancel()
		if err == nil {
			return &Pool{db: db, cfg: cfg}, nil
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
func (p *Pool) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return p.db.Query(query, args...)
}

// QueryContext executes a query with context that returns rows.
func (p *Pool) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return p.db.QueryContext(ctx, query, args...)
}

// Exec executes a query without returning rows.
func (p *Pool) Exec(query string, args ...interface{}) (sql.Result, error) {
	return p.db.Exec(query, args...)
}

// ExecContext executes a query with context without returning rows.
func (p *Pool) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return p.db.ExecContext(ctx, query, args...)
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = db.PingContext(ctx)
	cancel()
	if err != nil {
		db.Close()
		return fmt.Errorf("failed to ping after reset: %w", err)
	}

	p.db = db
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
