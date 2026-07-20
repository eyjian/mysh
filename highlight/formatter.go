package highlight

import (
	"strings"
	"unicode"
)

// FormatSQL reformats a SQL statement with consistent indentation and line breaks.
// It uses the tokenizer to produce a structured, readable output.
func FormatSQL(sql string) string {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return ""
	}

	t := NewTokenizer()
	tokens := t.Tokenize(sql)
	if len(tokens) == 0 {
		return sql
	}

	// Major keywords that start a new clause at the top level
	topLevelKeywords := map[string]bool{
		"SELECT": true, "FROM": true, "WHERE": true, "SET": true,
		"GROUP": true, "HAVING": true, "ORDER": true, "LIMIT": true,
		"INSERT": true, "INTO": true, "VALUES": true,
		"UPDATE": true, "DELETE": true,
		"CREATE": true, "ALTER": true, "DROP": true,
		"JOIN": true, "INNER": true, "LEFT": true, "RIGHT": true,
		"CROSS": true, "FULL": true, "NATURAL": true,
		"ON": true, "AND": true, "OR": true,
		"UNION": true, "EXCEPT": true, "INTERSECT": true,
		"WHEN": true, "THEN": true, "ELSE": true, "END": true,
		"CASE": true,
	}

	// Keywords after which we indent sub-expressions
	indentKeywords := map[string]bool{
		"SELECT": true, "FROM": true, "WHERE": true, "SET": true,
		"HAVING": true, "ON": true,
		"VALUES": true,
	}

	// Comma triggers a new line in SELECT and ORDER BY
	var lines []string
	var current strings.Builder
	indentLevel := 0
	parenDepth := 0
	afterIndent := false   // true right after an indent keyword
	inSelect := false      // true between SELECT and FROM
	inOrderBy := false     // true after ORDER
	inCreateTable := false // true after CREATE TABLE

	writeIndent := func() {
		current.WriteString(strings.Repeat("  ", indentLevel))
	}

	flushLine := func() {
		line := strings.TrimRight(current.String(), " ")
		if line != "" {
			lines = append(lines, line)
		}
		current.Reset()
		writeIndent()
	}

	for i, tok := range tokens {
		upper := strings.ToUpper(tok.Value)

		// Handle paren depth
		if tok.Value == "(" {
			parenDepth++
			current.WriteString(tok.Value)
			if parenDepth == 1 && afterIndent {
				// Align items inside parens
				flushLine()
				indentLevel++
				writeIndent()
			}
			afterIndent = false
			continue
		}
		if tok.Value == ")" {
			parenDepth--
			if parenDepth == 0 && indentLevel > 0 {
				flushLine()
				indentLevel--
			}
			current.WriteString(tok.Value)
			continue
		}

		// Skip comments in formatting
		if tok.Type == TokenComment {
			continue
		}

		// Handle top-level keywords
		if tok.Type == TokenKeyword && parenDepth == 0 && topLevelKeywords[upper] {
			// Special: ORDER BY is two tokens
			if upper == "ORDER" {
				inOrderBy = true
			} else if upper != "BY" {
				inOrderBy = false
			}
			if upper == "SELECT" {
				inSelect = true
			}
			if upper == "FROM" {
				inSelect = false
			}
			if upper == "CREATE" {
				inCreateTable = true
			}

			// Don't break line for certain contexts
			shouldBreak := true
			if upper == "INTO" && i > 0 {
				// INSERT INTO — keep on same line
				shouldBreak = false
			}
			if upper == "BY" && inOrderBy {
				// ORDER BY — keep on same line as ORDER
				shouldBreak = false
			}
			if upper == "TABLE" && inCreateTable {
				// CREATE TABLE — keep on same line
				shouldBreak = false
				inCreateTable = false
			}
			if upper == "SET" {
				// UPDATE SET — keep on same line
				prevKeyword := prevKeywordValue(tokens, i)
				if prevKeyword == "UPDATE" {
					shouldBreak = false
				}
			}
			if upper == "AND" || upper == "OR" {
				shouldBreak = true
			}
			if upper == "THEN" || upper == "ELSE" || upper == "WHEN" {
				shouldBreak = true
			}

			if shouldBreak {
				flushLine()
				if upper == "AND" || upper == "OR" {
					// AND/OR at same indent as WHERE
				} else if indentKeywords[upper] {
					indentLevel = 1
				}
				current.WriteString(upper)
				if indentKeywords[upper] {
					afterIndent = true
				} else {
					current.WriteString(" ")
				}
				continue
			}
		}

		// Handle commas in SELECT list
		if tok.Value == "," && parenDepth <= 1 {
			if inSelect || inOrderBy {
				current.WriteString(",")
				flushLine()
				continue
			}
		}

		// Default: append token with spacing
		if current.Len() > 0 && afterIndent {
			// First token after indent keyword
			current.WriteString(" ")
			current.WriteString(tok.Value)
			afterIndent = false
		} else if current.Len() > 0 && needsSpaceBefore(tok, tokens, i) {
			current.WriteString(" ")
			current.WriteString(tok.Value)
		} else {
			current.WriteString(tok.Value)
		}
	}

	// Flush remaining
	line := strings.TrimRight(current.String(), " ")
	if line != "" {
		lines = append(lines, line)
	}

	result := strings.Join(lines, "\n")
	result = strings.TrimSpace(result)

	// Add trailing semicolon if the original had one
	if strings.HasSuffix(strings.TrimSpace(sql), ";") && !strings.HasSuffix(result, ";") {
		result += ";"
	}

	return result
}

// prevKeywordValue returns the value of the previous keyword token.
func prevKeywordValue(tokens []Token, idx int) string {
	for i := idx - 1; i >= 0; i-- {
		if tokens[i].Type == TokenKeyword {
			return strings.ToUpper(tokens[i].Value)
		}
		if tokens[i].Type == TokenComment {
			continue
		}
		break
	}
	return ""
}

// needsSpaceBefore determines if a space should be inserted before the token.
func needsSpaceBefore(tok Token, tokens []Token, idx int) bool {
	if idx == 0 {
		return false
	}
	prev := tokens[idx-1]

	// No space after opening paren
	if prev.Value == "(" {
		return false
	}
	// No space before closing paren or comma
	if tok.Value == ")" || tok.Value == "," || tok.Value == "." {
		return false
	}
	// No space before opening paren for function calls
	if tok.Value == "(" && prev.Type == TokenFunction {
		return false
	}
	// No space after dot
	if prev.Value == "." {
		return false
	}
	// No space before dot
	if tok.Value == "." {
		return false
	}

	// Punctuation generally doesn't need space before
	if tok.Type == TokenPunctuation {
		return false
	}

	// Always space between words/identifiers/keywords
	if isWordLike(prev) && isWordLike(tok) {
		return true
	}

	// Space between word and number
	if isWordLike(prev) && tok.Type == TokenNumber {
		return true
	}

	// Space after comma
	if prev.Value == "," {
		return true
	}

	// Space after closing paren if followed by word
	if prev.Value == ")" && isWordLike(tok) {
		return true
	}

	// Default: add space
	return true
}

// isWordLike returns true if the token is a word (keyword, identifier, function).
func isWordLike(tok Token) bool {
	return tok.Type == TokenKeyword || tok.Type == TokenIdentifier ||
		tok.Type == TokenFunction || tok.Type == TokenVariable
}

// CompactSQL condenses SQL to a single line, normalizing whitespace.
func CompactSQL(sql string) string {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return ""
	}

	t := NewTokenizer()
	tokens := t.Tokenize(sql)

	var sb strings.Builder
	prevEnd := 0

	for _, tok := range tokens {
		// Preserve gaps between tokens but normalize to single space
		if tok.Start > prevEnd {
			sb.WriteString(" ")
		}
		sb.WriteString(tok.Value)
		prevEnd = tok.End
	}

	return strings.TrimSpace(sb.String())
}

// isAlpha checks if a rune is a letter.
func isAlpha(r rune) bool {
	return unicode.IsLetter(r)
}

// FormatCreateTableSQL reformats the PARTITION clause in a CREATE TABLE
// statement so that each PARTITION definition appears on its own line.
// MySQL's SHOW CREATE TABLE returns all partition definitions on a single
// line, which is hard to read; this function inserts line breaks and
// indentation to make the output more legible.
func FormatCreateTableSQL(sql string) string {
	// Find "PARTITION BY" (case-insensitive)
	upper := strings.ToUpper(sql)
	idx := strings.Index(upper, "PARTITION BY")
	if idx == -1 {
		return sql
	}

	// Find the opening '(' after PARTITION BY ... that encloses the
	// partition list. We need to skip the function-call parens in
	// expressions like "PARTITION BY RANGE ( month(pay_time) ) ("
	parenStart := strings.IndexByte(sql[idx:], '(')
	if parenStart == -1 {
		return sql
	}
	parenStart += idx

	// Walk forward to find the matching '(' that starts the partition
	// list. The partition list opening paren is the one that appears
	// *after* the RANGE/ LIST/ KEY/ HASH expression closing paren.
	// Strategy: track depth; when depth returns to 0 after seeing at
	// least one nested paren, the *next* '(' at depth 0 is our target.
	depth := 0
	listOpen := -1
	for i := parenStart; i < len(sql); i++ {
		switch sql[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				// This closing paren ends the RANGE expression.
				// The next '(' starts the partition list.
				for j := i + 1; j < len(sql); j++ {
					if sql[j] == '(' {
						listOpen = j
						break
					}
					if !isSpace(rune(sql[j])) {
						// Unexpected character; give up
						return sql
					}
				}
				break
			}
		}
		if listOpen != -1 {
			break
		}
	}
	if listOpen == -1 {
		// Could not find partition list opening paren;
		// maybe the paren we found IS the list (e.g., KEY/HASH).
		// Heuristic: if the content after '(' contains "PARTITION ",
		// treat it as the list.
		listOpen = parenStart
	}

	// Find the matching closing paren
	depth = 0
	listClose := -1
	for i := listOpen; i < len(sql); i++ {
		switch sql[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				listClose = i
				break
			}
		}
	}
	if listClose == -1 {
		return sql
	}

	// Extract the partition definitions between the parens
	inner := sql[listOpen+1 : listClose]

	// Split on "PARTITION " boundaries (case-insensitive).
	// Each chunk starts with "PARTITION " or is the leading whitespace.
	parts := splitPartitionDefs(inner)

	if len(parts) <= 1 {
		return sql
	}

	// Rebuild with newlines and indentation
	var sb strings.Builder
	// Insert a newline before PARTITION BY so it starts on its own line.
	// Trim trailing spaces from the part before PARTITION BY so that
	// PARTITION BY begins at the start of the line (flush-left).
	beforePartition := strings.TrimRight(sql[:idx], " \t")
	sb.WriteString(beforePartition)
	sb.WriteString("\n")
	sb.WriteString(sql[idx:listOpen+1]) // PARTITION BY ... (
	for i, part := range parts {
		if i == 0 {
			// First part may be empty/whitespace before the first PARTITION
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				// Skip the empty leading whitespace; no blank line
				continue
			}
		}
		sb.WriteString("\n  ")
		sb.WriteString(strings.TrimSpace(part))
	}
	sb.WriteString("\n")            // newline before closing ')'
	sb.WriteString(sql[listClose:]) // closing ')' and everything after

	return sb.String()
}

// splitPartitionDefs splits the inner content of a PARTITION ... () clause
// into individual partition definitions. Each element starts with "PARTITION ".
func splitPartitionDefs(inner string) []string {
	upper := strings.ToUpper(inner)
	var parts []string
	start := 0

	i := 0
	for i < len(inner) {
		// Look for "PARTITION " keyword
		if i+9 <= len(inner) && upper[i:i+9] == "PARTITION" {
			// Check that it's followed by a space (not part of a longer word)
			if i+9 < len(inner) && isSpace(rune(inner[i+9])) {
				// Also check it's at a word boundary (preceded by whitespace or start)
				if i == 0 || isSpace(rune(inner[i-1])) || inner[i-1] == ',' {
					if start < i {
						parts = append(parts, inner[start:i])
					}
					start = i
					i += 9
					continue
				}
			}
		}
		i++
	}
	if start < len(inner) {
		parts = append(parts, inner[start:])
	}

	return parts
}

// isSpace checks if a rune is whitespace.
func isSpace(r rune) bool {
	return unicode.IsSpace(r)
}
