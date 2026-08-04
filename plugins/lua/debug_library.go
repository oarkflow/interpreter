package lua

import "strings"

func (s *State) openDebugLibrary() {
	lib := NewTable(0, 8)
	lib.SetString("sethook", Native(func(state *State, args []Value) ([]Value, error) {
		target := state
		if len(args) > 0 && args[0].kind == ThreadKind {
			if args[0].Thread() == nil {
				return nil, runtimeError("thread expected")
			}
			target = args[0].Thread().state
			args = args[1:]
		}
		if len(args) == 0 || args[0].kind == NilKind {
			target.hook, target.hookMask, target.hookCount, target.hookCounter = Nil, "", 0, 0
			return nil, nil
		}
		if args[0].kind != FunctionKind {
			return nil, runtimeError("function expected")
		}
		target.hook = args[0]
		target.hookMask = ""
		if len(args) > 1 && args[1].kind != NilKind {
			mask, err := needString(args, 1)
			if err != nil {
				return nil, err
			}
			target.hookMask = mask
		}
		target.hookCount = 0
		if len(args) > 2 {
			count, err := needNumber(args, 2)
			if err != nil {
				return nil, err
			}
			target.hookCount = int(count)
		}
		target.hookCounter = 0
		if len(target.frames) > 0 {
			frame := &target.frames[len(target.frames)-1]
			target.hookSkipFunction = frame.fn
			pc := frame.pc - 1
			if frame.fn.Proto != nil && pc >= 0 && pc < len(frame.fn.Proto.Lines) {
				target.hookSkipLine = frame.fn.Proto.Lines[pc]
			}
		}
		return nil, nil
	}))
	lib.SetString("gethook", Native(func(state *State, args []Value) ([]Value, error) {
		target := state
		if len(args) > 0 && args[0].kind == ThreadKind && args[0].Thread() != nil {
			target = args[0].Thread().state
		}
		if target.hook.kind == NilKind {
			return []Value{Nil}, nil
		}
		return []Value{target.hook, String(target.hookMask), Number(float64(target.hookCount))}, nil
	}))
	lib.SetString("setmetatable", Native(func(state *State, args []Value) ([]Value, error) {
		if len(args) < 2 || args[1].kind != NilKind && args[1].kind != TableKind {
			return nil, runtimeError("nil or table expected")
		}
		var meta *Table
		if args[1].kind == TableKind {
			meta = args[1].Table()
		}
		switch args[0].kind {
		case TableKind:
			args[0].Table().SetMetatable(meta)
			weakKeys, weakValues := tableWeakMode(args[0].Table())
			state.weakTables = state.weakTables || weakKeys || weakValues
		case UserdataKind:
			if args[0].ptr != nil {
				(*userdataBox)(args[0].ptr).meta = meta
			}
		default:
			state.typeMetatables[args[0].kind] = meta
		}
		return []Value{args[0]}, nil
	}))
	lib.SetString("getmetatable", Native(func(state *State, args []Value) ([]Value, error) {
		if len(args) == 0 {
			return []Value{Nil}, nil
		}
		if meta := state.metatable(args[0]); meta != nil {
			return []Value{TableValue(meta)}, nil
		}
		return []Value{Nil}, nil
	}))
	lib.SetString("getfenv", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) == 0 || args[0].kind != ThreadKind || args[0].Thread() == nil {
			return nil, runtimeError("thread expected")
		}
		return []Value{TableValue(args[0].Thread().state.globals)}, nil
	}))
	lib.SetString("setfenv", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) < 2 || args[0].kind != ThreadKind || args[0].Thread() == nil || args[1].kind != TableKind {
			return nil, runtimeError("thread and table expected")
		}
		args[0].Thread().state.globals = args[1].Table()
		return []Value{args[0]}, nil
	}))
	lib.SetString("getupvalue", Native(func(_ *State, args []Value) ([]Value, error) {
		fn, index, ok := debugUpvalue(args)
		if !ok || index >= len(fn.Up) {
			return []Value{Nil}, nil
		}
		return []Value{String(fn.Proto.UpvalueNames[index]), fn.Up[index].value}, nil
	}))
	lib.SetString("setupvalue", Native(func(_ *State, args []Value) ([]Value, error) {
		fn, index, ok := debugUpvalue(args)
		if !ok || index >= len(fn.Up) || len(args) < 3 {
			return []Value{Nil}, nil
		}
		fn.Up[index].value = args[2]
		return []Value{String(fn.Proto.UpvalueNames[index])}, nil
	}))
	lib.SetString("getinfo", Native(func(state *State, args []Value) ([]Value, error) {
		if len(args) == 0 {
			return nil, runtimeError("level or function expected")
		}
		var fn *Function
		currentLine := -1
		frameIndex := -1
		directFunction := args[0].kind == FunctionKind
		if directFunction {
			fn = args[0].Function()
		} else {
			level, ok := toNumber(args[0])
			if !ok || level < 0 || level != float64(int(level)) {
				return nil, runtimeError("invalid level")
			}
			index := len(state.frames) - int(level)
			if index < 0 || index >= len(state.frames) {
				return []Value{Nil}, nil
			}
			frame := &state.frames[index]
			frameIndex = index
			fn = frame.fn
			if fn.Proto != nil && frame.pc > 0 && frame.pc-1 < len(fn.Proto.Lines) {
				currentLine = fn.Proto.Lines[frame.pc-1]
			}
		}
		info := state.newTable(0, 12)
		if fn == nil || fn.Proto == nil {
			info.SetString("what", String("C"))
			info.SetString("source", String("=[C]"))
			info.SetString("short_src", String("[C]"))
			info.SetString("linedefined", Number(-1))
			info.SetString("lastlinedefined", Number(-1))
		} else {
			proto := fn.Proto
			info.SetString("what", String("Lua"))
			info.SetString("source", String(proto.Source))
			short := shortSource(proto.Source)
			info.SetString("short_src", String(short))
			first, last := proto.DefinedLine, proto.LastDefinedLine
			info.SetString("linedefined", Number(float64(first)))
			info.SetString("lastlinedefined", Number(float64(last)))
			info.SetString("nups", Number(float64(len(proto.Upvalues))))
			active := state.newTable(0, len(proto.Lines))
			for _, line := range proto.Lines {
				if line > first && line <= last {
					_ = active.Set(Number(float64(line)), True)
				}
			}
			if last > first {
				_ = active.Set(Number(float64(last)), True)
			}
			info.SetString("activelines", TableValue(active))
		}
		info.SetString("currentline", Number(float64(currentLine)))
		name, nameWhat := Nil, ""
		if directFunction {
			name = Nil
		} else {
			name, nameWhat = state.stackFunctionName(frameIndex, fn)
		}
		info.SetString("namewhat", String(nameWhat))
		info.SetString("name", name)
		info.SetString("func", FunctionValue(fn))
		return []Value{TableValue(info)}, nil
	}))
	s.globals.SetString("debug", TableValue(lib))
}

func (s *State) stackFunctionName(frameIndex int, fn *Function) (Value, string) {
	if frameIndex > 0 {
		caller := &s.frames[frameIndex-1]
		for register, cell := range caller.regs {
			if cell.value.kind == FunctionKind && cell.value.Function() == fn {
				if name := localNameAt(caller.fn.Proto, register, caller.pc); name != "" {
					return String(name), "local"
				}
			}
			if name, ok := tableFunctionField(cell.value, fn); ok {
				return String(name), "field"
			}
		}
		for index, up := range caller.fn.Up {
			if up.value.kind == FunctionKind && up.value.Function() == fn && index < len(caller.fn.Proto.UpvalueNames) {
				return String(caller.fn.Proto.UpvalueNames[index]), "upvalue"
			}
			if name, ok := tableFunctionField(up.value, fn); ok {
				return String(name), "field"
			}
		}
	}
	if name := s.functionName(fn); name.kind != NilKind {
		return name, "global"
	}
	return Nil, ""
}

func localNameAt(proto *Prototype, register, pc int) string {
	if proto == nil {
		return ""
	}
	for i := len(proto.LocalVariables) - 1; i >= 0; i-- {
		local := proto.LocalVariables[i]
		if local.Register == register && pc >= local.StartPC && pc <= local.EndPC {
			return local.Name
		}
	}
	return ""
}

func tableFunctionField(value Value, fn *Function) (string, bool) {
	if value.kind != TableKind {
		return "", false
	}
	name := ""
	value.Table().ForEach(func(key, field Value) bool {
		if key.kind == StringKind && field.kind == FunctionKind && field.Function() == fn {
			name = key.StringValue()
			return false
		}
		return true
	})
	return name, name != ""
}

func shortSource(source string) string {
	if strings.HasPrefix(source, "=") {
		return strings.TrimPrefix(source, "=")
	}
	if strings.HasPrefix(source, "@") {
		short := strings.TrimPrefix(source, "@")
		if len(short) > 60 {
			short = "..." + short[len(short)-57:]
		}
		return short
	}
	text := source
	if newline := strings.IndexAny(text, "\r\n"); newline >= 0 {
		if newline == 0 {
			return "[string \"...\"]"
		}
		text = text[:newline]
	}
	if len(text) > 45 {
		text = text[:42] + "..."
	}
	return "[string \"" + text + "\"]"
}

func debugUpvalue(args []Value) (*Function, int, bool) {
	if len(args) < 2 || args[0].kind != FunctionKind || args[0].Function().Proto == nil {
		return nil, 0, false
	}
	n, ok := toNumber(args[1])
	index := int(n) - 1
	if !ok || n != float64(int(n)) || index < 0 {
		return nil, 0, false
	}
	fn := args[0].Function()
	if len(fn.Proto.UpvalueNames) < len(fn.Up) {
		return nil, 0, false
	}
	return fn, index, true
}

func (s *State) functionName(fn *Function) Value {
	if fn == nil {
		return Nil
	}
	var name Value
	env := fn.Env
	if env == nil {
		env = s.globals
	}
	env.ForEach(func(key, value Value) bool {
		if key.kind == StringKind && value.kind == FunctionKind && value.Function() == fn {
			name = key
			return false
		}
		return true
	})
	return name
}
