package lua

import "math"

// callLuaFast executes the allocation-sensitive, single-result-compatible
// instruction subset with an iterative frame trampoline. Programs requiring
// dynamic result lists/coroutine suspension use the general VM.
func (s *State) callLuaFast(root *Function, args []Value) (callResult, error) {
	start := len(s.frames)
	if err := s.pushFastFrame(root, args, -1, 0); err != nil {
		return callResult{}, err
	}
	for len(s.frames) > start {
		fi := len(s.frames) - 1
		f := &s.frames[fi]
		p, fn, regs := f.fn.Proto, f.fn, f.regs
		if f.pc >= len(p.Code) {
			result := callResult{}
			if done, err := s.fastReturn(start, result); done || err != nil {
				return result, err
			}
			continue
		}
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
			target := regs[b].value
			if target.kind == TableKind && target.Table().meta == nil {
				key := regs[c].value
				if key.kind == NumberKind {
					if value, ok := target.Table().getDenseNumber(key.Number()); ok {
						regs[a].value = value
						continue
					}
				}
				regs[a].value = target.Table().Get(key)
			} else {
				value, err := s.index(target, regs[c].value, 0)
				if err != nil {
					return callResult{}, s.vmWrap(p, pc, err)
				}
				regs[a].value = value
			}
		case opGetTableK:
			target, key := regs[b].value, p.Constants[c]
			if target.kind == TableKind && target.Table().meta == nil {
				if key.kind == StringKind {
					table, cache := target.Table(), &p.FieldCaches[pc]
					if table.shape != nil && cache.shape == table.shape {
						regs[a].value = table.fields[cache.slot].value
						continue
					}
					if slot, ok := s.cachedField(p, pc, target.Table(), key.StringValue()); ok {
						regs[a].value = target.Table().fields[slot].value
						continue
					}
				}
				if key.kind == NumberKind {
					if value, ok := target.Table().getDenseNumber(key.Number()); ok {
						regs[a].value = value
						continue
					}
				}
				regs[a].value = target.Table().Get(key)
			} else {
				value, err := s.index(target, key, 0)
				if err != nil {
					return callResult{}, s.vmWrap(p, pc, err)
				}
				regs[a].value = value
			}
		case opGetArrayI:
			target := regs[b].value
			if target.kind == TableKind && target.Table().meta == nil && c <= len(target.Table().array) {
				regs[a].value = target.Table().array[c-1]
			} else {
				value, err := s.index(target, Number(float64(c)), 0)
				if err != nil {
					return callResult{}, s.vmWrap(p, pc, err)
				}
				regs[a].value = value
			}
		case opGetFieldK:
			target, name := regs[b].value, p.Constants[c].StringValue()
			if target.kind == TableKind && target.Table().meta == nil {
				table, cache := target.Table(), &p.FieldCaches[pc]
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
				value, err := s.index(target, p.Constants[c], 0)
				if err != nil {
					return callResult{}, s.vmWrap(p, pc, err)
				}
				regs[a].value = value
			}
		case opSetTable:
			target := regs[a].value
			if target.kind == TableKind && target.Table().meta == nil {
				key, value := regs[b].value, regs[c].value
				if key.kind == NumberKind && target.Table().setDenseNumber(key.Number(), value) {
					continue
				}
				if err := target.Table().Set(key, value); err != nil {
					return callResult{}, s.vmWrap(p, pc, err)
				}
			} else if err := s.assignIndex(target, regs[b].value, regs[c].value, 0); err != nil {
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
			target, key, value := regs[a].value, p.Constants[b], regs[c].value
			if target.kind == TableKind && target.Table().meta == nil {
				if key.kind == StringKind {
					table, cache := target.Table(), &p.FieldCaches[pc]
					if table.shape != nil && cache.shape == table.shape {
						table.fields[cache.slot].value = value
						continue
					}
					if slot, ok := s.cachedField(p, pc, target.Table(), key.StringValue()); ok {
						target.Table().fields[slot].value = value
						continue
					}
				}
				if key.kind == NumberKind && target.Table().setDenseNumber(key.Number(), value) {
					continue
				}
				if err := target.Table().Set(key, value); err != nil {
					return callResult{}, s.vmWrap(p, pc, err)
				}
			} else if err := s.assignIndex(target, key, value, 0); err != nil {
				return callResult{}, s.vmWrap(p, pc, err)
			}
		case opSetArrayI:
			target, value := regs[a].value, regs[c].value
			if target.kind == TableKind && target.Table().meta == nil {
				table := target.Table()
				if b <= len(table.array) {
					table.array[b-1] = value
					continue
				}
				if table.appendDenseNumber(b, value) {
					continue
				}
			}
			if err := s.assignIndex(target, Number(float64(b)), value, 0); err != nil {
				return callResult{}, s.vmWrap(p, pc, err)
			}
		case opSetFieldK:
			target, value, name := regs[a].value, regs[c].value, p.Constants[b].StringValue()
			if target.kind == TableKind && target.Table().meta == nil {
				table, cache := target.Table(), &p.FieldCaches[pc]
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
			if err := s.assignIndex(target, p.Constants[b], value, 0); err != nil {
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
				name := arithmeticMetaName(ins.op())
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
			if v.kind == StringKind {
				regs[a].value = Number(float64(len(v.StringValue())))
			} else if v.kind == TableKind && v.Table().meta == nil {
				regs[a].value = Number(float64(v.Table().Len()))
			} else if value, ok, err := s.unaryMetamethod(v, "__len"); err != nil {
				return callResult{}, err
			} else if ok {
				regs[a].value = value
			} else if v.kind == TableKind {
				regs[a].value = Number(float64(v.Table().Len()))
			} else {
				return callResult{}, s.vmError(p, pc, "attempt to get length of a %s value", v.TypeName())
			}
		case opConcat:
			x, xok := stringCoerce(regs[b].value)
			y, yok := stringCoerce(regs[c].value)
			if xok && yok {
				regs[a].value = String(x + y)
			} else if value, ok, err := s.binaryMetamethod(regs[b].value, regs[c].value, "__concat"); err != nil {
				return callResult{}, err
			} else if ok {
				regs[a].value = value
			} else {
				return callResult{}, s.vmError(p, pc, "attempt to concatenate a %s value", badStringType(regs[b].value, regs[c].value))
			}
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
			if callee.kind == FunctionKind && callee.Function().Native == nil && callee.Function().Proto != nil && callee.Function().Proto.Fast {
				if err := s.pushFastFromRegisters(callee.Function(), regs, a+1, b, a, c); err != nil {
					return callResult{}, err
				}
				continue
			}
			var argv []Value
			if b <= len(s.scratchArgs) {
				argv = s.scratchArgs[:b]
			} else {
				argv = make([]Value, b)
			}
			for i := 0; i < b; i++ {
				argv[i] = regs[a+1+i].value
			}
			results, err := s.callValue(callee, argv)
			if err != nil {
				return callResult{}, err
			}
			f = &s.frames[fi]
			s.applyFastResults(f, a, c, results)
		case opReturn:
			result := callResult{count: b}
			for i := 0; i < b; i++ {
				result.inline[i] = regs[a+i].value
			}
			done, err := s.fastReturn(start, result)
			if done || err != nil {
				return result, err
			}
		default:
			return callResult{}, s.vmError(p, pc, "opcode %d escaped fast VM", ins.op())
		}
	}
	return callResult{}, nil
}

func callNumericPure(p *Prototype, source []cell, argStart, nargs int) (float64, bool) {
	if nargs < int(p.Parameters) {
		return 0, false
	}
	var regs [16]float64
	for i := 0; i < int(p.Parameters); i++ {
		value := source[argStart+i].value
		if value.kind != NumberKind {
			return 0, false
		}
		regs[i] = value.Number()
	}
	if formula := p.NumericFormula; formula != nil {
		x, y := regs[0], regs[1]
		n := formula.numerator[0] + x*formula.numerator[1] + y*formula.numerator[2] + x*x*formula.numerator[3] + x*y*formula.numerator[4] + y*y*formula.numerator[5]
		d := formula.denominator[0] + x*formula.denominator[1] + y*formula.denominator[2] + x*x*formula.denominator[3] + x*y*formula.denominator[4] + y*y*formula.denominator[5]
		return n / d, true
	}
	for i, constant := range p.Constants {
		regs[int(p.Parameters)+i] = constant.Number()
	}
	for _, ins := range p.NumericCode {
		a, b, c := ins.a(), ins.b(), ins.c()
		switch ins.op() {
		case opMove:
			regs[a] = regs[b]
		case opLoadK:
			regs[a] = p.Constants[ins.bx()].Number()
		case opAdd:
			regs[a] = regs[b] + regs[c]
		case opSub:
			regs[a] = regs[b] - regs[c]
		case opMul:
			regs[a] = regs[b] * regs[c]
		case opDiv:
			regs[a] = regs[b] / regs[c]
		case opMod:
			regs[a] = regs[b] - math.Floor(regs[b]/regs[c])*regs[c]
		case opPow:
			regs[a] = math.Pow(regs[b], regs[c])
		case opNeg:
			regs[a] = -regs[b]
		case opReturn:
			return regs[a], true
		}
	}
	return 0, false
}

func (s *State) pushFastFrame(fn *Function, args []Value, returnBase, returnWant int) error {
	p := fn.Proto
	size := int(p.MaxRegisters)
	if size < int(p.Parameters) {
		size = int(p.Parameters)
	}
	frame := frame{fn: fn, returnBase: returnBase, returnWant: returnWant, stackBase: s.top}
	if p.Captured {
		frame.regs = make([]cell, size)
		frame.heap = true
	} else {
		if s.top+size > len(s.stack) {
			return runtimeError("Lua stack overflow")
		}
		frame.regs = s.stack[s.top : s.top+size]
		s.top += size
		for i := range frame.regs {
			frame.regs[i].value = Nil
		}
	}
	for i := 0; i < len(args) && i < int(p.Parameters); i++ {
		frame.regs[i].value = args[i]
	}
	s.frames = append(s.frames, frame)
	return nil
}
func (s *State) pushFastFromRegisters(fn *Function, source []cell, argStart, nargs, returnBase, returnWant int) error {
	before := len(s.frames)
	if err := s.pushFastFrame(fn, nil, returnBase, returnWant); err != nil {
		return err
	}
	frame := &s.frames[before]
	for i := 0; i < nargs && i < int(fn.Proto.Parameters); i++ {
		frame.regs[i].value = source[argStart+i].value
	}
	return nil
}
func (s *State) fastReturn(start int, result callResult) (bool, error) {
	i := len(s.frames) - 1
	finished := s.frames[i]
	if !finished.heap {
		for j := range finished.regs {
			finished.regs[j].value = Nil
		}
		s.top = finished.stackBase
	}
	s.frames = s.frames[:i]
	if len(s.frames) == start {
		return true, nil
	}
	caller := &s.frames[len(s.frames)-1]
	s.applyFastResults(caller, finished.returnBase, finished.returnWant, result)
	return false, nil
}
func (s *State) applyFastResults(f *frame, base, want int, result callResult) {
	for i := 0; i < want; i++ {
		if i < result.count {
			f.regs[base+i].value = result.at(i)
		} else {
			f.regs[base+i].value = Nil
		}
	}
}
func arithmeticMetaName(op opcode) string {
	switch op {
	case opAdd:
		return "__add"
	case opSub:
		return "__sub"
	case opMul:
		return "__mul"
	case opDiv:
		return "__div"
	case opMod:
		return "__mod"
	default:
		return "__pow"
	}
}

func regularConstantOpcode(op opcode) opcode {
	switch op {
	case opAddK:
		return opAdd
	case opSubK:
		return opSub
	case opMulK:
		return opMul
	case opDivK:
		return opDiv
	case opModK:
		return opMod
	default:
		return opPow
	}
}
