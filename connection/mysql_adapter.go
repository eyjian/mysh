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

func (mysqlAdapter) DSN(host string, port int, user, password, database, charset, sslMode string) string {
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

func (mysqlAdapter) CurrentSchema(db *sql.DB) (string, error) {
	// In MySQL, schemas are databases.
	return mysqlAdapter{}.CurrentDB(db)
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

func (mysqlAdapter) Schemas(db *sql.DB) ([]string, error) {
	// In MySQL, schemas are databases
	return mysqlAdapter{}.Databases(db)
}

func (mysqlAdapter) Users(db *sql.DB) ([]string, error) {
	rows, err := db.Query("SELECT User FROM mysql.user ORDER BY User")
	if err != nil {
		// May lack permission; try alternative
		rows, err = db.Query("SELECT CURRENT_USER()")
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var user string
		if rows.Next() {
			if err := rows.Scan(&user); err != nil {
				return nil, err
			}
		}
		return []string{user}, rows.Err()
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

func (mysqlAdapter) Views(db *sql.DB, database string) ([]string, error) {
	query := "SHOW FULL TABLES WHERE Table_type = 'VIEW'"
	if database != "" {
		query = fmt.Sprintf("SHOW FULL TABLES FROM `%s` WHERE Table_type = 'VIEW'", database)
	}
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var views []string
	for rows.Next() {
		var name, typ string
		if err := rows.Scan(&name, &typ); err != nil {
			return nil, err
		}
		views = append(views, name)
	}
	return views, rows.Err()
}

func (mysqlAdapter) ShowFunction(db *sql.DB, database, function string) (string, error) {
	query := fmt.Sprintf("SHOW CREATE FUNCTION `%s`", function)
	if database != "" {
		query = fmt.Sprintf("SHOW CREATE FUNCTION `%s`.`%s`", database, function)
	}
	rows, err := db.Query(query)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	if rows.Next() {
		cols, _ := rows.Columns()
		values := make([]sql.NullString, len(cols))
		scanArgs := make([]interface{}, len(cols))
		for i := range values {
			scanArgs[i] = &values[i]
		}
		if err := rows.Scan(scanArgs...); err != nil {
			return "", err
		}
		// The function body is typically in the 2nd or 3rd column
		for i := len(values) - 1; i >= 0; i-- {
			if values[i].Valid && len(values[i].String) > 20 {
				return values[i].String, nil
			}
		}
		// Fallback: return whatever we have
		for _, v := range values {
			if v.Valid {
				return v.String, nil
			}
		}
	}
	return "", fmt.Errorf("function %s not found", function)
}

func (mysqlAdapter) TablePrivileges(db *sql.DB, database, table string) ([]string, error) {
	query := fmt.Sprintf("SHOW GRANTS")
	if database != "" && table != "" {
		// MySQL doesn't have SHOW GRANTS for specific table, query information_schema
		query = fmt.Sprintf(
			"SELECT GRANTEE, PRIVILEGE_TYPE, IS_GRANTABLE FROM information_schema.TABLE_PRIVILEGES WHERE TABLE_SCHEMA = '%s' AND TABLE_NAME = '%s' ORDER BY GRANTEE, PRIVILEGE_TYPE",
			database, table)
	}
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var privileges []string
	for rows.Next() {
		cols, _ := rows.Columns()
		if len(cols) == 1 {
			var grant string
			if err := rows.Scan(&grant); err != nil {
				return nil, err
			}
			privileges = append(privileges, grant)
		} else {
			// information_schema format
			var grantee, priv, grantable string
			if err := rows.Scan(&grantee, &priv, &grantable); err != nil {
				return nil, err
			}
			g := "N"
			if grantable == "YES" {
				g = "Y"
			}
			privileges = append(privileges, fmt.Sprintf("%s: %s (grantable: %s)", grantee, priv, g))
		}
	}
	return privileges, rows.Err()
}

func (mysqlAdapter) ListIndexes(db *sql.DB, database, table string) ([]TableIndexInfo, error) {
	if table != "" {
		return showIndexesForTable(db, database, table)
	}
	// No table specified: list indexes for all tables
	tables, err := mysqlAdapter{}.Tables(db, database)
	if err != nil {
		return nil, err
	}
	var allIndexes []TableIndexInfo
	for _, t := range tables {
		idxs, err := showIndexesForTable(db, database, t)
		if err != nil {
			continue
		}
		allIndexes = append(allIndexes, idxs...)
	}
	return allIndexes, nil
}

// showIndexesForTable returns index info for a single table.
func showIndexesForTable(db *sql.DB, database, table string) ([]TableIndexInfo, error) {
	query := fmt.Sprintf("SHOW INDEX FROM `%s`", table)
	if database != "" {
		query = fmt.Sprintf("SHOW INDEX FROM `%s`.`%s`", database, table)
	}
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Use dynamic column scanning
	cols, _ := rows.Columns()
	values := make([]interface{}, len(cols))
	for i := range values {
		values[i] = new(sql.NullString)
	}

	var indexes []TableIndexInfo
	for rows.Next() {
		if err := rows.Scan(values...); err != nil {
			return nil, err
		}
		nonUnique := "0"
		keyName := ""
		colName := ""
		idxType := ""
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
				idxType = v.String
			}
		}
		indexes = append(indexes, TableIndexInfo{
			Name:      keyName,
			Columns:   colName,
			NonUnique: nonUnique == "1",
			Type:      idxType,
		})
	}
	return indexes, rows.Err()
}

func (mysqlAdapter) SetEncoding(db *sql.DB, encoding string) error {
	_, err := db.Exec(fmt.Sprintf("SET NAMES '%s'", encoding))
	return err
}

func (mysqlAdapter) GetEncoding(db *sql.DB) (string, error) {
	var value string
	err := db.QueryRow("SELECT @@character_set_client").Scan(&value)
	return value, err
}
