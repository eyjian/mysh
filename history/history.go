package history

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
)

// History manages command history with file persistence.
type History struct {
	mu      sync.RWMutex
	entries []string
	file    string
	max     int
	pos     int // current position for navigation (0 = most recent)
}

// New creates a new History instance.
// file is the path for persistence; max is the maximum number of entries.
func New(file string, max int) *History {
	return &History{
		entries: make([]string, 0),
		file:    file,
		max:     max,
		pos:     -1,
	}
}

// Load reads history entries from the persisted file.
func (h *History) Load() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	f, err := os.Open(h.file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no history file is fine
		}
		return fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	var entries []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			entries = append(entries, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read history file: %w", err)
	}

	// Trim to max entries if needed
	if len(entries) > h.max {
		entries = entries[len(entries)-h.max:]
	}

	h.entries = entries
	h.pos = -1
	return nil
}

// Save writes history entries to the persisted file.
func (h *History) Save() error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	f, err := os.Create(h.file)
	if err != nil {
		return fmt.Errorf("create history file: %w", err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, entry := range h.entries {
		if _, err := w.WriteString(entry + "\n"); err != nil {
			return fmt.Errorf("write history entry: %w", err)
		}
	}
	return w.Flush()
}

// Append adds a new entry to history.
// Empty or duplicate of the last entry is ignored.
func (h *History) Append(entry string) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Skip if duplicate of last entry (but still reset navigation position)
	if len(h.entries) > 0 && h.entries[len(h.entries)-1] == entry {
		h.pos = -1
		return
	}

	h.entries = append(h.entries, entry)

	// Trim to max entries
	if len(h.entries) > h.max {
		h.entries = h.entries[len(h.entries)-h.max:]
	}

	h.pos = -1 // reset navigation position
}

// Entries returns a copy of all history entries.
func (h *History) Entries() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]string, len(h.entries))
	copy(result, h.entries)
	return result
}

// Search returns entries matching the given pattern (case-insensitive substring match).
func (h *History) Search(pattern string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	pattern = strings.ToLower(pattern)
	var results []string
	for _, entry := range h.entries {
		if strings.Contains(strings.ToLower(entry), pattern) {
			results = append(results, entry)
		}
	}
	return results
}

// SearchIncremental returns entries matching the pattern (case-insensitive substring),
// ordered from most recent to oldest. Used by Ctrl+R incremental search for cycling.
func (h *History) SearchIncremental(pattern string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	pattern = strings.ToLower(pattern)
	var results []string
	// Iterate from most recent (end) to oldest (beginning)
	for i := len(h.entries) - 1; i >= 0; i-- {
		if strings.Contains(strings.ToLower(h.entries[i]), pattern) {
			results = append(results, h.entries[i])
		}
	}
	return results
}

// Prev returns the previous entry in navigation (older entry).
// Returns empty string if at the beginning.
func (h *History) Prev() string {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.entries) == 0 {
		return ""
	}

	if h.pos < len(h.entries)-1 {
		h.pos++
	}
	return h.entries[len(h.entries)-1-h.pos]
}

// Next returns the next entry in navigation (newer entry).
// Returns empty string if at the end.
func (h *History) Next() string {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.pos > 0 {
		h.pos--
		return h.entries[len(h.entries)-1-h.pos]
	}
	h.pos = -1
	return ""
}

// ResetNav resets the navigation position.
func (h *History) ResetNav() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pos = -1
}

// Len returns the number of history entries.
func (h *History) Len() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.entries)
}

// Clear removes all history entries.
func (h *History) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = h.entries[:0]
	h.pos = -1
}
