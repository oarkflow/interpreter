package lua

import "fmt"

type parser struct {
	lexer   *lexer
	current token
	peek    token
	err     error
}

func parse(source, name string) (*block, error) {
	p := &parser{lexer: newLexer(source, name)}
	p.current, p.err = p.lexer.next()
	if p.err != nil {
		return nil, p.err
	}
	p.peek, p.err = p.lexer.next()
	if p.err != nil {
		return nil, p.err
	}
	body := p.parseBlock(tEOF)
	if p.err != nil {
		return nil, p.err
	}
	if p.current.kind != tEOF {
		p.fail(p.current, "unexpected token %q", p.current.lit)
		return nil, p.err
	}
	return body, nil
}

func (p *parser) advance() {
	if p.err != nil {
		return
	}
	p.current = p.peek
	p.peek, p.err = p.lexer.next()
}

func (p *parser) accept(kind tokenKind) bool {
	if p.current.kind != kind {
		return false
	}
	p.advance()
	return true
}

func (p *parser) expect(kind tokenKind, description string) token {
	t := p.current
	if p.err != nil {
		return t
	}
	if t.kind != kind {
		p.fail(t, "expected %s, got %q", description, t.lit)
		return t
	}
	p.advance()
	return t
}

func (p *parser) fail(t token, format string, args ...any) {
	if p.err == nil {
		p.err = &Error{Source: p.lexer.name, Line: t.line, Msg: fmt.Sprintf(format, args...)}
	}
}

func (p *parser) parseBlock(ends ...tokenKind) *block {
	result := &block{}
	for p.err == nil && !containsToken(ends, p.current.kind) {
		stmt := p.parseStatement()
		if stmt != nil {
			result.statements = append(result.statements, stmt)
		}
		p.accept(tSemi)
		if _, ok := stmt.(*returnStatement); ok {
			break
		}
		if _, ok := stmt.(*breakStatement); ok {
			break
		}
	}
	return result
}

func containsToken(items []tokenKind, value tokenKind) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func (p *parser) parseStatement() statement {
	switch p.current.kind {
	case tLocal:
		return p.parseLocal()
	case tFunction:
		return p.parseNamedFunction(false)
	case tIf:
		return p.parseIf()
	case tWhile:
		return p.parseWhile()
	case tRepeat:
		return p.parseRepeat()
	case tFor:
		return p.parseFor()
	case tDo:
		line := p.current.line
		p.advance()
		body := p.parseBlock(tEnd)
		p.expect(tEnd, "end")
		return &doStatement{line: line, body: body}
	case tReturn:
		line := p.current.line
		p.advance()
		var values []expression
		if !containsToken([]tokenKind{tEOF, tEnd, tElse, tElseIf, tUntil, tSemi}, p.current.kind) {
			values = p.parseExpressionList()
		}
		return &returnStatement{line: line, values: values}
	case tBreak:
		line := p.current.line
		p.advance()
		return &breakStatement{line: line}
	default:
		return p.parseAssignmentOrCall()
	}
}

func (p *parser) parseLocal() statement {
	line := p.current.line
	p.advance()
	if p.accept(tFunction) {
		name := p.expect(tName, "local function name")
		fn := p.parseFunctionBody(line)
		return &localStatement{line: line, names: []string{name.lit}, values: []expression{fn}}
	}
	names := []string{p.expect(tName, "local name").lit}
	for p.accept(tComma) {
		names = append(names, p.expect(tName, "local name").lit)
	}
	var values []expression
	if p.accept(tAssign) {
		values = p.parseExpressionList()
	}
	return &localStatement{line: line, names: names, values: values}
}

func (p *parser) parseNamedFunction(local bool) statement {
	line := p.current.line
	p.advance()
	var target expression = &nameExpression{line: line, name: p.expect(tName, "function name").lit}
	for p.accept(tDot) {
		name := p.expect(tName, "field name")
		target = &indexExpression{line: name.line, table: target, key: &literalExpression{line: name.line, value: String(name.lit)}}
	}
	method := ""
	if p.accept(tColon) {
		name := p.expect(tName, "method name")
		method = name.lit
		target = &indexExpression{line: name.line, table: target, key: &literalExpression{line: name.line, value: String(name.lit)}}
	}
	fn := p.parseFunctionBody(line)
	if method != "" {
		fn.parameters = append([]string{"self"}, fn.parameters...)
	}
	return &assignStatement{line: line, targets: []expression{target}, values: []expression{fn}}
}

func (p *parser) parseIf() statement {
	line := p.current.line
	p.advance()
	result := &ifStatement{line: line}
	for {
		condition := p.parseExpression(0)
		p.expect(tThen, "then")
		body := p.parseBlock(tElseIf, tElse, tEnd)
		result.branches = append(result.branches, ifBranch{condition: condition, body: body})
		if !p.accept(tElseIf) {
			break
		}
	}
	if p.accept(tElse) {
		result.otherwise = p.parseBlock(tEnd)
	}
	p.expect(tEnd, "end")
	return result
}

func (p *parser) parseWhile() statement {
	line := p.current.line
	p.advance()
	condition := p.parseExpression(0)
	p.expect(tDo, "do")
	body := p.parseBlock(tEnd)
	p.expect(tEnd, "end")
	return &whileStatement{line: line, condition: condition, body: body}
}

func (p *parser) parseRepeat() statement {
	line := p.current.line
	p.advance()
	body := p.parseBlock(tUntil)
	p.expect(tUntil, "until")
	return &repeatStatement{line: line, body: body, condition: p.parseExpression(0)}
}

func (p *parser) parseFor() statement {
	line := p.current.line
	p.advance()
	first := p.expect(tName, "for variable")
	if p.accept(tAssign) {
		initial := p.parseExpression(0)
		p.expect(tComma, ",")
		limit := p.parseExpression(0)
		step := expression(&literalExpression{line: line, value: Number(1)})
		if p.accept(tComma) {
			step = p.parseExpression(0)
		}
		p.expect(tDo, "do")
		body := p.parseBlock(tEnd)
		p.expect(tEnd, "end")
		return &numericForStatement{line: line, name: first.lit, initial: initial, limit: limit, step: step, body: body}
	}
	names := []string{first.lit}
	for p.accept(tComma) {
		names = append(names, p.expect(tName, "for variable").lit)
	}
	p.expect(tIn, "in")
	values := p.parseExpressionList()
	p.expect(tDo, "do")
	body := p.parseBlock(tEnd)
	p.expect(tEnd, "end")
	return &genericForStatement{line: line, names: names, values: values, body: body}
}

func (p *parser) parseAssignmentOrCall() statement {
	first := p.parsePrefixExpression()
	if call, ok := first.(*callExpression); ok && p.current.kind != tAssign && p.current.kind != tComma {
		return &callStatement{line: call.line, call: call}
	}
	targets := []expression{first}
	for p.accept(tComma) {
		targets = append(targets, p.parsePrefixExpression())
	}
	for _, target := range targets {
		switch target.(type) {
		case *nameExpression, *indexExpression:
		default:
			p.fail(p.current, "assignment target expected")
		}
	}
	p.expect(tAssign, "=")
	return &assignStatement{line: first.lineNumber(), targets: targets, values: p.parseExpressionList()}
}

func (p *parser) parseExpressionList() []expression {
	values := []expression{p.parseExpression(0)}
	for p.accept(tComma) {
		values = append(values, p.parseExpression(0))
	}
	return values
}

var binaryPrecedence = map[tokenKind]int{
	tOr: 1, tAnd: 2, tLT: 3, tGT: 3, tLTE: 3, tGTE: 3, tNotEq: 3, tEqEq: 3,
	tConcat: 4, tPlus: 5, tMinus: 5, tStar: 6, tSlash: 6, tPercent: 6, tCaret: 8,
}

func (p *parser) parseExpression(min int) expression {
	left := p.parseUnary()
	for p.err == nil {
		precedence, ok := binaryPrecedence[p.current.kind]
		if !ok || precedence < min {
			break
		}
		op := p.current
		p.advance()
		next := precedence + 1
		if op.kind == tConcat || op.kind == tCaret {
			next = precedence
		}
		right := p.parseExpression(next)
		left = &binaryExpression{line: op.line, operator: op.kind, left: left, right: right}
	}
	return left
}

func (p *parser) parseUnary() expression {
	if p.current.kind == tNot || p.current.kind == tMinus || p.current.kind == tHash {
		op := p.current
		p.advance()
		if p.err != nil {
			return &literalExpression{line: op.line, value: Nil}
		}
		return &unaryExpression{line: op.line, operator: op.kind, value: p.parseExpression(7)}
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() expression {
	t := p.current
	switch t.kind {
	case tNil:
		p.advance()
		return &literalExpression{line: t.line, value: Nil}
	case tTrue:
		p.advance()
		return &literalExpression{line: t.line, value: True}
	case tFalse:
		p.advance()
		return &literalExpression{line: t.line, value: False}
	case tNumber:
		p.advance()
		return &literalExpression{line: t.line, value: Number(t.num)}
	case tString:
		p.advance()
		return &literalExpression{line: t.line, value: String(t.lit)}
	case tDots:
		p.advance()
		return &varargExpression{line: t.line}
	case tFunction:
		p.advance()
		return p.parseFunctionBody(t.line)
	case tLBrace:
		return p.parseTable()
	case tName, tLParen:
		return p.parsePrefixExpression()
	default:
		p.fail(t, "expression expected, got %q", t.lit)
		return &literalExpression{line: t.line, value: Nil}
	}
}

func (p *parser) parsePrefixExpression() expression {
	var result expression
	if p.current.kind == tName {
		t := p.current
		p.advance()
		result = &nameExpression{line: t.line, name: t.lit}
	} else {
		open := p.expect(tLParen, "(")
		value := p.parseExpression(0)
		p.expect(tRParen, ")")
		result = &parenthesizedExpression{line: open.line, value: value}
	}
	for p.err == nil {
		switch p.current.kind {
		case tDot:
			p.advance()
			name := p.expect(tName, "field name")
			result = &indexExpression{line: name.line, table: result, key: &literalExpression{line: name.line, value: String(name.lit)}}
		case tLBracket:
			line := p.current.line
			p.advance()
			key := p.parseExpression(0)
			p.expect(tRBracket, "]")
			result = &indexExpression{line: line, table: result, key: key}
		case tColon:
			line := p.current.line
			p.advance()
			name := p.expect(tName, "method name")
			receiver := result
			fn := &indexExpression{line: line, table: receiver, key: &literalExpression{line: name.line, value: String(name.lit)}}
			result = &callExpression{line: line, function: fn, receiver: receiver, args: p.parseArguments()}
		case tLParen, tLBrace, tString:
			line := p.current.line
			args := p.parseArguments()
			result = &callExpression{line: line, function: result, args: args}
		default:
			return result
		}
	}
	return result
}

func (p *parser) parseArguments() []expression {
	if p.accept(tLParen) {
		if p.accept(tRParen) {
			return nil
		}
		args := p.parseExpressionList()
		p.expect(tRParen, ")")
		return args
	}
	if p.current.kind == tLBrace {
		return []expression{p.parseTable()}
	}
	t := p.expect(tString, "call arguments")
	return []expression{&literalExpression{line: t.line, value: String(t.lit)}}
}

func (p *parser) parseFunctionBody(line int) *functionExpression {
	p.expect(tLParen, "(")
	fn := &functionExpression{line: line}
	if !p.accept(tRParen) {
		if p.accept(tDots) {
			fn.vararg = true
		} else {
			fn.parameters = append(fn.parameters, p.expect(tName, "parameter").lit)
			for p.accept(tComma) {
				if p.accept(tDots) {
					fn.vararg = true
					break
				}
				fn.parameters = append(fn.parameters, p.expect(tName, "parameter").lit)
			}
		}
		p.expect(tRParen, ")")
	}
	fn.body = p.parseBlock(tEnd)
	fn.lastLine = p.expect(tEnd, "end").line
	return fn
}

func (p *parser) parseTable() expression {
	line := p.current.line
	p.advance()
	table := &tableExpression{line: line}
	listIndex := 1
	for p.err == nil && p.current.kind != tRBrace {
		field := tableField{}
		if p.accept(tLBracket) {
			field.key = p.parseExpression(0)
			p.expect(tRBracket, "]")
			p.expect(tAssign, "=")
			field.value = p.parseExpression(0)
		} else if p.current.kind == tName && p.peek.kind == tAssign {
			name := p.current
			p.advance()
			p.advance()
			field.key = &literalExpression{line: name.line, value: String(name.lit)}
			field.value = p.parseExpression(0)
		} else {
			field.list = true
			field.key = &literalExpression{line: p.current.line, value: Number(float64(listIndex))}
			listIndex++
			field.value = p.parseExpression(0)
		}
		table.fields = append(table.fields, field)
		if !p.accept(tComma) && !p.accept(tSemi) {
			break
		}
	}
	p.expect(tRBrace, "}")
	return table
}
