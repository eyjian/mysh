package completer

import "strings"

// Snippet represents a SQL code snippet template.
type Snippet struct {
	Trigger     string // keyword that triggers this snippet
	Description string // human-readable description
	Template    string // template text with ${N:default} placeholders
	Context     string // applicable context
}

// BuiltInSnippets returns the built-in SQL snippet list.
func BuiltInSnippets() []Snippet {
	return []Snippet{
		{
			Trigger:     "create",
			Description: "CREATE TABLE template",
			Template:    "CREATE TABLE table_name (\n  id INT PRIMARY KEY AUTO_INCREMENT,\n  column_name VARCHAR(255)\n)",
			Context:     "statement",
		},
		{
			Trigger:     "alter",
			Description: "ALTER TABLE ADD COLUMN template",
			Template:    "ALTER TABLE table_name ADD COLUMN column_name VARCHAR(255)",
			Context:     "statement",
		},
		{
			Trigger:     "insert",
			Description: "INSERT INTO template",
			Template:    "INSERT INTO table_name (columns) VALUES (values)",
			Context:     "statement",
		},
		{
			Trigger:     "update",
			Description: "UPDATE template",
			Template:    "UPDATE table_name SET column = value WHERE condition",
			Context:     "statement",
		},
		{
			Trigger:     "select",
			Description: "SELECT template",
			Template:    "SELECT columns FROM table_name WHERE condition",
			Context:     "statement",
		},
		{
			Trigger:     "delete",
			Description: "DELETE template",
			Template:    "DELETE FROM table_name WHERE condition",
			Context:     "statement",
		},
		{
			Trigger:     "join",
			Description: "INNER JOIN template",
			Template:    "INNER JOIN table_name ON condition",
			Context:     "after_from",
		},
		{
			Trigger:     "left",
			Description: "LEFT JOIN template",
			Template:    "LEFT JOIN table_name ON condition",
			Context:     "after_from",
		},
		{
			Trigger:     "index",
			Description: "CREATE INDEX template",
			Template:    "CREATE INDEX index_name ON table_name (column)",
			Context:     "statement",
		},
		{
			Trigger:     "create_user",
			Description: "CREATE USER template",
			Template:    "CREATE USER 'username'@'host' IDENTIFIED BY 'password'",
			Context:     "statement",
		},
		{
			Trigger:     "grant",
			Description: "GRANT privileges template",
			Template:    "GRANT SELECT, INSERT ON database.table TO 'user'@'host'",
			Context:     "statement",
		},
		{
			Trigger:     "select_into",
			Description: "SELECT INTO OUTFILE template",
			Template:    "SELECT columns INTO OUTFILE '/tmp/file.csv' FIELDS TERMINATED BY ',' FROM table_name",
			Context:     "statement",
		},
		{
			Trigger:     "right",
			Description: "RIGHT JOIN template",
			Template:    "RIGHT JOIN table_name ON condition",
			Context:     "after_from",
		},
		{
			Trigger:     "cross",
			Description: "CROSS JOIN template",
			Template:    "CROSS JOIN table_name",
			Context:     "after_from",
		},
	}
}

// FindSnippetByTrigger finds a snippet by its trigger keyword (case-insensitive).
func FindSnippetByTrigger(trigger string) *Snippet {
	lower := strings.ToLower(trigger)
	for i := range BuiltInSnippets() {
		if strings.ToLower(BuiltInSnippets()[i].Trigger) == lower {
			return &BuiltInSnippets()[i]
		}
	}
	return nil
}

// MatchingSnippets returns snippets whose trigger starts with the given prefix (case-insensitive).
func MatchingSnippets(prefix string) []Snippet {
	if prefix == "" {
		return nil
	}
	lower := strings.ToLower(prefix)
	var matches []Snippet
	for _, s := range BuiltInSnippets() {
		if strings.HasPrefix(strings.ToLower(s.Trigger), lower) && strings.ToLower(s.Trigger) != lower {
			matches = append(matches, s)
		}
	}
	return matches
}
