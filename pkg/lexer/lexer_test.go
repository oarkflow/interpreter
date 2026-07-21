package lexer

import (
	"testing"

	"github.com/oarkflow/interpreter/pkg/token"
)

func TestLexerSkipsSupportedCommentForms(t *testing.T) {
	input := `#!/usr/bin/env spl
# let ignored = ;
let a = 1; // let alsoIgnored = ;
/* let blockIgnored = ;
   /* nested ignored */
*/
let b = 2;
`
	l := NewLexer(input)
	want := []token.TokenType{
		token.LET, token.IDENT, token.ASSIGN, token.INT, token.SEMICOLON,
		token.LET, token.IDENT, token.ASSIGN, token.INT, token.SEMICOLON,
		token.EOF,
	}
	for i, typ := range want {
		got := l.NextToken()
		if got.Type != typ {
			t.Fatalf("token %d: expected %v, got %v (%q)", i, typ, got.Type, got.Literal)
		}
	}
}

func TestLexerReadsMultilineStringFormats(t *testing.T) {
	input := "let triple = \"\"\"hello\nworld\"\"\";\nlet single = '''raw\ntext''';\nlet doc = <<EOF\nalpha\nbeta\nEOF\n"
	l := NewLexer(input)
	var strings []string
	for {
		tok := l.NextToken()
		if tok.Type == token.EOF {
			break
		}
		if tok.Type == token.STRING {
			strings = append(strings, tok.Literal)
		}
	}
	want := []string{"hello\nworld", "raw\ntext", "alpha\nbeta\n"}
	if len(strings) != len(want) {
		t.Fatalf("expected %d strings, got %#v", len(want), strings)
	}
	for i := range want {
		if strings[i] != want[i] {
			t.Fatalf("string %d: expected %q, got %q", i, want[i], strings[i])
		}
	}
}

func TestLexerKeepsShiftOperatorsWhenNotHeredoc(t *testing.T) {
	l := NewLexer("let shifted = 1 << 2; let assigned = 1; assigned <<= 1;")
	seenShift := false
	seenShiftAssign := false
	for {
		tok := l.NextToken()
		if tok.Type == token.EOF {
			break
		}
		if tok.Type == token.LSHIFT {
			seenShift = true
		}
		if tok.Type == token.LSHIFT_ASSIGN {
			seenShiftAssign = true
		}
	}
	if !seenShift || !seenShiftAssign {
		t.Fatalf("expected shift operators, seenShift=%v seenShiftAssign=%v", seenShift, seenShiftAssign)
	}
}
