package editor

import (
	"testing"
)

func TestNew(t *testing.T) {
	e := New()
	if e == nil {
		t.Fatal("New() returned nil")
	}
	if !e.IsEmpty() {
		t.Error("new editor should be empty")
	}
	if e.CursorPos() != 0 {
		t.Errorf("initial cursor = %d, want 0", e.CursorPos())
	}
}

func TestInsert(t *testing.T) {
	e := New()
	e.Insert("hello")
	if e.Text() != "hello" {
		t.Errorf("Text() = %q, want %q", e.Text(), "hello")
	}
	if e.CursorPos() != 5 {
		t.Errorf("CursorPos() = %d, want 5", e.CursorPos())
	}
}

func TestInsert_Middle(t *testing.T) {
	e := New()
	e.Insert("hello")
	e.MoveLeft()
	e.MoveLeft()
	e.Insert("X")
	if e.Text() != "helXlo" {
		t.Errorf("Text() = %q, want %q", e.Text(), "helXlo")
	}
}

func TestInsert_Multiline(t *testing.T) {
	e := New()
	e.Insert("line1\nline2\nline3")
	if e.LineCount() != 3 {
		t.Errorf("LineCount() = %d, want 3", e.LineCount())
	}
}

func TestDelete(t *testing.T) {
	e := New()
	e.Insert("hello")
	e.MoveLeft()
	e.Delete(1)
	if e.Text() != "hell" {
		t.Errorf("Text() = %q, want %q", e.Text(), "hell")
	}
}

func TestDelete_Zero(t *testing.T) {
	e := New()
	e.Insert("hello")
	e.Delete(0)
	if e.Text() != "hello" {
		t.Error("Delete(0) should not change text")
	}
}

func TestDelete_BeyondEnd(t *testing.T) {
	e := New()
	e.Insert("hi")
	e.MoveHome()
	e.Delete(100) // delete beyond end
	if e.Text() != "" {
		t.Errorf("Text() = %q, want empty", e.Text())
	}
}

func TestBackspace(t *testing.T) {
	e := New()
	e.Insert("hello")
	e.Backspace(1)
	if e.Text() != "hell" {
		t.Errorf("Text() = %q, want %q", e.Text(), "hell")
	}
	if e.CursorPos() != 4 {
		t.Errorf("CursorPos() = %d, want 4", e.CursorPos())
	}
}

func TestBackspace_Zero(t *testing.T) {
	e := New()
	e.Insert("hello")
	e.Backspace(0)
	if e.Text() != "hello" {
		t.Error("Backspace(0) should not change text")
	}
}

func TestBackspace_BeyondStart(t *testing.T) {
	e := New()
	e.Insert("hi")
	e.Backspace(100)
	if e.Text() != "" {
		t.Errorf("Text() = %q, want empty", e.Text())
	}
	if e.CursorPos() != 0 {
		t.Errorf("CursorPos() = %d, want 0", e.CursorPos())
	}
}

func TestMoveLeft(t *testing.T) {
	e := New()
	e.Insert("hello")
	e.MoveLeft()
	if e.CursorPos() != 4 {
		t.Errorf("CursorPos() = %d, want 4", e.CursorPos())
	}
	e.MoveLeft()
	if e.CursorPos() != 3 {
		t.Errorf("CursorPos() = %d, want 3", e.CursorPos())
	}
}

func TestMoveLeft_AtStart(t *testing.T) {
	e := New()
	e.MoveLeft() // already at start
	if e.CursorPos() != 0 {
		t.Errorf("CursorPos() = %d, want 0", e.CursorPos())
	}
}

func TestMoveRight(t *testing.T) {
	e := New()
	e.Insert("hello")
	e.MoveLeft()
	e.MoveRight()
	if e.CursorPos() != 5 {
		t.Errorf("CursorPos() = %d, want 5", e.CursorPos())
	}
}

func TestMoveRight_AtEnd(t *testing.T) {
	e := New()
	e.Insert("hi")
	e.MoveRight() // already at end
	if e.CursorPos() != 2 {
		t.Errorf("CursorPos() = %d, want 2 (should not go past end)", e.CursorPos())
	}
}

func TestMoveHome(t *testing.T) {
	e := New()
	e.Insert("hello")
	e.MoveHome()
	if e.CursorPos() != 0 {
		t.Errorf("CursorPos() = %d, want 0", e.CursorPos())
	}
}

func TestMoveHome_Multiline(t *testing.T) {
	e := New()
	e.Insert("line1\nline2")
	e.MoveHome() // go to start of "line2"
	// Cursor should be at start of the second line
	if e.CursorColumn() != 0 {
		t.Errorf("CursorColumn() = %d, want 0", e.CursorColumn())
	}
}

func TestMoveEnd(t *testing.T) {
	e := New()
	e.Insert("hello")
	e.MoveHome()
	e.MoveEnd()
	if e.CursorPos() != 5 {
		t.Errorf("CursorPos() = %d, want 5", e.CursorPos())
	}
}

func TestClear(t *testing.T) {
	e := New()
	e.Insert("hello")
	e.Clear()
	if e.Text() != "" {
		t.Errorf("Text() = %q, want empty", e.Text())
	}
	if e.CursorPos() != 0 {
		t.Errorf("CursorPos() = %d, want 0", e.CursorPos())
	}
	if !e.IsEmpty() {
		t.Error("IsEmpty() should be true after Clear()")
	}
}

func TestSetText(t *testing.T) {
	e := New()
	e.SetText("world")
	if e.Text() != "world" {
		t.Errorf("Text() = %q, want %q", e.Text(), "world")
	}
	if e.CursorPos() != 5 {
		t.Errorf("CursorPos() = %d, want 5 (cursor at end)", e.CursorPos())
	}
}

func TestLineCount(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 1},
		{"hello", 1},
		{"line1\nline2", 2},
		{"a\nb\nc", 3},
		{"\n\n", 3},
	}
	for _, tt := range tests {
		e := New()
		e.SetText(tt.input)
		if got := e.LineCount(); got != tt.want {
			t.Errorf("LineCount(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestCurrentLine(t *testing.T) {
	e := New()
	e.SetText("line1\nline2\nline3")
	// Cursor at end = line 2 (0-based)
	if got := e.CurrentLine(); got != 2 {
		t.Errorf("CurrentLine() = %d, want 2", got)
	}

	// Move cursor to line 0 by setting text and positioning at start
	e.Clear()
	e.Insert("line1\nline2\nline3")
	// Move to the very beginning
	for e.CursorPos() > 0 {
		e.MoveLeft()
	}
	if got := e.CurrentLine(); got != 0 {
		t.Errorf("CurrentLine() at start = %d, want 0", got)
	}

	// Move to end of line1 (position 5, just before \n)
	for i := 0; i < 5; i++ {
		e.MoveRight()
	}
	if got := e.CurrentLine(); got != 0 {
		t.Errorf("CurrentLine() at end of line1 = %d, want 0", got)
	}

	// Move past \n to line 1
	e.MoveRight()
	if got := e.CurrentLine(); got != 1 {
		t.Errorf("CurrentLine() on line2 = %d, want 1", got)
	}
}

func TestCursorColumn(t *testing.T) {
	e := New()
	e.Insert("hello")
	if got := e.CursorColumn(); got != 5 {
		t.Errorf("CursorColumn() = %d, want 5", got)
	}
	e.MoveHome()
	if got := e.CursorColumn(); got != 0 {
		t.Errorf("CursorColumn() at home = %d, want 0", got)
	}
}

func TestWordBeforeCursor(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"SELECT ", ""},
		{"SELECT FRO", "FRO"},
		{"hello", "hello"},
	}

	for _, tt := range tests {
		e := New()
		e.SetText(tt.input)
		got := e.WordBeforeCursor()
		if got != tt.want {
			t.Errorf("WordBeforeCursor() after SetText(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestReplaceWordBeforeCursor(t *testing.T) {
	e := New()
	e.SetText("SEL")
	e.MoveHome() // move to start
	e.MoveRight()
	e.MoveRight()
	e.MoveRight()
	// Now cursor at end of "SEL"
	e.ReplaceWordBeforeCursor("SELECT")
	if e.Text() != "SELECT" {
		t.Errorf("Text() = %q, want %q", e.Text(), "SELECT")
	}
}

func TestIsEmpty(t *testing.T) {
	e := New()
	if !e.IsEmpty() {
		t.Error("new editor should be empty")
	}
	e.Insert("x")
	if e.IsEmpty() {
		t.Error("editor with text should not be empty")
	}
}

func TestCurrentLineText(t *testing.T) {
	e := New()
	e.SetText("line1\nline2\nline3")
	// Cursor at end, on line 2
	if got := e.CurrentLineText(); got != "line3" {
		t.Errorf("CurrentLineText() = %q, want %q", got, "line3")
	}
}

func TestComplexEditSequence(t *testing.T) {
	e := New()
	e.Insert("Hello World")
	// Move cursor to position after "Hello" (before the space)
	for e.CursorPos() > 5 {
		e.MoveLeft()
	}
	e.Delete(1)  // delete the space (cursor stays at pos 5)
	e.Insert(",") // insert comma at pos 5
	if e.Text() != "Hello,World" {
		t.Errorf("Text() = %q, want %q", e.Text(), "Hello,World")
	}
}
