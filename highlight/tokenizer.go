package highlight

// TokenType represents the type of a SQL token.
type TokenType string

const (
	TokenKeyword    TokenType = "Keyword"
	TokenString     TokenType = "String"
	TokenNumber     TokenType = "Number"
	TokenComment    TokenType = "Comment"
	TokenIdentifier TokenType = "Identifier"
	TokenOperator   TokenType = "Operator"
	TokenFunction   TokenType = "Function"
	TokenVariable   TokenType = "Variable"
	TokenPunctuation TokenType = "Punctuation"
)

// Token represents a single SQL token with its position.
type Token struct {
	Type  TokenType
	Value string
	Start int
	End   int
}

// sqlKeywords is the set of MySQL keywords for tokenization.
var sqlKeywords = map[string]bool{
	// DML
	"SELECT": true, "FROM": true, "WHERE": true, "INSERT": true, "INTO": true,
	"UPDATE": true, "DELETE": true, "VALUES": true, "SET": true,
	// DDL
	"CREATE": true, "ALTER": true, "DROP": true, "TRUNCATE": true, "RENAME": true,
	"TABLE": true, "INDEX": true, "VIEW": true, "DATABASE": true, "SCHEMA": true,
	// Clauses
	"AND": true, "OR": true, "NOT": true, "IN": true, "IS": true, "NULL": true,
	"LIKE": true, "BETWEEN": true, "EXISTS": true, "AS": true, "ON": true,
	"JOIN": true, "INNER": true, "LEFT": true, "RIGHT": true, "OUTER": true,
	"CROSS": true, "FULL": true, "NATURAL": true, "USING": true,
	"GROUP": true, "BY": true, "ORDER": true, "ASC": true, "DESC": true,
	"HAVING": true, "LIMIT": true, "OFFSET": true, "UNION": true, "ALL": true,
	"DISTINCT": true, "DISTINCTROW": true,
	// Flow
	"IF": true, "ELSE": true, "ELSEIF": true, "END": true, "THEN": true,
	"CASE": true, "WHEN": true, "BEGIN": true, "COMMIT": true, "ROLLBACK": true,
	"START": true, "TRANSACTION": true,
	// Modifiers
	"PRIMARY": true, "KEY": true, "UNIQUE": true, "FOREIGN": true, "REFERENCES": true,
	"CONSTRAINT": true, "DEFAULT": true, "AUTO_INCREMENT": true,
	"UNSIGNED": true, "ZEROFILL": true,
	// Types
	"INT": true, "INTEGER": true, "TINYINT": true, "SMALLINT": true, "MEDIUMINT": true,
	"BIGINT": true, "FLOAT": true, "DOUBLE": true, "DECIMAL": true, "NUMERIC": true,
	"CHAR": true, "VARCHAR": true, "TEXT": true, "TINYTEXT": true, "MEDIUMTEXT": true,
	"LONGTEXT": true, "BLOB": true, "TINYBLOB": true, "MEDIUMBLOB": true, "LONGBLOB": true,
	"DATE": true, "DATETIME": true, "TIMESTAMP": true, "TIME": true, "YEAR": true,
	"BOOLEAN": true, "BOOL": true, "BINARY": true, "VARBINARY": true, "ENUM": true,
	"JSON": true, "GEOMETRY": true, "POINT": true, "LINESTRING": true, "POLYGON": true,
	// Other
	"SHOW": true, "DESCRIBE": true, "EXPLAIN": true, "USE": true,
	"GRANT": true, "REVOKE": true, "FLUSH": true, "KILL": true, "LOCK": true,
	"UNLOCK": true, "TABLES": true, "COLUMNS": true, "STATUS": true, "PROCESSLIST": true,
	"VARIABLES": true, "GLOBAL": true, "SESSION": true, "LOCAL": true,
	"WITH": true, "RECURSIVE": true, "OVER": true, "PARTITION": true, "WINDOW": true,
	"ROWS": true, "RANGE": true, "PRECEDING": true, "FOLLOWING": true, "CURRENT": true,
	"ROW": true, "UNBOUNDED": true,
	"EXCEPT": true, "INTERSECT": true, "MINUS": true,
	"RETURN": true, "RETURNS": true, "DETERMINISTIC": true, "READS": true,
	"MODIFIES": true, "DATA": true, "SQL": true, "SECURITY": true, "DEFINER": true,
	"INVOKER": true, "CASCADE": true, "RESTRICT": true, "NO": true, "ACTION": true,
	"ENGINE": true, "CHARSET": true, "COLLATE": true, "COMMENT": true,
	"REPLACE": true, "IGNORE": true, "DELAYED": true, "LOW_PRIORITY": true,
	"HIGH_PRIORITY": true, "QUICK": true, "EXTENDED": true,
}

// sqlFunctions is the set of MySQL built-in functions.
var sqlFunctions = map[string]bool{
	// Aggregate
	"COUNT": true, "SUM": true, "AVG": true, "MIN": true, "MAX": true,
	"GROUP_CONCAT": true, "STDDEV": true, "VARIANCE": true,
	// String
	"CONCAT": true, "CONCAT_WS": true, "LENGTH": true, "CHAR_LENGTH": true,
	"SUBSTRING": true, "SUBSTR": true, "TRIM": true, "LTRIM": true, "RTRIM": true,
	"UPPER": true, "LOWER": true, "REPLACE": true, "REVERSE": true,
	"LEFT": true, "RIGHT": true, "LPAD": true, "RPAD": true,
	"REPEAT": true, "SPACE": true, "STRCMP": true, "LOCATE": true,
	"INSTR": true, "FIELD": true, "ELT": true, "INSERT": false, // INSERT is keyword
	"FORMAT": true, "HEX": true, "UNHEX": true, "ORD": true,
	"ASCII": true, "BIN": true, "OCT": true,
	// Numeric
	"ABS": true, "CEIL": true, "CEILING": true, "FLOOR": true, "ROUND": true,
	"MOD": true, "POW": true, "POWER": true, "SQRT": true, "SIGN": true,
	"RAND": true, "TRUNCATE": false, // TRUNCATE is keyword
	// Date/Time
	"NOW": true, "CURDATE": true, "CURTIME": true, "DATE_ADD": true, "DATE_SUB": true,
	"DATEDIFF": true, "DATE_FORMAT": true, "STR_TO_DATE": true,
	"YEAR": true, "MONTH": true, "DAY": true, "HOUR": true, "MINUTE": true, "SECOND": true,
	"DAYOFYEAR": true, "DAYOFMONTH": true, "DAYOFWEEK": true, "WEEKDAY": true,
	"WEEK": true, "QUARTER": true, "LAST_DAY": true,
	"UNIX_TIMESTAMP": true, "FROM_UNIXTIME": true,
	"TIMESTAMPADD": true, "TIMESTAMPDIFF": true,
	// Control flow
	"IF": false, "CASE": false, // These are keywords
	"IFNULL": true, "NULLIF": true, "COALESCE": true,
	// Conversion
	"CAST": true, "CONVERT": true,
	// Info
	"DATABASE": false, "USER": true, "CURRENT_USER": true, "SYSTEM_USER": true,
	"SESSION_USER": true, "VERSION": true, "CONNECTION_ID": true,
	"LAST_INSERT_ID": true, "ROW_COUNT": true, "FOUND_ROWS": true,
	// JSON
	"JSON_ARRAY": true, "JSON_OBJECT": true, "JSON_EXTRACT": true,
	"JSON_CONTAINS": true, "JSON_CONTAINS_PATH": true,
	"JSON_SET": true, "JSON_INSERT": true, "JSON_REPLACE": true,
	"JSON_REMOVE": true, "JSON_MERGE": true, "JSON_MERGE_PRESERVE": true,
	"JSON_VALID": true, "JSON_TYPE": true, "JSON_KEYS": true,
	"JSON_SEARCH": true, "JSON_VALUE": true, "JSON_TABLE": true,
	"JSON_SCHEMA_VALID": true, "JSON_SCHEMA_VALIDATION_REPORT": true,
	// Window functions
	"ROW_NUMBER": true, "RANK": true, "DENSE_RANK": true,
	"NTILE": true, "LEAD": true, "LAG": true,
	"FIRST_VALUE": true, "LAST_VALUE": true, "NTH_VALUE": true,
	"PERCENT_RANK": true, "CUME_DIST": true,
}

// Tokenizer performs SQL tokenization with error tolerance.
type Tokenizer struct{}

// NewTokenizer creates a new Tokenizer instance.
func NewTokenizer() *Tokenizer {
	return &Tokenizer{}
}

// Tokenize tokenizes the input SQL string into a list of tokens.
// It is error-tolerant: incomplete SQL will still produce partial tokens.
// It never panics on invalid input.
func (t *Tokenizer) Tokenize(input string) []Token {
	var tokens []Token
	pos := 0
	runes := []rune(input)

	for pos < len(runes) {
		// Skip whitespace (but record position correctly)
		if isWhitespace(runes[pos]) {
			pos++
			continue
		}

		// Comment: -- or #
		if runes[pos] == '-' && pos+1 < len(runes) && runes[pos+1] == '-' {
			start := pos
			pos = scanLineComment(runes, pos)
			tokens = append(tokens, Token{
				Type:  TokenComment,
				Value: string(runes[start:pos]),
				Start: start,
				End:   pos,
			})
			continue
		}
		if runes[pos] == '#' {
			start := pos
			pos = scanLineComment(runes, pos)
			tokens = append(tokens, Token{
				Type:  TokenComment,
				Value: string(runes[start:pos]),
				Start: start,
				End:   pos,
			})
			continue
		}

		// Block comment: /* ... */
		if runes[pos] == '/' && pos+1 < len(runes) && runes[pos+1] == '*' {
			start := pos
			pos = scanBlockComment(runes, pos)
			tokens = append(tokens, Token{
				Type:  TokenComment,
				Value: string(runes[start:pos]),
				Start: start,
				End:   pos,
			})
			continue
		}

		// String literal: 'xxx' or "xxx"
		if runes[pos] == '\'' || runes[pos] == '"' {
			start := pos
			pos = scanString(runes, pos)
			tokens = append(tokens, Token{
				Type:  TokenString,
				Value: string(runes[start:pos]),
				Start: start,
				End:   pos,
			})
			continue
		}

		// Variable: @xxx or @@xxx
		if runes[pos] == '@' {
			start := pos
			pos = scanVariable(runes, pos)
			tokens = append(tokens, Token{
				Type:  TokenVariable,
				Value: string(runes[start:pos]),
				Start: start,
				End:   pos,
			})
			continue
		}

		// Number
		if isDigit(runes[pos]) || (runes[pos] == '.' && pos+1 < len(runes) && isDigit(runes[pos+1])) {
			start := pos
			pos = scanNumber(runes, pos)
			tokens = append(tokens, Token{
				Type:  TokenNumber,
				Value: string(runes[start:pos]),
				Start: start,
				End:   pos,
			})
			continue
		}

		// Punctuation
		if isPunctuation(runes[pos]) {
			start := pos
			tokens = append(tokens, Token{
				Type:  TokenPunctuation,
				Value: string(runes[pos : pos+1]),
				Start: start,
				End:   start + 1,
			})
			pos++
			continue
		}

		// Operator
		if isOperatorStart(runes[pos]) {
			start := pos
			pos = scanOperator(runes, pos)
			tokens = append(tokens, Token{
				Type:  TokenOperator,
				Value: string(runes[start:pos]),
				Start: start,
				End:   pos,
			})
			continue
		}

		// Identifier or keyword
		if isIdentStart(runes[pos]) {
			start := pos
			pos = scanIdentifier(runes, pos)
			word := string(runes[start:pos])
			upper := toUpper(word)

			tokenType := TokenIdentifier
			if sqlKeywords[upper] {
				tokenType = TokenKeyword
			} else if sqlFunctions[upper] {
				tokenType = TokenFunction
			}

			tokens = append(tokens, Token{
				Type:  tokenType,
				Value: word,
				Start: start,
				End:   pos,
			})
			continue
		}

		// Backtick-quoted identifier
		if runes[pos] == '`' {
			start := pos
			pos = scanBacktickIdentifier(runes, pos)
			tokens = append(tokens, Token{
				Type:  TokenIdentifier,
				Value: string(runes[start:pos]),
				Start: start,
				End:   pos,
			})
			continue
		}

		// Fallback: treat as identifier
		start := pos
		pos++
		tokens = append(tokens, Token{
			Type:  TokenIdentifier,
			Value: string(runes[start:pos]),
			Start: start,
			End:   pos,
		})
	}

	return tokens
}

// ---- scanning helpers ----

func isWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func isIdentStart(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_'
}

func isIdentContinue(r rune) bool {
	return isIdentStart(r) || isDigit(r)
}

func isPunctuation(r rune) bool {
	return r == '(' || r == ')' || r == ',' || r == ';' || r == '.' || r == '{' || r == '}'
}

func isOperatorStart(r rune) bool {
	return r == '=' || r == '<' || r == '>' || r == '!' || r == '|' || r == '&' || r == '+' || r == '-' || r == '*' || r == '/' || r == '%' || r == '^' || r == '~'
}

func toUpper(s string) string {
	runes := []rune(s)
	for i, r := range runes {
		if r >= 'a' && r <= 'z' {
			runes[i] = r - 32
		}
	}
	return string(runes)
}

func scanLineComment(runes []rune, pos int) int {
	for pos < len(runes) && runes[pos] != '\n' {
		pos++
	}
	return pos
}

func scanBlockComment(runes []rune, pos int) int {
	pos += 2 // skip /*
	for pos < len(runes)-1 {
		if runes[pos] == '*' && runes[pos+1] == '/' {
			return pos + 2
		}
		pos++
	}
	return len(runes) // unterminated comment: return end
}

func scanString(runes []rune, pos int) int {
	quote := runes[pos]
	pos++ // skip opening quote
	for pos < len(runes) {
		if runes[pos] == '\\' {
			pos += 2 // skip escape
			continue
		}
		if runes[pos] == quote {
			return pos + 1 // include closing quote
		}
		pos++
	}
	return pos // unterminated string
}

func scanVariable(runes []rune, pos int) int {
	pos++ // skip @
	if pos < len(runes) && runes[pos] == '@' {
		pos++ // skip second @
	}
	// Read identifier
	if pos < len(runes) && isIdentStart(runes[pos]) {
		for pos < len(runes) && isIdentContinue(runes[pos]) {
			pos++
		}
	}
	return pos
}

func scanNumber(runes []rune, pos int) int {
	// Integer part
	for pos < len(runes) && isDigit(runes[pos]) {
		pos++
	}
	// Decimal part
	if pos < len(runes) && runes[pos] == '.' {
		pos++
		for pos < len(runes) && isDigit(runes[pos]) {
			pos++
		}
	}
	// Exponent
	if pos < len(runes) && (runes[pos] == 'e' || runes[pos] == 'E') {
		pos++
		if pos < len(runes) && (runes[pos] == '+' || runes[pos] == '-') {
			pos++
		}
		for pos < len(runes) && isDigit(runes[pos]) {
			pos++
		}
	}
	return pos
}

func scanOperator(runes []rune, pos int) int {
	pos++
	if pos >= len(runes) {
		return pos
	}
	// Two-character operators
	switch {
	case runes[pos-1] == '<' && (runes[pos] == '=' || runes[pos] == '>' || runes[pos] == '<'):
		pos++
	case runes[pos-1] == '>' && (runes[pos] == '=' || runes[pos] == '>'):
		pos++
	case runes[pos-1] == '=' && runes[pos] == '=':
		pos++
	case runes[pos-1] == '!' && runes[pos] == '=':
		pos++
	case runes[pos-1] == '|' && runes[pos] == '|':
		pos++
	case runes[pos-1] == '&' && runes[pos] == '&':
		pos++
	case runes[pos-1] == '-' && runes[pos] == '-':
		// This is a comment, but we should have caught it earlier.
		// Treat as single operator.
	}
	return pos
}

func scanIdentifier(runes []rune, pos int) int {
	for pos < len(runes) && isIdentContinue(runes[pos]) {
		pos++
	}
	return pos
}

func scanBacktickIdentifier(runes []rune, pos int) int {
	pos++ // skip opening backtick
	for pos < len(runes) {
		if runes[pos] == '`' {
			pos++ // include closing backtick
			// Handle double backtick escape
			if pos < len(runes) && runes[pos] == '`' {
				pos++
				continue
			}
			return pos
		}
		pos++
	}
	return pos // unterminated
}
