package metadata

import (
	"fmt"
	"strings"
	"sync"

	"mysh/connection"
)

// ColumnInfo holds metadata about a database column.
type ColumnInfo struct {
	Name     string
	Type     string
	Nullable bool
	Key      string
	Default  string
	Extra    string
}

// Cache stores schema metadata (databases, tables, columns) for auto-completion.
type Cache struct {
	pool      *connection.Pool
	mu        sync.RWMutex
	databases []string
	tables    map[string][]string            // database -> table names
	columns   map[string]map[string][]string  // database -> table -> column names
	dirty     bool
}

// NewCache creates a new metadata cache and loads initial data.
func NewCache(pool *connection.Pool) (*Cache, error) {
	if pool == nil {
		return nil, fmt.Errorf("connection pool is nil")
	}

	c := &Cache{
		pool:    pool,
		tables:  make(map[string][]string),
		columns: make(map[string]map[string][]string),
		dirty:   true, // mark dirty so first completion triggers lazy load
	}

	return c, nil
}

// Refresh reloads all metadata from the database.
func (c *Cache) Refresh() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Load databases
	dbs, err := c.pool.Databases()
	if err != nil {
		return fmt.Errorf("failed to load databases: %w", err)
	}
	c.databases = dbs

	// Load tables for current database
	currentDB := c.pool.CurrentDB()
	if currentDB != "" {
		if err := c.loadDatabaseMetadata(currentDB); err != nil {
			// Log but don't fail
			c.dirty = true
		}
	}

	c.dirty = false
	return nil
}

// RefreshDatabase loads metadata for a specific database.
func (c *Cache) RefreshDatabase(dbName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loadDatabaseMetadata(dbName)
}

// loadDatabaseMetadata loads tables and columns for a database.
func (c *Cache) loadDatabaseMetadata(dbName string) error {
	tables, err := c.pool.Tables(dbName)
	if err != nil {
		return fmt.Errorf("failed to load tables for %s: %w", dbName, err)
	}
	c.tables[dbName] = tables

	// Load columns for each table
	if c.columns[dbName] == nil {
		c.columns[dbName] = make(map[string][]string)
	}

	for _, table := range tables {
		cols, err := c.pool.Columns(dbName, table)
		if err != nil {
			continue // Skip tables we can't read
		}
		colNames := make([]string, len(cols))
		for i, col := range cols {
			colNames[i] = col.Name
		}
		c.columns[dbName][table] = colNames
	}

	return nil
}

// MarkDirty marks the cache as needing refresh (e.g., after DDL).
func (c *Cache) MarkDirty() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dirty = true
}

// IsDirty returns whether the cache needs refreshing.
func (c *Cache) IsDirty() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.dirty
}

// Databases returns the cached list of database names.
func (c *Cache) Databases() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]string, len(c.databases))
	copy(result, c.databases)
	return result
}

// Tables returns all cached table names across all databases.
func (c *Cache) Tables() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []string
	for _, tables := range c.tables {
		result = append(result, tables...)
	}
	return result
}

// TablesByDB returns table names for a specific database.
func (c *Cache) TablesByDB(db string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	tables, ok := c.tables[db]
	if !ok {
		return nil
	}
	result := make([]string, len(tables))
	copy(result, tables)
	return result
}

// Columns returns the cached list of column names for a table across all databases.
// Table name matching is case-insensitive.
func (c *Cache) Columns(tableName string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	upper := strings.ToUpper(tableName)
	var result []string
	for _, tables := range c.columns {
		// Exact match first
		if cols, ok := tables[tableName]; ok {
			result = append(result, cols...)
			continue
		}
		// Case-insensitive fallback
		for t, cols := range tables {
			if strings.ToUpper(t) == upper {
				result = append(result, cols...)
				break
			}
		}
	}
	return result
}

// ColumnsByDB returns column names for a table in a specific database.
// Table name matching is case-insensitive.
func (c *Cache) ColumnsByDB(db, table string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	dbCols, ok := c.columns[db]
	if !ok {
		return nil
	}
	// Case-insensitive table name lookup
	cols, ok := dbCols[table]
	if !ok {
		cols = c.findTableCaseInsensitive(dbCols, table)
		if cols == nil {
			return nil
		}
	}
	result := make([]string, len(cols))
	copy(result, cols)
	return result
}

// ColumnsInfo returns ColumnInfo structs for a table in a specific database.
// Table name matching is case-insensitive.
func (c *Cache) ColumnsInfo(db, table string) []ColumnInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	dbCols, ok := c.columns[db]
	if !ok {
		return nil
	}
	colNames, ok := dbCols[table]
	if !ok {
		colNames = c.findTableCaseInsensitive(dbCols, table)
		if colNames == nil {
			return nil
		}
	}
	result := make([]ColumnInfo, len(colNames))
	for i, name := range colNames {
		result[i] = ColumnInfo{Name: name}
	}
	return result
}

// Functions returns a list of MySQL built-in function names.
func (c *Cache) Functions() []string {
	return mySQLFunctions
}

// TableNames returns all table names across all databases (for fuzzy matching).
func (c *Cache) TableNames() []string {
	return c.Tables()
}

// ResolveTableName returns the actual table name with correct casing.
// If no match is found, returns the input unchanged.
func (c *Cache) ResolveTableName(name string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	upper := strings.ToUpper(name)
	for _, tables := range c.tables {
		for _, t := range tables {
			if strings.ToUpper(t) == upper {
				return t
			}
		}
	}
	return name
}

// SearchTables returns table names matching the given prefix (case-insensitive).
func (c *Cache) SearchTables(prefix string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	upper := strings.ToUpper(prefix)
	var result []string
	for _, tables := range c.tables {
		for _, t := range tables {
			if strings.HasPrefix(strings.ToUpper(t), upper) {
				result = append(result, t)
			}
		}
	}
	return result
}

// SearchColumns returns column names matching the given prefix in any table (case-insensitive).
func (c *Cache) SearchColumns(prefix string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	upper := strings.ToUpper(prefix)
	var result []string
	for _, tables := range c.columns {
		for _, cols := range tables {
			for _, col := range cols {
				if strings.HasPrefix(strings.ToUpper(col), upper) {
					result = append(result, col)
				}
			}
		}
	}
	return result
}

// findTableCaseInsensitive looks up a table name case-insensitively in a column map.
func (c *Cache) findTableCaseInsensitive(colMap map[string][]string, table string) []string {
	upper := strings.ToUpper(table)
	for t, cols := range colMap {
		if strings.ToUpper(t) == upper {
			return cols
		}
	}
	return nil
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
