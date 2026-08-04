package lua

import (
	"fmt"
	"math"
	"strings"
)

type variableClass uint8

const (
	globalVariable variableClass = iota
	localVariable
	upvalueVariable
)

type localBinding struct {
	name            string
	register, depth int
	startPC         int
}
type resolvedVariable struct {
	class variableClass
	index int
	name  string
}
type loopContext struct {
	breaks    []int
	closeFrom int
}

type compiler struct {
	parent           *compiler
	proto            *Prototype
	locals           []localBinding
	upvalueNames     []string
	depth, free, max int
	loops            []loopContext
	captured         map[int]bool
}

func Compile(source, name string) (*Prototype, error) {
	ast, err := parse(source, name)
	if err != nil {
		return nil, err
	}
	c := newCompiler(nil, name, nil, false)
	normalized, _ := normalizeLuaNewlines(source)
	c.proto.EndLine = strings.Count(strings.TrimRight(normalized, "\n"), "\n") + 1
	if err = c.compileBlock(ast); err != nil {
		return nil, err
	}
	if len(c.proto.Code) == 0 || c.proto.Code[len(c.proto.Code)-1].op() != opReturn {
		c.emit(abc(opReturn, 0, 0, 0), 0)
	}
	c.finalizeLocals()
	if c.max > 255 {
		return nil, c.error(0, "function needs more than 255 registers")
	}
	c.proto.MaxRegisters = uint8(c.max)
	markFastPrototypes(c.proto)
	markNumericPrototypes(c.proto)
	initializePrototypeCaches(c.proto)
	return c.proto, nil
}

func initializePrototypeCaches(p *Prototype) {
	p.FieldCaches = make([]fieldCache, len(p.Code))
	for _, child := range p.Children {
		initializePrototypeCaches(child)
	}
}

func markNumericPrototypes(p *Prototype) {
	for _, child := range p.Children {
		markNumericPrototypes(child)
	}
	if p.Vararg || len(p.Upvalues) != 0 || p.MaxRegisters > 16 {
		return
	}
	for _, constant := range p.Constants {
		if constant.kind != NumberKind {
			return
		}
	}
	for _, ins := range p.Code {
		switch ins.op() {
		case opMove, opLoadK, opAdd, opAddK, opSub, opSubK, opMul, opMulK, opDiv, opDivK, opMod, opModK, opPow, opPowK, opNeg:
		case opReturn:
			if ins.b() != 1 {
				return
			}
		default:
			return
		}
	}
	if len(p.Code) == 0 {
		return
	}
	var aliases [256]int
	for i := range aliases {
		aliases[i] = -1
	}
	next := int(p.Parameters) + len(p.Constants)
	if next >= 16 {
		return
	}
	for i := 0; i < int(p.Parameters); i++ {
		aliases[i] = i
	}
	plan := make([]Instruction, 0, len(p.Code))
	for _, ins := range p.Code {
		a, b, c := ins.a(), ins.b(), ins.c()
		switch ins.op() {
		case opMove:
			if aliases[b] < 0 {
				return
			}
			aliases[a] = aliases[b]
		case opLoadK:
			aliases[a] = int(p.Parameters) + ins.bx()
		case opAdd, opSub, opMul, opDiv, opMod, opPow:
			if aliases[b] < 0 || aliases[c] < 0 || next >= 16 {
				return
			}
			aliases[a] = next
			plan = append(plan, abc(ins.op(), uint8(next), uint8(aliases[b]), uint8(aliases[c])))
			next++
		case opAddK, opSubK, opMulK, opDivK, opModK, opPowK:
			if aliases[b] < 0 || next >= 16 {
				return
			}
			regular := map[opcode]opcode{opAddK: opAdd, opSubK: opSub, opMulK: opMul, opDivK: opDiv, opModK: opMod, opPowK: opPow}[ins.op()]
			aliases[a] = next
			plan = append(plan, abc(regular, uint8(next), uint8(aliases[b]), uint8(int(p.Parameters)+c)))
			next++
		case opNeg:
			if aliases[b] < 0 || next >= 16 {
				return
			}
			aliases[a] = next
			plan = append(plan, abc(opNeg, uint8(next), uint8(aliases[b]), 0))
			next++
		case opReturn:
			if aliases[a] < 0 {
				return
			}
			plan = append(plan, abc(opReturn, uint8(aliases[a]), 1, 0))
		}
	}
	p.NumericPure = true
	p.NumericCode = plan
	p.NumericRegisters = uint8(next)
	p.NumericFormula = buildNumericFormula(p)
}

type numericPolynomial struct {
	coeff [6]float64 // 1, x, y, x*x, x*y, y*y
	ok    bool
}

func buildNumericFormula(p *Prototype) *numericFormula {
	if p.Parameters != 2 || p.NumericRegisters > 16 {
		return nil
	}
	var regs [16]numericPolynomial
	regs[0], regs[1] = numericPolynomial{coeff: [6]float64{0, 1}, ok: true}, numericPolynomial{coeff: [6]float64{0, 0, 1}, ok: true}
	for i, constant := range p.Constants {
		index := int(p.Parameters) + i
		if index >= len(regs) || constant.kind != NumberKind {
			return nil
		}
		regs[index] = numericPolynomial{coeff: [6]float64{constant.Number()}, ok: true}
	}
	for pc, ins := range p.NumericCode {
		a, b, c := int(ins.a()), int(ins.b()), int(ins.c())
		switch ins.op() {
		case opMove:
			regs[a] = regs[b]
		case opLoadK:
			constant := p.Constants[ins.bx()]
			regs[a] = numericPolynomial{coeff: [6]float64{constant.Number()}, ok: constant.kind == NumberKind}
		case opAdd, opSub:
			if !regs[b].ok || !regs[c].ok {
				return nil
			}
			out := numericPolynomial{ok: true}
			for i := range out.coeff {
				out.coeff[i] = regs[b].coeff[i] + regs[c].coeff[i]
				if ins.op() == opSub {
					out.coeff[i] = regs[b].coeff[i] - regs[c].coeff[i]
				}
			}
			regs[a] = out
		case opNeg:
			if !regs[b].ok {
				return nil
			}
			out := numericPolynomial{ok: true}
			for i := range out.coeff {
				out.coeff[i] = -regs[b].coeff[i]
			}
			regs[a] = out
		case opMul:
			out, ok := multiplyQuadratic(regs[b], regs[c])
			if !ok {
				return nil
			}
			regs[a] = out
		case opDiv:
			if !regs[b].ok || !regs[c].ok || pc+1 >= len(p.NumericCode) || p.NumericCode[pc+1].op() != opReturn || int(p.NumericCode[pc+1].a()) != a {
				return nil
			}
			return &numericFormula{numerator: regs[b].coeff, denominator: regs[c].coeff}
		case opReturn:
			if !regs[a].ok {
				return nil
			}
			return &numericFormula{numerator: regs[a].coeff, denominator: [6]float64{1}}
		default:
			return nil
		}
	}
	return nil
}

func multiplyQuadratic(a, b numericPolynomial) (numericPolynomial, bool) {
	if !a.ok || !b.ok {
		return numericPolynomial{}, false
	}
	// Any quadratic term multiplied by a non-constant term would exceed the
	// compact representation.
	if (a.coeff[3] != 0 || a.coeff[4] != 0 || a.coeff[5] != 0) && (b.coeff[1] != 0 || b.coeff[2] != 0 || b.coeff[3] != 0 || b.coeff[4] != 0 || b.coeff[5] != 0) ||
		(b.coeff[3] != 0 || b.coeff[4] != 0 || b.coeff[5] != 0) && (a.coeff[1] != 0 || a.coeff[2] != 0) {
		return numericPolynomial{}, false
	}
	out := numericPolynomial{ok: true}
	out.coeff[0] = a.coeff[0] * b.coeff[0]
	out.coeff[1] = a.coeff[0]*b.coeff[1] + a.coeff[1]*b.coeff[0]
	out.coeff[2] = a.coeff[0]*b.coeff[2] + a.coeff[2]*b.coeff[0]
	out.coeff[3] = a.coeff[0]*b.coeff[3] + a.coeff[1]*b.coeff[1] + a.coeff[3]*b.coeff[0]
	out.coeff[4] = a.coeff[0]*b.coeff[4] + a.coeff[1]*b.coeff[2] + a.coeff[2]*b.coeff[1] + a.coeff[4]*b.coeff[0]
	out.coeff[5] = a.coeff[0]*b.coeff[5] + a.coeff[2]*b.coeff[2] + a.coeff[5]*b.coeff[0]
	return out, true
}

func markFastPrototypes(p *Prototype) bool {
	ok := !p.Vararg
	for _, child := range p.Children {
		if !markFastPrototypes(child) {
			ok = false
		}
	}
	for _, ins := range p.Code {
		switch ins.op() {
		case opMove, opLoadK, opLoadKX, opLoadNil, opLoadBool, opGetUp, opSetUp, opGetGlobal, opGetGlobalX, opSetGlobal, opSetGlobalX, opGetTable, opGetTableK, opGetArrayI, opGetFieldK, opSetTable, opSetTableK, opSetArrayI, opSetFieldK, opSwapTable, opAddTable, opNewTable, opSetListMulti, opAdd, opAddK, opSub, opSubK, opMul, opMulK, opDiv, opDivK, opMod, opModK, opPow, opPowK, opNeg, opNot, opLen, opConcat, opEq, opEqK, opNEK, opLT, opLTK, opGTK, opLE, opLEK, opGEK, opJumpCompareK, opJump, opBreak, opJumpFalse, opForPrep, opForLoop, opForLoopV, opClosure, opCloseUpvalues, opCall, opTailCall:
		case opReturn:
			if ins.b() == 255 || ins.b() > 4 {
				ok = false
			}
		default:
			ok = false
		}
	}
	p.Fast = ok
	return ok
}
func newCompiler(parent *compiler, source string, parameters []string, vararg bool) *compiler {
	c := &compiler{parent: parent, proto: &Prototype{Source: source, Parameters: uint8(len(parameters)), Vararg: vararg}, captured: make(map[int]bool)}
	for _, name := range parameters {
		c.addLocal(name)
	}
	if vararg {
		c.addLocal("arg")
	}
	return c
}
func (c *compiler) emit(i Instruction, line int) int {
	pc := len(c.proto.Code)
	c.proto.Code = append(c.proto.Code, i)
	c.proto.Lines = append(c.proto.Lines, line)
	live := c.free
	if live > math.MaxUint8 {
		live = math.MaxUint8
	}
	c.proto.LiveRegisters = append(c.proto.LiveRegisters, uint8(live))
	return pc
}
func (c *compiler) markErrorContext(pc int, expressions ...expression) {
	for _, expr := range expressions {
		if context := c.expressionContext(expr); context != "" {
			if c.proto.ErrorContexts == nil {
				c.proto.ErrorContexts = make(map[int][]string)
			}
			c.proto.ErrorContexts[pc] = append(c.proto.ErrorContexts[pc], context)
		}
	}
}
func (c *compiler) expressionContext(expr expression) string {
	switch value := expr.(type) {
	case *nameExpression:
		resolved := c.resolve(value.name)
		class := "global"
		if resolved.class == localVariable {
			class = "local"
		} else if resolved.class == upvalueVariable {
			class = "upvalue"
		}
		return fmt.Sprintf("%s '%s'", class, value.name)
	case *indexExpression:
		if literal, ok := value.key.(*literalExpression); ok && literal.value.kind == StringKind {
			return fmt.Sprintf("field '%s'", literal.value.StringValue())
		}
	}
	return ""
}
func (c *compiler) markCallErrorContext(pc int, call *callExpression) {
	if call.receiver != nil {
		if indexed, ok := call.function.(*indexExpression); ok {
			if literal, ok := indexed.key.(*literalExpression); ok && literal.value.kind == StringKind {
				if c.proto.ErrorContexts == nil {
					c.proto.ErrorContexts = make(map[int][]string)
				}
				c.proto.ErrorContexts[pc] = append(c.proto.ErrorContexts[pc], fmt.Sprintf("method '%s'", literal.value.StringValue()))
				return
			}
		}
	}
	c.markErrorContext(pc, call.function)
}
func (c *compiler) patch(pc, target int) {
	i := c.proto.Code[pc]
	off := target - (pc + 1)
	c.proto.Code[pc] = abx(i.op(), uint8(i.a()), uint16(int16(off)))
}
func (c *compiler) alloc() int {
	r := c.free
	c.free++
	if c.free > c.max {
		c.max = c.free
	}
	return r
}
func (c *compiler) allocN(n int) int {
	r := c.free
	c.free += n
	if c.free > c.max {
		c.max = c.free
	}
	return r
}
func (c *compiler) constant(v Value) int {
	for i, k := range c.proto.Constants {
		if equal(k, v) {
			return i
		}
	}
	c.proto.Constants = append(c.proto.Constants, v)
	return len(c.proto.Constants) - 1
}
func (c *compiler) loadConstant(target int, v Value, line int) error {
	i := c.constant(v)
	if i > math.MaxUint16 {
		pc := c.emit(abc(opLoadKX, uint8(target), 0, 0), line)
		if c.proto.ExtraConstants == nil {
			c.proto.ExtraConstants = make(map[int]int)
		}
		c.proto.ExtraConstants[pc] = i
		return nil
	}
	c.emit(abx(opLoadK, uint8(target), uint16(i)), line)
	return nil
}
func (c *compiler) error(line int, format string, args ...any) error {
	return &Error{Source: c.proto.Source, Line: line, Msg: fmt.Sprintf(format, args...)}
}
func (c *compiler) addLocal(name string) int {
	r := c.alloc()
	if c.proto.RegisterNames == nil {
		c.proto.RegisterNames = make(map[int]string)
	}
	c.proto.RegisterNames[r] = name
	c.locals = append(c.locals, localBinding{name: name, register: r, depth: c.depth, startPC: len(c.proto.Code)})
	return r
}
func (c *compiler) beginScope() { c.depth++ }
func (c *compiler) endScope() {
	closeFrom := -1
	for i := len(c.locals) - 1; i >= 0 && c.locals[i].depth == c.depth; i-- {
		if c.captured[c.locals[i].register] {
			closeFrom = c.locals[i].register
		}
	}
	if closeFrom >= 0 {
		c.emit(abc(opCloseUpvalues, uint8(closeFrom), 0, 0), 0)
	}
	for len(c.locals) > 0 && c.locals[len(c.locals)-1].depth == c.depth {
		local := c.locals[len(c.locals)-1]
		c.proto.LocalVariables = append(c.proto.LocalVariables, LocalVariableInfo{Name: local.name, Register: local.register, StartPC: local.startPC, EndPC: len(c.proto.Code)})
		c.free = local.register
		c.locals = c.locals[:len(c.locals)-1]
	}
	c.depth--
}
func (c *compiler) finalizeLocals() {
	for _, local := range c.locals {
		c.proto.LocalVariables = append(c.proto.LocalVariables, LocalVariableInfo{Name: local.name, Register: local.register, StartPC: local.startPC, EndPC: len(c.proto.Code)})
	}
}
func (c *compiler) closeCapturedFrom(register int, line int) {
	closeFrom := -1
	for captured := range c.captured {
		if captured >= register && (closeFrom < 0 || captured < closeFrom) {
			closeFrom = captured
		}
	}
	if closeFrom >= 0 {
		c.emit(abc(opCloseUpvalues, uint8(closeFrom), 0, 0), line)
	}
}
func (c *compiler) resolve(name string) resolvedVariable {
	for i := len(c.locals) - 1; i >= 0; i-- {
		if c.locals[i].name == name {
			return resolvedVariable{localVariable, c.locals[i].register, name}
		}
	}
	if c.parent != nil {
		parent := c.parent.resolve(name)
		if parent.class != globalVariable {
			for i, n := range c.upvalueNames {
				if n == name {
					return resolvedVariable{upvalueVariable, i, name}
				}
			}
			if parent.class == localVariable {
				c.parent.proto.Captured = true
				c.parent.captured[parent.index] = true
			}
			c.proto.Upvalues = append(c.proto.Upvalues, UpvalueDescriptor{uint8(parent.index), parent.class == localVariable})
			c.upvalueNames = append(c.upvalueNames, name)
			return resolvedVariable{upvalueVariable, len(c.proto.Upvalues) - 1, name}
		}
	}
	return resolvedVariable{class: globalVariable, name: name}
}
func (c *compiler) compileBlock(b *block) error {
	for _, s := range b.statements {
		mark := c.free
		if err := c.compileStatement(s); err != nil {
			return err
		}
		persistent := mark
		if len(c.locals) > 0 && c.locals[len(c.locals)-1].register+1 > persistent {
			persistent = c.locals[len(c.locals)-1].register + 1
		}
		if c.free > persistent {
			c.free = persistent
		}
	}
	return nil
}

func (c *compiler) compileStatement(stmt statement) error {
	switch n := stmt.(type) {
	case *localStatement:
		if len(n.names) == 1 && len(n.values) == 1 {
			if fn, ok := n.values[0].(*functionExpression); ok {
				r := c.addLocal(n.names[0])
				if err := c.compileTo(fn, r); err != nil {
					return err
				}
				return nil
			}
		}
		localBase := c.free
		values, err := c.compileValueList(n.values, len(n.names))
		if err != nil {
			return err
		}
		// RHS expressions are evaluated before the new locals enter scope.
		// Reuse their temporary registers as the local slots.
		c.free = localBase
		for i, name := range n.names {
			r := c.addLocal(name)
			if r != values[i] {
				c.emit(abc(opMove, uint8(r), uint8(values[i]), 0), n.line)
			}
		}
		return nil
	case *assignStatement:
		return c.compileAssignment(n)
	case *callStatement:
		_, err := c.compileCall(n.call, 0)
		return err
	case *returnStatement:
		if len(n.values) > 0 {
			last := n.values[len(n.values)-1]
			if call, ok := last.(*callExpression); ok {
				return c.compileTailReturn(n.line, n.values[:len(n.values)-1], func() (int, error) { return c.compileCall(call, 255) })
			}
			if _, ok := last.(*varargExpression); ok {
				return c.compileTailReturn(n.line, n.values[:len(n.values)-1], func() (int, error) {
					c.proto.UsesDots = true
					r := c.alloc()
					c.emit(abc(opVararg, uint8(r), 255, 0), n.line)
					return r, nil
				})
			}
		}
		values, err := c.compileValueList(n.values, len(n.values))
		if err != nil {
			return err
		}
		if len(values) == 0 {
			c.emit(abc(opReturn, 0, 0, 0), n.line)
			return nil
		}
		if len(values) == 1 {
			c.emit(abc(opReturn, uint8(values[0]), 1, 0), n.line)
			return nil
		}
		base := c.allocN(len(values))
		for i, r := range values {
			c.emit(abc(opMove, uint8(base+i), uint8(r), 0), n.line)
		}
		c.emit(abc(opReturn, uint8(base), uint8(len(values)), 0), n.line)
		return nil
	case *breakStatement:
		if len(c.loops) == 0 {
			return c.error(n.line, "break outside loop")
		}
		i := len(c.loops) - 1
		c.closeCapturedFrom(c.loops[i].closeFrom, n.line)
		pc := c.emit(abx(opBreak, 0, 0), n.line)
		c.loops[i].breaks = append(c.loops[i].breaks, pc)
		return nil
	case *doStatement:
		c.beginScope()
		err := c.compileBlock(n.body)
		c.endScope()
		return err
	case *whileStatement:
		return c.compileWhile(n)
	case *repeatStatement:
		return c.compileRepeat(n)
	case *ifStatement:
		return c.compileIf(n)
	case *numericForStatement:
		return c.compileNumericFor(n)
	case *genericForStatement:
		return c.compileGenericFor(n)
	default:
		return c.error(stmt.lineNumber(), "unsupported statement %T", stmt)
	}
}

type assignmentTarget struct {
	variable       *resolvedVariable
	table, key     int
	constantKey    int
	hasConstantKey bool
	hasFieldKey    bool
	arrayIndex     uint8
	hasArrayIndex  bool
}

func (c *compiler) prepareTarget(e expression) (assignmentTarget, error) {
	switch n := e.(type) {
	case *nameExpression:
		v := c.resolve(n.name)
		return assignmentTarget{variable: &v}, nil
	case *indexExpression:
		t := c.alloc()
		if err := c.compileTo(n.table, t); err != nil {
			return assignmentTarget{}, err
		}
		if literal, ok := n.key.(*literalExpression); ok {
			if index, ok := literalArrayIndex(literal); ok {
				return assignmentTarget{table: t, arrayIndex: index, hasArrayIndex: true}, nil
			}
			index := c.constant(literal.value)
			if index <= math.MaxUint8 {
				return assignmentTarget{table: t, constantKey: index, hasConstantKey: true, hasFieldKey: literal.value.kind == StringKind}, nil
			}
		}
		k := c.alloc()
		err := c.compileTo(n.key, k)
		return assignmentTarget{table: t, key: k}, err
	default:
		return assignmentTarget{}, c.error(e.lineNumber(), "invalid assignment target")
	}
}
func (c *compiler) compileAssignment(n *assignStatement) error {
	if table, first, second, ok := c.tableSwapRegisters(n); ok {
		c.emit(abc(opSwapTable, uint8(table), uint8(first), uint8(second)), n.line)
		return nil
	}
	if accumulator, table, key, ok := c.tableReductionRegisters(n); ok {
		c.emit(abc(opAddTable, uint8(accumulator), uint8(table), uint8(key)), n.line)
		return nil
	}
	if len(n.targets) == 1 && len(n.values) == 1 {
		if name, ok := n.targets[0].(*nameExpression); ok {
			variable := c.resolve(name.name)
			if variable.class == localVariable && directAssignmentExpression(n.values[0]) {
				return c.compileTo(n.values[0], variable.index)
			}
		}
	}
	targets := make([]assignmentTarget, len(n.targets))
	for i, e := range n.targets {
		t, err := c.prepareTarget(e)
		if err != nil {
			return err
		}
		targets[i] = t
	}
	values, err := c.compileValueList(n.values, len(targets))
	if err != nil {
		return err
	}
	snapshotValues := false
	for _, target := range targets {
		if target.variable == nil || target.variable.class != localVariable {
			continue
		}
		for _, value := range values {
			if target.variable.index == value {
				snapshotValues = true
				break
			}
		}
	}
	if snapshotValues {
		valueBase := c.allocN(len(values))
		for i, value := range values {
			c.emit(abc(opMove, uint8(valueBase+i), uint8(value), 0), n.line)
			values[i] = valueBase + i
		}
	}
	for i, t := range targets {
		if t.variable != nil {
			if err = c.storeVariable(*t.variable, values[i], n.line); err != nil {
				return err
			}
		} else {
			if t.hasArrayIndex {
				c.emit(abc(opSetArrayI, uint8(t.table), t.arrayIndex, uint8(values[i])), n.line)
			} else if t.hasConstantKey {
				op := opSetTableK
				if t.hasFieldKey {
					op = opSetFieldK
				}
				c.emit(abc(op, uint8(t.table), uint8(t.constantKey), uint8(values[i])), n.line)
			} else {
				c.emit(abc(opSetTable, uint8(t.table), uint8(t.key), uint8(values[i])), n.line)
			}
		}
	}
	return nil
}

func (c *compiler) tableReductionRegisters(n *assignStatement) (int, int, int, bool) {
	if len(n.targets) != 1 || len(n.values) != 1 {
		return 0, 0, 0, false
	}
	target, ok := n.targets[0].(*nameExpression)
	value, okValue := n.values[0].(*binaryExpression)
	if !ok || !okValue || value.operator != tPlus {
		return 0, 0, 0, false
	}
	left, leftOK := value.left.(*nameExpression)
	indexed, indexOK := value.right.(*indexExpression)
	if !leftOK || !indexOK || left.name != target.name {
		return 0, 0, 0, false
	}
	tableName, tableOK := indexed.table.(*nameExpression)
	keyName, keyOK := indexed.key.(*nameExpression)
	if !tableOK || !keyOK {
		return 0, 0, 0, false
	}
	accumulator, table, key := c.resolve(target.name), c.resolve(tableName.name), c.resolve(keyName.name)
	if accumulator.class != localVariable || table.class != localVariable || key.class != localVariable {
		return 0, 0, 0, false
	}
	return accumulator.index, table.index, key.index, true
}

// tableSwapRegisters recognizes a[i],a[j] = a[j],a[i]. Restricting every
// expression to an existing local preserves Lua's evaluation order because
// evaluating these table and key expressions has no side effects.
func (c *compiler) tableSwapRegisters(n *assignStatement) (int, int, int, bool) {
	if len(n.targets) != 2 || len(n.values) != 2 {
		return 0, 0, 0, false
	}
	t0, ok0 := n.targets[0].(*indexExpression)
	t1, ok1 := n.targets[1].(*indexExpression)
	v0, ok2 := n.values[0].(*indexExpression)
	v1, ok3 := n.values[1].(*indexExpression)
	if !ok0 || !ok1 || !ok2 || !ok3 {
		return 0, 0, 0, false
	}
	table0, a := t0.table.(*nameExpression)
	table1, b := t1.table.(*nameExpression)
	table2, d := v0.table.(*nameExpression)
	table3, e := v1.table.(*nameExpression)
	key0, f := t0.key.(*nameExpression)
	key1, g := t1.key.(*nameExpression)
	valueKey0, h := v0.key.(*nameExpression)
	valueKey1, i := v1.key.(*nameExpression)
	if !a || !b || !d || !e || !f || !g || !h || !i || table0.name != table1.name || table0.name != table2.name || table0.name != table3.name || key0.name != valueKey1.name || key1.name != valueKey0.name {
		return 0, 0, 0, false
	}
	tableVar, firstVar, secondVar := c.resolve(table0.name), c.resolve(key0.name), c.resolve(key1.name)
	if tableVar.class != localVariable || firstVar.class != localVariable || secondVar.class != localVariable {
		return 0, 0, 0, false
	}
	return tableVar.index, firstVar.index, secondVar.index, true
}

func directAssignmentExpression(expression expression) bool {
	switch value := expression.(type) {
	case *literalExpression, *nameExpression, *unaryExpression, *indexExpression:
		return true
	case *binaryExpression:
		return value.operator != tAnd && value.operator != tOr
	default:
		return false
	}
}

func literalArrayIndex(literal *literalExpression) (uint8, bool) {
	if literal.value.kind != NumberKind {
		return 0, false
	}
	n := literal.value.Number()
	i := int(n)
	return uint8(i), i >= 1 && i <= math.MaxUint8 && float64(i) == n
}
func (c *compiler) globalInstruction(op opcode, register int, name string, line int) error {
	i := c.constant(String(name))
	if i > math.MaxUint16 {
		extended := opGetGlobalX
		if op == opSetGlobal {
			extended = opSetGlobalX
		}
		pc := c.emit(abc(extended, uint8(register), 0, 0), line)
		if c.proto.ExtraConstants == nil {
			c.proto.ExtraConstants = make(map[int]int)
		}
		c.proto.ExtraConstants[pc] = i
		return nil
	}
	c.emit(abx(op, uint8(register), uint16(i)), line)
	return nil
}
func (c *compiler) loadVariable(v resolvedVariable, target, line int) error {
	switch v.class {
	case localVariable:
		if target != v.index {
			c.emit(abc(opMove, uint8(target), uint8(v.index), 0), line)
		}
	case upvalueVariable:
		c.emit(abc(opGetUp, uint8(target), uint8(v.index), 0), line)
	default:
		return c.globalInstruction(opGetGlobal, target, v.name, line)
	}
	return nil
}
func (c *compiler) storeVariable(v resolvedVariable, source, line int) error {
	switch v.class {
	case localVariable:
		if v.index != source {
			c.emit(abc(opMove, uint8(v.index), uint8(source), 0), line)
		}
	case upvalueVariable:
		c.emit(abc(opSetUp, uint8(source), uint8(v.index), 0), line)
	default:
		return c.globalInstruction(opSetGlobal, source, v.name, line)
	}
	return nil
}
func (c *compiler) compileValueList(values []expression, want int) ([]int, error) {
	result := make([]int, 0, want)
	for i, e := range values {
		remaining := want - len(result)
		if call, ok := e.(*callExpression); ok && i == len(values)-1 && remaining > 1 {
			base, err := c.compileCall(call, remaining)
			if err != nil {
				return nil, err
			}
			for j := 0; j < remaining; j++ {
				result = append(result, base+j)
			}
			break
		}
		if _, ok := e.(*varargExpression); ok && i == len(values)-1 && remaining > 1 {
			c.proto.UsesDots = true
			base := c.allocN(remaining)
			c.emit(abc(opVararg, uint8(base), uint8(remaining), 0), e.lineNumber())
			for j := 0; j < remaining; j++ {
				result = append(result, base+j)
			}
			break
		}
		r, err := c.compileExpression(e)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	for len(result) < want {
		r := c.alloc()
		c.emit(abc(opLoadNil, uint8(r), 0, 0), 0)
		result = append(result, r)
	}
	if len(result) > want {
		result = result[:want]
	}
	return result, nil
}
func (c *compiler) compileExpression(e expression) (int, error) {
	if name, ok := e.(*nameExpression); ok {
		resolved := c.resolve(name.name)
		if resolved.class == localVariable {
			return resolved.index, nil
		}
	}
	r := c.alloc()
	return r, c.compileTo(e, r)
}
func (c *compiler) compileTo(e expression, target int) error {
	switch n := e.(type) {
	case *literalExpression:
		switch n.value.kind {
		case NilKind:
			c.emit(abc(opLoadNil, uint8(target), 0, 0), n.line)
		case BoolKind:
			c.emit(abc(opLoadBool, uint8(target), boolByte(n.value.Bool()), 0), n.line)
		default:
			return c.loadConstant(target, n.value, n.line)
		}
		return nil
	case *nameExpression:
		return c.loadVariable(c.resolve(n.name), target, n.line)
	case *parenthesizedExpression:
		return c.compileTo(n.value, target)
	case *unaryExpression:
		v, err := c.compileExpression(n.value)
		if err != nil {
			return err
		}
		ops := map[tokenKind]opcode{tMinus: opNeg, tNot: opNot, tHash: opLen}
		pc := c.emit(abc(ops[n.operator], uint8(target), uint8(v), 0), n.line)
		c.markErrorContext(pc, n.value)
		return nil
	case *binaryExpression:
		return c.compileBinary(n, target)
	case *indexExpression:
		t, err := c.compileExpression(n.table)
		if err != nil {
			return err
		}
		if literal, ok := n.key.(*literalExpression); ok {
			if index, ok := literalArrayIndex(literal); ok {
				pc := c.emit(abc(opGetArrayI, uint8(target), uint8(t), index), n.line)
				c.markErrorContext(pc, n.table)
				return nil
			}
			index := c.constant(literal.value)
			if index <= math.MaxUint8 {
				op := opGetTableK
				if literal.value.kind == StringKind {
					op = opGetFieldK
				}
				pc := c.emit(abc(op, uint8(target), uint8(t), uint8(index)), n.line)
				c.markErrorContext(pc, n.table)
				return nil
			}
		}
		k, err := c.compileExpression(n.key)
		if err != nil {
			return err
		}
		pc := c.emit(abc(opGetTable, uint8(target), uint8(t), uint8(k)), n.line)
		c.markErrorContext(pc, n.table)
		return nil
	case *tableExpression:
		arrayHint, hashHint := 0, 0
		for _, field := range n.fields {
			if literal, ok := field.key.(*literalExpression); ok && literal.value.kind == NumberKind {
				arrayHint++
			} else {
				hashHint++
			}
		}
		if arrayHint > math.MaxUint8 {
			arrayHint = math.MaxUint8
		}
		if hashHint > math.MaxUint8 {
			hashHint = math.MaxUint8
		}
		c.emit(abc(opNewTable, uint8(target), uint8(arrayHint), uint8(hashHint)), n.line)
		for fieldIndex, f := range n.fields {
			fieldMark := c.free
			if f.list && fieldIndex == len(n.fields)-1 {
				start := int(f.key.(*literalExpression).value.Number())
				switch value := f.value.(type) {
				case *callExpression:
					if _, err := c.compileCall(value, 255); err != nil {
						return err
					}
					c.emit(abx(opSetListMulti, uint8(target), uint16(start)), n.line)
					c.free = fieldMark
					continue
				case *varargExpression:
					c.proto.UsesDots = true
					r := c.alloc()
					c.emit(abc(opVararg, uint8(r), 255, 0), value.line)
					c.emit(abx(opSetListMulti, uint8(target), uint16(start)), n.line)
					c.free = fieldMark
					continue
				}
			}
			if literal, ok := f.key.(*literalExpression); ok {
				if index, ok := literalArrayIndex(literal); ok {
					v, err := c.compileExpression(f.value)
					if err != nil {
						return err
					}
					c.emit(abc(opSetArrayI, uint8(target), index, uint8(v)), n.line)
					c.free = fieldMark
					continue
				}
				index := c.constant(literal.value)
				if index <= math.MaxUint8 {
					v, err := c.compileExpression(f.value)
					if err != nil {
						return err
					}
					op := opSetTableK
					if literal.value.kind == StringKind {
						op = opSetFieldK
					}
					c.emit(abc(op, uint8(target), uint8(index), uint8(v)), n.line)
					c.free = fieldMark
					continue
				}
			}
			k, err := c.compileExpression(f.key)
			if err != nil {
				return err
			}
			v, err := c.compileExpression(f.value)
			if err != nil {
				return err
			}
			c.emit(abc(opSetTable, uint8(target), uint8(k), uint8(v)), n.line)
			c.free = fieldMark
		}
		return nil
	case *functionExpression:
		child := newCompiler(c, c.proto.Source, n.parameters, n.vararg)
		if err := child.compileBlock(n.body); err != nil {
			return err
		}
		if len(child.proto.Code) == 0 || child.proto.Code[len(child.proto.Code)-1].op() != opReturn {
			child.emit(abc(opReturn, 0, 0, 0), n.line)
		}
		child.finalizeLocals()
		if child.max > 255 {
			return c.error(n.line, "function needs more than 255 registers")
		}
		child.proto.MaxRegisters = uint8(child.max)
		child.proto.DefinedLine = n.line
		child.proto.LastDefinedLine = n.lastLine
		child.proto.EndLine = n.lastLine
		child.proto.UpvalueNames = append([]string(nil), child.upvalueNames...)
		index := len(c.proto.Children)
		c.proto.Children = append(c.proto.Children, child.proto)
		c.emit(abx(opClosure, uint8(target), uint16(index)), n.line)
		return nil
	case *callExpression:
		base, err := c.compileCall(n, 1)
		if err == nil && base != target {
			c.emit(abc(opMove, uint8(target), uint8(base), 0), n.line)
		}
		return err
	case *varargExpression:
		c.proto.UsesDots = true
		c.emit(abc(opVararg, uint8(target), 1, 0), n.line)
		return nil
	default:
		return c.error(e.lineNumber(), "unsupported expression %T", e)
	}
}
func (c *compiler) compileBinary(n *binaryExpression, target int) error {
	left, err := c.compileExpression(n.left)
	if err != nil {
		return err
	}
	if n.operator == tAnd || n.operator == tOr {
		c.emit(abc(opMove, uint8(target), uint8(left), 0), n.line)
		if n.operator == tOr {
			not := c.alloc()
			c.emit(abc(opNot, uint8(not), uint8(left), 0), n.line)
			left = not
		}
		jump := c.emit(abx(opJumpFalse, uint8(left), 0), n.line)
		right, err := c.compileExpression(n.right)
		if err != nil {
			return err
		}
		c.emit(abc(opMove, uint8(target), uint8(right), 0), n.line)
		c.patch(jump, len(c.proto.Code))
		return nil
	}
	if literal, ok := n.right.(*literalExpression); ok {
		variants := map[tokenKind]opcode{
			tPlus: opAddK, tMinus: opSubK, tStar: opMulK, tSlash: opDivK,
			tPercent: opModK, tCaret: opPowK, tEqEq: opEqK, tNotEq: opNEK,
			tLT: opLTK, tLTE: opLEK, tGT: opGTK, tGTE: opGEK,
		}
		if op, exists := variants[n.operator]; exists {
			index := c.constant(literal.value)
			if index <= math.MaxUint8 {
				pc := c.emit(abc(op, uint8(target), uint8(left), uint8(index)), n.line)
				c.markErrorContext(pc, n.left)
				return nil
			}
		}
	}
	right, err := c.compileExpression(n.right)
	if err != nil {
		return err
	}
	ops := map[tokenKind]opcode{tPlus: opAdd, tMinus: opSub, tStar: opMul, tSlash: opDiv, tPercent: opMod, tCaret: opPow, tConcat: opConcat, tEqEq: opEq, tLT: opLT, tLTE: opLE}
	op, ok := ops[n.operator]
	if !ok {
		switch n.operator {
		case tNotEq:
			c.emit(abc(opEq, uint8(target), uint8(left), uint8(right)), n.line)
			c.emit(abc(opNot, uint8(target), uint8(target), 0), n.line)
			return nil
		case tGT:
			left, right = right, left
			op = opLT
		case tGTE:
			left, right = right, left
			op = opLE
		default:
			return c.error(n.line, "unsupported binary operator")
		}
	}
	pc := c.emit(abc(op, uint8(target), uint8(left), uint8(right)), n.line)
	c.markErrorContext(pc, n.left, n.right)
	return nil
}
func (c *compiler) compileCall(n *callExpression, want int) (int, error) {
	args := n.args
	var tail expression
	if len(args) > 0 {
		last := args[len(args)-1]
		switch last.(type) {
		case *callExpression, *varargExpression:
			tail = last
			args = args[:len(args)-1]
		}
	}
	count := 1 + len(args)
	if n.receiver != nil {
		count++
	}
	extra := maxInt(want-1, 0)
	if want == 255 {
		extra = 0
	}
	base := c.allocN(count + extra)
	nargs := 0
	if n.receiver != nil {
		if err := c.compileTo(n.receiver, base+1); err != nil {
			return 0, err
		}
		method := n.function.(*indexExpression)
		if literal, ok := method.key.(*literalExpression); ok {
			constant := c.constant(literal.value)
			if constant <= math.MaxUint8 {
				op := opGetTableK
				if literal.value.kind == StringKind {
					op = opGetFieldK
				}
				pc := c.emit(abc(op, uint8(base), uint8(base+1), uint8(constant)), n.line)
				c.markErrorContext(pc, n.receiver)
			} else {
				key := c.alloc()
				if err := c.loadConstant(key, literal.value, n.line); err != nil {
					return 0, err
				}
				pc := c.emit(abc(opGetTable, uint8(base), uint8(base+1), uint8(key)), n.line)
				c.markErrorContext(pc, n.receiver)
			}
		} else {
			key, err := c.compileExpression(method.key)
			if err != nil {
				return 0, err
			}
			pc := c.emit(abc(opGetTable, uint8(base), uint8(base+1), uint8(key)), n.line)
			c.markErrorContext(pc, n.receiver)
		}
		nargs++
	} else if err := c.compileTo(n.function, base); err != nil {
		return 0, err
	}
	for _, arg := range args {
		if err := c.compileTo(arg, base+1+nargs); err != nil {
			return 0, err
		}
		nargs++
	}
	if tail != nil {
		switch value := tail.(type) {
		case *callExpression:
			if _, err := c.compileCall(value, 255); err != nil {
				return 0, err
			}
		case *varargExpression:
			c.proto.UsesDots = true
			r := c.alloc()
			c.emit(abc(opVararg, uint8(r), 255, 0), value.line)
		}
		pc := c.emit(abc(opCallTail, uint8(base), uint8(nargs), uint8(want)), n.line)
		c.markCallErrorContext(pc, n)
	} else {
		pc := c.emit(abc(opCall, uint8(base), uint8(nargs), uint8(want)), n.line)
		c.markCallErrorContext(pc, n)
	}
	return base, nil
}

func (c *compiler) compileTailReturn(line int, prefix []expression, tail func() (int, error)) error {
	if len(prefix) == 0 {
		base, err := tail()
		if err != nil {
			return err
		}
		last := len(c.proto.Code) - 1
		if last >= 0 && c.proto.Code[last].op() == opCall {
			call := c.proto.Code[last]
			c.proto.Code[last] = abc(opTailCall, uint8(call.a()), uint8(call.b()), 0)
			return nil
		}
		c.emit(abc(opReturn, uint8(base), 255, 0), line)
		return nil
	}
	values, err := c.compileValueList(prefix, len(prefix))
	if err != nil {
		return err
	}
	base := c.allocN(len(values))
	for i, r := range values {
		c.emit(abc(opMove, uint8(base+i), uint8(r), 0), line)
	}
	tailBase, err := tail()
	if err != nil {
		return err
	}
	c.emit(abc(opReturnTail, uint8(base), uint8(len(values)), uint8(tailBase)), line)
	return nil
}
func (c *compiler) compileWhile(n *whileStatement) error {
	start := len(c.proto.Code)
	conditionMark := c.free
	exit, err := c.compileConditionJumpFalse(n.condition, n.condition.lineNumber())
	if err != nil {
		return err
	}
	c.free = conditionMark
	c.loops = append(c.loops, loopContext{closeFrom: c.free})
	c.beginScope()
	err = c.compileBlock(n.body)
	c.endScope()
	if err != nil {
		return err
	}
	c.emit(abx(opJump, 0, uint16(int16(start-(len(c.proto.Code)+1)))), n.line)
	end := len(c.proto.Code)
	c.patch(exit, end)
	c.finishLoop(end)
	return nil
}
func (c *compiler) compileRepeat(n *repeatStatement) error {
	start := len(c.proto.Code)
	c.loops = append(c.loops, loopContext{closeFrom: c.free})
	c.beginScope()
	if err := c.compileBlock(n.body); err != nil {
		return err
	}
	if literal, ok := n.condition.(*literalExpression); ok && literal.value.kind == BoolKind && !literal.value.Bool() {
		c.endScope()
		c.emit(abx(opJump, 0, uint16(int16(start-(len(c.proto.Code)+1)))), n.line)
		end := len(c.proto.Code)
		c.finishLoop(end)
		return nil
	}
	condition, err := c.compileExpression(n.condition)
	if err != nil {
		return err
	}
	c.endScope()
	exit := c.emit(abx(opJumpFalse, uint8(condition), 0), n.condition.lineNumber())
	c.patch(exit, start)
	end := len(c.proto.Code)
	c.finishLoop(end)
	return nil
}
func (c *compiler) compileIf(n *ifStatement) error {
	var exits []int
	for _, branch := range n.branches {
		next, err := c.compileConditionJumpFalse(branch.condition, branch.condition.lineNumber())
		if err != nil {
			return err
		}
		c.beginScope()
		if err = c.compileBlock(branch.body); err != nil {
			return err
		}
		c.endScope()
		exits = append(exits, c.emit(abx(opJump, 0, 0), n.line))
		c.patch(next, len(c.proto.Code))
	}
	if n.otherwise != nil {
		c.beginScope()
		if err := c.compileBlock(n.otherwise); err != nil {
			return err
		}
		c.endScope()
	}
	end := len(c.proto.Code)
	for _, pc := range exits {
		c.patch(pc, end)
	}
	return nil
}

func (c *compiler) compileConditionJumpFalse(condition expression, line int) (int, error) {
	if binary, ok := condition.(*binaryExpression); ok {
		if literal, ok := binary.right.(*literalExpression); ok {
			modes := map[tokenKind]uint8{tEqEq: compareEQ, tNotEq: compareNE, tLT: compareLT, tLTE: compareLE, tGT: compareGT, tGTE: compareGE}
			if mode, exists := modes[binary.operator]; exists {
				left, err := c.compileExpression(binary.left)
				if err != nil {
					return 0, err
				}
				index := c.constant(literal.value)
				if index <= math.MaxUint8 {
					c.emit(abc(opJumpCompareK, uint8(left), uint8(index), mode), line)
					return c.emit(abx(opJump, 0, 0), line), nil
				}
			}
		}
	}
	value, err := c.compileExpression(condition)
	if err != nil {
		return 0, err
	}
	return c.emit(abx(opJumpFalse, uint8(value), 0), line), nil
}
func (c *compiler) compileNumericFor(n *numericForStatement) error {
	initial, err := c.compileExpression(n.initial)
	if err != nil {
		return err
	}
	limit, err := c.compileExpression(n.limit)
	if err != nil {
		return err
	}
	step, err := c.compileExpression(n.step)
	if err != nil {
		return err
	}
	c.beginScope()
	base := c.allocN(3)
	c.emit(abc(opMove, uint8(base), uint8(initial), 0), n.line)
	c.emit(abc(opMove, uint8(base+1), uint8(limit), 0), n.line)
	c.emit(abc(opMove, uint8(base+2), uint8(step), 0), n.line)
	loopVar := c.addLocal(n.name)
	prep := c.emit(abx(opForPrep, uint8(base), 0), n.line)
	bodyStart := len(c.proto.Code)
	c.loops = append(c.loops, loopContext{closeFrom: loopVar})
	if err = c.compileBlock(n.body); err != nil {
		return err
	}
	c.closeCapturedFrom(loopVar, n.line)
	loopPC := c.emit(abc(opForLoopV, uint8(base), uint8(loopVar), 0), n.line)
	loopOffset := c.emit(abx(opJump, 0, 0), n.line)
	c.patch(loopOffset, bodyStart)
	c.patch(prep, loopPC)
	end := len(c.proto.Code)
	c.finishLoop(end)
	c.endScope()
	return nil
}
func (c *compiler) compileGenericFor(n *genericForStatement) error {
	values, err := c.compileValueList(n.values, 3)
	if err != nil {
		return err
	}
	c.beginScope()
	iterationLine := n.line
	if len(n.values) > 0 {
		iterationLine = n.values[0].lineNumber()
	}
	base := c.allocN(3)
	for i, r := range values {
		c.emit(abc(opMove, uint8(base+i), uint8(r), 0), iterationLine)
	}
	vars := make([]int, len(n.names))
	for i, name := range n.names {
		vars[i] = c.addLocal(name)
	}
	start := len(c.proto.Code)
	c.emit(abc(opTForCall, uint8(base), uint8(len(vars)), 0), iterationLine)
	exit := c.emit(abx(opJumpFalse, uint8(vars[0]), 0), n.line)
	c.loops = append(c.loops, loopContext{closeFrom: vars[0]})
	if err = c.compileBlock(n.body); err != nil {
		return err
	}
	c.closeCapturedFrom(vars[0], n.line)
	c.emit(abx(opJump, 0, uint16(int16(start-(len(c.proto.Code)+1)))), n.line)
	end := len(c.proto.Code)
	c.patch(exit, end)
	c.finishLoop(end)
	c.endScope()
	return nil
}
func (c *compiler) finishLoop(end int) {
	i := len(c.loops) - 1
	for _, pc := range c.loops[i].breaks {
		c.patch(pc, end)
	}
	c.loops = c.loops[:i]
}
func boolByte(v bool) uint8 {
	if v {
		return 1
	}
	return 0
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
