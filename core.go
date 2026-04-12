package interpreter

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/oarkflow/squealx"
)

// Token types
type TokenType int

const (
	// Literals
	TOKEN_INT TokenType = iota
	TOKEN_FLOAT
	TOKEN_STRING
	TOKEN_IDENT
	TOKEN_TRUE
	TOKEN_FALSE
	TOKEN_NULL

	// Keywords
	TOKEN_LET
	TOKEN_IF
	TOKEN_ELSE
	TOKEN_WHILE
	TOKEN_FOR
	TOKEN_BREAK
	TOKEN_CONTINUE
	TOKEN_FUNCTION
	TOKEN_RETURN
	TOKEN_PRINT
	TOKEN_CONST
	TOKEN_IMPORT
	TOKEN_EXPORT
	TOKEN_TRY
	TOKEN_CATCH
	TOKEN_THROW
	TOKEN_SWITCH
	TOKEN_CASE
	TOKEN_DEFAULT
	TOKEN_IN
	TOKEN_DO
	TOKEN_TYPEOF
	TOKEN_MATCH
	TOKEN_ASYNC
	TOKEN_AWAIT
	TOKEN_INIT
	TOKEN_NEW

	// Operators
	TOKEN_ASSIGN
	TOKEN_PLUS
	TOKEN_MINUS
	TOKEN_MULTIPLY
	TOKEN_DIVIDE
	TOKEN_MODULO
	TOKEN_EQ
	TOKEN_NEQ
	TOKEN_LT
	TOKEN_GT
	TOKEN_LTE
	TOKEN_GTE
	TOKEN_AND
	TOKEN_OR
	TOKEN_NOT
	TOKEN_INCREMENT       // ++
	TOKEN_DECREMENT       // --
	TOKEN_PLUS_ASSIGN     // +=
	TOKEN_MINUS_ASSIGN    // -=
	TOKEN_MULTIPLY_ASSIGN // *=
	TOKEN_DIVIDE_ASSIGN   // /=
	TOKEN_MODULO_ASSIGN   // %=
	TOKEN_NULLISH         // ??
	TOKEN_NULLISH_ASSIGN  // ??=
	TOKEN_BITAND_ASSIGN   // &=
	TOKEN_BITOR_ASSIGN    // |=
	TOKEN_BITXOR_ASSIGN   // ^=
	TOKEN_LSHIFT_ASSIGN   // <<=
	TOKEN_RSHIFT_ASSIGN   // >>=
	TOKEN_POWER_ASSIGN    // **=
	TOKEN_AND_ASSIGN      // &&=
	TOKEN_OR_ASSIGN       // ||=
	TOKEN_PIPELINE        // |>

	// Delimiters
	TOKEN_LPAREN
	TOKEN_RPAREN
	TOKEN_LBRACE
	TOKEN_RBRACE
	TOKEN_LBRACKET
	TOKEN_RBRACKET
	TOKEN_COMMA
	TOKEN_SEMICOLON
	TOKEN_COLON
	TOKEN_DOT
	TOKEN_OPTIONAL_DOT // ?.
	TOKEN_SPREAD       // ...
	TOKEN_RANGE        // ..
	TOKEN_ARROW
	TOKEN_QUESTION

	TOKEN_BITAND // &
	TOKEN_BITOR  // |
	TOKEN_BITXOR // ^
	TOKEN_BITNOT // ~
	TOKEN_LSHIFT // <<
	TOKEN_RSHIFT // >>
	TOKEN_POWER  // **

	TOKEN_EOF
	TOKEN_ILLEGAL
)

type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

func tokenTypeName(t TokenType) string {
	switch t {
	case TOKEN_INT:
		return "integer"
	case TOKEN_FLOAT:
		return "float"
	case TOKEN_STRING:
		return "string"
	case TOKEN_IDENT:
		return "identifier"
	case TOKEN_TRUE:
		return "true"
	case TOKEN_FALSE:
		return "false"
	case TOKEN_NULL:
		return "null"
	case TOKEN_LET:
		return "let"
	case TOKEN_IF:
		return "if"
	case TOKEN_ELSE:
		return "else"
	case TOKEN_WHILE:
		return "while"
	case TOKEN_FOR:
		return "for"
	case TOKEN_BREAK:
		return "break"
	case TOKEN_CONTINUE:
		return "continue"
	case TOKEN_FUNCTION:
		return "function"
	case TOKEN_RETURN:
		return "return"
	case TOKEN_PRINT:
		return "print"
	case TOKEN_CONST:
		return "const"
	case TOKEN_IMPORT:
		return "import"
	case TOKEN_EXPORT:
		return "export"
	case TOKEN_TRY:
		return "try"
	case TOKEN_CATCH:
		return "catch"
	case TOKEN_THROW:
		return "throw"
	case TOKEN_SWITCH:
		return "switch"
	case TOKEN_CASE:
		return "case"
	case TOKEN_DEFAULT:
		return "default"
	case TOKEN_IN:
		return "in"
	case TOKEN_DO:
		return "do"
	case TOKEN_ASSIGN:
		return "="
	case TOKEN_PLUS:
		return "+"
	case TOKEN_MINUS:
		return "-"
	case TOKEN_MULTIPLY:
		return "*"
	case TOKEN_DIVIDE:
		return "/"
	case TOKEN_MODULO:
		return "%"
	case TOKEN_EQ:
		return "=="
	case TOKEN_NEQ:
		return "!="
	case TOKEN_LT:
		return "<"
	case TOKEN_GT:
		return ">"
	case TOKEN_LTE:
		return "<="
	case TOKEN_GTE:
		return ">="
	case TOKEN_AND:
		return "&&"
	case TOKEN_OR:
		return "||"
	case TOKEN_NOT:
		return "!"
	case TOKEN_INCREMENT:
		return "++"
	case TOKEN_DECREMENT:
		return "--"
	case TOKEN_PLUS_ASSIGN:
		return "+="
	case TOKEN_MINUS_ASSIGN:
		return "-="
	case TOKEN_MULTIPLY_ASSIGN:
		return "*="
	case TOKEN_DIVIDE_ASSIGN:
		return "/="
	case TOKEN_MODULO_ASSIGN:
		return "%="
	case TOKEN_NULLISH:
		return "??"
	case TOKEN_NULLISH_ASSIGN:
		return "??="
	case TOKEN_BITAND_ASSIGN:
		return "&="
	case TOKEN_BITOR_ASSIGN:
		return "|="
	case TOKEN_BITXOR_ASSIGN:
		return "^="
	case TOKEN_LSHIFT_ASSIGN:
		return "<<="
	case TOKEN_RSHIFT_ASSIGN:
		return ">>="
	case TOKEN_POWER_ASSIGN:
		return "**="
	case TOKEN_AND_ASSIGN:
		return "&&="
	case TOKEN_OR_ASSIGN:
		return "||="
	case TOKEN_PIPELINE:
		return "|>"
	case TOKEN_TYPEOF:
		return "typeof"
	case TOKEN_MATCH:
		return "match"
	case TOKEN_ASYNC:
		return "async"
	case TOKEN_AWAIT:
		return "await"
	case TOKEN_INIT:
		return "init"
	case TOKEN_NEW:
		return "new"
	case TOKEN_LPAREN:
		return "("
	case TOKEN_RPAREN:
		return ")"
	case TOKEN_LBRACE:
		return "{"
	case TOKEN_RBRACE:
		return "}"
	case TOKEN_LBRACKET:
		return "["
	case TOKEN_RBRACKET:
		return "]"
	case TOKEN_COMMA:
		return ","
	case TOKEN_SEMICOLON:
		return ";"
	case TOKEN_COLON:
		return ":"
	case TOKEN_DOT:
		return "."
	case TOKEN_OPTIONAL_DOT:
		return "?."
	case TOKEN_SPREAD:
		return "..."
	case TOKEN_RANGE:
		return ".."
	case TOKEN_ARROW:
		return "=>"
	case TOKEN_QUESTION:
		return "?"
	case TOKEN_BITAND:
		return "&"
	case TOKEN_BITOR:
		return "|"
	case TOKEN_BITXOR:
		return "^"
	case TOKEN_BITNOT:
		return "~"
	case TOKEN_LSHIFT:
		return "<<"
	case TOKEN_RSHIFT:
		return ">>"
	case TOKEN_POWER:
		return "**"
	case TOKEN_EOF:
		return "end of file"
	case TOKEN_ILLEGAL:
		return "invalid token"
	default:
		return fmt.Sprintf("token(%d)", t)
	}
}

func tokenDebug(tok Token) string {
	name := tokenTypeName(tok.Type)
	if tok.Literal != "" && tok.Type != TOKEN_EOF {
		return fmt.Sprintf("%s (%q)", name, tok.Literal)
	}
	return name
}

func expectedTokenHint(t TokenType) string {
	switch t {
	case TOKEN_SEMICOLON:
		return "Hint: a statement may be missing a trailing ';'."
	case TOKEN_RPAREN:
		return "Hint: check for missing ')' in expressions or function calls."
	case TOKEN_RBRACE:
		return "Hint: check for missing '}' to close a block."
	case TOKEN_RBRACKET:
		return "Hint: check for missing ']' in array expressions."
	case TOKEN_IDENT:
		return "Hint: an identifier (variable/function name) is expected here."
	default:
		return ""
	}
}

// Lexer
type Lexer struct {
	input        string
	position     int
	readPosition int
	ch           byte
	line         int
	column       int
}

func NewLexer(input string) *Lexer {
	l := &Lexer{input: input, line: 1, column: 0}
	l.readChar()
	return l
}

type LexerState struct {
	position     int
	readPosition int
	ch           byte
	line         int
	column       int
}

func (l *Lexer) SaveState() LexerState {
	return LexerState{l.position, l.readPosition, l.ch, l.line, l.column}
}

func (l *Lexer) RestoreState(s LexerState) {
	l.position = s.position
	l.readPosition = s.readPosition
	l.ch = s.ch
	l.line = s.line
	l.column = s.column
}

func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPosition]
	}
	l.position = l.readPosition
	l.readPosition++
	l.column++
	if l.ch == '\n' {
		l.line++
		l.column = 0
	}
}

func (l *Lexer) peekChar() byte {
	if l.readPosition >= len(l.input) {
		return 0
	}
	return l.input[l.readPosition]
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		l.readChar()
	}
}

func (l *Lexer) skipComment() {
	if l.ch == '/' && l.peekChar() == '/' {
		for l.ch != '\n' && l.ch != 0 {
			l.readChar()
		}
	} else if l.ch == '/' && l.peekChar() == '*' {
		l.readChar() // consume '/'
		l.readChar() // consume '*'
		for {
			if l.ch == 0 {
				break // unterminated comment
			}
			if l.ch == '*' && l.peekChar() == '/' {
				l.readChar() // consume '*'
				l.readChar() // consume '/'
				break
			}
			l.readChar()
		}
	}
}

func (l *Lexer) readIdentifier() string {
	position := l.position
	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' {
		l.readChar()
	}
	return l.input[position:l.position]
}
func (l *Lexer) readNumber() (TokenType, string) {
	position := l.position
	// Hex literal: 0x or 0X
	if l.ch == '0' && (l.peekChar() == 'x' || l.peekChar() == 'X') {
		l.readChar() // consume '0'
		l.readChar() // consume 'x'
		for isHexDigit(l.ch) || l.ch == '_' {
			l.readChar()
		}
		return TOKEN_INT, l.input[position:l.position]
	}
	// Binary literal: 0b or 0B
	if l.ch == '0' && (l.peekChar() == 'b' || l.peekChar() == 'B') {
		l.readChar() // consume '0'
		l.readChar() // consume 'b'
		for l.ch == '0' || l.ch == '1' || l.ch == '_' {
			l.readChar()
		}
		return TOKEN_INT, l.input[position:l.position]
	}
	// Octal literal: 0o or 0O
	if l.ch == '0' && (l.peekChar() == 'o' || l.peekChar() == 'O') {
		l.readChar() // consume '0'
		l.readChar() // consume 'o'
		for (l.ch >= '0' && l.ch <= '7') || l.ch == '_' {
			l.readChar()
		}
		return TOKEN_INT, l.input[position:l.position]
	}
	dotFound := false
	for isDigit(l.ch) || l.ch == '.' || l.ch == '_' {
		if l.ch == '.' {
			if dotFound {
				break
			}
			// Check for range operator '..'
			if l.peekChar() == '.' {
				break
			}
			dotFound = true
		}
		l.readChar()
	}
	if dotFound {
		return TOKEN_FLOAT, l.input[position:l.position]
	}
	return TOKEN_INT, l.input[position:l.position]
}

func (l *Lexer) readString(quote byte) string {
	start := l.position + 1
	hasEscape := false
	for {
		l.readChar()
		if l.ch == quote || l.ch == 0 {
			break
		}
		if l.ch == '\\' {
			hasEscape = true
			break
		}
	}

	if !hasEscape {
		return l.input[start:l.position]
	}

	var out strings.Builder
	out.Grow((l.position - start) + 8)
	out.WriteString(l.input[start:l.position])

	for {
		if l.ch == quote || l.ch == 0 {
			break
		}
		if l.ch == '\\' {
			l.readChar()
			switch l.ch {
			case 'n':
				out.WriteByte('\n')
			case 'r':
				out.WriteByte('\r')
			case 't':
				out.WriteByte('\t')
			case '"':
				out.WriteByte('"')
			case '\'':
				out.WriteByte('\'')
			case '\\':
				out.WriteByte('\\')
			default:
				out.WriteByte('\\')
				out.WriteByte(l.ch)
			}
		} else {
			out.WriteByte(l.ch)
		}
		l.readChar()
	}

	return out.String()
}

func (l *Lexer) readTemplateLiteral() string {
	var out strings.Builder
	l.readChar() // consume opening backtick
	for l.ch != '`' && l.ch != 0 {
		if l.ch == '$' && l.peekChar() == '{' {
			// ${expr} interpolation — we store it as ${...} for the parser to handle
			out.WriteString("${")
			l.readChar() // consume $
			l.readChar() // consume {
			depth := 1
			for depth > 0 && l.ch != 0 {
				if l.ch == '{' {
					depth++
				} else if l.ch == '}' {
					depth--
					if depth == 0 {
						break
					}
				}
				out.WriteByte(l.ch)
				l.readChar()
			}
			out.WriteByte('}')
			l.readChar() // consume closing }
		} else if l.ch == '\\' {
			l.readChar()
			switch l.ch {
			case 'n':
				out.WriteByte('\n')
			case 'r':
				out.WriteByte('\r')
			case 't':
				out.WriteByte('\t')
			case '`':
				out.WriteByte('`')
			case '\\':
				out.WriteByte('\\')
			case '$':
				out.WriteByte('$')
			default:
				out.WriteByte('\\')
				out.WriteByte(l.ch)
			}
			l.readChar()
		} else {
			out.WriteByte(l.ch)
			l.readChar()
		}
	}
	return out.String()
}

func (l *Lexer) NextToken() Token {
	var tok Token

	for {
		l.skipWhitespace()
		if l.ch == '/' && (l.peekChar() == '/' || l.peekChar() == '*') {
			l.skipComment()
			continue
		}
		break
	}

	tok.Line = l.line
	tok.Column = l.column

	switch l.ch {
	case '=':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_EQ, Literal: "=="}
		} else if l.peekChar() == '>' {
			l.readChar()
			tok = Token{Type: TOKEN_ARROW, Literal: "=>"}
		} else {
			tok = Token{Type: TOKEN_ASSIGN, Literal: "="}
		}
	case '+':
		if l.peekChar() == '+' {
			l.readChar()
			tok = Token{Type: TOKEN_INCREMENT, Literal: "++"}
		} else if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_PLUS_ASSIGN, Literal: "+="}
		} else {
			tok = Token{Type: TOKEN_PLUS, Literal: "+"}
		}
	case '-':
		if l.peekChar() == '-' {
			l.readChar()
			tok = Token{Type: TOKEN_DECREMENT, Literal: "--"}
		} else if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_MINUS_ASSIGN, Literal: "-="}
		} else {
			tok = Token{Type: TOKEN_MINUS, Literal: "-"}
		}
	case '*':
		if l.peekChar() == '*' {
			l.readChar()
			if l.peekChar() == '=' {
				l.readChar()
				tok = Token{Type: TOKEN_POWER_ASSIGN, Literal: "**="}
			} else {
				tok = Token{Type: TOKEN_POWER, Literal: "**"}
			}
		} else if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_MULTIPLY_ASSIGN, Literal: "*="}
		} else {
			tok = Token{Type: TOKEN_MULTIPLY, Literal: "*"}
		}
	case '/':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_DIVIDE_ASSIGN, Literal: "/="}
		} else {
			tok = Token{Type: TOKEN_DIVIDE, Literal: "/"}
		}
	case '%':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_MODULO_ASSIGN, Literal: "%="}
		} else {
			tok = Token{Type: TOKEN_MODULO, Literal: "%"}
		}
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_NEQ, Literal: "!="}
		} else {
			tok = Token{Type: TOKEN_NOT, Literal: "!"}
		}
	case '<':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_LTE, Literal: "<="}
		} else if l.peekChar() == '<' {
			l.readChar()
			if l.peekChar() == '=' {
				l.readChar()
				tok = Token{Type: TOKEN_LSHIFT_ASSIGN, Literal: "<<="}
			} else {
				tok = Token{Type: TOKEN_LSHIFT, Literal: "<<"}
			}
		} else {
			tok = Token{Type: TOKEN_LT, Literal: "<"}
		}
	case '>':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_GTE, Literal: ">="}
		} else if l.peekChar() == '>' {
			l.readChar()
			if l.peekChar() == '=' {
				l.readChar()
				tok = Token{Type: TOKEN_RSHIFT_ASSIGN, Literal: ">>="}
			} else {
				tok = Token{Type: TOKEN_RSHIFT, Literal: ">>"}
			}
		} else {
			tok = Token{Type: TOKEN_GT, Literal: ">"}
		}
	case '&':
		if l.peekChar() == '&' {
			l.readChar()
			if l.peekChar() == '=' {
				l.readChar()
				tok = Token{Type: TOKEN_AND_ASSIGN, Literal: "&&="}
			} else {
				tok = Token{Type: TOKEN_AND, Literal: "&&"}
			}
		} else if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_BITAND_ASSIGN, Literal: "&="}
		} else {
			tok = Token{Type: TOKEN_BITAND, Literal: "&"}
		}
	case '|':
		if l.peekChar() == '|' {
			l.readChar()
			if l.peekChar() == '=' {
				l.readChar()
				tok = Token{Type: TOKEN_OR_ASSIGN, Literal: "||="}
			} else {
				tok = Token{Type: TOKEN_OR, Literal: "||"}
			}
		} else if l.peekChar() == '>' {
			l.readChar()
			tok = Token{Type: TOKEN_PIPELINE, Literal: "|>"}
		} else if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_BITOR_ASSIGN, Literal: "|="}
		} else {
			tok = Token{Type: TOKEN_BITOR, Literal: "|"}
		}
	case '^':
		if l.peekChar() == '=' {
			l.readChar()
			tok = Token{Type: TOKEN_BITXOR_ASSIGN, Literal: "^="}
		} else {
			tok = Token{Type: TOKEN_BITXOR, Literal: "^"}
		}
	case '~':
		tok = Token{Type: TOKEN_BITNOT, Literal: "~"}
	case '(':
		tok = Token{Type: TOKEN_LPAREN, Literal: "("}
	case ')':
		tok = Token{Type: TOKEN_RPAREN, Literal: ")"}
	case '{':
		tok = Token{Type: TOKEN_LBRACE, Literal: "{"}
	case '}':
		tok = Token{Type: TOKEN_RBRACE, Literal: "}"}
	case '[':
		tok = Token{Type: TOKEN_LBRACKET, Literal: "["}
	case ']':
		tok = Token{Type: TOKEN_RBRACKET, Literal: "]"}
	case ',':
		tok = Token{Type: TOKEN_COMMA, Literal: ","}
	case ';':
		tok = Token{Type: TOKEN_SEMICOLON, Literal: ";"}
	case ':':
		tok = Token{Type: TOKEN_COLON, Literal: ":"}
	case '.':
		if l.peekChar() == '.' {
			l.readChar()
			if l.peekChar() == '.' {
				l.readChar()
				tok = Token{Type: TOKEN_SPREAD, Literal: "..."}
			} else {
				tok = Token{Type: TOKEN_RANGE, Literal: ".."}
			}
		} else {
			tok = Token{Type: TOKEN_DOT, Literal: "."}
		}
	case '?':
		if l.peekChar() == '?' {
			l.readChar()
			if l.peekChar() == '=' {
				l.readChar()
				tok = Token{Type: TOKEN_NULLISH_ASSIGN, Literal: "??="}
			} else {
				tok = Token{Type: TOKEN_NULLISH, Literal: "??"}
			}
		} else if l.peekChar() == '.' {
			l.readChar()
			tok = Token{Type: TOKEN_OPTIONAL_DOT, Literal: "?."}
		} else {
			tok = Token{Type: TOKEN_QUESTION, Literal: "?"}
		}
	case '`':
		tok.Type = TOKEN_STRING
		tok.Literal = l.readTemplateLiteral()
	case '"', '\'':
		tok.Type = TOKEN_STRING
		tok.Literal = l.readString(l.ch)
	case 0:
		tok.Literal = ""
		tok.Type = TOKEN_EOF
	default:
		if isLetter(l.ch) {
			tok.Literal = l.readIdentifier()
			tok.Type = lookupIdent(tok.Literal)
			return tok
		} else if isDigit(l.ch) {
			tok.Type, tok.Literal = l.readNumber()
			return tok
		} else {
			tok = Token{Type: TOKEN_ILLEGAL, Literal: tokenLiteralByte(l.ch)}
		}
	}

	l.readChar()
	return tok
}

func tokenLiteralByte(ch byte) string {
	if ch == 0 {
		return ""
	}
	return string([]byte{ch})
}

func isLetter(ch byte) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch == '_'
}

func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}

func isHexDigit(ch byte) bool {
	return ('0' <= ch && ch <= '9') || ('a' <= ch && ch <= 'f') || ('A' <= ch && ch <= 'F')
}

var keywordTokens = map[string]TokenType{
	"let":      TOKEN_LET,
	"if":       TOKEN_IF,
	"else":     TOKEN_ELSE,
	"while":    TOKEN_WHILE,
	"for":      TOKEN_FOR,
	"break":    TOKEN_BREAK,
	"continue": TOKEN_CONTINUE,
	"function": TOKEN_FUNCTION,
	"return":   TOKEN_RETURN,
	"const":    TOKEN_CONST,
	"import":   TOKEN_IMPORT,
	"export":   TOKEN_EXPORT,
	"try":      TOKEN_TRY,
	"catch":    TOKEN_CATCH,
	"throw":    TOKEN_THROW,
	"switch":   TOKEN_SWITCH,
	"case":     TOKEN_CASE,
	"default":  TOKEN_DEFAULT,
	"in":       TOKEN_IN,
	"do":       TOKEN_DO,
	"print":    TOKEN_PRINT,
	"true":     TOKEN_TRUE,
	"false":    TOKEN_FALSE,
	"null":     TOKEN_NULL,
	"typeof":   TOKEN_TYPEOF,
	"match":    TOKEN_MATCH,
	"async":    TOKEN_ASYNC,
	"await":    TOKEN_AWAIT,
	"init":     TOKEN_INIT,
	"new":      TOKEN_NEW,
	"type":     TOKEN_IDENT,
	"lazy":     TOKEN_IDENT,
}

func lookupIdent(ident string) TokenType {
	if tok, ok := keywordTokens[ident]; ok {
		return tok
	}
	return TOKEN_IDENT
}

// AST Nodes
type Node interface {
	String() string
}

type Expression interface {
	Node
	expressionNode()
}

type Statement interface {
	Node
	statementNode()
}

type Program struct {
	Statements []Statement
}

func (p *Program) String() string {
	var out strings.Builder
	for _, s := range p.Statements {
		out.WriteString(s.String())
	}
	return out.String()
}

type IntegerLiteral struct {
	Value int64
}

func (il *IntegerLiteral) expressionNode() {}
func (il *IntegerLiteral) String() string  { return fmt.Sprintf("%d", il.Value) }

type FloatLiteral struct {
	Value float64
}

func (fl *FloatLiteral) expressionNode() {}
func (fl *FloatLiteral) String() string  { return fmt.Sprintf("%g", fl.Value) }

type StringLiteral struct {
	Value string
}

func (sl *StringLiteral) expressionNode() {}
func (sl *StringLiteral) String() string  { return fmt.Sprintf("\"%s\"", sl.Value) }

type BooleanLiteral struct {
	Value bool
}

func (bl *BooleanLiteral) expressionNode() {}
func (bl *BooleanLiteral) String() string  { return fmt.Sprintf("%t", bl.Value) }

type NullLiteral struct{}

func (nl *NullLiteral) expressionNode() {}
func (nl *NullLiteral) String() string  { return "null" }

type Identifier struct {
	Name string
}

func (i *Identifier) expressionNode() {}
func (i *Identifier) String() string  { return i.Name }

type ArrayLiteral struct {
	Elements []Expression
}

func (al *ArrayLiteral) expressionNode() {}
func (al *ArrayLiteral) String() string {
	var out strings.Builder
	out.WriteString("[")
	for i, el := range al.Elements {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(el.String())
	}
	out.WriteString("]")
	return out.String()
}

type HashEntry struct {
	Key        Expression // nil when IsSpread is true
	Value      Expression // the spread source when IsSpread is true
	IsSpread   bool
	IsComputed bool
}

type HashLiteral struct {
	Entries []HashEntry
}

func (hl *HashLiteral) expressionNode() {}
func (hl *HashLiteral) String() string {
	var out strings.Builder
	pairs := []string{}
	for _, entry := range hl.Entries {
		if entry.IsSpread {
			pairs = append(pairs, "..."+entry.Value.String())
		} else {
			pairs = append(pairs, entry.Key.String()+":"+entry.Value.String())
		}
	}
	out.WriteString("{")
	out.WriteString(strings.Join(pairs, ", "))
	out.WriteString("}")
	return out.String()
}

type IndexExpression struct {
	Left  Expression
	Index Expression
}

func (ie *IndexExpression) expressionNode() {}
func (ie *IndexExpression) String() string {
	return fmt.Sprintf("(%s[%s])", ie.Left.String(), ie.Index.String())
}

type DotExpression struct {
	Left  Expression
	Right *Identifier // The property or method name
}

func (de *DotExpression) expressionNode() {}
func (de *DotExpression) String() string {
	return fmt.Sprintf("(%s.%s)", de.Left.String(), de.Right.String())
}

type OptionalDotExpression struct {
	Left  Expression
	Right *Identifier
}

func (od *OptionalDotExpression) expressionNode() {}
func (od *OptionalDotExpression) String() string {
	return "(" + od.Left.String() + "?." + od.Right.String() + ")"
}

type PrefixExpression struct {
	Operator string
	Right    Expression
}

func (pe *PrefixExpression) expressionNode() {}
func (pe *PrefixExpression) String() string {
	return fmt.Sprintf("(%s%s)", pe.Operator, pe.Right.String())
}

type PostfixExpression struct {
	Operator string     // "++" or "--"
	Target   Expression // Identifier, DotExpression, or IndexExpression
}

func (pe *PostfixExpression) expressionNode() {}
func (pe *PostfixExpression) String() string {
	return "(" + pe.Target.String() + pe.Operator + ")"
}

type InfixExpression struct {
	Left     Expression
	Operator string
	Right    Expression
}

func (ie *InfixExpression) expressionNode() {}
func (ie *InfixExpression) String() string {
	return fmt.Sprintf("(%s %s %s)", ie.Left.String(), ie.Operator, ie.Right.String())
}

type IfExpression struct {
	Condition   Expression
	Consequence *BlockStatement
	Alternative *BlockStatement
}

func (ie *IfExpression) expressionNode() {}
func (ie *IfExpression) String() string {
	return fmt.Sprintf("if %s %s else %s", ie.Condition.String(), ie.Consequence.String(), ie.Alternative.String())
}

type FunctionLiteral struct {
	Parameters []*Identifier
	ParamTypes []string     // optional static type names per parameter
	Defaults   []Expression // parallel to Parameters, nil means no default
	ReturnType string       // optional static return type
	HasRest    bool         // true if last param is ...rest
	Body       *BlockStatement
	IsArrow    bool // true for arrow functions
	IsAsync    bool // true for async functions
}

func (fl *FunctionLiteral) expressionNode() {}
func (fl *FunctionLiteral) String() string {
	var out strings.Builder
	out.WriteString("function(")
	for i, p := range fl.Parameters {
		if i > 0 {
			out.WriteString(", ")
		}
		if fl.HasRest && i == len(fl.Parameters)-1 {
			out.WriteString("...")
		}
		out.WriteString(p.String())
		if i < len(fl.ParamTypes) && fl.ParamTypes[i] != "" {
			out.WriteString(": ")
			out.WriteString(fl.ParamTypes[i])
		}
		if i < len(fl.Defaults) && fl.Defaults[i] != nil {
			out.WriteString(" = ")
			out.WriteString(fl.Defaults[i].String())
		}
	}
	out.WriteString(") ")
	if fl.ReturnType != "" {
		out.WriteString(": ")
		out.WriteString(fl.ReturnType)
		out.WriteString(" ")
	}
	out.WriteString(fl.Body.String())
	return out.String()
}

type SpreadExpression struct {
	Value Expression
}

func (se *SpreadExpression) expressionNode() {}
func (se *SpreadExpression) String() string  { return "..." + se.Value.String() }

type CallExpression struct {
	Function  Expression
	Arguments []Expression
	Line      int
	Column    int
}

func (ce *CallExpression) expressionNode() {}
func (ce *CallExpression) String() string {
	var out strings.Builder
	out.WriteString(ce.Function.String())
	out.WriteString("(")
	for i, a := range ce.Arguments {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(a.String())
	}
	out.WriteString(")")
	return out.String()
}

type AssignExpression struct {
	Target Expression // Identifier, DotExpression, or IndexExpression
	Value  Expression
}

func (ae *AssignExpression) expressionNode() {}
func (ae *AssignExpression) String() string {
	return fmt.Sprintf("%s = %s", ae.Target.String(), ae.Value.String())
}

type CompoundAssignExpression struct {
	Target   Expression // Identifier, DotExpression, or IndexExpression
	Operator string     // the underlying op: +, -, *, /, %, etc.
	Value    Expression
}

func (ca *CompoundAssignExpression) expressionNode() {}
func (ca *CompoundAssignExpression) String() string {
	return "(" + ca.Target.String() + " " + ca.Operator + "= " + ca.Value.String() + ")"
}

type LetStatement struct {
	Name     *Identifier   // Deprecated: use Names[0]
	Names    []*Identifier // Support for multi-assignment
	TypeName string        // optional static type name for single-name let/const
	Value    Expression
}

func (ls *LetStatement) statementNode() {}
func (ls *LetStatement) String() string {
	if ls.TypeName != "" {
		return fmt.Sprintf("let %s: %s = %s;", ls.Name.String(), ls.TypeName, ls.Value.String())
	}
	return fmt.Sprintf("let %s = %s;", ls.Name.String(), ls.Value.String())
}

type ClassStatement struct {
	Name    *Identifier
	Methods []*ClassMethod
}

type ClassMethod struct {
	Name       *Identifier
	Parameters []*Identifier
	Body       *BlockStatement
}

func (cs *ClassStatement) statementNode() {}
func (cs *ClassStatement) String() string {
	var out strings.Builder
	out.WriteString("class ")
	out.WriteString(cs.Name.String())
	out.WriteString(" {")
	for _, m := range cs.Methods {
		out.WriteByte(' ')
		out.WriteString(m.String())
	}
	out.WriteString(" }")
	return out.String()
}

func (cm *ClassMethod) String() string {
	var out strings.Builder
	out.WriteString(cm.Name.String())
	out.WriteString("(")
	for i, p := range cm.Parameters {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(p.String())
	}
	out.WriteString(") ")
	out.WriteString(cm.Body.String())
	return out.String()
}

type InterfaceStatement struct {
	Name    *Identifier
	Methods []*InterfaceMethod
}

type InterfaceMethod struct {
	Name       *Identifier
	Parameters []*Identifier
	ParamTypes []string
	ReturnType string
}

func (is *InterfaceStatement) statementNode() {}
func (is *InterfaceStatement) String() string {
	var out strings.Builder
	out.WriteString("interface ")
	out.WriteString(is.Name.String())
	out.WriteString(" {")
	for _, m := range is.Methods {
		out.WriteByte(' ')
		out.WriteString(m.String())
	}
	out.WriteString(" }")
	return out.String()
}

func (im *InterfaceMethod) String() string {
	var out strings.Builder
	out.WriteString(im.Name.String())
	out.WriteString("(")
	for i, p := range im.Parameters {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(p.String())
		if i < len(im.ParamTypes) && im.ParamTypes[i] != "" {
			out.WriteString(": ")
			out.WriteString(im.ParamTypes[i])
		}
	}
	out.WriteString(")")
	if im.ReturnType != "" {
		out.WriteString(": ")
		out.WriteString(im.ReturnType)
	}
	out.WriteString(";")
	return out.String()
}

type InitStatement struct {
	Body *BlockStatement
}

func (is *InitStatement) statementNode() {}
func (is *InitStatement) String() string {
	if is.Body == nil {
		return "init {}"
	}
	return "init " + is.Body.String()
}

type TestStatement struct {
	Name string
	Body *BlockStatement
}

func (ts *TestStatement) statementNode() {}
func (ts *TestStatement) String() string {
	if ts.Body == nil {
		return fmt.Sprintf("test %q {}", ts.Name)
	}
	return fmt.Sprintf("test %q %s", ts.Name, ts.Body.String())
}

type TypeDeclarationStatement struct {
	Name     *Identifier
	Variants []*ADTVariantDecl
}

type ADTVariantDecl struct {
	Name   *Identifier
	Fields []*Identifier
}

func (td *TypeDeclarationStatement) statementNode() {}
func (td *TypeDeclarationStatement) String() string {
	var out strings.Builder
	out.WriteString("type ")
	out.WriteString(td.Name.String())
	out.WriteString(" = ")
	for i, v := range td.Variants {
		if i > 0 {
			out.WriteString(" | ")
		}
		out.WriteString(v.Name.String())
		out.WriteString("(")
		for j, f := range v.Fields {
			if j > 0 {
				out.WriteString(", ")
			}
			out.WriteString(f.String())
		}
		out.WriteString(")")
	}
	out.WriteString(";")
	return out.String()
}

type ADTTypeDef struct {
	TypeName string
	Variants map[string]int
	Order    []string
}

func (a *ADTTypeDef) Type() ObjectType { return ADT_TYPE_OBJ }
func (a *ADTTypeDef) Inspect() string {
	if a == nil {
		return "<adt-type>"
	}
	return fmt.Sprintf("<adt %s variants=%d>", a.TypeName, len(a.Order))
}

type ADTValue struct {
	TypeName    string
	VariantName string
	FieldNames  []string
	Values      []Object
	AllVariants []string
}

func (a *ADTValue) Type() ObjectType { return ADT_VALUE_OBJ }
func (a *ADTValue) Inspect() string {
	if a == nil {
		return "<adt-value>"
	}
	parts := make([]string, 0, len(a.Values))
	for _, v := range a.Values {
		parts = append(parts, v.Inspect())
	}
	return fmt.Sprintf("%s(%s)", a.VariantName, strings.Join(parts, ", "))
}

type InterfaceLiteral struct {
	Methods map[string]*InterfaceMethod
}

func (il *InterfaceLiteral) Type() ObjectType { return INTERFACE_OBJ }
func (il *InterfaceLiteral) Inspect() string {
	if il == nil {
		return "<interface>"
	}
	return fmt.Sprintf("<interface methods=%d>", len(il.Methods))
}

type DestructurePattern struct {
	Kind     string        // "object" or "array"
	Keys     []Expression  // Object: StringLiteral keys; Array: unused
	Names    []*Identifier // Bound variable names
	Defaults []Expression  // Parallel to Names; nil = no default
	RestName *Identifier   // For ...rest (nil if absent)
}

type DestructureLetStatement struct {
	Pattern *DestructurePattern
	Value   Expression
	IsConst bool
}

func (d *DestructureLetStatement) statementNode() {}
func (d *DestructureLetStatement) String() string {
	return fmt.Sprintf("let <destructure> = %s;", d.Value.String())
}

type ReturnStatement struct {
	ReturnValue Expression
}

func (rs *ReturnStatement) statementNode() {}
func (rs *ReturnStatement) String() string {
	if rs.ReturnValue != nil {
		return fmt.Sprintf("return %s;", rs.ReturnValue.String())
	}
	return "return;"
}

type BreakStatement struct{}

func (bs *BreakStatement) statementNode() {}
func (bs *BreakStatement) String() string { return "break;" }

type ContinueStatement struct{}

func (cs *ContinueStatement) statementNode() {}
func (cs *ContinueStatement) String() string { return "continue;" }

type ExpressionStatement struct {
	Expression Expression
}

func (es *ExpressionStatement) statementNode() {}
func (es *ExpressionStatement) String() string {
	if es.Expression != nil {
		return es.Expression.String()
	}
	return ""
}

type BlockStatement struct {
	Statements []Statement
}

func (bs *BlockStatement) statementNode() {}
func (bs *BlockStatement) String() string {
	var out strings.Builder
	out.WriteString("{ ")
	for _, s := range bs.Statements {
		out.WriteString(s.String())
		out.WriteString(" ")
	}
	out.WriteString("}")
	return out.String()
}

type WhileStatement struct {
	Condition Expression
	Body      *BlockStatement
}

func (ws *WhileStatement) statementNode() {}
func (ws *WhileStatement) String() string {
	return fmt.Sprintf("while (%s) %s", ws.Condition.String(), ws.Body.String())
}

type DoWhileStatement struct {
	Body      *BlockStatement
	Condition Expression
}

func (dw *DoWhileStatement) statementNode() {}
func (dw *DoWhileStatement) String() string {
	return "do " + dw.Body.String() + " while (" + dw.Condition.String() + ")"
}

type ForStatement struct {
	Init      Statement
	Condition Expression
	Post      Statement
	Body      *BlockStatement
}

func (fs *ForStatement) statementNode() {}
func (fs *ForStatement) String() string {
	var out strings.Builder
	out.WriteString("for (")
	if fs.Init != nil {
		out.WriteString(fs.Init.String())
	} else {
		out.WriteString(";")
	}
	if fs.Condition != nil {
		out.WriteString(" " + fs.Condition.String() + ";")
	}
	if fs.Post != nil {
		out.WriteString(" " + fs.Post.String())
	}
	out.WriteString(") ")
	out.WriteString(fs.Body.String())
	return out.String()
}

type ForInStatement struct {
	KeyName   *Identifier // optional, for hash iteration
	ValueName *Identifier // the loop variable
	Iterable  Expression
	Body      *BlockStatement
}

func (fi *ForInStatement) statementNode() {}
func (fi *ForInStatement) String() string {
	var out strings.Builder
	out.WriteString("for (")
	if fi.KeyName != nil {
		out.WriteString(fi.KeyName.String())
		out.WriteString(", ")
	}
	out.WriteString(fi.ValueName.String())
	out.WriteString(" in ")
	out.WriteString(fi.Iterable.String())
	out.WriteString(") ")
	out.WriteString(fi.Body.String())
	return out.String()
}

type PrintStatement struct {
	Expression Expression
}

func (ps *PrintStatement) statementNode() {}
func (ps *PrintStatement) String() string {
	return fmt.Sprintf("print %s;", ps.Expression.String())
}

type ImportStatement struct {
	Path  Expression
	Alias *Identifier
	Names []*Identifier
}

func (is *ImportStatement) statementNode() {}
func (is *ImportStatement) String() string {
	if is.Path == nil {
		return "import;"
	}
	if len(is.Names) > 0 {
		parts := make([]string, 0, len(is.Names))
		for _, n := range is.Names {
			parts = append(parts, n.String())
		}
		return fmt.Sprintf("import {%s} from %s;", strings.Join(parts, ", "), is.Path.String())
	}
	if is.Alias != nil {
		return fmt.Sprintf("import %s as %s;", is.Path.String(), is.Alias.String())
	}
	return fmt.Sprintf("import %s;", is.Path.String())
}

type ExportStatement struct {
	Declaration Statement
	IsConst     bool
}

func (es *ExportStatement) statementNode() {}
func (es *ExportStatement) String() string {
	kind := "let"
	if es.IsConst {
		kind = "const"
	}
	if es.Declaration == nil {
		return fmt.Sprintf("export %s;", kind)
	}
	if ls, ok := es.Declaration.(*LetStatement); ok && len(ls.Names) > 0 {
		return fmt.Sprintf("export %s %s = %s;", kind, ls.Names[0].String(), ls.Value.String())
	}
	return fmt.Sprintf("export %s %s", kind, es.Declaration.String())
}

type ThrowStatement struct {
	Value Expression
}

func (ts *ThrowStatement) statementNode() {}
func (ts *ThrowStatement) String() string {
	if ts.Value == nil {
		return "throw;"
	}
	return fmt.Sprintf("throw %s;", ts.Value.String())
}

type TryCatchExpression struct {
	TryBlock     *BlockStatement
	CatchIdent   *Identifier
	CatchType    string
	CatchBlock   *BlockStatement
	FinallyBlock *BlockStatement
}

func (tce *TryCatchExpression) expressionNode() {}
func (tce *TryCatchExpression) String() string {
	var out strings.Builder
	out.WriteString("try ")
	out.WriteString(tce.TryBlock.String())
	if tce.CatchIdent != nil {
		if tce.CatchType != "" {
			fmt.Fprintf(&out, " catch (%s: %s) %s", tce.CatchIdent.String(), tce.CatchType, tce.CatchBlock.String())
		} else {
			fmt.Fprintf(&out, " catch (%s) %s", tce.CatchIdent.String(), tce.CatchBlock.String())
		}
	} else if tce.CatchBlock != nil {
		out.WriteString(" catch ")
		out.WriteString(tce.CatchBlock.String())
	}
	if tce.FinallyBlock != nil {
		out.WriteString(" finally ")
		out.WriteString(tce.FinallyBlock.String())
	}
	return out.String()
}

type TernaryExpression struct {
	Condition   Expression
	Consequence Expression
	Alternative Expression
}

func (te *TernaryExpression) expressionNode() {}
func (te *TernaryExpression) String() string {
	return "(" + te.Condition.String() + " ? " + te.Consequence.String() + " : " + te.Alternative.String() + ")"
}

type SwitchStatement struct {
	Value   Expression
	Cases   []*SwitchCase
	Default *BlockStatement
}

type SwitchCase struct {
	Values []Expression
	Body   *BlockStatement
}

func (ss *SwitchStatement) statementNode() {}
func (ss *SwitchStatement) String() string {
	var out strings.Builder
	out.WriteString("switch (")
	out.WriteString(ss.Value.String())
	out.WriteString(") { ")
	for _, c := range ss.Cases {
		out.WriteString("case ")
		for i, v := range c.Values {
			if i > 0 {
				out.WriteString(", ")
			}
			out.WriteString(v.String())
		}
		out.WriteString(": ")
		out.WriteString(c.Body.String())
	}
	if ss.Default != nil {
		out.WriteString("default: ")
		out.WriteString(ss.Default.String())
	}
	out.WriteString("}")
	return out.String()
}

// --- Pattern matching AST ---

// Pattern is the interface for all pattern types in match expressions.
type Pattern interface {
	patternNode()
	String() string
}

// LiteralPattern matches a specific literal value (42, "hello", true, null).
type LiteralPattern struct{ Value Expression }

func (p *LiteralPattern) patternNode()   {}
func (p *LiteralPattern) String() string { return p.Value.String() }

// WildcardPattern matches anything (_).
type WildcardPattern struct{}

func (p *WildcardPattern) patternNode()   {}
func (p *WildcardPattern) String() string { return "_" }

// BindingPattern binds the matched value to a variable, optionally with a type constraint.
type BindingPattern struct {
	Name     *Identifier
	TypeName string // "" if no type constraint
}

func (p *BindingPattern) patternNode() {}
func (p *BindingPattern) String() string {
	if p.TypeName != "" {
		return p.Name.String() + ": " + p.TypeName
	}
	return p.Name.String()
}

// ArrayPattern matches an array and destructures its elements.
type ArrayPattern struct {
	Elements []Pattern
	Rest     *Identifier // nil if no ...rest
}

func (p *ArrayPattern) patternNode() {}
func (p *ArrayPattern) String() string {
	var out strings.Builder
	out.WriteString("[")
	for i, el := range p.Elements {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(el.String())
	}
	if p.Rest != nil {
		if len(p.Elements) > 0 {
			out.WriteString(", ")
		}
		out.WriteString("..." + p.Rest.String())
	}
	out.WriteString("]")
	return out.String()
}

// ObjectPattern matches a hash and destructures its keys.
type ObjectPattern struct {
	Keys     []string  // field names
	Patterns []Pattern // corresponding patterns (BindingPattern or nested)
	Rest     *Identifier
}

func (p *ObjectPattern) patternNode() {}
func (p *ObjectPattern) String() string {
	var out strings.Builder
	out.WriteString("{")
	for i, key := range p.Keys {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(key + ": " + p.Patterns[i].String())
	}
	if p.Rest != nil {
		if len(p.Keys) > 0 {
			out.WriteString(", ")
		}
		out.WriteString("..." + p.Rest.String())
	}
	out.WriteString("}")
	return out.String()
}

// OrPattern matches if any sub-pattern matches (p1 | p2 | p3).
type OrPattern struct{ Patterns []Pattern }

func (p *OrPattern) patternNode() {}
func (p *OrPattern) String() string {
	parts := make([]string, len(p.Patterns))
	for i, pat := range p.Patterns {
		parts[i] = pat.String()
	}
	return strings.Join(parts, " | ")
}

// ExtractorPattern matches extractors like Some(x), None, All(...), Any(...), Tuple(...), Regex(...).
type ExtractorPattern struct {
	Name string    // "Some", "None", "Nil", "All", "Any", "Tuple", "Regex"
	Args []Pattern // inner patterns/args (nil for None/Nil without parens)
}

func (p *ExtractorPattern) patternNode() {}
func (p *ExtractorPattern) String() string {
	if p.Args == nil {
		return p.Name
	}
	parts := make([]string, len(p.Args))
	for i, a := range p.Args {
		parts[i] = a.String()
	}
	return p.Name + "(" + strings.Join(parts, ", ") + ")"
}

// ConstructorPattern matches ADT constructors like Ok(v), Err(e).
type ConstructorPattern struct {
	Name string
	Args []Pattern
}

func (p *ConstructorPattern) patternNode() {}
func (p *ConstructorPattern) String() string {
	parts := make([]string, len(p.Args))
	for i, a := range p.Args {
		parts[i] = a.String()
	}
	return p.Name + "(" + strings.Join(parts, ", ") + ")"
}

// RangePattern matches a value within a range (1..10, "a".."z").
type RangePattern struct {
	Low  Expression
	High Expression
}

func (p *RangePattern) patternNode()   {}
func (p *RangePattern) String() string { return p.Low.String() + ".." + p.High.String() }

// ComparisonPattern matches using a comparison operator (> 10, >= 0, != "x").
type ComparisonPattern struct {
	Operator TokenType
	Value    Expression
}

func (p *ComparisonPattern) patternNode() {}
func (p *ComparisonPattern) String() string {
	return tokenTypeName(p.Operator) + " " + p.Value.String()
}

// RangeExpression represents a range literal like 1..10 or "a".."z".
// When evaluated, it produces an Array of values in the range (inclusive both ends).
type RangeExpression struct {
	Left  Expression
	Right Expression
}

func (re *RangeExpression) expressionNode() {}
func (re *RangeExpression) String() string  { return re.Left.String() + ".." + re.Right.String() }

// MatchExpression represents a match expression/statement.
type MatchExpression struct {
	Value Expression
	Cases []*MatchCase
}

// MatchCase represents a single case arm in a match expression.
type MatchCase struct {
	Pattern Pattern
	Guard   Expression // nil if no guard
	Body    *BlockStatement
	Line    int
}

func (me *MatchExpression) expressionNode() {}
func (me *MatchExpression) statementNode()  {}
func (me *MatchExpression) String() string {
	var out strings.Builder
	out.WriteString("match (")
	out.WriteString(me.Value.String())
	out.WriteString(") { ")
	for _, c := range me.Cases {
		out.WriteString("case ")
		out.WriteString(c.Pattern.String())
		if c.Guard != nil {
			out.WriteString(" if ")
			out.WriteString(c.Guard.String())
		}
		out.WriteString(" => ")
		out.WriteString(c.Body.String())
		out.WriteString(" ")
	}
	out.WriteString("}")
	return out.String()
}

// AwaitExpression represents an await expr.
type AwaitExpression struct {
	Value Expression
}

func (ae *AwaitExpression) expressionNode() {}
func (ae *AwaitExpression) String() string  { return "await " + ae.Value.String() }

type TemplateLiteral struct {
	Parts []Expression // alternating StringLiteral and embedded expressions
}

func (tl *TemplateLiteral) expressionNode() {}
func (tl *TemplateLiteral) String() string {
	var out strings.Builder
	out.WriteString("`")
	for _, p := range tl.Parts {
		if sl, ok := p.(*StringLiteral); ok {
			out.WriteString(sl.Value)
		} else {
			out.WriteString("${")
			out.WriteString(p.String())
			out.WriteString("}")
		}
	}
	out.WriteString("`")
	return out.String()
}

type LazyExpression struct {
	Value Expression
}

func (le *LazyExpression) expressionNode() {}
func (le *LazyExpression) String() string {
	return "lazy " + le.Value.String()
}

type LazyValue struct {
	env       *Environment
	expr      Expression
	evaluated bool
	result    Object
	mu        sync.Mutex
}

type OwnedValue struct {
	ownerID string
	inner   Object
}

func (ov *OwnedValue) Type() ObjectType { return OWNED_OBJ }
func (ov *OwnedValue) Inspect() string {
	if ov == nil || ov.inner == nil {
		return "null"
	}
	return ov.inner.Inspect()
}

func (lv *LazyValue) Type() ObjectType { return LAZY_OBJ }
func (lv *LazyValue) Inspect() string {
	res := lv.Force()
	if res == nil {
		return "null"
	}
	return res.Inspect()
}

func (lv *LazyValue) Force() Object {
	if lv == nil {
		return NULL
	}
	lv.mu.Lock()
	defer lv.mu.Unlock()
	if lv.evaluated {
		return lv.result
	}
	if lv.expr == nil {
		lv.result = NULL
		lv.evaluated = true
		return lv.result
	}
	lv.result = Eval(lv.expr, lv.env)
	lv.evaluated = true
	if lv.result == nil {
		lv.result = NULL
	}
	return lv.result
}

// Parser
type Parser struct {
	l      *Lexer
	errors []string

	curToken  Token
	peekToken Token

	identIntern map[string]*Identifier
	initBlocks  []*InitStatement
}

type ParserState struct {
	curToken   Token
	peekToken  Token
	lexState   LexerState
	errorCount int
}

func (p *Parser) saveState() ParserState {
	return ParserState{p.curToken, p.peekToken, p.l.SaveState(), len(p.errors)}
}

func (p *Parser) restoreState(s ParserState) {
	p.curToken = s.curToken
	p.peekToken = s.peekToken
	p.l.RestoreState(s.lexState)
	p.errors = p.errors[:s.errorCount]
}

func normalizePos(line, col int, fallback Token) (int, int) {
	if line <= 0 {
		line = fallback.Line
	}
	if col <= 0 {
		col = fallback.Column
	}
	if line <= 0 {
		line = 1
	}
	if col < 0 {
		col = 0
	}
	return line, col
}

func lineContext(input string, line, col int) string {
	if line <= 0 {
		return ""
	}
	lines := strings.Split(input, "\n")
	if line > len(lines) {
		return ""
	}
	text := lines[line-1]
	if strings.TrimSpace(text) == "" {
		return ""
	}
	if col < 1 {
		col = 1
	}
	caretPad := strings.Repeat(" ", col-1)
	return fmt.Sprintf("Source: %s\n        %s^", text, caretPad)
}

func NewParser(l *Lexer) *Parser {
	p := &Parser{
		l:           l,
		errors:      []string{},
		identIntern: make(map[string]*Identifier, 128),
		initBlocks:  make([]*InitStatement, 0, 2),
	}
	p.nextToken()
	p.nextToken()
	return p
}

func (p *Parser) internIdentifier(name string) *Identifier {
	if ident, ok := p.identIntern[name]; ok {
		return ident
	}
	ident := &Identifier{Name: name}
	p.identIntern[name] = ident
	return ident
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
}

func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) peekError(t TokenType) {
	line, col := normalizePos(p.peekToken.Line, p.peekToken.Column, p.curToken)
	msg := fmt.Sprintf(
		"Line %d:%d -> expected %s, got %s.",
		line,
		col,
		tokenTypeName(t),
		tokenDebug(p.peekToken),
	)
	if hint := expectedTokenHint(t); hint != "" {
		msg += " " + hint
	}
	if ctx := lineContext(p.l.input, line, col); ctx != "" {
		msg += "\n" + ctx
	}
	p.errors = append(p.errors, msg)
}

func (p *Parser) curTokenIs(t TokenType) bool {
	return p.curToken.Type == t
}

func (p *Parser) peekTokenIs(t TokenType) bool {
	return p.peekToken.Type == t
}

func (p *Parser) expectPeek(t TokenType) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	}
	p.peekError(t)
	return false
}

func (p *Parser) ParseProgram() *Program {
	program := &Program{Statements: make([]Statement, 0, 64)}

	for !p.curTokenIs(TOKEN_EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
		}
		p.nextToken()
	}

	return program
}

func (p *Parser) parseStatement() Statement {
	switch p.curToken.Type {
	case TOKEN_LET:
		return p.parseLetStatement()
	case TOKEN_CONST:
		return p.parseConstStatement()
	case TOKEN_RETURN:
		return p.parseReturnStatement()
	case TOKEN_WHILE:
		return p.parseWhileStatement()
	case TOKEN_FOR:
		return p.parseForStatement()
	case TOKEN_BREAK:
		return p.parseBreakStatement()
	case TOKEN_CONTINUE:
		return p.parseContinueStatement()
	case TOKEN_PRINT:
		return p.parsePrintStatement()
	case TOKEN_IMPORT:
		return p.parseImportStatement()
	case TOKEN_EXPORT:
		return p.parseExportStatement()
	case TOKEN_THROW:
		return p.parseThrowStatement()
	case TOKEN_SWITCH:
		return p.parseSwitchStatement()
	case TOKEN_MATCH:
		return &ExpressionStatement{Expression: p.parseMatchExpression()}
	case TOKEN_DO:
		return p.parseDoWhileStatement()
	case TOKEN_FUNCTION:
		// Named function declaration: function foo(...) { ... }
		if p.peekTokenIs(TOKEN_IDENT) {
			return p.parseFunctionDeclaration()
		}
		return p.parseExpressionStatement()
	case TOKEN_INIT:
		return p.parseInitStatement()
	case TOKEN_IDENT:
		if p.curToken.Literal == "lazy" {
			return p.parseExpressionStatement()
		}
		if p.curToken.Literal == "type" && p.isTypeDeclarationAhead() {
			return p.parseTypeDeclarationStatement()
		}
		if p.curToken.Literal == "class" {
			return p.parseClassStatement()
		}
		if p.curToken.Literal == "interface" {
			return p.parseInterfaceStatement()
		}
		if p.curToken.Literal == "test" {
			return p.parseTestStatement()
		}
		return p.parseExpressionStatement()
	default:
		return p.parseExpressionStatement()
	}
}

func (p *Parser) isTypeDeclarationAhead() bool {
	if !p.curTokenIs(TOKEN_IDENT) || p.curToken.Literal != "type" {
		return false
	}
	saved := p.saveState()
	defer p.restoreState(saved)
	if !p.peekTokenIs(TOKEN_IDENT) {
		return false
	}
	p.nextToken()
	if !p.peekTokenIs(TOKEN_ASSIGN) {
		return false
	}
	return true
}

func (p *Parser) parseLetStatement() Statement {
	// Detect destructuring: let { or let [
	if p.peekTokenIs(TOKEN_LBRACE) || p.peekTokenIs(TOKEN_LBRACKET) {
		return p.parseDestructureLetStatement(false)
	}

	stmt := &LetStatement{}

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}

	// Always populate Names
	firstIdent := p.internIdentifier(p.curToken.Literal)
	stmt.Names = make([]*Identifier, 1, 2)
	stmt.Names[0] = firstIdent
	stmt.Name = firstIdent // Keep backward compat for now inside struct, though logic changes

	// Check for tuple assignment: let x, y = ...
	for p.peekTokenIs(TOKEN_COMMA) {
		p.nextToken() // consume comma
		if !p.expectPeek(TOKEN_IDENT) {
			return nil
		}
		stmt.Names = append(stmt.Names, p.internIdentifier(p.curToken.Literal))
	}

	if len(stmt.Names) == 1 && p.peekTokenIs(TOKEN_COLON) {
		p.nextToken()
		typeName := p.parseTypeName()
		if typeName == "" {
			return nil
		}
		stmt.TypeName = typeName
	}

	if !p.expectPeek(TOKEN_ASSIGN) {
		return nil
	}

	p.nextToken()

	stmt.Value = p.parseExpression(LOWEST)

	if p.peekTokenIs(TOKEN_SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseConstStatement() Statement {
	// Detect destructuring: const { or const [
	if p.peekTokenIs(TOKEN_LBRACE) || p.peekTokenIs(TOKEN_LBRACKET) {
		return p.parseDestructureLetStatement(true)
	}

	stmt := &LetStatement{} // Reuse LetStatement for now

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}

	ident := p.internIdentifier(p.curToken.Literal)
	stmt.Name = ident
	stmt.Names = []*Identifier{ident}
	if p.peekTokenIs(TOKEN_COLON) {
		p.nextToken()
		typeName := p.parseTypeName()
		if typeName == "" {
			return nil
		}
		stmt.TypeName = typeName
	}

	if !p.expectPeek(TOKEN_ASSIGN) {
		return nil
	}

	p.nextToken()

	stmt.Value = p.parseExpression(LOWEST)

	if p.peekTokenIs(TOKEN_SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseDestructureLetStatement(isConst bool) *DestructureLetStatement {
	stmt := &DestructureLetStatement{IsConst: isConst}
	p.nextToken() // advance to { or [
	if p.curTokenIs(TOKEN_LBRACE) {
		stmt.Pattern = p.parseObjectDestructurePattern()
	} else {
		stmt.Pattern = p.parseArrayDestructurePattern()
	}
	if stmt.Pattern == nil {
		return nil
	}
	if !p.expectPeek(TOKEN_ASSIGN) {
		return nil
	}
	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)
	if p.peekTokenIs(TOKEN_SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseObjectDestructurePattern() *DestructurePattern {
	pat := &DestructurePattern{Kind: "object"}
	for !p.peekTokenIs(TOKEN_RBRACE) {
		p.nextToken()
		if p.curTokenIs(TOKEN_SPREAD) {
			p.nextToken() // move to ident
			pat.RestName = p.internIdentifier(p.curToken.Literal)
			break
		}
		keyName := p.curToken.Literal
		key := &StringLiteral{Value: keyName}
		var bindName *Identifier
		if p.peekTokenIs(TOKEN_COLON) {
			p.nextToken() // consume :
			p.nextToken() // move to renamed ident
			bindName = p.internIdentifier(p.curToken.Literal)
		} else {
			bindName = p.internIdentifier(keyName)
		}
		var def Expression
		if p.peekTokenIs(TOKEN_ASSIGN) {
			p.nextToken() // consume =
			p.nextToken()
			def = p.parseExpression(LOWEST)
		}
		pat.Keys = append(pat.Keys, key)
		pat.Names = append(pat.Names, bindName)
		pat.Defaults = append(pat.Defaults, def)
		if !p.peekTokenIs(TOKEN_RBRACE) && !p.expectPeek(TOKEN_COMMA) {
			return nil
		}
	}
	if !p.expectPeek(TOKEN_RBRACE) {
		return nil
	}
	return pat
}

func (p *Parser) parseArrayDestructurePattern() *DestructurePattern {
	pat := &DestructurePattern{Kind: "array"}
	for !p.peekTokenIs(TOKEN_RBRACKET) {
		p.nextToken()
		if p.curTokenIs(TOKEN_SPREAD) {
			p.nextToken()
			pat.RestName = p.internIdentifier(p.curToken.Literal)
			break
		}
		ident := p.internIdentifier(p.curToken.Literal)
		var def Expression
		if p.peekTokenIs(TOKEN_ASSIGN) {
			p.nextToken()
			p.nextToken()
			def = p.parseExpression(LOWEST)
		}
		pat.Names = append(pat.Names, ident)
		pat.Defaults = append(pat.Defaults, def)
		if !p.peekTokenIs(TOKEN_RBRACKET) && !p.expectPeek(TOKEN_COMMA) {
			return nil
		}
	}
	if !p.expectPeek(TOKEN_RBRACKET) {
		return nil
	}
	return pat
}

func (p *Parser) parseReturnStatement() *ReturnStatement {
	stmt := &ReturnStatement{}

	p.nextToken()

	if p.curTokenIs(TOKEN_SEMICOLON) {
		return stmt
	}

	stmt.ReturnValue = p.parseExpression(LOWEST)

	if p.peekTokenIs(TOKEN_SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseWhileStatement() *WhileStatement {
	stmt := &WhileStatement{}

	if !p.expectPeek(TOKEN_LPAREN) {
		return nil
	}

	p.nextToken()
	stmt.Condition = p.parseExpression(LOWEST)

	if !p.expectPeek(TOKEN_RPAREN) {
		return nil
	}

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()

	return stmt
}

func (p *Parser) parseForStatement() Statement {
	if !p.expectPeek(TOKEN_LPAREN) {
		return nil
	}

	p.nextToken() // Consume (

	// Detect for-in: IDENT IN ... or IDENT , IDENT IN ...
	if p.curTokenIs(TOKEN_IDENT) {
		if p.peekTokenIs(TOKEN_IN) {
			// for (x in expr) { }
			valueName := p.internIdentifier(p.curToken.Literal)
			p.nextToken() // consume 'in'
			p.nextToken() // move to iterable
			iterable := p.parseExpression(LOWEST)
			if !p.expectPeek(TOKEN_RPAREN) {
				return nil
			}
			if !p.expectPeek(TOKEN_LBRACE) {
				return nil
			}
			return &ForInStatement{ValueName: valueName, Iterable: iterable, Body: p.parseBlockStatement()}
		}
		if p.peekTokenIs(TOKEN_COMMA) {
			// Could be for (k, v in expr) - save first ident
			firstIdent := p.internIdentifier(p.curToken.Literal)
			p.nextToken() // consume ','
			if p.peekTokenIs(TOKEN_IDENT) {
				p.nextToken() // on second ident
				secondIdent := p.internIdentifier(p.curToken.Literal)
				if p.peekTokenIs(TOKEN_IN) {
					p.nextToken() // consume 'in'
					p.nextToken() // on iterable
					iterable := p.parseExpression(LOWEST)
					if !p.expectPeek(TOKEN_RPAREN) {
						return nil
					}
					if !p.expectPeek(TOKEN_LBRACE) {
						return nil
					}
					return &ForInStatement{KeyName: firstIdent, ValueName: secondIdent, Iterable: iterable, Body: p.parseBlockStatement()}
				}
			}
			// Fall through - not a for-in
			_ = firstIdent
		}
	}

	// Regular C-style for loop. We already consumed the token after (
	stmt := &ForStatement{}
	if !p.curTokenIs(TOKEN_SEMICOLON) {
		stmt.Init = p.parseStatement()
		if p.curTokenIs(TOKEN_SEMICOLON) {
			p.nextToken()
		}
	} else {
		p.nextToken()
	}

	if !p.curTokenIs(TOKEN_SEMICOLON) {
		stmt.Condition = p.parseExpression(LOWEST)
	}
	if !p.expectPeek(TOKEN_SEMICOLON) {
		return nil
	}

	if !p.peekTokenIs(TOKEN_RPAREN) {
		p.nextToken()
		exp := p.parseExpression(LOWEST)
		stmt.Post = &ExpressionStatement{Expression: exp}
	}

	if !p.expectPeek(TOKEN_RPAREN) {
		return nil
	}

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()

	return stmt
}

func (p *Parser) parseBreakStatement() *BreakStatement {
	stmt := &BreakStatement{}
	p.nextToken() // consume break
	if p.peekTokenIs(TOKEN_SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseContinueStatement() *ContinueStatement {
	stmt := &ContinueStatement{}
	p.nextToken() // consume continue
	if p.peekTokenIs(TOKEN_SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parsePrintStatement() *PrintStatement {
	stmt := &PrintStatement{}

	p.nextToken()
	stmt.Expression = p.parseExpression(LOWEST)

	if p.peekTokenIs(TOKEN_SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseImportStatement() *ImportStatement {
	stmt := &ImportStatement{}

	if p.peekTokenIs(TOKEN_MULTIPLY) {
		p.nextToken()
		if !p.expectPeek(TOKEN_IDENT) || strings.ToLower(p.curToken.Literal) != "as" {
			p.errors = append(p.errors, fmt.Sprintf("Line %d: expected 'as' after import *", p.curToken.Line))
			return nil
		}
		if !p.expectPeek(TOKEN_IDENT) {
			return nil
		}
		stmt.Alias = p.internIdentifier(p.curToken.Literal)
		if !p.expectPeek(TOKEN_IDENT) || strings.ToLower(p.curToken.Literal) != "from" {
			p.errors = append(p.errors, fmt.Sprintf("Line %d: expected 'from' after import alias", p.curToken.Line))
			return nil
		}
		p.nextToken()
		stmt.Path = p.parseExpression(LOWEST)
		if p.peekTokenIs(TOKEN_SEMICOLON) {
			p.nextToken()
		}
		return stmt
	}

	if p.peekTokenIs(TOKEN_LBRACE) {
		p.nextToken()
		for {
			if !p.expectPeek(TOKEN_IDENT) {
				return nil
			}
			stmt.Names = append(stmt.Names, p.internIdentifier(p.curToken.Literal))
			if p.peekTokenIs(TOKEN_COMMA) {
				p.nextToken()
				continue
			}
			if !p.expectPeek(TOKEN_RBRACE) {
				return nil
			}
			break
		}

		if !p.expectPeek(TOKEN_IDENT) || strings.ToLower(p.curToken.Literal) != "from" {
			p.errors = append(p.errors, fmt.Sprintf("Line %d: expected 'from' after import list", p.curToken.Line))
			return nil
		}

		p.nextToken()
		stmt.Path = p.parseExpression(LOWEST)

		if p.peekTokenIs(TOKEN_SEMICOLON) {
			p.nextToken()
		}
		return stmt
	}

	p.nextToken()
	stmt.Path = p.parseExpression(LOWEST)

	if p.peekTokenIs(TOKEN_IDENT) {
		peek := strings.ToLower(p.peekToken.Literal)
		if peek == "as" {
			p.nextToken()
			if !p.expectPeek(TOKEN_IDENT) {
				return nil
			}
			stmt.Alias = p.internIdentifier(p.curToken.Literal)
		}
	}

	if p.peekTokenIs(TOKEN_SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseExportStatement() *ExportStatement {
	stmt := &ExportStatement{}

	if p.peekTokenIs(TOKEN_LET) {
		p.nextToken()
		stmt.Declaration = p.parseLetStatement()
		stmt.IsConst = false
		return stmt
	}

	if p.peekTokenIs(TOKEN_CONST) {
		p.nextToken()
		stmt.Declaration = p.parseConstStatement()
		stmt.IsConst = true
		return stmt
	}

	msg := fmt.Sprintf("Line %d: export must be followed by let or const", p.curToken.Line)
	p.errors = append(p.errors, msg)
	return nil
}

func (p *Parser) parseThrowStatement() *ThrowStatement {
	stmt := &ThrowStatement{}

	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)

	if p.peekTokenIs(TOKEN_SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseExpressionStatement() *ExpressionStatement {
	stmt := &ExpressionStatement{}

	stmt.Expression = p.parseExpression(LOWEST)

	if p.peekTokenIs(TOKEN_SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseBlockStatement() *BlockStatement {
	block := &BlockStatement{}
	block.Statements = []Statement{}

	p.nextToken()

	for !p.curTokenIs(TOKEN_RBRACE) && !p.curTokenIs(TOKEN_EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
		p.nextToken()
	}

	return block
}

const (
	_ int = iota
	LOWEST
	ASSIGN
	NULLISH_COALESCE
	LOGICAL_OR
	LOGICAL_AND
	BIT_OR
	BIT_XOR
	BIT_AND
	EQUALS
	LESSGREATER
	BIT_SHIFT
	SUM
	RANGE_PREC
	PRODUCT
	POWER
	PREFIX
	CALL
	INDEX
	POSTFIX
)

var precedences = map[TokenType]int{
	TOKEN_ASSIGN:          ASSIGN,
	TOKEN_QUESTION:        ASSIGN,
	TOKEN_PLUS_ASSIGN:     ASSIGN,
	TOKEN_MINUS_ASSIGN:    ASSIGN,
	TOKEN_MULTIPLY_ASSIGN: ASSIGN,
	TOKEN_DIVIDE_ASSIGN:   ASSIGN,
	TOKEN_MODULO_ASSIGN:   ASSIGN,
	TOKEN_NULLISH:         NULLISH_COALESCE,
	TOKEN_NULLISH_ASSIGN:  ASSIGN,
	TOKEN_BITAND_ASSIGN:   ASSIGN,
	TOKEN_BITOR_ASSIGN:    ASSIGN,
	TOKEN_BITXOR_ASSIGN:   ASSIGN,
	TOKEN_LSHIFT_ASSIGN:   ASSIGN,
	TOKEN_RSHIFT_ASSIGN:   ASSIGN,
	TOKEN_POWER_ASSIGN:    ASSIGN,
	TOKEN_AND_ASSIGN:      ASSIGN,
	TOKEN_OR_ASSIGN:       ASSIGN,
	TOKEN_PIPELINE:        ASSIGN,
	TOKEN_OR:              LOGICAL_OR,
	TOKEN_AND:             LOGICAL_AND,
	TOKEN_BITOR:           BIT_OR,
	TOKEN_BITXOR:          BIT_XOR,
	TOKEN_BITAND:          BIT_AND,
	TOKEN_EQ:              EQUALS,
	TOKEN_NEQ:             EQUALS,
	TOKEN_LT:              LESSGREATER,
	TOKEN_GT:              LESSGREATER,
	TOKEN_LTE:             LESSGREATER,
	TOKEN_GTE:             LESSGREATER,
	TOKEN_LSHIFT:          BIT_SHIFT,
	TOKEN_RSHIFT:          BIT_SHIFT,
	TOKEN_PLUS:            SUM,
	TOKEN_MINUS:           SUM,
	TOKEN_RANGE:           RANGE_PREC,
	TOKEN_MULTIPLY:        PRODUCT,
	TOKEN_DIVIDE:          PRODUCT,
	TOKEN_MODULO:          PRODUCT,
	TOKEN_POWER:           POWER,
	TOKEN_LPAREN:          CALL,
	TOKEN_LBRACKET:        INDEX,
	TOKEN_DOT:             INDEX,
	TOKEN_OPTIONAL_DOT:    INDEX,
	TOKEN_INCREMENT:       POSTFIX,
	TOKEN_DECREMENT:       POSTFIX,
	TOKEN_ARROW:           ASSIGN,
}

func (p *Parser) peekPrecedence() int {
	if p, ok := precedences[p.peekToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) curPrecedence() int {
	if p, ok := precedences[p.curToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) parseExpression(precedence int) Expression {
	var leftExp Expression
	switch p.curToken.Type {
	case TOKEN_IDENT:
		if p.curToken.Literal == "lazy" {
			leftExp = p.parseLazyExpression()
		} else {
			leftExp = p.parseIdentifier()
		}
	case TOKEN_INT:
		leftExp = p.parseIntegerLiteral()
	case TOKEN_FLOAT:
		leftExp = p.parseFloatLiteral()
	case TOKEN_STRING:
		leftExp = p.parseStringLiteral()
	case TOKEN_TRUE, TOKEN_FALSE:
		leftExp = p.parseBooleanLiteral()
	case TOKEN_NULL:
		leftExp = p.parseNullLiteral()
	case TOKEN_MINUS, TOKEN_NOT, TOKEN_BITNOT, TOKEN_TYPEOF:
		leftExp = p.parsePrefixExpression()
	case TOKEN_LPAREN:
		leftExp = p.parseGroupedExpression()
	case TOKEN_IF:
		leftExp = p.parseIfExpression()
	case TOKEN_FUNCTION:
		leftExp = p.parseFunctionLiteral()
	case TOKEN_LBRACKET:
		leftExp = p.parseArrayLiteral()
	case TOKEN_LBRACE:
		leftExp = p.parseHashLiteral()
	case TOKEN_TRY:
		leftExp = p.parseTryCatchExpression()
	case TOKEN_MATCH:
		leftExp = p.parseMatchExpression()
	case TOKEN_ASYNC:
		leftExp = p.parseAsyncExpression()
	case TOKEN_AWAIT:
		leftExp = p.parseAwaitExpression()
	case TOKEN_NEW:
		leftExp = p.parseNewExpression()
	default:
		p.noPrefixParseFnError(p.curToken.Type)
		return nil
	}

	for !p.peekTokenIs(TOKEN_SEMICOLON) && precedence < p.peekPrecedence() {
		switch p.peekToken.Type {
		case TOKEN_PLUS, TOKEN_MINUS, TOKEN_MULTIPLY, TOKEN_DIVIDE, TOKEN_MODULO,
			TOKEN_EQ, TOKEN_NEQ, TOKEN_LT, TOKEN_GT, TOKEN_LTE, TOKEN_GTE,
			TOKEN_AND, TOKEN_OR,
			TOKEN_BITAND, TOKEN_BITOR, TOKEN_BITXOR, TOKEN_LSHIFT, TOKEN_RSHIFT,
			TOKEN_NULLISH:
			p.nextToken()
			leftExp = p.parseInfixExpression(leftExp)
		case TOKEN_RANGE:
			p.nextToken()
			leftExp = p.parseRangeExpression(leftExp)
		case TOKEN_PIPELINE:
			p.nextToken()
			leftExp = p.parsePipelineExpression(leftExp)
		case TOKEN_POWER:
			p.nextToken()
			leftExp = p.parsePowerExpression(leftExp)
		case TOKEN_LPAREN:
			p.nextToken()
			leftExp = p.parseCallExpression(leftExp)
		case TOKEN_LBRACKET:
			p.nextToken()
			leftExp = p.parseIndexExpression(leftExp)
		case TOKEN_DOT:
			p.nextToken()
			leftExp = p.parseDotExpression(leftExp)
		case TOKEN_OPTIONAL_DOT:
			p.nextToken()
			leftExp = p.parseOptionalDotExpression(leftExp)
		case TOKEN_ASSIGN:
			p.nextToken()
			leftExp = p.parseAssignExpression(leftExp)
		case TOKEN_QUESTION:
			p.nextToken()
			leftExp = p.parseTernaryExpression(leftExp)
		case TOKEN_PLUS_ASSIGN, TOKEN_MINUS_ASSIGN, TOKEN_MULTIPLY_ASSIGN, TOKEN_DIVIDE_ASSIGN, TOKEN_MODULO_ASSIGN, TOKEN_NULLISH_ASSIGN,
			TOKEN_BITAND_ASSIGN, TOKEN_BITOR_ASSIGN, TOKEN_BITXOR_ASSIGN, TOKEN_LSHIFT_ASSIGN, TOKEN_RSHIFT_ASSIGN, TOKEN_POWER_ASSIGN,
			TOKEN_AND_ASSIGN, TOKEN_OR_ASSIGN:
			p.nextToken()
			leftExp = p.parseCompoundAssignExpression(leftExp)
		case TOKEN_INCREMENT, TOKEN_DECREMENT:
			p.nextToken()
			leftExp = p.parsePostfixExpression(leftExp)
		case TOKEN_ARROW:
			// Single-param arrow: x => expr
			p.nextToken() // curToken is now =>
			ident, ok := leftExp.(*Identifier)
			if !ok {
				p.errors = append(p.errors, fmt.Sprintf("Line %d: left side of => must be an identifier", p.curToken.Line))
				return nil
			}
			p.nextToken() // advance past =>
			lit := &FunctionLiteral{
				IsArrow:    true,
				Parameters: []*Identifier{ident},
				ParamTypes: []string{""},
				Defaults:   []Expression{nil},
			}
			if p.curTokenIs(TOKEN_LBRACE) {
				lit.Body = p.parseBlockStatement()
			} else {
				expr := p.parseExpression(LOWEST)
				lit.Body = &BlockStatement{
					Statements: []Statement{
						&ReturnStatement{ReturnValue: expr},
					},
				}
			}
			leftExp = lit
		default:
			return leftExp
		}
	}

	return leftExp
}

func (p *Parser) parsePostfixExpression(left Expression) Expression {
	switch left.(type) {
	case *Identifier, *DotExpression, *IndexExpression:
		// valid postfix target
	default:
		p.errors = append(p.errors, fmt.Sprintf("Line %d: postfix %s requires an identifier or property", p.curToken.Line, p.curToken.Literal))
		return nil
	}
	return &PostfixExpression{Operator: p.curToken.Literal, Target: left}
}

func (p *Parser) prefixParseFn() func() Expression {
	switch p.curToken.Type {
	case TOKEN_IDENT:
		return p.parseIdentifier
	case TOKEN_INT:
		return p.parseIntegerLiteral
	case TOKEN_FLOAT:
		return p.parseFloatLiteral
	case TOKEN_STRING:
		return p.parseStringLiteral
	case TOKEN_TRUE, TOKEN_FALSE:
		return p.parseBooleanLiteral
	case TOKEN_NULL:
		return p.parseNullLiteral
	case TOKEN_MINUS, TOKEN_NOT, TOKEN_BITNOT:
		return p.parsePrefixExpression
	case TOKEN_LPAREN:
		return p.parseGroupedExpression
	case TOKEN_IF:
		return p.parseIfExpression
	case TOKEN_FUNCTION:
		return p.parseFunctionLiteral
	case TOKEN_LBRACKET:
		return p.parseArrayLiteral
	case TOKEN_LBRACE:
		return p.parseHashLiteral
	case TOKEN_TRY:
		return p.parseTryCatchExpression
	case TOKEN_MATCH:
		return p.parseMatchExpression
	}
	return nil
}

func (p *Parser) parseTryCatchExpression() Expression {
	expr := &TryCatchExpression{}

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	expr.TryBlock = p.parseBlockStatement()

	// catch block (optional if finally is present)
	if p.peekTokenIs(TOKEN_CATCH) {
		p.nextToken()

		if !p.expectPeek(TOKEN_LPAREN) {
			return nil
		}
		if !p.expectPeek(TOKEN_IDENT) {
			return nil
		}
		expr.CatchIdent = p.internIdentifier(p.curToken.Literal)
		if p.peekTokenIs(TOKEN_COLON) {
			p.nextToken()
			expr.CatchType = p.parseTypeName()
			if expr.CatchType == "" {
				return nil
			}
		}

		if !p.expectPeek(TOKEN_RPAREN) {
			return nil
		}
		if !p.expectPeek(TOKEN_LBRACE) {
			return nil
		}

		expr.CatchBlock = p.parseBlockStatement()
	}

	// finally block (optional)
	if p.peekTokenIs(TOKEN_IDENT) && p.peekToken.Literal == "finally" {
		p.nextToken() // consume "finally"
		if !p.expectPeek(TOKEN_LBRACE) {
			return nil
		}
		expr.FinallyBlock = p.parseBlockStatement()
	}

	if expr.CatchBlock == nil && expr.FinallyBlock == nil {
		p.errors = append(p.errors, fmt.Sprintf("Line %d: expected catch or finally block after try", p.curToken.Line))
		return nil
	}

	return expr
}

func errorObjectToHash(obj Object) Object {
	errObj, ok := obj.(*Error)
	if !ok || errObj == nil {
		return &Hash{Pairs: map[HashKey]HashPair{}}
	}
	pairs := make(map[HashKey]HashPair, 5)
	put := func(key string, value Object) {
		k := &String{Value: key}
		pairs[k.HashKey()] = HashPair{Key: k, Value: value}
	}
	put("name", &String{Value: "Error"})
	put("message", &String{Value: errObj.Message})
	code := strings.TrimSpace(errObj.Code)
	if code == "" {
		code = "E_RUNTIME"
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(errObj.Message)), "identifier not found") {
			code = "E_NAME"
		}
	}
	put("code", &String{Value: code})
	if trace := formatCallStack(errObj.Stack); trace != "" {
		put("stack", &String{Value: trace})
	} else {
		put("stack", &String{Value: ""})
	}
	if errObj.Path != "" {
		put("path", &String{Value: errObj.Path})
	}
	if errObj.Line > 0 {
		put("line", integerObj(int64(errObj.Line)))
	}
	if errObj.Column > 0 {
		put("column", integerObj(int64(errObj.Column)))
	}
	return &Hash{Pairs: pairs}
}

func (p *Parser) parseTernaryExpression(condition Expression) Expression {
	expr := &TernaryExpression{Condition: condition}

	p.nextToken()
	expr.Consequence = p.parseExpression(LOWEST)

	if !p.expectPeek(TOKEN_COLON) {
		return nil
	}

	p.nextToken()
	expr.Alternative = p.parseExpression(LOWEST)

	return expr
}

func (p *Parser) parseSwitchStatement() *SwitchStatement {
	stmt := &SwitchStatement{}

	if !p.expectPeek(TOKEN_LPAREN) {
		return nil
	}
	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)
	if !p.expectPeek(TOKEN_RPAREN) {
		return nil
	}
	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}

	p.nextToken()

	for !p.curTokenIs(TOKEN_RBRACE) && !p.curTokenIs(TOKEN_EOF) {
		if p.curTokenIs(TOKEN_CASE) {
			sc := &SwitchCase{}
			p.nextToken()
			sc.Values = append(sc.Values, p.parseExpression(LOWEST))
			for p.peekTokenIs(TOKEN_COMMA) {
				p.nextToken()
				p.nextToken()
				sc.Values = append(sc.Values, p.parseExpression(LOWEST))
			}
			if !p.expectPeek(TOKEN_COLON) {
				return nil
			}
			sc.Body = p.parseSwitchCaseBody()
			stmt.Cases = append(stmt.Cases, sc)
		} else if p.curTokenIs(TOKEN_DEFAULT) {
			if !p.expectPeek(TOKEN_COLON) {
				return nil
			}
			stmt.Default = p.parseSwitchCaseBody()
		} else {
			p.nextToken()
		}
	}

	return stmt
}

func (p *Parser) parseSwitchCaseBody() *BlockStatement {
	block := &BlockStatement{Statements: []Statement{}}
	p.nextToken()
	for !p.curTokenIs(TOKEN_CASE) && !p.curTokenIs(TOKEN_DEFAULT) && !p.curTokenIs(TOKEN_RBRACE) && !p.curTokenIs(TOKEN_EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
		p.nextToken()
	}
	return block
}

// --- Async/Await parser ---

func (p *Parser) parseAsyncExpression() Expression {
	// curToken is TOKEN_ASYNC
	p.nextToken() // advance past 'async'

	if p.curTokenIs(TOKEN_FUNCTION) {
		lit := p.parseFunctionLiteral()
		if fl, ok := lit.(*FunctionLiteral); ok {
			fl.IsAsync = true
		}
		return lit
	}

	// async (params) => body  — async arrow function
	if p.curTokenIs(TOKEN_LPAREN) || p.curTokenIs(TOKEN_IDENT) {
		// Try parsing as expression (which may turn into arrow function)
		expr := p.parseExpression(LOWEST)
		if fl, ok := expr.(*FunctionLiteral); ok {
			fl.IsAsync = true
		}
		return expr
	}

	p.errors = append(p.errors, fmt.Sprintf("Line %d: expected function or arrow function after 'async'", p.curToken.Line))
	return nil
}

func (p *Parser) parseAwaitExpression() Expression {
	// curToken is TOKEN_AWAIT
	p.nextToken() // advance past 'await'
	value := p.parseExpression(PREFIX)
	return &AwaitExpression{Value: value}
}

func (p *Parser) parseLazyExpression() Expression {
	p.nextToken()
	value := p.parseExpression(PREFIX)
	return &LazyExpression{Value: value}
}

// --- Pattern matching parser ---

func (p *Parser) parseMatchExpression() Expression {
	expr := &MatchExpression{}

	if !p.expectPeek(TOKEN_LPAREN) {
		return nil
	}
	p.nextToken()
	expr.Value = p.parseExpression(LOWEST)
	if !p.expectPeek(TOKEN_RPAREN) {
		return nil
	}
	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}

	p.nextToken()
	for !p.curTokenIs(TOKEN_RBRACE) && !p.curTokenIs(TOKEN_EOF) {
		if !p.curTokenIs(TOKEN_CASE) {
			p.errors = append(p.errors, fmt.Sprintf("Line %d: expected 'case' in match expression, got %s", p.curToken.Line, tokenTypeName(p.curToken.Type)))
			p.nextToken()
			continue
		}
		mc := p.parseMatchCase()
		if mc != nil {
			expr.Cases = append(expr.Cases, mc)
		}
	}

	return expr
}

func (p *Parser) parseMatchCase() *MatchCase {
	mc := &MatchCase{Line: p.curToken.Line}

	p.nextToken() // skip 'case'
	mc.Pattern = p.parsePattern()
	if mc.Pattern == nil {
		return nil
	}

	// Check for OR patterns: case 1 | 2 | 3
	if p.peekTokenIs(TOKEN_BITOR) {
		patterns := []Pattern{mc.Pattern}
		for p.peekTokenIs(TOKEN_BITOR) {
			p.nextToken() // skip |
			p.nextToken() // move to next pattern
			pat := p.parseSinglePattern()
			if pat != nil {
				patterns = append(patterns, pat)
			}
		}
		mc.Pattern = &OrPattern{Patterns: patterns}
	}

	// Optional guard: if condition
	if p.peekTokenIs(TOKEN_IF) {
		p.nextToken() // skip 'if'
		p.nextToken()
		mc.Guard = p.parseExpression(ASSIGN)
	}

	// Expect =>
	if !p.expectPeek(TOKEN_ARROW) {
		return nil
	}

	// Parse body: either a block { ... } or a single expression
	if p.peekTokenIs(TOKEN_LBRACE) {
		p.nextToken()
		mc.Body = p.parseBlockStatement()
	} else {
		p.nextToken()
		stmt := p.parseStatement()
		if stmt != nil {
			mc.Body = &BlockStatement{
				Statements: []Statement{stmt},
			}
		} else {
			mc.Body = &BlockStatement{Statements: []Statement{}}
		}
	}

	// Advance past semicolons/newlines to next case or }
	for p.peekTokenIs(TOKEN_SEMICOLON) {
		p.nextToken()
	}
	if p.peekTokenIs(TOKEN_CASE) || p.peekTokenIs(TOKEN_RBRACE) {
		p.nextToken()
	}

	return mc
}

func (p *Parser) parsePattern() Pattern {
	pat := p.parseSinglePattern()
	if pat == nil {
		return nil
	}
	// Check for OR patterns at this level too (for direct parsePattern calls)
	// But we handle OR in parseMatchCase to avoid ambiguity with guards
	return pat
}

var extractorNames = map[string]bool{
	"Some": true, "None": true, "Nil": true,
	"All": true, "Any": true, "Tuple": true, "Regex": true,
}

func (p *Parser) parseSinglePattern() Pattern {
	switch p.curToken.Type {
	case TOKEN_INT:
		lit := p.parseIntegerLiteral()
		if p.peekTokenIs(TOKEN_RANGE) {
			p.nextToken() // skip ..
			p.nextToken()
			high := p.parseExpression(ASSIGN)
			return &RangePattern{Low: lit, High: high}
		}
		return &LiteralPattern{Value: lit}
	case TOKEN_FLOAT:
		lit := p.parseFloatLiteral()
		if p.peekTokenIs(TOKEN_RANGE) {
			p.nextToken() // skip ..
			p.nextToken()
			high := p.parseExpression(ASSIGN)
			return &RangePattern{Low: lit, High: high}
		}
		return &LiteralPattern{Value: lit}
	case TOKEN_STRING:
		lit := p.parseStringLiteral()
		if p.peekTokenIs(TOKEN_RANGE) {
			p.nextToken() // skip ..
			p.nextToken()
			high := p.parseExpression(ASSIGN)
			return &RangePattern{Low: lit, High: high}
		}
		return &LiteralPattern{Value: lit}
	case TOKEN_TRUE, TOKEN_FALSE:
		return &LiteralPattern{Value: p.parseBooleanLiteral()}
	case TOKEN_NULL:
		return &LiteralPattern{Value: p.parseNullLiteral()}
	case TOKEN_MINUS:
		// Negative number literal: case -1 =>
		return &LiteralPattern{Value: p.parsePrefixExpression()}
	case TOKEN_LBRACKET:
		return p.parseArrayPattern()
	case TOKEN_LBRACE:
		return p.parseObjectPattern()
	case TOKEN_GT, TOKEN_GTE, TOKEN_LT, TOKEN_LTE, TOKEN_NEQ:
		return p.parseComparisonPattern()
	case TOKEN_IDENT:
		name := p.curToken.Literal

		// Wildcard
		if name == "_" {
			// Check for _: type
			if p.peekTokenIs(TOKEN_COLON) {
				p.nextToken() // skip :
				p.nextToken()
				typeName := p.curToken.Literal
				return &BindingPattern{Name: &Identifier{Name: "_"}, TypeName: typeName}
			}
			return &WildcardPattern{}
		}

		// Extractor patterns: Some(x), None, Nil, All(...), Any(...), Tuple(...), Regex(...)
		if extractorNames[name] {
			if p.peekTokenIs(TOKEN_LPAREN) {
				p.nextToken() // skip (
				return p.parseExtractorPattern(name)
			}
			// None and Nil without parens
			if name == "None" || name == "Nil" {
				return &ExtractorPattern{Name: name}
			}
		}

		if p.peekTokenIs(TOKEN_LPAREN) {
			p.nextToken()
			return p.parseConstructorPattern(name)
		}

		// Range pattern: identifier..expr (unlikely but handle ident case)
		if p.peekTokenIs(TOKEN_RANGE) {
			lowExpr := &Identifier{Name: name}
			p.nextToken() // skip ..
			p.nextToken()
			highExpr := p.parseExpression(ASSIGN)
			return &RangePattern{Low: lowExpr, High: highExpr}
		}

		// Type pattern: x: integer
		if p.peekTokenIs(TOKEN_COLON) {
			p.nextToken() // skip :
			p.nextToken()
			typeName := p.curToken.Literal
			return &BindingPattern{Name: &Identifier{Name: name}, TypeName: typeName}
		}

		// Simple binding pattern
		return &BindingPattern{Name: &Identifier{Name: name}}
	}

	// Literal followed by range: 1..10
	// Already handled in INT/FLOAT/STRING cases above, but range needs special handling
	p.errors = append(p.errors, fmt.Sprintf("Line %d: unexpected token in pattern: %s", p.curToken.Line, p.curToken.Literal))
	return nil
}

func (p *Parser) parseArrayPattern() Pattern {
	pat := &ArrayPattern{}
	p.nextToken() // skip [

	for !p.curTokenIs(TOKEN_RBRACKET) && !p.curTokenIs(TOKEN_EOF) {
		// Check for rest: ...name
		if p.curTokenIs(TOKEN_SPREAD) {
			if !p.expectPeek(TOKEN_IDENT) {
				return nil
			}
			pat.Rest = &Identifier{Name: p.curToken.Literal}
			if p.peekTokenIs(TOKEN_COMMA) {
				p.nextToken()
			}
			p.nextToken()
			continue
		}

		elem := p.parseSinglePattern()
		if elem != nil {
			pat.Elements = append(pat.Elements, elem)
		}
		if p.peekTokenIs(TOKEN_COMMA) {
			p.nextToken()
		}
		p.nextToken()
	}

	return pat
}

func (p *Parser) parseObjectPattern() Pattern {
	pat := &ObjectPattern{}
	p.nextToken() // skip {

	for !p.curTokenIs(TOKEN_RBRACE) && !p.curTokenIs(TOKEN_EOF) {
		// Check for rest: ...name
		if p.curTokenIs(TOKEN_SPREAD) {
			if !p.expectPeek(TOKEN_IDENT) {
				return nil
			}
			pat.Rest = &Identifier{Name: p.curToken.Literal}
			if p.peekTokenIs(TOKEN_COMMA) {
				p.nextToken()
			}
			p.nextToken()
			continue
		}

		if !p.curTokenIs(TOKEN_IDENT) && !p.curTokenIs(TOKEN_STRING) {
			p.errors = append(p.errors, fmt.Sprintf("Line %d: expected key in object pattern, got %s", p.curToken.Line, p.curToken.Literal))
			p.nextToken()
			continue
		}
		key := p.curToken.Literal

		if p.peekTokenIs(TOKEN_COLON) {
			p.nextToken() // skip :
			p.nextToken()
			// Value can be a literal pattern (for matching) or a binding pattern (for renaming)
			valuePat := p.parseSinglePattern()
			pat.Keys = append(pat.Keys, key)
			pat.Patterns = append(pat.Patterns, valuePat)
		} else {
			// Shorthand: {name} means key "name" bound to variable "name"
			pat.Keys = append(pat.Keys, key)
			pat.Patterns = append(pat.Patterns, &BindingPattern{Name: &Identifier{Name: key}})
		}

		if p.peekTokenIs(TOKEN_COMMA) {
			p.nextToken()
		}
		p.nextToken()
	}

	return pat
}

func (p *Parser) parseExtractorPattern(name string) Pattern {
	pat := &ExtractorPattern{Name: name}

	p.nextToken() // skip (
	for !p.curTokenIs(TOKEN_RPAREN) && !p.curTokenIs(TOKEN_EOF) {
		arg := p.parseSinglePattern()
		if arg != nil {
			pat.Args = append(pat.Args, arg)
		}
		if p.peekTokenIs(TOKEN_COMMA) {
			p.nextToken()
		}
		p.nextToken()
	}

	return pat
}

func (p *Parser) parseConstructorPattern(name string) Pattern {
	pat := &ConstructorPattern{Name: name}
	p.nextToken() // skip (
	for !p.curTokenIs(TOKEN_RPAREN) && !p.curTokenIs(TOKEN_EOF) {
		arg := p.parseSinglePattern()
		if arg != nil {
			pat.Args = append(pat.Args, arg)
		}
		if p.peekTokenIs(TOKEN_COMMA) {
			p.nextToken()
		}
		p.nextToken()
	}
	return pat
}

func (p *Parser) parseComparisonPattern() Pattern {
	op := p.curToken.Type
	p.nextToken()
	value := p.parseExpression(PREFIX)
	return &ComparisonPattern{Operator: op, Value: value}
}

func (p *Parser) parseDoWhileStatement() *DoWhileStatement {
	stmt := &DoWhileStatement{}

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	stmt.Body = p.parseBlockStatement()

	if !p.expectPeek(TOKEN_WHILE) {
		p.errors = append(p.errors, fmt.Sprintf("Line %d: expected 'while' after do block", p.curToken.Line))
		return nil
	}

	if !p.expectPeek(TOKEN_LPAREN) {
		return nil
	}
	p.nextToken()
	stmt.Condition = p.parseExpression(LOWEST)
	if !p.expectPeek(TOKEN_RPAREN) {
		return nil
	}

	if p.peekTokenIs(TOKEN_SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseOptionalDotExpression(left Expression) Expression {
	exp := &OptionalDotExpression{Left: left}
	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	exp.Right = p.internIdentifier(p.curToken.Literal)
	return exp
}

func (p *Parser) parsePowerExpression(left Expression) Expression {
	expression := &InfixExpression{
		Operator: p.curToken.Literal,
		Left:     left,
	}

	precedence := p.curPrecedence() - 1 // right-associative
	p.nextToken()
	expression.Right = p.parseExpression(precedence)

	return expression
}

func (p *Parser) infixParseFn() func(Expression) Expression {
	switch p.peekToken.Type {
	case TOKEN_PLUS, TOKEN_MINUS, TOKEN_MULTIPLY, TOKEN_DIVIDE, TOKEN_MODULO,
		TOKEN_EQ, TOKEN_NEQ, TOKEN_LT, TOKEN_GT, TOKEN_LTE, TOKEN_GTE,
		TOKEN_AND, TOKEN_OR,
		TOKEN_BITAND, TOKEN_BITOR, TOKEN_BITXOR, TOKEN_LSHIFT, TOKEN_RSHIFT,
		TOKEN_POWER:
		return p.parseInfixExpression
	case TOKEN_RANGE:
		return p.parseRangeExpression
	case TOKEN_LPAREN:
		return p.parseCallExpression
	case TOKEN_LBRACKET:
		return p.parseIndexExpression
	case TOKEN_DOT:
		return p.parseDotExpression
	case TOKEN_OPTIONAL_DOT:
		return p.parseOptionalDotExpression
	case TOKEN_ASSIGN:
		return p.parseAssignExpression
	}
	return nil
}

func (p *Parser) parseAssignExpression(left Expression) Expression {
	switch left.(type) {
	case *Identifier, *DotExpression, *IndexExpression:
		// valid assignment target
	default:
		p.errors = append(p.errors, fmt.Sprintf("Line %d: invalid assignment target", p.curToken.Line))
		return nil
	}

	exp := &AssignExpression{Target: left}

	p.nextToken()
	exp.Value = p.parseExpression(LOWEST)

	return exp
}

func (p *Parser) parseCompoundAssignExpression(left Expression) Expression {
	switch left.(type) {
	case *Identifier, *DotExpression, *IndexExpression:
		// valid compound assignment target
	default:
		p.errors = append(p.errors, fmt.Sprintf("Line %d: invalid compound assignment target", p.curToken.Line))
		return nil
	}

	var op string
	switch p.curToken.Type {
	case TOKEN_PLUS_ASSIGN:
		op = "+"
	case TOKEN_MINUS_ASSIGN:
		op = "-"
	case TOKEN_MULTIPLY_ASSIGN:
		op = "*"
	case TOKEN_DIVIDE_ASSIGN:
		op = "/"
	case TOKEN_MODULO_ASSIGN:
		op = "%"
	case TOKEN_NULLISH_ASSIGN:
		op = "??"
	case TOKEN_BITAND_ASSIGN:
		op = "&"
	case TOKEN_BITOR_ASSIGN:
		op = "|"
	case TOKEN_BITXOR_ASSIGN:
		op = "^"
	case TOKEN_LSHIFT_ASSIGN:
		op = "<<"
	case TOKEN_RSHIFT_ASSIGN:
		op = ">>"
	case TOKEN_POWER_ASSIGN:
		op = "**"
	case TOKEN_AND_ASSIGN:
		op = "&&"
	case TOKEN_OR_ASSIGN:
		op = "||"
	}

	p.nextToken()
	value := p.parseExpression(LOWEST)

	return &CompoundAssignExpression{Target: left, Operator: op, Value: value}
}

func (p *Parser) noPrefixParseFnError(t TokenType) {
	line, col := normalizePos(p.curToken.Line, p.curToken.Column, p.peekToken)
	msg := fmt.Sprintf(
		"Line %d:%d -> unexpected token %s at start of expression.",
		line,
		col,
		tokenDebug(p.curToken),
	)
	msg += " Hint: check for missing value/expression before this token."
	if ctx := lineContext(p.l.input, line, col); ctx != "" {
		msg += "\n" + ctx
	}
	p.errors = append(p.errors, msg)
}

func (p *Parser) parseIdentifier() Expression {
	return p.internIdentifier(p.curToken.Literal)
}

func (p *Parser) parseIntegerLiteral() Expression {
	lit := &IntegerLiteral{}

	raw := strings.ReplaceAll(p.curToken.Literal, "_", "")
	value, err := strconv.ParseInt(raw, 0, 64)
	if err != nil {
		msg := fmt.Sprintf("Line %d: could not parse %q as integer", p.curToken.Line, p.curToken.Literal)
		p.errors = append(p.errors, msg)
		return nil
	}

	lit.Value = value
	return lit
}

func (p *Parser) parseFloatLiteral() Expression {
	lit := &FloatLiteral{}

	raw := strings.ReplaceAll(p.curToken.Literal, "_", "")
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		msg := fmt.Sprintf("Line %d: could not parse %q as float", p.curToken.Line, p.curToken.Literal)
		p.errors = append(p.errors, msg)
		return nil
	}

	lit.Value = value
	return lit
}

func (p *Parser) parseStringLiteral() Expression {
	raw := p.curToken.Literal
	if !strings.Contains(raw, "${") {
		return &StringLiteral{Value: raw}
	}
	// Template literal with interpolation
	tl := &TemplateLiteral{}
	i := 0
	for i < len(raw) {
		idx := strings.Index(raw[i:], "${")
		if idx == -1 {
			tl.Parts = append(tl.Parts, &StringLiteral{Value: raw[i:]})
			break
		}
		if idx > 0 {
			tl.Parts = append(tl.Parts, &StringLiteral{Value: raw[i : i+idx]})
		}
		// Find matching }
		start := i + idx + 2
		depth := 1
		j := start
		for j < len(raw) && depth > 0 {
			if raw[j] == '{' {
				depth++
			} else if raw[j] == '}' {
				depth--
			}
			if depth > 0 {
				j++
			}
		}
		exprStr := raw[start:j]
		exprLexer := NewLexer(exprStr)
		exprParser := NewParser(exprLexer)
		expr := exprParser.parseExpression(LOWEST)
		if expr != nil {
			tl.Parts = append(tl.Parts, expr)
		}
		i = j + 1
	}
	if len(tl.Parts) == 1 {
		if sl, ok := tl.Parts[0].(*StringLiteral); ok {
			return sl
		}
	}
	return tl
}

func (p *Parser) parseBooleanLiteral() Expression {
	return &BooleanLiteral{Value: p.curTokenIs(TOKEN_TRUE)}
}

func (p *Parser) parseNullLiteral() Expression {
	return &NullLiteral{}
}

func (p *Parser) parseArrayLiteral() Expression {
	array := &ArrayLiteral{}
	array.Elements = p.parseExpressionList(TOKEN_RBRACKET)
	return array
}

func (p *Parser) parseExpressionList(end TokenType) []Expression {
	list := make([]Expression, 0, 4)

	if p.peekTokenIs(end) {
		p.nextToken()
		return list
	}

	p.nextToken()
	if p.curTokenIs(TOKEN_SPREAD) {
		p.nextToken()
		list = append(list, &SpreadExpression{Value: p.parseExpression(LOWEST)})
	} else {
		list = append(list, p.parseExpression(LOWEST))
	}

	for p.peekTokenIs(TOKEN_COMMA) {
		p.nextToken()
		p.nextToken()
		if p.curTokenIs(TOKEN_SPREAD) {
			p.nextToken()
			list = append(list, &SpreadExpression{Value: p.parseExpression(LOWEST)})
		} else {
			list = append(list, p.parseExpression(LOWEST))
		}
	}

	if !p.expectPeek(end) {
		return nil
	}

	return list
}

func (p *Parser) parseHashLiteral() Expression {
	hash := &HashLiteral{}

	for !p.peekTokenIs(TOKEN_RBRACE) {
		p.nextToken()

		// Handle spread: ...expr
		if p.curTokenIs(TOKEN_SPREAD) {
			p.nextToken()
			spreadExpr := p.parseExpression(LOWEST)
			hash.Entries = append(hash.Entries, HashEntry{Value: spreadExpr, IsSpread: true})
		} else if p.curTokenIs(TOKEN_LBRACKET) {
			// Computed property key: { [expr]: value }
			p.nextToken() // past [
			keyExpr := p.parseExpression(LOWEST)
			if !p.expectPeek(TOKEN_RBRACKET) {
				return nil
			}
			if !p.expectPeek(TOKEN_COLON) {
				return nil
			}
			p.nextToken()
			value := p.parseExpression(LOWEST)
			hash.Entries = append(hash.Entries, HashEntry{Key: keyExpr, Value: value, IsComputed: true})
		} else {
			key := p.parseExpression(LOWEST)

			// Property shorthand: { x } is sugar for { "x": x }
			if ident, ok := key.(*Identifier); ok && !p.peekTokenIs(TOKEN_COLON) {
				strKey := &StringLiteral{Value: ident.Name}
				hash.Entries = append(hash.Entries, HashEntry{Key: strKey, Value: ident})
			} else {
				if !p.expectPeek(TOKEN_COLON) {
					return nil
				}

				p.nextToken()
				value := p.parseExpression(LOWEST)

				hash.Entries = append(hash.Entries, HashEntry{Key: key, Value: value})
			}
		}

		if !p.peekTokenIs(TOKEN_RBRACE) && !p.expectPeek(TOKEN_COMMA) {
			return nil
		}
	}

	if !p.expectPeek(TOKEN_RBRACE) {
		return nil
	}

	return hash
}

func (p *Parser) parsePrefixExpression() Expression {
	expression := &PrefixExpression{
		Operator: p.curToken.Literal,
	}

	p.nextToken()

	expression.Right = p.parseExpression(PREFIX)

	return expression
}

func (p *Parser) parseInfixExpression(left Expression) Expression {
	expression := &InfixExpression{
		Operator: p.curToken.Literal,
		Left:     left,
	}

	precedence := p.curPrecedence()
	p.nextToken()
	expression.Right = p.parseExpression(precedence)

	return expression
}

func (p *Parser) parseRangeExpression(left Expression) Expression {
	p.nextToken() // consume ..
	right := p.parseExpression(RANGE_PREC)
	return &RangeExpression{Left: left, Right: right}
}

func (p *Parser) parsePipelineExpression(left Expression) Expression {
	p.nextToken() // move to right side expression
	right := p.parseExpression(ASSIGN)
	if right == nil {
		return nil
	}
	if call, ok := right.(*CallExpression); ok {
		args := make([]Expression, 0, len(call.Arguments)+1)
		args = append(args, left)
		args = append(args, call.Arguments...)
		call.Arguments = args
		return call
	}
	return &CallExpression{Function: right, Arguments: []Expression{left}, Line: p.curToken.Line, Column: p.curToken.Column}
}

func (p *Parser) parseNewExpression() Expression {
	p.nextToken()
	constructor := p.parseExpression(ASSIGN)
	if constructor == nil {
		return nil
	}
	if call, ok := constructor.(*CallExpression); ok {
		return call
	}
	return &CallExpression{Function: constructor, Arguments: []Expression{}, Line: p.curToken.Line, Column: p.curToken.Column}
}

func (p *Parser) isArrowAfterParens() bool {
	saved := p.saveState()
	defer p.restoreState(saved)
	// curToken is TOKEN_LPAREN
	depth := 1
	p.nextToken() // advance past (
	for depth > 0 {
		switch p.curToken.Type {
		case TOKEN_LPAREN:
			depth++
		case TOKEN_RPAREN:
			depth--
			if depth == 0 {
				return p.peekTokenIs(TOKEN_ARROW)
			}
		case TOKEN_EOF:
			return false
		}
		p.nextToken()
	}
	return false
}

func (p *Parser) parseArrowFunction() Expression {
	lit := &FunctionLiteral{IsArrow: true}
	// curToken is TOKEN_LPAREN
	lit.Parameters, lit.ParamTypes, lit.Defaults, lit.HasRest = p.parseFunctionParameters()
	if !p.expectPeek(TOKEN_ARROW) {
		return nil
	}
	p.nextToken() // advance past =>
	if p.curTokenIs(TOKEN_LBRACE) {
		lit.Body = p.parseBlockStatement()
	} else {
		expr := p.parseExpression(LOWEST)
		lit.Body = &BlockStatement{
			Statements: []Statement{
				&ReturnStatement{ReturnValue: expr},
			},
		}
	}
	return lit
}

func (p *Parser) parseGroupedExpression() Expression {
	if p.isArrowAfterParens() {
		return p.parseArrowFunction()
	}
	p.nextToken()

	exp := p.parseExpression(LOWEST)

	if !p.expectPeek(TOKEN_RPAREN) {
		return nil
	}

	return exp
}

func (p *Parser) parseIfExpression() Expression {
	expression := &IfExpression{}

	if !p.expectPeek(TOKEN_LPAREN) {
		return nil
	}

	p.nextToken()
	expression.Condition = p.parseExpression(LOWEST)

	if !p.expectPeek(TOKEN_RPAREN) {
		return nil
	}

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}

	expression.Consequence = p.parseBlockStatement()

	if p.peekTokenIs(TOKEN_ELSE) {
		p.nextToken()

		if !p.expectPeek(TOKEN_LBRACE) {
			return nil
		}

		expression.Alternative = p.parseBlockStatement()
	}

	return expression
}

func (p *Parser) parseFunctionLiteral() Expression {
	lit := &FunctionLiteral{}

	if !p.expectPeek(TOKEN_LPAREN) {
		return nil
	}

	lit.Parameters, lit.ParamTypes, lit.Defaults, lit.HasRest = p.parseFunctionParameters()
	if p.peekTokenIs(TOKEN_COLON) {
		p.nextToken()
		lit.ReturnType = p.parseTypeName()
		if lit.ReturnType == "" {
			return nil
		}
	}

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}

	lit.Body = p.parseBlockStatement()

	return lit
}

func (p *Parser) parseFunctionDeclaration() Statement {
	// curToken is TOKEN_FUNCTION, peekToken is TOKEN_IDENT
	p.nextToken() // advance to the name
	name := p.internIdentifier(p.curToken.Literal)

	if !p.expectPeek(TOKEN_LPAREN) {
		return nil
	}
	params, paramTypes, defaults, hasRest := p.parseFunctionParameters()
	returnType := ""
	if p.peekTokenIs(TOKEN_COLON) {
		p.nextToken()
		returnType = p.parseTypeName()
		if returnType == "" {
			return nil
		}
	}

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	body := p.parseBlockStatement()

	fn := &FunctionLiteral{Parameters: params, ParamTypes: paramTypes, Defaults: defaults, HasRest: hasRest, ReturnType: returnType, Body: body}
	return &LetStatement{Name: name, Names: []*Identifier{name}, Value: fn}
}

func (p *Parser) parseTypeName() string {
	if !p.expectPeek(TOKEN_IDENT) {
		return ""
	}
	return strings.TrimSpace(p.curToken.Literal)
}

func (p *Parser) parseInitStatement() Statement {
	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	stmt := &InitStatement{Body: p.parseBlockStatement()}
	p.initBlocks = append(p.initBlocks, stmt)
	return stmt
}

func (p *Parser) parseClassStatement() Statement {
	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt := &ClassStatement{Name: p.internIdentifier(p.curToken.Literal)}
	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	p.nextToken()
	for !p.curTokenIs(TOKEN_RBRACE) && !p.curTokenIs(TOKEN_EOF) {
		if !p.curTokenIs(TOKEN_IDENT) {
			p.errors = append(p.errors, fmt.Sprintf("Line %d: expected class method name, got %s", p.curToken.Line, tokenDebug(p.curToken)))
			return nil
		}
		m := &ClassMethod{Name: p.internIdentifier(p.curToken.Literal)}
		if !p.expectPeek(TOKEN_LPAREN) {
			return nil
		}
		params, _, _, _ := p.parseFunctionParameters()
		m.Parameters = params
		if !p.expectPeek(TOKEN_LBRACE) {
			return nil
		}
		m.Body = p.parseBlockStatement()
		stmt.Methods = append(stmt.Methods, m)
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseInterfaceStatement() Statement {
	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt := &InterfaceStatement{Name: p.internIdentifier(p.curToken.Literal)}
	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	p.nextToken()
	for !p.curTokenIs(TOKEN_RBRACE) && !p.curTokenIs(TOKEN_EOF) {
		if !p.curTokenIs(TOKEN_IDENT) {
			p.errors = append(p.errors, fmt.Sprintf("Line %d: expected interface method name, got %s", p.curToken.Line, tokenDebug(p.curToken)))
			return nil
		}
		methodName := p.internIdentifier(p.curToken.Literal)
		if !p.expectPeek(TOKEN_LPAREN) {
			return nil
		}
		params, paramTypes, _, _ := p.parseFunctionParameters()
		retType := ""
		if p.peekTokenIs(TOKEN_COLON) {
			p.nextToken()
			retType = p.parseTypeName()
			if retType == "" {
				return nil
			}
		}
		if p.peekTokenIs(TOKEN_SEMICOLON) {
			p.nextToken()
		}
		stmt.Methods = append(stmt.Methods, &InterfaceMethod{Name: methodName, Parameters: params, ParamTypes: paramTypes, ReturnType: retType})
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseTestStatement() Statement {
	if !p.expectPeek(TOKEN_STRING) {
		return nil
	}
	name := p.curToken.Literal
	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	body := p.parseBlockStatement()
	return &TestStatement{Name: name, Body: body}
}

func (p *Parser) parseTypeDeclarationStatement() Statement {
	stmt := &TypeDeclarationStatement{}
	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt.Name = p.internIdentifier(p.curToken.Literal)
	if !p.expectPeek(TOKEN_ASSIGN) {
		return nil
	}
	for {
		if p.peekTokenIs(TOKEN_BITOR) {
			p.nextToken()
		}
		if !p.expectPeek(TOKEN_IDENT) {
			return nil
		}
		variant := &ADTVariantDecl{Name: p.internIdentifier(p.curToken.Literal)}
		if p.peekTokenIs(TOKEN_LPAREN) {
			p.nextToken()
			if !p.peekTokenIs(TOKEN_RPAREN) {
				for {
					if !p.expectPeek(TOKEN_IDENT) {
						return nil
					}
					variant.Fields = append(variant.Fields, p.internIdentifier(p.curToken.Literal))
					if !p.peekTokenIs(TOKEN_COMMA) {
						break
					}
					p.nextToken()
				}
			}
			if !p.expectPeek(TOKEN_RPAREN) {
				return nil
			}
		}
		stmt.Variants = append(stmt.Variants, variant)
		if !p.peekTokenIs(TOKEN_BITOR) {
			break
		}
	}
	if p.peekTokenIs(TOKEN_SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseFunctionParameters() ([]*Identifier, []string, []Expression, bool) {
	identifiers := []*Identifier{}
	paramTypes := []string{}
	defaults := []Expression{}
	hasRest := false

	if p.peekTokenIs(TOKEN_RPAREN) {
		p.nextToken()
		return identifiers, paramTypes, defaults, false
	}

	p.nextToken()

	if p.curTokenIs(TOKEN_SPREAD) {
		p.nextToken() // move to ident
		hasRest = true
	}
	ident := p.internIdentifier(p.curToken.Literal)
	identifiers = append(identifiers, ident)
	if !hasRest && p.peekTokenIs(TOKEN_COLON) {
		p.nextToken()
		typeName := p.parseTypeName()
		if typeName == "" {
			return nil, nil, nil, false
		}
		paramTypes = append(paramTypes, typeName)
	} else {
		paramTypes = append(paramTypes, "")
	}

	if !hasRest && p.peekTokenIs(TOKEN_ASSIGN) {
		p.nextToken() // consume =
		p.nextToken() // move to expression
		defaults = append(defaults, p.parseExpression(LOWEST))
	} else {
		defaults = append(defaults, nil)
	}

	for p.peekTokenIs(TOKEN_COMMA) {
		if hasRest {
			p.errors = append(p.errors, fmt.Sprintf("Line %d: rest parameter must be last", p.curToken.Line))
			return nil, nil, nil, false
		}
		p.nextToken()
		p.nextToken()

		if p.curTokenIs(TOKEN_SPREAD) {
			p.nextToken()
			hasRest = true
		}
		ident := p.internIdentifier(p.curToken.Literal)
		identifiers = append(identifiers, ident)
		if !hasRest && p.peekTokenIs(TOKEN_COLON) {
			p.nextToken()
			typeName := p.parseTypeName()
			if typeName == "" {
				return nil, nil, nil, false
			}
			paramTypes = append(paramTypes, typeName)
		} else {
			paramTypes = append(paramTypes, "")
		}

		if !hasRest && p.peekTokenIs(TOKEN_ASSIGN) {
			p.nextToken()
			p.nextToken()
			defaults = append(defaults, p.parseExpression(LOWEST))
		} else {
			defaults = append(defaults, nil)
		}
	}

	if !p.expectPeek(TOKEN_RPAREN) {
		return nil, nil, nil, false
	}

	return identifiers, paramTypes, defaults, hasRest
}

func (p *Parser) parseDotExpression(left Expression) Expression {
	exp := &DotExpression{Left: left}

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}

	exp.Right = p.internIdentifier(p.curToken.Literal)

	return exp
}

func (p *Parser) parseCallExpression(function Expression) Expression {
	exp := &CallExpression{
		Function: function,
		Line:     p.curToken.Line,
		Column:   p.curToken.Column,
	}
	exp.Arguments = p.parseExpressionList(TOKEN_RPAREN)
	return exp
}

func (p *Parser) parseIndexExpression(left Expression) Expression {
	exp := &IndexExpression{Left: left}

	p.nextToken()
	exp.Index = p.parseExpression(LOWEST)

	if !p.expectPeek(TOKEN_RBRACKET) {
		return nil
	}

	return exp
}

// Evaluator
type ObjectType int

const (
	INTEGER_OBJ ObjectType = iota
	FLOAT_OBJ
	BOOLEAN_OBJ
	STRING_OBJ
	NULL_OBJ
	ERROR_OBJ
	RETURN_VALUE_OBJ
	BREAK_OBJ
	CONTINUE_OBJ
	FUNCTION_OBJ
	BUILTIN_OBJ
	ARRAY_OBJ
	HASH_OBJ
	DB_OBJ
	DB_TX_OBJ
	FUTURE_OBJ
	INTERFACE_OBJ
	ADT_TYPE_OBJ
	ADT_VALUE_OBJ
	LAZY_OBJ
	OWNED_OBJ
	SECRET_OBJ
)

func (ot ObjectType) String() string {
	switch ot {
	case INTEGER_OBJ:
		return "INTEGER"
	case FLOAT_OBJ:
		return "FLOAT"
	case BOOLEAN_OBJ:
		return "BOOLEAN"
	case STRING_OBJ:
		return "STRING"
	case NULL_OBJ:
		return "NULL"
	case ERROR_OBJ:
		return "ERROR"
	case RETURN_VALUE_OBJ:
		return "RETURN_VALUE"
	case BREAK_OBJ:
		return "BREAK"
	case CONTINUE_OBJ:
		return "CONTINUE"
	case FUNCTION_OBJ:
		return "FUNCTION"
	case BUILTIN_OBJ:
		return "BUILTIN"
	case ARRAY_OBJ:
		return "ARRAY"
	case HASH_OBJ:
		return "HASH"
	case DB_OBJ:
		return "DB"
	case DB_TX_OBJ:
		return "DB_TX"
	case FUTURE_OBJ:
		return "FUTURE"
	case INTERFACE_OBJ:
		return "INTERFACE"
	case ADT_TYPE_OBJ:
		return "ADT_TYPE"
	case ADT_VALUE_OBJ:
		return "ADT_VALUE"
	case LAZY_OBJ:
		return "LAZY"
	case OWNED_OBJ:
		return "OWNED"
	case SECRET_OBJ:
		return "SECRET"
	case SERVER_OBJ:
		return "SERVER"
	case REQUEST_OBJ:
		return "REQUEST"
	case RESPONSE_OBJ:
		return "RESPONSE"
	case SSE_WRITER_OBJ:
		return "SSE_WRITER"
	case QUERY_BUILDER_OBJ:
		return "QUERY_BUILDER"
	case LAZY_DB_QUERY_OBJ:
		return "LAZY_DB_QUERY"
	case SIGNAL_OBJ:
		return "SIGNAL"
	case COMPUTED_OBJ:
		return "COMPUTED"
	case EFFECT_OBJ:
		return "EFFECT"
	default:
		return "UNKNOWN"
	}
}

type Integer struct {
	Value int64
}

func (i *Integer) Type() ObjectType { return INTEGER_OBJ }
func (i *Integer) Inspect() string  { return fmt.Sprintf("%d", i.Value) }

type Float struct {
	Value float64
}

func (f *Float) Type() ObjectType { return FLOAT_OBJ }
func (f *Float) Inspect() string  { return fmt.Sprintf("%g", f.Value) }

type Boolean struct {
	Value bool
}

func (b *Boolean) Type() ObjectType { return BOOLEAN_OBJ }
func (b *Boolean) Inspect() string  { return fmt.Sprintf("%t", b.Value) }

type String struct {
	Value string
}

func (s *String) Type() ObjectType { return STRING_OBJ }
func (s *String) Inspect() string  { return s.Value }

type Secret struct {
	Value string
}

func (s *Secret) Type() ObjectType { return SECRET_OBJ }
func (s *Secret) Inspect() string {
	if s == nil {
		return "***"
	}
	return "***"
}

type Null struct{}

func (n *Null) Type() ObjectType { return NULL_OBJ }
func (n *Null) Inspect() string  { return "null" }

type Error struct {
	Message string
	Code    string
	Path    string
	Line    int
	Column  int
	Stack   []CallFrame
}

func (e *Error) Type() ObjectType { return ERROR_OBJ }
func (e *Error) Inspect() string {
	if e == nil {
		return "ERROR"
	}
	var out strings.Builder
	out.WriteString("ERROR: ")
	if strings.TrimSpace(e.Code) != "" {
		out.WriteString("[")
		out.WriteString(e.Code)
		out.WriteString("] ")
	}
	out.WriteString(e.Message)
	if trace := formatCallStack(e.Stack); trace != "" {
		out.WriteByte('\n')
		out.WriteString(trace)
	}
	return out.String()
}

func newError(format string, args ...interface{}) Object {
	msg := fmt.Sprintf(format, args...)
	code := "E_RUNTIME"
	lower := strings.ToLower(strings.TrimSpace(msg))
	switch {
	case strings.Contains(lower, "identifier not found"):
		code = "E_NAME"
	case strings.Contains(lower, "type mismatch"):
		code = "E_TYPE"
	case strings.Contains(lower, "parse"):
		code = "E_PARSE"
	case strings.Contains(lower, "permission") || strings.Contains(lower, "denied"):
		code = "E_PERMISSION"
	}
	return &Error{Message: msg, Code: code}
}

func (e *Error) withFrame(frame CallFrame) *Error {
	if e == nil {
		return nil
	}
	if frame.Function == "" && frame.Path == "" && frame.Line == 0 && frame.Column == 0 {
		return e
	}
	cloned := *e
	cloned.Stack = append(append([]CallFrame(nil), e.Stack...), frame)
	if cloned.Path == "" {
		cloned.Path = frame.Path
	}
	if cloned.Line == 0 {
		cloned.Line = frame.Line
	}
	if cloned.Column == 0 {
		cloned.Column = frame.Column
	}
	return &cloned
}

func sameCallFrame(a, b CallFrame) bool {
	return a.Function == b.Function && a.Path == b.Path && a.Line == b.Line && a.Column == b.Column
}

func formatCallFrame(frame CallFrame) string {
	location := frame.Path
	if location == "" {
		location = "<memory>"
	}
	if frame.Line > 0 {
		location = fmt.Sprintf("%s:%d", location, frame.Line)
		if frame.Column > 0 {
			location = fmt.Sprintf("%s:%d", location, frame.Column)
		}
	}
	if frame.Function == "" {
		return location
	}
	return fmt.Sprintf("%s (%s)", frame.Function, location)
}

func formatCallStack(stack []CallFrame) string {
	if len(stack) == 0 {
		return ""
	}
	lines := make([]string, 0, len(stack)+1)
	lines = append(lines, "Stack trace:")
	for i := 0; i < len(stack); i++ {
		lines = append(lines, "  at "+formatCallFrame(stack[i]))
	}
	return strings.Join(lines, "\n")
}

func objectErrorString(obj Object) string {
	if obj == nil {
		return ""
	}
	if h, ok := obj.(*Hash); ok {
		if pair, ok := h.Pairs[(&String{Value: "message"}).HashKey()]; ok {
			return objectToDisplayString(pair.Value)
		}
	}
	if errObj, ok := obj.(*Error); ok {
		return errObj.Message
	}
	if owned, ok := obj.(*OwnedValue); ok {
		return objectErrorString(owned.inner)
	}
	if strObj, ok := obj.(*String); ok && strings.HasPrefix(strObj.Value, "ERROR:") {
		return strings.TrimPrefix(strObj.Value, "ERROR: ")
	}
	return obj.Inspect()
}

type ReturnValue struct {
	Value Object
}

func (rv *ReturnValue) Type() ObjectType { return RETURN_VALUE_OBJ }
func (rv *ReturnValue) Inspect() string  { return rv.Value.Inspect() }

type Break struct{}

func (b *Break) Type() ObjectType { return BREAK_OBJ }
func (b *Break) Inspect() string  { return "break" }

type Continue struct{}

func (c *Continue) Type() ObjectType { return CONTINUE_OBJ }
func (c *Continue) Inspect() string  { return "continue" }

type Function struct {
	Parameters []*Identifier
	ParamTypes []string
	Defaults   []Expression
	ReturnType string
	HasRest    bool
	Body       *BlockStatement
	Env        *Environment
	IsAsync    bool
}

func (f *Function) Type() ObjectType { return FUNCTION_OBJ }
func (f *Function) Inspect() string {
	var out strings.Builder
	out.WriteString("function(")
	for i, p := range f.Parameters {
		if i > 0 {
			out.WriteString(", ")
		}
		if f.HasRest && i == len(f.Parameters)-1 {
			out.WriteString("...")
		}
		out.WriteString(p.String())
		if i < len(f.ParamTypes) && f.ParamTypes[i] != "" {
			out.WriteString(": ")
			out.WriteString(f.ParamTypes[i])
		}
		if i < len(f.Defaults) && f.Defaults[i] != nil {
			out.WriteString(" = ")
			out.WriteString(f.Defaults[i].String())
		}
	}
	out.WriteString(") {\n")
	if f.ReturnType != "" {
		out.WriteString("// -> ")
		out.WriteString(f.ReturnType)
		out.WriteString("\n")
	}
	out.WriteString(f.Body.String())
	out.WriteString("\n}")
	return out.String()
}

type Array struct {
	Elements []Object
}

func (ao *Array) Type() ObjectType { return ARRAY_OBJ }
func (ao *Array) Inspect() string {
	var out strings.Builder
	out.WriteString("[")
	for i, e := range ao.Elements {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(e.Inspect())
	}
	out.WriteString("]")
	return out.String()
}

type HashKey struct {
	Type  ObjectType
	Value uint64
}

type Hashable interface {
	HashKey() HashKey
}

func (b *Boolean) HashKey() HashKey {
	var value uint64
	if b.Value {
		value = 1
	} else {
		value = 0
	}
	return HashKey{Type: b.Type(), Value: value}
}

func (i *Integer) HashKey() HashKey {
	return HashKey{Type: i.Type(), Value: uint64(i.Value)}
}

func (s *String) HashKey() HashKey {
	h := uint64(0)
	for _, ch := range s.Value {
		h = 31*h + uint64(ch)
	}
	return HashKey{Type: s.Type(), Value: h}
}

type HashPair struct {
	Key   Object
	Value Object
}

type Hash struct {
	Pairs map[HashKey]HashPair
}

func (h *Hash) Type() ObjectType { return HASH_OBJ }
func (h *Hash) Inspect() string {
	var out strings.Builder
	pairs := make([]string, 0, len(h.Pairs))
	for _, pair := range h.Pairs {
		pairs = append(pairs, fmt.Sprintf("%s: %s", pair.Key.Inspect(), pair.Value.Inspect()))
	}
	out.WriteString("{")
	out.WriteString(strings.Join(pairs, ", "))
	out.WriteString("}")
	return out.String()
}

// DB represents a database connection
type DB struct {
	*squealx.DB
}

func (db *DB) Type() ObjectType { return DB_OBJ }
func (db *DB) Inspect() string  { return "<db connection>" }

// DBTx represents a database transaction.
type DBTx struct {
	*squealx.Tx
}

func (tx *DBTx) Type() ObjectType { return DB_TX_OBJ }
func (tx *DBTx) Inspect() string  { return "<db transaction>" }

type CallFrame struct {
	Function string
	Path     string
	Line     int
	Column   int
}

// Future represents the result of an async operation.
type Future struct {
	ch     chan Object
	result Object
	done   bool
	mu     sync.Mutex
}

func (f *Future) Type() ObjectType { return FUTURE_OBJ }
func (f *Future) Inspect() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done {
		return fmt.Sprintf("<future: resolved=%s>", f.result.Inspect())
	}
	return "<future: pending>"
}

// Resolve blocks until the future's result is available and returns it.
func (f *Future) Resolve() Object {
	f.mu.Lock()
	if f.done {
		f.mu.Unlock()
		return f.result
	}
	f.mu.Unlock()
	result := <-f.ch
	f.mu.Lock()
	f.done = true
	f.result = result
	f.mu.Unlock()
	return result
}

// Environment
type Environment struct {
	mu             sync.RWMutex
	store          map[string]Object
	outer          *Environment
	moduleContext  *ModuleContext
	moduleDir      string
	sourcePath     string
	moduleCache    map[string]ModuleCacheEntry
	moduleLoading  map[string]bool
	runtimeLimits  *RuntimeLimits
	securityPolicy *SecurityPolicy
	output         io.Writer
	callStack      []CallFrame
	ownerID        string
}

type TestStats struct {
	mu     sync.Mutex
	total  int64
	passed int64
	failed int64
}

func (s *TestStats) Reset() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total = 0
	s.passed = 0
	s.failed = 0
}

func (s *TestStats) Pass() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total++
	s.passed++
}

func (s *TestStats) Fail() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total++
	s.failed++
}

func (s *TestStats) Snapshot() (total, passed, failed int64) {
	if s == nil {
		return 0, 0, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.total, s.passed, s.failed
}

type ModuleContext struct {
	Exports map[string]Object
}

type ModuleCacheEntry struct {
	Exports map[string]Object
	ModTime int64
}

type RuntimeLimits struct {
	MaxDepth       int
	CurrentDepth   int
	MaxSteps       int64
	Steps          int64
	Deadline       time.Time
	Ctx            context.Context
	MaxHeapBytes   uint64
	heapCheckEvery int64
}

func NewEnvironment() *Environment {
	return &Environment{
		store:   make(map[string]Object),
		ownerID: fmt.Sprintf("env-%d", time.Now().UnixNano()),
	}
}

func NewGlobalEnvironment(args []string) *Environment {
	env := NewEnvironment()
	argsArray := &Array{Elements: []Object{}}
	for _, arg := range args {
		argsArray.Elements = append(argsArray.Elements, &String{Value: arg})
	}
	env.Set("ARGS", argsArray)
	env.runtimeLimits = loadRuntimeLimitsFromEnv()
	env.securityPolicy = loadSecurityPolicyFromEnv()
	env.sourcePath = "<memory>"
	return env
}

func NewEnclosedEnvironment(outer *Environment) *Environment {
	return &Environment{
		store:          make(map[string]Object),
		outer:          outer,
		moduleContext:  outer.moduleContext,
		moduleDir:      outer.moduleDir,
		sourcePath:     outer.sourcePath,
		moduleCache:    outer.moduleCache,
		moduleLoading:  outer.moduleLoading,
		runtimeLimits:  outer.runtimeLimits,
		securityPolicy: outer.securityPolicy,
		output:         outer.output,
		callStack:      append([]CallFrame(nil), outer.callStack...),
		ownerID:        fmt.Sprintf("env-%d", time.Now().UnixNano()),
	}
}

func (e *Environment) moduleCacheMap() map[string]ModuleCacheEntry {
	if e.moduleCache == nil {
		e.moduleCache = make(map[string]ModuleCacheEntry)
	}
	return e.moduleCache
}

func (e *Environment) moduleLoadingMap() map[string]bool {
	if e.moduleLoading == nil {
		e.moduleLoading = make(map[string]bool)
	}
	return e.moduleLoading
}

func (e *Environment) Get(name string) (Object, bool) {
	e.mu.RLock()
	obj, ok := e.store[name]
	e.mu.RUnlock()
	if !ok && e.outer != nil {
		obj, ok = e.outer.Get(name)
	}
	return obj, ok
}

func (e *Environment) Set(name string, val Object) Object {
	e.mu.Lock()
	e.store[name] = val
	e.mu.Unlock()
	return val
}

func (e *Environment) Assign(name string, val Object) (Object, bool) {
	e.mu.Lock()
	_, ok := e.store[name]
	if ok {
		e.store[name] = val
		e.mu.Unlock()
		return val, true
	}
	e.mu.Unlock()
	if e.outer != nil {
		return e.outer.Assign(name, val)
	}
	return nil, false
}

func parsePositiveIntEnv(name string) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func parsePositiveInt64Env(name string) int64 {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func loadRuntimeLimitsFromEnv() *RuntimeLimits {
	maxDepth := parsePositiveIntEnv("SPL_MAX_RECURSION")
	maxSteps := parsePositiveInt64Env("SPL_MAX_STEPS")
	timeoutMs := parsePositiveInt64Env("SPL_EVAL_TIMEOUT_MS")
	maxHeapMB := parsePositiveInt64Env("SPL_MAX_HEAP_MB")

	if maxDepth == 0 && maxSteps == 0 && timeoutMs == 0 && maxHeapMB == 0 {
		return nil
	}

	rl := &RuntimeLimits{
		MaxDepth:       maxDepth,
		MaxSteps:       maxSteps,
		MaxHeapBytes:   uint64(maxHeapMB) * 1024 * 1024,
		heapCheckEvery: 128,
	}
	if timeoutMs > 0 {
		rl.Deadline = time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	}
	return rl
}

func (rl *RuntimeLimits) enter() Object {
	rl.Steps++
	if rl.MaxSteps > 0 && rl.Steps > rl.MaxSteps {
		return newError("execution step limit exceeded (%d)", rl.MaxSteps)
	}

	if !rl.Deadline.IsZero() && time.Now().After(rl.Deadline) {
		return newError("execution timeout exceeded")
	}

	if rl.Ctx != nil {
		select {
		case <-rl.Ctx.Done():
			if err := rl.Ctx.Err(); err != nil {
				return newError("execution cancelled: %s", err.Error())
			}
			return newError("execution cancelled")
		default:
		}
	}

	if rl.MaxHeapBytes > 0 && rl.Steps%rl.heapCheckEvery == 0 {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		if ms.Alloc > rl.MaxHeapBytes {
			return newError("heap usage exceeded (%d MB)", rl.MaxHeapBytes/(1024*1024))
		}
	}

	return nil
}

func importSearchPaths() []string {
	raw := strings.TrimSpace(os.Getenv("SPL_MODULE_PATH"))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, string(os.PathListSeparator))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func resolveImportPath(importPath string, env *Environment) (string, error) {
	if filepath.IsAbs(importPath) {
		return sanitizePath(importPath)
	}
	if resolved, ok, err := resolveManifestImport(importPath, env); ok || err != nil {
		return resolved, err
	}

	candidates := make([]string, 0, 4)
	if env != nil && env.moduleDir != "" {
		candidates = append(candidates, filepath.Join(env.moduleDir, importPath))
	}
	candidates = append(candidates, importPath)
	for _, base := range importSearchPaths() {
		candidates = append(candidates, filepath.Join(base, importPath))
	}

	var lastErr error
	for _, c := range candidates {
		resolved, err := sanitizePath(c)
		if err != nil {
			lastErr = err
			continue
		}
		if _, err := os.Stat(resolved); err == nil {
			return resolved, nil
		}
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("module not found: %s", importPath)
}

var (
	TRUE  = &Boolean{Value: true}
	FALSE = &Boolean{Value: false}
	NULL  = &Null{}
	BREAK = &Break{}
	CONT  = &Continue{}
)

const (
	defaultIntCacheMin = int64(-256)
	defaultIntCacheMax = int64(1_000_000)
	maxIntCacheLimit   = int64(2_000_000)
)

var (
	intCacheMin  = defaultIntCacheMin
	intCacheMax  = defaultIntCacheMax
	integerCache []*Integer
)

func init() {
	if cfgMax := parsePositiveInt64Env("SPL_INT_CACHE_MAX"); cfgMax > 0 {
		if cfgMax > maxIntCacheLimit {
			cfgMax = maxIntCacheLimit
		}
		if cfgMax >= defaultIntCacheMin {
			intCacheMax = cfgMax
		}
	}

	cacheLen := int(intCacheMax-intCacheMin) + 1
	integerCache = make([]*Integer, cacheLen)
	for i := intCacheMin; i <= intCacheMax; i++ {
		integerCache[int(i-intCacheMin)] = &Integer{Value: i}
	}
}

func integerObj(v int64) *Integer {
	if v >= intCacheMin && v <= intCacheMax {
		return integerCache[int(v-intCacheMin)]
	}
	return &Integer{Value: v}
}

func Eval(node Node, env *Environment) Object {
	if env != nil && env.runtimeLimits != nil {
		if errObj := env.runtimeLimits.enter(); errObj != nil {
			return errObj
		}
	}

	switch node := node.(type) {
	case *Program:
		return evalProgram(node, env)

	case *ExpressionStatement:
		return Eval(node.Expression, env)

	case *IntegerLiteral:
		return integerObj(node.Value)

	case *FloatLiteral:
		return &Float{Value: node.Value}

	case *StringLiteral:
		return &String{Value: node.Value}

	case *BooleanLiteral:
		return nativeBoolToBooleanObject(node.Value)

	case *NullLiteral:
		return NULL

	case *ArrayLiteral:
		elements := evalExpressions(node.Elements, env)
		if len(elements) == 1 && isError(elements[0]) {
			return elements[0]
		}
		return &Array{Elements: elements}

	case *HashLiteral:
		return evalHashLiteral(node, env)

	case *IndexExpression:
		left := Eval(node.Left, env)
		if isError(left) {
			return left
		}
		index := Eval(node.Index, env)
		if isError(index) {
			return index
		}
		return evalIndexExpression(left, index)

	case *DotExpression:
		left := Eval(node.Left, env)
		if isError(left) {
			return left
		}
		return evalDotExpression(left, node.Right.Name)

	case *OptionalDotExpression:
		left := Eval(node.Left, env)
		if isError(left) {
			return left
		}
		if left == NULL || left == nil {
			return NULL
		}
		result := evalDotExpression(left, node.Right.Name)
		if isError(result) {
			return NULL
		}
		return result

	case *PrefixExpression:

		right := Eval(node.Right, env)
		if isError(right) {
			return right
		}
		return evalPrefixExpression(node.Operator, right)

	case *AssignExpression:
		val := Eval(node.Value, env)
		if isError(val) {
			return val
		}
		return evalTargetAssign(node.Target, val, env)

	case *CompoundAssignExpression:
		current := evalTargetGet(node.Target, env)
		if isError(current) {
			return current
		}
		// Handle ??= specially: only assign if current is null
		if node.Operator == "??" {
			if current != NULL && current != nil {
				return current
			}
			right := Eval(node.Value, env)
			if isError(right) {
				return right
			}
			return evalTargetAssign(node.Target, right, env)
		}
		// Handle &&= : only assign if current is truthy
		if node.Operator == "&&" {
			if !isTruthy(current) {
				return current
			}
			right := Eval(node.Value, env)
			if isError(right) {
				return right
			}
			return evalTargetAssign(node.Target, right, env)
		}
		// Handle ||= : only assign if current is falsy
		if node.Operator == "||" {
			if isTruthy(current) {
				return current
			}
			right := Eval(node.Value, env)
			if isError(right) {
				return right
			}
			return evalTargetAssign(node.Target, right, env)
		}
		right := Eval(node.Value, env)
		if isError(right) {
			return right
		}
		result := evalInfixExpression(node.Operator, current, right)
		if isError(result) {
			return result
		}
		return evalTargetAssign(node.Target, result, env)

	case *PostfixExpression:
		current := evalTargetGet(node.Target, env)
		if isError(current) {
			return current
		}
		var newVal Object
		switch node.Operator {
		case "++":
			switch v := current.(type) {
			case *Integer:
				newVal = integerObj(v.Value + 1)
			case *Float:
				newVal = &Float{Value: v.Value + 1}
			default:
				return newError("postfix ++ not supported on %s", current.Type())
			}
		case "--":
			switch v := current.(type) {
			case *Integer:
				newVal = integerObj(v.Value - 1)
			case *Float:
				newVal = &Float{Value: v.Value - 1}
			default:
				return newError("postfix -- not supported on %s", current.Type())
			}
		}
		evalTargetAssign(node.Target, newVal, env)
		return current // return old value (postfix semantics)

	case *InfixExpression:
		if node.Operator == "??" {
			left := Eval(node.Left, env)
			if isError(left) {
				return left
			}
			if left == NULL || left == nil {
				return Eval(node.Right, env)
			}
			return left
		}

		left := Eval(node.Left, env)
		if isError(left) {
			return left
		}

		if node.Operator == "&&" {
			if !isTruthy(left) {
				return FALSE
			}
		}
		if node.Operator == "||" {
			if isTruthy(left) {
				return TRUE
			}
		}

		right := Eval(node.Right, env)
		if isError(right) {
			return right
		}

		return evalInfixExpression(node.Operator, left, right)

	case *BlockStatement:
		return evalBlockStatement(node, env)

	case *IfExpression:
		return evalIfExpression(node, env)

	case *WhileStatement:
		return evalWhileStatement(node, env)

	case *DoWhileStatement:
		return evalDoWhileStatement(node, env)

	case *ForStatement:
		return evalForStatement(node, env)

	case *ForInStatement:
		return evalForInStatement(node, env)

	case *ImportStatement:
		return evalImportStatement(node, env)

	case *ExportStatement:
		return evalExportStatement(node, env)

	case *ThrowStatement:
		thrown := Eval(node.Value, env)
		if isError(thrown) {
			return thrown
		}
		if errHash, ok := thrown.(*Hash); ok {
			if pair, ok := errHash.Pairs[(&String{Value: "message"}).HashKey()]; ok {
				msg := objectToDisplayString(pair.Value)
				errObj := &Error{Message: msg}
				if codePair, ok := errHash.Pairs[(&String{Value: "code"}).HashKey()]; ok {
					if codeStr, ok := codePair.Value.(*String); ok && strings.TrimSpace(codeStr.Value) != "" {
						errObj.Message = fmt.Sprintf("[%s] %s", codeStr.Value, msg)
					}
				}
				return errObj
			}
		}
		return newError("%s", objectErrorString(thrown))

	case *TryCatchExpression:
		return evalTryCatchExpression(node, env)

	case *TernaryExpression:
		condition := Eval(node.Condition, env)
		if isError(condition) {
			return condition
		}
		if isTruthy(condition) {
			return Eval(node.Consequence, env)
		}
		return Eval(node.Alternative, env)

	case *SwitchStatement:
		return evalSwitchStatement(node, env)

	case *MatchExpression:
		return evalMatchExpression(node, env)

	case *RangeExpression:
		return evalRangeExpression(node, env)

	case *AwaitExpression:
		obj := Eval(node.Value, env)
		if isError(obj) {
			return obj
		}
		if lazy, ok := obj.(*LazyValue); ok {
			obj = lazy.Force()
		}
		if future, ok := obj.(*Future); ok {
			return future.Resolve()
		}
		return obj // already resolved, pass through

	case *LazyExpression:
		return &LazyValue{env: env, expr: node.Value}

	case *TemplateLiteral:
		return evalTemplateLiteral(node, env)

	case *ReturnStatement:
		val := Eval(node.ReturnValue, env)
		if isError(val) {
			return val
		}
		return &ReturnValue{Value: val}

	case *BreakStatement:
		return BREAK

	case *ContinueStatement:
		return CONT

	case *LetStatement:
		val := Eval(node.Value, env)
		if isError(val) {
			return val
		}

		if len(node.Names) > 1 {
			// Expecting an Array result for multi-assignment
			// Support Golang-style tuple returns

			arr, ok := val.(*Array)
			if !ok {
				// Allow single value to be assigned to first var, others null?
				// Or stricter: Error. Go is strict.
				return newError("assignment mismatch: %d variables but 1 value", len(node.Names))
			}

			if len(node.Names) != len(arr.Elements) {
				// Go is strict about count mismatch
				// We can be strict or loose. Let's be semi-strict to match expectations.
				return newError("assignment mismatch: %d variables but %d values", len(node.Names), len(arr.Elements))
			}

			for i, name := range node.Names {
				env.Set(name.Name, arr.Elements[i])
			}
		} else {
			// Backward compat or single assignment
			targetName := node.Name
			if targetName == nil && len(node.Names) > 0 {
				targetName = node.Names[0]
			}
			if targetName != nil {
				if node.TypeName != "" {
					if !valueMatchesTypeName(val, node.TypeName) {
						return newError("type mismatch for %s: expected %s, got %s", targetName.Name, node.TypeName, val.Type())
					}
				}
				env.Set(targetName.Name, maybeWrapOwned(val, env))
			}
		}

	case *DestructureLetStatement:
		val := Eval(node.Value, env)
		if isError(val) {
			return val
		}
		return evalDestructure(node.Pattern, val, env)

	case *PrintStatement:
		val := Eval(node.Expression, env)
		if isError(val) {
			return val
		}
		var out io.Writer = os.Stdout
		if env != nil && env.output != nil {
			out = env.output
		}
		fmt.Fprintln(out, val.Inspect())
		return NULL

	case *Identifier:
		return evalIdentifier(node, env)

	case *FunctionLiteral:
		return &Function{Parameters: node.Parameters, ParamTypes: node.ParamTypes, Defaults: node.Defaults, HasRest: node.HasRest, ReturnType: node.ReturnType, Env: env, Body: node.Body, IsAsync: node.IsAsync}

	case *ClassStatement:
		instanceMethods := make(map[HashKey]HashPair, len(node.Methods))
		for _, method := range node.Methods {
			fn := &Function{Parameters: method.Parameters, Defaults: make([]Expression, len(method.Parameters)), HasRest: false, Env: env, Body: method.Body}
			name := &String{Value: method.Name.Name}
			instanceMethods[name.HashKey()] = HashPair{Key: name, Value: fn}
		}
		classFn := &Builtin{FnWithEnv: func(bindEnv *Environment, args ...Object) Object {
			obj := &Hash{Pairs: make(map[HashKey]HashPair, len(instanceMethods)+1)}
			for k, pair := range instanceMethods {
				obj.Pairs[k] = pair
			}
			if ctorPair, ok := obj.Pairs[(&String{Value: "constructor"}).HashKey()]; ok {
				if ctorFn, ok := ctorPair.Value.(*Function); ok {
					bound := &Function{Parameters: ctorFn.Parameters, Defaults: ctorFn.Defaults, HasRest: ctorFn.HasRest, Env: NewEnclosedEnvironment(bindEnv), Body: ctorFn.Body}
					bound.Env.Set("this", obj)
					result := applyFunction(bound, args, bindEnv, nil)
					if isError(result) {
						return result
					}
				}
			}
			for hk, pair := range obj.Pairs {
				if fn, ok := pair.Value.(*Function); ok {
					wrapped := &Builtin{FnWithEnv: func(callEnv *Environment, callArgs ...Object) Object {
						methodEnv := NewEnclosedEnvironment(fn.Env)
						methodEnv.Set("this", obj)
						boundFn := &Function{Parameters: fn.Parameters, Defaults: fn.Defaults, HasRest: fn.HasRest, Env: methodEnv, Body: fn.Body}
						return applyFunction(boundFn, callArgs, callEnv, nil)
					}}
					obj.Pairs[hk] = HashPair{Key: pair.Key, Value: wrapped}
				}
			}
			return obj
		}}
		env.Set(node.Name.Name, classFn.bindEnv(env))
		return classFn.bindEnv(env)

	case *InterfaceStatement:
		methods := make(map[string]*InterfaceMethod, len(node.Methods))
		for _, m := range node.Methods {
			methods[m.Name.Name] = m
		}
		iface := &InterfaceLiteral{Methods: methods}
		env.Set(node.Name.Name, iface)
		return iface

	case *InitStatement:
		if node.Body == nil {
			return NULL
		}
		return Eval(node.Body, env)

	case *TestStatement:
		if node.Body == nil {
			return NULL
		}
		res := Eval(node.Body, env)
		if isError(res) {
			return res
		}
		return NULL

	case *TypeDeclarationStatement:
		if node.Name == nil || len(node.Variants) == 0 {
			return newError("invalid type declaration")
		}
		def := &ADTTypeDef{TypeName: node.Name.Name, Variants: make(map[string]int, len(node.Variants)), Order: make([]string, 0, len(node.Variants))}
		for _, variant := range node.Variants {
			if variant == nil || variant.Name == nil {
				return newError("invalid ADT variant declaration")
			}
			if _, exists := def.Variants[variant.Name.Name]; exists {
				return newError("duplicate ADT variant: %s", variant.Name.Name)
			}
			def.Variants[variant.Name.Name] = len(variant.Fields)
			def.Order = append(def.Order, variant.Name.Name)
			fieldNames := make([]string, 0, len(variant.Fields))
			for _, f := range variant.Fields {
				fieldNames = append(fieldNames, f.Name)
			}
			variantName := variant.Name.Name
			ctor := &Builtin{FnWithEnv: func(bindEnv *Environment, args ...Object) Object {
				if len(args) != len(fieldNames) {
					return newError("%s expects %d argument(s), got %d", variantName, len(fieldNames), len(args))
				}
				vals := make([]Object, len(args))
				copy(vals, args)
				allVariants := make([]string, len(def.Order))
				copy(allVariants, def.Order)
				return &ADTValue{TypeName: def.TypeName, VariantName: variantName, FieldNames: append([]string(nil), fieldNames...), Values: vals, AllVariants: allVariants}
			}}
			env.Set(variantName, ctor.bindEnv(env))
		}
		env.Set(node.Name.Name, def)
		return def

	case *CallExpression:
		if env != nil && env.runtimeLimits != nil && env.runtimeLimits.MaxDepth > 0 {
			env.runtimeLimits.CurrentDepth++
			defer func() {
				env.runtimeLimits.CurrentDepth--
			}()
			if env.runtimeLimits.CurrentDepth > env.runtimeLimits.MaxDepth {
				return newError("max recursion depth exceeded (%d)", env.runtimeLimits.MaxDepth)
			}
		}

		function := Eval(node.Function, env)
		if isError(function) {
			return function
		}

		args := evalExpressions(node.Arguments, env)
		if len(args) == 1 && isError(args[0]) {
			return args[0]
		}

		result := applyFunction(function, args, env, node)
		if runtimeErr, ok := result.(*Error); ok {
			frame := callFrameFromExpression(node.Function, env, node.Line, node.Column)
			if len(runtimeErr.Stack) == 0 || !sameCallFrame(runtimeErr.Stack[len(runtimeErr.Stack)-1], frame) {
				runtimeErr = runtimeErr.withFrame(frame)
			}
			if len(runtimeErr.Stack) == 0 && env != nil && len(env.callStack) > 0 {
				cloned := *runtimeErr
				cloned.Stack = cloneCallStack(env.callStack)
				runtimeErr = &cloned
			}
			return runtimeErr
		}
		return result

	default:
		return newError("unsupported AST node: %T", node)
	}
	return nil
}

func evalProgram(program *Program, env *Environment) Object {
	var result Object
	if env != nil {
		if inited, ok := env.Get("__module_init_done"); !ok || !isTruthy(inited) {
			for _, statement := range program.Statements {
				if initStmt, ok := statement.(*InitStatement); ok {
					initResult := Eval(initStmt.Body, env)
					if isError(initResult) {
						return initResult
					}
				}
			}
			env.Set("__module_init_done", TRUE)
		}
	}

	for _, statement := range program.Statements {
		if _, isInit := statement.(*InitStatement); isInit {
			continue
		}
		result = runProgramStatement(statement, env)

		switch result := result.(type) {
		case *ReturnValue:
			return result.Value
		case *Error:
			return result
		}
	}

	return result
}

func runProgramStatement(statement Statement, env *Environment) Object {
	switch statement.(type) {
	case *ReturnStatement, *BreakStatement, *ContinueStatement:
		return Eval(statement, env)
	}
	prog := &Program{Statements: []Statement{statement}}
	compiled, err := compileToBytecode(prog)
	if err == nil {
		return runOnVM(compiled, env)
	}
	if _, ok := err.(*errUnsupportedNode); ok {
		return Eval(statement, env)
	}
	return newError("bytecode compile failed: %s", err)
}

func evalBlockStatement(block *BlockStatement, env *Environment) Object {
	var result Object
	var setup *BlockStatement
	var teardown *BlockStatement
	for _, statement := range block.Statements {
		if exprStmt, ok := statement.(*ExpressionStatement); ok {
			if ident, ok := exprStmt.Expression.(*Identifier); ok {
				if ident.Name == "setup" {
					if blockStmt, ok := nextStatementBlock(block.Statements, statement); ok {
						setup = blockStmt
					}
				}
				if ident.Name == "teardown" {
					if blockStmt, ok := nextStatementBlock(block.Statements, statement); ok {
						teardown = blockStmt
					}
				}
			}
		}
	}
	if setup != nil {
		setupResult := Eval(setup, env)
		if isError(setupResult) {
			return setupResult
		}
	}

	for _, statement := range block.Statements {
		if exprStmt, ok := statement.(*ExpressionStatement); ok {
			if ident, ok := exprStmt.Expression.(*Identifier); ok {
				if ident.Name == "setup" || ident.Name == "teardown" {
					continue
				}
			}
		}
		result = Eval(statement, env)

		if result != nil {
			rt := result.Type()
			if rt == RETURN_VALUE_OBJ || rt == BREAK_OBJ || rt == CONTINUE_OBJ {
				return result
			}
			if isError(result) {
				return result
			}
		}
	}
	if teardown != nil {
		teardownResult := Eval(teardown, env)
		if isError(teardownResult) {
			return teardownResult
		}
	}

	return result
}

func nextStatementBlock(stmts []Statement, current Statement) (*BlockStatement, bool) {
	for i, s := range stmts {
		if s != current {
			continue
		}
		if i+1 < len(stmts) {
			if exprStmt, ok := stmts[i+1].(*ExpressionStatement); ok {
				if block, ok := exprStmt.Expression.(*FunctionLiteral); ok && block.Body != nil {
					return block.Body, true
				}
			}
		}
		break
	}
	return nil, false
}

func nativeBoolToBooleanObject(input bool) *Boolean {
	if input {
		return TRUE
	}
	return FALSE
}

func evalPrefixExpression(operator string, right Object) Object {
	if lazy, ok := right.(*LazyValue); ok {
		right = lazy.Force()
	}
	if owned, ok := right.(*OwnedValue); ok {
		right = owned.inner
	}
	switch operator {
	case "!":
		return evalBangOperatorExpression(right)
	case "-":
		return evalMinusPrefixOperatorExpression(right)
	case "~":
		return evalBitwiseNotExpression(right)
	case "typeof":
		return evalTypeofExpression(right)
	default:
		return newError("unknown operator: %s%s", operator, right.Type())
	}
}

func evalBitwiseNotExpression(right Object) Object {
	switch v := right.(type) {
	case *Integer:
		return integerObj(^v.Value)
	default:
		return newError("bitwise NOT not supported on %s", right.Type())
	}
}

func evalTypeofExpression(right Object) Object {
	if right == nil || right == NULL {
		return &String{Value: "null"}
	}
	switch right.(type) {
	case *Integer:
		return &String{Value: "integer"}
	case *Float:
		return &String{Value: "float"}
	case *String:
		return &String{Value: "string"}
	case *Boolean:
		return &String{Value: "boolean"}
	case *Array:
		return &String{Value: "array"}
	case *Hash:
		return &String{Value: "hash"}
	case *Function, *Builtin:
		return &String{Value: "function"}
	default:
		return &String{Value: right.Type().String()}
	}
}

func valueMatchesTypeName(val Object, typeName string) bool {
	t := strings.ToLower(strings.TrimSpace(typeName))
	switch t {
	case "any", "object":
		return true
	case "int", "integer":
		return val != nil && val.Type() == INTEGER_OBJ
	case "float", "number":
		return val != nil && (val.Type() == FLOAT_OBJ || val.Type() == INTEGER_OBJ)
	case "bool", "boolean":
		return val != nil && val.Type() == BOOLEAN_OBJ
	case "string":
		return val != nil && val.Type() == STRING_OBJ
	case "array", "list":
		return val != nil && val.Type() == ARRAY_OBJ
	case "hash", "map":
		return val != nil && val.Type() == HASH_OBJ
	case "null", "nil":
		return val == nil || val == NULL || val.Type() == NULL_OBJ
	case "function", "fn":
		if val == nil {
			return false
		}
		return val.Type() == FUNCTION_OBJ || val.Type() == BUILTIN_OBJ
	default:
		return true
	}
}

func evalBangOperatorExpression(right Object) Object {
	switch right {
	case TRUE:
		return FALSE
	case FALSE:
		return TRUE
	case NULL:
		return TRUE
	default:
		return FALSE
	}
}

func evalMinusPrefixOperatorExpression(right Object) Object {
	switch right.Type() {
	case INTEGER_OBJ:
		value := right.(*Integer).Value
		return &Integer{Value: -value}
	case FLOAT_OBJ:
		value := right.(*Float).Value
		return &Float{Value: -value}
	default:
		return newError("unknown operator: -%s", right.Type())
	}
}

func evalInfixExpression(operator string, left, right Object) Object {
	if lazy, ok := left.(*LazyValue); ok {
		left = lazy.Force()
	}
	if lazy, ok := right.(*LazyValue); ok {
		right = lazy.Force()
	}
	if owned, ok := left.(*OwnedValue); ok {
		left = owned.inner
	}
	if owned, ok := right.(*OwnedValue); ok {
		right = owned.inner
	}
	if operator == "&&" {
		if !isTruthy(left) {
			return FALSE
		}
		return nativeBoolToBooleanObject(isTruthy(right))
	}

	if operator == "||" {
		if isTruthy(left) {
			return TRUE
		}
		return nativeBoolToBooleanObject(isTruthy(right))
	}

	if operator == "??" {
		if left == NULL || left == nil {
			return right
		}
		return left
	}

	switch {
	case left.Type() == INTEGER_OBJ && right.Type() == INTEGER_OBJ:
		return evalIntegerInfixExpression(operator, left, right)
	case left.Type() == FLOAT_OBJ && right.Type() == FLOAT_OBJ:
		return evalFloatInfixExpression(operator, left, right)
	case left.Type() == INTEGER_OBJ && right.Type() == FLOAT_OBJ:
		return evalFloatInfixExpression(operator, &Float{Value: float64(left.(*Integer).Value)}, right)
	case left.Type() == FLOAT_OBJ && right.Type() == INTEGER_OBJ:
		return evalFloatInfixExpression(operator, left, &Float{Value: float64(right.(*Integer).Value)})
	case left.Type() == STRING_OBJ && right.Type() == STRING_OBJ:
		return evalStringInfixExpression(operator, left, right)
	case operator == "==":
		return nativeBoolToBooleanObject(left == right)
	case operator == "!=":
		return nativeBoolToBooleanObject(left != right)
	case operator == "+" && (left.Type() == STRING_OBJ || right.Type() == STRING_OBJ):
		return evalMixedStringConcatenation(left, right)
	case left.Type() != right.Type():
		return newError("type mismatch: %s %s %s", left.Type(), operator, right.Type())
	default:
		return newError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func evalFloatInfixExpression(operator string, left, right Object) Object {
	leftVal := left.(*Float).Value
	rightVal := right.(*Float).Value

	switch operator {
	case "+":
		return &Float{Value: leftVal + rightVal}
	case "-":
		return &Float{Value: leftVal - rightVal}
	case "*":
		return &Float{Value: leftVal * rightVal}
	case "/":
		if rightVal == 0 {
			return newError("division by zero")
		}
		return &Float{Value: leftVal / rightVal}
	case "<":
		return nativeBoolToBooleanObject(leftVal < rightVal)
	case ">":
		return nativeBoolToBooleanObject(leftVal > rightVal)
	case "<=":
		return nativeBoolToBooleanObject(leftVal <= rightVal)
	case ">=":
		return nativeBoolToBooleanObject(leftVal >= rightVal)
	case "==":
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return nativeBoolToBooleanObject(leftVal != rightVal)
	case "**":
		return &Float{Value: math.Pow(leftVal, rightVal)}
	default:
		return newError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func evalIntegerInfixExpression(operator string, left, right Object) Object {
	leftVal := left.(*Integer).Value
	rightVal := right.(*Integer).Value

	switch operator {
	case "+":
		return integerObj(leftVal + rightVal)
	case "-":
		return integerObj(leftVal - rightVal)
	case "*":
		return integerObj(leftVal * rightVal)
	case "/":
		if rightVal == 0 {
			return newError("division by zero")
		}
		return integerObj(leftVal / rightVal)
	case "%":
		if rightVal == 0 {
			return newError("division by zero")
		}
		return integerObj(leftVal % rightVal)
	case "<":
		return nativeBoolToBooleanObject(leftVal < rightVal)
	case ">":
		return nativeBoolToBooleanObject(leftVal > rightVal)
	case "<=":
		return nativeBoolToBooleanObject(leftVal <= rightVal)
	case ">=":
		return nativeBoolToBooleanObject(leftVal >= rightVal)
	case "==":
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return nativeBoolToBooleanObject(leftVal != rightVal)
	case "&":
		return integerObj(leftVal & rightVal)
	case "|":
		return integerObj(leftVal | rightVal)
	case "^":
		return integerObj(leftVal ^ rightVal)
	case "<<":
		return integerObj(leftVal << uint(rightVal))
	case ">>":
		return integerObj(leftVal >> uint(rightVal))
	case "**":
		return integerObj(int64(math.Pow(float64(leftVal), float64(rightVal))))
	default:
		return newError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func evalStringInfixExpression(operator string, left, right Object) Object {
	leftVal := left.(*String).Value
	rightVal := right.(*String).Value

	switch operator {
	case "+":
		return &String{Value: leftVal + rightVal}
	case "==":
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return nativeBoolToBooleanObject(leftVal != rightVal)
	default:
		return newError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func evalMixedStringConcatenation(left, right Object) Object {
	return &String{Value: left.Inspect() + right.Inspect()}
}

func evalIfExpression(ie *IfExpression, env *Environment) Object {
	condition := Eval(ie.Condition, env)
	if isError(condition) {
		return condition
	}

	if isTruthy(condition) {
		return Eval(ie.Consequence, env)
	} else if ie.Alternative != nil {
		return Eval(ie.Alternative, env)
	} else {
		return NULL
	}
}

var (
	cliTimeoutDur time.Duration
	cliMaxDepth   int
	cliMaxSteps   int64
	cliMaxHeapMB  int64
)

func evalWhileStatement(ws *WhileStatement, env *Environment) Object {
	var result Object = NULL

	for {
		condition := Eval(ws.Condition, env)
		if isError(condition) {
			return condition
		}

		if !isTruthy(condition) {
			break
		}

		result = Eval(ws.Body, env)
		if isError(result) {
			return result
		}

		if result != nil {
			rt := result.Type()
			if rt == RETURN_VALUE_OBJ {
				return result
			}
			if rt == BREAK_OBJ {
				result = NULL // Clear break object
				break
			}
			if rt == CONTINUE_OBJ {
				result = NULL // Clear continue object
				continue
			}
		}
	}

	return result
}

func evalForStatement(fs *ForStatement, env *Environment) Object {
	// 1. Init
	if fs.Init != nil {
		init := Eval(fs.Init, env)
		if isError(init) {
			return init
		}
	}

	var result Object = NULL

	for {
		// 2. Condition (with support for Break/Continue)
		if fs.Condition != nil {
			condition := Eval(fs.Condition, env)
			if isError(condition) {
				return condition
			}
			if !isTruthy(condition) {
				break
			}
		}

		// 3. Body
		result = Eval(fs.Body, env)
		if isError(result) {
			return result
		}

		if result != nil {
			rt := result.Type()
			if rt == RETURN_VALUE_OBJ {
				return result
			}
			if rt == BREAK_OBJ {
				result = NULL
				break
			}
			if rt == CONTINUE_OBJ {
				result = NULL
				// Fallthrough to Post
			}
		}

		// 4. Post
		if fs.Post != nil {
			post := Eval(fs.Post, env)
			if isError(post) {
				return post
			}
		}
	}

	return result
}

func evalTryCatchExpression(node *TryCatchExpression, env *Environment) Object {
	tryResult := Eval(node.TryBlock, env)

	var result Object
	if isError(tryResult) && node.CatchBlock != nil {
		catchEnv := NewEnclosedEnvironment(env)
		if node.CatchIdent != nil {
			if node.CatchType != "" {
				catchEnv.Set(node.CatchIdent.Name, errorObjectToHash(tryResult))
			} else {
				catchEnv.Set(node.CatchIdent.Name, &String{Value: objectErrorString(tryResult)})
			}
		}
		result = Eval(node.CatchBlock, catchEnv)
	} else {
		result = tryResult
	}

	if node.FinallyBlock != nil {
		finallyResult := Eval(node.FinallyBlock, env)
		// finally doesn't override the result unless it errors
		if isError(finallyResult) {
			return finallyResult
		}
	}

	if node.CatchType != "" && node.CatchBlock != nil && isError(result) {
		if errObj, ok := result.(*Error); ok {
			return errorObjectToHash(errObj)
		}
	}

	return result
}

// evalTargetGet reads the current value of an assignment target (Identifier, DotExpression, IndexExpression).
func evalTargetGet(target Expression, env *Environment) Object {
	switch t := target.(type) {
	case *Identifier:
		val, ok := env.Get(t.Name)
		if !ok {
			return newError("variable %s not declared", t.Name)
		}
		return val
	case *DotExpression:
		obj := Eval(t.Left, env)
		if isError(obj) {
			return obj
		}
		if owned, ok := obj.(*OwnedValue); ok {
			obj = owned.inner
		}
		hash, ok := obj.(*Hash)
		if !ok {
			return newError("dot access on %s is not supported", obj.Type())
		}
		key := &String{Value: t.Right.Name}
		pair, ok := hash.Pairs[key.HashKey()]
		if !ok {
			return NULL
		}
		return pair.Value
	case *IndexExpression:
		obj := Eval(t.Left, env)
		if isError(obj) {
			return obj
		}
		if owned, ok := obj.(*OwnedValue); ok {
			obj = owned.inner
		}
		index := Eval(t.Index, env)
		if isError(index) {
			return index
		}
		return evalIndexExpression(obj, index)
	default:
		return newError("invalid assignment target")
	}
}

// evalTargetAssign writes a value to an assignment target (Identifier, DotExpression, IndexExpression).
func evalTargetAssign(target Expression, val Object, env *Environment) Object {
	if owned, ok := val.(*OwnedValue); ok {
		val = owned.inner
	}
	switch t := target.(type) {
	case *Identifier:
		if _, ok := env.Assign(t.Name, val); ok {
			return val
		}
		return newError("variable %s not declared", t.Name)
	case *DotExpression:
		obj := Eval(t.Left, env)
		if isError(obj) {
			return obj
		}
		if _, ok := obj.(*ImmutableValue); ok {
			return newError("cannot mutate immutable value")
		}
		hash, ok := obj.(*Hash)
		if !ok {
			return newError("cannot set property on %s", obj.Type())
		}
		key := &String{Value: t.Right.Name}
		hash.Pairs[key.HashKey()] = HashPair{Key: key, Value: val}
		return val
	case *IndexExpression:
		obj := Eval(t.Left, env)
		if isError(obj) {
			return obj
		}
		if _, ok := obj.(*ImmutableValue); ok {
			return newError("cannot mutate immutable value")
		}
		index := Eval(t.Index, env)
		if isError(index) {
			return index
		}
		switch o := obj.(type) {
		case *Array:
			idx, ok := index.(*Integer)
			if !ok {
				return newError("array index must be integer, got %s", index.Type())
			}
			i := int(idx.Value)
			if i < 0 || i >= len(o.Elements) {
				return newError("index %d out of range [0..%d]", i, len(o.Elements)-1)
			}
			o.Elements[i] = val
			return val
		case *Hash:
			hashKey, ok := index.(Hashable)
			if !ok {
				return newError("unusable as hash key: %s", index.Type())
			}
			o.Pairs[hashKey.HashKey()] = HashPair{Key: index, Value: val}
			return val
		default:
			return newError("index assignment not supported on %s", obj.Type())
		}
	default:
		return newError("invalid assignment target")
	}
}

func evalDoWhileStatement(dw *DoWhileStatement, env *Environment) Object {
	var result Object = NULL

	for {
		result = Eval(dw.Body, env)
		if isError(result) {
			return result
		}
		if result != nil {
			rt := result.Type()
			if rt == RETURN_VALUE_OBJ {
				return result
			}
			if rt == BREAK_OBJ {
				return NULL
			}
			if rt == CONTINUE_OBJ {
				result = NULL
			}
		}

		condition := Eval(dw.Condition, env)
		if isError(condition) {
			return condition
		}
		if !isTruthy(condition) {
			break
		}
	}

	return result
}

func evalForInStatement(fi *ForInStatement, env *Environment) Object {
	iterable := Eval(fi.Iterable, env)
	if isError(iterable) {
		return iterable
	}

	var result Object = NULL

	switch iter := iterable.(type) {
	case *Array:
		for i, el := range iter.Elements {
			loopEnv := NewEnclosedEnvironment(env)
			if fi.KeyName != nil {
				loopEnv.Set(fi.KeyName.Name, integerObj(int64(i)))
			}
			loopEnv.Set(fi.ValueName.Name, el)

			result = Eval(fi.Body, loopEnv)
			if isError(result) {
				return result
			}
			if result != nil {
				rt := result.Type()
				if rt == RETURN_VALUE_OBJ {
					return result
				}
				if rt == BREAK_OBJ {
					return NULL
				}
				if rt == CONTINUE_OBJ {
					result = NULL
					continue
				}
			}
		}
	case *Hash:
		for _, pair := range iter.Pairs {
			loopEnv := NewEnclosedEnvironment(env)
			if fi.KeyName != nil {
				loopEnv.Set(fi.KeyName.Name, pair.Key)
			}
			loopEnv.Set(fi.ValueName.Name, pair.Value)

			result = Eval(fi.Body, loopEnv)
			if isError(result) {
				return result
			}
			if result != nil {
				rt := result.Type()
				if rt == RETURN_VALUE_OBJ {
					return result
				}
				if rt == BREAK_OBJ {
					return NULL
				}
				if rt == CONTINUE_OBJ {
					result = NULL
					continue
				}
			}
		}
	case *String:
		for i, ch := range iter.Value {
			loopEnv := NewEnclosedEnvironment(env)
			if fi.KeyName != nil {
				loopEnv.Set(fi.KeyName.Name, integerObj(int64(i)))
			}
			loopEnv.Set(fi.ValueName.Name, &String{Value: string(ch)})

			result = Eval(fi.Body, loopEnv)
			if isError(result) {
				return result
			}
			if result != nil {
				rt := result.Type()
				if rt == RETURN_VALUE_OBJ {
					return result
				}
				if rt == BREAK_OBJ {
					return NULL
				}
				if rt == CONTINUE_OBJ {
					result = NULL
					continue
				}
			}
		}
	default:
		return newError("cannot iterate over %s", iterable.Type())
	}

	return result
}

func evalSwitchStatement(ss *SwitchStatement, env *Environment) Object {
	switchVal := Eval(ss.Value, env)
	if isError(switchVal) {
		return switchVal
	}

	for _, sc := range ss.Cases {
		for _, caseVal := range sc.Values {
			cv := Eval(caseVal, env)
			if isError(cv) {
				return cv
			}
			if objectsEqual(switchVal, cv) {
				result := Eval(sc.Body, env)
				return unwrapSwitchResult(result)
			}
		}
	}

	if ss.Default != nil {
		result := Eval(ss.Default, env)
		return unwrapSwitchResult(result)
	}

	return NULL
}

func unwrapSwitchResult(result Object) Object {
	if result == nil {
		return NULL
	}
	if result.Type() == BREAK_OBJ {
		return NULL
	}
	return result
}

func evalTemplateLiteral(tl *TemplateLiteral, env *Environment) Object {
	var out strings.Builder
	for _, part := range tl.Parts {
		val := Eval(part, env)
		if isError(val) {
			return val
		}
		out.WriteString(val.Inspect())
	}
	return &String{Value: out.String()}
}

func objectsEqual(a, b Object) bool {
	if a.Type() != b.Type() {
		return false
	}
	switch av := a.(type) {
	case *Integer:
		return av.Value == b.(*Integer).Value
	case *Float:
		return av.Value == b.(*Float).Value
	case *String:
		return av.Value == b.(*String).Value
	case *Boolean:
		return av.Value == b.(*Boolean).Value
	case *Null:
		return true
	default:
		return a == b
	}
}

// evalRangeExpression evaluates a range expression like 1..5 or "a".."e" into an Array.
func evalRangeExpression(node *RangeExpression, env *Environment) Object {
	left := Eval(node.Left, env)
	if isError(left) {
		return left
	}
	right := Eval(node.Right, env)
	if isError(right) {
		return right
	}

	switch l := left.(type) {
	case *Integer:
		r, ok := right.(*Integer)
		if !ok {
			return newError("range: right side must be integer, got %s", right.Type())
		}
		low, high := l.Value, r.Value
		if low > high {
			// Descending range
			elements := make([]Object, 0, low-high+1)
			for i := low; i >= high; i-- {
				elements = append(elements, &Integer{Value: i})
			}
			return &Array{Elements: elements}
		}
		elements := make([]Object, 0, high-low+1)
		for i := low; i <= high; i++ {
			elements = append(elements, &Integer{Value: i})
		}
		return &Array{Elements: elements}

	case *String:
		r, ok := right.(*String)
		if !ok {
			return newError("range: right side must be string, got %s", right.Type())
		}
		lRunes := []rune(l.Value)
		rRunes := []rune(r.Value)
		if len(lRunes) != 1 || len(rRunes) != 1 {
			return newError("range: string range requires single characters, got %q..%q", l.Value, r.Value)
		}
		low, high := lRunes[0], rRunes[0]
		if low > high {
			elements := make([]Object, 0, low-high+1)
			for ch := low; ch >= high; ch-- {
				elements = append(elements, &String{Value: string(ch)})
			}
			return &Array{Elements: elements}
		}
		elements := make([]Object, 0, high-low+1)
		for ch := low; ch <= high; ch++ {
			elements = append(elements, &String{Value: string(ch)})
		}
		return &Array{Elements: elements}

	default:
		return newError("range: unsupported type %s", left.Type())
	}
}

func maybeWrapOwned(val Object, env *Environment) Object {
	if env == nil || val == nil {
		return val
	}
	switch v := val.(type) {
	case *OwnedValue:
		v.ownerID = env.ownerID
		return v
	case *Array, *Hash, *ADTValue:
		return &OwnedValue{ownerID: env.ownerID, inner: val}
	default:
		return val
	}
}

// --- Pattern matching evaluator ---

func evalMatchExpression(node *MatchExpression, env *Environment) Object {
	val := Eval(node.Value, env)
	if isError(val) {
		return val
	}
	if adt, ok := val.(*ADTValue); ok {
		if err := ensureExhaustiveMatchADT(node, adt); err != nil {
			return err
		}
	}

	for _, mc := range node.Cases {
		matchEnv := NewEnclosedEnvironment(env)
		if MatchPattern(mc.Pattern, val, matchEnv) {
			if mc.Guard != nil {
				guardResult := Eval(mc.Guard, matchEnv)
				if isError(guardResult) {
					return guardResult
				}
				if !isTruthy(guardResult) {
					continue
				}
			}
			result := Eval(mc.Body, matchEnv)
			if result == nil {
				return NULL
			}
			// Unwrap break like switch does
			if result.Type() == BREAK_OBJ {
				return NULL
			}
			return result
		}
	}
	return NULL
}

func ensureExhaustiveMatchADT(node *MatchExpression, val *ADTValue) Object {
	if node == nil || val == nil || len(val.AllVariants) == 0 {
		return nil
	}
	covered := make(map[string]bool, len(val.AllVariants))
	hasWildcard := false
	for _, c := range node.Cases {
		switch p := c.Pattern.(type) {
		case *WildcardPattern:
			hasWildcard = true
		case *BindingPattern:
			hasWildcard = true
		case *OrPattern:
			for _, sub := range p.Patterns {
				if cp, ok := sub.(*ConstructorPattern); ok {
					covered[cp.Name] = true
				}
			}
		case *ConstructorPattern:
			covered[p.Name] = true
		}
	}
	if hasWildcard {
		return nil
	}
	missing := make([]string, 0)
	for _, v := range val.AllVariants {
		if !covered[v] {
			missing = append(missing, v)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return newError("non-exhaustive ADT match for %s: missing %s", val.TypeName, strings.Join(missing, ", "))
}

func MatchPattern(pattern Pattern, value Object, env *Environment) bool {
	switch p := pattern.(type) {
	case *LiteralPattern:
		litVal := Eval(p.Value, env)
		if isError(litVal) {
			return false
		}
		return objectsEqual(value, litVal)

	case *WildcardPattern:
		return true

	case *BindingPattern:
		if p.TypeName != "" {
			if !typeMatches(value, p.TypeName) {
				return false
			}
		}
		if p.Name != nil && p.Name.Name != "_" {
			env.Set(p.Name.Name, value)
		}
		return true

	case *ArrayPattern:
		if owned, ok := value.(*OwnedValue); ok {
			value = owned.inner
		}
		arr, ok := value.(*Array)
		if !ok {
			return false
		}
		if p.Rest == nil {
			if len(arr.Elements) != len(p.Elements) {
				return false
			}
		} else {
			if len(arr.Elements) < len(p.Elements) {
				return false
			}
		}
		for i, elemPat := range p.Elements {
			if !MatchPattern(elemPat, arr.Elements[i], env) {
				return false
			}
		}
		if p.Rest != nil {
			restElems := arr.Elements[len(p.Elements):]
			env.Set(p.Rest.Name, &Array{Elements: restElems})
		}
		return true

	case *ObjectPattern:
		if owned, ok := value.(*OwnedValue); ok {
			value = owned.inner
		}
		hash, ok := value.(*Hash)
		if !ok {
			return false
		}
		for i, key := range p.Keys {
			strKey := &String{Value: key}
			hk := strKey.HashKey()
			pair, exists := hash.Pairs[hk]
			if !exists {
				return false
			}
			if !MatchPattern(p.Patterns[i], pair.Value, env) {
				return false
			}
		}
		if p.Rest != nil {
			rest := make(map[HashKey]HashPair)
			bound := make(map[string]bool, len(p.Keys))
			for _, key := range p.Keys {
				bound[key] = true
			}
			for hk, pair := range hash.Pairs {
				if strKey, ok := pair.Key.(*String); ok {
					if !bound[strKey.Value] {
						rest[hk] = pair
					}
				}
			}
			env.Set(p.Rest.Name, &Hash{Pairs: rest})
		}
		return true

	case *OrPattern:
		for _, subPat := range p.Patterns {
			tempEnv := NewEnclosedEnvironment(env)
			if MatchPattern(subPat, value, tempEnv) {
				// Copy bindings from temp env
				for k, v := range tempEnv.store {
					env.Set(k, v)
				}
				return true
			}
		}
		return false

	case *ExtractorPattern:
		return matchExtractor(p, value, env)

	case *ConstructorPattern:
		return matchConstructorPattern(p, value, env)

	case *RangePattern:
		return matchRange(p, value, env)

	case *ComparisonPattern:
		return matchComparison(p, value, env)
	}
	return false
}

func typeMatches(obj Object, typeName string) bool {
	switch typeName {
	case "integer":
		return obj.Type() == INTEGER_OBJ
	case "float":
		return obj.Type() == FLOAT_OBJ
	case "string":
		return obj.Type() == STRING_OBJ
	case "boolean":
		return obj.Type() == BOOLEAN_OBJ
	case "array":
		return obj.Type() == ARRAY_OBJ
	case "hash":
		return obj.Type() == HASH_OBJ
	case "function":
		_, ok1 := obj.(*Function)
		_, ok2 := obj.(*Builtin)
		return ok1 || ok2
	case "null":
		return obj == NULL
	}
	return false
}

func matchExtractor(p *ExtractorPattern, value Object, env *Environment) bool {
	switch p.Name {
	case "Some":
		if value == nil || value == NULL {
			return false
		}
		if len(p.Args) > 0 {
			return MatchPattern(p.Args[0], value, env)
		}
		return true

	case "None", "Nil":
		return value == nil || value == NULL

	case "All":
		for _, arg := range p.Args {
			tempEnv := NewEnclosedEnvironment(env)
			if !MatchPattern(arg, value, tempEnv) {
				return false
			}
			// Copy bindings
			for k, v := range tempEnv.store {
				env.Set(k, v)
			}
		}
		return true

	case "Any":
		for _, arg := range p.Args {
			tempEnv := NewEnclosedEnvironment(env)
			if MatchPattern(arg, value, tempEnv) {
				for k, v := range tempEnv.store {
					env.Set(k, v)
				}
				return true
			}
		}
		return false

	case "Tuple":
		arr, ok := value.(*Array)
		if !ok {
			return false
		}
		if len(arr.Elements) != len(p.Args) {
			return false
		}
		for i, arg := range p.Args {
			if !MatchPattern(arg, value.(*Array).Elements[i], env) {
				return false
			}
		}
		return true

	case "Regex":
		strVal, ok := value.(*String)
		if !ok {
			return false
		}
		if len(p.Args) == 0 {
			return false
		}
		// The regex arg should be a literal pattern with a string
		litPat, ok := p.Args[0].(*LiteralPattern)
		if !ok {
			return false
		}
		regexStr, ok := litPat.Value.(*StringLiteral)
		if !ok {
			return false
		}
		re, err := regexp.Compile(regexStr.Value)
		if err != nil {
			return false
		}
		return re.MatchString(strVal.Value)
	}
	return false
}

func matchConstructorPattern(p *ConstructorPattern, value Object, env *Environment) bool {
	if owned, ok := value.(*OwnedValue); ok {
		value = owned.inner
	}
	adt, ok := value.(*ADTValue)
	if !ok {
		return false
	}
	if p.Name != adt.VariantName {
		return false
	}
	if len(p.Args) != len(adt.Values) {
		return false
	}
	for i, arg := range p.Args {
		if !MatchPattern(arg, adt.Values[i], env) {
			return false
		}
	}
	return true
}

func objectCompare(a, b Object) (int, bool) {
	if a.Type() != b.Type() {
		// Allow int/float cross-comparison
		af, aOk := toFloat(a)
		bf, bOk := toFloat(b)
		if aOk && bOk {
			if af < bf {
				return -1, true
			} else if af > bf {
				return 1, true
			}
			return 0, true
		}
		return 0, false
	}
	switch av := a.(type) {
	case *Integer:
		bv := b.(*Integer).Value
		if av.Value < bv {
			return -1, true
		} else if av.Value > bv {
			return 1, true
		}
		return 0, true
	case *Float:
		bv := b.(*Float).Value
		if av.Value < bv {
			return -1, true
		} else if av.Value > bv {
			return 1, true
		}
		return 0, true
	case *String:
		bv := b.(*String).Value
		if av.Value < bv {
			return -1, true
		} else if av.Value > bv {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func toFloat(obj Object) (float64, bool) {
	switch v := obj.(type) {
	case *Integer:
		return float64(v.Value), true
	case *Float:
		return v.Value, true
	}
	return 0, false
}

func matchRange(p *RangePattern, value Object, env *Environment) bool {
	low := Eval(p.Low, env)
	if isError(low) {
		return false
	}
	high := Eval(p.High, env)
	if isError(high) {
		return false
	}
	cmpLow, okLow := objectCompare(value, low)
	cmpHigh, okHigh := objectCompare(value, high)
	if !okLow || !okHigh {
		return false
	}
	return cmpLow >= 0 && cmpHigh <= 0
}

func matchComparison(p *ComparisonPattern, value Object, env *Environment) bool {
	patVal := Eval(p.Value, env)
	if isError(patVal) {
		return false
	}
	cmp, ok := objectCompare(value, patVal)
	if !ok {
		return false
	}
	switch p.Operator {
	case TOKEN_GT:
		return cmp > 0
	case TOKEN_GTE:
		return cmp >= 0
	case TOKEN_LT:
		return cmp < 0
	case TOKEN_LTE:
		return cmp <= 0
	case TOKEN_NEQ:
		return cmp != 0
	}
	return false
}

func evalDestructure(pat *DestructurePattern, val Object, env *Environment) Object {
	if pat.Kind == "object" {
		hash, ok := val.(*Hash)
		if !ok {
			return newError("object destructuring requires a hash, got %s", val.Type())
		}
		for i, keyExpr := range pat.Keys {
			sl, ok := keyExpr.(*StringLiteral)
			if !ok {
				return newError("destructuring key must be a string")
			}
			key := &String{Value: sl.Value}
			hashed := key.HashKey()
			if pair, ok := hash.Pairs[hashed]; ok {
				env.Set(pat.Names[i].Name, pair.Value)
			} else if i < len(pat.Defaults) && pat.Defaults[i] != nil {
				def := Eval(pat.Defaults[i], env)
				if isError(def) {
					return def
				}
				env.Set(pat.Names[i].Name, def)
			} else {
				env.Set(pat.Names[i].Name, NULL)
			}
		}
		if pat.RestName != nil {
			rest := make(map[HashKey]HashPair)
			bound := make(map[HashKey]bool)
			for _, keyExpr := range pat.Keys {
				sl := keyExpr.(*StringLiteral)
				key := &String{Value: sl.Value}
				bound[key.HashKey()] = true
			}
			for k, v := range hash.Pairs {
				if !bound[k] {
					rest[k] = v
				}
			}
			env.Set(pat.RestName.Name, &Hash{Pairs: rest})
		}
	} else {
		// array destructuring
		arr, ok := val.(*Array)
		if !ok {
			return newError("array destructuring requires an array, got %s", val.Type())
		}
		for i, name := range pat.Names {
			if i < len(arr.Elements) {
				env.Set(name.Name, arr.Elements[i])
			} else if i < len(pat.Defaults) && pat.Defaults[i] != nil {
				def := Eval(pat.Defaults[i], env)
				if isError(def) {
					return def
				}
				env.Set(name.Name, def)
			} else {
				env.Set(name.Name, NULL)
			}
		}
		if pat.RestName != nil {
			startIdx := len(pat.Names)
			if startIdx < len(arr.Elements) {
				env.Set(pat.RestName.Name, &Array{Elements: arr.Elements[startIdx:]})
			} else {
				env.Set(pat.RestName.Name, &Array{Elements: []Object{}})
			}
		}
	}
	return NULL
}

func evalExportStatement(node *ExportStatement, env *Environment) Object {
	if env.moduleContext == nil {
		return newError("export is only allowed in module context")
	}
	if node.Declaration == nil {
		return newError("invalid export declaration")
	}

	result := Eval(node.Declaration, env)
	if isError(result) {
		return result
	}

	switch decl := node.Declaration.(type) {
	case *LetStatement:
		for _, name := range decl.Names {
			value, ok := env.Get(name.Name)
			if !ok {
				return newError("missing export binding: %s", name.Name)
			}
			env.moduleContext.Exports[name.Name] = value
		}
	case *DestructureLetStatement:
		for _, name := range decl.Pattern.Names {
			value, ok := env.Get(name.Name)
			if !ok {
				return newError("missing export binding: %s", name.Name)
			}
			env.moduleContext.Exports[name.Name] = value
		}
		if decl.Pattern.RestName != nil {
			value, ok := env.Get(decl.Pattern.RestName.Name)
			if !ok {
				return newError("missing export binding: %s", decl.Pattern.RestName.Name)
			}
			env.moduleContext.Exports[decl.Pattern.RestName.Name] = value
		}
	default:
		return newError("invalid export declaration type")
	}

	return NULL
}

func exportsToHashObject(exports map[string]Object) *Hash {
	pairs := make(map[HashKey]HashPair, len(exports))
	for name, value := range exports {
		key := &String{Value: name}
		pairs[key.HashKey()] = HashPair{Key: key, Value: value}
	}
	return &Hash{Pairs: pairs}
}

func evalImportStatement(node *ImportStatement, env *Environment) Object {
	if env == nil {
		return newError("cannot import without environment")
	}

	pathObj := Eval(node.Path, env)
	if isError(pathObj) {
		return pathObj
	}

	pathStr, ok := pathObj.(*String)
	if !ok {
		return newError("import path must be STRING, got %s", pathObj.Type())
	}

	resolvedPath, err := resolveImportPath(pathStr.Value, env)
	if err != nil {
		return newError("invalid import path %q: %s", pathStr.Value, err)
	}

	moduleLoading := env.moduleLoadingMap()
	if moduleLoading[resolvedPath] {
		return newError("module cycle detected while importing %q", pathStr.Value)
	}

	moduleCache := env.moduleCacheMap()
	if cachedEntry, ok := moduleCache[resolvedPath]; ok {
		cachedExports := cachedEntry.Exports
		if fi, statErr := os.Stat(resolvedPath); statErr == nil {
			if fi.ModTime().UnixNano() != cachedEntry.ModTime {
				delete(moduleCache, resolvedPath)
				cachedExports = nil
			}
		}
		if cachedExports == nil {
			goto evaluateModule
		}
		if len(node.Names) > 0 {
			for _, ident := range node.Names {
				val, exists := cachedExports[ident.Name]
				if !exists {
					return newError("module %q does not export %q", pathStr.Value, ident.Name)
				}
				env.Set(ident.Name, val)
			}
			return NULL
		}
		if node.Alias != nil {
			env.Set(node.Alias.Name, exportsToHashObject(cachedExports))
			return NULL
		}
		for name, value := range cachedExports {
			env.Set(name, value)
		}
		return NULL
	}

evaluateModule:

	moduleLoading[resolvedPath] = true
	defer delete(moduleLoading, resolvedPath)

	if err := checkFileReadAllowed(resolvedPath); err != nil {
		return newError("module read denied for %q: %s", pathStr.Value, err)
	}
	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return newError("failed to read module %q: %s", pathStr.Value, err)
	}

	moduleLexer := NewLexer(string(content))
	moduleParser := NewParser(moduleLexer)
	moduleProgram := moduleParser.ParseProgram()
	if len(moduleParser.Errors()) > 0 {
		return newError("module parse error in %q: %s", pathStr.Value, strings.Join(moduleParser.Errors(), "; "))
	}

	moduleEnv := NewEnvironment()
	moduleEnv.moduleContext = &ModuleContext{Exports: map[string]Object{}}
	moduleEnv.moduleCache = moduleCache
	moduleEnv.moduleLoading = moduleLoading
	moduleEnv.runtimeLimits = env.runtimeLimits
	moduleEnv.securityPolicy = env.securityPolicy
	moduleEnv.output = env.output
	moduleEnv.moduleDir = filepath.Dir(resolvedPath)
	moduleEnv.sourcePath = resolvedPath
	moduleEnv.callStack = cloneCallStack(env.callStack)

	moduleResult := Eval(moduleProgram, moduleEnv)
	if isError(moduleResult) {
		if errObj, ok := moduleResult.(*Error); ok {
			cloned := *errObj
			cloned.Message = fmt.Sprintf("module runtime error in %q: %s", pathStr.Value, errObj.Message)
			if cloned.Path == "" {
				cloned.Path = resolvedPath
			}
			return &cloned
		}
		return newError("module runtime error in %q: %s", pathStr.Value, objectErrorString(moduleResult))
	}

	if len(moduleEnv.moduleContext.Exports) == 0 {
		return newError("module %q has no exports", pathStr.Value)
	}

	exports := make(map[string]Object, len(moduleEnv.moduleContext.Exports))
	for name, value := range moduleEnv.moduleContext.Exports {
		exports[name] = value
	}
	modTime := int64(0)
	if fi, statErr := os.Stat(resolvedPath); statErr == nil {
		modTime = fi.ModTime().UnixNano()
	}
	moduleCache[resolvedPath] = ModuleCacheEntry{Exports: exports, ModTime: modTime}

	if len(node.Names) > 0 {
		selected := make(map[string]Object, len(node.Names))
		for _, ident := range node.Names {
			val, exists := exports[ident.Name]
			if !exists {
				return newError("module %q does not export %q", pathStr.Value, ident.Name)
			}
			selected[ident.Name] = val
		}
		if node.Alias != nil {
			env.Set(node.Alias.Name, exportsToHashObject(selected))
			return NULL
		}
		for name, value := range selected {
			env.Set(name, value)
		}
		return NULL
	}

	if node.Alias != nil {
		env.Set(node.Alias.Name, exportsToHashObject(exports))
		return NULL
	}

	for name, value := range exports {
		env.Set(name, value)
	}

	return NULL
}

func evalIndexExpression(left, index Object) Object {
	if imm, ok := left.(*ImmutableValue); ok {
		left = imm.inner
	}
	if gen, ok := left.(*GeneratorValue); ok {
		left = &Array{Elements: gen.elements}
	}
	switch {
	case left.Type() == ARRAY_OBJ && index.Type() == INTEGER_OBJ:
		return evalArrayIndexExpression(left, index)
	case left.Type() == HASH_OBJ:
		return evalHashIndexExpression(left, index)
	default:
		return newError("index operator not supported: %s", left.Type())
	}
}

func evalArrayIndexExpression(array, index Object) Object {
	arrayObject := array.(*Array)
	idx := index.(*Integer).Value
	max := int64(len(arrayObject.Elements) - 1)

	if idx < 0 || idx > max {
		return NULL
	}

	return arrayObject.Elements[idx]
}

func evalHashLiteral(node *HashLiteral, env *Environment) Object {
	pairs := make(map[HashKey]HashPair)

	for _, entry := range node.Entries {
		if entry.IsSpread {
			spreadObj := Eval(entry.Value, env)
			if isError(spreadObj) {
				return spreadObj
			}
			spreadHash, ok := spreadObj.(*Hash)
			if !ok {
				return newError("spread in object literal requires a hash, got %s", spreadObj.Type())
			}
			for k, v := range spreadHash.Pairs {
				pairs[k] = v
			}
			continue
		}

		key := Eval(entry.Key, env)
		if isError(key) {
			return key
		}

		// For non-computed keys (string/int/bool literals), use as-is.
		// For computed keys, the key expression was already evaluated above.
		hashKey, ok := key.(Hashable)
		if !ok {
			return newError("unusable as hash key: %s", key.Type())
		}

		value := Eval(entry.Value, env)
		if isError(value) {
			return value
		}

		hashed := hashKey.HashKey()
		pairs[hashed] = HashPair{Key: key, Value: value}
	}

	return &Hash{Pairs: pairs}
}

func evalHashIndexExpression(hash, index Object) Object {
	hashObject := hash.(*Hash)

	key, ok := index.(Hashable)
	if !ok {
		return newError("unusable as hash key: %s", index.Type())
	}

	pair, ok := hashObject.Pairs[key.HashKey()]
	if !ok {
		return NULL
	}

	return pair.Value
}

func isTruthy(obj Object) bool {
	if imm, ok := obj.(*ImmutableValue); ok {
		obj = imm.inner
	}
	if gen, ok := obj.(*GeneratorValue); ok {
		obj = &Array{Elements: gen.elements}
	}
	if owned, ok := obj.(*OwnedValue); ok {
		obj = owned.inner
	}
	switch obj {
	case NULL:
		return false
	case TRUE:
		return true
	case FALSE:
		return false
	default:
		return true
	}
}

// IsTruthy returns whether an SPL object is truthy.
func IsTruthy(obj Object) bool { return isTruthy(obj) }

func isError(obj Object) bool {
	switch v := obj.(type) {
	case *Error:
		return true
	case *String:
		return strings.HasPrefix(v.Value, "ERROR:")
	default:
		_ = v
		return false
	}
}

func evalIdentifier(node *Identifier, env *Environment) Object {
	if val, ok := env.Get(node.Name); ok {
		if lazy, ok := val.(*LazyValue); ok {
			return lazy.Force()
		}
		if owned, ok := val.(*OwnedValue); ok {
			if env != nil && owned.ownerID != "" && owned.ownerID != env.ownerID {
				return newError("ownership violation: value moved to another scope")
			}
			return owned.inner
		}
		return val
	}

	if builtin, ok := builtins[node.Name]; ok {
		return builtin.bindEnv(env)
	}

	return newError("identifier not found: %s", node.Name)
}

func evalExpressions(exps []Expression, env *Environment) []Object {
	result := make([]Object, 0, len(exps))

	for _, e := range exps {
		if spread, ok := e.(*SpreadExpression); ok {
			evaluated := Eval(spread.Value, env)
			if isError(evaluated) {
				return []Object{evaluated}
			}
			if arr, ok := evaluated.(*Array); ok {
				result = append(result, arr.Elements...)
			} else {
				return []Object{newError("spread operator requires an array, got %s", evaluated.Type())}
			}
			continue
		}
		evaluated := Eval(e, env)
		if isError(evaluated) {
			return []Object{evaluated}
		}
		result = append(result, evaluated)
	}

	return result
}

// applyFn is an indirect reference to applyFunction, used by builtins to avoid init cycles.
var applyFn func(Object, []Object, *Environment, *CallExpression) Object

func init() {
	applyFn = applyFunction
}

func cloneCallStack(stack []CallFrame) []CallFrame {
	if len(stack) == 0 {
		return nil
	}
	return append([]CallFrame(nil), stack...)
}

func callFrameFromExpression(function Expression, env *Environment, line, column int) CallFrame {
	frame := CallFrame{
		Function: strings.TrimSpace(function.String()),
		Line:     line,
		Column:   column,
	}
	if env != nil {
		frame.Path = env.sourcePath
	}
	return frame
}

func applyFunction(fn Object, args []Object, callerEnv *Environment, call *CallExpression) Object {
	if fn == nil {
		return newError("attempting to call nil function")
	}

	if errObj := validateFunctionCall(fn, len(args)); errObj != nil {
		if call != nil {
			if runtimeErr, ok := errObj.(*Error); ok {
				return runtimeErr.withFrame(callFrameFromExpression(call.Function, callerEnv, call.Line, call.Column))
			}
		}
		return errObj
	}

	switch fn := fn.(type) {
	case *Function:
		extendedEnv := extendFunctionEnv(fn, args, callerEnv, call)
		if fn.IsAsync {
			ch := make(chan Object, 1)
			go func() {
				result := Eval(fn.Body, extendedEnv)
				ch <- unwrapReturnValue(result)
			}()
			return &Future{ch: ch}
		}
		evaluated := Eval(fn.Body, extendedEnv)
		return unwrapReturnValue(evaluated)

	case *Builtin:
		if fn.FnWithEnv != nil {
			return fn.FnWithEnv(fn.Env, args...)
		}
		return fn.Fn(args...)

	default:
		return newError("not a function: %s", fn.Type())
	}
}

func validateFunctionCall(fn Object, argc int) Object {
	switch fn := fn.(type) {
	case *Function:
		if fn.HasRest {
			actualMin := 0
			for i := 0; i < len(fn.Parameters)-1; i++ {
				if i >= len(fn.Defaults) || fn.Defaults[i] == nil {
					actualMin = i + 1
				}
			}
			if argc < actualMin {
				return newError("wrong number of arguments. got=%d, want at least %d", argc, actualMin)
			}
			return nil
		}
		minArgs := 0
		for i, d := range fn.Defaults {
			if d == nil {
				minArgs = i + 1
			}
		}
		if argc < minArgs {
			return newError("wrong number of arguments. got=%d, want at least %d", argc, minArgs)
		}
		if argc > len(fn.Parameters) {
			return newError("wrong number of arguments. got=%d, want at most %d", argc, len(fn.Parameters))
		}
		return nil
	case *Builtin:
		return nil
	default:
		return newError("not a function: %s", fn.Type())
	}
}

func extendFunctionEnv(fn *Function, args []Object, callerEnv *Environment, call *CallExpression) *Environment {
	callStack := cloneCallStack(nil)
	if callerEnv != nil {
		callStack = cloneCallStack(callerEnv.callStack)
	}
	if call != nil {
		callStack = append(callStack, callFrameFromExpression(call.Function, callerEnv, call.Line, call.Column))
	}
	env := &Environment{
		store:          make(map[string]Object, len(fn.Parameters)),
		outer:          fn.Env,
		moduleContext:  fn.Env.moduleContext,
		moduleDir:      fn.Env.moduleDir,
		sourcePath:     fn.Env.sourcePath,
		moduleCache:    fn.Env.moduleCache,
		moduleLoading:  fn.Env.moduleLoading,
		runtimeLimits:  fn.Env.runtimeLimits,
		securityPolicy: fn.Env.securityPolicy,
		output:         fn.Env.output,
		callStack:      callStack,
	}
	if len(fn.ParamTypes) > 0 {
		env.Set("__param_types", toObject(fn.ParamTypes))
	}
	if fn.ReturnType != "" {
		env.Set("__return_type", &String{Value: fn.ReturnType})
	}

	if fn.HasRest {
		regularCount := len(fn.Parameters) - 1
		for i := 0; i < regularCount; i++ {
			if i < len(args) {
				env.Set(fn.Parameters[i].Name, args[i])
			} else if i < len(fn.Defaults) && fn.Defaults[i] != nil {
				defaultVal := Eval(fn.Defaults[i], fn.Env)
				env.Set(fn.Parameters[i].Name, defaultVal)
			} else {
				env.Set(fn.Parameters[i].Name, NULL)
			}
		}
		restElements := []Object{}
		if len(args) > regularCount {
			restElements = args[regularCount:]
		}
		env.Set(fn.Parameters[regularCount].Name, &Array{Elements: restElements})
	} else {
		for paramIdx, param := range fn.Parameters {
			if paramIdx < len(args) {
				env.Set(param.Name, args[paramIdx])
			} else if paramIdx < len(fn.Defaults) && fn.Defaults[paramIdx] != nil {
				defaultVal := Eval(fn.Defaults[paramIdx], fn.Env)
				env.Set(param.Name, defaultVal)
			} else {
				env.Set(param.Name, NULL)
			}
		}
	}

	return env
}

func unwrapReturnValue(obj Object) Object {
	if returnValue, ok := obj.(*ReturnValue); ok {
		return returnValue.Value
	}
	return obj
}

func StartCLI() {
	rand.Seed(time.Now().UnixNano())
	timeout := flag.Duration("timeout", 0, "Execution timeout (0 = no limit)")
	maxDepth := flag.Int("max-depth", 0, "Max recursion depth (0 = unlimited)")
	maxSteps := flag.Int64("max-steps", 0, "Max evaluation steps (0 = unlimited)")
	maxHeapMB := flag.Int64("max-heap-mb", 0, "Max heap usage in MB (0 = unlimited)")
	flag.Parse()
	cliTimeoutDur = *timeout
	cliMaxDepth = *maxDepth
	cliMaxSteps = *maxSteps
	cliMaxHeapMB = *maxHeapMB

	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "Runtime Panic: %v\n", r)
			os.Exit(2)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nCancelling execution...")
		os.Exit(130)
	}()

	if cliTimeoutDur > 0 {
		time.AfterFunc(cliTimeoutDur, func() {
			fmt.Println("\nTimeout reached.")
			os.Exit(3)
		})
	}

	args := flag.Args()
	if len(args) > 0 {
		runFile(args[0], args[1:])
	} else {
		runRepl()
	}
}

func runFile(filename string, args []string) {
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", paint(fmt.Sprintf("Error reading file: %v", err), colorBold+colorRed))
		os.Exit(1)
	}

	sb := DefaultExecSandboxConfig()
	vm, vmErr := NewSandboxVM(args, filename, filepath.Dir(filename), sb)
	if vmErr != nil {
		fmt.Fprintf(os.Stderr, "%s\n", paint(fmt.Sprintf("Sandbox configuration error: %v", vmErr), colorBold+colorRed))
		os.Exit(1)
	}
	env := vm.Environment()
	env.sourcePath = filename
	if env.runtimeLimits == nil {
		env.runtimeLimits = &RuntimeLimits{heapCheckEvery: 128}
	}
	if cliMaxDepth > 0 {
		env.runtimeLimits.MaxDepth = cliMaxDepth
	}
	if cliMaxSteps > 0 {
		env.runtimeLimits.MaxSteps = cliMaxSteps
	}
	if cliMaxHeapMB > 0 {
		env.runtimeLimits.MaxHeapBytes = uint64(cliMaxHeapMB) * 1024 * 1024
	}
	if cliTimeoutDur > 0 {
		env.runtimeLimits.Deadline = time.Now().Add(cliTimeoutDur)
	}
	if env.runtimeLimits.MaxDepth == 0 && env.runtimeLimits.MaxSteps == 0 && env.runtimeLimits.MaxHeapBytes == 0 && env.runtimeLimits.Deadline.IsZero() {
		env.runtimeLimits = nil
	}
	l := NewLexer(string(content))
	p := NewParser(l)
	program := p.ParseProgram()

	if len(p.Errors()) != 0 {
		for _, msg := range p.Errors() {
			fmt.Println(paint(msg, colorRed))
		}
		os.Exit(1)
	}

	evaluated := runProgramSandboxed(program, env, vm.Policy())
	if evaluated != nil {
		if isError(evaluated) {
			fmt.Println(paint("ERROR: "+objectErrorString(evaluated), colorBold+colorRed))
			os.Exit(1)
		} else if evaluated.Type() == RETURN_VALUE_OBJ {
			// Check if return value is integer to use as exit code
			val := evaluated.(*ReturnValue).Value
			if val.Type() == INTEGER_OBJ {
				os.Exit(int(val.(*Integer).Value))
			}
		}
	}
}

// Helper to secure file paths
func sanitizePath(userPath string) (string, error) {
	base := activeSandboxBaseDir()
	if strings.TrimSpace(base) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		base = cwd
	}

	// 1. Get Absolute Path
	absPath, err := filepath.Abs(userPath)
	if err != nil {
		return "", err
	}

	// 2. Resolve Symlinks (handle non-existent files for write ops)
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, verify parent directory instead
			dir := filepath.Dir(absPath)
			realDir, dirErr := filepath.EvalSymlinks(dir)
			if dirErr != nil {
				// If parent doesn't exist or is invalid, we can't safely verify or write
				return "", dirErr
			}
			realPath = filepath.Join(realDir, filepath.Base(absPath))
		} else {
			return "", err
		}
	}

	cleanPath := filepath.Clean(realPath)

	// 3. Prefix Check (Robust)
	// Add separator to ensure partial prefix matches like /dir vs /dir-hack fail
	baseClean := filepath.Clean(base)
	if !strings.HasSuffix(baseClean, string(os.PathSeparator)) {
		baseClean += string(os.PathSeparator)
	}

	checkPath := cleanPath
	if !strings.HasSuffix(checkPath, string(os.PathSeparator)) {
		checkPath += string(os.PathSeparator)
	}

	if !strings.HasPrefix(checkPath, baseClean) {
		return "", fmt.Errorf("access denied: path '%s' is outside project root", userPath)
	}
	return cleanPath, nil
}

func runRepl() {
	fmt.Println(paint("Welcome to the Simple Programming Language!", colorBold+colorMagenta))
	fmt.Println(paint("Type 'exit' to quit", colorCyan))
	fmt.Println(paint("Type ':help' for interactive shortcuts", colorCyan))
	fmt.Println(paint("For multi-line input: ensure braces {} are balanced", colorGray))
	fmt.Println(paint("Or run: go run ./cmd/interpreter <filename>", colorGray))
	fmt.Println()

	sb := DefaultReplSandboxConfig()
	vm, vmErr := NewSandboxVM([]string{}, "<repl>", ".", sb)
	if vmErr != nil {
		fmt.Printf("%s\n", paint(fmt.Sprintf("Sandbox configuration error: %v", vmErr), colorBold+colorRed))
		return
	}
	env := vm.Environment()
	if err := runReplInteractive(env); err != nil {
		fmt.Printf("%s\n", paint(fmt.Sprintf("Interactive mode unavailable (%v). Falling back to basic mode.", err), colorYellow))
		runReplBasic(env)
	}
}

func countBraces(line string) int {
	count := 0
	for _, ch := range line {
		switch ch {
		case '{':
			count++
		case '}':
			count--
		}
	}
	return count
}

func evalDotExpression(left Object, name string) Object {
	if imm, ok := left.(*ImmutableValue); ok {
		left = imm.inner
	}
	if gen, ok := left.(*GeneratorValue); ok {
		left = &Array{Elements: gen.elements}
	}
	// 1. Property Access for Hash
	if hash, ok := left.(*Hash); ok {
		key := &String{Value: name}
		hashed := key.HashKey()
		if pair, ok := hash.Pairs[hashed]; ok {
			return pair.Value
		}
		// Check for hash methods
		if method := getHashMethod(hash, name); method != nil {
			return method
		}
		return NULL
	}

	// 2. Method Access
	switch obj := left.(type) {
	case *Array:
		return getArrayMethod(obj, name)
	case *String:
		return getStringMethod(obj, name)
	case *Integer:
		return getIntegerMethod(obj, name)
	case *Float:
		return getFloatMethod(obj, name)
	case *SPLServer:
		if prop := GetServerProperty(obj, name); prop != nil {
			return prop
		}
	case *SPLRequest:
		if prop := GetRequestProperty(obj, name); prop != nil {
			return prop
		}
	case *SPLResponse:
		if prop := GetResponseProperty(obj, name); prop != nil {
			return prop
		}
	case *SSEWriter:
		if prop := GetSSEWriterProperty(obj, name); prop != nil {
			return prop
		}
	case *QueryBuilder:
		if prop := GetQueryBuilderProperty(obj, name); prop != nil {
			return prop
		}
	case *LazyDBQuery:
		// Force the lazy query and then access property on result
		result := obj.Force()
		return evalDotExpression(result, name)
	case *Signal:
		if prop := GetSignalProperty(obj, name); prop != nil {
			return prop
		}
	case *Computed:
		if prop := GetComputedProperty(obj, name); prop != nil {
			return prop
		}
	case *Effect:
		if prop := GetEffectProperty(obj, name); prop != nil {
			return prop
		}
	}

	return newError("property or method '%s' not found on %s", name, left.Type())
}

func bindMethod(receiver Object, methodName, builtinName string) Object {
	b, ok := builtins[builtinName]
	if !ok {
		return newError("method '%s' is unavailable", methodName)
	}
	return &Builtin{
		Fn: func(args ...Object) Object {
			callArgs := make([]Object, 0, len(args)+1)
			callArgs = append(callArgs, receiver)
			callArgs = append(callArgs, args...)
			return b.Fn(callArgs...)
		},
	}
}

func getHashMethod(hash *Hash, name string) Object {
	switch name {
	case "keys":
		return &Builtin{Fn: func(args ...Object) Object {
			keys := make([]Object, 0, len(hash.Pairs))
			for _, pair := range hash.Pairs {
				keys = append(keys, pair.Key)
			}
			return &Array{Elements: keys}
		}}
	case "values":
		return &Builtin{Fn: func(args ...Object) Object {
			values := make([]Object, 0, len(hash.Pairs))
			for _, pair := range hash.Pairs {
				values = append(values, pair.Value)
			}
			return &Array{Elements: values}
		}}
	case "entries":
		return &Builtin{Fn: func(args ...Object) Object {
			entries := make([]Object, 0, len(hash.Pairs))
			for _, pair := range hash.Pairs {
				entries = append(entries, &Array{Elements: []Object{pair.Key, pair.Value}})
			}
			return &Array{Elements: entries}
		}}
	case "length":
		return &Integer{Value: int64(len(hash.Pairs))}
	default:
		return nil
	}
}

func getStringMethod(str *String, name string) Object {
	switch name {
	case "length":
		return &Integer{Value: int64(len([]rune(str.Value)))}
	case "upper", "toUpperCase":
		return bindMethod(str, name, "upper")
	case "lower", "toLowerCase":
		return bindMethod(str, name, "lower")
	case "trim":
		return bindMethod(str, name, "trim")
	case "starts_with", "startsWith":
		return bindMethod(str, name, "starts_with")
	case "ends_with", "endsWith":
		return bindMethod(str, name, "ends_with")
	case "includes":
		return bindMethod(str, name, "contains")
	case "replace":
		return bindMethod(str, name, "replace")
	case "repeat":
		return bindMethod(str, name, "repeat")
	case "substring":
		return bindMethod(str, name, "substring")
	case "index_of", "indexOf":
		return bindMethod(str, name, "index_of")
	case "split":
		return bindMethod(str, name, "split")
	case "title":
		return bindMethod(str, name, "title")
	case "slug":
		return bindMethod(str, name, "slug")
	case "snake_case":
		return bindMethod(str, name, "snake_case")
	case "kebab_case":
		return bindMethod(str, name, "kebab_case")
	case "camel_case":
		return bindMethod(str, name, "camel_case")
	case "pascal_case":
		return bindMethod(str, name, "pascal_case")
	case "swap_case":
		return bindMethod(str, name, "swap_case")
	case "count_substr":
		return bindMethod(str, name, "count_substr")
	case "truncate":
		return bindMethod(str, name, "truncate")
	case "split_lines":
		return bindMethod(str, name, "split_lines")
	case "regex_match":
		return bindMethod(str, name, "regex_match")
	case "regex_replace":
		return bindMethod(str, name, "regex_replace")
	case "trim_prefix":
		return bindMethod(str, name, "trim_prefix")
	case "trim_suffix":
		return bindMethod(str, name, "trim_suffix")
	case "pad_left", "padStart":
		return bindMethod(str, name, "pad_left")
	case "pad_right", "padEnd":
		return bindMethod(str, name, "pad_right")
	case "charAt":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return newError("charAt() takes exactly 1 argument")
			}
			idx, ok := args[0].(*Integer)
			if !ok {
				return newError("charAt() argument must be integer")
			}
			runes := []rune(str.Value)
			i := int(idx.Value)
			if i < 0 || i >= len(runes) {
				return &String{Value: ""}
			}
			return &String{Value: string(runes[i])}
		}}
	case "at":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return newError("at() takes exactly 1 argument")
			}
			idx, ok := args[0].(*Integer)
			if !ok {
				return newError("at() argument must be integer")
			}
			runes := []rune(str.Value)
			i := int(idx.Value)
			if i < 0 {
				i = len(runes) + i
			}
			if i < 0 || i >= len(runes) {
				return NULL
			}
			return &String{Value: string(runes[i])}
		}}
	default:
		return newError("method '%s' not found on STRING", name)
	}
}

func methodNoArg(receiver Object, name string, fn func() Object) Object {
	return &Builtin{
		Fn: func(args ...Object) Object {
			if len(args) != 0 {
				return newError("%s expects 0 arguments, got %d", name, len(args))
			}
			return fn()
		},
	}
}

func bindIntegerTimeMethod(ts *Integer, methodName, builtinName string) Object {
	b, ok := builtins[builtinName]
	if !ok {
		return newError("method '%s' is unavailable", methodName)
	}
	return &Builtin{
		Fn: func(args ...Object) Object {
			callArgs := make([]Object, 0, len(args)+1)
			callArgs = append(callArgs, ts)
			callArgs = append(callArgs, args...)
			return b.Fn(callArgs...)
		},
	}
}

func getIntegerMethod(num *Integer, name string) Object {
	switch name {
	case "to_string", "toString":
		return bindMethod(num, name, "to_string")
	case "to_float", "toFloat":
		return bindMethod(num, name, "to_float")
	case "abs":
		return methodNoArg(num, name, func() Object { return &Integer{Value: int64(math.Abs(float64(num.Value)))} })
	case "is_even", "isEven":
		return methodNoArg(num, name, func() Object { return nativeBoolToBooleanObject(num.Value%2 == 0) })
	case "is_odd", "isOdd":
		return methodNoArg(num, name, func() Object { return nativeBoolToBooleanObject(num.Value%2 != 0) })
	case "sqrt":
		return bindMethod(num, name, "sqrt")
	case "pow":
		return bindMethod(num, name, "pow")
	case "round", "floor", "ceil":
		return methodNoArg(num, name, func() Object { return &Integer{Value: num.Value} })

	// Timestamp/time helpers on unix-seconds integer values.
	case "to_iso", "toISO":
		return bindIntegerTimeMethod(num, name, "unix_to_iso")
	case "format":
		return bindIntegerTimeMethod(num, name, "format_time")
	case "format_tz", "formatTZ":
		return bindIntegerTimeMethod(num, name, "format_time_tz")
	case "add":
		return bindIntegerTimeMethod(num, name, "time_add")
	case "sub":
		return bindIntegerTimeMethod(num, name, "time_sub")
	case "diff":
		return bindIntegerTimeMethod(num, name, "time_diff")
	case "start_of_day", "startOfDay":
		return bindIntegerTimeMethod(num, name, "start_of_day")
	case "end_of_day", "endOfDay":
		return bindIntegerTimeMethod(num, name, "end_of_day")
	case "start_of_week", "startOfWeek":
		return bindIntegerTimeMethod(num, name, "start_of_week")
	case "end_of_month", "endOfMonth":
		return bindIntegerTimeMethod(num, name, "end_of_month")
	case "add_months", "addMonths":
		return bindIntegerTimeMethod(num, name, "add_months")
	case "to_timezone", "toTimezone":
		return bindIntegerTimeMethod(num, name, "to_timezone")
	default:
		return newError("method '%s' not found on INTEGER", name)
	}
}

func getFloatMethod(num *Float, name string) Object {
	switch name {
	case "to_string", "toString":
		return bindMethod(num, name, "to_string")
	case "to_int", "toInt":
		return bindMethod(num, name, "to_int")
	case "abs":
		return methodNoArg(num, name, func() Object { return &Float{Value: math.Abs(num.Value)} })
	case "round":
		return bindMethod(num, name, "round")
	case "floor":
		return bindMethod(num, name, "floor")
	case "ceil":
		return bindMethod(num, name, "ceil")
	default:
		return newError("method '%s' not found on FLOAT", name)
	}
}

func getArrayMethod(arr *Array, name string) Object {
	switch name {
	case "map":
		return &Builtin{
			Fn: func(args ...Object) Object {
				if len(args) != 1 {
					return newError("map expects 1 argument, got %d", len(args))
				}
				_, ok := args[0].(*Function)
				if !ok {
					// Also support Builtin as callback?
					_, isBuiltin := args[0].(*Builtin)
					if !isBuiltin {
						return newError("map expects a function")
					}
				}

				newElements := make([]Object, len(arr.Elements))
				for i, el := range arr.Elements {
					// Call function
					// We need 'applyFunction' logic.
					// Since we are inside a Builtin Fn, we don't have access to 'applyFunction' easily unless we export it or duplicate logic.
					// Or call Eval? No, Eval takes AST.
					// We need to execute the function object.
					// Helper: executeCallback(fn, []Object{el})
					res := executeCallback(args[0], []Object{el})
					if isError(res) {
						return res
					}
					newElements[i] = res
				}
				return &Array{Elements: newElements}
			},
		}
	case "filter":
		return &Builtin{
			Fn: func(args ...Object) Object {
				if len(args) != 1 {
					return newError("filter expects 1 argument")
				}

				newElements := []Object{}
				for _, el := range arr.Elements {
					res := executeCallback(args[0], []Object{el})
					if isError(res) {
						return res
					}
					if isTruthy(res) {
						newElements = append(newElements, el)
					}
				}
				return &Array{Elements: newElements}
			},
		}
	case "forEach":
		return &Builtin{
			Fn: func(args ...Object) Object {
				if len(args) != 1 {
					return newError("forEach expects 1 argument")
				}
				for _, el := range arr.Elements {
					res := executeCallback(args[0], []Object{el})
					if isError(res) {
						return res
					}
				}
				return NULL
			},
		}
	case "push":
		return &Builtin{
			Fn: func(args ...Object) Object {
				// Mutating the array in place?
				// The Array struct has a slice. If we append, we might need to update the pointer or slice header.
				// Since we passed *Array, we can modify Elements.
				for _, arg := range args {
					arr.Elements = append(arr.Elements, arg)
				}
				return &Integer{Value: int64(len(arr.Elements))}
			},
		}
	case "find":
		return &Builtin{
			Fn: func(args ...Object) Object {
				if len(args) != 1 {
					return newError("find expects 1 argument")
				}
				for _, el := range arr.Elements {
					res := executeCallback(args[0], []Object{el})
					if isTruthy(res) {
						return el
					}
				}
				return NULL
			},
		}
	case "length":
		return &Integer{Value: int64(len(arr.Elements))}
	case "every":
		return &Builtin{
			Fn: func(args ...Object) Object {
				if len(args) != 1 {
					return newError("every expects 1 argument")
				}
				for _, el := range arr.Elements {
					res := executeCallback(args[0], []Object{el})
					if isError(res) {
						return res
					}
					if !isTruthy(res) {
						return FALSE
					}
				}
				return TRUE
			},
		}
	case "some":
		return &Builtin{
			Fn: func(args ...Object) Object {
				if len(args) != 1 {
					return newError("some expects 1 argument")
				}
				for _, el := range arr.Elements {
					res := executeCallback(args[0], []Object{el})
					if isError(res) {
						return res
					}
					if isTruthy(res) {
						return TRUE
					}
				}
				return FALSE
			},
		}
	case "reduce":
		return &Builtin{
			Fn: func(args ...Object) Object {
				if len(args) < 1 || len(args) > 2 {
					return newError("reduce expects 1-2 arguments (callback, optional initial)")
				}
				var acc Object
				startIdx := 0
				if len(args) == 2 {
					acc = args[1]
				} else {
					if len(arr.Elements) == 0 {
						return newError("reduce of empty array with no initial value")
					}
					acc = arr.Elements[0]
					startIdx = 1
				}
				for i := startIdx; i < len(arr.Elements); i++ {
					res := executeCallback(args[0], []Object{acc, arr.Elements[i]})
					if isError(res) {
						return res
					}
					acc = res
				}
				return acc
			},
		}
	case "indexOf":
		return &Builtin{
			Fn: func(args ...Object) Object {
				if len(args) != 1 {
					return newError("indexOf expects 1 argument")
				}
				target := args[0]
				for i, el := range arr.Elements {
					if objectsEqual(el, target) {
						return integerObj(int64(i))
					}
				}
				return integerObj(-1)
			},
		}
	case "includes":
		return &Builtin{
			Fn: func(args ...Object) Object {
				if len(args) != 1 {
					return newError("includes expects 1 argument")
				}
				target := args[0]
				for _, el := range arr.Elements {
					if objectsEqual(el, target) {
						return TRUE
					}
				}
				return FALSE
			},
		}
	case "join":
		return &Builtin{
			Fn: func(args ...Object) Object {
				sep := ","
				if len(args) > 0 {
					if s, ok := args[0].(*String); ok {
						sep = s.Value
					}
				}
				parts := make([]string, len(arr.Elements))
				for i, el := range arr.Elements {
					parts[i] = el.Inspect()
				}
				return &String{Value: strings.Join(parts, sep)}
			},
		}
	case "flat":
		return &Builtin{
			Fn: func(args ...Object) Object {
				result := []Object{}
				for _, el := range arr.Elements {
					if inner, ok := el.(*Array); ok {
						result = append(result, inner.Elements...)
					} else {
						result = append(result, el)
					}
				}
				return &Array{Elements: result}
			},
		}
	case "flatMap":
		return &Builtin{
			Fn: func(args ...Object) Object {
				if len(args) != 1 {
					return newError("flatMap expects 1 argument")
				}
				result := []Object{}
				for _, el := range arr.Elements {
					res := executeCallback(args[0], []Object{el})
					if isError(res) {
						return res
					}
					if inner, ok := res.(*Array); ok {
						result = append(result, inner.Elements...)
					} else {
						result = append(result, res)
					}
				}
				return &Array{Elements: result}
			},
		}
	case "reverse":
		return &Builtin{
			Fn: func(args ...Object) Object {
				n := len(arr.Elements)
				result := make([]Object, n)
				for i, el := range arr.Elements {
					result[n-1-i] = el
				}
				return &Array{Elements: result}
			},
		}
	case "slice":
		return &Builtin{
			Fn: func(args ...Object) Object {
				if len(args) < 1 {
					return newError("slice expects at least 1 argument")
				}
				start, ok := args[0].(*Integer)
				if !ok {
					return newError("slice start must be an integer")
				}
				s := int(start.Value)
				if s < 0 {
					s = len(arr.Elements) + s
				}
				if s < 0 {
					s = 0
				}
				if s > len(arr.Elements) {
					s = len(arr.Elements)
				}
				e := len(arr.Elements)
				if len(args) > 1 {
					end, ok := args[1].(*Integer)
					if !ok {
						return newError("slice end must be an integer")
					}
					e = int(end.Value)
					if e < 0 {
						e = len(arr.Elements) + e
					}
					if e < 0 {
						e = 0
					}
					if e > len(arr.Elements) {
						e = len(arr.Elements)
					}
				}
				if s > e {
					return &Array{Elements: []Object{}}
				}
				return &Array{Elements: append([]Object{}, arr.Elements[s:e]...)}
			},
		}
	case "sort":
		return &Builtin{
			Fn: func(args ...Object) Object {
				if len(arr.Elements) == 0 {
					return &Array{Elements: []Object{}}
				}
				sorted := make([]Object, len(arr.Elements))
				copy(sorted, arr.Elements)
				sort.Slice(sorted, func(i, j int) bool {
					a, b := sorted[i], sorted[j]
					if a.Type() == INTEGER_OBJ && b.Type() == INTEGER_OBJ {
						return a.(*Integer).Value < b.(*Integer).Value
					}
					if a.Type() == FLOAT_OBJ && b.Type() == FLOAT_OBJ {
						return a.(*Float).Value < b.(*Float).Value
					}
					if a.Type() == STRING_OBJ && b.Type() == STRING_OBJ {
						return a.(*String).Value < b.(*String).Value
					}
					return a.Inspect() < b.Inspect()
				})
				return &Array{Elements: sorted}
			},
		}
	case "pop":
		return &Builtin{
			Fn: func(args ...Object) Object {
				if len(arr.Elements) == 0 {
					return NULL
				}
				last := arr.Elements[len(arr.Elements)-1]
				arr.Elements = arr.Elements[:len(arr.Elements)-1]
				return last
			},
		}
	case "shift":
		return &Builtin{
			Fn: func(args ...Object) Object {
				if len(arr.Elements) == 0 {
					return NULL
				}
				first := arr.Elements[0]
				arr.Elements = arr.Elements[1:]
				return first
			},
		}
	case "unshift":
		return &Builtin{
			Fn: func(args ...Object) Object {
				newElements := make([]Object, 0, len(args)+len(arr.Elements))
				newElements = append(newElements, args...)
				newElements = append(newElements, arr.Elements...)
				arr.Elements = newElements
				return integerObj(int64(len(arr.Elements)))
			},
		}
	}
	if name == "at" {
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return newError("at() takes exactly 1 argument")
			}
			idx, ok := args[0].(*Integer)
			if !ok {
				return newError("at() argument must be integer")
			}
			i := int(idx.Value)
			if i < 0 {
				i = len(arr.Elements) + i
			}
			if i < 0 || i >= len(arr.Elements) {
				return NULL
			}
			return arr.Elements[i]
		}}
	}
	return newError("method '%s' not found on ARRAY", name)
}

func executeCallback(fnObj Object, args []Object) Object {
	switch fn := fnObj.(type) {
	case *Function:
		extendedEnv := NewEnclosedEnvironment(fn.Env)
		for i, param := range fn.Parameters {
			if i < len(args) {
				extendedEnv.Set(param.Name, args[i])
			}
		}
		// Also support implicit 'it' if no params? No.

		evaluated := Eval(fn.Body, extendedEnv)
		return unwrapReturnValue(evaluated)

	case *Builtin:
		return fn.Fn(args...)

	default:
		return newError("not a function")
	}
}
