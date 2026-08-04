package lua

import (
	"fmt"
	"strings"
)

func (s *State) openDebugLibrary() {
	lib := NewTable(0, 8)
	lib.SetString("getregistry", Native(func(state *State, _ []Value) ([]Value, error) {
		return []Value{TableValue(state.registry)}, nil
	}))
	lib.SetString("traceback", Native(func(state *State, args []Value) ([]Value, error) {
		target := state
		if len(args) > 0 && args[0].kind == ThreadKind && args[0].Thread() != nil {
			target = args[0].Thread().state
			args = args[1:]
		}
		if len(args) > 0 && args[0].kind != NilKind && args[0].kind != StringKind {
			return []Value{args[0]}, nil
		}
		message := ""
		if len(args) > 0 && args[0].kind == StringKind {
			message = args[0].StringValue() + "\n"
		}
		level := 1
		includeTraceback := false
		if len(args) > 1 {
			if n, ok := toNumber(args[1]); ok && n >= 0 {
				includeTraceback = n == 0
				level = int(n) + 1
			}
		}
		var out strings.Builder
		out.WriteString(message)
		out.WriteString("stack traceback:\n")
		if includeTraceback {
			out.WriteString("\t[C]: in function 'traceback'\n")
		}
		if target.currentThread != nil && !target.currentThread.running && len(target.frames) > 0 {
			out.WriteString("\t[C]: in function 'yield'\n")
		}
		if len(target.savedTrace) > 0 {
			for _, line := range target.savedTrace {
				out.WriteString(line)
				out.WriteByte('\n')
			}
			return []Value{String(out.String())}, nil
		}
		for ; ; level++ {
			fn, tail, index, ok := target.debugStackEntry(level)
			if !ok {
				break
			}
			if tail {
				out.WriteString("\t(tail call): ?\n")
				continue
			}
			line := -1
			if index >= 0 {
				frame := &target.frames[index]
				pc := frame.pc - 1
				if fn != nil && fn.Proto != nil && pc >= 0 && pc < len(fn.Proto.Lines) {
					line = fn.Proto.Lines[pc]
				}
			}
			source := "[C]"
			defined := -1
			if fn != nil && fn.Proto != nil {
				source = shortSource(fn.Proto.Source)
				defined = fn.Proto.DefinedLine
			}
			name, _ := target.stackFunctionName(index, fn)
			if name.kind == StringKind && line > 0 {
				fmt.Fprintf(&out, "\t%s:%d: in function '%s'\n", source, line, name.StringValue())
			} else if line > 0 {
				fmt.Fprintf(&out, "\t%s:%d: in function <%s:%d>\n", source, line, source, defined)
			} else {
				fmt.Fprintf(&out, "\t%s: in function <%s:%d>\n", source, source, defined)
			}
		}
		return []Value{String(out.String())}, nil
	}))
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
	lib.SetString("getlocal", Native(func(state *State, args []Value) ([]Value, error) {
		if len(args) > 0 && args[0].kind == ThreadKind && args[0].Thread() != nil {
			state = args[0].Thread().state
			args = args[1:]
		}
		if len(args) >= 2 {
			level, levelOK := toNumber(args[0])
			index, indexOK := toNumber(args[1])
			if levelOK && level == 0 && indexOK && index >= 1 && index == float64(int(index)) {
				i := int(index) - 1
				if i < len(args) {
					return []Value{String("(*temporary)"), args[i]}, nil
				}
				return []Value{Nil}, nil
			}
		}
		frame, local, ok := state.debugLocal(args)
		if !ok {
			return []Value{Nil}, nil
		}
		return []Value{String(local.Name), frame.regs[local.Register].value}, nil
	}))
	lib.SetString("setlocal", Native(func(state *State, args []Value) ([]Value, error) {
		if len(args) > 0 && args[0].kind == ThreadKind && args[0].Thread() != nil {
			state = args[0].Thread().state
			args = args[1:]
		}
		frame, local, ok := state.debugLocal(args)
		if !ok || len(args) < 3 {
			return []Value{Nil}, nil
		}
		frame.regs[local.Register].value = args[2]
		return []Value{String(local.Name)}, nil
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
		if len(args) > 0 && args[0].kind == ThreadKind && args[0].Thread() != nil {
			state = args[0].Thread().state
			args = args[1:]
		}
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
			if state.hookActive && int(level) == 2 && state.hookSubject != nil {
				fn = state.hookSubject
			} else {
				entryFn, tail, index, ok := state.debugStackEntry(int(level))
				if !ok {
					return []Value{Nil}, nil
				}
				if tail {
					info := state.newTable(0, 8)
					info.SetString("what", String("tail"))
					info.SetString("source", String("=(tail call)"))
					info.SetString("short_src", String("(tail call)"))
					info.SetString("linedefined", Number(-1))
					info.SetString("lastlinedefined", Number(-1))
					info.SetString("currentline", Number(-1))
					info.SetString("name", Nil)
					info.SetString("namewhat", String(""))
					info.SetString("func", Nil)
					return []Value{TableValue(info)}, nil
				}
				frame := &state.frames[index]
				frameIndex = index
				fn = entryFn
				if fn.Proto != nil && frame.pc > 0 && frame.pc-1 < len(fn.Proto.Lines) {
					currentLine = fn.Proto.Lines[frame.pc-1]
				}
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
			what := "Lua"
			if proto.DefinedLine == 0 {
				what = "main"
			}
			info.SetString("what", String(what))
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

func (s *State) debugStackEntry(level int) (*Function, bool, int, bool) {
	if level < 1 {
		return nil, false, -1, false
	}
	remaining := level
	for i := len(s.frames) - 1; i >= 0; i-- {
		frame := &s.frames[i]
		if remaining == 1 {
			return frame.fn, frame.tail, i, true
		}
		remaining--
		for j := len(frame.tailBelow) - 1; j >= 0; j-- {
			if remaining == 1 {
				return frame.tailBelow[j], true, -1, true
			}
			remaining--
		}
	}
	return nil, false, -1, false
}

func (s *State) captureErrorTrace(nativeName string) {
	if s.currentThread == nil && !s.captureErrors {
		return
	}
	lines := make([]string, 0, len(s.frames)+1)
	if nativeName != "" {
		lines = append(lines, fmt.Sprintf("\t[C]: in function '%s'", nativeName))
	}
	for level := 1; ; level++ {
		fn, tail, index, ok := s.debugStackEntry(level)
		if !ok {
			break
		}
		if tail {
			lines = append(lines, "\t(tail call): ?")
			continue
		}
		source, line := "[C]", -1
		if fn != nil && fn.Proto != nil {
			source = shortSource(fn.Proto.Source)
			if index >= 0 {
				pc := s.frames[index].pc - 1
				if pc >= 0 && pc < len(fn.Proto.Lines) {
					line = fn.Proto.Lines[pc]
				}
			}
		}
		name, _ := s.stackFunctionName(index, fn)
		if name.kind == StringKind {
			lines = append(lines, fmt.Sprintf("\t%s:%d: in function '%s'", source, line, name.StringValue()))
		} else {
			lines = append(lines, fmt.Sprintf("\t%s:%d: in function <%s>", source, line, source))
		}
	}
	s.savedTrace = lines
}

func (s *State) debugLocal(args []Value) (*frame, LocalVariableInfo, bool) {
	if len(args) < 2 {
		return nil, LocalVariableInfo{}, false
	}
	level, levelOK := toNumber(args[0])
	n, nOK := toNumber(args[1])
	if !levelOK || !nOK || level < 1 || n < 1 || level != float64(int(level)) || n != float64(int(n)) {
		return nil, LocalVariableInfo{}, false
	}
	frameIndex := len(s.frames) - int(level)
	if frameIndex < 0 || frameIndex >= len(s.frames) {
		return nil, LocalVariableInfo{}, false
	}
	frame := &s.frames[frameIndex]
	pc := frame.pc
	active := make([]LocalVariableInfo, 0, len(frame.fn.Proto.LocalVariables))
	for _, local := range frame.fn.Proto.LocalVariables {
		if pc >= local.StartPC && pc <= local.EndPC {
			active = append(active, local)
		}
	}
	live := len(frame.regs)
	livePC := frame.pc - 1
	if livePC >= 0 && livePC < len(frame.fn.Proto.LiveRegisters) {
		live = int(frame.fn.Proto.LiveRegisters[livePC])
		if live > len(frame.regs) {
			live = len(frame.regs)
		}
		instruction := frame.fn.Proto.Code[livePC]
		if (instruction.op() == opCall || instruction.op() == opTailCall) && int(instruction.a()) < live {
			live = int(instruction.a())
		}
	}
	for register := 0; register < live; register++ {
		named := false
		for _, local := range active {
			if local.Register == register {
				named = true
				break
			}
		}
		if !named && frame.regs[register].value.kind != NilKind {
			active = append(active, LocalVariableInfo{Name: "(*temporary)", Register: register})
		}
	}
	for i := 1; i < len(active); i++ {
		for j := i; j > 0 && active[j].Register < active[j-1].Register; j-- {
			active[j], active[j-1] = active[j-1], active[j]
		}
	}
	index := int(n) - 1
	if index >= len(active) || active[index].Register >= len(frame.regs) {
		return nil, LocalVariableInfo{}, false
	}
	return frame, active[index], true
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
