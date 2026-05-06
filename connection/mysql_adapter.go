package connection

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// mysqlAdapter implements DBAdapter for MySQL / MariaDB.
type mysqlAdapter struct{}

func (mysqlAdapter) DriverName() string { return "mysql" }

func (mysqlAdapter) DefaultPort() int { return 3306 }

func (mysqlAdapter) DSN(host string, port int, user, password, database, charset string) string {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", user, password, host, port, database)
	if charset != "" {
		dsn += "?charset=" + charset
	}
	return dsn
}

func (mysqlAdapter) QuoteIdentifier(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func (mysqlAdapter) CurrentDB(db *sql.DB) (string, error) {
	var name string
	err := db.QueryRow("SELECT DATABASE()").Scan(&name)
	return name, err
}

func (mysqlAdapter) UseDB(ctx context.Context, db *sql.DB, name string) error {
	_, err := db.ExecContext(ctx, "USE `"+name+"`")
	return err
}

func (mysqlAdapter) Databases(db *sql.DB) ([]string, error) {
	rows, err := db.Query("SHOW DATABASES")
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

func (mysqlAdapter) Tables(db *sql.DB, database string) ([]string, error) {
	query := "SHOW TABLES"
	if database != "" {
		query = fmt.Sprintf("SHOW TABLES FROM `%s`", database)
	}

	rows, err := db.Query(query)
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

func (mysqlAdapter) Columns(db *sql.DB, database, table string) ([]ColumnInfo, error) {
	query := fmt.Sprintf("SHOW COLUMNS FROM `%s`", table)
	if database != "" {
		query = fmt.Sprintf("SHOW COLUMNS FROM `%s`.`%s`", database, table)
	}

	rows, err := db.Query(query)
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

func (mysqlAdapter) ShowCreateTable(db *sql.DB, database, table string) (string, error) {
	query := fmt.Sprintf("SHOW CREATE TABLE `%s`", table)
	if database != "" {
		query = fmt.Sprintf("SHOW CREATE TABLE `%s`.`%s`", database, table)
	}

	rows, err := db.Query(query)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	values := make([]sql.NullString, len(cols))
	scanArgs := make([]interface{}, len(cols))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	if rows.Next() {
		if err := rows.Scan(scanArgs...); err != nil {
			return "", err
		}
		if len(values) >= 2 && values[1].Valid {
			return values[1].String, nil
		}
	}
	return "", rows.Err()
}

func (mysqlAdapter) ShowIndexes(db *sql.DB, database, table string) ([]TableIndexInfo, error) {
	query := fmt.Sprintf("SHOW INDEX FROM `%s`", table)
	if database != "" {
		query = fmt.Sprintf("SHOW INDEX FROM `%s`.`%s`", database, table)
	}

	rows, err := db.Query(query)
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
			return showIndexesSimple(db, database, table)
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
func showIndexesSimple(db *sql.DB, database, table string) ([]TableIndexInfo, error) {
	query := fmt.Sprintf("SHOW INDEX FROM `%s`", table)
	if database != "" {
		query = fmt.Sprintf("SHOW INDEX FROM `%s`.`%s`", database, table)
	}

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexes []TableIndexInfo
	for rows.Next() {
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

func (mysqlAdapter) DescribeTableSQL(database, table string) string {
	if database != "" {
		return fmt.Sprintf("DESCRIBE `%s`.`%s`", database, table)
	}
	return fmt.Sprintf("DESCRIBE `%s`", table)
}

func (mysqlAdapter) DescribeFullSQL(database, table string) string {
	if database != "" {
		return fmt.Sprintf("SHOW FULL COLUMNS FROM `%s`.`%s`", database, table)
	}
	return fmt.Sprintf("SHOW FULL COLUMNS FROM `%s`", table)
}

// mySQLFunctions is a curated list of common MySQL functions.
var mySQLFunctions = []string{
	"COUNT", "SUM", "AVG", "MIN", "MAX", "GROUP_CONCAT",
	"CONCAT", "CONCAT_WS", "LENGTH", "CHAR_LENGTH", "SUBSTRING", "SUBSTR",
	"TRIM", "LTRIM", "RTRIM", "UPPER", "LOWER", "REPLACE", "REVERSE",
	"LEFT", "RIGHT", "LPAD", "RPAD", "REPEAT", "SPACE",
	"NOW", "CURDATE", "CURTIME", "DATE", "TIME", "YEAR", "MONTH", "DAY",
	"HOUR", "MINUTE", "SECOND", "DATE_ADD", "DATE_SUB", "DATEDIFF",
	"DATE_FORMAT", "STR_TO_DATE", "TIMESTAMPDIFF", "TIMESTAMPADD",
	"COALESCE", "IFNULL", "NULLIF", "IF", "CASE",
	"CAST", "CONVERT", "FORMAT", "ROUND", "FLOOR", "CEIL", "ABS",
	"MOD", "POWER", "SQRT", "RAND", "TRUNCATE",
	"DATABASE", "USER", "CURRENT_USER", "VERSION", "CONNECTION_ID",
	"LAST_INSERT_ID", "FOUND_ROWS", "ROW_COUNT",
	"INET_ATON", "INET_NTOA", "MD5", "SHA1", "SHA2",
	"JSON_ARRAY", "JSON_OBJECT", "JSON_EXTRACT", "JSON_CONTAINS",
	"JSON_UNQUOTE", "JSON_KEYS", "JSON_LENGTH", "JSON_TYPE",
}

func (mysqlAdapter) Functions() []string {
	return mySQLFunctions
}

func (mysqlAdapter) IsConnectionError(err error) bool {
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
