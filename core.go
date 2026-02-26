package interpreter

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
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
	TOKEN_ARROW

	TOKEN_EOF
	TOKEN_ILLEGAL
)

type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
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
	dotFound := false
	for isDigit(l.ch) || l.ch == '.' {
		if l.ch == '.' {
			if dotFound {
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

func (l *Lexer) readString() string {
	var out strings.Builder
	for {
		l.readChar()
		if l.ch == '"' || l.ch == 0 {
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
			case '\\':
				out.WriteByte('\\')
			default:
				out.WriteByte('\\')
				out.WriteByte(l.ch)
			}
		} else {
			out.WriteByte(l.ch)
		}
	}
	return out.String()
}

func (l *Lexer) NextToken() Token {
	var tok Token

	for {
		l.skipWhitespace()
		if l.ch == '/' && l.peekChar() == '/' {
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
			ch := l.ch
			l.readChar()
			tok = Token{Type: TOKEN_EQ, Literal: string(ch) + string(l.ch)}
		} else if l.peekChar() == '>' {
			ch := l.ch
			l.readChar()
			tok = Token{Type: TOKEN_ARROW, Literal: string(ch) + string(l.ch)}
		} else {
			tok = Token{Type: TOKEN_ASSIGN, Literal: string(l.ch)}
		}
	case '+':
		tok = Token{Type: TOKEN_PLUS, Literal: string(l.ch)}
	case '-':
		tok = Token{Type: TOKEN_MINUS, Literal: string(l.ch)}
	case '*':
		tok = Token{Type: TOKEN_MULTIPLY, Literal: string(l.ch)}
	case '/':
		tok = Token{Type: TOKEN_DIVIDE, Literal: string(l.ch)}
	case '%':
		tok = Token{Type: TOKEN_MODULO, Literal: string(l.ch)}
	case '!':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok = Token{Type: TOKEN_NEQ, Literal: string(ch) + string(l.ch)}
		} else {
			tok = Token{Type: TOKEN_NOT, Literal: string(l.ch)}
		}
	case '<':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok = Token{Type: TOKEN_LTE, Literal: string(ch) + string(l.ch)}
		} else {
			tok = Token{Type: TOKEN_LT, Literal: string(l.ch)}
		}
	case '>':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok = Token{Type: TOKEN_GTE, Literal: string(ch) + string(l.ch)}
		} else {
			tok = Token{Type: TOKEN_GT, Literal: string(l.ch)}
		}
	case '&':
		if l.peekChar() == '&' {
			ch := l.ch
			l.readChar()
			tok = Token{Type: TOKEN_AND, Literal: string(ch) + string(l.ch)}
		} else {
			tok = Token{Type: TOKEN_ILLEGAL, Literal: string(l.ch)}
		}
	case '|':
		if l.peekChar() == '|' {
			ch := l.ch
			l.readChar()
			tok = Token{Type: TOKEN_OR, Literal: string(ch) + string(l.ch)}
		} else {
			tok = Token{Type: TOKEN_ILLEGAL, Literal: string(l.ch)}
		}
	case '(':
		tok = Token{Type: TOKEN_LPAREN, Literal: string(l.ch)}
	case ')':
		tok = Token{Type: TOKEN_RPAREN, Literal: string(l.ch)}
	case '{':
		tok = Token{Type: TOKEN_LBRACE, Literal: string(l.ch)}
	case '}':
		tok = Token{Type: TOKEN_RBRACE, Literal: string(l.ch)}
	case '[':
		tok = Token{Type: TOKEN_LBRACKET, Literal: string(l.ch)}
	case ']':
		tok = Token{Type: TOKEN_RBRACKET, Literal: string(l.ch)}
	case ',':
		tok = Token{Type: TOKEN_COMMA, Literal: string(l.ch)}
	case ';':
		tok = Token{Type: TOKEN_SEMICOLON, Literal: string(l.ch)}
	case ':':
		tok = Token{Type: TOKEN_COLON, Literal: string(l.ch)}
	case '.':
		tok = Token{Type: TOKEN_DOT, Literal: string(l.ch)}
	case '"':
		tok.Type = TOKEN_STRING
		tok.Literal = l.readString()
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
			tok = Token{Type: TOKEN_ILLEGAL, Literal: string(l.ch)}
		}
	}

	l.readChar()
	return tok
}

func isLetter(ch byte) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z'
}

func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}

func lookupIdent(ident string) TokenType {
	keywords := map[string]TokenType{
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
		"print":    TOKEN_PRINT,
		"true":     TOKEN_TRUE,
		"false":    TOKEN_FALSE,
		"null":     TOKEN_NULL,
	}

	if tok, ok := keywords[ident]; ok {
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

type HashLiteral struct {
	Pairs map[Expression]Expression
}

func (hl *HashLiteral) expressionNode() {}
func (hl *HashLiteral) String() string {
	var out strings.Builder
	pairs := []string{}
	for key, value := range hl.Pairs {
		pairs = append(pairs, key.String()+":"+value.String())
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

type PrefixExpression struct {
	Operator string
	Right    Expression
}

func (pe *PrefixExpression) expressionNode() {}
func (pe *PrefixExpression) String() string {
	return fmt.Sprintf("(%s%s)", pe.Operator, pe.Right.String())
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
	Body       *BlockStatement
}

func (fl *FunctionLiteral) expressionNode() {}
func (fl *FunctionLiteral) String() string {
	var out strings.Builder
	out.WriteString("function(")
	for i, p := range fl.Parameters {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(p.String())
	}
	out.WriteString(") ")
	out.WriteString(fl.Body.String())
	return out.String()
}

type CallExpression struct {
	Function  Expression
	Arguments []Expression
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
	Name  *Identifier
	Value Expression
}

func (ae *AssignExpression) expressionNode() {}
func (ae *AssignExpression) String() string {
	return fmt.Sprintf("%s = %s", ae.Name.String(), ae.Value.String())
}

type LetStatement struct {
	Name  *Identifier   // Deprecated: use Names[0]
	Names []*Identifier // Support for multi-assignment
	Value Expression
}

func (ls *LetStatement) statementNode() {}
func (ls *LetStatement) String() string {
	return fmt.Sprintf("let %s = %s;", ls.Name.String(), ls.Value.String())
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

type PrintStatement struct {
	Expression Expression
}

func (ps *PrintStatement) statementNode() {}
func (ps *PrintStatement) String() string {
	return fmt.Sprintf("print %s;", ps.Expression.String())
}

// Parser
type Parser struct {
	l      *Lexer
	errors []string

	curToken  Token
	peekToken Token
}

func NewParser(l *Lexer) *Parser {
	p := &Parser{l: l, errors: []string{}}
	p.nextToken()
	p.nextToken()
	return p
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
}

func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) peekError(t TokenType) {
	msg := fmt.Sprintf("Line %d: expected next token to be %v, got %v instead",
		p.peekToken.Line, t, p.peekToken.Type)
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
	program := &Program{}
	program.Statements = []Statement{}

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
	default:
		return p.parseExpressionStatement()
	}
}

func (p *Parser) parseLetStatement() *LetStatement {
	stmt := &LetStatement{}

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}

	// Always populate Names
	firstIdent := &Identifier{Name: p.curToken.Literal}
	stmt.Names = append(stmt.Names, firstIdent)
	stmt.Name = firstIdent // Keep backward compat for now inside struct, though logic changes

	// Check for tuple assignment: let x, y = ...
	for p.peekTokenIs(TOKEN_COMMA) {
		p.nextToken() // consume comma
		if !p.expectPeek(TOKEN_IDENT) {
			return nil
		}
		stmt.Names = append(stmt.Names, &Identifier{Name: p.curToken.Literal})
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

func (p *Parser) parseConstStatement() *LetStatement {
	stmt := &LetStatement{} // Reuse LetStatement for now

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}

	ident := &Identifier{Name: p.curToken.Literal}
	stmt.Name = ident
	stmt.Names = []*Identifier{ident}

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

func (p *Parser) parseForStatement() *ForStatement {
	stmt := &ForStatement{}

	if !p.expectPeek(TOKEN_LPAREN) {
		return nil
	}

	p.nextToken() // Consume (

	// Parse Init
	if !p.curTokenIs(TOKEN_SEMICOLON) {
		stmt.Init = p.parseStatement()
		// If Init was parsed (Let/ExprStatement), it typically consumes the semicolon.
		// So curToken is now ';'.
		// We need to move past it to get to Condition.
		if p.curTokenIs(TOKEN_SEMICOLON) {
			p.nextToken()
		}
	} else {
		// Empty init
		p.nextToken() // Consume ;
	}

	// Parse Condition
	if !p.curTokenIs(TOKEN_SEMICOLON) {
		stmt.Condition = p.parseExpression(LOWEST)
	}
	if !p.expectPeek(TOKEN_SEMICOLON) {
		return nil
	}

	// Parse Post - allow parsing but don't require it
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
	LOGICAL_OR
	LOGICAL_AND
	EQUALS
	LESSGREATER
	SUM
	PRODUCT
	PREFIX
	CALL
	INDEX
)

var precedences = map[TokenType]int{
	TOKEN_ASSIGN:   ASSIGN,
	TOKEN_OR:       LOGICAL_OR,
	TOKEN_AND:      LOGICAL_AND,
	TOKEN_EQ:       EQUALS,
	TOKEN_NEQ:      EQUALS,
	TOKEN_LT:       LESSGREATER,
	TOKEN_GT:       LESSGREATER,
	TOKEN_LTE:      LESSGREATER,
	TOKEN_GTE:      LESSGREATER,
	TOKEN_PLUS:     SUM,
	TOKEN_MINUS:    SUM,
	TOKEN_MULTIPLY: PRODUCT,
	TOKEN_DIVIDE:   PRODUCT,
	TOKEN_MODULO:   PRODUCT,
	TOKEN_LPAREN:   CALL,
	TOKEN_LBRACKET: INDEX,
	TOKEN_DOT:      INDEX,
	TOKEN_ARROW:    LOWEST,
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
	prefix := p.prefixParseFn()
	if prefix == nil {
		p.noPrefixParseFnError(p.curToken.Type)
		return nil
	}
	leftExp := prefix()

	for !p.peekTokenIs(TOKEN_SEMICOLON) && precedence < p.peekPrecedence() {
		infix := p.infixParseFn()
		if infix == nil {
			return leftExp
		}

		p.nextToken()

		leftExp = infix(leftExp)
	}

	return leftExp
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
	case TOKEN_MINUS, TOKEN_NOT:
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
	}
	return nil
}

func (p *Parser) infixParseFn() func(Expression) Expression {
	switch p.peekToken.Type {
	case TOKEN_PLUS, TOKEN_MINUS, TOKEN_MULTIPLY, TOKEN_DIVIDE, TOKEN_MODULO,
		TOKEN_EQ, TOKEN_NEQ, TOKEN_LT, TOKEN_GT, TOKEN_LTE, TOKEN_GTE,
		TOKEN_AND, TOKEN_OR:
		return p.parseInfixExpression
	case TOKEN_LPAREN:
		return p.parseCallExpression
	case TOKEN_LBRACKET:
		return p.parseIndexExpression
	case TOKEN_DOT:
		return p.parseDotExpression
	case TOKEN_ASSIGN:
		return p.parseAssignExpression
	}
	return nil
}

func (p *Parser) parseAssignExpression(left Expression) Expression {
	ident, ok := left.(*Identifier)
	if !ok {
		p.errors = append(p.errors, fmt.Sprintf("Line %d: assignment to non-identifier", p.curToken.Line))
		return nil
	}

	exp := &AssignExpression{Name: ident}

	p.nextToken()
	exp.Value = p.parseExpression(LOWEST)

	return exp
}

func (p *Parser) noPrefixParseFnError(t TokenType) {
	msg := fmt.Sprintf("Line %d: no prefix parse function for %v found", p.curToken.Line, t)
	p.errors = append(p.errors, msg)
}

func (p *Parser) parseIdentifier() Expression {
	return &Identifier{Name: p.curToken.Literal}
}

func (p *Parser) parseIntegerLiteral() Expression {
	lit := &IntegerLiteral{}

	value, err := strconv.ParseInt(p.curToken.Literal, 0, 64)
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

	value, err := strconv.ParseFloat(p.curToken.Literal, 64)
	if err != nil {
		msg := fmt.Sprintf("Line %d: could not parse %q as float", p.curToken.Line, p.curToken.Literal)
		p.errors = append(p.errors, msg)
		return nil
	}

	lit.Value = value
	return lit
}

func (p *Parser) parseStringLiteral() Expression {
	return &StringLiteral{Value: p.curToken.Literal}
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
	list := []Expression{}

	if p.peekTokenIs(end) {
		p.nextToken()
		return list
	}

	p.nextToken()
	list = append(list, p.parseExpression(LOWEST))

	for p.peekTokenIs(TOKEN_COMMA) {
		p.nextToken()
		p.nextToken()
		list = append(list, p.parseExpression(LOWEST))
	}

	if !p.expectPeek(end) {
		return nil
	}

	return list
}

func (p *Parser) parseHashLiteral() Expression {
	hash := &HashLiteral{Pairs: make(map[Expression]Expression)}

	for !p.peekTokenIs(TOKEN_RBRACE) {
		p.nextToken()
		key := p.parseExpression(LOWEST)

		if !p.expectPeek(TOKEN_COLON) {
			return nil
		}

		p.nextToken()
		value := p.parseExpression(LOWEST)

		hash.Pairs[key] = value

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

func (p *Parser) parseGroupedExpression() Expression {
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

	lit.Parameters = p.parseFunctionParameters()

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}

	lit.Body = p.parseBlockStatement()

	return lit
}

func (p *Parser) parseFunctionParameters() []*Identifier {
	identifiers := []*Identifier{}

	if p.peekTokenIs(TOKEN_RPAREN) {
		p.nextToken()
		return identifiers
	}

	p.nextToken()

	ident := &Identifier{Name: p.curToken.Literal}
	identifiers = append(identifiers, ident)

	for p.peekTokenIs(TOKEN_COMMA) {
		p.nextToken()
		p.nextToken()
		ident := &Identifier{Name: p.curToken.Literal}
		identifiers = append(identifiers, ident)
	}

	if !p.expectPeek(TOKEN_RPAREN) {
		return nil
	}

	return identifiers
}

func (p *Parser) parseDotExpression(left Expression) Expression {
	exp := &DotExpression{Left: left}

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}

	exp.Right = &Identifier{Name: p.curToken.Literal}

	return exp
}

func (p *Parser) parseCallExpression(function Expression) Expression {
	exp := &CallExpression{Function: function}
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

type Null struct{}

func (n *Null) Type() ObjectType { return NULL_OBJ }
func (n *Null) Inspect() string  { return "null" }

type Error struct {
	Message string
}

func (e *Error) Type() ObjectType { return ERROR_OBJ }
func (e *Error) Inspect() string  { return "ERROR: " + e.Message }

func newError(format string, args ...interface{}) Object {
	return &Error{Message: fmt.Sprintf(format, args...)}
}

func objectErrorString(obj Object) string {
	if obj == nil {
		return ""
	}
	if errObj, ok := obj.(*Error); ok {
		return errObj.Message
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
	Body       *BlockStatement
	Env        *Environment
}

func (f *Function) Type() ObjectType { return FUNCTION_OBJ }
func (f *Function) Inspect() string {
	var out strings.Builder
	out.WriteString("function(")
	for i, p := range f.Parameters {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(p.String())
	}
	out.WriteString(") {\n")
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
	pairs := []string{}
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

// Environment
type Environment struct {
	store map[string]Object
	outer *Environment
}

func NewEnvironment() *Environment {
	s := make(map[string]Object)
	return &Environment{store: s, outer: nil}
}

func NewGlobalEnvironment(args []string) *Environment {
	env := NewEnvironment()
	argsArray := &Array{Elements: []Object{}}
	for _, arg := range args {
		argsArray.Elements = append(argsArray.Elements, &String{Value: arg})
	}
	env.Set("ARGS", argsArray)
	return env
}

func NewEnclosedEnvironment(outer *Environment) *Environment {
	env := NewEnvironment()
	env.outer = outer
	return env
}

func (e *Environment) Get(name string) (Object, bool) {
	obj, ok := e.store[name]
	if !ok && e.outer != nil {
		obj, ok = e.outer.Get(name)
	}
	return obj, ok
}

func (e *Environment) Set(name string, val Object) Object {
	e.store[name] = val
	return val
}

func (e *Environment) Assign(name string, val Object) (Object, bool) {
	if _, ok := e.store[name]; ok {
		e.store[name] = val
		return val, true
	}
	if e.outer != nil {
		return e.outer.Assign(name, val)
	}
	return nil, false
}

var (
	TRUE  = &Boolean{Value: true}
	FALSE = &Boolean{Value: false}
	NULL  = &Null{}
	BREAK = &Break{}
	CONT  = &Continue{}
)

func Eval(node Node, env *Environment) Object {
	switch node := node.(type) {
	case *Program:
		return evalProgram(node, env)

	case *ExpressionStatement:
		return Eval(node.Expression, env)

	case *IntegerLiteral:
		return &Integer{Value: node.Value}

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
		if _, ok := env.Assign(node.Name.Name, val); ok {
			return val
		}
		return newError("variable %s not declared", node.Name.Name)

	case *InfixExpression:
		left := Eval(node.Left, env)
		if isError(left) {
			return left
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

	case *ForStatement:
		return evalForStatement(node, env)

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
				env.Set(targetName.Name, val)
			}
		}

	case *PrintStatement:
		val := Eval(node.Expression, env)
		if isError(val) {
			return val
		}
		fmt.Println(val.Inspect())
		return NULL

	case *Identifier:
		return evalIdentifier(node, env)

	case *FunctionLiteral:
		params := node.Parameters
		body := node.Body
		return &Function{Parameters: params, Env: env, Body: body}

	case *CallExpression:
		function := Eval(node.Function, env)
		if isError(function) {
			return function
		}

		args := evalExpressions(node.Arguments, env)
		if len(args) == 1 && isError(args[0]) {
			return args[0]
		}

		return applyFunction(function, args)
	}

	return nil
}

func evalProgram(program *Program, env *Environment) Object {
	var result Object

	for _, statement := range program.Statements {
		result = Eval(statement, env)

		switch result := result.(type) {
		case *ReturnValue:
			return result.Value
		case *Error:
			return result
		case *String:
			if strings.HasPrefix(result.Value, "ERROR:") {
				return result
			}
		}
	}

	return result
}

func evalBlockStatement(block *BlockStatement, env *Environment) Object {
	var result Object

	for _, statement := range block.Statements {
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

	return result
}

func nativeBoolToBooleanObject(input bool) *Boolean {
	if input {
		return TRUE
	}
	return FALSE
}

func evalPrefixExpression(operator string, right Object) Object {
	switch operator {
	case "!":
		return evalBangOperatorExpression(right)
	case "-":
		return evalMinusPrefixOperatorExpression(right)
	default:
		return newError("unknown operator: %s%s", operator, right.Type())
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
	case operator == "&&":
		return nativeBoolToBooleanObject(isTruthy(left) && isTruthy(right))
	case operator == "||":
		return nativeBoolToBooleanObject(isTruthy(left) || isTruthy(right))
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
	default:
		return newError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func evalIntegerInfixExpression(operator string, left, right Object) Object {
	leftVal := left.(*Integer).Value
	rightVal := right.(*Integer).Value

	switch operator {
	case "+":
		return &Integer{Value: leftVal + rightVal}
	case "-":
		return &Integer{Value: leftVal - rightVal}
	case "*":
		return &Integer{Value: leftVal * rightVal}
	case "/":
		if rightVal == 0 {
			return newError("division by zero")
		}
		return &Integer{Value: leftVal / rightVal}
	case "%":
		return &Integer{Value: leftVal % rightVal}
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

// Global cancellation channel
var CancelCh = make(chan struct{})

func evalWhileStatement(ws *WhileStatement, env *Environment) Object {
	var result Object = NULL

	for {
		select {
		case <-CancelCh:
			return newError("execution cancelled")
		default:
		}

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
		select {
		case <-CancelCh:
			return newError("execution cancelled")
		default:
		}

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

func evalIndexExpression(left, index Object) Object {
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

	for keyNode, valueNode := range node.Pairs {
		key := Eval(keyNode, env)
		if isError(key) {
			return key
		}

		hashKey, ok := key.(Hashable)
		if !ok {
			return newError("unusable as hash key: %s", key.Type())
		}

		value := Eval(valueNode, env)
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

func isError(obj Object) bool {
	if obj != nil {
		if obj.Type() == ERROR_OBJ {
			return true
		}
		if obj.Type() == STRING_OBJ {
			return strings.HasPrefix(obj.(*String).Value, "ERROR:")
		}
	}
	return false
}

func evalIdentifier(node *Identifier, env *Environment) Object {
	if val, ok := env.Get(node.Name); ok {
		return val
	}

	if builtin, ok := builtins[node.Name]; ok {
		return builtin
	}

	return newError("identifier not found: %s", node.Name)
}

func evalExpressions(exps []Expression, env *Environment) []Object {
	var result []Object

	for _, e := range exps {
		evaluated := Eval(e, env)
		if isError(evaluated) {
			return []Object{evaluated}
		}
		result = append(result, evaluated)
	}

	return result
}

func applyFunction(fn Object, args []Object) Object {
	if fn == nil {
		return newError("attempting to call nil function")
	}

	switch fn := fn.(type) {
	case *Function:
		if len(args) != len(fn.Parameters) {
			return newError("wrong number of arguments. got=%d, want=%d", len(args), len(fn.Parameters))
		}
		extendedEnv := extendFunctionEnv(fn, args)
		evaluated := Eval(fn.Body, extendedEnv)
		return unwrapReturnValue(evaluated)

	case *Builtin:
		return fn.Fn(args...)

	default:
		return newError("not a function: %s", fn.Type())
	}
}

func extendFunctionEnv(fn *Function, args []Object) *Environment {
	env := NewEnclosedEnvironment(fn.Env)

	for paramIdx, param := range fn.Parameters {
		env.Set(param.Name, args[paramIdx])
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
	flag.Parse()

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
		close(CancelCh)
		os.Exit(130)
	}()

	if *timeout > 0 {
		time.AfterFunc(*timeout, func() {
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

	env := NewGlobalEnvironment(args)
	l := NewLexer(string(content))
	p := NewParser(l)
	program := p.ParseProgram()

	if len(p.Errors()) != 0 {
		for _, msg := range p.Errors() {
			fmt.Println(paint(msg, colorRed))
		}
		os.Exit(1)
	}

	evaluated := Eval(program, env)
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
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
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
	cwdClean := filepath.Clean(cwd)
	if !strings.HasSuffix(cwdClean, string(os.PathSeparator)) {
		cwdClean += string(os.PathSeparator)
	}

	checkPath := cleanPath
	if !strings.HasSuffix(checkPath, string(os.PathSeparator)) {
		checkPath += string(os.PathSeparator)
	}

	if !strings.HasPrefix(checkPath, cwdClean) {
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

	env := NewGlobalEnvironment([]string{})
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
	// 1. Property Access for Hash
	if hash, ok := left.(*Hash); ok {
		key := &String{Value: name}
		hashed := key.HashKey()
		if pair, ok := hash.Pairs[hashed]; ok {
			return pair.Value
		}
		// If key not found, could proceed to check for Hash methods like 'keys'
	}

	// 2. Method Access
	switch left.(type) {
	case *Array:
		return getArrayMethod(left.(*Array), name)
	case *String:
		return getStringMethod(left.(*String), name)
	case *Integer:
		return getIntegerMethod(left.(*Integer), name)
	case *Float:
		return getFloatMethod(left.(*Float), name)
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
	case "pad_left":
		return bindMethod(str, name, "pad_left")
	case "pad_right":
		return bindMethod(str, name, "pad_right")
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
