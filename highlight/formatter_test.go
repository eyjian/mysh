package highlight

import (
	"strings"
	"testing"
)

func TestFormatSQL_SelectBasic(t *testing.T) {
	input := "SELECT id, name FROM users WHERE id = 1"
	result := FormatSQL(input)
	// Should be multi-line
	if !strings.Contains(result, "SELECT") {
		t.Errorf("Expected SELECT in result, got: %s", result)
	}
	if !strings.Contains(result, "FROM") {
		t.Errorf("Expected FROM in result, got: %s", result)
	}
	if !strings.Contains(result, "WHERE") {
		t.Errorf("Expected WHERE in result, got: %s", result)
	}
	// Should have line breaks
	lines := strings.Split(result, "\n")
	if len(lines) < 3 {
		t.Errorf("Expected multi-line output, got %d lines: %s", len(lines), result)
	}
}

func TestFormatSQL_SelectWithSemicolon(t *testing.T) {
	input := "SELECT id FROM users;"
	result := FormatSQL(input)
	if !strings.HasSuffix(result, ";") {
		t.Errorf("Expected trailing semicolon, got: %s", result)
	}
}

func TestFormatSQL_InsertStatement(t *testing.T) {
	input := "INSERT INTO users (id, name) VALUES (1, 'test')"
	result := FormatSQL(input)
	if !strings.Contains(result, "INSERT") {
		t.Errorf("Expected INSERT in result, got: %s", result)
	}
}

func TestFormatSQL_Empty(t *testing.T) {
	result := FormatSQL("")
	if result != "" {
		t.Errorf("Expected empty result, got: %s", result)
	}
}

func TestFormatSQL_SimpleSelect(t *testing.T) {
	input := "SELECT 1"
	result := FormatSQL(input)
	if !strings.Contains(result, "SELECT") {
		t.Errorf("Expected SELECT in result, got: %s", result)
	}
}

func TestCompactSQL(t *testing.T) {
	input := "  SELECT   id,  name   FROM   users  WHERE  id = 1  "
	result := CompactSQL(input)
	// Should normalize whitespace
	if strings.Contains(result, "  ") {
		t.Errorf("Expected single spaces, got: %s", result)
	}
	if strings.Contains(result, "\n") {
		t.Errorf("Expected single line, got: %s", result)
	}
	if !strings.HasPrefix(result, "SELECT") {
		t.Errorf("Expected to start with SELECT, got: %s", result)
	}
}

func TestCompactSQL_Empty(t *testing.T) {
	result := CompactSQL("")
	if result != "" {
		t.Errorf("Expected empty, got: %s", result)
	}
}

func TestFormatSQL_UpdateStatement(t *testing.T) {
	input := "UPDATE users SET name = 'test' WHERE id = 1"
	result := FormatSQL(input)
	if !strings.Contains(result, "UPDATE") {
		t.Errorf("Expected UPDATE in result, got: %s", result)
	}
	if !strings.Contains(result, "SET") {
		t.Errorf("Expected SET in result, got: %s", result)
	}
}

func TestFormatSQL_JoinStatement(t *testing.T) {
	input := "SELECT u.id, o.total FROM users u JOIN orders o ON u.id = o.user_id WHERE o.total > 100"
	result := FormatSQL(input)
	if !strings.Contains(result, "JOIN") {
		t.Errorf("Expected JOIN in result, got: %s", result)
	}
	if !strings.Contains(result, "ON") {
		t.Errorf("Expected ON in result, got: %s", result)
	}
}
