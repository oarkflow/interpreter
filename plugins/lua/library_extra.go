package lua

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	debugpkg "runtime/debug"
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
	s.openDebugLibrary()
	s.openPackageLibrary()
}

func (s *State) openExtraBase() {
	memoryCount := func() (float64, float64) {
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		return float64(stats.Alloc / 1024), float64(stats.Alloc % 1024)
	}
	s.globals.SetString("gcinfo", Native(func(_ *State, _ []Value) ([]Value, error) {
		kilobytes, bytes := memoryCount()
		return []Value{Number(kilobytes), Number(bytes)}, nil
	}))
	s.globals.SetString("collectgarbage", Native(func(state *State, args []Value) ([]Value, error) {
		option := "collect"
		if len(args) > 0 {
			var err error
			option, err = needString(args, 0)
			if err != nil {
				return nil, err
			}
		}
		switch option {
		case "collect":
			state.collectTables()
			runtime.GC()
			return []Value{Number(0)}, nil
		case "count":
			kilobytes, bytes := memoryCount()
			return []Value{Number(kilobytes + bytes/1024)}, nil
		case "stop":
			state.gcPercent = debugpkg.SetGCPercent(-1)
			return []Value{Number(0)}, nil
		case "restart":
			percent := state.gcPercent
			if percent < 0 {
				percent = 100
			}
			debugpkg.SetGCPercent(percent)
			return []Value{Number(0)}, nil
		case "step":
			state.collectTables()
			return []Value{True}, nil
		case "setpause":
			previous := state.gcPause
			if len(args) > 1 {
				n, err := needNumber(args, 1)
				if err != nil {
					return nil, err
				}
				state.gcPause = int(n)
			}
			return []Value{Number(float64(previous))}, nil
		case "setstepmul":
			previous := state.gcStepMul
			if len(args) > 1 {
				n, err := needNumber(args, 1)
				if err != nil {
					return nil, err
				}
				state.gcStepMul = int(n)
			}
			return []Value{Number(float64(previous))}, nil
		default:
			return nil, runtimeError("invalid option '%s'", option)
		}
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
	lib.SetString("find", Native(func(_ *State, args []Value) ([]Value, error) {
		source, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		pattern, err := needString(args, 1)
		if err != nil {
			return nil, err
		}
		init := patternInitialIndex(args, 2, len(source))
		plain := len(args) > 3 && args[3].Truthy()
		if plain {
			if init > len(source)+1 {
				return []Value{Nil}, nil
			}
			index := strings.Index(source[init-1:], pattern)
			if index < 0 {
				return []Value{Nil}, nil
			}
			start := init + index
			return []Value{Number(float64(start)), Number(float64(start + len(pattern) - 1))}, nil
		}
		compiled, err := compilePattern(source, pattern)
		if err != nil {
			return nil, err
		}
		result, ok := compiled.find(init)
		if !ok {
			return []Value{Nil}, nil
		}
		values := []Value{Number(float64(result.start + 1)), Number(float64(result.end))}
		if len(result.captures) > 0 {
			values = append(values, patternCaptureValues(source, result)...)
		}
		return values, nil
	}))
	lib.SetString("match", Native(func(_ *State, args []Value) ([]Value, error) {
		source, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		pattern, err := needString(args, 1)
		if err != nil {
			return nil, err
		}
		compiled, err := compilePattern(source, pattern)
		if err != nil {
			return nil, err
		}
		result, ok := compiled.find(patternInitialIndex(args, 2, len(source)))
		if !ok {
			return []Value{Nil}, nil
		}
		return patternCaptureValues(source, result), nil
	}))
	lib.SetString("gmatch", Native(func(_ *State, args []Value) ([]Value, error) {
		source, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		pattern, err := needString(args, 1)
		if err != nil {
			return nil, err
		}
		compiled, err := compilePattern(source, pattern)
		if err != nil {
			return nil, err
		}
		next := 1
		return []Value{Native(func(_ *State, _ []Value) ([]Value, error) {
			if next > len(source)+1 {
				return nil, nil
			}
			result, ok := compiled.find(next)
			if !ok {
				next = len(source) + 2
				return nil, nil
			}
			next = result.end + 1
			if result.end == result.start {
				next++
			}
			return patternCaptureValues(source, result), nil
		})}, nil
	}))
	lib.SetString("gfind", lib.GetString("gmatch"))
	lib.SetString("gsub", Native(func(state *State, args []Value) ([]Value, error) {
		source, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		pattern, err := needString(args, 1)
		if err != nil {
			return nil, err
		}
		if len(args) < 3 {
			return nil, runtimeError("value expected")
		}
		limit := len(source) + 1
		if len(args) > 3 {
			n, numberErr := needNumber(args, 3)
			if numberErr != nil {
				return nil, numberErr
			}
			limit = int(n)
		}
		compiled, err := compilePattern(source, pattern)
		if err != nil {
			return nil, err
		}
		var output strings.Builder
		cursor, search, count := 0, 0, 0
		for search <= len(source) && count < limit {
			result, ok := compiled.find(search + 1)
			if !ok {
				break
			}
			output.WriteString(source[cursor:result.start])
			replacement, replace, replacementErr := patternReplacement(state, args[2], source, result)
			if replacementErr != nil {
				return nil, replacementErr
			}
			if replace {
				output.WriteString(replacement)
			} else {
				output.WriteString(source[result.start:result.end])
			}
			count++
			cursor, search = result.end, result.end
			if result.end == result.start {
				if cursor < len(source) {
					output.WriteByte(source[cursor])
					cursor++
				}
				search++
			}
		}
		output.WriteString(source[cursor:])
		return []Value{String(output.String()), Number(float64(count))}, nil
	}))
	lib.SetString("byte", Native(func(_ *State, args []Value) ([]Value, error) {
		str, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		start, end := luaStringRange(args, len(str))
		if start < 1 {
			if len(args) <= 2 {
				return nil, nil
			}
			start = 1
		}
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

func patternInitialIndex(args []Value, index, length int) int {
	initial := 1
	if len(args) > index {
		if n, ok := toNumber(args[index]); ok {
			initial = int(n)
		}
	}
	if initial < 0 {
		initial = length + initial + 1
	}
	if initial < 1 {
		initial = 1
	}
	return initial
}

func patternReplacement(state *State, replacement Value, source string, result patternResult) (string, bool, error) {
	for _, capture := range result.captures {
		if !capture.position && capture.end < capture.start {
			return "", false, runtimeError("unfinished capture in pattern result")
		}
	}
	values := patternCaptureValues(source, result)
	key := values[0]
	if len(result.captures) == 0 {
		key = String(source[result.start:result.end])
	}
	switch replacement.kind {
	case StringKind, NumberKind:
		template := valueString(replacement)
		var output strings.Builder
		for i := 0; i < len(template); i++ {
			if template[i] != '%' || i+1 >= len(template) {
				output.WriteByte(template[i])
				continue
			}
			i++
			if template[i] == '%' {
				output.WriteByte('%')
				continue
			}
			if template[i] < '0' || template[i] > '9' {
				return "", false, runtimeError("invalid use of '%%' in replacement string")
			}
			capture := int(template[i] - '1')
			if template[i] == '0' {
				output.WriteString(source[result.start:result.end])
			} else if template[i] == '1' && len(result.captures) == 0 {
				output.WriteString(source[result.start:result.end])
			} else if capture >= 0 && capture < len(result.captures) {
				output.WriteString(values[capture].Repr())
			} else {
				return "", false, runtimeError("invalid capture index")
			}
		}
		return output.String(), true, nil
	case TableKind:
		value, indexErr := state.index(replacement, key, 0)
		if indexErr != nil {
			return "", false, indexErr
		}
		if value.kind == NilKind || value.kind == BoolKind && !value.Bool() {
			return "", false, nil
		}
		if value.kind != StringKind && value.kind != NumberKind {
			return "", false, runtimeError("invalid replacement value (a %s)", value.TypeName())
		}
		return valueString(value), true, nil
	case FunctionKind:
		called, err := state.callValue(replacement, values)
		if err != nil {
			return "", false, err
		}
		if called.count == 0 || called.at(0).kind == NilKind || called.at(0).kind == BoolKind && !called.at(0).Bool() {
			return "", false, nil
		}
		value := called.at(0)
		if value.kind != StringKind && value.kind != NumberKind {
			return "", false, runtimeError("invalid replacement value (a %s)", value.TypeName())
		}
		return valueString(value), true, nil
	default:
		return "", false, runtimeError("string/function/table expected")
	}
}

func (s *State) openExtraTable() {
	lib := s.globals.GetString("table").Table()
	lib.SetString("getn", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) == 0 || args[0].kind != TableKind {
			return nil, runtimeError("table expected")
		}
		return []Value{Number(float64(args[0].Table().Len()))}, nil
	}))
	lib.SetString("setn", Native(func(_ *State, _ []Value) ([]Value, error) {
		return nil, runtimeError("'setn' is obsolete")
	}))
	lib.SetString("foreachi", Native(func(state *State, args []Value) ([]Value, error) {
		if len(args) < 2 || args[0].kind != TableKind || args[1].kind != FunctionKind {
			return nil, runtimeError("table and function expected")
		}
		table := args[0].Table()
		for i := 1; i <= table.Len(); i++ {
			result, err := state.callValue(args[1], []Value{Number(float64(i)), table.Get(Number(float64(i)))})
			if err != nil {
				return nil, err
			}
			if result.count > 0 && result.at(0).kind != NilKind {
				return []Value{result.at(0)}, nil
			}
		}
		return nil, nil
	}))
	lib.SetString("foreach", Native(func(state *State, args []Value) ([]Value, error) {
		if len(args) < 2 || args[0].kind != TableKind || args[1].kind != FunctionKind {
			return nil, runtimeError("table and function expected")
		}
		var returned Value
		var callbackErr error
		args[0].Table().ForEach(func(key, value Value) bool {
			result, err := state.callValue(args[1], []Value{key, value})
			if err != nil {
				callbackErr = err
				return false
			}
			if result.count > 0 && result.at(0).kind != NilKind {
				returned = result.at(0)
				return false
			}
			return true
		})
		if callbackErr != nil {
			return nil, callbackErr
		}
		if returned.kind != NilKind {
			return []Value{returned}, nil
		}
		return nil, nil
	}))
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
	lib.SetString("time", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) == 0 || args[0].kind == NilKind {
			return []Value{Number(float64(time.Now().Unix()))}, nil
		}
		if args[0].kind != TableKind {
			return nil, runtimeError("table expected")
		}
		table := args[0].Table()
		field := func(name string, fallback int) (int, error) {
			value := table.GetString(name)
			if value.kind == NilKind {
				return fallback, nil
			}
			n, ok := toNumber(value)
			if !ok {
				return 0, runtimeError("field '%s' is not a number", name)
			}
			return int(n), nil
		}
		year, err := field("year", 0)
		if err != nil {
			return nil, err
		}
		month, err := field("month", 0)
		if err != nil {
			return nil, err
		}
		day, err := field("day", 0)
		if err != nil {
			return nil, err
		}
		hour, err := field("hour", 12)
		if err != nil {
			return nil, err
		}
		minute, err := field("min", 0)
		if err != nil {
			return nil, err
		}
		second, err := field("sec", 0)
		if err != nil {
			return nil, err
		}
		return []Value{Number(float64(time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local).Unix()))}, nil
	}))
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
	lib.SetString("date", Native(func(state *State, args []Value) ([]Value, error) {
		format := "%c"
		if len(args) > 0 && args[0].kind != NilKind {
			var err error
			format, err = needString(args, 0)
			if err != nil {
				return nil, err
			}
		}
		stamp := time.Now().Unix()
		if len(args) > 1 && args[1].kind != NilKind {
			n, err := needNumber(args, 1)
			if err != nil {
				return nil, err
			}
			stamp = int64(n)
		}
		when := time.Unix(stamp, 0)
		if strings.HasPrefix(format, "!") {
			when = when.UTC()
			format = format[1:]
		} else {
			when = when.In(time.Local)
		}
		if format == "*t" {
			table := state.newTable(0, 10)
			table.SetString("year", Number(float64(when.Year())))
			table.SetString("month", Number(float64(when.Month())))
			table.SetString("day", Number(float64(when.Day())))
			table.SetString("hour", Number(float64(when.Hour())))
			table.SetString("min", Number(float64(when.Minute())))
			table.SetString("sec", Number(float64(when.Second())))
			table.SetString("wday", Number(float64(when.Weekday()+1)))
			table.SetString("yday", Number(float64(when.YearDay())))
			table.SetString("isdst", False)
			return []Value{TableValue(table)}, nil
		}
		return []Value{String(formatLuaDate(format, when))}, nil
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
	lib.SetString("setlocale", Native(func(_ *State, args []Value) ([]Value, error) {
		locale := ""
		if len(args) > 0 && args[0].kind != NilKind {
			var err error
			locale, err = needString(args, 0)
			if err != nil {
				return nil, err
			}
		}
		if locale == "" || locale == "C" || locale == "POSIX" {
			return []Value{String("C")}, nil
		}
		return []Value{Nil}, nil
	}))
	lib.SetString("tmpname", Native(func(_ *State, _ []Value) ([]Value, error) {
		file, err := os.CreateTemp("", "lua-*")
		if err != nil {
			return nil, err
		}
		name := file.Name()
		if err = file.Close(); err != nil {
			return nil, err
		}
		return []Value{String(name)}, nil
	}))
	lib.SetString("remove", Native(func(_ *State, args []Value) ([]Value, error) {
		name, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		return ioFailure(os.Remove(name))
	}))
	lib.SetString("rename", Native(func(_ *State, args []Value) ([]Value, error) {
		oldName, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		newName, err := needString(args, 1)
		if err != nil {
			return nil, err
		}
		return ioFailure(os.Rename(oldName, newName))
	}))
	lib.SetString("execute", Native(func(_ *State, args []Value) ([]Value, error) {
		if len(args) == 0 || args[0].kind == NilKind {
			return []Value{Number(1)}, nil
		}
		command, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		runErr := exec.Command("sh", "-c", command).Run()
		if runErr == nil {
			return []Value{Number(0)}, nil
		}
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			return []Value{Number(float64(exitErr.ExitCode()))}, nil
		}
		return []Value{Number(-1)}, nil
	}))
	s.globals.SetString("os", TableValue(lib))
}

func formatLuaDate(format string, when time.Time) string {
	var out strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			out.WriteByte(format[i])
			continue
		}
		i++
		var value string
		switch format[i] {
		case '%':
			value = "%"
		case 'Y':
			value = strconv.Itoa(when.Year())
		case 'm':
			value = fmtTwo(int(when.Month()))
		case 'd':
			value = fmtTwo(when.Day())
		case 'H':
			value = fmtTwo(when.Hour())
		case 'M':
			value = fmtTwo(when.Minute())
		case 'S':
			value = fmtTwo(when.Second())
		case 'w':
			value = strconv.Itoa(int(when.Weekday()))
		case 'j':
			value = fmtThree(when.YearDay())
		case 'c':
			value = when.Format("Mon Jan _2 15:04:05 2006")
		default:
			value = "%" + string(format[i])
		}
		out.WriteString(value)
	}
	return out.String()
}

func fmtTwo(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

func fmtThree(value int) string {
	if value < 10 {
		return "00" + strconv.Itoa(value)
	}
	if value < 100 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

func (s *State) openPackageLibrary() {
	loaded, preload := NewTable(0, 16), NewTable(0, 8)
	lib := NewTable(0, 8)
	lib.SetString("loaded", TableValue(loaded))
	lib.SetString("preload", TableValue(preload))
	lib.SetString("path", String("./?.lua;./?/init.lua"))
	lib.SetString("cpath", String(""))
	lib.SetString("loadlib", Native(func(_ *State, args []Value) ([]Value, error) {
		name := "dynamic libraries are unavailable in the pure-Go Lua plugin"
		if len(args) > 0 && args[0].kind == StringKind {
			name = "cannot load " + args[0].StringValue() + ": pure-Go build"
		}
		return []Value{Nil, String(name), String("open")}, nil
	}))
	lib.SetString("seeall", Native(func(state *State, args []Value) ([]Value, error) {
		if len(args) == 0 || args[0].kind != TableKind {
			return nil, runtimeError("table expected")
		}
		meta := args[0].Table().Metatable()
		if meta == nil {
			meta = state.newTable(0, 1)
			args[0].Table().SetMetatable(meta)
		}
		meta.SetString("__index", TableValue(state.globals))
		return nil, nil
	}))
	s.globals.SetString("package", TableValue(lib))
	for _, name := range []string{"_G", "string", "table", "math", "io", "os", "coroutine", "debug", "package"} {
		if value := s.globals.GetString(name); value.kind != NilKind {
			loaded.SetString(name, value)
		}
	}
	s.globals.SetString("module", Native(func(state *State, args []Value) ([]Value, error) {
		name, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		module := loaded.GetString(name)
		if module.kind != TableKind {
			current := state.globals
			parts := strings.Split(name, ".")
			for _, part := range parts {
				child := current.GetString(part)
				if child.kind == NilKind {
					table := state.newTable(0, 8)
					child = TableValue(table)
					current.SetString(part, child)
				} else if child.kind != TableKind {
					return nil, runtimeError("name conflict for module '%s'", name)
				}
				current = child.Table()
			}
			module = TableValue(current)
			loaded.SetString(name, module)
		}
		packageName := ""
		if dot := strings.LastIndex(name, "."); dot >= 0 {
			packageName = name[:dot+1]
		}
		module.Table().SetString("_M", module)
		module.Table().SetString("_NAME", String(name))
		module.Table().SetString("_PACKAGE", String(packageName))
		if len(state.frames) > 0 {
			state.frames[len(state.frames)-1].fn.Env = module.Table()
		}
		for _, option := range args[1:] {
			if option.kind != FunctionKind {
				return nil, runtimeError("function expected")
			}
			if _, err = state.callValue(option, []Value{module}); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}))
	s.globals.SetString("require", Native(func(state *State, args []Value) ([]Value, error) {
		name, err := needString(args, 0)
		if err != nil {
			return nil, err
		}
		if value := loaded.GetString(name); value.Truthy() {
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
		return strconv.FormatFloat(value.Number(), 'g', 14, 64)
	}
	return value.Repr()
}
