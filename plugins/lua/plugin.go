package lua

import (
	"fmt"
	"math"
	"strings"

	"github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

const scriptObjectType object.ObjectType = 114

type scriptObject struct{ script *Script }

func (*scriptObject) Type() object.ObjectType { return scriptObjectType }
func (s *scriptObject) Inspect() string       { return s.script.Inspect() }

func init() {
	_ = interpreter.RegisterEmbeddedLanguage("lua", evalBlock)
	exports := map[string]interpreter.Object{"run": &object.Builtin{FnWithEnv: builtinRun}, "eval": &object.Builtin{FnWithEnv: builtinEval}, "load": &object.Builtin{FnWithEnv: builtinLoad}, "version": &object.Builtin{Fn: builtinVersion}}
	_ = interpreter.RegisterStdModule("lua", exports)
	eval.RegisterPluginBuiltins(map[string]*object.Builtin{"lua_run": {FnWithEnv: builtinRun}, "lua_eval": {FnWithEnv: builtinEval}, "lua_load": {FnWithEnv: builtinLoad}, "lua_version": {Fn: builtinVersion}})
	previous := eval.DotExpressionHook
	eval.DotExpressionHook = func(left object.Object, name string) object.Object {
		if s, ok := left.(*scriptObject); ok {
			return scriptMethod(s.script, name)
		}
		if previous != nil {
			return previous(left, name)
		}
		return nil
	}
}
func builtinVersion(args ...object.Object) object.Object {
	if len(args) != 0 {
		return object.NewError("lua_version expects no arguments")
	}
	return &object.String{Value: "Lua 5.1 (native Go, no external dependencies)"}
}
func builtinRun(env *object.Environment, args ...object.Object) object.Object {
	return executeBuiltin(env, false, false, args...)
}
func builtinEval(env *object.Environment, args ...object.Object) object.Object {
	return executeBuiltin(env, false, true, args...)
}
func builtinLoad(env *object.Environment, args ...object.Object) object.Object {
	return executeBuiltin(env, true, false, args...)
}
func executeBuiltin(env *object.Environment, persistent, expression bool, args ...object.Object) object.Object {
	if len(args) < 1 || len(args) > 2 {
		return object.NewError("Lua execution expects source [, globals]")
	}
	source, ok := args[0].(*object.String)
	if !ok {
		return object.NewError("Lua source must be STRING")
	}
	code := source.Value
	if expression {
		code = "return " + code
	}
	globals := map[string]Value{}
	if len(args) == 2 && args[1] != object.NULL {
		h, ok := args[1].(*object.Hash)
		if !ok {
			return object.NewError("Lua globals must be HASH")
		}
		for _, pair := range h.Pairs {
			name, ok := pair.Key.(*object.String)
			if !ok {
				return object.NewError("Lua global names must be STRING")
			}
			v, err := fromSPL(pair.Value, env, 0)
			if err != nil {
				return tuple(object.NULL, err)
			}
			globals[name.Value] = v
		}
	}
	script, results, err := LoadScript(code, "=(SPL Lua)", globals)
	if err != nil {
		return tuple(object.NULL, err)
	}
	if persistent {
		return tuple(&scriptObject{script: script}, nil)
	}
	result, err := resultsToSPL(results)
	return tuple(result, err)
}
func evalBlock(ctx interpreter.EmbeddedLanguageContext) object.Object {
	script, results, err := LoadScript(ctx.Code, "=(lua block)", nil)
	_ = script
	if err != nil {
		return object.NewError("lua: %s", err)
	}
	result, err := resultsToSPL(results)
	if err != nil {
		return object.NewError("lua: %s", err)
	}
	return result
}
func scriptMethod(script *Script, name string) object.Object {
	switch strings.ToLower(name) {
	case "call":
		return &object.Builtin{FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("script.call(name [, args...]) requires name")
			}
			n, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("Lua function name must be STRING")
			}
			values := make([]Value, len(args)-1)
			for i, arg := range args[1:] {
				v, err := fromSPL(arg, env, 0)
				if err != nil {
					return tuple(object.NULL, err)
				}
				values[i] = v
			}
			results, err := script.Call(n.Value, values...)
			if err != nil {
				return tuple(object.NULL, err)
			}
			result, err := resultsToSPL(results)
			return tuple(result, err)
		}}
	case "get":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("script.get(name) expects 1 argument")
			}
			n, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("name must be STRING")
			}
			result, err := toSPL(script.Get(n.Value), 0, map[*Table]bool{})
			return tuple(result, err)
		}}
	case "set":
		return &object.Builtin{FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
			if len(args) != 2 {
				return object.NewError("script.set(name, value) expects 2 arguments")
			}
			name, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("name must be STRING")
			}
			value, err := fromSPL(args[1], env, 0)
			if err != nil {
				return tuple(object.NULL, err)
			}
			script.Set(name.Value, value)
			return tuple(args[1], nil)
		}}
	default:
		return object.NewError("Lua script method %q not found", name)
	}
}

func fromSPL(value object.Object, env *object.Environment, depth int) (Value, error) {
	if depth > 128 {
		return Nil, fmt.Errorf("Lua value nesting limit exceeded")
	}
	switch v := value.(type) {
	case nil, *object.Null:
		return Nil, nil
	case *object.Boolean:
		return Bool(v.Value), nil
	case *object.Integer:
		return Number(float64(v.Value)), nil
	case *object.Float:
		return Number(v.Value), nil
	case *object.String:
		return String(v.Value), nil
	case *object.Array:
		t := NewTable(len(v.Elements), 0)
		for i, e := range v.Elements {
			x, err := fromSPL(e, env, depth+1)
			if err != nil {
				return Nil, err
			}
			_ = t.Set(Number(float64(i+1)), x)
		}
		return TableValue(t), nil
	case *object.Hash:
		t := NewTable(0, len(v.Pairs))
		for _, pair := range v.Pairs {
			k, err := fromSPL(pair.Key, env, depth+1)
			if err != nil {
				return Nil, err
			}
			x, err := fromSPL(pair.Value, env, depth+1)
			if err != nil {
				return Nil, err
			}
			if err = t.Set(k, x); err != nil {
				return Nil, err
			}
		}
		return TableValue(t), nil
	case *object.Builtin, *object.Function:
		return Native(func(_ *State, args []Value) ([]Value, error) {
			objects := make([]object.Object, len(args))
			for i, arg := range args {
				x, err := toSPL(arg, 0, map[*Table]bool{})
				if err != nil {
					return nil, err
				}
				objects[i] = x
			}
			if object.ApplyFunctionFn == nil {
				return nil, fmt.Errorf("SPL callback evaluator unavailable")
			}
			result := object.ApplyFunctionFn(value, objects, env)
			if e, ok := result.(*object.Error); ok {
				return nil, fmt.Errorf("%s", e.Inspect())
			}
			x, err := fromSPL(result, env, 0)
			if err != nil {
				return nil, err
			}
			return []Value{x}, nil
		}), nil
	default:
		return Nil, fmt.Errorf("cannot convert SPL %s to Lua", value.Type())
	}
}
func toSPL(v Value, depth int, seen map[*Table]bool) (object.Object, error) {
	if depth > 128 {
		return nil, fmt.Errorf("SPL value nesting limit exceeded")
	}
	switch v.kind {
	case NilKind:
		return object.NULL, nil
	case BoolKind:
		if v.Bool() {
			return object.TRUE, nil
		}
		return object.FALSE, nil
	case NumberKind:
		n := v.Number()
		if math.Trunc(n) == n && n >= math.MinInt64 && n <= math.MaxInt64 {
			return &object.Integer{Value: int64(n)}, nil
		}
		return &object.Float{Value: n}, nil
	case StringKind:
		return &object.String{Value: v.StringValue()}, nil
	case TableKind:
		t := v.Table()
		if seen[t] {
			return nil, fmt.Errorf("cannot convert cyclic Lua table")
		}
		seen[t] = true
		defer delete(seen, t)
		length := t.Len()
		arrayOnly := true
		count := 0
		t.ForEach(func(k, value Value) bool {
			count++
			i, ok := numberKey(k.Number())
			if k.kind != NumberKind || !ok || i < 1 || i > int64(length) {
				arrayOnly = false
			}
			return true
		})
		if arrayOnly && count == length {
			items := make([]object.Object, length)
			for i := range items {
				x, err := toSPL(t.Get(Number(float64(i+1))), depth+1, seen)
				if err != nil {
					return nil, err
				}
				items[i] = x
			}
			return &object.Array{Elements: items}, nil
		}
		pairs := map[object.HashKey]object.HashPair{}
		var conversionErr error
		t.ForEach(func(k, value Value) bool {
			key, err := toSPL(k, depth+1, seen)
			if err != nil {
				conversionErr = err
				return false
			}
			hashable, ok := key.(object.Hashable)
			if !ok {
				key = &object.String{Value: key.Inspect()}
				hashable = key.(object.Hashable)
			}
			x, err := toSPL(value, depth+1, seen)
			if err != nil {
				conversionErr = err
				return false
			}
			pairs[hashable.HashKey()] = object.HashPair{Key: key, Value: x}
			return true
		})
		return &object.Hash{Pairs: pairs}, conversionErr
	default:
		return nil, fmt.Errorf("cannot convert Lua %s to SPL", v.TypeName())
	}
}
func resultsToSPL(results []Value) (object.Object, error) {
	if len(results) == 0 {
		return object.NULL, nil
	}
	if len(results) == 1 {
		return toSPL(results[0], 0, map[*Table]bool{})
	}
	items := make([]object.Object, len(results))
	for i, v := range results {
		x, err := toSPL(v, 0, map[*Table]bool{})
		if err != nil {
			return nil, err
		}
		items[i] = x
	}
	return &object.Array{Elements: items}, nil
}
func tuple(value object.Object, err error) object.Object {
	if value == nil {
		value = object.NULL
	}
	e := object.Object(object.NULL)
	if err != nil {
		e = &object.String{Value: err.Error()}
	}
	return &object.Array{Elements: []object.Object{value, e}}
}
