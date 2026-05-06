package connection

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// pgAdapter implements DBAdapter for PostgreSQL.
type pgAdapter struct{}

func (pgAdapter) DriverName() string { return "postgres" }

func (pgAdapter) DefaultPort() int { return 5432 }

func (pgAdapter) DSN(host string, port int, user, password, database, charset string) string {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, database)
	if charset != "" {
		dsn += " client_encoding=" + charset
	}
	return dsn
}

func (pgAdapter) QuoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (pgAdapter) CurrentDB(db *sql.DB) (string, error) {
	var name string
	err := db.QueryRow("SELECT current_database()").Scan(&name)
	return name, err
}

func (pgAdapter) UseDB(ctx context.Context, db *sql.DB, name string) error {
	_, err := db.ExecContext(ctx, `SET search_path TO "`+strings.ReplaceAll(name, `"`, `""`)+`"`)
	return err
}

func (pgAdapter) Databases(db *sql.DB) ([]string, error) {
	rows, err := db.Query("SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname")
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

func (pgAdapter) Tables(db *sql.DB, schema string) ([]string, error) {
	query := "SELECT tablename FROM pg_tables WHERE schemaname = $1 ORDER BY tablename"
	if schema == "" {
		schema = "public"
	}

	rows, err := db.Query(query, schema)
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

func (pgAdapter) Columns(db *sql.DB, schema, table string) ([]ColumnInfo, error) {
	if schema == "" {
		schema = "public"
	}
	query := `SELECT c.column_name, c.data_type,
	                 CASE WHEN c.is_nullable = 'YES' THEN true ELSE false END,
	                 COALESCE(c.column_default, '') as column_default
	          FROM information_schema.columns c
	          WHERE c.table_schema = $1 AND c.table_name = $2
	          ORDER BY c.ordinal_position`

	rows, err := db.Query(query, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []ColumnInfo
	for rows.Next() {
		var name, typ, def string
		var nullable bool
		if err := rows.Scan(&name, &typ, &nullable, &def); err != nil {
			return nil, err
		}
		key := ""
		// Check if this is a primary key column
		if isPrimaryKey(db, schema, table, name) {
			key = "PRI"
		}
		cols = append(cols, ColumnInfo{
			Name:     name,
			Type:     typ,
			Nullable: nullable,
			Key:      key,
		})
	}
	return cols, rows.Err()
}

// isPrimaryKey checks if a column is part of the primary key.
func isPrimaryKey(db *sql.DB, schema, table, column string) bool {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
		WHERE tc.constraint_type = 'PRIMARY KEY'
			AND tc.table_schema = $1 AND tc.table_name = $2 AND kcu.column_name = $3`,
		schema, table, column).Scan(&count)
	return err == nil && count > 0
}

func (pgAdapter) ShowCreateTable(db *sql.DB, schema, table string) (string, error) {
	if schema == "" {
		schema = "public"
	}
	// PostgreSQL has no direct SHOW CREATE TABLE; reconstruct from pg_catalog.
	query := `SELECT
	'CREATE TABLE ' || quote_ident(c.relname) || ' (' ||
	string_agg(
		quote_ident(a.attname) || ' ' || pg_catalog.format_type(a.atttypid, a.atttypmod) ||
		CASE WHEN a.attnotnull THEN ' NOT NULL' ELSE '' END ||
		CASE WHEN pg_catalog.pg_get_expr(d.adbin, d.adrelid) IS NOT NULL
			THEN ' DEFAULT ' || pg_catalog.pg_get_expr(d.adbin, d.adrelid)
			ELSE '' END,
		E',\n '
	) || E');'
	FROM pg_class c
	JOIN pg_namespace n ON n.oid = c.relnamespace
	JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
	LEFT JOIN pg_attrdef d ON d.adrelid = c.oid AND d.adnum = a.attnum
	WHERE n.nspname = $1 AND c.relname = $2 AND c.relkind = 'r'
	GROUP BY c.relname`

	var ddl string
	err := db.QueryRow(query, schema, table).Scan(&ddl)
	if err != nil {
		return "", fmt.Errorf("failed to get table DDL: %w", err)
	}
	return ddl, nil
}

func (pgAdapter) ShowIndexes(db *sql.DB, schema, table string) ([]TableIndexInfo, error) {
	if schema == "" {
		schema = "public"
	}
	query := `SELECT i.relname as index_name,
	                  a.attname as column_name,
	                  NOT ix.indisunique as non_unique,
	                  am.amname as index_type
	           FROM pg_class t
	           JOIN pg_namespace n ON n.oid = t.relnamespace
	           JOIN pg_index ix ON ix.indrelid = t.oid
	           JOIN pg_class i ON i.oid = ix.indexrelid
	           JOIN pg_am am ON am.oid = i.relam
	           JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)
	           WHERE n.nspname = $1 AND t.relname = $2
	           ORDER BY i.relname, a.attnum`

	rows, err := db.Query(query, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexes []TableIndexInfo
	for rows.Next() {
		var name, colName, idxType string
		var nonUnique bool
		if err := rows.Scan(&name, &colName, &nonUnique, &idxType); err != nil {
			return nil, err
		}
		indexes = append(indexes, TableIndexInfo{
			Name:      name,
			Columns:   colName,
			NonUnique: nonUnique,
			Type:      idxType,
		})
	}
	return indexes, rows.Err()
}

func (pgAdapter) DescribeTableSQL(schema, table string) string {
	if schema != "" {
		return fmt.Sprintf(`SELECT column_name AS Field, data_type AS Type,
       CASE WHEN is_nullable = 'YES' THEN 'YES' ELSE 'NO' END AS Null,
       CASE WHEN EXISTS (
           SELECT 1 FROM information_schema.table_constraints tc
           JOIN information_schema.key_column_usage kcu
               ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
           WHERE tc.constraint_type = 'PRIMARY KEY'
               AND tc.table_schema = '%s' AND tc.table_name = '%s' AND kcu.column_name = columns.column_name
       ) THEN 'PRI' ELSE '' END AS Key,
       column_default AS Default
FROM information_schema.columns
WHERE table_schema = '%s' AND table_name = '%s'
ORDER BY ordinal_position`, schema, table, schema, table)
	}
	return fmt.Sprintf(`SELECT column_name AS Field, data_type AS Type,
       CASE WHEN is_nullable = 'YES' THEN 'YES' ELSE 'NO' END AS Null,
       CASE WHEN EXISTS (
           SELECT 1 FROM information_schema.table_constraints tc
           JOIN information_schema.key_column_usage kcu
               ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
           WHERE tc.constraint_type = 'PRIMARY KEY'
               AND tc.table_schema = 'public' AND tc.table_name = '%s' AND kcu.column_name = columns.column_name
       ) THEN 'PRI' ELSE '' END AS Key,
       column_default AS Default
FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = '%s'
ORDER BY ordinal_position`, table, table)
}

func (pgAdapter) DescribeFullSQL(schema, table string) string {
	// For PG, the "full" describe is similar to the regular describe with extra info
	if schema != "" {
		return fmt.Sprintf(`SELECT column_name AS Field, data_type AS Type,
       collation_name AS Collation,
       CASE WHEN is_nullable = 'YES' THEN 'YES' ELSE 'NO' END AS Null,
       CASE WHEN EXISTS (
           SELECT 1 FROM information_schema.table_constraints tc
           JOIN information_schema.key_column_usage kcu
               ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
           WHERE tc.constraint_type = 'PRIMARY KEY'
               AND tc.table_schema = '%s' AND tc.table_name = '%s' AND kcu.column_name = columns.column_name
       ) THEN 'PRI' ELSE '' END AS Key,
       column_default AS Default,
       '' AS Extra,
       '' AS Privileges,
       '' AS Comment
FROM information_schema.columns
WHERE table_schema = '%s' AND table_name = '%s'
ORDER BY ordinal_position`, schema, table, schema, table)
	}
	return fmt.Sprintf(`SELECT column_name AS Field, data_type AS Type,
       collation_name AS Collation,
       CASE WHEN is_nullable = 'YES' THEN 'YES' ELSE 'NO' END AS Null,
       CASE WHEN EXISTS (
           SELECT 1 FROM information_schema.table_constraints tc
           JOIN information_schema.key_column_usage kcu
               ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
           WHERE tc.constraint_type = 'PRIMARY KEY'
               AND tc.table_schema = 'public' AND tc.table_name = '%s' AND kcu.column_name = columns.column_name
       ) THEN 'PRI' ELSE '' END AS Key,
       column_default AS Default,
       '' AS Extra,
       '' AS Privileges,
       '' AS Comment
FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = '%s'
ORDER BY ordinal_position`, table, table)
}

// pgFunctions is a curated list of common PostgreSQL functions.
var pgFunctions = []string{
	"COUNT", "SUM", "AVG", "MIN", "MAX", "STRING_AGG", "ARRAY_AGG",
	"CONCAT", "LENGTH", "SUBSTRING", "TRIM", "LTRIM", "RTRIM",
	"UPPER", "LOWER", "REPLACE", "REVERSE", "LEFT", "RIGHT",
	"LPAD", "RPAD", "REPEAT", "OVERLAY", "POSITION",
	"NOW", "CURRENT_DATE", "CURRENT_TIME", "CURRENT_TIMESTAMP",
	"DATE", "TIME", "EXTRACT", "DATE_TRUNC", "AGE",
	"TO_CHAR", "TO_DATE", "TO_TIMESTAMP",
	"COALESCE", "NULLIF", "GREATEST", "LEAST",
	"CAST", "::", "ROUND", "FLOOR", "CEIL", "ABS",
	"MOD", "POWER", "SQRT", "RANDOM", "TRUNC",
	"CURRENT_DATABASE", "CURRENT_USER", "SESSION_USER", "VERSION",
	"NEXTVAL", "CURRVAL", "LASTVAL",
	"MD5", "ENCODE", "DECODE",
	"ROW_NUMBER", "RANK", "DENSE_RANK", "LAG", "LEAD",
	"JSON_BUILD_OBJECT", "JSON_BUILD_ARRAY", "JSONB_BUILD_OBJECT", "JSONB_BUILD_ARRAY",
	"JSON_EXTRACT_PATH", "JSONB_EXTRACT_PATH", "JSONB_PRETTY",
	"TO_JSONB", "JSONB_AGG", "JSON_AGG",
	"GENERATE_SERIES", "UNNEST", "ARRAY",
}

func (pgAdapter) Functions() []string {
	return pgFunctions
}

func (pgAdapter) IsConnectionError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "connect: connection refused") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "driver: bad conn") ||
		strings.Contains(msg, "conn closed") ||
		strings.Contains(msg, "connection unexpectedly closed") ||
		strings.Contains(msg, "no such host")
}
