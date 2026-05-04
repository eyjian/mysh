package metadata

import (
	"testing"

	"mysh/completer"
)

// ---- CacheAdapter construction ----

func TestNewCacheAdapter(t *testing.T) {
	cache := &Cache{
		databases: []string{"db1"},
		tables: map[string][]string{
			"db1": {"users"},
		},
		columns: map[string]map[string][]string{
			"db1": {
				"users": {"id", "name"},
			},
		},
	}

	adapter := NewCacheAdapter(cache)
	if adapter == nil {
		t.Error("NewCacheAdapter should not return nil")
	}
}

// ---- CacheAdapter.Databases ----

func TestCacheAdapter_Databases(t *testing.T) {
	cache := &Cache{
		databases: []string{"db1", "db2"},
		tables:    make(map[string][]string),
		columns:   make(map[string]map[string][]string),
	}

	adapter := NewCacheAdapter(cache)
	dbs := adapter.Databases()
	if len(dbs) != 2 {
		t.Errorf("Databases() = %d, want 2", len(dbs))
	}
}

// ---- CacheAdapter.Tables ----

func TestCacheAdapter_Tables_AllDBs(t *testing.T) {
	cache := &Cache{
		tables: map[string][]string{
			"db1": {"users", "orders"},
			"db2": {"products"},
		},
		columns: make(map[string]map[string][]string),
	}

	adapter := NewCacheAdapter(cache)
	tables := adapter.Tables("")
	if len(tables) != 3 {
		t.Errorf("Tables('') = %d, want 3", len(tables))
	}
}

func TestCacheAdapter_Tables_SpecificDB(t *testing.T) {
	cache := &Cache{
		tables: map[string][]string{
			"db1": {"users", "orders"},
			"db2": {"products"},
		},
		columns: make(map[string]map[string][]string),
	}

	adapter := NewCacheAdapter(cache)
	tables := adapter.Tables("db1")
	if len(tables) != 2 {
		t.Errorf("Tables(db1) = %d, want 2", len(tables))
	}
}

// ---- CacheAdapter.Columns ----

func TestCacheAdapter_Columns(t *testing.T) {
	cache := &Cache{
		tables: make(map[string][]string),
		columns: map[string]map[string][]string{
			"db1": {
				"users": {"id", "name", "email"},
			},
		},
	}

	adapter := NewCacheAdapter(cache)
	cols := adapter.Columns("db1", "users")
	if len(cols) != 3 {
		t.Errorf("Columns(db1, users) = %d, want 3", len(cols))
	}
	for _, col := range cols {
		if col.Name == "" {
			t.Error("ColumnInfo.Name should not be empty")
		}
	}
}

func TestCacheAdapter_Columns_NonExistent(t *testing.T) {
	cache := &Cache{
		tables:  make(map[string][]string),
		columns: make(map[string]map[string][]string),
	}

	adapter := NewCacheAdapter(cache)
	cols := adapter.Columns("nodb", "notable")
	if len(cols) != 0 {
		t.Errorf("Columns(nodb, notable) = %d, want 0", len(cols))
	}
}

// ---- CacheAdapter.Functions ----

func TestCacheAdapter_Functions(t *testing.T) {
	cache := &Cache{
		tables:  make(map[string][]string),
		columns: make(map[string]map[string][]string),
	}

	adapter := NewCacheAdapter(cache)
	fns := adapter.Functions()
	if len(fns) == 0 {
		t.Error("Functions() should return non-empty list")
	}
}

// ---- CacheAdapter.IsDirty ----

func TestCacheAdapter_IsDirty(t *testing.T) {
	cache := &Cache{
		tables:  make(map[string][]string),
		columns: make(map[string]map[string][]string),
	}

	adapter := NewCacheAdapter(cache)
	if adapter.IsDirty() {
		t.Error("new cache should not be dirty")
	}

	cache.MarkDirty()
	if !adapter.IsDirty() {
		t.Error("cache should be dirty after MarkDirty")
	}
}

// ---- Compile-time interface check ----

func TestCacheAdapter_ImplementsInterface(t *testing.T) {
	cache := &Cache{
		tables:  make(map[string][]string),
		columns: make(map[string]map[string][]string),
	}
	var _ completer.MetadataCache = NewCacheAdapter(cache)
}
