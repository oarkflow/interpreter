package token

import "fmt"

// TokenType represents the type of a token.
type TokenType int

const (
	// Literals
	INT TokenType = iota
	FLOAT
	STRING
	IDENT
	TRUE
	FALSE
	NULL

	// Keywords
	LET
	IF
	ELSE
	WHILE
	FOR
	BREAK
	CONTINUE
	FUNCTION
	RETURN
	PRINT
	CONST
	IMPORT
	EXPORT
	TRY
	CATCH
	THROW
	SWITCH
	CASE
	DEFAULT
	IN
	DO
	TYPEOF
	MATCH
	ASYNC
	AWAIT
	INIT
	NEW

	// Operators
	ASSIGN
	PLUS
	MINUS
	MULTIPLY
	DIVIDE
	MODULO
	EQ
	NEQ
	LT
	GT
	LTE
	GTE
	AND
	OR
	NOT
	INCREMENT       // ++
	DECREMENT       // --
	PLUS_ASSIGN     // +=
	MINUS_ASSIGN    // -=
	MULTIPLY_ASSIGN // *=
	DIVIDE_ASSIGN   // /=
	MODULO_ASSIGN   // %=
	NULLISH         // ??
	NULLISH_ASSIGN  // ??=
	BITAND_ASSIGN   // &=
	BITOR_ASSIGN    // |=
	BITXOR_ASSIGN   // ^=
	LSHIFT_ASSIGN   // <<=
	RSHIFT_ASSIGN   // >>=
	POWER_ASSIGN    // **=
	AND_ASSIGN      // &&=
	OR_ASSIGN       // ||=
	PIPELINE        // |>

	// Delimiters
	LPAREN
	RPAREN
	LBRACE
	RBRACE
	LBRACKET
	RBRACKET
	COMMA
	SEMICOLON
	COLON
	DOT
	OPTIONAL_DOT // ?.
	SPREAD       // ...
	RANGE        // ..
	ARROW
	QUESTION

	BITAND // &
	BITOR  // |
	BITXOR // ^
	BITNOT // ~
	LSHIFT // <<
	RSHIFT // >>
	POWER  // **

	EOF
	ILLEGAL
)

// Token represents a lexical token with its type, literal value, and position.
type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

// TypeName returns a human-readable name for a token type.
func TypeName(t TokenType) string {
	switch t {
	case INT:
		return "integer"
	case FLOAT:
		return "float"
	case STRING:
		return "string"
	case IDENT:
		return "identifier"
	case TRUE:
		return "true"
	case FALSE:
		return "false"
	case NULL:
		return "null"
	case LET:
		return "let"
	case IF:
		return "if"
	case ELSE:
		return "else"
	case WHILE:
		return "while"
	case FOR:
		return "for"
	case BREAK:
		return "break"
	case CONTINUE:
		return "continue"
	case FUNCTION:
		return "function"
	case RETURN:
		return "return"
	case PRINT:
		return "print"
	case CONST:
		return "const"
	case IMPORT:
		return "import"
	case EXPORT:
		return "export"
	case TRY:
		return "try"
	case CATCH:
		return "catch"
	case THROW:
		return "throw"
	case SWITCH:
		return "switch"
	case CASE:
		return "case"
	case DEFAULT:
		return "default"
	case IN:
		return "in"
	case DO:
		return "do"
	case ASSIGN:
		return "="
	case PLUS:
		return "+"
	case MINUS:
		return "-"
	case MULTIPLY:
		return "*"
	case DIVIDE:
		return "/"
	case MODULO:
		return "%"
	case EQ:
		return "=="
	case NEQ:
		return "!="
	case LT:
		return "<"
	case GT:
		return ">"
	case LTE:
		return "<="
	case GTE:
		return ">="
	case AND:
		return "&&"
	case OR:
		return "||"
	case NOT:
		return "!"
	case INCREMENT:
		return "++"
	case DECREMENT:
		return "--"
	case PLUS_ASSIGN:
		return "+="
	case MINUS_ASSIGN:
		return "-="
	case MULTIPLY_ASSIGN:
		return "*="
	case DIVIDE_ASSIGN:
		return "/="
	case MODULO_ASSIGN:
		return "%="
	case NULLISH:
		return "??"
	case NULLISH_ASSIGN:
		return "??="
	case BITAND_ASSIGN:
		return "&="
	case BITOR_ASSIGN:
		return "|="
	case BITXOR_ASSIGN:
		return "^="
	case LSHIFT_ASSIGN:
		return "<<="
	case RSHIFT_ASSIGN:
		return ">>="
	case POWER_ASSIGN:
		return "**="
	case AND_ASSIGN:
		return "&&="
	case OR_ASSIGN:
		return "||="
	case PIPELINE:
		return "|>"
	case TYPEOF:
		return "typeof"
	case MATCH:
		return "match"
	case ASYNC:
		return "async"
	case AWAIT:
		return "await"
	case INIT:
		return "init"
	case NEW:
		return "new"
	case LPAREN:
		return "("
	case RPAREN:
		return ")"
	case LBRACE:
		return "{"
	case RBRACE:
		return "}"
	case LBRACKET:
		return "["
	case RBRACKET:
		return "]"
	case COMMA:
		return ","
	case SEMICOLON:
		return ";"
	case COLON:
		return ":"
	case DOT:
		return "."
	case OPTIONAL_DOT:
		return "?."
	case SPREAD:
		return "..."
	case RANGE:
		return ".."
	case ARROW:
		return "=>"
	case QUESTION:
		return "?"
	case BITAND:
		return "&"
	case BITOR:
		return "|"
	case BITXOR:
		return "^"
	case BITNOT:
		return "~"
	case LSHIFT:
		return "<<"
	case RSHIFT:
		return ">>"
	case POWER:
		return "**"
	case EOF:
		return "end of file"
	case ILLEGAL:
		return "invalid token"
	default:
		return fmt.Sprintf("token(%d)", t)
	}
}

// Debug returns a debug string for a token.
func Debug(tok Token) string {
	name := TypeName(tok.Type)
	if tok.Literal != "" && tok.Type != EOF {
		return fmt.Sprintf("%s (%q)", name, tok.Literal)
	}
	return name
}

// ExpectedHint returns a helpful hint when a specific token type was expected.
func ExpectedHint(t TokenType) string {
	switch t {
	case SEMICOLON:
		return "Hint: a statement may be missing a trailing ';'."
	case RPAREN:
		return "Hint: check for missing ')' in expressions or function calls."
	case RBRACE:
		return "Hint: check for missing '}' to close a block."
	case RBRACKET:
		return "Hint: check for missing ']' in array expressions."
	case IDENT:
		return "Hint: an identifier (variable/function name) is expected here."
	default:
		return ""
	}
}
