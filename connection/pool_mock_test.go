package connection

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"mysh/config"
)

// newMockPool creates a Pool backed by sqlmock with ping monitoring enabled.
func newMockPool(t *testing.T) (*Pool, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	cfg := &config.ConnectionConfig{
		Host:     "localhost",
		Port:     3306,
		User:     "root",
		Password: "",
		Database: "testdb",
	}
	pool := NewWithDB(db, cfg)
	return pool, mock
}

// closePool safely closes the pool and verifies all mock expectations.
func closePool(t *testing.T, pool *Pool, mock sqlmock.Sqlmock) {
	t.Helper()
	mock.ExpectClose()
	pool.Close()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// --- Close ---

func TestPool_Close(t *testing.T) {
	pool, mock := newMockPool(t)
	mock.ExpectClose()
	if err := pool.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
	if !pool.IsClosed() {
		t.Error("IsClosed() should be true after Close()")
	}
	// Double close should not error
	if err := pool.Close(); err != nil {
		t.Errorf("double Close() error = %v", err)
	}
}

// --- Ping ---

func TestPool_Ping(t *testing.T) {
	pool, mock := newMockPool(t)

	mock.ExpectPing()
	if err := pool.Ping(context.Background()); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
	closePool(t, pool, mock)
}

func TestPool_Ping_Error(t *testing.T) {
	pool, mock := newMockPool(t)

	mock.ExpectPing().WillReturnError(errors.New("ping failed"))
	if err := pool.Ping(context.Background()); err == nil {
		t.Error("Ping() should return error")
	}
	closePool(t, pool, mock)
}

// --- IsConnected ---

func TestPool_IsConnected(t *testing.T) {
	pool, mock := newMockPool(t)

	mock.ExpectPing()
	if !pool.IsConnected() {
		t.Error("IsConnected() should be true")
	}
	closePool(t, pool, mock)
}

func TestPool_IsConnected_Closed(t *testing.T) {
	pool, _ := newMockPool(t)
	pool.closed = true
	if pool.IsConnected() {
		t.Error("IsConnected() should be false after close")
	}
}

func TestPool_IsConnected_PingFails(t *testing.T) {
	pool, mock := newMockPool(t)

	mock.ExpectPing().WillReturnError(errors.New("down"))
	if pool.IsConnected() {
		t.Error("IsConnected() should be false when ping fails")
	}
	closePool(t, pool, mock)
}

// --- Query ---

func TestPool_Query(t *testing.T) {
	pool, mock := newMockPool(t)

	rows := sqlmock.NewRows([]string{"id", "name"}).
		AddRow(1, "alice").
		AddRow(2, "bob")
	mock.ExpectQuery("SELECT \\* FROM users").WillReturnRows(rows)

	result, err := pool.Query("SELECT * FROM users")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	var id int
	var name string
	if !result.Next() {
		t.Fatal("expected row")
	}
	if err := result.Scan(&id, &name); err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if id != 1 || name != "alice" {
		t.Errorf("got (%d, %s), want (1, alice)", id, name)
	}
	result.Close() // close before pool.Close to satisfy mock expectations
	closePool(t, pool, mock)
}

func TestPool_Query_Error(t *testing.T) {
	pool, mock := newMockPool(t)

	mock.ExpectQuery("SELECT 1").WillReturnError(errors.New("query failed"))
	_, err := pool.Query("SELECT 1")
	if err == nil {
		t.Error("Query() should return error")
	}
	closePool(t, pool, mock)
}

// --- QueryContext ---

func TestPool_QueryContext(t *testing.T) {
	pool, mock := newMockPool(t)

	rows := sqlmock.NewRows([]string{"cnt"}).AddRow(42)
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(rows)

	ctx := context.Background()
	result, err := pool.QueryContext(ctx, "SELECT COUNT(*) FROM users")
	if err != nil {
		t.Fatalf("QueryContext() error = %v", err)
	}
	result.Close()
	closePool(t, pool, mock)
}

// --- Exec ---

func TestPool_Exec(t *testing.T) {
	pool, mock := newMockPool(t)

	mock.ExpectExec("INSERT INTO users").WillReturnResult(sqlmock.NewResult(1, 1))

	res, err := pool.Exec("INSERT INTO users (name) VALUES (?)", "charlie")
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	affected, _ := res.RowsAffected()
	if affected != 1 {
		t.Errorf("RowsAffected() = %d, want 1", affected)
	}
	closePool(t, pool, mock)
}

func TestPool_Exec_Error(t *testing.T) {
	pool, mock := newMockPool(t)

	mock.ExpectExec("DELETE FROM").WillReturnError(errors.New("exec failed"))
	_, err := pool.Exec("DELETE FROM nonexistent")
	if err == nil {
		t.Error("Exec() should return error")
	}
	closePool(t, pool, mock)
}

// --- ExecContext ---

func TestPool_ExecContext(t *testing.T) {
	pool, mock := newMockPool(t)

	mock.ExpectExec("UPDATE users").WillReturnResult(sqlmock.NewResult(0, 2))

	ctx := context.Background()
	res, err := pool.ExecContext(ctx, "UPDATE users SET name=? WHERE id=?", "dave", 1)
	if err != nil {
		t.Fatalf("ExecContext() error = %v", err)
	}
	affected, _ := res.RowsAffected()
	if affected != 2 {
		t.Errorf("RowsAffected() = %d, want 2", affected)
	}
	closePool(t, pool, mock)
}

// --- CurrentDB ---

func TestPool_CurrentDB(t *testing.T) {
	pool, mock := newMockPool(t)

	rows := sqlmock.NewRows([]string{"DATABASE()"}).AddRow("testdb")
	mock.ExpectQuery("SELECT DATABASE").WillReturnRows(rows)

	db := pool.CurrentDB()
	if db != "testdb" {
		t.Errorf("CurrentDB() = %q, want %q", db, "testdb")
	}
	closePool(t, pool, mock)
}

func TestPool_CurrentDB_Error(t *testing.T) {
	pool, mock := newMockPool(t)

	mock.ExpectQuery("SELECT DATABASE").WillReturnError(errors.New("no db"))

	db := pool.CurrentDB()
	if db != "" {
		t.Errorf("CurrentDB() on error should return empty string, got %q", db)
	}
	closePool(t, pool, mock)
}

// --- UseDB ---

func TestPool_UseDB(t *testing.T) {
	pool, mock := newMockPool(t)

	mock.ExpectExec("USE `mydb`").WillReturnResult(sqlmock.NewResult(0, 0))

	if err := pool.UseDB(context.Background(), "mydb"); err != nil {
		t.Errorf("UseDB() error = %v", err)
	}
	closePool(t, pool, mock)
}

func TestPool_UseDB_Error(t *testing.T) {
	pool, mock := newMockPool(t)

	mock.ExpectExec("USE `baddb`").WillReturnError(errors.New("unknown database"))

	if err := pool.UseDB(context.Background(), "baddb"); err == nil {
		t.Error("UseDB() should return error for unknown database")
	}
	closePool(t, pool, mock)
}

// --- DB ---

func TestPool_DB(t *testing.T) {
	pool, _ := newMockPool(t)
	if pool.DB() == nil {
		t.Error("DB() should not return nil")
	}
}

// --- IsClosed initial state ---

func TestPool_IsClosed_InitialState(t *testing.T) {
	pool, _ := newMockPool(t)
	if pool.IsClosed() {
		t.Error("IsClosed() should be false initially")
	}
}

// --- Databases ---

func TestPool_Databases(t *testing.T) {
	pool, mock := newMockPool(t)

	rows := sqlmock.NewRows([]string{"Database"}).
		AddRow("information_schema").
		AddRow("mysql").
		AddRow("testdb")
	mock.ExpectQuery("SHOW DATABASES").WillReturnRows(rows)

	dbs, err := pool.Databases()
	if err != nil {
		t.Fatalf("Databases() error = %v", err)
	}
	if len(dbs) != 3 {
		t.Fatalf("Databases() returned %d dbs, want 3", len(dbs))
	}
	if dbs[0] != "information_schema" || dbs[2] != "testdb" {
		t.Errorf("Databases() = %v, unexpected order", dbs)
	}
	closePool(t, pool, mock)
}

func TestPool_Databases_Error(t *testing.T) {
	pool, mock := newMockPool(t)

	mock.ExpectQuery("SHOW DATABASES").WillReturnError(errors.New("failed"))

	_, err := pool.Databases()
	if err == nil {
		t.Error("Databases() should return error")
	}
	closePool(t, pool, mock)
}

// --- Tables ---

func TestPool_Tables(t *testing.T) {
	pool, mock := newMockPool(t)

	rows := sqlmock.NewRows([]string{fmt.Sprintf("Tables_in_%s", "testdb")}).
		AddRow("users").
		AddRow("orders")
	mock.ExpectQuery("SHOW TABLES").WillReturnRows(rows)

	tables, err := pool.Tables("testdb")
	if err != nil {
		t.Fatalf("Tables() error = %v", err)
	}
	if len(tables) != 2 {
		t.Errorf("Tables() returned %d, want 2", len(tables))
	}
	closePool(t, pool, mock)
}

func TestPool_Tables_EmptyDatabase(t *testing.T) {
	pool, mock := newMockPool(t)

	rows := sqlmock.NewRows([]string{"Tables_in_testdb"})
	mock.ExpectQuery("SHOW TABLES").WillReturnRows(rows)

	tables, err := pool.Tables("")
	if err != nil {
		t.Fatalf("Tables() error = %v", err)
	}
	if len(tables) != 0 {
		t.Errorf("Tables() returned %d, want 0", len(tables))
	}
	closePool(t, pool, mock)
}

// --- Columns ---

func TestPool_Columns(t *testing.T) {
	pool, mock := newMockPool(t)

	rows := sqlmock.NewRows([]string{"Field", "Type", "Null", "Key", "Default", "Extra"}).
		AddRow("id", "int(11)", "NO", "PRI", nil, "auto_increment").
		AddRow("name", "varchar(255)", "YES", "", nil, "")
	mock.ExpectQuery("SHOW COLUMNS").WillReturnRows(rows)

	cols, err := pool.Columns("testdb", "users")
	if err != nil {
		t.Fatalf("Columns() error = %v", err)
	}
	if len(cols) != 2 {
		t.Fatalf("Columns() returned %d, want 2", len(cols))
	}
	if cols[0].Name != "id" {
		t.Errorf("cols[0].Name = %q, want %q", cols[0].Name, "id")
	}
	if cols[0].Key != "PRI" {
		t.Errorf("cols[0].Key = %q, want %q", cols[0].Key, "PRI")
	}
	if !cols[1].Nullable {
		t.Errorf("cols[1].Nullable = false, want true")
	}
	closePool(t, pool, mock)
}

func TestPool_Columns_Error(t *testing.T) {
	pool, mock := newMockPool(t)

	mock.ExpectQuery("SHOW COLUMNS").WillReturnError(errors.New("no table"))

	_, err := pool.Columns("", "nonexistent")
	if err == nil {
		t.Error("Columns() should return error")
	}
	closePool(t, pool, mock)
}

// --- NewWithDB ---

func TestNewWithDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectClose()
	cfg := &config.ConnectionConfig{Host: "localhost", Port: 3306, User: "root"}
	pool := NewWithDB(db, cfg)
	if pool == nil {
		t.Error("NewWithDB() returned nil")
	}
	if pool.DB() != db {
		t.Error("DB() should return the injected db")
	}
	db.Close()
}

// --- Context cancellation ---

func TestPool_QueryContext_Cancel(t *testing.T) {
	pool, mock := newMockPool(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// When context is already cancelled, the driver may or may not call the query.
	// We don't set an expectation; if the query is attempted, sqlmock will error,
	// which is also acceptable since the context is cancelled.
	_, err := pool.QueryContext(ctx, "SELECT 1")
	if err == nil {
		t.Error("QueryContext with cancelled context should return error")
	}
	closePool(t, pool, mock)
}
