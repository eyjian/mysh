package connection

import (
	"context"
	"database/sql"
)

// Execer abstracts statement execution so that operations can run either on
// the whole pool (*sql.DB) or on a single dedicated connection (*sql.Conn).
// This matters for session-scoped statements such as MySQL's "USE db" which
// only affect the connection they run on.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

// DBAdapter abstracts database-specific operations so that mysh can support
// multiple database backends (MySQL, PostgreSQL, etc.).
type DBAdapter interface {
	// DriverName returns the sql.Driver name used with sql.Open (e.g., "mysql", "postgres").
	DriverName() string

	// DSN builds the data-source-name string from a connection config.
	DSN(host string, port int, user, password, database, charset, sslMode string) string

	// DefaultPort returns the default TCP port for this database type.
	DefaultPort() int

	// QuoteIdentifier quotes a database identifier (table, column, schema name).
	QuoteIdentifier(name string) string

	// CurrentDB returns the name of the currently selected database.
	CurrentDB(db *sql.DB) (string, error)

	// CurrentSchema returns the name of the currently active schema.
	// For MySQL this is the same as CurrentDB; for PostgreSQL it returns
	// the current search_path schema (typically "public").
	CurrentSchema(db *sql.DB) (string, error)

	// UseDB switches to the specified database/schema on the given
	// connection (or pool). Note: on a pool this only affects one
	// connection; see UseDBViaDSN for pool-wide switching.
	UseDB(ctx context.Context, db Execer, name string) error

	// UseDBViaDSN reports whether the target database of UseDB can be baked
	// into the DSN, so that switching databases pool-wide is possible by
	// rebuilding the pool. MySQL: true ("USE db" ≡ DSN "/db").
	// PostgreSQL: false (UseDB maps to SET search_path, a session variable).
	UseDBViaDSN() bool

	// Databases returns a list of accessible databases/schemas.
	Databases(db *sql.DB) ([]string, error)

	// Tables returns a list of tables in the given database/schema.
	Tables(db *sql.DB, database string) ([]string, error)

	// Columns returns column metadata for a table.
	Columns(db *sql.DB, database, table string) ([]ColumnInfo, error)

	// ShowCreateTable returns the DDL for a table.
	ShowCreateTable(db *sql.DB, database, table string) (string, error)

	// ShowIndexes returns index information for a table.
	ShowIndexes(db *sql.DB, database, table string) ([]TableIndexInfo, error)

	// DescribeTableSQL returns the SQL to describe a table (for \desc columns mode).
	DescribeTableSQL(database, table string) string

	// DescribeFullSQL returns the SQL for \desc full mode.
	DescribeFullSQL(database, table string) string

	// Functions returns a list of built-in function names for this database type.
	Functions() []string

	// IsConnectionError checks if an error is a connection-level error.
	IsConnectionError(err error) bool

	// Schemas returns a list of schemas (MySQL: databases, PostgreSQL: schemas).
	Schemas(db *sql.DB) ([]string, error)

	// Users returns a list of database users.
	Users(db *sql.DB) ([]string, error)

	// Views returns a list of views in the given database/schema.
	Views(db *sql.DB, database string) ([]string, error)

	// ShowFunction returns the definition of a function.
	ShowFunction(db *sql.DB, database, function string) (string, error)

	// TablePrivileges returns privilege information for a table.
	TablePrivileges(db *sql.DB, database, table string) ([]string, error)

	// ListIndexes returns index information for a table (or all tables if table is empty).
	ListIndexes(db *sql.DB, database, table string) ([]TableIndexInfo, error)

	// SetEncoding sets the client character encoding.
	SetEncoding(db *sql.DB, encoding string) error

	// GetEncoding returns the current client character encoding.
	GetEncoding(db *sql.DB) (string, error)
}

// NewAdapter creates the appropriate DBAdapter based on the driver name.
// Supported values: "mysql" (default), "postgres" / "pg".
func NewAdapter(driver string) DBAdapter {
	switch driver {
	case "postgres", "pg":
		return &pgAdapter{}
	default:
		return &mysqlAdapter{}
	}
}
