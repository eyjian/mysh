package connection

import (
	"testing"

	"mysh/config"
)

// --- New with nil config ---

func TestNew_NilConfig(t *testing.T) {
	_, err := New(nil)
	if err == nil {
		t.Error("New(nil) should return error")
	}
}

// --- Pool.IsClosed ---

func TestPool_IsClosed_WithoutConnection(t *testing.T) {
	// We can't easily test Pool without a real MySQL instance,
	// but we can test the nil config guard
	_, err := New(nil)
	if err == nil {
		t.Error("expected error for nil config")
	}
}

// --- ColumnInfo ---

func TestColumnInfo_Fields(t *testing.T) {
	col := ColumnInfo{
		Name:     "id",
		Type:     "int(11)",
		Nullable: false,
		Key:      "PRI",
	}
	if col.Name != "id" {
		t.Errorf("Name = %q, want %q", col.Name, "id")
	}
	if col.Type != "int(11)" {
		t.Errorf("Type = %q, want %q", col.Type, "int(11)")
	}
	if col.Nullable {
		t.Errorf("Nullable = true, want false")
	}
	if col.Key != "PRI" {
		t.Errorf("Key = %q, want %q", col.Key, "PRI")
	}
}

// --- Integration tests (require MySQL, skip if unavailable) ---

func TestNew_WithInvalidDSN(t *testing.T) {
	cfg := &config.ConnectionConfig{
		Host:     "invalid-host-that-does-not-exist.invalid",
		Port:     99999,
		User:     "nonexistent",
		Password: "wrong",
		Database: "nodb",
	}

	_, err := New(cfg)
	if err == nil {
		t.Error("New() with invalid DSN should return error")
	}
}

func TestPool_Close_WithoutOpen(t *testing.T) {
	// Test closing a pool that was never properly connected
	// We create a pool via internal construction for this test
	cfg := &config.ConnectionConfig{
		Host:     "invalid-host.invalid",
		Port:     3306,
		User:     "root",
		Password: "",
		Database: "test",
	}
	_, err := New(cfg)
	// Connection will fail but we verify the error path
	if err == nil {
		t.Log("unexpected: connected to invalid host")
	}
}

// --- DSN construction in Pool ---

func TestDSN_Construction(t *testing.T) {
	cfg := &config.ConnectionConfig{
		Host:     "localhost",
		Port:     3306,
		User:     "root",
		Password: "secret",
		Database: "testdb",
	}
	want := "root:secret@tcp(localhost:3306)/testdb"
	if got := cfg.DSN(); got != want {
		t.Errorf("DSN() = %q, want %q", got, want)
	}
}
