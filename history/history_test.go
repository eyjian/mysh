package history

import (
	"os"
	"path/filepath"
	"testing"
)

// --- New tests ---

func TestNew(t *testing.T) {
	h := New("/tmp/test_history", 100)
	if h == nil {
		t.Fatal("New() returned nil")
	}
	if h.Len() != 0 {
		t.Errorf("Len() = %d, want 0", h.Len())
	}
}

// --- Append tests ---

func TestAppend(t *testing.T) {
	h := New(filepath.Join(t.TempDir(), "history"), 100)
	h.Append("SELECT 1")
	h.Append("SELECT 2")

	if h.Len() != 2 {
		t.Errorf("Len() = %d, want 2", h.Len())
	}
}

func TestAppend_Empty(t *testing.T) {
	h := New(filepath.Join(t.TempDir(), "history"), 100)
	h.Append("")
	h.Append("   ")

	if h.Len() != 0 {
		t.Errorf("Len() = %d, want 0 (empty entries ignored)", h.Len())
	}
}

func TestAppend_Duplicate(t *testing.T) {
	h := New(filepath.Join(t.TempDir(), "history"), 100)
	h.Append("SELECT 1")
	h.Append("SELECT 1") // duplicate of last entry should be ignored

	if h.Len() != 1 {
		t.Errorf("Len() = %d, want 1 (duplicate ignored)", h.Len())
	}
}

func TestAppend_DuplicateNotConsecutive(t *testing.T) {
	h := New(filepath.Join(t.TempDir(), "history"), 100)
	h.Append("SELECT 1")
	h.Append("SELECT 2")
	h.Append("SELECT 1") // not consecutive duplicate, should be added

	if h.Len() != 3 {
		t.Errorf("Len() = %d, want 3", h.Len())
	}
}

func TestAppend_MaxEntries(t *testing.T) {
	h := New(filepath.Join(t.TempDir(), "history"), 3)
	h.Append("cmd1")
	h.Append("cmd2")
	h.Append("cmd3")
	h.Append("cmd4") // should trim oldest

	if h.Len() != 3 {
		t.Errorf("Len() = %d, want 3", h.Len())
	}
	entries := h.Entries()
	if entries[0] != "cmd2" {
		t.Errorf("oldest entry = %q, want %q", entries[0], "cmd2")
	}
}

// --- Entries tests ---

func TestEntries_ReturnsCopy(t *testing.T) {
	h := New(filepath.Join(t.TempDir(), "history"), 100)
	h.Append("test")

	entries := h.Entries()
	entries[0] = "modified"

	if h.Entries()[0] == "modified" {
		t.Error("Entries() should return a copy, not a reference")
	}
}

// --- Search tests ---

func TestSearch(t *testing.T) {
	h := New(filepath.Join(t.TempDir(), "history"), 100)
	h.Append("SELECT * FROM users")
	h.Append("INSERT INTO orders VALUES (1)")
	h.Append("SELECT id FROM products")
	h.Append("UPDATE users SET name = 'test'")

	tests := []struct {
		pattern string
		want    int
	}{
		{"SELECT", 2},
		{"users", 2},
		{"insert", 1}, // case-insensitive
		{"DELETE", 0},
		{"from", 2},
	}

	for _, tt := range tests {
		results := h.Search(tt.pattern)
		if len(results) != tt.want {
			t.Errorf("Search(%q) = %d results, want %d", tt.pattern, len(results), tt.want)
		}
	}
}

func TestSearch_Empty(t *testing.T) {
	h := New(filepath.Join(t.TempDir(), "history"), 100)
	results := h.Search("anything")
	if len(results) != 0 {
		t.Errorf("Search on empty history = %d results, want 0", len(results))
	}
}

// --- Navigation tests ---

func TestPrevNext(t *testing.T) {
	h := New(filepath.Join(t.TempDir(), "history"), 100)
	h.Append("cmd1")
	h.Append("cmd2")
	h.Append("cmd3")

	// Navigate backward (older)
	if got := h.Prev(); got != "cmd3" {
		t.Errorf("Prev() = %q, want %q", got, "cmd3")
	}
	if got := h.Prev(); got != "cmd2" {
		t.Errorf("Prev() = %q, want %q", got, "cmd2")
	}
	if got := h.Prev(); got != "cmd1" {
		t.Errorf("Prev() = %q, want %q", got, "cmd1")
	}
	// At the beginning, should stay at oldest
	if got := h.Prev(); got != "cmd1" {
		t.Errorf("Prev() at beginning = %q, want %q", got, "cmd1")
	}

	// Navigate forward (newer)
	if got := h.Next(); got != "cmd2" {
		t.Errorf("Next() = %q, want %q", got, "cmd2")
	}
	if got := h.Next(); got != "cmd3" {
		t.Errorf("Next() = %q, want %q", got, "cmd3")
	}
	// At the end, should return empty
	if got := h.Next(); got != "" {
		t.Errorf("Next() at end = %q, want empty", got)
	}
}

func TestPrevNext_EmptyHistory(t *testing.T) {
	h := New(filepath.Join(t.TempDir(), "history"), 100)
	if got := h.Prev(); got != "" {
		t.Errorf("Prev() on empty = %q, want empty", got)
	}
	if got := h.Next(); got != "" {
		t.Errorf("Next() on empty = %q, want empty", got)
	}
}

func TestResetNav(t *testing.T) {
	h := New(filepath.Join(t.TempDir(), "history"), 100)
	h.Append("cmd1")
	h.Append("cmd2")

	h.Prev() // move position
	h.ResetNav()
	// After reset, Prev should start from most recent again
	if got := h.Prev(); got != "cmd2" {
		t.Errorf("Prev() after ResetNav = %q, want %q", got, "cmd2")
	}
}

// --- Persistence tests ---

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "history")

	h1 := New(file, 100)
	h1.Append("SELECT 1")
	h1.Append("SELECT 2")
	h1.Append("SELECT 3")

	if err := h1.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	h2 := New(file, 100)
	if err := h2.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if h2.Len() != 3 {
		t.Errorf("Len() after Load = %d, want 3", h2.Len())
	}

	entries := h2.Entries()
	if entries[0] != "SELECT 1" {
		t.Errorf("entries[0] = %q, want %q", entries[0], "SELECT 1")
	}
}

func TestLoad_NonexistentFile(t *testing.T) {
	h := New("/nonexistent/path/history", 100)
	err := h.Load()
	if err != nil {
		t.Errorf("Load() with nonexistent file should not error, got %v", err)
	}
}

func TestSave_EmptyHistory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "history")

	h := New(file, 100)
	if err := h.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// File should exist but be empty
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(data) != 0 {
		t.Errorf("file content = %q, want empty", data)
	}
}

func TestLoad_TrimToMax(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "history")

	// Create a file with 5 entries
	content := "cmd1\ncmd2\ncmd3\ncmd4\ncmd5\n"
	if err := os.WriteFile(file, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Load with max=3, should only keep last 3
	h := New(file, 3)
	if err := h.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if h.Len() != 3 {
		t.Errorf("Len() = %d, want 3", h.Len())
	}
	entries := h.Entries()
	if entries[0] != "cmd3" {
		t.Errorf("oldest entry = %q, want %q", entries[0], "cmd3")
	}
}

// --- Clear tests ---

func TestClear(t *testing.T) {
	h := New(filepath.Join(t.TempDir(), "history"), 100)
	h.Append("cmd1")
	h.Append("cmd2")

	h.Clear()
	if h.Len() != 0 {
		t.Errorf("Len() after Clear = %d, want 0", h.Len())
	}
}

// --- Concurrency test ---

func TestConcurrentAppend(t *testing.T) {
	h := New(filepath.Join(t.TempDir(), "history"), 1000)
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(n int) {
			h.Append("cmd" + string(rune('0'+n)))
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if h.Len() != 10 {
		t.Errorf("Len() after concurrent appends = %d, want 10", h.Len())
	}
}
