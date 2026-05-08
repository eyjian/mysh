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

func (pgAdapter) DSN(host string, port int, user, password, database, charset, sslMode string) string {
	if sslMode == "" {
		sslMode = "disable"
	}
	// Build DSN with only non-empty fields. An empty value (e.g. `dbname=`) can
	// confuse lib/pq's key-value parser and cause subsequent keys like sslmode
	// to be silently dropped, falling back to the default `require`.
	parts := []string{}
	if host != "" {
		parts = append(parts, fmt.Sprintf("host=%s", host))
	}
	if port > 0 {
		parts = append(parts, fmt.Sprintf("port=%d", port))
	}
	if user != "" {
		parts = append(parts, fmt.Sprintf("user=%s", user))
	}
	if password != "" {
		parts = append(parts, fmt.Sprintf("password=%s", pgQuoteValue(password)))
	}
	if database != "" {
		parts = append(parts, fmt.Sprintf("dbname=%s", database))
	}
	parts = append(parts, fmt.Sprintf("sslmode=%s", sslMode))
	if charset != "" {
		parts = append(parts, "client_encoding="+charset)
	}
	return strings.Join(parts, " ")
}

// pgQuoteValue wraps a value in single quotes if it contains spaces or quote
// characters, escaping embedded quotes/backslashes per libpq rules.
func pgQuoteValue(s string) string {
	needQuote := false
	for _, c := range s {
		if c == ' ' || c == '\'' || c == '\\' {
			needQuote = true
			break
		}
	}
	if !needQuote {
		return s
	}
	var b strings.Builder
	b.WriteByte('\'')
	for _, c := range s {
		if c == '\'' || c == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(c)
	}
	b.WriteByte('\'')
	return b.String()
}

func (pgAdapter) QuoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (pgAdapter) CurrentDB(db *sql.DB) (string, error) {
	var name string
	err := db.QueryRow("SELECT current_database()").Scan(&name)
	return name, err
}

func (pgAdapter) CurrentSchema(db *sql.DB) (string, error) {
	var name string
	err := db.QueryRow("SELECT current_schema()").Scan(&name)
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

	// 1. Get column definitions
	colQuery := `SELECT a.attname,
	       pg_catalog.format_type(a.atttypid, a.atttypmod),
	       NOT a.attnotnull,
	       COALESCE(pg_catalog.pg_get_expr(d.adbin, d.adrelid), '')
	FROM pg_class c
	JOIN pg_namespace n ON n.oid = c.relnamespace
	JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
	LEFT JOIN pg_attrdef d ON d.adrelid = c.oid AND d.adnum = a.attnum
	WHERE n.nspname = $1 AND c.relname = $2 AND c.relkind = 'r'
	ORDER BY a.attnum`

	colRows, err := db.Query(colQuery, schema, table)
	if err != nil {
		return "", fmt.Errorf("failed to get columns: %w", err)
	}
	defer colRows.Close()

	type colDef struct {
		name     string
		typ      string
		nullable bool
		defaults string
	}
	var columns []colDef
	for colRows.Next() {
		var c colDef
		if err := colRows.Scan(&c.name, &c.typ, &c.nullable, &c.defaults); err != nil {
			return "", fmt.Errorf("failed to scan column: %w", err)
		}
		columns = append(columns, c)
	}
	if err := colRows.Err(); err != nil {
		return "", err
	}
	if len(columns) == 0 {
		return "", fmt.Errorf("table %s.%s not found", schema, table)
	}

	// 2. Get primary key columns
	pkSet := pgGetPKColumns(db, schema, table)        // set of PK column names
	pkOrder := pgGetPKColumnsOrdered(db, schema, table) // ordered PK columns
	isSinglePK := len(pkOrder) == 1

	// 3. Get indexes (unique + non-unique)
	indexes := pgGetIndexes(db, schema, table)

	// 4. Assemble DDL
	var b strings.Builder
	b.WriteString(fmt.Sprintf("CREATE TABLE %s.%s (", quotePGIdent(schema), quotePGIdent(table)))

	var lines []string
	for _, c := range columns {
		var line strings.Builder
		line.WriteString("  ")
		line.WriteString(quotePGIdent(c.name))
		line.WriteByte(' ')
		line.WriteString(c.typ)
		if !c.nullable {
			line.WriteString(" NOT NULL")
		}
		if c.defaults != "" {
			line.WriteString(" DEFAULT ")
			line.WriteString(c.defaults)
		}
		// Single-column PK: annotate inline
		if isSinglePK && pkSet[c.name] {
			line.WriteString(" PRIMARY KEY")
		}
		lines = append(lines, line.String())
	}

	// Composite PK: add separate constraint line
	if len(pkOrder) > 1 {
		quoted := make([]string, len(pkOrder))
		for i, col := range pkOrder {
			quoted[i] = quotePGIdent(col)
		}
		lines = append(lines, fmt.Sprintf("  CONSTRAINT %s PRIMARY KEY (%s)",
			quotePGIdent(table+"_pkey"), strings.Join(quoted, ", ")))
	}

	for i, line := range lines {
		b.WriteString("\n")
		b.WriteString(line)
		if i < len(lines)-1 {
			b.WriteByte(',')
		}
	}

	b.WriteString("\n)")

	// Add indexes as comments after the CREATE TABLE
	for _, idx := range indexes {
		b.WriteString("\n-- ")
		if idx.unique {
			b.WriteString("UNIQUE ")
		}
		b.WriteString(fmt.Sprintf("INDEX %s ON %s.%s (%s)", quotePGIdent(idx.name), quotePGIdent(schema), quotePGIdent(table), idx.columns))
	}

	b.WriteByte(';')
	return b.String(), nil
}

// quotePGIdent quotes a PostgreSQL identifier.
func quotePGIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// pgGetPKColumns returns a set of primary key column names.
func pgGetPKColumns(db *sql.DB, schema, table string) map[string]bool {
	rows, err := db.Query(`SELECT kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
		WHERE tc.constraint_type = 'PRIMARY KEY'
			AND tc.table_schema = $1 AND tc.table_name = $2
		ORDER BY kcu.ordinal_position`, schema, table)
	if err != nil {
		return nil
	}
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil
		}
		result[col] = true
	}
	return result
}

// pgGetPKColumnsOrdered returns primary key columns in order.
func pgGetPKColumnsOrdered(db *sql.DB, schema, table string) []string {
	rows, err := db.Query(`SELECT kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
		WHERE tc.constraint_type = 'PRIMARY KEY'
			AND tc.table_schema = $1 AND tc.table_name = $2
		ORDER BY kcu.ordinal_position`, schema, table)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil
		}
		cols = append(cols, col)
	}
	return cols
}

type pgIndexInfo struct {
	name    string
	columns string
	unique  bool
}

// pgGetIndexes returns index information for a table.
func pgGetIndexes(db *sql.DB, schema, table string) []pgIndexInfo {
	rows, err := db.Query(`SELECT i.relname,
	       string_agg(a.attname, ', ' ORDER BY array_position(ix.indkey, a.attnum)),
	       ix.indisunique
	FROM pg_class t
	JOIN pg_namespace n ON n.oid = t.relnamespace
	JOIN pg_index ix ON ix.indrelid = t.oid
	JOIN pg_class i ON i.oid = ix.indexrelid
	JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)
	WHERE n.nspname = $1 AND t.relname = $2
	GROUP BY i.relname, ix.indisunique
	ORDER BY i.relname`, schema, table)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var indexes []pgIndexInfo
	for rows.Next() {
		var idx pgIndexInfo
		if err := rows.Scan(&idx.name, &idx.columns, &idx.unique); err != nil {
			return nil
		}
		indexes = append(indexes, idx)
	}
	return indexes
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

func (pgAdapter) Schemas(db *sql.DB) ([]string, error) {
	rows, err := db.Query("SELECT schema_name FROM information_schema.schemata WHERE schema_name NOT IN ('information_schema', 'pg_catalog', 'pg_toast') ORDER BY schema_name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schemas []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		schemas = append(schemas, name)
	}
	return schemas, rows.Err()
}

func (pgAdapter) Users(db *sql.DB) ([]string, error) {
	rows, err := db.Query("SELECT usename FROM pg_user ORDER BY usename")
	if err != nil {
		// Fallback: just show current user
		var user string
		if err := db.QueryRow("SELECT current_user").Scan(&user); err != nil {
			return nil, err
		}
		return []string{user}, nil
	}
	defer rows.Close()

	var users []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		users = append(users, name)
	}
	return users, rows.Err()
}

func (pgAdapter) Views(db *sql.DB, schema string) ([]string, error) {
	if schema == "" {
		schema = "public"
	}
	rows, err := db.Query("SELECT viewname FROM pg_views WHERE schemaname = $1 ORDER BY viewname", schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var views []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		views = append(views, name)
	}
	return views, rows.Err()
}

func (pgAdapter) ShowFunction(db *sql.DB, schema, function string) (string, error) {
	if schema == "" {
		schema = "public"
	}
	var def string
	err := db.QueryRow("SELECT pg_get_functiondef(oid) FROM pg_proc WHERE proname = $1 AND pronamespace = (SELECT oid FROM pg_namespace WHERE nspname = $2)", function, schema).Scan(&def)
	if err != nil {
		return "", fmt.Errorf("function %s not found: %w", function, err)
	}
	return def, nil
}

func (pgAdapter) TablePrivileges(db *sql.DB, schema, table string) ([]string, error) {
	if schema == "" {
		schema = "public"
	}
	rows, err := db.Query(`SELECT grantee, string_agg(privilege_type, ', ' ORDER BY privilege_type) as privileges
		FROM information_schema.role_table_grants
		WHERE table_schema = $1 AND table_name = $2
		GROUP BY grantee ORDER BY grantee`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var privileges []string
	for rows.Next() {
		var grantee, privs string
		if err := rows.Scan(&grantee, &privs); err != nil {
			return nil, err
		}
		privileges = append(privileges, fmt.Sprintf("%s: %s", grantee, privs))
	}
	return privileges, rows.Err()
}

func (pgAdapter) ListIndexes(db *sql.DB, schema, table string) ([]TableIndexInfo, error) {
	if schema == "" {
		schema = "public"
	}
	var query string
	var args []interface{}
	if table != "" {
		query = `SELECT i.relname as index_name,
		                a.attname as column_name,
		                NOT ix.indisunique as non_unique,
		                am.amname as index_type,
		                t.relname as table_name
		         FROM pg_class t
		         JOIN pg_namespace n ON n.oid = t.relnamespace
		         JOIN pg_index ix ON ix.indrelid = t.oid
		         JOIN pg_class i ON i.oid = ix.indexrelid
		         JOIN pg_am am ON am.oid = i.relam
		         JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)
		         WHERE n.nspname = $1 AND t.relname = $2
		         ORDER BY i.relname, a.attnum`
		args = []interface{}{schema, table}
	} else {
		query = `SELECT i.relname as index_name,
		                a.attname as column_name,
		                NOT ix.indisunique as non_unique,
		                am.amname as index_type,
		                t.relname as table_name
		         FROM pg_class t
		         JOIN pg_namespace n ON n.oid = t.relnamespace
		         JOIN pg_index ix ON ix.indrelid = t.oid
		         JOIN pg_class i ON i.oid = ix.indexrelid
		         JOIN pg_am am ON am.oid = i.relam
		         JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)
		         WHERE n.nspname = $1
		         ORDER BY t.relname, i.relname, a.attnum`
		args = []interface{}{schema}
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexes []TableIndexInfo
	for rows.Next() {
		var name, colName, idxType, tblName string
		var nonUnique bool
		if err := rows.Scan(&name, &colName, &nonUnique, &idxType, &tblName); err != nil {
			return nil, err
		}
		indexes = append(indexes, TableIndexInfo{
			Name:      tblName + "." + name,
			Columns:   colName,
			NonUnique: nonUnique,
			Type:      idxType,
		})
	}
	return indexes, rows.Err()
}

func (pgAdapter) SetEncoding(db *sql.DB, encoding string) error {
	_, err := db.Exec(fmt.Sprintf("SET client_encoding = '%s'", encoding))
	return err
}

func (pgAdapter) GetEncoding(db *sql.DB) (string, error) {
	var value string
	err := db.QueryRow("SELECT current_setting('client_encoding')").Scan(&value)
	return value, err
}
