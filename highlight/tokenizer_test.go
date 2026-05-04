package highlight

import (
	"testing"
)

func TestTokenizerEmpty(t *testing.T) {
	tok := NewTokenizer()
	tokens := tok.Tokenize("")
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(tokens))
	}
}

func TestTokenizerKeyword(t *testing.T) {
	tok := NewTokenizer()
	tokens := tok.Tokenize("SELECT")
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	if tokens[0].Type != TokenKeyword {
		t.Errorf("expected Keyword, got %s", tokens[0].Type)
	}
	if tokens[0].Value != "SELECT" {
		t.Errorf("expected SELECT, got %s", tokens[0].Value)
	}
}

func TestTokenizerString(t *testing.T) {
	tok := NewTokenizer()
	tokens := tok.Tokenize("'hello world'")
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	if tokens[0].Type != TokenString {
		t.Errorf("expected String, got %s", tokens[0].Type)
	}
}

func TestTokenizerDoubleQuotedString(t *testing.T) {
	tok := NewTokenizer()
	tokens := tok.Tokenize(`"hello"`)
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	if tokens[0].Type != TokenString {
		t.Errorf("expected String, got %s", tokens[0].Type)
	}
}

func TestTokenizerNumber(t *testing.T) {
	tests := []struct {
		input string
		value string
	}{
		{"42", "42"},
		{"3.14", "3.14"},
		{"1e10", "1e10"},
		{"1.5e-3", "1.5e-3"},
	}
	for _, tt := range tests {
		tok := NewTokenizer()
		tokens := tok.Tokenize(tt.input)
		if len(tokens) != 1 {
			t.Fatalf("input=%q: expected 1 token, got %d", tt.input, len(tokens))
		}
		if tokens[0].Type != TokenNumber {
			t.Errorf("input=%q: expected Number, got %s", tt.input, tokens[0].Type)
		}
		if tokens[0].Value != tt.value {
			t.Errorf("input=%q: expected %s, got %s", tt.input, tt.value, tokens[0].Value)
		}
	}
}

func TestTokenizerLineComment(t *testing.T) {
	tok := NewTokenizer()
	tokens := tok.Tokenize("-- this is a comment")
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	if tokens[0].Type != TokenComment {
		t.Errorf("expected Comment, got %s", tokens[0].Type)
	}
}

func TestTokenizerHashComment(t *testing.T) {
	tok := NewTokenizer()
	tokens := tok.Tokenize("# this is a comment")
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	if tokens[0].Type != TokenComment {
		t.Errorf("expected Comment, got %s", tokens[0].Type)
	}
}

func TestTokenizerBlockComment(t *testing.T) {
	tok := NewTokenizer()
	tokens := tok.Tokenize("/* block comment */")
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	if tokens[0].Type != TokenComment {
		t.Errorf("expected Comment, got %s", tokens[0].Type)
	}
}

func TestTokenizerVariable(t *testing.T) {
	tests := []struct {
		input  string
		expect TokenType
		value  string
	}{
		{"@var", TokenVariable, "@var"},
		{"@@global", TokenVariable, "@@global"},
	}
	for _, tt := range tests {
		tok := NewTokenizer()
		tokens := tok.Tokenize(tt.input)
		if len(tokens) != 1 {
			t.Fatalf("input=%q: expected 1 token, got %d", tt.input, len(tokens))
		}
		if tokens[0].Type != tt.expect {
			t.Errorf("input=%q: expected %s, got %s", tt.input, tt.expect, tokens[0].Type)
		}
		if tokens[0].Value != tt.value {
			t.Errorf("input=%q: expected %s, got %s", tt.input, tt.value, tokens[0].Value)
		}
	}
}

func TestTokenizerFunction(t *testing.T) {
	tok := NewTokenizer()
	tokens := tok.Tokenize("COUNT(*)")
	if len(tokens) < 2 {
		t.Fatalf("expected at least 2 tokens, got %d", len(tokens))
	}
	if tokens[0].Type != TokenFunction {
		t.Errorf("expected Function, got %s", tokens[0].Type)
	}
	if tokens[0].Value != "COUNT" {
		t.Errorf("expected COUNT, got %s", tokens[0].Value)
	}
}

func TestTokenizerOperator(t *testing.T) {
	tok := NewTokenizer()
	tokens := tok.Tokenize("a = b")
	// Should have: Identifier, Operator, Identifier
	var foundOp bool
	for _, tok := range tokens {
		if tok.Type == TokenOperator && tok.Value == "=" {
			foundOp = true
		}
	}
	if !foundOp {
		t.Errorf("expected to find = operator")
	}
}

func TestTokenizerPunctuation(t *testing.T) {
	tok := NewTokenizer()
	tokens := tok.Tokenize("f(x, y)")
	var puncts []string
	for _, tok := range tokens {
		if tok.Type == TokenPunctuation {
			puncts = append(puncts, tok.Value)
		}
	}
	if len(puncts) < 3 {
		t.Errorf("expected at least 3 punctuation tokens, got %v", puncts)
	}
}

func TestTokenizerComplex(t *testing.T) {
	tok := NewTokenizer()
	tokens := tok.Tokenize("SELECT id, name FROM users WHERE id = 1")

	expectedTypes := []TokenType{
		TokenKeyword,    // SELECT
		TokenIdentifier, // id
		TokenPunctuation, // ,
		TokenIdentifier, // name
		TokenKeyword,    // FROM
		TokenIdentifier, // users
		TokenKeyword,    // WHERE
		TokenIdentifier, // id
		TokenOperator,   // =
		TokenNumber,     // 1
	}

	if len(tokens) != len(expectedTypes) {
		t.Fatalf("expected %d tokens, got %d: %+v", len(expectedTypes), len(tokens), tokens)
	}

	for i, tok := range tokens {
		if tok.Type != expectedTypes[i] {
			t.Errorf("token %d: expected %s, got %s (value=%q)", i, expectedTypes[i], tok.Type, tok.Value)
		}
	}
}

func TestTokenizerUnterminatedString(t *testing.T) {
	tok := NewTokenizer()
	// Should not panic on unterminated string
	tokens := tok.Tokenize("'unterminated")
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	if tokens[0].Type != TokenString {
		t.Errorf("expected String, got %s", tokens[0].Type)
	}
}

func TestTokenizerUnterminatedBlockComment(t *testing.T) {
	tok := NewTokenizer()
	// Should not panic on unterminated block comment
	tokens := tok.Tokenize("/* unterminated")
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	if tokens[0].Type != TokenComment {
		t.Errorf("expected Comment, got %s", tokens[0].Type)
	}
}

func TestTokenizerBacktickIdentifier(t *testing.T) {
	tok := NewTokenizer()
	tokens := tok.Tokenize("`table-name`")
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	if tokens[0].Type != TokenIdentifier {
		t.Errorf("expected Identifier, got %s", tokens[0].Type)
	}
}

func TestTokenizerEscapeInString(t *testing.T) {
	tok := NewTokenizer()
	tokens := tok.Tokenize(`'it\'s ok'`)
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	if tokens[0].Type != TokenString {
		t.Errorf("expected String, got %s", tokens[0].Type)
	}
}

func TestTokenizerComparisonOperators(t *testing.T) {
	tok := NewTokenizer()
	tokens := tok.Tokenize("a <> b != c <= d >= e")
	for _, tok := range tokens {
		if tok.Type == TokenOperator {
			if tok.Value != "<>" && tok.Value != "!=" && tok.Value != "<=" && tok.Value != ">=" {
				t.Errorf("unexpected operator: %q", tok.Value)
			}
		}
	}
}
