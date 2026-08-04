package lua

import (
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *State) openExtraLibraries() {
	s.openExtraBase()
	s.openExtraMath()
	s.openExtraString()
	s.openExtraTable()
	s.openOSLibrary()
	s.openIOLibrary()
	s.openPackageLibrary()
}

func (s *State) openExtraBase() {
	s.globals.SetString("collectgarbage", Native(func(state *State, _ []Value) ([]Value, error) {
		state.collectTables()
		return []Value{Number(0)}, nil
	}))
	loadFile := Native(func(state *State, args []Value) ([]Value, error) {
		name, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		source, readErr := os.ReadFile(name)
		if readErr != nil {
			return []Value{Nil, String(readErr.Error())}, nil
		}
		fn, loadErr := state.Load(string(source), "@"+name)
		if loadErr != nil {
			return []Value{Nil, String(loadErr.Error())}, nil
		}
		return []Value{fn}, nil
	})
	s.globals.SetString("loadfile", loadFile)
	s.globals.SetString("dofile", Native(func(state *State, args []Value) ([]Value, error) {
		loaded, err := state.callValue(loadFile, args)
		if err != nil {
			return nil, err
		}
		if loaded.count == 0 || loaded.at(0).kind != FunctionKind {
			if loaded.count > 1 {
				return nil, runtimeError("%s", loaded.at(1).Repr())
			}
			return nil, runtimeError("cannot load file")
		}
		result, err := state.callValue(loaded.at(0), nil)
		if err != nil {
			return nil, err
		}
		return result.slice(), nil
	}))
}

func (s *State) openExtraMath() {
	lib := s.globals.GetString("math").Table()
	lib.SetString("atan2", numericBinary(math.Atan2))
	lib.SetString("ldexp", Native(func(_ *State, args []Value) ([]Value, error) {
		fraction, err := needNumber(args, 0)
		if err != nil {
			return nil, err
		}
		exponent, err := needNumber(args, 1)
		if err != nil {
			return nil, err
		}
		return []Value{Number(math.Ldexp(fraction, int(exponent)))}, nil
	}))
	lib.SetString("modf", Native(func(_ *State, args []Value) ([]Value, error) {
		n, err := needNumber(args, 0)
		if err != nil {
			return nil, err
		}
		integer, fraction := math.Modf(n)
		return []Value{Number(integer), Number(fraction)}, nil
	}))
	lib.SetString("frexp", Native(func(_ *State, args []Value) ([]Value, error) {
		n, err := needNumber(args, 0)
		if err != nil {
			return nil, err
		}
		fraction, exponent := math.Frexp(n)
		return []Value{Number(fraction), Number(float64(exponent))}, nil
	}))
	lib.SetString("randomseed", Native(func(state *State, args []Value) ([]Value, error) {
		n, err := needNumber(args, 0)
		if err != nil {
			return nil, err
		}
		state.randomState = uint64(n)
		if state.randomState == 0 {
			state.randomState = 1
		}
		return nil, nil
	}))
	lib.SetString("random", Native(func(state *State, args []Value) ([]Value, error) {
		state.randomState ^= state.randomState << 13
		state.randomState ^= state.randomState >> 7
		state.randomState ^= state.randomState << 17
		r := float64(state.randomState>>11) * (1.0 / (1 << 53))
		if len(args) == 0 {
			return []Value{Number(r)}, nil
		}
		high, err := needNumber(args, len(args)-1)
		if err != nil {
			return nil, err
		}
		low := float64(1)
		if len(args) > 1 {
			low, err = needNumber(args, 0)
			if err != nil {
				return nil, err
			}
		}
		if low > high {
			return nil, runtimeError("interval is empty")
		}
		return []Value{Number(math.Floor(r*(high-low+1)) + low)}, nil
	}))
}

func (s *State) openExtraString() {
	lib := s.globals.GetString("string").Table()
	lib.SetString("byte", Native(func(_ *State, args []Value) ([]Value, error) {
		str, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		start, end := luaStringRange(args, len(str))
		if start > end || start > len(str) {
			return nil, nil
		}
		if end > len(str) {
			end = len(str)
		}
		out := make([]Value, end-start+1)
		for i := range out {
			out[i] = Number(float64(str[start-1+i]))
		}
		return out, nil
	}))
	lib.SetString("char", Native(func(_ *State, args []Value) ([]Value, error) {
		out := make([]byte, len(args))
		for i := range args {
			n, err := needNumber(args, i)
			if err != nil || n < 0 || n > 255 {
				return nil, runtimeError("value out of range")
			}
			out[i] = byte(n)
		}
		return []Value{String(string(out))}, nil
	}))
	lib.SetString("sub", Native(func(_ *State, args []Value) ([]Value, error) {
		str, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		start, end := luaStringRange(args, len(str))
		if len(args) <= 2 {
			end = len(str)
		}
		if start < 1 {
			start = 1
		}
		if end > len(str) {
			end = len(str)
		}
		if start > end {
			return []Value{String("")}, nil
		}
		return []Value{String(str[start-1 : end])}, nil
	}))
}

func (s *State) openExtraTable() {
	lib := s.globals.GetString("table").Table()
	lib.SetString("remove", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) == 0 || args[0].kind != TableKind {
			return nil, runtimeError("table expected")
		}
		t := args[0].Table()
		pos := t.Len()
		if len(args) > 1 {
			n, err := needNumber(args, 1)
			if err != nil {
				return nil, err
			}
			pos = int(n)
		}
		if pos < 1 || pos > t.Len() {
			return []Value{Nil}, nil
		}
		removed := t.Get(Number(float64(pos)))
		for i := pos; i < t.Len(); i++ {
			_ = t.Set(Number(float64(i)), t.Get(Number(float64(i+1))))
		}
		_ = t.Set(Number(float64(t.Len())), Nil)
		return []Value{removed}, nil
	}))
	lib.SetString("maxn", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) == 0 || args[0].kind != TableKind {
			return nil, runtimeError("table expected")
		}
		max := float64(0)
		args[0].Table().ForEach(func(key, _ Value) bool {
			if key.kind == NumberKind && key.Number() > max {
				max = key.Number()
			}
			return true
		})
		return []Value{Number(max)}, nil
	}))
	lib.SetString("sort", Native(func(state *State, args []Value) ([]Value, error) {
		if len(args) == 0 || args[0].kind != TableKind {
			return nil, runtimeError("table expected")
		}
		t := args[0].Table()
		values := make([]Value, t.Len())
		for i := range values {
			values[i] = t.Get(Number(float64(i + 1)))
		}
		var compareErr error
		sort.SliceStable(values, func(i, j int) bool {
			if compareErr != nil {
				return false
			}
			if len(args) > 1 {
				result, err := state.callValue(args[1], []Value{values[i], values[j]})
				if err != nil {
					compareErr = err
					return false
				}
				return result.count > 0 && result.at(0).Truthy()
			}
			left, right := values[i], values[j]
			if left.kind == NumberKind && right.kind == NumberKind {
				return left.Number() < right.Number()
			}
			if left.kind == StringKind && right.kind == StringKind {
				return left.StringValue() < right.StringValue()
			}
			result, ok, err := state.binaryMetamethod(left, right, "__lt")
			if err != nil {
				compareErr = err
				return false
			}
			if !ok {
				compareErr = runtimeError("attempt to compare %s with %s", left.TypeName(), right.TypeName())
				return false
			}
			return result.Truthy()
		})
		if compareErr != nil {
			return nil, compareErr
		}
		for i, value := range values {
			_ = t.Set(Number(float64(i+1)), value)
		}
		return nil, nil
	}))
}

func (s *State) openOSLibrary() {
	started := time.Now()
	lib := NewTable(0, 12)
	lib.SetString("clock", Native(func(_ *State, _ []Value) ([]Value, error) { return []Value{Number(time.Since(started).Seconds())}, nil }))
	lib.SetString("time", Native(func(_ *State, _ []Value) ([]Value, error) { return []Value{Number(float64(time.Now().Unix()))}, nil }))
	lib.SetString("difftime", Native(func(_ *State, args []Value) ([]Value, error) {
		a, err := needNumber(args, 0)
		if err != nil {
			return nil, err
		}
		b, err := needNumber(args, 1)
		if err != nil {
			return nil, err
		}
		return []Value{Number(a - b)}, nil
	}))
	lib.SetString("getenv", Native(func(_ *State, args []Value) ([]Value, error) {
		name, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		value, ok := os.LookupEnv(name)
		if !ok {
			return []Value{Nil}, nil
		}
		return []Value{String(value)}, nil
	}))
	s.globals.SetString("os", TableValue(lib))
}

func (s *State) openIOLibrary() {
	lib := NewTable(0, 8)
	lib.SetString("write", Native(func(state *State, args []Value) ([]Value, error) {
		writer := state.Output
		if writer == nil {
			writer = os.Stdout
		}
		for _, value := range args {
			if _, err := io.WriteString(writer, value.Repr()); err != nil {
				return []Value{Nil, String(err.Error())}, nil
			}
		}
		return []Value{True}, nil
	}))
	lib.SetString("read", Native(func(_ *State, _ []Value) ([]Value, error) { return []Value{Nil}, nil }))
	s.globals.SetString("io", TableValue(lib))
}

func (s *State) openPackageLibrary() {
	loaded, preload := NewTable(0, 16), NewTable(0, 8)
	lib := NewTable(0, 8)
	lib.SetString("loaded", TableValue(loaded))
	lib.SetString("preload", TableValue(preload))
	lib.SetString("path", String("./?.lua;./?/init.lua"))
	s.globals.SetString("package", TableValue(lib))
	s.globals.SetString("require", Native(func(state *State, args []Value) ([]Value, error) {
		name, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		if value := loaded.GetString(name); value.kind != NilKind {
			return []Value{value}, nil
		}
		loader := preload.GetString(name)
		var failures []string
		if loader.kind == NilKind {
			pathValue := lib.GetString("path")
			path := "./?.lua;./?/init.lua"
			if pathValue.kind == StringKind {
				path = pathValue.StringValue()
			}
			modulePath := strings.ReplaceAll(name, ".", string(filepath.Separator))
			for _, pattern := range strings.Split(path, ";") {
				fileName := strings.ReplaceAll(pattern, "?", modulePath)
				source, readErr := os.ReadFile(fileName)
				if readErr != nil {
					failures = append(failures, "no file '"+fileName+"'")
					continue
				}
				loader, err = state.Load(string(source), "@"+fileName)
				if err != nil {
					return nil, err
				}
				break
			}
		}
		if loader.kind != FunctionKind {
			return nil, runtimeError("module '%s' not found:\n\t%s", name, strings.Join(failures, "\n\t"))
		}
		loaded.SetString(name, True)
		result, err := state.callValue(loader, []Value{String(name)})
		if err != nil {
			loaded.SetString(name, Nil)
			return nil, err
		}
		value := loaded.GetString(name)
		if result.count > 0 && result.at(0).kind != NilKind {
			value = result.at(0)
			loaded.SetString(name, value)
		}
		return []Value{value}, nil
	}))
}

func needNumber(args []Value, index int) (float64, error) {
	if len(args) <= index {
		return 0, runtimeError("number expected")
	}
	n, ok := toNumber(args[index])
	if !ok {
		return 0, runtimeError("number expected")
	}
	return n, nil
}

func luaStringRange(args []Value, length int) (int, int) {
	start, end := 1, 1
	if len(args) > 1 {
		if n, ok := toNumber(args[1]); ok {
			start = int(n)
		}
	}
	if len(args) > 2 {
		if n, ok := toNumber(args[2]); ok {
			end = int(n)
		}
	} else if len(args) > 1 {
		end = start
	}
	if start < 0 {
		start = length + start + 1
	}
	if end < 0 {
		end = length + end + 1
	}
	return start, end
}

func parseInteger(value Value) (int, bool) {
	n, ok := toNumber(value)
	return int(n), ok && math.Trunc(n) == n
}

func valueString(value Value) string {
	if value.kind == StringKind {
		return value.StringValue()
	}
	if value.kind == NumberKind {
		return strconv.FormatFloat(value.Number(), 'g', -1, 64)
	}
	return value.Repr()
}
