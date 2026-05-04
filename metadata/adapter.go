package metadata

import "mysh/completer"

// CacheAdapter wraps a Cache to satisfy the completer.MetadataCache interface.
// Usage: completerInstance.SetMetadata(metadata.NewCacheAdapter(cache))
type CacheAdapter struct {
	cache *Cache
}

// NewCacheAdapter creates an adapter from a Cache to completer.MetadataCache.
func NewCacheAdapter(cache *Cache) *CacheAdapter {
	return &CacheAdapter{cache: cache}
}

// Ensure CacheAdapter satisfies the completer.MetadataCache interface at compile time.
var _ completer.MetadataCache = (*CacheAdapter)(nil)

// Databases returns the list of cached database names.
func (a *CacheAdapter) Databases() []string {
	return a.cache.Databases()
}

// Tables returns table names for a specific database.
// If db is empty, returns all tables across all databases.
func (a *CacheAdapter) Tables(db string) []string {
	if db == "" {
		return a.cache.Tables()
	}
	return a.cache.TablesByDB(db)
}

// Columns returns column info for a table in a specific database.
func (a *CacheAdapter) Columns(db, table string) []completer.ColumnInfo {
	cols := a.cache.ColumnsInfo(db, table)
	result := make([]completer.ColumnInfo, len(cols))
	for i, col := range cols {
		result[i] = completer.ColumnInfo{
			Name:     col.Name,
			Type:     col.Type,
			Nullable: col.Nullable,
			Key:      col.Key,
			Default:  col.Default,
			Extra:    col.Extra,
		}
	}
	return result
}

// Functions returns a list of MySQL built-in function names.
func (a *CacheAdapter) Functions() []string {
	return a.cache.Functions()
}

// IsDirty returns whether the cache needs refreshing.
func (a *CacheAdapter) IsDirty() bool {
	return a.cache.IsDirty()
}
