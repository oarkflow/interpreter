package lua

import (
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

func Native(fn NativeFunction) Value { return FunctionValue(&Function{Native: fn}) }

// NativeBinary registers the allocation-free typed path for a Go numeric
// callback called from Lua.
func NativeBinary(fn func(float64, float64) float64) Value {
	return FunctionValue(&Function{NativeNumber2: fn})
}

func (s *State) openLibraries() {
	s.globals.SetString("_G", TableValue(s.globals))
	s.globals.SetString("_VERSION", String("Lua 5.1 (native Go)"))
	s.globals.SetString("type", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) < 1 {
			return []Value{String("nil")}, nil
		}
		return []Value{String(args[0].TypeName())}, nil
	}))
	s.globals.SetString("tostring", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) < 1 {
			return []Value{String("nil")}, nil
		}
		return []Value{String(args[0].Repr())}, nil
	}))
	s.globals.SetString("tonumber", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) < 1 {
			return []Value{Nil}, nil
		}
		if n, ok := toNumber(args[0]); ok {
			return []Value{Number(n)}, nil
		}
		return []Value{Nil}, nil
	}))
	s.globals.SetString("assert", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) == 0 || !args[0].Truthy() {
			message := "assertion failed!"
			if len(args) > 1 {
				message = args[1].Repr()
			}
			return nil, runtimeError("%s", message)
		}
		return args, nil
	}))
	s.globals.SetString("error", Native(func(_ *State, args []Value) ([]Value, error) {
		message := "error"
		if len(args) > 0 {
			message = args[0].Repr()
		}
		return nil, runtimeError("%s", message)
	}))
	s.globals.SetString("print", Native(func(state *State, args []Value) ([]Value, error) {
		w := state.Output
		if w == nil {
			w = os.Stdout
		}
		for i, v := range args {
			if i > 0 {
				_, _ = io.WriteString(w, "\t")
			}
			_, _ = io.WriteString(w, v.Repr())
		}
		_, _ = io.WriteString(w, "\n")
		return nil, nil
	}))
	nextFn := Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) < 1 || args[0].kind != TableKind {
			return nil, runtimeError("table expected")
		}
		previous := Nil
		if len(args) > 1 {
			previous = args[1]
		}
		key, value, ok := args[0].Table().Next(previous)
		if !ok {
			return []Value{Nil}, nil
		}
		return []Value{key, value}, nil
	})
	s.globals.SetString("next", nextFn)
	s.globals.SetString("pairs", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) < 1 || args[0].kind != TableKind {
			return nil, runtimeError("table expected")
		}
		return []Value{nextFn, args[0], Nil}, nil
	}))
	ipairsIterator := Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) < 2 || args[0].kind != TableKind {
			return []Value{Nil}, nil
		}
		index, ok := toNumber(args[1])
		if !ok {
			index = 0
		}
		index++
		value := args[0].Table().Get(Number(index))
		if value.kind == NilKind {
			return []Value{Nil}, nil
		}
		return []Value{Number(index), value}, nil
	})
	s.globals.SetString("ipairs", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) < 1 || args[0].kind != TableKind {
			return nil, runtimeError("table expected")
		}
		return []Value{ipairsIterator, args[0], Number(0)}, nil
	}))
	s.globals.SetString("getmetatable", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) < 1 || args[0].kind != TableKind {
			return []Value{Nil}, nil
		}
		meta := args[0].Table().Metatable()
		if meta == nil {
			return []Value{Nil}, nil
		}
		if protected := meta.GetString("__metatable"); protected.kind != NilKind {
			return []Value{protected}, nil
		}
		return []Value{TableValue(meta)}, nil
	}))
	s.globals.SetString("setmetatable", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) < 2 || args[0].kind != TableKind {
			return nil, runtimeError("table expected")
		}
		if old := args[0].Table().Metatable(); old != nil && old.GetString("__metatable").kind != NilKind {
			return nil, runtimeError("cannot change a protected metatable")
		}
		if args[1].kind == NilKind {
			args[0].Table().SetMetatable(nil)
		} else if args[1].kind == TableKind {
			args[0].Table().SetMetatable(args[1].Table())
		} else {
			return nil, runtimeError("nil or table expected")
		}
		return []Value{args[0]}, nil
	}))
	s.globals.SetString("rawget", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) < 2 || args[0].kind != TableKind {
			return nil, runtimeError("table expected")
		}
		return []Value{args[0].Table().Get(args[1])}, nil
	}))
	s.globals.SetString("rawset", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) < 3 || args[0].kind != TableKind {
			return nil, runtimeError("table expected")
		}
		if err := args[0].Table().Set(args[1], args[2]); err != nil {
			return nil, err
		}
		return []Value{args[0]}, nil
	}))
	s.globals.SetString("rawequal", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) < 2 {
			return []Value{False}, nil
		}
		return []Value{Bool(equal(args[0], args[1]))}, nil
	}))
	s.globals.SetString("unpack", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) < 1 || args[0].kind != TableKind {
			return nil, runtimeError("table expected")
		}
		start, end := 1, args[0].Table().Len()
		if len(args) > 1 {
			n, ok := toNumber(args[1])
			if !ok {
				return nil, runtimeError("number expected")
			}
			start = int(n)
		}
		if len(args) > 2 {
			n, ok := toNumber(args[2])
			if !ok {
				return nil, runtimeError("number expected")
			}
			end = int(n)
		}
		if end < start {
			return nil, nil
		}
		values := make([]Value, end-start+1)
		for i := range values {
			values[i] = args[0].Table().Get(Number(float64(start + i)))
		}
		return values, nil
	}))
	s.globals.SetString("select", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) == 0 {
			return nil, runtimeError("index expected")
		}
		if args[0].kind == StringKind && args[0].StringValue() == "#" {
			return []Value{Number(float64(len(args) - 1))}, nil
		}
		index, ok := toNumber(args[0])
		if !ok || math.Trunc(index) != index || index == 0 {
			return nil, runtimeError("index out of range")
		}
		i := int(index)
		count := len(args) - 1
		if i < 0 {
			i = count + i + 1
		}
		if i < 1 {
			return nil, runtimeError("index out of range")
		}
		if i > count {
			return nil, nil
		}
		return args[i:], nil
	}))
	s.globals.SetString("pcall", Native(func(state *State, args []Value) ([]Value, error) {
		if len(args) < 1 {
			return []Value{False, String("function expected")}, nil
		}
		result, err := state.callValue(args[0], args[1:])
		if err != nil {
			return []Value{False, String(err.Error())}, nil
		}
		values := make([]Value, result.count+1)
		values[0] = True
		for i := 0; i < result.count; i++ {
			values[i+1] = result.at(i)
		}
		return values, nil
	}))
	s.globals.SetString("loadstring", Native(func(state *State, args []Value) ([]Value, error) {
		source, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		name := source
		if len(args) > 1 {
			name, err = needString(args, 1)
			if err != nil {
				return nil, err
			}
		} else if len(name) > 48 {
			name = name[:48]
		}
		fn, loadErr := state.Load(source, name)
		if loadErr != nil {
			return []Value{Nil, String(loadErr.Error())}, nil
		}
		return []Value{fn}, nil
	}))
	s.globals.SetString("getfenv", Native(func(state *State, args []Value) ([]Value, error) {
		env, _, err := state.functionEnvironment(args, false)
		if err != nil {
			return nil, err
		}
		return []Value{TableValue(env)}, nil
	}))
	s.globals.SetString("setfenv", Native(func(state *State, args []Value) ([]Value, error) {
		if len(args) < 2 || args[1].kind != TableKind {
			return nil, runtimeError("table expected")
		}
		_, fn, err := state.functionEnvironment(args[:1], true)
		if err != nil {
			return nil, err
		}
		if fn == nil {
			// Lua 5.1 treats level zero as the running thread's environment.
			state.globals = args[1].Table()
		} else {
			fn.Env = args[1].Table()
		}
		return []Value{args[0]}, nil
	}))

	mathTable := NewTable(0, 32)
	mathTable.SetString("pi", Number(math.Pi))
	mathTable.SetString("huge", Number(math.Inf(1)))
	for name, fn := range map[string]func(float64) float64{"abs": math.Abs, "acos": math.Acos, "asin": math.Asin, "atan": math.Atan, "ceil": math.Ceil, "cos": math.Cos, "cosh": math.Cosh, "deg": func(v float64) float64 { return v * 180 / math.Pi }, "exp": math.Exp, "floor": math.Floor, "log": math.Log, "log10": math.Log10, "rad": func(v float64) float64 { return v * math.Pi / 180 }, "sin": math.Sin, "sinh": math.Sinh, "sqrt": math.Sqrt, "tan": math.Tan, "tanh": math.Tanh} {
		mathTable.SetString(name, FunctionValue(&Function{NativeNumber1: fn}))
	}
	mathTable.SetString("max", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) == 0 {
			return nil, runtimeError("value expected")
		}
		best, ok := toNumber(args[0])
		if !ok {
			return nil, runtimeError("number expected")
		}
		for _, v := range args[1:] {
			n, ok := toNumber(v)
			if !ok {
				return nil, runtimeError("number expected")
			}
			if n > best {
				best = n
			}
		}
		return []Value{Number(best)}, nil
	}))
	mathTable.SetString("min", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) == 0 {
			return nil, runtimeError("value expected")
		}
		best, ok := toNumber(args[0])
		if !ok {
			return nil, runtimeError("number expected")
		}
		for _, v := range args[1:] {
			n, ok := toNumber(v)
			if !ok {
				return nil, runtimeError("number expected")
			}
			if n < best {
				best = n
			}
		}
		return []Value{Number(best)}, nil
	}))
	mathTable.SetString("pow", numericBinary(math.Pow))
	mathTable.SetString("fmod", numericBinary(math.Mod))
	s.globals.SetString("math", TableValue(mathTable))

	stringTable := NewTable(0, 16)
	stringTable.SetString("len", Native(func(_ *State, args []Value) ([]Value, error) {
		v, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		return []Value{Number(float64(len(v)))}, nil
	}))
	stringTable.SetString("lower", stringUnary(strings.ToLower))
	stringTable.SetString("upper", stringUnary(strings.ToUpper))
	stringTable.SetString("reverse", stringUnary(func(v string) string {
		r := []rune(v)
		for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
			r[i], r[j] = r[j], r[i]
		}
		return string(r)
	}))
	stringTable.SetString("rep", Native(func(_ *State, args []Value) ([]Value, error) {
		v, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		if len(args) < 2 {
			return nil, runtimeError("number expected")
		}
		n, ok := toNumber(args[1])
		if !ok {
			return nil, runtimeError("number expected")
		}
		return []Value{String(strings.Repeat(v, int(n)))}, nil
	}))
	stringTable.SetString("format", Native(func(_ *State, args []Value) ([]Value, error) {
		format, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		formatted, err := luaStringFormat(format, args[1:])
		if err != nil {
			return nil, err
		}
		return []Value{String(formatted)}, nil
	}))
	s.globals.SetString("string", TableValue(stringTable))

	tableLib := NewTable(0, 8)
	tableLib.SetString("insert", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) < 2 || args[0].kind != TableKind {
			return nil, runtimeError("table expected")
		}
		t := args[0].Table()
		pos := t.Len() + 1
		value := args[1]
		if len(args) > 2 {
			p, ok := toNumber(args[1])
			if !ok {
				return nil, runtimeError("number expected")
			}
			pos = int(p)
			value = args[2]
			for i := t.Len(); i >= pos; i-- {
				_ = t.Set(Number(float64(i+1)), t.Get(Number(float64(i))))
			}
		}
		return nil, t.Set(Number(float64(pos)), value)
	}))
	tableLib.SetString("concat", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) < 1 || args[0].kind != TableKind {
			return nil, runtimeError("table expected")
		}
		sep := ""
		if len(args) > 1 {
			sep = args[1].Repr()
		}
		t := args[0].Table()
		parts := make([]string, t.Len())
		for i := range parts {
			parts[i] = t.Get(Number(float64(i + 1))).Repr()
		}
		return []Value{String(strings.Join(parts, sep))}, nil
	}))
	s.globals.SetString("table", TableValue(tableLib))

	coroutineTable := NewTable(0, 8)
	createCoroutine := func(state *State, function Value) (*Thread, error) {
		if function.kind != FunctionKind {
			return nil, runtimeError("function expected")
		}
		thread := &Thread{fn: function, resume: make(chan []Value), events: make(chan threadEvent)}
		thread.state = newCoroutineState(state, thread)
		return thread, nil
	}
	coroutineTable.SetString("create", Native(func(state *State, args []Value) ([]Value, error) {
		if len(args) == 0 {
			return nil, runtimeError("function expected")
		}
		thread, err := createCoroutine(state, args[0])
		if err != nil {
			return nil, err
		}
		return []Value{ThreadValue(thread)}, nil
	}))
	coroutineTable.SetString("resume", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) == 0 || args[0].kind != ThreadKind {
			return nil, runtimeError("coroutine expected")
		}
		values, err := args[0].Thread().Resume(args[1:])
		if err != nil {
			return []Value{False, String(err.Error())}, nil
		}
		result := make([]Value, len(values)+1)
		result[0] = True
		copy(result[1:], values)
		return result, nil
	}))
	coroutineTable.SetString("yield", Native(func(state *State, args []Value) ([]Value, error) {
		if state.currentThread == nil {
			return nil, runtimeError("attempt to yield from outside a coroutine")
		}
		return state.currentThread.Yield(args)
	}))
	coroutineTable.SetString("status", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) == 0 || args[0].kind != ThreadKind {
			return nil, runtimeError("coroutine expected")
		}
		return []Value{String(args[0].Thread().status())}, nil
	}))
	coroutineTable.SetString("running", Native(func(state *State, _ []Value) ([]Value, error) {
		if state.currentThread == nil {
			return []Value{Nil}, nil
		}
		return []Value{ThreadValue(state.currentThread)}, nil
	}))
	coroutineTable.SetString("wrap", Native(func(state *State, args []Value) ([]Value, error) {
		if len(args) == 0 {
			return nil, runtimeError("function expected")
		}
		thread, err := createCoroutine(state, args[0])
		if err != nil {
			return nil, err
		}
		return []Value{Native(func(_ *State, resumeArgs []Value) ([]Value, error) {
			return thread.Resume(resumeArgs)
		})}, nil
	}))
	s.globals.SetString("coroutine", TableValue(coroutineTable))
	s.openExtraLibraries()
}

// functionEnvironment implements the Lua 5.1 function-or-stack-level form
// shared by getfenv and setfenv. Native calls do not add a VM frame, so level
// one is the topmost Lua frame.
func (s *State) functionEnvironment(args []Value, setting bool) (*Table, *Function, error) {
	if len(args) == 0 {
		if len(s.frames) == 0 {
			return s.globals, nil, nil
		}
		fn := s.frames[len(s.frames)-1].fn
		if fn.Env == nil {
			return s.globals, fn, nil
		}
		return fn.Env, fn, nil
	}
	target := args[0]
	if target.kind == FunctionKind {
		fn := target.Function()
		if fn.Native != nil || fn.NativeNumber1 != nil || fn.NativeNumber2 != nil {
			return nil, nil, runtimeError("cannot change environment of given object")
		}
		if fn.Env == nil {
			return s.globals, fn, nil
		}
		return fn.Env, fn, nil
	}
	level, ok := toNumber(target)
	if !ok || math.Trunc(level) != level || level < 0 {
		return nil, nil, runtimeError("invalid level")
	}
	if level == 0 {
		return s.globals, nil, nil
	}
	index := len(s.frames) - int(level)
	if index < 0 || index >= len(s.frames) {
		return nil, nil, runtimeError("invalid level")
	}
	fn := s.frames[index].fn
	if fn.Env == nil {
		return s.globals, fn, nil
	}
	return fn.Env, fn, nil
}

func numericBinary(fn func(float64, float64) float64) Value {
	return Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) < 2 {
			return nil, runtimeError("two numbers expected")
		}
		a, aok := toNumber(args[0])
		b, bok := toNumber(args[1])
		if !aok || !bok {
			return nil, runtimeError("number expected")
		}
		return []Value{Number(fn(a, b))}, nil
	})
}
func needString(args []Value, index int) (string, error) {
	if len(args) <= index {
		return "", runtimeError("string expected")
	}
	v := args[index]
	if v.kind == StringKind {
		return v.StringValue(), nil
	}
	if v.kind == NumberKind {
		return strconv.FormatFloat(v.Number(), 'g', -1, 64), nil
	}
	return "", runtimeError("string expected, got %s", v.TypeName())
}
func stringUnary(fn func(string) string) Value {
	return Native(func(_ *State, args []Value) ([]Value, error) {
		v, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		return []Value{String(fn(v))}, nil
	})
}

func luaStringFormat(format string, args []Value) (string, error) {
	var out strings.Builder
	argument := 0
	for i := 0; i < len(format); {
		if format[i] != '%' {
			out.WriteByte(format[i])
			i++
			continue
		}
		if i+1 < len(format) && format[i+1] == '%' {
			out.WriteByte('%')
			i += 2
			continue
		}
		start := i
		i++
		for i < len(format) && strings.ContainsRune("-+ #0", rune(format[i])) {
			i++
		}
		for i < len(format) && format[i] >= '0' && format[i] <= '9' {
			i++
		}
		if i < len(format) && format[i] == '.' {
			i++
			for i < len(format) && format[i] >= '0' && format[i] <= '9' {
				i++
			}
		}
		if i >= len(format) {
			return "", runtimeError("invalid format (ends with '%%')")
		}
		verb := format[i]
		i++
		if argument >= len(args) {
			return "", runtimeError("no value")
		}
		value := args[argument]
		argument++
		spec := format[start:i]
		var rendered string
		switch verb {
		case 'd', 'i', 'o', 'u', 'x', 'X':
			n, ok := toNumber(value)
			if !ok {
				return "", runtimeError("number expected")
			}
			if verb == 'i' || verb == 'u' {
				spec = spec[:len(spec)-1] + "d"
			}
			if verb == 'u' {
				rendered = fmt.Sprintf(spec, uint64(int64(n)))
			} else {
				rendered = fmt.Sprintf(spec, int64(n))
			}
		case 'e', 'E', 'f', 'g', 'G':
			n, ok := toNumber(value)
			if !ok {
				return "", runtimeError("number expected")
			}
			rendered = fmt.Sprintf(spec, n)
		case 'c':
			n, ok := toNumber(value)
			if !ok {
				return "", runtimeError("number expected")
			}
			rendered = fmt.Sprintf(spec, rune(int(n)))
		case 'q':
			s, err := needString([]Value{value}, 0)
			if err != nil {
				return "", err
			}
			rendered = strconv.Quote(s)
		case 's':
			s, err := needString([]Value{value}, 0)
			if err != nil {
				return "", err
			}
			rendered = fmt.Sprintf(spec, s)
		default:
			return "", runtimeError("invalid option '%%%c' to format", verb)
		}
		out.WriteString(rendered)
	}
	return out.String(), nil
}

func (s *State) String() string { return fmt.Sprintf("Lua 5.1 state (%d globals)", len(s.globals.str)) }
