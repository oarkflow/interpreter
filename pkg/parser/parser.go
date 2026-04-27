package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/lexer"
	"github.com/oarkflow/interpreter/pkg/token"
)

// Parser produces an AST from a token stream.
type Parser struct {
	l      *lexer.Lexer
	errors []string

	curToken  token.Token
	peekToken token.Token

	identIntern map[string]*ast.Identifier
	initBlocks  []*ast.InitStatement
}

// ParserState captures the parser's position for back-tracking.
type ParserState struct {
	curToken   token.Token
	peekToken  token.Token
	lexState   lexer.LexerState
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

func normalizePos(line, col int, fallback token.Token) (int, int) {
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

func LineContext(input string, line, col int) string {
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

// NewParser creates a new Parser that reads tokens from l.
func NewParser(l *lexer.Lexer) *Parser {
	p := &Parser{
		l:           l,
		errors:      []string{},
		identIntern: make(map[string]*ast.Identifier, 128),
		initBlocks:  make([]*ast.InitStatement, 0, 2),
	}
	p.nextToken()
	p.nextToken()
	return p
}

// InitBlocks returns the init blocks collected during parsing.
func (p *Parser) InitBlocks() []*ast.InitStatement {
	return p.initBlocks
}

func (p *Parser) internIdentifier(name string) *ast.Identifier {
	if ident, ok := p.identIntern[name]; ok {
		return ident
	}
	ident := &ast.Identifier{Name: name}
	p.identIntern[name] = ident
	return ident
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
}

// Errors returns the list of parsing errors.
func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) peekError(t token.TokenType) {
	line, col := normalizePos(p.peekToken.Line, p.peekToken.Column, p.curToken)
	msg := fmt.Sprintf(
		"Line %d:%d -> expected %s, got %s.",
		line,
		col,
		token.TypeName(t),
		token.Debug(p.peekToken),
	)
	if hint := token.ExpectedHint(t); hint != "" {
		msg += " " + hint
	}
	if ctx := LineContext(p.l.Input(), line, col); ctx != "" {
		msg += "\n" + ctx
	}
	p.errors = append(p.errors, msg)
}

func (p *Parser) curTokenIs(t token.TokenType) bool {
	return p.curToken.Type == t
}

func (p *Parser) peekTokenIs(t token.TokenType) bool {
	return p.peekToken.Type == t
}

func (p *Parser) expectPeek(t token.TokenType) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	}
	p.peekError(t)
	return false
}

// ParseProgram parses the full input and returns the root AST node.
func (p *Parser) ParseProgram() *ast.Program {
	program := &ast.Program{Statements: make([]ast.Statement, 0, 64)}

	for !p.curTokenIs(token.EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
		}
		p.nextToken()
	}

	return program
}

func (p *Parser) parseStatement() ast.Statement {
	switch p.curToken.Type {
	case token.LET:
		return p.parseLetStatement()
	case token.CONST:
		return p.parseConstStatement()
	case token.RETURN:
		return p.parseReturnStatement()
	case token.WHILE:
		return p.parseWhileStatement()
	case token.FOR:
		return p.parseForStatement()
	case token.BREAK:
		return p.parseBreakStatement()
	case token.CONTINUE:
		return p.parseContinueStatement()
	case token.PRINT:
		return p.parsePrintStatement()
	case token.IMPORT:
		return p.parseImportStatement()
	case token.EXPORT:
		return p.parseExportStatement()
	case token.THROW:
		return p.parseThrowStatement()
	case token.SWITCH:
		return p.parseSwitchStatement()
	case token.MATCH:
		return &ast.ExpressionStatement{Expression: p.parseMatchExpression()}
	case token.DO:
		return p.parseDoWhileStatement()
	case token.FUNCTION:
		// Named function declaration: function foo(...) { ... }
		if p.peekTokenIs(token.IDENT) {
			return p.parseFunctionDeclaration()
		}
		return p.parseExpressionStatement()
	case token.INIT:
		return p.parseInitStatement()
	case token.IDENT:
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
	if !p.curTokenIs(token.IDENT) || p.curToken.Literal != "type" {
		return false
	}
	saved := p.saveState()
	defer p.restoreState(saved)
	if !p.peekTokenIs(token.IDENT) {
		return false
	}
	p.nextToken()
	if !p.peekTokenIs(token.ASSIGN) {
		return false
	}
	return true
}

func (p *Parser) parseLetStatement() ast.Statement {
	// Detect destructuring: let { or let [
	if p.peekTokenIs(token.LBRACE) || p.peekTokenIs(token.LBRACKET) {
		return p.parseDestructureLetStatement(false)
	}

	stmt := &ast.LetStatement{}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	// Always populate Names
	firstIdent := p.internIdentifier(p.curToken.Literal)
	stmt.Names = make([]*ast.Identifier, 1, 2)
	stmt.Names[0] = firstIdent
	stmt.Name = firstIdent

	// Check for tuple assignment: let x, y = ...
	for p.peekTokenIs(token.COMMA) {
		p.nextToken() // consume comma
		if !p.expectPeek(token.IDENT) {
			return nil
		}
		stmt.Names = append(stmt.Names, p.internIdentifier(p.curToken.Literal))
	}

	if len(stmt.Names) == 1 && p.peekTokenIs(token.COLON) {
		p.nextToken()
		typeName := p.parseTypeName()
		if typeName == "" {
			return nil
		}
		stmt.TypeName = typeName
	}

	if !p.expectPeek(token.ASSIGN) {
		return nil
	}

	p.nextToken()

	stmt.Value = p.parseExpression(LOWEST)

	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseConstStatement() ast.Statement {
	// Detect destructuring: const { or const [
	if p.peekTokenIs(token.LBRACE) || p.peekTokenIs(token.LBRACKET) {
		return p.parseDestructureLetStatement(true)
	}

	stmt := &ast.LetStatement{}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	ident := p.internIdentifier(p.curToken.Literal)
	stmt.Name = ident
	stmt.Names = []*ast.Identifier{ident}
	if p.peekTokenIs(token.COLON) {
		p.nextToken()
		typeName := p.parseTypeName()
		if typeName == "" {
			return nil
		}
		stmt.TypeName = typeName
	}

	if !p.expectPeek(token.ASSIGN) {
		return nil
	}

	p.nextToken()

	stmt.Value = p.parseExpression(LOWEST)

	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseDestructureLetStatement(isConst bool) *ast.DestructureLetStatement {
	stmt := &ast.DestructureLetStatement{IsConst: isConst}
	p.nextToken() // advance to { or [
	if p.curTokenIs(token.LBRACE) {
		stmt.Pattern = p.parseObjectDestructurePattern()
	} else {
		stmt.Pattern = p.parseArrayDestructurePattern()
	}
	if stmt.Pattern == nil {
		return nil
	}
	if !p.expectPeek(token.ASSIGN) {
		return nil
	}
	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)
	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseObjectDestructurePattern() *ast.DestructurePattern {
	pat := &ast.DestructurePattern{Kind: "object"}
	for !p.peekTokenIs(token.RBRACE) {
		p.nextToken()
		if p.curTokenIs(token.SPREAD) {
			p.nextToken() // move to ident
			pat.RestName = p.internIdentifier(p.curToken.Literal)
			break
		}
		keyName := p.curToken.Literal
		key := &ast.StringLiteral{Value: keyName}
		var bindName *ast.Identifier
		if p.peekTokenIs(token.COLON) {
			p.nextToken() // consume :
			p.nextToken() // move to renamed ident
			bindName = p.internIdentifier(p.curToken.Literal)
		} else {
			bindName = p.internIdentifier(keyName)
		}
		var def ast.Expression
		if p.peekTokenIs(token.ASSIGN) {
			p.nextToken() // consume =
			p.nextToken()
			def = p.parseExpression(LOWEST)
		}
		pat.Keys = append(pat.Keys, key)
		pat.Names = append(pat.Names, bindName)
		pat.Defaults = append(pat.Defaults, def)
		if !p.peekTokenIs(token.RBRACE) && !p.expectPeek(token.COMMA) {
			return nil
		}
	}
	if !p.expectPeek(token.RBRACE) {
		return nil
	}
	return pat
}

func (p *Parser) parseArrayDestructurePattern() *ast.DestructurePattern {
	pat := &ast.DestructurePattern{Kind: "array"}
	for !p.peekTokenIs(token.RBRACKET) {
		p.nextToken()
		if p.curTokenIs(token.SPREAD) {
			p.nextToken()
			pat.RestName = p.internIdentifier(p.curToken.Literal)
			break
		}
		ident := p.internIdentifier(p.curToken.Literal)
		var def ast.Expression
		if p.peekTokenIs(token.ASSIGN) {
			p.nextToken()
			p.nextToken()
			def = p.parseExpression(LOWEST)
		}
		pat.Names = append(pat.Names, ident)
		pat.Defaults = append(pat.Defaults, def)
		if !p.peekTokenIs(token.RBRACKET) && !p.expectPeek(token.COMMA) {
			return nil
		}
	}
	if !p.expectPeek(token.RBRACKET) {
		return nil
	}
	return pat
}

func (p *Parser) parseReturnStatement() *ast.ReturnStatement {
	stmt := &ast.ReturnStatement{}

	p.nextToken()

	if p.curTokenIs(token.SEMICOLON) {
		return stmt
	}

	stmt.ReturnValue = p.parseExpression(LOWEST)

	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseWhileStatement() *ast.WhileStatement {
	stmt := &ast.WhileStatement{}

	if !p.expectPeek(token.LPAREN) {
		return nil
	}

	p.nextToken()
	stmt.Condition = p.parseExpression(LOWEST)

	if !p.expectPeek(token.RPAREN) {
		return nil
	}

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()

	return stmt
}

func (p *Parser) parseForStatement() ast.Statement {
	if !p.expectPeek(token.LPAREN) {
		return nil
	}

	p.nextToken() // Consume (

	// Detect for-in: IDENT IN ... or IDENT , IDENT IN ...
	if p.curTokenIs(token.IDENT) {
		if p.peekTokenIs(token.IN) {
			// for (x in expr) { }
			valueName := p.internIdentifier(p.curToken.Literal)
			p.nextToken() // consume 'in'
			p.nextToken() // move to iterable
			iterable := p.parseExpression(LOWEST)
			if !p.expectPeek(token.RPAREN) {
				return nil
			}
			if !p.expectPeek(token.LBRACE) {
				return nil
			}
			return &ast.ForInStatement{ValueName: valueName, Iterable: iterable, Body: p.parseBlockStatement()}
		}
		if p.peekTokenIs(token.COMMA) {
			// Could be for (k, v in expr) - save first ident
			firstIdent := p.internIdentifier(p.curToken.Literal)
			p.nextToken() // consume ','
			if p.peekTokenIs(token.IDENT) {
				p.nextToken() // on second ident
				secondIdent := p.internIdentifier(p.curToken.Literal)
				if p.peekTokenIs(token.IN) {
					p.nextToken() // consume 'in'
					p.nextToken() // on iterable
					iterable := p.parseExpression(LOWEST)
					if !p.expectPeek(token.RPAREN) {
						return nil
					}
					if !p.expectPeek(token.LBRACE) {
						return nil
					}
					return &ast.ForInStatement{KeyName: firstIdent, ValueName: secondIdent, Iterable: iterable, Body: p.parseBlockStatement()}
				}
			}
			// Fall through - not a for-in
			_ = firstIdent
		}
	}

	// Regular C-style for loop. We already consumed the token after (
	stmt := &ast.ForStatement{}
	if !p.curTokenIs(token.SEMICOLON) {
		stmt.Init = p.parseStatement()
		if p.curTokenIs(token.SEMICOLON) {
			p.nextToken()
		}
	} else {
		p.nextToken()
	}

	if !p.curTokenIs(token.SEMICOLON) {
		stmt.Condition = p.parseExpression(LOWEST)
	}

	if !p.expectPeek(token.SEMICOLON) {
		return nil
	}

	if !p.peekTokenIs(token.RPAREN) {
		p.nextToken()
		exp := p.parseExpression(LOWEST)
		stmt.Post = &ast.ExpressionStatement{Expression: exp}
	}

	if !p.expectPeek(token.RPAREN) {
		return nil
	}

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()

	return stmt
}

func (p *Parser) parseBreakStatement() *ast.BreakStatement {
	stmt := &ast.BreakStatement{}
	p.nextToken() // consume break
	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseContinueStatement() *ast.ContinueStatement {
	stmt := &ast.ContinueStatement{}
	p.nextToken() // consume continue
	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parsePrintStatement() *ast.PrintStatement {
	stmt := &ast.PrintStatement{}

	p.nextToken()
	stmt.Expression = p.parseExpression(LOWEST)

	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseImportStatement() *ast.ImportStatement {
	stmt := &ast.ImportStatement{}

	if p.peekTokenIs(token.MULTIPLY) {
		p.nextToken()
		if !p.expectPeek(token.IDENT) || strings.ToLower(p.curToken.Literal) != "as" {
			p.errors = append(p.errors, fmt.Sprintf("Line %d: expected 'as' after import *", p.curToken.Line))
			return nil
		}
		if !p.expectPeek(token.IDENT) {
			return nil
		}
		stmt.Alias = p.internIdentifier(p.curToken.Literal)
		if !p.expectPeek(token.IDENT) || strings.ToLower(p.curToken.Literal) != "from" {
			p.errors = append(p.errors, fmt.Sprintf("Line %d: expected 'from' after import alias", p.curToken.Line))
			return nil
		}
		p.nextToken()
		stmt.Path = p.parseExpression(LOWEST)
		if p.peekTokenIs(token.SEMICOLON) {
			p.nextToken()
		}
		return stmt
	}

	if p.peekTokenIs(token.LBRACE) {
		p.nextToken()
		for {
			if !p.expectPeek(token.IDENT) {
				return nil
			}
			stmt.Names = append(stmt.Names, p.internIdentifier(p.curToken.Literal))
			if p.peekTokenIs(token.COMMA) {
				p.nextToken()
				continue
			}
			if !p.expectPeek(token.RBRACE) {
				return nil
			}
			break
		}

		if !p.expectPeek(token.IDENT) || strings.ToLower(p.curToken.Literal) != "from" {
			p.errors = append(p.errors, fmt.Sprintf("Line %d: expected 'from' after import list", p.curToken.Line))
			return nil
		}

		p.nextToken()
		stmt.Path = p.parseExpression(LOWEST)

		if p.peekTokenIs(token.SEMICOLON) {
			p.nextToken()
		}
		return stmt
	}

	p.nextToken()
	stmt.Path = p.parseExpression(LOWEST)

	if p.peekTokenIs(token.IDENT) {
		peek := strings.ToLower(p.peekToken.Literal)
		if peek == "as" {
			p.nextToken()
			if !p.expectPeek(token.IDENT) {
				return nil
			}
			stmt.Alias = p.internIdentifier(p.curToken.Literal)
		}
	}

	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseExportStatement() *ast.ExportStatement {
	stmt := &ast.ExportStatement{}

	if p.peekTokenIs(token.LET) {
		p.nextToken()
		stmt.Declaration = p.parseLetStatement()
		stmt.IsConst = false
		return stmt
	}

	if p.peekTokenIs(token.CONST) {
		p.nextToken()
		stmt.Declaration = p.parseConstStatement()
		stmt.IsConst = true
		return stmt
	}

	msg := fmt.Sprintf("Line %d: export must be followed by let or const", p.curToken.Line)
	p.errors = append(p.errors, msg)
	return nil
}

func (p *Parser) parseThrowStatement() *ast.ThrowStatement {
	stmt := &ast.ThrowStatement{}

	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)

	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseExpressionStatement() *ast.ExpressionStatement {
	stmt := &ast.ExpressionStatement{}

	stmt.Expression = p.parseExpression(LOWEST)

	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseBlockStatement() *ast.BlockStatement {
	block := &ast.BlockStatement{}
	block.Statements = []ast.Statement{}

	p.nextToken()

	for !p.curTokenIs(token.RBRACE) && !p.curTokenIs(token.EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
		p.nextToken()
	}

	return block
}

// Operator precedence levels.
const (
	_ int = iota
	LOWEST
	ASSIGN_PREC
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

var precedences = map[token.TokenType]int{
	token.ASSIGN:          ASSIGN_PREC,
	token.QUESTION:        ASSIGN_PREC,
	token.PLUS_ASSIGN:     ASSIGN_PREC,
	token.MINUS_ASSIGN:    ASSIGN_PREC,
	token.MULTIPLY_ASSIGN: ASSIGN_PREC,
	token.DIVIDE_ASSIGN:   ASSIGN_PREC,
	token.MODULO_ASSIGN:   ASSIGN_PREC,
	token.NULLISH:         NULLISH_COALESCE,
	token.NULLISH_ASSIGN:  ASSIGN_PREC,
	token.BITAND_ASSIGN:   ASSIGN_PREC,
	token.BITOR_ASSIGN:    ASSIGN_PREC,
	token.BITXOR_ASSIGN:   ASSIGN_PREC,
	token.LSHIFT_ASSIGN:   ASSIGN_PREC,
	token.RSHIFT_ASSIGN:   ASSIGN_PREC,
	token.POWER_ASSIGN:    ASSIGN_PREC,
	token.AND_ASSIGN:      ASSIGN_PREC,
	token.OR_ASSIGN:       ASSIGN_PREC,
	token.PIPELINE:        ASSIGN_PREC,
	token.OR:              LOGICAL_OR,
	token.AND:             LOGICAL_AND,
	token.BITOR:           BIT_OR,
	token.BITXOR:          BIT_XOR,
	token.BITAND:          BIT_AND,
	token.EQ:              EQUALS,
	token.NEQ:             EQUALS,
	token.LT:              LESSGREATER,
	token.GT:              LESSGREATER,
	token.LTE:             LESSGREATER,
	token.GTE:             LESSGREATER,
	token.LSHIFT:          BIT_SHIFT,
	token.RSHIFT:          BIT_SHIFT,
	token.PLUS:            SUM,
	token.MINUS:           SUM,
	token.RANGE:           RANGE_PREC,
	token.MULTIPLY:        PRODUCT,
	token.DIVIDE:          PRODUCT,
	token.MODULO:          PRODUCT,
	token.POWER:           POWER,
	token.LPAREN:          CALL,
	token.LBRACKET:        INDEX,
	token.DOT:             INDEX,
	token.OPTIONAL_DOT:    INDEX,
	token.INCREMENT:       POSTFIX,
	token.DECREMENT:       POSTFIX,
	token.ARROW:           ASSIGN_PREC,
}

func (p *Parser) peekPrecedence() int {
	if prec, ok := precedences[p.peekToken.Type]; ok {
		return prec
	}
	return LOWEST
}

func (p *Parser) curPrecedence() int {
	if prec, ok := precedences[p.curToken.Type]; ok {
		return prec
	}
	return LOWEST
}

func (p *Parser) parseExpression(precedence int) ast.Expression {
	var leftExp ast.Expression
	switch p.curToken.Type {
	case token.IDENT:
		if p.curToken.Literal == "lazy" {
			leftExp = p.parseLazyExpression()
		} else {
			leftExp = p.parseIdentifier()
		}
	case token.INT:
		leftExp = p.parseIntegerLiteral()
	case token.FLOAT:
		leftExp = p.parseFloatLiteral()
	case token.STRING:
		leftExp = p.parseStringLiteral()
	case token.TRUE, token.FALSE:
		leftExp = p.parseBooleanLiteral()
	case token.NULL:
		leftExp = p.parseNullLiteral()
	case token.MINUS, token.NOT, token.BITNOT, token.TYPEOF:
		leftExp = p.parsePrefixExpression()
	case token.LPAREN:
		leftExp = p.parseGroupedExpression()
	case token.IF:
		leftExp = p.parseIfExpression()
	case token.FUNCTION:
		leftExp = p.parseFunctionLiteral()
	case token.LBRACKET:
		leftExp = p.parseArrayLiteral()
	case token.LBRACE:
		leftExp = p.parseHashLiteral()
	case token.TRY:
		leftExp = p.parseTryCatchExpression()
	case token.MATCH:
		leftExp = p.parseMatchExpression()
	case token.ASYNC:
		leftExp = p.parseAsyncExpression()
	case token.AWAIT:
		leftExp = p.parseAwaitExpression()
	case token.NEW:
		leftExp = p.parseNewExpression()
	default:
		p.noPrefixParseFnError(p.curToken.Type)
		return nil
	}

	for !p.peekTokenIs(token.SEMICOLON) && precedence < p.peekPrecedence() {
		switch p.peekToken.Type {
		case token.PLUS, token.MINUS, token.MULTIPLY, token.DIVIDE, token.MODULO,
			token.EQ, token.NEQ, token.LT, token.GT, token.LTE, token.GTE,
			token.AND, token.OR,
			token.BITAND, token.BITOR, token.BITXOR, token.LSHIFT, token.RSHIFT,
			token.NULLISH:
			p.nextToken()
			leftExp = p.parseInfixExpression(leftExp)
		case token.RANGE:
			p.nextToken()
			leftExp = p.parseRangeExpression(leftExp)
		case token.PIPELINE:
			p.nextToken()
			leftExp = p.parsePipelineExpression(leftExp)
		case token.POWER:
			p.nextToken()
			leftExp = p.parsePowerExpression(leftExp)
		case token.LPAREN:
			p.nextToken()
			leftExp = p.parseCallExpression(leftExp)
		case token.LBRACKET:
			p.nextToken()
			leftExp = p.parseIndexExpression(leftExp)
		case token.DOT:
			p.nextToken()
			leftExp = p.parseDotExpression(leftExp)
		case token.OPTIONAL_DOT:
			p.nextToken()
			leftExp = p.parseOptionalDotExpression(leftExp)
		case token.ASSIGN:
			p.nextToken()
			leftExp = p.parseAssignExpression(leftExp)
		case token.QUESTION:
			p.nextToken()
			leftExp = p.parseTernaryExpression(leftExp)
		case token.PLUS_ASSIGN, token.MINUS_ASSIGN, token.MULTIPLY_ASSIGN, token.DIVIDE_ASSIGN, token.MODULO_ASSIGN, token.NULLISH_ASSIGN,
			token.BITAND_ASSIGN, token.BITOR_ASSIGN, token.BITXOR_ASSIGN, token.LSHIFT_ASSIGN, token.RSHIFT_ASSIGN, token.POWER_ASSIGN,
			token.AND_ASSIGN, token.OR_ASSIGN:
			p.nextToken()
			leftExp = p.parseCompoundAssignExpression(leftExp)
		case token.INCREMENT, token.DECREMENT:
			p.nextToken()
			leftExp = p.parsePostfixExpression(leftExp)
		case token.ARROW:
			// Single-param arrow: x => expr
			p.nextToken() // curToken is now =>
			ident, ok := leftExp.(*ast.Identifier)
			if !ok {
				p.errors = append(p.errors, fmt.Sprintf("Line %d: left side of => must be an identifier", p.curToken.Line))
				return nil
			}
			p.nextToken() // advance past =>
			lit := &ast.FunctionLiteral{
				IsArrow:    true,
				Parameters: []*ast.Identifier{ident},
				ParamTypes: []string{""},
				Defaults:   []ast.Expression{nil},
			}
			if p.curTokenIs(token.LBRACE) {
				lit.Body = p.parseBlockStatement()
			} else {
				expr := p.parseExpression(LOWEST)
				lit.Body = &ast.BlockStatement{
					Statements: []ast.Statement{
						&ast.ReturnStatement{ReturnValue: expr},
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

func (p *Parser) parsePostfixExpression(left ast.Expression) ast.Expression {
	switch left.(type) {
	case *ast.Identifier, *ast.DotExpression, *ast.IndexExpression:
		// valid postfix target
	default:
		p.errors = append(p.errors, fmt.Sprintf("Line %d: postfix %s requires an identifier or property", p.curToken.Line, p.curToken.Literal))
		return nil
	}
	return &ast.PostfixExpression{Operator: p.curToken.Literal, Target: left}
}

func (p *Parser) prefixParseFn() func() ast.Expression {
	switch p.curToken.Type {
	case token.IDENT:
		return p.parseIdentifier
	case token.INT:
		return p.parseIntegerLiteral
	case token.FLOAT:
		return p.parseFloatLiteral
	case token.STRING:
		return p.parseStringLiteral
	case token.TRUE, token.FALSE:
		return p.parseBooleanLiteral
	case token.NULL:
		return p.parseNullLiteral
	case token.MINUS, token.NOT, token.BITNOT:
		return p.parsePrefixExpression
	case token.LPAREN:
		return p.parseGroupedExpression
	case token.IF:
		return p.parseIfExpression
	case token.FUNCTION:
		return p.parseFunctionLiteral
	case token.LBRACKET:
		return p.parseArrayLiteral
	case token.LBRACE:
		return p.parseHashLiteral
	case token.TRY:
		return p.parseTryCatchExpression
	case token.MATCH:
		return p.parseMatchExpression
	}
	return nil
}

func (p *Parser) parseTryCatchExpression() ast.Expression {
	expr := &ast.TryCatchExpression{}

	if !p.expectPeek(token.LBRACE) {
		return nil
	}
	expr.TryBlock = p.parseBlockStatement()

	// catch block (optional if finally is present)
	if p.peekTokenIs(token.CATCH) {
		p.nextToken()

		if !p.expectPeek(token.LPAREN) {
			return nil
		}
		if !p.expectPeek(token.IDENT) {
			return nil
		}
		expr.CatchIdent = p.internIdentifier(p.curToken.Literal)
		if p.peekTokenIs(token.COLON) {
			p.nextToken()
			expr.CatchType = p.parseTypeName()
			if expr.CatchType == "" {
				return nil
			}
		}

		if !p.expectPeek(token.RPAREN) {
			return nil
		}
		if !p.expectPeek(token.LBRACE) {
			return nil
		}

		expr.CatchBlock = p.parseBlockStatement()
	}

	// finally block (optional)
	if p.peekTokenIs(token.IDENT) && p.peekToken.Literal == "finally" {
		p.nextToken() // consume "finally"
		if !p.expectPeek(token.LBRACE) {
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

func (p *Parser) parseTernaryExpression(condition ast.Expression) ast.Expression {
	expr := &ast.TernaryExpression{Condition: condition}

	p.nextToken()
	expr.Consequence = p.parseExpression(LOWEST)

	if !p.expectPeek(token.COLON) {
		return nil
	}

	p.nextToken()
	expr.Alternative = p.parseExpression(LOWEST)

	return expr
}

func (p *Parser) parseSwitchStatement() *ast.SwitchStatement {
	stmt := &ast.SwitchStatement{}

	if !p.expectPeek(token.LPAREN) {
		return nil
	}
	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)
	if !p.expectPeek(token.RPAREN) {
		return nil
	}
	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	p.nextToken()

	for !p.curTokenIs(token.RBRACE) && !p.curTokenIs(token.EOF) {
		if p.curTokenIs(token.CASE) {
			sc := &ast.SwitchCase{}
			p.nextToken()
			sc.Values = append(sc.Values, p.parseExpression(LOWEST))
			for p.peekTokenIs(token.COMMA) {
				p.nextToken()
				p.nextToken()
				sc.Values = append(sc.Values, p.parseExpression(LOWEST))
			}
			if !p.expectPeek(token.COLON) {
				return nil
			}
			sc.Body = p.parseSwitchCaseBody()
			stmt.Cases = append(stmt.Cases, sc)
		} else if p.curTokenIs(token.DEFAULT) {
			if !p.expectPeek(token.COLON) {
				return nil
			}
			stmt.Default = p.parseSwitchCaseBody()
		} else {
			p.nextToken()
		}
	}

	return stmt
}

func (p *Parser) parseSwitchCaseBody() *ast.BlockStatement {
	block := &ast.BlockStatement{Statements: []ast.Statement{}}
	p.nextToken()
	for !p.curTokenIs(token.CASE) && !p.curTokenIs(token.DEFAULT) && !p.curTokenIs(token.RBRACE) && !p.curTokenIs(token.EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
		p.nextToken()
	}
	return block
}

// --- Async/Await parser ---

func (p *Parser) parseAsyncExpression() ast.Expression {
	// curToken is token.ASYNC
	p.nextToken() // advance past 'async'

	if p.curTokenIs(token.FUNCTION) {
		lit := p.parseFunctionLiteral()
		if fl, ok := lit.(*ast.FunctionLiteral); ok {
			fl.IsAsync = true
		}
		return lit
	}

	// async (params) => body  — async arrow function
	if p.curTokenIs(token.LPAREN) || p.curTokenIs(token.IDENT) {
		// Try parsing as expression (which may turn into arrow function)
		expr := p.parseExpression(LOWEST)
		if fl, ok := expr.(*ast.FunctionLiteral); ok {
			fl.IsAsync = true
		}
		return expr
	}

	p.errors = append(p.errors, fmt.Sprintf("Line %d: expected function or arrow function after 'async'", p.curToken.Line))
	return nil
}

func (p *Parser) parseAwaitExpression() ast.Expression {
	// curToken is token.AWAIT
	p.nextToken() // advance past 'await'
	value := p.parseExpression(PREFIX)
	return &ast.AwaitExpression{Value: value}
}

func (p *Parser) parseLazyExpression() ast.Expression {
	p.nextToken()
	value := p.parseExpression(PREFIX)
	return &ast.LazyExpression{Value: value}
}

// --- Pattern matching parser ---

func (p *Parser) parseMatchExpression() ast.Expression {
	expr := &ast.MatchExpression{}

	if !p.expectPeek(token.LPAREN) {
		return nil
	}
	p.nextToken()
	expr.Value = p.parseExpression(LOWEST)
	if !p.expectPeek(token.RPAREN) {
		return nil
	}
	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	p.nextToken()
	for !p.curTokenIs(token.RBRACE) && !p.curTokenIs(token.EOF) {
		if !p.curTokenIs(token.CASE) {
			p.errors = append(p.errors, fmt.Sprintf("Line %d: expected 'case' in match expression, got %s", p.curToken.Line, token.TypeName(p.curToken.Type)))
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

func (p *Parser) parseMatchCase() *ast.MatchCase {
	mc := &ast.MatchCase{Line: p.curToken.Line}

	p.nextToken() // skip 'case'
	mc.Pattern = p.parsePattern()
	if mc.Pattern == nil {
		return nil
	}

	// Check for OR patterns: case 1 | 2 | 3
	if p.peekTokenIs(token.BITOR) {
		patterns := []ast.Pattern{mc.Pattern}
		for p.peekTokenIs(token.BITOR) {
			p.nextToken() // skip |
			p.nextToken() // move to next pattern
			pat := p.parseSinglePattern()
			if pat != nil {
				patterns = append(patterns, pat)
			}
		}
		mc.Pattern = &ast.OrPattern{Patterns: patterns}
	}

	// Optional guard: if condition
	if p.peekTokenIs(token.IF) {
		p.nextToken() // skip 'if'
		p.nextToken()
		mc.Guard = p.parseExpression(ASSIGN_PREC)
	}

	// Expect =>
	if !p.expectPeek(token.ARROW) {
		return nil
	}

	// Parse body: either a block { ... } or a single expression
	if p.peekTokenIs(token.LBRACE) {
		p.nextToken()
		mc.Body = p.parseBlockStatement()
	} else {
		p.nextToken()
		stmt := p.parseStatement()
		if stmt != nil {
			mc.Body = &ast.BlockStatement{
				Statements: []ast.Statement{stmt},
			}
		} else {
			mc.Body = &ast.BlockStatement{Statements: []ast.Statement{}}
		}
	}

	// Advance past semicolons/newlines to next case or }
	for p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}
	if p.peekTokenIs(token.CASE) || p.peekTokenIs(token.RBRACE) {
		p.nextToken()
	}

	return mc
}

func (p *Parser) parsePattern() ast.Pattern {
	pat := p.parseSinglePattern()
	if pat == nil {
		return nil
	}
	return pat
}

var extractorNames = map[string]bool{
	"Some": true, "None": true, "Nil": true,
	"All": true, "Any": true, "Tuple": true, "Regex": true,
}

func (p *Parser) parseSinglePattern() ast.Pattern {
	switch p.curToken.Type {
	case token.INT:
		lit := p.parseIntegerLiteral()
		if p.peekTokenIs(token.RANGE) {
			p.nextToken() // skip ..
			p.nextToken()
			high := p.parseExpression(ASSIGN_PREC)
			return &ast.RangePattern{Low: lit, High: high}
		}
		return &ast.LiteralPattern{Value: lit}
	case token.FLOAT:
		lit := p.parseFloatLiteral()
		if p.peekTokenIs(token.RANGE) {
			p.nextToken() // skip ..
			p.nextToken()
			high := p.parseExpression(ASSIGN_PREC)
			return &ast.RangePattern{Low: lit, High: high}
		}
		return &ast.LiteralPattern{Value: lit}
	case token.STRING:
		lit := p.parseStringLiteral()
		if p.peekTokenIs(token.RANGE) {
			p.nextToken() // skip ..
			p.nextToken()
			high := p.parseExpression(ASSIGN_PREC)
			return &ast.RangePattern{Low: lit, High: high}
		}
		return &ast.LiteralPattern{Value: lit}
	case token.TRUE, token.FALSE:
		return &ast.LiteralPattern{Value: p.parseBooleanLiteral()}
	case token.NULL:
		return &ast.LiteralPattern{Value: p.parseNullLiteral()}
	case token.MINUS:
		// Negative number literal: case -1 =>
		return &ast.LiteralPattern{Value: p.parsePrefixExpression()}
	case token.LBRACKET:
		return p.parseArrayPattern()
	case token.LBRACE:
		return p.parseObjectPattern()
	case token.GT, token.GTE, token.LT, token.LTE, token.NEQ:
		return p.parseComparisonPattern()
	case token.IDENT:
		name := p.curToken.Literal

		// Wildcard
		if name == "_" {
			// Check for _: type
			if p.peekTokenIs(token.COLON) {
				p.nextToken() // skip :
				p.nextToken()
				typeName := p.curToken.Literal
				return &ast.BindingPattern{Name: &ast.Identifier{Name: "_"}, TypeName: typeName}
			}
			return &ast.WildcardPattern{}
		}

		// Extractor patterns: Some(x), None, Nil, All(...), Any(...), Tuple(...), Regex(...)
		if extractorNames[name] {
			if p.peekTokenIs(token.LPAREN) {
				p.nextToken() // skip (
				return p.parseExtractorPattern(name)
			}
			// None and Nil without parens
			if name == "None" || name == "Nil" {
				return &ast.ExtractorPattern{Name: name}
			}
		}

		if p.peekTokenIs(token.LPAREN) {
			p.nextToken()
			return p.parseConstructorPattern(name)
		}

		// Range pattern: identifier..expr (unlikely but handle ident case)
		if p.peekTokenIs(token.RANGE) {
			lowExpr := &ast.Identifier{Name: name}
			p.nextToken() // skip ..
			p.nextToken()
			highExpr := p.parseExpression(ASSIGN_PREC)
			return &ast.RangePattern{Low: lowExpr, High: highExpr}
		}

		// Type pattern: x: integer
		if p.peekTokenIs(token.COLON) {
			p.nextToken() // skip :
			p.nextToken()
			typeName := p.curToken.Literal
			return &ast.BindingPattern{Name: &ast.Identifier{Name: name}, TypeName: typeName}
		}

		// Simple binding pattern
		return &ast.BindingPattern{Name: &ast.Identifier{Name: name}}
	}

	p.errors = append(p.errors, fmt.Sprintf("Line %d: unexpected token in pattern: %s", p.curToken.Line, p.curToken.Literal))
	return nil
}

func (p *Parser) parseArrayPattern() ast.Pattern {
	pat := &ast.ArrayPattern{}
	p.nextToken() // skip [

	for !p.curTokenIs(token.RBRACKET) && !p.curTokenIs(token.EOF) {
		// Check for rest: ...name
		if p.curTokenIs(token.SPREAD) {
			if !p.expectPeek(token.IDENT) {
				return nil
			}
			pat.Rest = &ast.Identifier{Name: p.curToken.Literal}
			if p.peekTokenIs(token.COMMA) {
				p.nextToken()
			}
			p.nextToken()
			continue
		}

		elem := p.parseSinglePattern()
		if elem != nil {
			pat.Elements = append(pat.Elements, elem)
		}
		if p.peekTokenIs(token.COMMA) {
			p.nextToken()
		}
		p.nextToken()
	}

	return pat
}

func (p *Parser) parseObjectPattern() ast.Pattern {
	pat := &ast.ObjectPattern{}
	p.nextToken() // skip {

	for !p.curTokenIs(token.RBRACE) && !p.curTokenIs(token.EOF) {
		// Check for rest: ...name
		if p.curTokenIs(token.SPREAD) {
			if !p.expectPeek(token.IDENT) {
				return nil
			}
			pat.Rest = &ast.Identifier{Name: p.curToken.Literal}
			if p.peekTokenIs(token.COMMA) {
				p.nextToken()
			}
			p.nextToken()
			continue
		}

		if !p.curTokenIs(token.IDENT) && !p.curTokenIs(token.STRING) {
			p.errors = append(p.errors, fmt.Sprintf("Line %d: expected key in object pattern, got %s", p.curToken.Line, p.curToken.Literal))
			p.nextToken()
			continue
		}
		key := p.curToken.Literal

		if p.peekTokenIs(token.COLON) {
			p.nextToken() // skip :
			p.nextToken()
			// Value can be a literal pattern (for matching) or a binding pattern (for renaming)
			valuePat := p.parseSinglePattern()
			pat.Keys = append(pat.Keys, key)
			pat.Patterns = append(pat.Patterns, valuePat)
		} else {
			// Shorthand: {name} means key "name" bound to variable "name"
			pat.Keys = append(pat.Keys, key)
			pat.Patterns = append(pat.Patterns, &ast.BindingPattern{Name: &ast.Identifier{Name: key}})
		}

		if p.peekTokenIs(token.COMMA) {
			p.nextToken()
		}
		p.nextToken()
	}

	return pat
}

func (p *Parser) parseExtractorPattern(name string) ast.Pattern {
	pat := &ast.ExtractorPattern{Name: name}

	p.nextToken() // skip (
	for !p.curTokenIs(token.RPAREN) && !p.curTokenIs(token.EOF) {
		arg := p.parseSinglePattern()
		if arg != nil {
			pat.Args = append(pat.Args, arg)
		}
		if p.peekTokenIs(token.COMMA) {
			p.nextToken()
		}
		p.nextToken()
	}

	return pat
}

func (p *Parser) parseConstructorPattern(name string) ast.Pattern {
	pat := &ast.ConstructorPattern{Name: name}
	p.nextToken() // skip (
	for !p.curTokenIs(token.RPAREN) && !p.curTokenIs(token.EOF) {
		arg := p.parseSinglePattern()
		if arg != nil {
			pat.Args = append(pat.Args, arg)
		}
		if p.peekTokenIs(token.COMMA) {
			p.nextToken()
		}
		p.nextToken()
	}
	return pat
}

func (p *Parser) parseComparisonPattern() ast.Pattern {
	op := p.curToken.Type
	p.nextToken()
	value := p.parseExpression(PREFIX)
	return &ast.ComparisonPattern{Operator: op, Value: value}
}

func (p *Parser) parseDoWhileStatement() *ast.DoWhileStatement {
	stmt := &ast.DoWhileStatement{}

	if !p.expectPeek(token.LBRACE) {
		return nil
	}
	stmt.Body = p.parseBlockStatement()

	if !p.expectPeek(token.WHILE) {
		p.errors = append(p.errors, fmt.Sprintf("Line %d: expected 'while' after do block", p.curToken.Line))
		return nil
	}

	if !p.expectPeek(token.LPAREN) {
		return nil
	}
	p.nextToken()
	stmt.Condition = p.parseExpression(LOWEST)
	if !p.expectPeek(token.RPAREN) {
		return nil
	}

	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseOptionalDotExpression(left ast.Expression) ast.Expression {
	exp := &ast.OptionalDotExpression{Left: left}
	if !p.expectPeek(token.IDENT) {
		return nil
	}
	exp.Right = p.internIdentifier(p.curToken.Literal)
	return exp
}

func (p *Parser) parsePowerExpression(left ast.Expression) ast.Expression {
	expression := &ast.InfixExpression{
		Operator: p.curToken.Literal,
		Left:     left,
	}

	precedence := p.curPrecedence() - 1 // right-associative
	p.nextToken()
	expression.Right = p.parseExpression(precedence)

	return expression
}

func (p *Parser) infixParseFn() func(ast.Expression) ast.Expression {
	switch p.peekToken.Type {
	case token.PLUS, token.MINUS, token.MULTIPLY, token.DIVIDE, token.MODULO,
		token.EQ, token.NEQ, token.LT, token.GT, token.LTE, token.GTE,
		token.AND, token.OR,
		token.BITAND, token.BITOR, token.BITXOR, token.LSHIFT, token.RSHIFT,
		token.POWER:
		return p.parseInfixExpression
	case token.RANGE:
		return p.parseRangeExpression
	case token.LPAREN:
		return p.parseCallExpression
	case token.LBRACKET:
		return p.parseIndexExpression
	case token.DOT:
		return p.parseDotExpression
	case token.OPTIONAL_DOT:
		return p.parseOptionalDotExpression
	case token.ASSIGN:
		return p.parseAssignExpression
	}
	return nil
}

func (p *Parser) parseAssignExpression(left ast.Expression) ast.Expression {
	switch left.(type) {
	case *ast.Identifier, *ast.DotExpression, *ast.IndexExpression:
		// valid assignment target
	default:
		p.errors = append(p.errors, fmt.Sprintf("Line %d: invalid assignment target", p.curToken.Line))
		return nil
	}

	exp := &ast.AssignExpression{Target: left}

	p.nextToken()
	exp.Value = p.parseExpression(LOWEST)

	return exp
}

func (p *Parser) parseCompoundAssignExpression(left ast.Expression) ast.Expression {
	switch left.(type) {
	case *ast.Identifier, *ast.DotExpression, *ast.IndexExpression:
		// valid compound assignment target
	default:
		p.errors = append(p.errors, fmt.Sprintf("Line %d: invalid compound assignment target", p.curToken.Line))
		return nil
	}

	var op string
	switch p.curToken.Type {
	case token.PLUS_ASSIGN:
		op = "+"
	case token.MINUS_ASSIGN:
		op = "-"
	case token.MULTIPLY_ASSIGN:
		op = "*"
	case token.DIVIDE_ASSIGN:
		op = "/"
	case token.MODULO_ASSIGN:
		op = "%"
	case token.NULLISH_ASSIGN:
		op = "??"
	case token.BITAND_ASSIGN:
		op = "&"
	case token.BITOR_ASSIGN:
		op = "|"
	case token.BITXOR_ASSIGN:
		op = "^"
	case token.LSHIFT_ASSIGN:
		op = "<<"
	case token.RSHIFT_ASSIGN:
		op = ">>"
	case token.POWER_ASSIGN:
		op = "**"
	case token.AND_ASSIGN:
		op = "&&"
	case token.OR_ASSIGN:
		op = "||"
	}

	p.nextToken()
	value := p.parseExpression(LOWEST)

	return &ast.CompoundAssignExpression{Target: left, Operator: op, Value: value}
}

func (p *Parser) noPrefixParseFnError(t token.TokenType) {
	line, col := normalizePos(p.curToken.Line, p.curToken.Column, p.peekToken)
	msg := fmt.Sprintf(
		"Line %d:%d -> unexpected token %s at start of expression.",
		line,
		col,
		token.Debug(p.curToken),
	)
	msg += " Hint: check for missing value/expression before this token."
	if ctx := LineContext(p.l.Input(), line, col); ctx != "" {
		msg += "\n" + ctx
	}
	p.errors = append(p.errors, msg)
}

func (p *Parser) parseIdentifier() ast.Expression {
	return p.internIdentifier(p.curToken.Literal)
}

func (p *Parser) parseIntegerLiteral() ast.Expression {
	lit := &ast.IntegerLiteral{}

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

func (p *Parser) parseFloatLiteral() ast.Expression {
	lit := &ast.FloatLiteral{}

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

func (p *Parser) parseStringLiteral() ast.Expression {
	raw := p.curToken.Literal
	if !strings.Contains(raw, "${") {
		return &ast.StringLiteral{Value: raw}
	}
	// Template literal with interpolation
	tl := &ast.TemplateLiteral{}
	i := 0
	for i < len(raw) {
		idx := strings.Index(raw[i:], "${")
		if idx == -1 {
			tl.Parts = append(tl.Parts, &ast.StringLiteral{Value: raw[i:]})
			break
		}
		if idx > 0 {
			tl.Parts = append(tl.Parts, &ast.StringLiteral{Value: raw[i : i+idx]})
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
		exprLexer := lexer.NewLexer(exprStr)
		exprParser := NewParser(exprLexer)
		expr := exprParser.parseExpression(LOWEST)
		if expr != nil {
			tl.Parts = append(tl.Parts, expr)
		}
		i = j + 1
	}
	if len(tl.Parts) == 1 {
		if sl, ok := tl.Parts[0].(*ast.StringLiteral); ok {
			return sl
		}
	}
	return tl
}

func (p *Parser) parseBooleanLiteral() ast.Expression {
	return &ast.BooleanLiteral{Value: p.curTokenIs(token.TRUE)}
}

func (p *Parser) parseNullLiteral() ast.Expression {
	return &ast.NullLiteral{}
}

func (p *Parser) parseArrayLiteral() ast.Expression {
	array := &ast.ArrayLiteral{}
	array.Elements = p.parseExpressionList(token.RBRACKET)
	return array
}

func (p *Parser) parseExpressionList(end token.TokenType) []ast.Expression {
	list := make([]ast.Expression, 0, 4)

	if p.peekTokenIs(end) {
		p.nextToken()
		return list
	}

	p.nextToken()
	if p.curTokenIs(token.SPREAD) {
		p.nextToken()
		list = append(list, &ast.SpreadExpression{Value: p.parseExpression(LOWEST)})
	} else {
		list = append(list, p.parseExpression(LOWEST))
	}

	for p.peekTokenIs(token.COMMA) {
		p.nextToken()
		p.nextToken()
		if p.curTokenIs(token.SPREAD) {
			p.nextToken()
			list = append(list, &ast.SpreadExpression{Value: p.parseExpression(LOWEST)})
		} else {
			list = append(list, p.parseExpression(LOWEST))
		}
	}

	if !p.expectPeek(end) {
		return nil
	}

	return list
}

func (p *Parser) parseHashLiteral() ast.Expression {
	hash := &ast.HashLiteral{}

	for !p.peekTokenIs(token.RBRACE) {
		p.nextToken()

		// Handle spread: ...expr
		if p.curTokenIs(token.SPREAD) {
			p.nextToken()
			spreadExpr := p.parseExpression(LOWEST)
			hash.Entries = append(hash.Entries, ast.HashEntry{Value: spreadExpr, IsSpread: true})
		} else if p.curTokenIs(token.LBRACKET) {
			// Computed property key: { [expr]: value }
			p.nextToken() // past [
			keyExpr := p.parseExpression(LOWEST)
			if !p.expectPeek(token.RBRACKET) {
				return nil
			}
			if !p.expectPeek(token.COLON) {
				return nil
			}
			p.nextToken()
			value := p.parseExpression(LOWEST)
			hash.Entries = append(hash.Entries, ast.HashEntry{Key: keyExpr, Value: value, IsComputed: true})
		} else {
			key := p.parseExpression(LOWEST)

			// Property shorthand: { x } is sugar for { "x": x }
			if ident, ok := key.(*ast.Identifier); ok && !p.peekTokenIs(token.COLON) {
				strKey := &ast.StringLiteral{Value: ident.Name}
				hash.Entries = append(hash.Entries, ast.HashEntry{Key: strKey, Value: ident})
			} else {
				if !p.expectPeek(token.COLON) {
					return nil
				}

				p.nextToken()
				value := p.parseExpression(LOWEST)

				hash.Entries = append(hash.Entries, ast.HashEntry{Key: key, Value: value})
			}
		}

		if !p.peekTokenIs(token.RBRACE) && !p.expectPeek(token.COMMA) {
			return nil
		}
	}

	if !p.expectPeek(token.RBRACE) {
		return nil
	}

	return hash
}

func (p *Parser) parsePrefixExpression() ast.Expression {
	expression := &ast.PrefixExpression{
		Operator: p.curToken.Literal,
	}

	p.nextToken()

	expression.Right = p.parseExpression(PREFIX)

	return expression
}

func (p *Parser) parseInfixExpression(left ast.Expression) ast.Expression {
	expression := &ast.InfixExpression{
		Operator: p.curToken.Literal,
		Left:     left,
	}

	precedence := p.curPrecedence()
	p.nextToken()
	expression.Right = p.parseExpression(precedence)

	return expression
}

func (p *Parser) parseRangeExpression(left ast.Expression) ast.Expression {
	p.nextToken() // consume ..
	right := p.parseExpression(RANGE_PREC)
	return &ast.RangeExpression{Left: left, Right: right}
}

func (p *Parser) parsePipelineExpression(left ast.Expression) ast.Expression {
	p.nextToken() // move to right side expression
	right := p.parseExpression(ASSIGN_PREC)
	if right == nil {
		return nil
	}
	if call, ok := right.(*ast.CallExpression); ok {
		args := make([]ast.Expression, 0, len(call.Arguments)+1)
		args = append(args, left)
		args = append(args, call.Arguments...)
		call.Arguments = args
		return call
	}
	return &ast.CallExpression{Function: right, Arguments: []ast.Expression{left}, Line: p.curToken.Line, Column: p.curToken.Column}
}

func (p *Parser) parseNewExpression() ast.Expression {
	p.nextToken()
	constructor := p.parseExpression(ASSIGN_PREC)
	if constructor == nil {
		return nil
	}
	if call, ok := constructor.(*ast.CallExpression); ok {
		return call
	}
	return &ast.CallExpression{Function: constructor, Arguments: []ast.Expression{}, Line: p.curToken.Line, Column: p.curToken.Column}
}

func (p *Parser) isArrowAfterParens() bool {
	saved := p.saveState()
	defer p.restoreState(saved)
	// curToken is token.LPAREN
	depth := 1
	p.nextToken() // advance past (
	for depth > 0 {
		switch p.curToken.Type {
		case token.LPAREN:
			depth++
		case token.RPAREN:
			depth--
			if depth == 0 {
				return p.peekTokenIs(token.ARROW)
			}
		case token.EOF:
			return false
		}
		p.nextToken()
	}
	return false
}

func (p *Parser) parseArrowFunction() ast.Expression {
	lit := &ast.FunctionLiteral{IsArrow: true}
	// curToken is token.LPAREN
	lit.Parameters, lit.ParamTypes, lit.Defaults, lit.HasRest = p.parseFunctionParameters()
	if !p.expectPeek(token.ARROW) {
		return nil
	}
	p.nextToken() // advance past =>
	if p.curTokenIs(token.LBRACE) {
		lit.Body = p.parseBlockStatement()
	} else {
		expr := p.parseExpression(LOWEST)
		lit.Body = &ast.BlockStatement{
			Statements: []ast.Statement{
				&ast.ReturnStatement{ReturnValue: expr},
			},
		}
	}
	return lit
}

func (p *Parser) parseGroupedExpression() ast.Expression {
	if p.isArrowAfterParens() {
		return p.parseArrowFunction()
	}
	p.nextToken()

	exp := p.parseExpression(LOWEST)

	if !p.expectPeek(token.RPAREN) {
		return nil
	}

	return exp
}

func (p *Parser) parseIfExpression() ast.Expression {
	expression := &ast.IfExpression{}

	if !p.expectPeek(token.LPAREN) {
		return nil
	}

	p.nextToken()
	expression.Condition = p.parseExpression(LOWEST)

	if !p.expectPeek(token.RPAREN) {
		return nil
	}

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	expression.Consequence = p.parseBlockStatement()

	if p.peekTokenIs(token.ELSE) {
		p.nextToken()

		if !p.expectPeek(token.LBRACE) {
			return nil
		}

		expression.Alternative = p.parseBlockStatement()
	}

	return expression
}

func (p *Parser) parseFunctionLiteral() ast.Expression {
	lit := &ast.FunctionLiteral{}

	if p.peekTokenIs(token.IDENT) {
		p.nextToken()
		lit.Name = p.internIdentifier(p.curToken.Literal)
	}

	if !p.expectPeek(token.LPAREN) {
		return nil
	}

	lit.Parameters, lit.ParamTypes, lit.Defaults, lit.HasRest = p.parseFunctionParameters()
	if p.peekTokenIs(token.COLON) {
		p.nextToken()
		lit.ReturnType = p.parseTypeName()
		if lit.ReturnType == "" {
			return nil
		}
	}

	if !p.expectPeek(token.LBRACE) {
		return nil
	}

	lit.Body = p.parseBlockStatement()

	return lit
}

func (p *Parser) parseFunctionDeclaration() ast.Statement {
	// curToken is token.FUNCTION, peekToken is token.IDENT
	p.nextToken() // advance to the name
	name := p.internIdentifier(p.curToken.Literal)

	if !p.expectPeek(token.LPAREN) {
		return nil
	}
	params, paramTypes, defaults, hasRest := p.parseFunctionParameters()
	returnType := ""
	if p.peekTokenIs(token.COLON) {
		p.nextToken()
		returnType = p.parseTypeName()
		if returnType == "" {
			return nil
		}
	}

	if !p.expectPeek(token.LBRACE) {
		return nil
	}
	body := p.parseBlockStatement()

	fn := &ast.FunctionLiteral{Name: name, Parameters: params, ParamTypes: paramTypes, Defaults: defaults, HasRest: hasRest, ReturnType: returnType, Body: body}
	return &ast.LetStatement{Name: name, Names: []*ast.Identifier{name}, Value: fn}
}

func (p *Parser) parseTypeName() string {
	if !p.expectPeek(token.IDENT) {
		return ""
	}
	return strings.TrimSpace(p.curToken.Literal)
}

func (p *Parser) parseInitStatement() ast.Statement {
	if !p.expectPeek(token.LBRACE) {
		return nil
	}
	stmt := &ast.InitStatement{Body: p.parseBlockStatement()}
	p.initBlocks = append(p.initBlocks, stmt)
	return stmt
}

func (p *Parser) parseClassStatement() ast.Statement {
	if !p.expectPeek(token.IDENT) {
		return nil
	}
	stmt := &ast.ClassStatement{Name: p.internIdentifier(p.curToken.Literal)}
	if !p.expectPeek(token.LBRACE) {
		return nil
	}
	p.nextToken()
	for !p.curTokenIs(token.RBRACE) && !p.curTokenIs(token.EOF) {
		if !p.curTokenIs(token.IDENT) {
			p.errors = append(p.errors, fmt.Sprintf("Line %d: expected class method name, got %s", p.curToken.Line, token.Debug(p.curToken)))
			return nil
		}
		m := &ast.ClassMethod{Name: p.internIdentifier(p.curToken.Literal)}
		if !p.expectPeek(token.LPAREN) {
			return nil
		}
		params, _, _, _ := p.parseFunctionParameters()
		m.Parameters = params
		if !p.expectPeek(token.LBRACE) {
			return nil
		}
		m.Body = p.parseBlockStatement()
		stmt.Methods = append(stmt.Methods, m)
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseInterfaceStatement() ast.Statement {
	if !p.expectPeek(token.IDENT) {
		return nil
	}
	stmt := &ast.InterfaceStatement{Name: p.internIdentifier(p.curToken.Literal)}
	if !p.expectPeek(token.LBRACE) {
		return nil
	}
	p.nextToken()
	for !p.curTokenIs(token.RBRACE) && !p.curTokenIs(token.EOF) {
		if !p.curTokenIs(token.IDENT) {
			p.errors = append(p.errors, fmt.Sprintf("Line %d: expected interface method name, got %s", p.curToken.Line, token.Debug(p.curToken)))
			return nil
		}
		methodName := p.internIdentifier(p.curToken.Literal)
		if !p.expectPeek(token.LPAREN) {
			return nil
		}
		params, paramTypes, _, _ := p.parseFunctionParameters()
		retType := ""
		if p.peekTokenIs(token.COLON) {
			p.nextToken()
			retType = p.parseTypeName()
			if retType == "" {
				return nil
			}
		}
		if p.peekTokenIs(token.SEMICOLON) {
			p.nextToken()
		}
		stmt.Methods = append(stmt.Methods, &ast.InterfaceMethod{Name: methodName, Parameters: params, ParamTypes: paramTypes, ReturnType: retType})
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseTestStatement() ast.Statement {
	if !p.expectPeek(token.STRING) {
		return nil
	}
	name := p.curToken.Literal
	if !p.expectPeek(token.LBRACE) {
		return nil
	}
	body := p.parseBlockStatement()
	return &ast.TestStatement{Name: name, Body: body}
}

func (p *Parser) parseTypeDeclarationStatement() ast.Statement {
	stmt := &ast.TypeDeclarationStatement{}
	if !p.expectPeek(token.IDENT) {
		return nil
	}
	stmt.Name = p.internIdentifier(p.curToken.Literal)
	if !p.expectPeek(token.ASSIGN) {
		return nil
	}
	for {
		if p.peekTokenIs(token.BITOR) {
			p.nextToken()
		}
		if !p.expectPeek(token.IDENT) {
			return nil
		}
		variant := &ast.ADTVariantDecl{Name: p.internIdentifier(p.curToken.Literal)}
		if p.peekTokenIs(token.LPAREN) {
			p.nextToken()
			if !p.peekTokenIs(token.RPAREN) {
				for {
					if !p.expectPeek(token.IDENT) {
						return nil
					}
					variant.Fields = append(variant.Fields, p.internIdentifier(p.curToken.Literal))
					if !p.peekTokenIs(token.COMMA) {
						break
					}
					p.nextToken()
				}
			}
			if !p.expectPeek(token.RPAREN) {
				return nil
			}
		}
		stmt.Variants = append(stmt.Variants, variant)
		if !p.peekTokenIs(token.BITOR) {
			break
		}
	}
	if p.peekTokenIs(token.SEMICOLON) {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseFunctionParameters() ([]*ast.Identifier, []string, []ast.Expression, bool) {
	identifiers := []*ast.Identifier{}
	paramTypes := []string{}
	defaults := []ast.Expression{}
	hasRest := false

	if p.peekTokenIs(token.RPAREN) {
		p.nextToken()
		return identifiers, paramTypes, defaults, false
	}

	p.nextToken()

	if p.curTokenIs(token.SPREAD) {
		p.nextToken() // move to ident
		hasRest = true
	}
	ident := p.internIdentifier(p.curToken.Literal)
	identifiers = append(identifiers, ident)
	if !hasRest && p.peekTokenIs(token.COLON) {
		p.nextToken()
		typeName := p.parseTypeName()
		if typeName == "" {
			return nil, nil, nil, false
		}
		paramTypes = append(paramTypes, typeName)
	} else {
		paramTypes = append(paramTypes, "")
	}

	if !hasRest && p.peekTokenIs(token.ASSIGN) {
		p.nextToken() // consume =
		p.nextToken() // move to expression
		defaults = append(defaults, p.parseExpression(LOWEST))
	} else {
		defaults = append(defaults, nil)
	}

	for p.peekTokenIs(token.COMMA) {
		if hasRest {
			p.errors = append(p.errors, fmt.Sprintf("Line %d: rest parameter must be last", p.curToken.Line))
			return nil, nil, nil, false
		}
		p.nextToken()
		p.nextToken()

		if p.curTokenIs(token.SPREAD) {
			p.nextToken()
			hasRest = true
		}
		ident := p.internIdentifier(p.curToken.Literal)
		identifiers = append(identifiers, ident)
		if !hasRest && p.peekTokenIs(token.COLON) {
			p.nextToken()
			typeName := p.parseTypeName()
			if typeName == "" {
				return nil, nil, nil, false
			}
			paramTypes = append(paramTypes, typeName)
		} else {
			paramTypes = append(paramTypes, "")
		}

		if !hasRest && p.peekTokenIs(token.ASSIGN) {
			p.nextToken()
			p.nextToken()
			defaults = append(defaults, p.parseExpression(LOWEST))
		} else {
			defaults = append(defaults, nil)
		}
	}

	if !p.expectPeek(token.RPAREN) {
		return nil, nil, nil, false
	}

	return identifiers, paramTypes, defaults, hasRest
}

func (p *Parser) parseDotExpression(left ast.Expression) ast.Expression {
	exp := &ast.DotExpression{Left: left}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	exp.Right = p.internIdentifier(p.curToken.Literal)

	return exp
}

func (p *Parser) parseCallExpression(function ast.Expression) ast.Expression {
	exp := &ast.CallExpression{
		Function: function,
		Line:     p.curToken.Line,
		Column:   p.curToken.Column,
	}
	exp.Arguments = p.parseExpressionList(token.RPAREN)
	return exp
}

func (p *Parser) parseIndexExpression(left ast.Expression) ast.Expression {
	exp := &ast.IndexExpression{Left: left}

	p.nextToken()
	exp.Index = p.parseExpression(LOWEST)

	if !p.expectPeek(token.RBRACKET) {
		return nil
	}

	return exp
}
