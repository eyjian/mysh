package metadata

import (
	"testing"
)

// ---- Cache construction with nil pool ----

func TestNewCache_NilPool(t *testing.T) {
	_, err := NewCache(nil)
	if err == nil {
		t.Error("NewCache(nil) should return error")
	}
}

// ---- MarkDirty / IsDirty ----

func TestCache_MarkDirty_IsDirty(t *testing.T) {
	c := &Cache{
		tables:  make(map[string][]string),
		columns: make(map[string]map[string][]string),
	}

	if c.IsDirty() {
		t.Error("new cache should not be dirty")
	}

	c.MarkDirty()
	if !c.IsDirty() {
		t.Error("cache should be dirty after MarkDirty()")
	}
}

// ---- Databases ----

func TestCache_Databases(t *testing.T) {
	c := &Cache{
		databases: []string{"db1", "db2", "db3"},
		tables:    make(map[string][]string),
		columns:   make(map[string]map[string][]string),
	}

	dbs := c.Databases()
	if len(dbs) != 3 {
		t.Errorf("Databases() = %d, want 3", len(dbs))
	}

	// Should return a copy
	dbs[0] = "modified"
	if c.Databases()[0] == "modified" {
		t.Error("Databases() should return a copy")
	}
}

// ---- Tables ----

func TestCache_Tables(t *testing.T) {
	c := &Cache{
		tables: map[string][]string{
			"db1": {"users", "orders"},
			"db2": {"products"},
		},
		columns: make(map[string]map[string][]string),
	}

	tables := c.Tables()
	if len(tables) != 3 {
		t.Errorf("Tables() = %d, want 3", len(tables))
	}
}

func TestCache_TablesByDB(t *testing.T) {
	c := &Cache{
		tables: map[string][]string{
			"db1": {"users", "orders"},
		},
		columns: make(map[string]map[string][]string),
	}

	tables := c.TablesByDB("db1")
	if len(tables) != 2 {
		t.Errorf("TablesByDB(db1) = %d, want 2", len(tables))
	}

	tables = c.TablesByDB("nonexistent")
	if tables != nil {
		t.Errorf("TablesByDB(nonexistent) = %v, want nil", tables)
	}
}

// ---- Columns ----

func TestCache_Columns(t *testing.T) {
	c := &Cache{
		tables: make(map[string][]string),
		columns: map[string]map[string][]string{
			"db1": {
				"users": {"id", "name", "email"},
			},
		},
	}

	cols := c.Columns("users")
	if len(cols) != 3 {
		t.Errorf("Columns(users) = %d, want 3", len(cols))
	}
}

func TestCache_ColumnsByDB(t *testing.T) {
	c := &Cache{
		tables: make(map[string][]string),
		columns: map[string]map[string][]string{
			"db1": {
				"users": {"id", "name"},
			},
		},
	}

	cols := c.ColumnsByDB("db1", "users")
	if len(cols) != 2 {
		t.Errorf("ColumnsByDB(db1, users) = %d, want 2", len(cols))
	}

	cols = c.ColumnsByDB("db1", "nonexistent")
	if cols != nil {
		t.Errorf("ColumnsByDB(db1, nonexistent) = %v, want nil", cols)
	}

	cols = c.ColumnsByDB("nonexistent", "users")
	if cols != nil {
		t.Errorf("ColumnsByDB(nonexistent, users) = %v, want nil", cols)
	}
}

// ---- SearchTables ----

func TestCache_SearchTables(t *testing.T) {
	c := &Cache{
		tables: map[string][]string{
			"db1": {"users", "user_details", "orders"},
		},
		columns: make(map[string]map[string][]string),
	}

	results := c.SearchTables("US")
	if len(results) != 2 {
		t.Errorf("SearchTables(US) = %d, want 2", len(results))
	}

	results = c.SearchTables("ORD")
	if len(results) != 1 {
		t.Errorf("SearchTables(ORD) = %d, want 1", len(results))
	}

	results = c.SearchTables("NONEXISTENT")
	if len(results) != 0 {
		t.Errorf("SearchTables(NONEXISTENT) = %d, want 0", len(results))
	}
}

// ---- SearchColumns ----

func TestCache_SearchColumns(t *testing.T) {
	c := &Cache{
		tables: make(map[string][]string),
		columns: map[string]map[string][]string{
			"db1": {
				"users":  {"id", "name", "email"},
				"orders": {"id", "user_id"},
			},
		},
	}

	results := c.SearchColumns("ID")
	if len(results) < 2 {
		t.Errorf("SearchColumns(ID) = %d, want >= 2 (id, user_id)", len(results))
	}

	results = c.SearchColumns("NAME")
	if len(results) != 1 {
		t.Errorf("SearchColumns(NAME) = %d, want 1", len(results))
	}
}

// ---- TableNames ----

func TestCache_TableNames(t *testing.T) {
	c := &Cache{
		tables: map[string][]string{
			"db1": {"t1", "t2"},
		},
		columns: make(map[string]map[string][]string),
	}

	names := c.TableNames()
	if len(names) != 2 {
		t.Errorf("TableNames() = %d, want 2", len(names))
	}
}

// ---- ColumnsInfo ----

func TestCache_ColumnsInfo(t *testing.T) {
	c := &Cache{
		tables: make(map[string][]string),
		columns: map[string]map[string][]string{
			"db1": {
				"users": {"id", "name", "email"},
			},
		},
	}

	cols := c.ColumnsInfo("db1", "users")
	if len(cols) != 3 {
		t.Errorf("ColumnsInfo(db1, users) = %d, want 3", len(cols))
	}
	if cols[0].Name != "id" {
		t.Errorf("ColumnsInfo[0].Name = %q, want %q", cols[0].Name, "id")
	}
}

func TestCache_ColumnsInfo_NonExistentDB(t *testing.T) {
	c := &Cache{
		tables:  make(map[string][]string),
		columns: make(map[string]map[string][]string),
	}

	cols := c.ColumnsInfo("nodb", "users")
	if cols != nil {
		t.Errorf("ColumnsInfo(nodb, users) = %v, want nil", cols)
	}
}

func TestCache_ColumnsInfo_NonExistentTable(t *testing.T) {
	c := &Cache{
		tables: make(map[string][]string),
		columns: map[string]map[string][]string{
			"db1": {},
		},
	}

	cols := c.ColumnsInfo("db1", "notable")
	if cols != nil {
		t.Errorf("ColumnsInfo(db1, notable) = %v, want nil", cols)
	}
}

// ---- Functions ----

func TestCache_Functions(t *testing.T) {
	c := &Cache{
		tables:  make(map[string][]string),
		columns: make(map[string]map[string][]string),
	}

	fns := c.Functions()
	if len(fns) == 0 {
		t.Error("Functions() should return non-empty list")
	}
	// Check a few known functions
	found := false
	for _, fn := range fns {
		if fn == "COUNT" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Functions() should contain COUNT")
	}
}

// ---- TablesByDB returns copy ----

func TestCache_TablesByDB_ReturnsCopy(t *testing.T) {
	c := &Cache{
		tables: map[string][]string{
			"db1": {"users", "orders"},
		},
		columns: make(map[string]map[string][]string),
	}

	tables := c.TablesByDB("db1")
	tables[0] = "modified"
	if c.TablesByDB("db1")[0] == "modified" {
		t.Error("TablesByDB should return a copy")
	}
}

// ---- ColumnsByDB returns copy ----

func TestCache_ColumnsByDB_ReturnsCopy(t *testing.T) {
	c := &Cache{
		tables: make(map[string][]string),
		columns: map[string]map[string][]string{
			"db1": {
				"users": {"id", "name"},
			},
		},
	}

	cols := c.ColumnsByDB("db1", "users")
	cols[0] = "modified"
	if c.ColumnsByDB("db1", "users")[0] == "modified" {
		t.Error("ColumnsByDB should return a copy")
	}
}

// ---- Empty cache ----

func TestCache_EmptyCache(t *testing.T) {
	c := &Cache{
		tables:  make(map[string][]string),
		columns: make(map[string]map[string][]string),
	}

	if len(c.Databases()) != 0 {
		t.Error("empty cache should have no databases")
	}
	if len(c.Tables()) != 0 {
		t.Error("empty cache should have no tables")
	}
	if len(c.Columns("any")) != 0 {
		t.Error("empty cache should have no columns")
	}
	if len(c.SearchTables("a")) != 0 {
		t.Error("empty cache search should return empty")
	}
	if len(c.SearchColumns("a")) != 0 {
		t.Error("empty cache search columns should return empty")
	}
}

// ---- SearchColumns no match ----

func TestCache_SearchColumns_NoMatch(t *testing.T) {
	c := &Cache{
		tables: make(map[string][]string),
		columns: map[string]map[string][]string{
			"db1": {
				"users": {"id", "name"},
			},
		},
	}

	results := c.SearchColumns("ZZZ")
	if len(results) != 0 {
		t.Errorf("SearchColumns(ZZZ) = %d, want 0", len(results))
	}
}
