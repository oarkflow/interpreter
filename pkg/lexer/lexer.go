package lexer

import (
	"strings"

	"github.com/oarkflow/interpreter/pkg/token"
)

// Lexer performs lexical analysis on the input string,
// producing a stream of tokens.
type Lexer struct {
	input        string
	position     int
	readPosition int
	ch           byte
	line         int
	column       int
}

// NewLexer creates a new Lexer for the given input string.
func NewLexer(input string) *Lexer {
	l := &Lexer{input: input, line: 1, column: 0}
	l.readChar()
	return l
}

// Input returns the original input string.
func (l *Lexer) Input() string { return l.input }

// LexerState captures the internal state of a Lexer so it can be
// saved and restored (e.g. for parser back-tracking).
type LexerState struct {
	Position     int
	ReadPosition int
	Ch           byte
	Line         int
	Column       int
}

// SaveState returns a snapshot of the lexer's current position.
func (l *Lexer) SaveState() LexerState {
	return LexerState{l.position, l.readPosition, l.ch, l.line, l.column}
}

// RestoreState rewinds the lexer to a previously saved state.
func (l *Lexer) RestoreState(s LexerState) {
	l.position = s.Position
	l.readPosition = s.ReadPosition
	l.ch = s.Ch
	l.line = s.Line
	l.column = s.Column
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

func (l *Lexer) readNumber() (token.TokenType, string) {
	position := l.position
	// Hex literal: 0x or 0X
	if l.ch == '0' && (l.peekChar() == 'x' || l.peekChar() == 'X') {
		l.readChar() // consume '0'
		l.readChar() // consume 'x'
		for isHexDigit(l.ch) || l.ch == '_' {
			l.readChar()
		}
		return token.INT, l.input[position:l.position]
	}
	// Binary literal: 0b or 0B
	if l.ch == '0' && (l.peekChar() == 'b' || l.peekChar() == 'B') {
		l.readChar() // consume '0'
		l.readChar() // consume 'b'
		for l.ch == '0' || l.ch == '1' || l.ch == '_' {
			l.readChar()
		}
		return token.INT, l.input[position:l.position]
	}
	// Octal literal: 0o or 0O
	if l.ch == '0' && (l.peekChar() == 'o' || l.peekChar() == 'O') {
		l.readChar() // consume '0'
		l.readChar() // consume 'o'
		for (l.ch >= '0' && l.ch <= '7') || l.ch == '_' {
			l.readChar()
		}
		return token.INT, l.input[position:l.position]
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
		return token.FLOAT, l.input[position:l.position]
	}
	return token.INT, l.input[position:l.position]
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

// NextToken reads and returns the next token from the input.
func (l *Lexer) NextToken() token.Token {
	var tok token.Token

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
			tok = token.Token{Type: token.EQ, Literal: "=="}
		} else if l.peekChar() == '>' {
			l.readChar()
			tok = token.Token{Type: token.ARROW, Literal: "=>"}
		} else {
			tok = token.Token{Type: token.ASSIGN, Literal: "="}
		}
	case '+':
		if l.peekChar() == '+' {
			l.readChar()
			tok = token.Token{Type: token.INCREMENT, Literal: "++"}
		} else if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.PLUS_ASSIGN, Literal: "+="}
		} else {
			tok = token.Token{Type: token.PLUS, Literal: "+"}
		}
	case '-':
		if l.peekChar() == '-' {
			l.readChar()
			tok = token.Token{Type: token.DECREMENT, Literal: "--"}
		} else if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.MINUS_ASSIGN, Literal: "-="}
		} else {
			tok = token.Token{Type: token.MINUS, Literal: "-"}
		}
	case '*':
		if l.peekChar() == '*' {
			l.readChar()
			if l.peekChar() == '=' {
				l.readChar()
				tok = token.Token{Type: token.POWER_ASSIGN, Literal: "**="}
			} else {
				tok = token.Token{Type: token.POWER, Literal: "**"}
			}
		} else if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.MULTIPLY_ASSIGN, Literal: "*="}
		} else {
			tok = token.Token{Type: token.MULTIPLY, Literal: "*"}
		}
	case '/':
		if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.DIVIDE_ASSIGN, Literal: "/="}
		} else {
			tok = token.Token{Type: token.DIVIDE, Literal: "/"}
		}
	case '%':
		if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.MODULO_ASSIGN, Literal: "%="}
		} else {
			tok = token.Token{Type: token.MODULO, Literal: "%"}
		}
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.NEQ, Literal: "!="}
		} else {
			tok = token.Token{Type: token.NOT, Literal: "!"}
		}
	case '<':
		if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.LTE, Literal: "<="}
		} else if l.peekChar() == '<' {
			l.readChar()
			if l.peekChar() == '=' {
				l.readChar()
				tok = token.Token{Type: token.LSHIFT_ASSIGN, Literal: "<<="}
			} else {
				tok = token.Token{Type: token.LSHIFT, Literal: "<<"}
			}
		} else {
			tok = token.Token{Type: token.LT, Literal: "<"}
		}
	case '>':
		if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.GTE, Literal: ">="}
		} else if l.peekChar() == '>' {
			l.readChar()
			if l.peekChar() == '=' {
				l.readChar()
				tok = token.Token{Type: token.RSHIFT_ASSIGN, Literal: ">>="}
			} else {
				tok = token.Token{Type: token.RSHIFT, Literal: ">>"}
			}
		} else {
			tok = token.Token{Type: token.GT, Literal: ">"}
		}
	case '&':
		if l.peekChar() == '&' {
			l.readChar()
			if l.peekChar() == '=' {
				l.readChar()
				tok = token.Token{Type: token.AND_ASSIGN, Literal: "&&="}
			} else {
				tok = token.Token{Type: token.AND, Literal: "&&"}
			}
		} else if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.BITAND_ASSIGN, Literal: "&="}
		} else {
			tok = token.Token{Type: token.BITAND, Literal: "&"}
		}
	case '|':
		if l.peekChar() == '|' {
			l.readChar()
			if l.peekChar() == '=' {
				l.readChar()
				tok = token.Token{Type: token.OR_ASSIGN, Literal: "||="}
			} else {
				tok = token.Token{Type: token.OR, Literal: "||"}
			}
		} else if l.peekChar() == '>' {
			l.readChar()
			tok = token.Token{Type: token.PIPELINE, Literal: "|>"}
		} else if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.BITOR_ASSIGN, Literal: "|="}
		} else {
			tok = token.Token{Type: token.BITOR, Literal: "|"}
		}
	case '^':
		if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.BITXOR_ASSIGN, Literal: "^="}
		} else {
			tok = token.Token{Type: token.BITXOR, Literal: "^"}
		}
	case '~':
		tok = token.Token{Type: token.BITNOT, Literal: "~"}
	case '(':
		tok = token.Token{Type: token.LPAREN, Literal: "("}
	case ')':
		tok = token.Token{Type: token.RPAREN, Literal: ")"}
	case '{':
		tok = token.Token{Type: token.LBRACE, Literal: "{"}
	case '}':
		tok = token.Token{Type: token.RBRACE, Literal: "}"}
	case '[':
		tok = token.Token{Type: token.LBRACKET, Literal: "["}
	case ']':
		tok = token.Token{Type: token.RBRACKET, Literal: "]"}
	case ',':
		tok = token.Token{Type: token.COMMA, Literal: ","}
	case ';':
		tok = token.Token{Type: token.SEMICOLON, Literal: ";"}
	case ':':
		tok = token.Token{Type: token.COLON, Literal: ":"}
	case '.':
		if l.peekChar() == '.' {
			l.readChar()
			if l.peekChar() == '.' {
				l.readChar()
				tok = token.Token{Type: token.SPREAD, Literal: "..."}
			} else {
				tok = token.Token{Type: token.RANGE, Literal: ".."}
			}
		} else {
			tok = token.Token{Type: token.DOT, Literal: "."}
		}
	case '?':
		if l.peekChar() == '?' {
			l.readChar()
			if l.peekChar() == '=' {
				l.readChar()
				tok = token.Token{Type: token.NULLISH_ASSIGN, Literal: "??="}
			} else {
				tok = token.Token{Type: token.NULLISH, Literal: "??"}
			}
		} else if l.peekChar() == '.' {
			l.readChar()
			tok = token.Token{Type: token.OPTIONAL_DOT, Literal: "?."}
		} else {
			tok = token.Token{Type: token.QUESTION, Literal: "?"}
		}
	case '`':
		tok.Type = token.STRING
		tok.Literal = l.readTemplateLiteral()
	case '"', '\'':
		tok.Type = token.STRING
		tok.Literal = l.readString(l.ch)
	case 0:
		tok.Literal = ""
		tok.Type = token.EOF
	default:
		if isLetter(l.ch) {
			tok.Literal = l.readIdentifier()
			tok.Type = LookupIdent(tok.Literal)
			return tok
		} else if isDigit(l.ch) {
			tok.Type, tok.Literal = l.readNumber()
			return tok
		} else {
			tok = token.Token{Type: token.ILLEGAL, Literal: tokenLiteralByte(l.ch)}
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

// KeywordTokens maps keyword strings to their token types.
var KeywordTokens = map[string]token.TokenType{
	"let":      token.LET,
	"if":       token.IF,
	"else":     token.ELSE,
	"while":    token.WHILE,
	"for":      token.FOR,
	"break":    token.BREAK,
	"continue": token.CONTINUE,
	"function": token.FUNCTION,
	"return":   token.RETURN,
	"const":    token.CONST,
	"import":   token.IMPORT,
	"export":   token.EXPORT,
	"try":      token.TRY,
	"catch":    token.CATCH,
	"throw":    token.THROW,
	"switch":   token.SWITCH,
	"case":     token.CASE,
	"default":  token.DEFAULT,
	"in":       token.IN,
	"do":       token.DO,
	"print":    token.PRINT,
	"true":     token.TRUE,
	"false":    token.FALSE,
	"null":     token.NULL,
	"typeof":   token.TYPEOF,
	"match":    token.MATCH,
	"async":    token.ASYNC,
	"await":    token.AWAIT,
	"init":     token.INIT,
	"new":      token.NEW,
	"type":     token.IDENT,
	"lazy":     token.IDENT,
}

// LookupIdent checks if the given identifier is a keyword and returns
// the corresponding token type. Returns token.IDENT for non-keywords.
func LookupIdent(ident string) token.TokenType {
	if tok, ok := KeywordTokens[ident]; ok {
		return tok
	}
	return token.IDENT
}
