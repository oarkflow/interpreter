package lua

import (
	"fmt"
	"math"
)

func (s *State) Load(source, name string) (Value, error) {
	proto, err := Compile(source, name)
	if err != nil {
		return Nil, err
	}
	return FunctionValue(&Function{Proto: proto, Env: s.globals}), nil
}

func (s *State) DoString(source string) ([]Value, error) {
	fn, err := s.Load(source, "=(string)")
	if err != nil {
		return nil, err
	}
	return s.Call(fn)
}

func (s *State) GetGlobal(name string) Value        { return s.globals.GetString(name) }
func (s *State) SetGlobal(name string, value Value) { s.globals.SetString(name, value) }

func (s *State) Call(function Value, args ...Value) ([]Value, error) {
	if function.kind != FunctionKind {
		return nil, runtimeError("attempt to call a %s value", function.TypeName())
	}
	result, err := s.dispatch(function.Function(), args)
	if err != nil {
		return nil, err
	}
	values := result.slice()
	for _, v := range values {
		if v.kind == TableKind {
			s.escapeValue(v, map[*Table]bool{})
		}
	}
	return values, nil
}

// CallInto invokes a Lua function without allocating a result slice. It
// returns the total result count and copies as many values as fit in results.
// Callers that need every result should provide sufficient destination space.
func (s *State) CallInto(function Value, args, results []Value) (int, error) {
	if function.kind != FunctionKind {
		return 0, runtimeError("attempt to call a %s value", function.TypeName())
	}
	fn := function.Function()
	if len(args) == 2 && len(results) > 0 && args[0].kind == NumberKind && args[1].kind == NumberKind {
		if count, ok := numericLeafResults(fn, args[0].Number(), args[1].Number(), results); ok {
			return count, nil
		}
		if value, ok := numericLeafValue(fn, args[0].Number(), args[1].Number()); ok {
			results[0] = Number(value)
			return 1, nil
		}
	}
	result, err := s.dispatch(fn, args)
	if err != nil {
		return 0, err
	}
	for i := 0; i < result.count && i < len(results); i++ {
		value := result.at(i)
		results[i] = value
		if value.kind == TableKind {
			s.escapeValue(value, map[*Table]bool{})
		}
	}
	return result.count, nil
}

func numericLeafResults(fn *Function, x, y float64, results []Value) (int, bool) {
	if fn.Native != nil || fn.NativeNumber1 != nil || fn.NativeNumber2 != nil || fn.Proto == nil || fn.Proto.Parameters != 2 {
		return 0, false
	}
	code := fn.Proto.Code
	if len(code) != 5 || code[2].op() != opMove || code[3].op() != opMove || code[4].op() != opReturn || code[4].b() != 2 || code[2].b() != code[0].a() || code[3].b() != code[1].a() || code[4].a() != code[2].a() || code[3].a() != code[2].a()+1 {
		return 0, false
	}
	evaluate := func(ins Instruction) (float64, bool) {
		if ins.b() > 1 || ins.c() > 1 {
			return 0, false
		}
		a, b := x, y
		if ins.b() == 1 {
			a = y
		}
		if ins.c() == 0 {
			b = x
		}
		switch ins.op() {
		case opAdd:
			return a + b, true
		case opSub:
			return a - b, true
		case opMul:
			return a * b, true
		case opDiv:
			return a / b, true
		default:
			return 0, false
		}
	}
	first, ok := evaluate(code[0])
	if !ok {
		return 0, false
	}
	second, ok := evaluate(code[1])
	if !ok {
		return 0, false
	}
	results[0] = Number(first)
	if len(results) > 1 {
		results[1] = Number(second)
	}
	return 2, true
}

func numericLeafValue(fn *Function, a, b float64) (float64, bool) {
	if fn.Native != nil || fn.NativeNumber1 != nil || fn.NativeNumber2 != nil || fn.Proto == nil || fn.Proto.Parameters != 2 || len(fn.Proto.Code) != 2 {
		return 0, false
	}
	op, ret := fn.Proto.Code[0], fn.Proto.Code[1]
	if ret.op() != opReturn || ret.b() != 1 || ret.a() != op.a() || op.b() >= 2 || op.c() >= 2 {
		return 0, false
	}
	x, y := a, b
	if op.b() == 1 {
		x = b
	}
	if op.c() == 0 {
		y = a
	}
	switch op.op() {
	case opAdd:
		return x + y, true
	case opSub:
		return x - y, true
	case opMul:
		return x * y, true
	case opDiv:
		return x / y, true
	case opMod:
		return x - math.Floor(x/y)*y, true
	case opPow:
		return math.Pow(x, y), true
	default:
		return 0, false
	}
}

// CallNumber2 is the allocation-free embedding path for the common numeric
// binary-function case measured by the embedding benchmark.
func (s *State) CallNumber2(function Value, a, b float64) (float64, error) {
	if function.kind != FunctionKind {
		return 0, runtimeError("attempt to call a %s value", function.TypeName())
	}
	fn := function.Function()
	// Numeric leaf functions are common at embedding boundaries. The compiler
	// emits one arithmetic instruction followed by RETURN for these functions;
	// execute that verified superinstruction without constructing a VM frame.
	if value, ok := numericLeafValue(fn, a, b); ok {
		return value, nil
	}
	s.scratchArgs[0], s.scratchArgs[1] = Number(a), Number(b)
	result, err := s.dispatch(fn, s.scratchArgs[:2])
	if err != nil {
		return 0, err
	}
	if result.count == 0 || result.at(0).kind != NumberKind {
		return 0, runtimeError("function did not return a number")
	}
	return result.at(0).Number(), nil
}

type callResult struct {
	inline [4]Value
	extra  []Value
	count  int
}

func (r callResult) at(i int) Value {
	if i < 4 {
		return r.inline[i]
	}
	return r.extra[i-4]
}
func (r *callResult) set(i int, v Value) {
	if i < 4 {
		r.inline[i] = v
	} else {
		r.extra[i-4] = v
	}
}
func (r callResult) slice() []Value {
	out := make([]Value, r.count)
	for i := range out {
		out[i] = r.at(i)
	}
	return out
}
func resultFromSlice(values []Value) callResult {
	r := callResult{count: len(values)}
	for i, v := range values {
		if i < 4 {
			r.inline[i] = v
		} else {
			if r.extra == nil {
				r.extra = make([]Value, len(values)-4)
			}
			r.extra[i-4] = v
		}
	}
	return r
}

func (s *State) dispatch(fn *Function, args []Value) (callResult, error) {
	if fn.NativeNumber1 != nil {
		if len(args) < 1 {
			return callResult{}, runtimeError("number expected")
		}
		n, ok := toNumber(args[0])
		if !ok {
			return callResult{}, runtimeError("number expected")
		}
		return callResult{inline: [4]Value{Number(fn.NativeNumber1(n))}, count: 1}, nil
	}
	if fn.NativeNumber2 != nil {
		if len(args) < 2 {
			return callResult{}, runtimeError("two numbers expected")
		}
		a, aok := toNumber(args[0])
		b, bok := toNumber(args[1])
		if !aok || !bok {
			return callResult{}, runtimeError("number expected")
		}
		return callResult{inline: [4]Value{Number(fn.NativeNumber2(a, b))}, count: 1}, nil
	}
	if fn.Native != nil {
		values, err := fn.Native(s, args)
		return resultFromSlice(values), err
	}
	if fn.Proto != nil && fn.Proto.Fast {
		return s.callLuaFast(fn, args)
	}
	return s.callLua(fn, args)
}

func (s *State) callLua(fn *Function, args []Value) (callResult, error) {
	if fn.Proto == nil {
		return callResult{}, runtimeError("invalid function")
	}
	p := fn.Proto
	size := int(p.MaxRegisters)
	if size < int(p.Parameters) {
		size = int(p.Parameters)
	}
	var regs []cell
	stackBase := s.top
	if p.Captured {
		regs = make([]cell, size)
	} else {
		if stackBase+size > len(s.stack) {
			return callResult{}, runtimeError("Lua stack overflow")
		}
		regs = s.stack[stackBase : stackBase+size]
		s.top += size
		for i := range regs {
			regs[i].value = Nil
		}
		defer func() {
			for i := range regs {
				regs[i].value = Nil
			}
			s.top = stackBase
		}()
	}
	for i := 0; i < len(args) && i < int(p.Parameters); i++ {
		regs[i].value = args[i]
	}
	var varargs []Value
	if len(args) > int(p.Parameters) {
		varargs = args[p.Parameters:]
	}
	f := frame{fn: fn, regs: regs, varargs: varargs}
	s.frames = append(s.frames, f)
	defer func() { s.frames = s.frames[:len(s.frames)-1] }()
	for f.pc < len(p.Code) {
		pc := f.pc
		ins := p.Code[pc]
		f.pc++
		a, b, c := ins.a(), ins.b(), ins.c()
		switch ins.op() {
		case opMove:
			regs[a].value = regs[b].value
		case opLoadK:
			regs[a].value = p.Constants[ins.bx()]
		case opLoadNil:
			regs[a].value = Nil
		case opLoadBool:
			regs[a].value = Bool(b != 0)
		case opGetUp:
			regs[a].value = fn.Up[b].value
		case opSetUp:
			fn.Up[b].value = regs[a].value
		case opGetGlobal:
			env := fn.Env
			if env == nil {
				env = s.globals
			}
			regs[a].value = env.GetString(p.Constants[ins.bx()].StringValue())
		case opSetGlobal:
			env := fn.Env
			if env == nil {
				env = s.globals
			}
			env.SetString(p.Constants[ins.bx()].StringValue(), regs[a].value)
		case opGetTable:
			t := regs[b].value
			key := regs[c].value
			if t.kind == TableKind && t.Table().meta == nil && key.kind == NumberKind {
				if value, ok := t.Table().getDenseNumber(key.Number()); ok {
					regs[a].value = value
					continue
				}
			}
			value, err := s.index(t, key, 0)
			if err != nil {
				return callResult{}, s.vmWrap(p, pc, err)
			}
			regs[a].value = value
		case opGetTableK:
			t, key := regs[b].value, p.Constants[c]
			if t.kind == TableKind && t.Table().meta == nil && key.kind == StringKind {
				table, cache := t.Table(), &p.FieldCaches[pc]
				if table.shape != nil && cache.shape == table.shape {
					regs[a].value = table.fields[cache.slot].value
					continue
				}
				if slot, ok := s.cachedField(p, pc, t.Table(), key.StringValue()); ok {
					regs[a].value = t.Table().fields[slot].value
					continue
				}
			}
			if t.kind == TableKind && t.Table().meta == nil && key.kind == NumberKind {
				if value, ok := t.Table().getDenseNumber(key.Number()); ok {
					regs[a].value = value
					continue
				}
			}
			value, err := s.index(t, key, 0)
			if err != nil {
				return callResult{}, s.vmWrap(p, pc, err)
			}
			regs[a].value = value
		case opGetArrayI:
			t := regs[b].value
			if t.kind == TableKind && t.Table().meta == nil && c <= len(t.Table().array) {
				regs[a].value = t.Table().array[c-1]
			} else {
				value, err := s.index(t, Number(float64(c)), 0)
				if err != nil {
					return callResult{}, s.vmWrap(p, pc, err)
				}
				regs[a].value = value
			}
		case opGetFieldK:
			t, name := regs[b].value, p.Constants[c].StringValue()
			if t.kind == TableKind && t.Table().meta == nil {
				table, cache := t.Table(), &p.FieldCaches[pc]
				if table.shape != nil && cache.shape == table.shape {
					regs[a].value = table.fields[cache.slot].value
					continue
				}
				if slot, ok := s.cachedField(p, pc, table, name); ok {
					regs[a].value = table.fields[slot].value
					continue
				}
				regs[a].value = table.GetString(name)
			} else {
				value, err := s.index(t, p.Constants[c], 0)
				if err != nil {
					return callResult{}, s.vmWrap(p, pc, err)
				}
				regs[a].value = value
			}
		case opSetTable:
			t := regs[a].value
			key, value := regs[b].value, regs[c].value
			if t.kind == TableKind && t.Table().meta == nil && key.kind == NumberKind && t.Table().setDenseNumber(key.Number(), value) {
				continue
			}
			if err := s.assignIndex(t, key, value, 0); err != nil {
				return callResult{}, s.vmWrap(p, pc, err)
			}
		case opSwapTable:
			if err := s.swapTable(regs[a].value, regs[b].value, regs[c].value); err != nil {
				return callResult{}, s.vmWrap(p, pc, err)
			}
		case opAddTable:
			if err := s.addTableValue(&regs[a].value, regs[b].value, regs[c].value); err != nil {
				return callResult{}, s.vmWrap(p, pc, err)
			}
		case opSetTableK:
			t, key, value := regs[a].value, p.Constants[b], regs[c].value
			if t.kind == TableKind && t.Table().meta == nil && key.kind == StringKind {
				table, cache := t.Table(), &p.FieldCaches[pc]
				if table.shape != nil && cache.shape == table.shape {
					table.fields[cache.slot].value = value
					continue
				}
				if slot, ok := s.cachedField(p, pc, t.Table(), key.StringValue()); ok {
					t.Table().fields[slot].value = value
					continue
				}
			}
			if t.kind == TableKind && t.Table().meta == nil && key.kind == NumberKind && t.Table().setDenseNumber(key.Number(), value) {
				continue
			}
			if err := s.assignIndex(t, key, value, 0); err != nil {
				return callResult{}, s.vmWrap(p, pc, err)
			}
		case opSetArrayI:
			t, value := regs[a].value, regs[c].value
			if t.kind == TableKind && t.Table().meta == nil {
				table := t.Table()
				if b <= len(table.array) {
					table.array[b-1] = value
					continue
				}
				if table.appendDenseNumber(b, value) {
					continue
				}
			}
			if err := s.assignIndex(t, Number(float64(b)), value, 0); err != nil {
				return callResult{}, s.vmWrap(p, pc, err)
			}
		case opSetFieldK:
			t, value, name := regs[a].value, regs[c].value, p.Constants[b].StringValue()
			if t.kind == TableKind && t.Table().meta == nil {
				table, cache := t.Table(), &p.FieldCaches[pc]
				if table.shape != nil && cache.shape == table.shape {
					table.fields[cache.slot].value = value
					continue
				}
				if slot, ok := s.cachedField(p, pc, table, name); ok {
					table.fields[slot].value = value
					continue
				}
				table.SetString(name, value)
				continue
			}
			if err := s.assignIndex(t, p.Constants[b], value, 0); err != nil {
				return callResult{}, s.vmWrap(p, pc, err)
			}
		case opNewTable:
			regs[a].value = TableValue(s.newTable(b, c))
		case opAdd, opSub, opMul, opDiv, opMod, opPow:
			left, right := regs[b].value, regs[c].value
			x, y := left.Number(), right.Number()
			xok, yok := left.kind == NumberKind, right.kind == NumberKind
			if !xok {
				x, xok = toNumber(left)
			}
			if !yok {
				y, yok = toNumber(right)
			}
			if !xok || !yok {
				name := map[opcode]string{opAdd: "__add", opSub: "__sub", opMul: "__mul", opDiv: "__div", opMod: "__mod", opPow: "__pow"}[ins.op()]
				value, ok, err := s.binaryMetamethod(regs[b].value, regs[c].value, name)
				if err != nil {
					return callResult{}, err
				}
				if !ok {
					return callResult{}, s.vmError(p, pc, "attempt to perform arithmetic on a %s value", badNumericType(regs[b].value, regs[c].value))
				}
				regs[a].value = value
				continue
			}
			switch ins.op() {
			case opAdd:
				x += y
			case opSub:
				x -= y
			case opMul:
				x *= y
			case opDiv:
				x /= y
			case opMod:
				x = x - math.Floor(x/y)*y
			case opPow:
				x = math.Pow(x, y)
			}
			regs[a].value = Number(x)
		case opAddK, opSubK, opMulK, opDivK, opModK, opPowK:
			left, right := regs[b].value, p.Constants[c]
			x, y := left.Number(), right.Number()
			xok, yok := left.kind == NumberKind, right.kind == NumberKind
			if !xok {
				x, xok = toNumber(left)
			}
			if !yok {
				y, yok = toNumber(right)
			}
			regular := regularConstantOpcode(ins.op())
			if !xok || !yok {
				value, ok, err := s.binaryMetamethod(left, right, arithmeticMetaName(regular))
				if err != nil {
					return callResult{}, err
				}
				if !ok {
					return callResult{}, s.vmError(p, pc, "attempt to perform arithmetic on a %s value", badNumericType(left, right))
				}
				regs[a].value = value
				continue
			}
			switch regular {
			case opAdd:
				x += y
			case opSub:
				x -= y
			case opMul:
				x *= y
			case opDiv:
				x /= y
			case opMod:
				x = x - math.Floor(x/y)*y
			case opPow:
				x = math.Pow(x, y)
			}
			regs[a].value = Number(x)
		case opNeg:
			x, ok := toNumber(regs[b].value)
			if !ok {
				return callResult{}, s.vmError(p, pc, "attempt to perform arithmetic on a %s value", regs[b].value.TypeName())
			}
			regs[a].value = Number(-x)
		case opNot:
			regs[a].value = Bool(!regs[b].value.Truthy())
		case opLen:
			v := regs[b].value
			if value, ok, err := s.unaryMetamethod(v, "__len"); err != nil {
				return callResult{}, err
			} else if ok {
				regs[a].value = value
				continue
			}
			switch v.kind {
			case StringKind:
				regs[a].value = Number(float64(len(v.StringValue())))
			case TableKind:
				regs[a].value = Number(float64(v.Table().Len()))
			default:
				return callResult{}, s.vmError(p, pc, "attempt to get length of a %s value", v.TypeName())
			}
		case opConcat:
			x, xok := stringCoerce(regs[b].value)
			y, yok := stringCoerce(regs[c].value)
			if !xok || !yok {
				value, ok, err := s.binaryMetamethod(regs[b].value, regs[c].value, "__concat")
				if err != nil {
					return callResult{}, err
				}
				if !ok {
					return callResult{}, s.vmError(p, pc, "attempt to concatenate a %s value", badStringType(regs[b].value, regs[c].value))
				}
				regs[a].value = value
				continue
			}
			regs[a].value = String(x + y)
		case opEq:
			left, right := regs[b].value, regs[c].value
			if equal(left, right) {
				regs[a].value = True
			} else if value, ok, err := s.binaryMetamethod(left, right, "__eq"); err != nil {
				return callResult{}, err
			} else if ok {
				regs[a].value = Bool(value.Truthy())
			} else {
				regs[a].value = False
			}
		case opEqK:
			left, right := regs[b].value, p.Constants[c]
			if equal(left, right) {
				regs[a].value = True
			} else if value, ok, err := s.binaryMetamethod(left, right, "__eq"); err != nil {
				return callResult{}, err
			} else if ok {
				regs[a].value = Bool(value.Truthy())
			} else {
				regs[a].value = False
			}
		case opNEK:
			left, right := regs[b].value, p.Constants[c]
			if equal(left, right) {
				regs[a].value = False
			} else if value, ok, err := s.binaryMetamethod(left, right, "__eq"); err != nil {
				return callResult{}, err
			} else if ok {
				regs[a].value = Bool(!value.Truthy())
			} else {
				regs[a].value = True
			}
		case opLT, opLE:
			x, y := regs[b].value, regs[c].value
			var result bool
			if x.kind == NumberKind && y.kind == NumberKind {
				if ins.op() == opLT {
					result = x.Number() < y.Number()
				} else {
					result = x.Number() <= y.Number()
				}
			} else if x.kind == StringKind && y.kind == StringKind {
				if ins.op() == opLT {
					result = x.StringValue() < y.StringValue()
				} else {
					result = x.StringValue() <= y.StringValue()
				}
			} else {
				name := "__lt"
				if ins.op() == opLE {
					name = "__le"
				}
				value, ok, err := s.binaryMetamethod(x, y, name)
				if err != nil {
					return callResult{}, err
				}
				if !ok {
					return callResult{}, s.vmError(p, pc, "attempt to compare %s with %s", x.TypeName(), y.TypeName())
				}
				result = value.Truthy()
			}
			regs[a].value = Bool(result)
		case opLTK, opLEK, opGTK, opGEK:
			x, y := regs[b].value, p.Constants[c]
			var result bool
			if x.kind == NumberKind && y.kind == NumberKind {
				switch ins.op() {
				case opLTK:
					result = x.Number() < y.Number()
				case opLEK:
					result = x.Number() <= y.Number()
				case opGTK:
					result = x.Number() > y.Number()
				case opGEK:
					result = x.Number() >= y.Number()
				}
			} else if x.kind == StringKind && y.kind == StringKind {
				switch ins.op() {
				case opLTK:
					result = x.StringValue() < y.StringValue()
				case opLEK:
					result = x.StringValue() <= y.StringValue()
				case opGTK:
					result = x.StringValue() > y.StringValue()
				case opGEK:
					result = x.StringValue() >= y.StringValue()
				}
			} else {
				name, left, right := "__lt", x, y
				if ins.op() == opLEK {
					name = "__le"
				} else if ins.op() == opGTK {
					left, right = y, x
				} else if ins.op() == opGEK {
					name = "__le"
					left, right = y, x
				}
				value, ok, err := s.binaryMetamethod(left, right, name)
				if err != nil {
					return callResult{}, err
				}
				if !ok {
					return callResult{}, s.vmError(p, pc, "attempt to compare %s with %s", x.TypeName(), y.TypeName())
				}
				result = value.Truthy()
			}
			regs[a].value = Bool(result)
		case opJumpCompareK:
			left, right, mode := regs[a].value, p.Constants[b], uint8(c)
			var condition bool
			if left.kind == NumberKind && right.kind == NumberKind {
				x, y := left.Number(), right.Number()
				switch mode {
				case compareEQ:
					condition = x == y
				case compareNE:
					condition = x != y
				case compareLT:
					condition = x < y
				case compareLE:
					condition = x <= y
				case compareGT:
					condition = x > y
				case compareGE:
					condition = x >= y
				}
			} else if left.kind == StringKind && right.kind == StringKind {
				x, y := left.StringValue(), right.StringValue()
				switch mode {
				case compareEQ:
					condition = x == y
				case compareNE:
					condition = x != y
				case compareLT:
					condition = x < y
				case compareLE:
					condition = x <= y
				case compareGT:
					condition = x > y
				case compareGE:
					condition = x >= y
				}
			} else {
				name, x, y := "__eq", left, right
				switch mode {
				case compareLT:
					name = "__lt"
				case compareLE:
					name = "__le"
				case compareGT:
					name, x, y = "__lt", right, left
				case compareGE:
					name, x, y = "__le", right, left
				}
				if (mode == compareEQ || mode == compareNE) && equal(left, right) {
					condition = mode == compareEQ
				} else if value, ok, err := s.binaryMetamethod(x, y, name); err != nil {
					return callResult{}, err
				} else if ok {
					condition = value.Truthy()
					if mode == compareNE {
						condition = !condition
					}
				} else if mode == compareEQ || mode == compareNE {
					condition = mode == compareNE
				} else {
					return callResult{}, s.vmError(p, pc, "attempt to compare %s with %s", left.TypeName(), right.TypeName())
				}
			}
			offset := p.Code[f.pc].sbx()
			f.pc++
			if !condition {
				f.pc += offset
			}
		case opJump:
			f.pc += ins.sbx()
		case opJumpFalse:
			if !regs[a].value.Truthy() {
				f.pc += ins.sbx()
			}
		case opForPrep:
			initial, iok := toNumber(regs[a].value)
			limit, lok := toNumber(regs[a+1].value)
			step, sok := toNumber(regs[a+2].value)
			if !iok || !lok || !sok {
				return callResult{}, s.vmError(p, pc, "numeric for values must be numbers")
			}
			regs[a].value = Number(initial - step)
			regs[a+1].value = Number(limit)
			regs[a+2].value = Number(step)
			f.pc += ins.sbx()
		case opForLoop:
			index := regs[a].value.Number() + regs[a+2].value.Number()
			limit, step := regs[a+1].value.Number(), regs[a+2].value.Number()
			regs[a].value = Number(index)
			if step > 0 && index <= limit || step <= 0 && index >= limit {
				f.pc += ins.sbx()
			}
		case opForLoopV:
			index := regs[a].value.Number() + regs[a+2].value.Number()
			limit, step := regs[a+1].value.Number(), regs[a+2].value.Number()
			indexValue := Number(index)
			regs[a].value = indexValue
			offset := p.Code[f.pc].sbx()
			f.pc++
			if step > 0 && index <= limit || step <= 0 && index >= limit {
				regs[b].value = indexValue
				f.pc += offset
			}
		case opTForCall:
			callee := regs[a].value
			if callee.kind != FunctionKind {
				return callResult{}, s.vmError(p, pc, "attempt to call a %s value", callee.TypeName())
			}
			s.scratchArgs[0], s.scratchArgs[1] = regs[a+1].value, regs[a+2].value
			results, err := s.dispatch(callee.Function(), s.scratchArgs[:2])
			if err != nil {
				return callResult{}, err
			}
			for i := 0; i < b; i++ {
				if i < results.count {
					regs[a+3+i].value = results.at(i)
				} else {
					regs[a+3+i].value = Nil
				}
			}
			regs[a+2].value = regs[a+3].value
		case opVararg:
			if b == 255 {
				f.multi = resultFromSlice(f.varargs)
				continue
			}
			for i := 0; i < b; i++ {
				if i < len(f.varargs) {
					regs[a+i].value = f.varargs[i]
				} else {
					regs[a+i].value = Nil
				}
			}
		case opClosure:
			child := p.Children[ins.bx()]
			closure := &Function{Proto: child, Env: fn.Env, Up: make([]*cell, len(child.Upvalues))}
			for i, up := range child.Upvalues {
				if up.Local {
					closure.Up[i] = &regs[up.Index]
				} else {
					closure.Up[i] = fn.Up[up.Index]
				}
			}
			regs[a].value = FunctionValue(closure)
		case opCall:
			callee := regs[a].value
			if c >= 1 && c != 255 && b >= 2 && callee.kind == FunctionKind && callee.Function().NativeNumber2 != nil && regs[a+1].value.kind == NumberKind && regs[a+2].value.kind == NumberKind {
				regs[a].value = Number(callee.Function().NativeNumber2(regs[a+1].value.Number(), regs[a+2].value.Number()))
				for i := 1; i < c; i++ {
					regs[a+i].value = Nil
				}
				continue
			}
			if c >= 1 && c != 255 && b >= 1 && callee.kind == FunctionKind && callee.Function().NativeNumber1 != nil && regs[a+1].value.kind == NumberKind {
				regs[a].value = Number(callee.Function().NativeNumber1(regs[a+1].value.Number()))
				for i := 1; i < c; i++ {
					regs[a+i].value = Nil
				}
				continue
			}
			if c == 1 && callee.kind == FunctionKind && callee.Function().Proto != nil && callee.Function().Proto.NumericPure {
				if value, ok := callNumericPure(callee.Function().Proto, regs, a+1, b); ok {
					regs[a].value = Number(value)
					continue
				}
			}
			if s.argTop+b > len(s.callArgs) {
				return callResult{}, s.vmError(p, pc, "Lua argument stack overflow")
			}
			argBase := s.argTop
			s.argTop += b
			argv := s.callArgs[argBase:s.argTop]
			for i := range argv {
				argv[i] = regs[a+1+i].value
			}
			results, err := s.callValue(callee, argv)
			for i := range argv {
				argv[i] = Nil
			}
			s.argTop = argBase
			if err != nil {
				return callResult{}, err
			}
			if c == 255 {
				f.multi = results
				continue
			}
			for i := 0; i < c; i++ {
				if i < results.count {
					regs[a+i].value = results.at(i)
				} else {
					regs[a+i].value = Nil
				}
			}
		case opCallTail:
			callee := regs[a].value
			argv := make([]Value, b+f.multi.count)
			for i := 0; i < b; i++ {
				argv[i] = regs[a+1+i].value
			}
			for i := 0; i < f.multi.count; i++ {
				argv[b+i] = f.multi.at(i)
			}
			results, err := s.callValue(callee, argv)
			if err != nil {
				return callResult{}, err
			}
			if c == 255 {
				f.multi = results
				continue
			}
			for i := 0; i < c; i++ {
				if i < results.count {
					regs[a+i].value = results.at(i)
				} else {
					regs[a+i].value = Nil
				}
			}
		case opReturn:
			if b == 255 {
				return f.multi, nil
			}
			results := callResult{count: b}
			if b > 4 {
				results.extra = make([]Value, b-4)
			}
			for i := 0; i < b; i++ {
				if i < 4 {
					results.inline[i] = regs[a+i].value
				} else {
					results.extra[i-4] = regs[a+i].value
				}
			}
			return results, nil
		case opReturnTail:
			result := callResult{count: b + f.multi.count}
			if result.count > 4 {
				result.extra = make([]Value, result.count-4)
			}
			for i := 0; i < b; i++ {
				result.set(i, regs[a+i].value)
			}
			for i := 0; i < f.multi.count; i++ {
				result.set(b+i, f.multi.at(i))
			}
			return result, nil
		default:
			return callResult{}, s.vmError(p, pc, "invalid opcode %d", ins.op())
		}
	}
	return callResult{}, nil
}

func (s *State) vmError(p *Prototype, pc int, format string, args ...any) error {
	line := 0
	if pc < len(p.Lines) {
		line = p.Lines[pc]
	}
	return &Error{Source: p.Source, Line: line, Msg: fmt.Sprintf(format, args...)}
}
func (s *State) vmWrap(p *Prototype, pc int, err error) error {
	if _, ok := err.(*Error); ok {
		return s.vmError(p, pc, "%s", err)
	}
	return err
}
func stringCoerce(v Value) (string, bool) {
	if v.kind == StringKind {
		return v.StringValue(), true
	}
	if v.kind == NumberKind {
		return v.Repr(), true
	}
	return "", false
}
func badNumericType(a, b Value) string {
	if _, ok := toNumber(a); !ok {
		return a.TypeName()
	}
	return b.TypeName()
}
func badStringType(a, b Value) string {
	if _, ok := stringCoerce(a); !ok {
		return a.TypeName()
	}
	return b.TypeName()
}

func metaField(v Value, name string) Value {
	if v.kind == TableKind && v.Table().meta != nil {
		return v.Table().meta.GetString(name)
	}
	return Nil
}
func (s *State) callValue(callee Value, args []Value) (callResult, error) {
	if callee.kind == FunctionKind {
		return s.dispatch(callee.Function(), args)
	}
	method := metaField(callee, "__call")
	if method.kind == FunctionKind {
		withSelf := make([]Value, len(args)+1)
		withSelf[0] = callee
		copy(withSelf[1:], args)
		return s.dispatch(method.Function(), withSelf)
	}
	return callResult{}, runtimeError("attempt to call a %s value", callee.TypeName())
}
func (s *State) index(target, key Value, depth int) (Value, error) {
	if depth > 100 {
		return Nil, runtimeError("loop in gettable")
	}
	if target.kind == TableKind {
		if value := target.Table().Get(key); value.kind != NilKind {
			return value, nil
		}
		handler := metaField(target, "__index")
		if handler.kind == NilKind {
			return Nil, nil
		}
		if handler.kind == FunctionKind {
			r, err := s.dispatch(handler.Function(), []Value{target, key})
			if err != nil {
				return Nil, err
			}
			if r.count > 0 {
				return r.at(0), nil
			}
			return Nil, nil
		}
		return s.index(handler, key, depth+1)
	}
	if target.kind == StringKind && key.kind == StringKind {
		if library := s.globals.GetString("string"); library.kind == TableKind {
			return library.Table().GetString(key.StringValue()), nil
		}
	}
	return Nil, runtimeError("attempt to index a %s value", target.TypeName())
}
func (s *State) assignIndex(target, key, value Value, depth int) error {
	if depth > 100 {
		return runtimeError("loop in settable")
	}
	if target.kind != TableKind {
		return runtimeError("attempt to index a %s value", target.TypeName())
	}
	table := target.Table()
	if table.Get(key).kind != NilKind {
		return table.Set(key, value)
	}
	handler := metaField(target, "__newindex")
	if handler.kind == NilKind {
		return table.Set(key, value)
	}
	if handler.kind == FunctionKind {
		_, err := s.dispatch(handler.Function(), []Value{target, key, value})
		return err
	}
	return s.assignIndex(handler, key, value, depth+1)
}

func (s *State) swapTable(target, first, second Value) error {
	if target.kind == TableKind && target.Table().meta == nil && first.kind == NumberKind && second.kind == NumberKind {
		table := target.Table()
		i, j := int(first.Number()), int(second.Number())
		if i >= 1 && j >= 1 && i <= len(table.array) && j <= len(table.array) && float64(i) == first.Number() && float64(j) == second.Number() {
			table.array[i-1], table.array[j-1] = table.array[j-1], table.array[i-1]
			return nil
		}
	}
	left, err := s.index(target, second, 0)
	if err != nil {
		return err
	}
	right, err := s.index(target, first, 0)
	if err != nil {
		return err
	}
	if err = s.assignIndex(target, first, left, 0); err != nil {
		return err
	}
	return s.assignIndex(target, second, right, 0)
}

func (s *State) addTableValue(accumulator *Value, target, key Value) error {
	if accumulator.kind == NumberKind && target.kind == TableKind && target.Table().meta == nil && key.kind == NumberKind {
		if value, ok := target.Table().getDenseNumber(key.Number()); ok && value.kind == NumberKind {
			*accumulator = Number(accumulator.Number() + value.Number())
			return nil
		}
	}
	value, err := s.index(target, key, 0)
	if err != nil {
		return err
	}
	x, xok := toNumber(*accumulator)
	y, yok := toNumber(value)
	if xok && yok {
		*accumulator = Number(x + y)
		return nil
	}
	result, ok, err := s.binaryMetamethod(*accumulator, value, "__add")
	if err != nil {
		return err
	}
	if !ok {
		return runtimeError("attempt to perform arithmetic on a %s value", badNumericType(*accumulator, value))
	}
	*accumulator = result
	return nil
}
func (s *State) binaryMetamethod(a, b Value, name string) (Value, bool, error) {
	method := metaField(a, name)
	if method.kind == NilKind {
		method = metaField(b, name)
	}
	if method.kind != FunctionKind {
		return Nil, false, nil
	}
	r, err := s.dispatch(method.Function(), []Value{a, b})
	if err != nil {
		return Nil, true, err
	}
	if r.count == 0 {
		return Nil, true, nil
	}
	return r.at(0), true, nil
}
func (s *State) unaryMetamethod(v Value, name string) (Value, bool, error) {
	method := metaField(v, name)
	if method.kind != FunctionKind {
		return Nil, false, nil
	}
	r, err := s.dispatch(method.Function(), []Value{v})
	if err != nil {
		return Nil, true, err
	}
	if r.count == 0 {
		return Nil, true, nil
	}
	return r.at(0), true, nil
}
