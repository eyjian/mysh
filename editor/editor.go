package editor

import (
	"strings"
)

// Editor manages an input buffer with cursor position, supporting multi-line text editing.
type Editor struct {
	text   []rune
	cursor int // 0-based position in the text
}

// New creates a new Editor with empty text.
func New() *Editor {
	return &Editor{
		text:   make([]rune, 0),
		cursor: 0,
	}
}

// Insert inserts text at the current cursor position and moves the cursor forward.
func (e *Editor) Insert(text string) {
	runes := []rune(text)
	tail := make([]rune, len(e.text)-e.cursor)
	copy(tail, e.text[e.cursor:])

	e.text = append(e.text[:e.cursor], runes...)
	e.text = append(e.text, tail...)
	e.cursor += len(runes)
}

// Delete deletes n characters after the cursor.
// If n exceeds available characters, deletes to the end.
func (e *Editor) Delete(n int) {
	if n <= 0 {
		return
	}
	end := e.cursor + n
	if end > len(e.text) {
		end = len(e.text)
	}
	e.text = append(e.text[:e.cursor], e.text[end:]...)
}

// Backspace deletes n characters before the cursor.
// If n exceeds available characters, deletes to the beginning.
func (e *Editor) Backspace(n int) {
	if n <= 0 {
		return
	}
	start := e.cursor - n
	if start < 0 {
		start = 0
	}
	e.text = append(e.text[:start], e.text[e.cursor:]...)
	e.cursor = start
}

// MoveLeft moves the cursor one character to the left.
func (e *Editor) MoveLeft() {
	if e.cursor > 0 {
		e.cursor--
	}
}

// MoveRight moves the cursor one character to the right.
func (e *Editor) MoveRight() {
	if e.cursor < len(e.text) {
		e.cursor++
	}
}

// MoveHome moves the cursor to the beginning of the current line.
func (e *Editor) MoveHome() {
	// Find the start of the current line
	if e.cursor == 0 {
		return
	}
	// Search backwards for newline
	pos := e.cursor - 1
	for pos >= 0 && e.text[pos] != '\n' {
		pos--
	}
	e.cursor = pos + 1
}

// MoveEnd moves the cursor to the end of the current line.
func (e *Editor) MoveEnd() {
	// Search forward for newline
	pos := e.cursor
	for pos < len(e.text) && e.text[pos] != '\n' {
		pos++
	}
	e.cursor = pos
}

// Text returns the full text content.
func (e *Editor) Text() string {
	return string(e.text)
}

// CursorPos returns the current cursor position (0-based offset in the full text).
func (e *Editor) CursorPos() int {
	return e.cursor
}

// Clear removes all text and resets the cursor.
func (e *Editor) Clear() {
	e.text = e.text[:0]
	e.cursor = 0
}

// SetText replaces the editor content and places cursor at the end.
func (e *Editor) SetText(text string) {
	e.text = []rune(text)
	e.cursor = len(e.text)
}

// LineCount returns the number of lines in the text.
func (e *Editor) LineCount() int {
	if len(e.text) == 0 {
		return 1
	}
	count := 1
	for _, r := range e.text {
		if r == '\n' {
			count++
		}
	}
	return count
}

// CurrentLine returns the line number (0-based) where the cursor is.
func (e *Editor) CurrentLine() int {
	line := 0
	for i := 0; i < e.cursor && i < len(e.text); i++ {
		if e.text[i] == '\n' {
			line++
		}
	}
	return line
}

// CurrentLineText returns the text of the line where the cursor is.
func (e *Editor) CurrentLineText() string {
	lines := strings.Split(string(e.text), "\n")
	lineNum := e.CurrentLine()
	if lineNum < len(lines) {
		return lines[lineNum]
	}
	return ""
}

// CursorColumn returns the column position within the current line (0-based).
func (e *Editor) CursorColumn() int {
	pos := e.cursor
	col := 0
	for pos > 0 && e.text[pos-1] != '\n' {
		pos--
		col++
	}
	return col
}

// IsEmpty returns true if the editor has no text.
func (e *Editor) IsEmpty() bool {
	return len(e.text) == 0
}

// WordBeforeCursor returns the word immediately before the cursor.
// Used for auto-completion triggers.
func (e *Editor) WordBeforeCursor() string {
	if e.cursor == 0 {
		return ""
	}
	end := e.cursor
	start := e.cursor - 1
	for start >= 0 && isWordChar(e.text[start]) {
		start--
	}
	start++ // move to first word char
	if start >= end {
		return ""
	}
	return string(e.text[start:end])
}

// ReplaceWordBeforeCursor replaces the word before the cursor with newText.
// Used when accepting a completion suggestion.
func (e *Editor) ReplaceWordBeforeCursor(newText string) {
	if e.cursor == 0 {
		e.Insert(newText)
		return
	}
	end := e.cursor
	start := e.cursor - 1
	for start >= 0 && isWordChar(e.text[start]) {
		start--
	}
	start++ // move to first word char

	// Delete the old word
	e.text = append(e.text[:start], e.text[end:]...)
	e.cursor = start

	// Insert new text
	e.Insert(newText)
}

// isWordChar returns true if the rune is a valid word character for completion.
func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') || r == '_' || r == '.'
}
