package eval

import (
	"math"
	"sort"
	"strings"

	"github.com/oarkflow/interpreter/pkg/object"
)

// ---------------------------------------------------------------------------
// Dot expression dispatcher
// ---------------------------------------------------------------------------

func evalDotExpression(left object.Object, name string) object.Object {
	if imm, ok := left.(*object.ImmutableValue); ok {
		left = imm.Inner
	}
	if gen, ok := left.(*object.GeneratorValue); ok {
		left = &object.Array{Elements: gen.Elements}
	}

	// 1. Property Access for Hash
	if hash, ok := left.(*object.Hash); ok {
		key := &object.String{Value: name}
		hashed := key.HashKey()
		if pair, ok := hash.Pairs[hashed]; ok {
			return pair.Value
		}
		if method := getHashMethod(hash, name); method != nil {
			return method
		}
		return object.NULL
	}

	// 2. Method Access on built-in types
	switch obj := left.(type) {
	case *object.Array:
		return getArrayMethod(obj, name)
	case *object.String:
		return getStringMethod(obj, name)
	case *object.Integer:
		return getIntegerMethod(obj, name)
	case *object.Float:
		return getFloatMethod(obj, name)
	}

	// 3. Extensible hook for types defined outside this package
	if DotExpressionHook != nil {
		if result := DotExpressionHook(left, name); result != nil {
			return result
		}
	}

	return object.NewError("property or method '%s' not found on %s", name, left.Type())
}

// ---------------------------------------------------------------------------
// bindMethod — binds a receiver as the first arg to a named builtin
// ---------------------------------------------------------------------------

func bindMethod(receiver object.Object, methodName, builtinName string) object.Object {
	b, ok := Builtins[builtinName]
	if !ok {
		return object.NewError("method '%s' is unavailable", methodName)
	}
	return &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			callArgs := make([]object.Object, 0, len(args)+1)
			callArgs = append(callArgs, receiver)
			callArgs = append(callArgs, args...)
			return b.Fn(callArgs...)
		},
	}
}

func methodNoArg(receiver object.Object, name string, fn func() object.Object) object.Object {
	return &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 0 {
				return object.NewError("%s expects 0 arguments, got %d", name, len(args))
			}
			return fn()
		},
	}
}

func bindIntegerTimeMethod(ts *object.Integer, methodName, builtinName string) object.Object {
	b, ok := Builtins[builtinName]
	if !ok {
		return object.NewError("method '%s' is unavailable", methodName)
	}
	return &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			callArgs := make([]object.Object, 0, len(args)+1)
			callArgs = append(callArgs, ts)
			callArgs = append(callArgs, args...)
			return b.Fn(callArgs...)
		},
	}
}

// ---------------------------------------------------------------------------
// Hash methods
// ---------------------------------------------------------------------------

func getHashMethod(hash *object.Hash, name string) object.Object {
	switch name {
	case "keys":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			keys := make([]object.Object, 0, len(hash.Pairs))
			for _, pair := range hash.Pairs {
				keys = append(keys, pair.Key)
			}
			return &object.Array{Elements: keys}
		}}
	case "values":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			values := make([]object.Object, 0, len(hash.Pairs))
			for _, pair := range hash.Pairs {
				values = append(values, pair.Value)
			}
			return &object.Array{Elements: values}
		}}
	case "entries":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			entries := make([]object.Object, 0, len(hash.Pairs))
			for _, pair := range hash.Pairs {
				entries = append(entries, &object.Array{Elements: []object.Object{pair.Key, pair.Value}})
			}
			return &object.Array{Elements: entries}
		}}
	case "length":
		return &object.Integer{Value: int64(len(hash.Pairs))}
	default:
		return nil
	}
}

// ---------------------------------------------------------------------------
// String methods
// ---------------------------------------------------------------------------

func getStringMethod(str *object.String, name string) object.Object {
	switch name {
	case "length":
		return &object.Integer{Value: int64(len([]rune(str.Value)))}
	case "upper", "toUpperCase":
		return bindMethod(str, name, "upper")
	case "lower", "toLowerCase":
		return bindMethod(str, name, "lower")
	case "trim":
		return bindMethod(str, name, "trim")
	case "starts_with", "startsWith":
		return bindMethod(str, name, "starts_with")
	case "ends_with", "endsWith":
		return bindMethod(str, name, "ends_with")
	case "includes":
		return bindMethod(str, name, "contains")
	case "replace":
		return bindMethod(str, name, "replace")
	case "repeat":
		return bindMethod(str, name, "repeat")
	case "substring":
		return bindMethod(str, name, "substring")
	case "index_of", "indexOf":
		return bindMethod(str, name, "index_of")
	case "split":
		return bindMethod(str, name, "split")
	case "title":
		return bindMethod(str, name, "title")
	case "slug":
		return bindMethod(str, name, "slug")
	case "snake_case":
		return bindMethod(str, name, "snake_case")
	case "kebab_case":
		return bindMethod(str, name, "kebab_case")
	case "camel_case":
		return bindMethod(str, name, "camel_case")
	case "pascal_case":
		return bindMethod(str, name, "pascal_case")
	case "swap_case":
		return bindMethod(str, name, "swap_case")
	case "count_substr":
		return bindMethod(str, name, "count_substr")
	case "truncate":
		return bindMethod(str, name, "truncate")
	case "split_lines":
		return bindMethod(str, name, "split_lines")
	case "regex_match":
		return bindMethod(str, name, "regex_match")
	case "regex_replace":
		return bindMethod(str, name, "regex_replace")
	case "trim_prefix":
		return bindMethod(str, name, "trim_prefix")
	case "trim_suffix":
		return bindMethod(str, name, "trim_suffix")
	case "pad_left", "padStart":
		return bindMethod(str, name, "pad_left")
	case "pad_right", "padEnd":
		return bindMethod(str, name, "pad_right")
	case "charAt":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("charAt() takes exactly 1 argument")
			}
			idx, ok := args[0].(*object.Integer)
			if !ok {
				return object.NewError("charAt() argument must be integer")
			}
			runes := []rune(str.Value)
			i := int(idx.Value)
			if i < 0 || i >= len(runes) {
				return &object.String{Value: ""}
			}
			return &object.String{Value: string(runes[i])}
		}}
	case "at":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("at() takes exactly 1 argument")
			}
			idx, ok := args[0].(*object.Integer)
			if !ok {
				return object.NewError("at() argument must be integer")
			}
			runes := []rune(str.Value)
			i := int(idx.Value)
			if i < 0 {
				i = len(runes) + i
			}
			if i < 0 || i >= len(runes) {
				return object.NULL
			}
			return &object.String{Value: string(runes[i])}
		}}
	default:
		return object.NewError("method '%s' not found on STRING", name)
	}
}

// ---------------------------------------------------------------------------
// Integer methods
// ---------------------------------------------------------------------------

func getIntegerMethod(num *object.Integer, name string) object.Object {
	switch name {
	case "to_string", "toString":
		return bindMethod(num, name, "to_string")
	case "to_float", "toFloat":
		return bindMethod(num, name, "to_float")
	case "abs":
		return methodNoArg(num, name, func() object.Object { return &object.Integer{Value: int64(math.Abs(float64(num.Value)))} })
	case "is_even", "isEven":
		return methodNoArg(num, name, func() object.Object { return object.NativeBoolToBooleanObject(num.Value%2 == 0) })
	case "is_odd", "isOdd":
		return methodNoArg(num, name, func() object.Object { return object.NativeBoolToBooleanObject(num.Value%2 != 0) })
	case "sqrt":
		return bindMethod(num, name, "sqrt")
	case "pow":
		return bindMethod(num, name, "pow")
	case "round", "floor", "ceil":
		return methodNoArg(num, name, func() object.Object { return &object.Integer{Value: num.Value} })
	case "to_iso", "toISO":
		return bindIntegerTimeMethod(num, name, "unix_to_iso")
	case "format":
		return bindIntegerTimeMethod(num, name, "format_time")
	case "format_tz", "formatTZ":
		return bindIntegerTimeMethod(num, name, "format_time_tz")
	case "add":
		return bindIntegerTimeMethod(num, name, "time_add")
	case "sub":
		return bindIntegerTimeMethod(num, name, "time_sub")
	case "diff":
		return bindIntegerTimeMethod(num, name, "time_diff")
	case "start_of_day", "startOfDay":
		return bindIntegerTimeMethod(num, name, "start_of_day")
	case "end_of_day", "endOfDay":
		return bindIntegerTimeMethod(num, name, "end_of_day")
	case "start_of_week", "startOfWeek":
		return bindIntegerTimeMethod(num, name, "start_of_week")
	case "end_of_month", "endOfMonth":
		return bindIntegerTimeMethod(num, name, "end_of_month")
	case "add_months", "addMonths":
		return bindIntegerTimeMethod(num, name, "add_months")
	case "to_timezone", "toTimezone":
		return bindIntegerTimeMethod(num, name, "to_timezone")
	default:
		return object.NewError("method '%s' not found on INTEGER", name)
	}
}

// ---------------------------------------------------------------------------
// Float methods
// ---------------------------------------------------------------------------

func getFloatMethod(num *object.Float, name string) object.Object {
	switch name {
	case "to_string", "toString":
		return bindMethod(num, name, "to_string")
	case "to_int", "toInt":
		return bindMethod(num, name, "to_int")
	case "abs":
		return methodNoArg(num, name, func() object.Object { return &object.Float{Value: math.Abs(num.Value)} })
	case "round":
		return bindMethod(num, name, "round")
	case "floor":
		return bindMethod(num, name, "floor")
	case "ceil":
		return bindMethod(num, name, "ceil")
	default:
		return object.NewError("method '%s' not found on FLOAT", name)
	}
}

// ---------------------------------------------------------------------------
// Array methods
// ---------------------------------------------------------------------------

func getArrayMethod(arr *object.Array, name string) object.Object {
	switch name {
	case "map":
		return &object.Builtin{
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("map expects 1 argument, got %d", len(args))
				}
				_, ok := args[0].(*object.Function)
				if !ok {
					_, isBuiltin := args[0].(*object.Builtin)
					if !isBuiltin {
						return object.NewError("map expects a function")
					}
				}
				newElements := make([]object.Object, len(arr.Elements))
				for i, el := range arr.Elements {
					res := executeCallback(args[0], []object.Object{el})
					if object.IsError(res) {
						return res
					}
					newElements[i] = res
				}
				return &object.Array{Elements: newElements}
			},
		}
	case "filter":
		return &object.Builtin{
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("filter expects 1 argument")
				}
				newElements := []object.Object{}
				for _, el := range arr.Elements {
					res := executeCallback(args[0], []object.Object{el})
					if object.IsError(res) {
						return res
					}
					if object.IsTruthy(res) {
						newElements = append(newElements, el)
					}
				}
				return &object.Array{Elements: newElements}
			},
		}
	case "forEach":
		return &object.Builtin{
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("forEach expects 1 argument")
				}
				for _, el := range arr.Elements {
					res := executeCallback(args[0], []object.Object{el})
					if object.IsError(res) {
						return res
					}
				}
				return object.NULL
			},
		}
	case "push":
		return &object.Builtin{
			Fn: func(args ...object.Object) object.Object {
				for _, arg := range args {
					arr.Elements = append(arr.Elements, arg)
				}
				return &object.Integer{Value: int64(len(arr.Elements))}
			},
		}
	case "find":
		return &object.Builtin{
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("find expects 1 argument")
				}
				for _, el := range arr.Elements {
					res := executeCallback(args[0], []object.Object{el})
					if object.IsTruthy(res) {
						return el
					}
				}
				return object.NULL
			},
		}
	case "length":
		return &object.Integer{Value: int64(len(arr.Elements))}
	case "every":
		return &object.Builtin{
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("every expects 1 argument")
				}
				for _, el := range arr.Elements {
					res := executeCallback(args[0], []object.Object{el})
					if object.IsError(res) {
						return res
					}
					if !object.IsTruthy(res) {
						return object.FALSE
					}
				}
				return object.TRUE
			},
		}
	case "some":
		return &object.Builtin{
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("some expects 1 argument")
				}
				for _, el := range arr.Elements {
					res := executeCallback(args[0], []object.Object{el})
					if object.IsError(res) {
						return res
					}
					if object.IsTruthy(res) {
						return object.TRUE
					}
				}
				return object.FALSE
			},
		}
	case "reduce":
		return &object.Builtin{
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return object.NewError("reduce expects 1-2 arguments (callback, optional initial)")
				}
				var acc object.Object
				startIdx := 0
				if len(args) == 2 {
					acc = args[1]
				} else {
					if len(arr.Elements) == 0 {
						return object.NewError("reduce of empty array with no initial value")
					}
					acc = arr.Elements[0]
					startIdx = 1
				}
				for i := startIdx; i < len(arr.Elements); i++ {
					res := executeCallback(args[0], []object.Object{acc, arr.Elements[i]})
					if object.IsError(res) {
						return res
					}
					acc = res
				}
				return acc
			},
		}
	case "indexOf":
		return &object.Builtin{
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("indexOf expects 1 argument")
				}
				target := args[0]
				for i, el := range arr.Elements {
					if objectsEqual(el, target) {
						return object.IntegerObj(int64(i))
					}
				}
				return object.IntegerObj(-1)
			},
		}
	case "includes":
		return &object.Builtin{
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("includes expects 1 argument")
				}
				target := args[0]
				for _, el := range arr.Elements {
					if objectsEqual(el, target) {
						return object.TRUE
					}
				}
				return object.FALSE
			},
		}
	case "join":
		return &object.Builtin{
			Fn: func(args ...object.Object) object.Object {
				sep := ","
				if len(args) > 0 {
					if s, ok := args[0].(*object.String); ok {
						sep = s.Value
					}
				}
				parts := make([]string, len(arr.Elements))
				for i, el := range arr.Elements {
					parts[i] = el.Inspect()
				}
				return &object.String{Value: strings.Join(parts, sep)}
			},
		}
	case "flat":
		return &object.Builtin{
			Fn: func(args ...object.Object) object.Object {
				result := []object.Object{}
				for _, el := range arr.Elements {
					if inner, ok := el.(*object.Array); ok {
						result = append(result, inner.Elements...)
					} else {
						result = append(result, el)
					}
				}
				return &object.Array{Elements: result}
			},
		}
	case "flatMap":
		return &object.Builtin{
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("flatMap expects 1 argument")
				}
				result := []object.Object{}
				for _, el := range arr.Elements {
					res := executeCallback(args[0], []object.Object{el})
					if object.IsError(res) {
						return res
					}
					if inner, ok := res.(*object.Array); ok {
						result = append(result, inner.Elements...)
					} else {
						result = append(result, res)
					}
				}
				return &object.Array{Elements: result}
			},
		}
	case "reverse":
		return &object.Builtin{
			Fn: func(args ...object.Object) object.Object {
				n := len(arr.Elements)
				result := make([]object.Object, n)
				for i, el := range arr.Elements {
					result[n-1-i] = el
				}
				return &object.Array{Elements: result}
			},
		}
	case "slice":
		return &object.Builtin{
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 {
					return object.NewError("slice expects at least 1 argument")
				}
				start, ok := args[0].(*object.Integer)
				if !ok {
					return object.NewError("slice start must be an integer")
				}
				s := int(start.Value)
				if s < 0 {
					s = len(arr.Elements) + s
				}
				if s < 0 {
					s = 0
				}
				if s > len(arr.Elements) {
					s = len(arr.Elements)
				}
				e := len(arr.Elements)
				if len(args) > 1 {
					end, ok := args[1].(*object.Integer)
					if !ok {
						return object.NewError("slice end must be an integer")
					}
					e = int(end.Value)
					if e < 0 {
						e = len(arr.Elements) + e
					}
					if e < 0 {
						e = 0
					}
					if e > len(arr.Elements) {
						e = len(arr.Elements)
					}
				}
				if s > e {
					return &object.Array{Elements: []object.Object{}}
				}
				return &object.Array{Elements: append([]object.Object{}, arr.Elements[s:e]...)}
			},
		}
	case "sort":
		return &object.Builtin{
			Fn: func(args ...object.Object) object.Object {
				if len(arr.Elements) == 0 {
					return &object.Array{Elements: []object.Object{}}
				}
				sorted := make([]object.Object, len(arr.Elements))
				copy(sorted, arr.Elements)
				sort.Slice(sorted, func(i, j int) bool {
					a, b := sorted[i], sorted[j]
					if a.Type() == object.INTEGER_OBJ && b.Type() == object.INTEGER_OBJ {
						return a.(*object.Integer).Value < b.(*object.Integer).Value
					}
					if a.Type() == object.FLOAT_OBJ && b.Type() == object.FLOAT_OBJ {
						return a.(*object.Float).Value < b.(*object.Float).Value
					}
					if a.Type() == object.STRING_OBJ && b.Type() == object.STRING_OBJ {
						return a.(*object.String).Value < b.(*object.String).Value
					}
					return a.Inspect() < b.Inspect()
				})
				return &object.Array{Elements: sorted}
			},
		}
	case "pop":
		return &object.Builtin{
			Fn: func(args ...object.Object) object.Object {
				if len(arr.Elements) == 0 {
					return object.NULL
				}
				last := arr.Elements[len(arr.Elements)-1]
				arr.Elements = arr.Elements[:len(arr.Elements)-1]
				return last
			},
		}
	case "shift":
		return &object.Builtin{
			Fn: func(args ...object.Object) object.Object {
				if len(arr.Elements) == 0 {
					return object.NULL
				}
				first := arr.Elements[0]
				arr.Elements = arr.Elements[1:]
				return first
			},
		}
	case "unshift":
		return &object.Builtin{
			Fn: func(args ...object.Object) object.Object {
				newElements := make([]object.Object, 0, len(args)+len(arr.Elements))
				newElements = append(newElements, args...)
				newElements = append(newElements, arr.Elements...)
				arr.Elements = newElements
				return object.IntegerObj(int64(len(arr.Elements)))
			},
		}
	case "at":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("at() takes exactly 1 argument")
			}
			idx, ok := args[0].(*object.Integer)
			if !ok {
				return object.NewError("at() argument must be integer")
			}
			i := int(idx.Value)
			if i < 0 {
				i = len(arr.Elements) + i
			}
			if i < 0 || i >= len(arr.Elements) {
				return object.NULL
			}
			return arr.Elements[i]
		}}
	}
	return object.NewError("method '%s' not found on ARRAY", name)
}

// ---------------------------------------------------------------------------
// executeCallback — invoke a function or builtin with args
// ---------------------------------------------------------------------------

// ExecuteCallback invokes a function or builtin with args (exported wrapper).
func ExecuteCallback(fnObj object.Object, args []object.Object) object.Object {
	return executeCallback(fnObj, args)
}

func executeCallback(fnObj object.Object, args []object.Object) object.Object {
	switch fn := fnObj.(type) {
	case *object.Function:
		extendedEnv := object.NewEnclosedEnvironment(fn.Env)
		for i, param := range fn.Parameters {
			if i < len(args) {
				extendedEnv.Set(param.Name, args[i])
			}
		}
		evaluated := Eval(fn.Body, extendedEnv)
		return unwrapReturnValue(evaluated)
	case *object.Builtin:
		return fn.Fn(args...)
	default:
		return object.NewError("not a function")
	}
}
