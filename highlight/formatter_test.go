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

func TestFormatCreateTableSQL_PartitionRange(t *testing.T) {
	input := "CREATE TABLE `t` (\n  `id` int NOT NULL,\n  PRIMARY KEY (`id`)\n) ENGINE=InnoDB PARTITION BY RANGE ( month(pay_time) ) ( PARTITION p202501 VALUES LESS THAN (202502), PARTITION p202502 VALUES LESS THAN (202503), PARTITION pcatchall VALUES LESS THAN (MAXVALUE))"
	result := FormatCreateTableSQL(input)

	// PARTITION BY should start on its own line (flush-left)
	if !strings.Contains(result, "\nPARTITION BY RANGE") {
		t.Errorf("Expected 'PARTITION BY RANGE' on its own line, got: %s", result)
	}
	// Each PARTITION should be on its own line with indentation
	if !strings.Contains(result, "\n  PARTITION p202501") {
		t.Errorf("Expected 'PARTITION p202501' on its own line, got: %s", result)
	}
	if !strings.Contains(result, "\n  PARTITION p202502") {
		t.Errorf("Expected 'PARTITION p202502' on its own line, got: %s", result)
	}
	if !strings.Contains(result, "\n  PARTITION pcatchall") {
		t.Errorf("Expected 'PARTITION pcatchall' on its own line, got: %s", result)
	}
	// No blank line after the opening '('
	if strings.Contains(result, "(\n\n") {
		t.Errorf("Expected no blank line after opening '(', got: %s", result)
	}
	// Closing paren should be on its own line
	if !strings.Contains(result, "\n)") {
		t.Errorf("Expected closing paren on its own line, got: %s", result)
	}
	// No trailing space before PARTITION BY
	if strings.Contains(result, " \nPARTITION BY") {
		t.Errorf("Expected no trailing space before PARTITION BY, got: %s", result)
	}
}

func TestFormatCreateTableSQL_NoPartition(t *testing.T) {
	input := "CREATE TABLE `t` (\n  `id` int NOT NULL,\n  PRIMARY KEY (`id`)\n) ENGINE=InnoDB"
	result := FormatCreateTableSQL(input)
	// No PARTITION BY → returned unchanged
	if result != input {
		t.Errorf("Expected unchanged output for non-partitioned table, got: %s", result)
	}
}

func TestFormatCreateTableSQL_ManyPartitions(t *testing.T) {
	input := "CREATE TABLE `t` (\n  `id` int NOT NULL\n) PARTITION BY RANGE ( month(pay_time) ) ( PARTITION p202501 VALUES LESS THAN (202502), PARTITION p202502 VALUES LESS THAN (202503), PARTITION p202503 VALUES LESS THAN (202504), PARTITION pcatchall VALUES LESS THAN (MAXVALUE))"
	result := FormatCreateTableSQL(input)

	lines := strings.Split(result, "\n")
	// Count lines containing "PARTITION p"
	partitionLines := 0
	for _, line := range lines {
		if strings.Contains(line, "PARTITION p") {
			partitionLines++
		}
	}
	if partitionLines != 4 {
		t.Errorf("Expected 4 partition lines, got %d: %s", partitionLines, result)
	}
	// PARTITION BY should be on its own line, flush-left
	if !strings.Contains(result, "\nPARTITION BY RANGE") {
		t.Errorf("Expected 'PARTITION BY RANGE' on its own line, got: %s", result)
	}
}
