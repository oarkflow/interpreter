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

// classContextMatches reports whether env is currently executing inside a
// method whose defining class is ownerName (tracked via the "__class__"
// marker set on method-call environments in evalClassCall).
func classContextMatches(env *object.Environment, ownerName string) bool {
	if env == nil {
		return false
	}
	val, ok := env.Get("__class__")
	if !ok {
		return false
	}
	s, ok := val.(*object.String)
	return ok && s.Value == ownerName
}

func evalDotExpression(left object.Object, name string, env *object.Environment) object.Object {
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

	// 1b. Property Access for ClassInstance
	if ci, ok := left.(*object.ClassInstance); ok {
		if owner, isPrivate := ci.PrivateOwner[name]; isPrivate {
			if !classContextMatches(env, owner) {
				return object.NewError("'%s' is a private member of class %s", name, owner)
			}
		}
		if val, ok := ci.Get(name); ok {
			return val
		}
		return object.NULL
	}

	// 1c. Static method/field access on a class itself (ClassName.member)
	if cls, ok := left.(*object.ClassObject); ok {
		if fn, owner, ok := cls.GetStaticMethodOwner(name); ok {
			if owner.PrivateMembers[name] && !classContextMatches(env, owner.Name) {
				return object.NewError("'%s' is a private static member of class %s", name, owner.Name)
			}
			staticEnv := object.NewEnclosedEnvironment(fn.Env)
			staticEnv.Set("__class__", &object.String{Value: owner.Name})
			bound := &object.Function{
				Name:       fn.Name,
				Parameters: fn.Parameters,
				ParamTypes: fn.ParamTypes,
				Defaults:   fn.Defaults,
				ReturnType: fn.ReturnType,
				HasRest:    fn.HasRest,
				Env:        staticEnv,
				Body:       fn.Body,
			}
			return bound
		}
		if val, owner, ok := cls.GetStaticFieldOwner(name); ok {
			if owner.PrivateMembers[name] && !classContextMatches(env, owner.Name) {
				return object.NewError("'%s' is a private static member of class %s", name, owner.Name)
			}
			return val
		}
		return object.NewError("static property or method '%s' not found on class %s", name, cls.Name)
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

func collectionStringFields(args []object.Object, method string, min int) ([]string, object.Object) {
	if len(args) < min {
		return nil, object.NewError("%s expects at least %d field argument(s), got %d", method, min, len(args))
	}
	fields := make([]string, len(args))
	for i, arg := range args {
		field, ok := arg.(*object.String)
		if !ok {
			return nil, object.NewError("%s field %d must be STRING, got %s", method, i+1, arg.Type())
		}
		if strings.TrimSpace(field.Value) == "" {
			return nil, object.NewError("%s fields cannot be empty", method)
		}
		fields[i] = field.Value
	}
	return fields, nil
}

func collectionHashGetPath(hash *object.Hash, path string) (object.Object, bool) {
	var current object.Object = hash
	for _, segment := range strings.Split(path, ".") {
		nextHash, ok := current.(*object.Hash)
		if !ok {
			return nil, false
		}
		pair, ok := nextHash.Pairs[(&object.String{Value: segment}).HashKey()]
		if !ok {
			return nil, false
		}
		current = pair.Value
	}
	return current, true
}

func collectionHashSetString(hash *object.Hash, key string, value object.Object) {
	keyObject := &object.String{Value: key}
	hash.Pairs[keyObject.HashKey()] = object.HashPair{Key: keyObject, Value: value}
}

func collectionHashSetPath(hash *object.Hash, path []string, value object.Object) {
	if len(path) == 1 {
		collectionHashSetString(hash, path[0], value)
		return
	}
	keyObject := &object.String{Value: path[0]}
	hashed := keyObject.HashKey()
	child := &object.Hash{Pairs: make(map[object.HashKey]object.HashPair)}
	if pair, ok := hash.Pairs[hashed]; ok {
		if existing, ok := pair.Value.(*object.Hash); ok {
			child = existing
		}
	}
	collectionHashSetPath(child, path[1:], value)
	hash.Pairs[hashed] = object.HashPair{Key: keyObject, Value: child}
}

func collectionHashOnly(hash *object.Hash, fields []string) *object.Hash {
	out := &object.Hash{Pairs: make(map[object.HashKey]object.HashPair)}
	for _, field := range fields {
		if value, ok := collectionHashGetPath(hash, field); ok {
			collectionHashSetPath(out, strings.Split(field, "."), value)
		}
	}
	return out
}

func collectionHashClone(hash *object.Hash) *object.Hash {
	pairs := make(map[object.HashKey]object.HashPair, len(hash.Pairs))
	for key, pair := range hash.Pairs {
		pairs[key] = pair
	}
	return &object.Hash{Pairs: pairs}
}

func collectionHashExceptPath(hash *object.Hash, path []string) *object.Hash {
	out := collectionHashClone(hash)
	key := &object.String{Value: path[0]}
	hashed := key.HashKey()
	if len(path) == 1 {
		delete(out.Pairs, hashed)
		return out
	}
	pair, ok := out.Pairs[hashed]
	if !ok {
		return out
	}
	child, ok := pair.Value.(*object.Hash)
	if !ok {
		return out
	}
	pair.Value = collectionHashExceptPath(child, path[1:])
	out.Pairs[hashed] = pair
	return out
}

func collectionHashExcept(hash *object.Hash, fields []string) *object.Hash {
	out := collectionHashClone(hash)
	for _, field := range fields {
		out = collectionHashExceptPath(out, strings.Split(field, "."))
	}
	return out
}

func collectionIdentity(value object.Object) string {
	if value == nil {
		return "NULL:null"
	}
	return value.Type().String() + ":" + value.Inspect()
}

func collectionLess(left, right object.Object) bool {
	switch l := left.(type) {
	case *object.Integer:
		switch r := right.(type) {
		case *object.Integer:
			return l.Value < r.Value
		case *object.Float:
			return float64(l.Value) < r.Value
		}
	case *object.Float:
		switch r := right.(type) {
		case *object.Integer:
			return l.Value < float64(r.Value)
		case *object.Float:
			return l.Value < r.Value
		}
	case *object.String:
		if r, ok := right.(*object.String); ok {
			return l.Value < r.Value
		}
	}
	return left.Inspect() < right.Inspect()
}

func getHashMethod(hash *object.Hash, name string) object.Object {
	switch name {
	case "only", "pick":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			fields, errObj := collectionStringFields(args, name, 1)
			if errObj != nil {
				return errObj
			}
			return collectionHashOnly(hash, fields)
		}}
	case "except", "omit":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			fields, errObj := collectionStringFields(args, name, 1)
			if errObj != nil {
				return errObj
			}
			return collectionHashExcept(hash, fields)
		}}
	case "has":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			fields, errObj := collectionStringFields(args, name, 1)
			if errObj != nil {
				return errObj
			}
			for _, field := range fields {
				if _, ok := collectionHashGetPath(hash, field); !ok {
					return object.FALSE
				}
			}
			return object.TRUE
		}}
	case "get":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 || len(args) > 2 {
				return object.NewError("get expects 1-2 arguments (field, optional fallback)")
			}
			fields, errObj := collectionStringFields(args[:1], name, 1)
			if errObj != nil {
				return errObj
			}
			if value, ok := collectionHashGetPath(hash, fields[0]); ok {
				return value
			}
			if len(args) == 2 {
				return args[1]
			}
			return object.NULL
		}}
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
	case "first":
		return methodNoArg(arr, name, func() object.Object {
			if len(arr.Elements) == 0 {
				return object.NULL
			}
			return arr.Elements[0]
		})
	case "last":
		return methodNoArg(arr, name, func() object.Object {
			if len(arr.Elements) == 0 {
				return object.NULL
			}
			return arr.Elements[len(arr.Elements)-1]
		})
	case "pluck":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) == 0 {
				return &object.Array{Elements: append([]object.Object(nil), arr.Elements...)}
			}
			fields, errObj := collectionStringFields(args, name, 1)
			if errObj != nil {
				return errObj
			}
			out := make([]object.Object, 0, len(arr.Elements))
			for _, element := range arr.Elements {
				hash, ok := element.(*object.Hash)
				if !ok {
					out = append(out, &object.Hash{Pairs: make(map[object.HashKey]object.HashPair)})
					continue
				}
				out = append(out, collectionHashOnly(hash, fields))
			}
			return &object.Array{Elements: out}
		}}
	case "column", "values_of", "valuesOf":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			fields, errObj := collectionStringFields(args, name, 1)
			if errObj != nil || len(fields) != 1 {
				if errObj != nil {
					return errObj
				}
				return object.NewError("%s expects exactly 1 field", name)
			}
			out := make([]object.Object, 0, len(arr.Elements))
			for _, element := range arr.Elements {
				hash, ok := element.(*object.Hash)
				if !ok {
					out = append(out, object.NULL)
					continue
				}
				value, exists := collectionHashGetPath(hash, fields[0])
				if !exists {
					value = object.NULL
				}
				out = append(out, value)
			}
			return &object.Array{Elements: out}
		}}
	case "only", "select":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			fields, errObj := collectionStringFields(args, name, 1)
			if errObj != nil {
				return errObj
			}
			out := make([]object.Object, 0, len(arr.Elements))
			for _, element := range arr.Elements {
				if hash, ok := element.(*object.Hash); ok {
					out = append(out, collectionHashOnly(hash, fields))
				}
			}
			return &object.Array{Elements: out}
		}}
	case "except", "omit":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			fields, errObj := collectionStringFields(args, name, 1)
			if errObj != nil {
				return errObj
			}
			out := make([]object.Object, 0, len(arr.Elements))
			for _, element := range arr.Elements {
				if hash, ok := element.(*object.Hash); ok {
					out = append(out, collectionHashExcept(hash, fields))
				} else {
					out = append(out, element)
				}
			}
			return &object.Array{Elements: out}
		}}
	case "where", "first_where", "firstWhere":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return object.NewError("%s expects 2 arguments (field, value)", name)
			}
			fields, errObj := collectionStringFields(args[:1], name, 1)
			if errObj != nil {
				return errObj
			}
			firstMatch := name == "first_where" || name == "firstWhere"
			out := make([]object.Object, 0)
			for _, element := range arr.Elements {
				hash, ok := element.(*object.Hash)
				if !ok {
					continue
				}
				value, exists := collectionHashGetPath(hash, fields[0])
				if exists && objectsEqual(value, args[1]) {
					if firstMatch {
						return element
					}
					out = append(out, element)
				}
			}
			if firstMatch {
				return object.NULL
			}
			return &object.Array{Elements: out}
		}}
	case "where_in", "whereIn":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 2 {
				return object.NewError("where_in expects a field and one or more values")
			}
			fields, errObj := collectionStringFields(args[:1], name, 1)
			if errObj != nil {
				return errObj
			}
			values := args[1:]
			if valuesArray, ok := args[1].(*object.Array); ok && len(args) == 2 {
				values = valuesArray.Elements
			}
			out := make([]object.Object, 0)
			for _, element := range arr.Elements {
				hash, ok := element.(*object.Hash)
				if !ok {
					continue
				}
				value, exists := collectionHashGetPath(hash, fields[0])
				if !exists {
					continue
				}
				for _, candidate := range values {
					if objectsEqual(value, candidate) {
						out = append(out, element)
						break
					}
				}
			}
			return &object.Array{Elements: out}
		}}
	case "group_by", "groupBy", "key_by", "keyBy":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			fields, errObj := collectionStringFields(args, name, 1)
			if errObj != nil || len(fields) != 1 {
				if errObj != nil {
					return errObj
				}
				return object.NewError("%s expects exactly 1 field", name)
			}
			keyBy := name == "key_by" || name == "keyBy"
			out := &object.Hash{Pairs: make(map[object.HashKey]object.HashPair)}
			for _, element := range arr.Elements {
				hash, ok := element.(*object.Hash)
				if !ok {
					continue
				}
				value, exists := collectionHashGetPath(hash, fields[0])
				if !exists {
					continue
				}
				hashable, ok := value.(object.Hashable)
				if !ok {
					return object.NewError("%s field %q must contain hashable values, got %s", name, fields[0], value.Type())
				}
				key := hashable.HashKey()
				if keyBy {
					out.Pairs[key] = object.HashPair{Key: value, Value: element}
					continue
				}
				group := &object.Array{Elements: []object.Object{}}
				if existing, ok := out.Pairs[key]; ok {
					group = existing.Value.(*object.Array)
				}
				group.Elements = append(group.Elements, element)
				out.Pairs[key] = object.HashPair{Key: value, Value: group}
			}
			return out
		}}
	case "sort_by", "sortBy":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 || len(args) > 2 {
				return object.NewError("sort_by expects 1-2 arguments (field, optional direction)")
			}
			fields, errObj := collectionStringFields(args[:1], name, 1)
			if errObj != nil {
				return errObj
			}
			descending := false
			if len(args) == 2 {
				direction, ok := args[1].(*object.String)
				if !ok || (direction.Value != "asc" && direction.Value != "desc") {
					return object.NewError("sort_by direction must be \"asc\" or \"desc\"")
				}
				descending = direction.Value == "desc"
			}
			out := append([]object.Object(nil), arr.Elements...)
			sort.SliceStable(out, func(i, j int) bool {
				leftHash, leftOK := out[i].(*object.Hash)
				rightHash, rightOK := out[j].(*object.Hash)
				if !leftOK || !rightOK {
					return false
				}
				left, leftExists := collectionHashGetPath(leftHash, fields[0])
				right, rightExists := collectionHashGetPath(rightHash, fields[0])
				if !leftExists || !rightExists {
					return leftExists && !rightExists
				}
				if descending {
					return collectionLess(right, left)
				}
				return collectionLess(left, right)
			})
			return &object.Array{Elements: out}
		}}
	case "unique_by", "uniqueBy":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			fields, errObj := collectionStringFields(args, name, 1)
			if errObj != nil || len(fields) != 1 {
				if errObj != nil {
					return errObj
				}
				return object.NewError("unique_by expects exactly 1 field")
			}
			seen := make(map[string]bool)
			out := make([]object.Object, 0, len(arr.Elements))
			for _, element := range arr.Elements {
				hash, ok := element.(*object.Hash)
				if !ok {
					continue
				}
				value, exists := collectionHashGetPath(hash, fields[0])
				if !exists {
					value = object.NULL
				}
				identity := collectionIdentity(value)
				if !seen[identity] {
					seen[identity] = true
					out = append(out, element)
				}
			}
			return &object.Array{Elements: out}
		}}
	case "compact":
		return methodNoArg(arr, name, func() object.Object {
			out := make([]object.Object, 0, len(arr.Elements))
			for _, element := range arr.Elements {
				if element != nil && element != object.NULL {
					out = append(out, element)
				}
			}
			return &object.Array{Elements: out}
		})
	case "take", "drop":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("%s expects exactly 1 integer argument", name)
			}
			count, ok := args[0].(*object.Integer)
			if !ok {
				return object.NewError("%s count must be INTEGER, got %s", name, args[0].Type())
			}
			n := int(count.Value)
			length := len(arr.Elements)
			if n > length {
				n = length
			}
			if n < -length {
				n = -length
			}
			start, end := 0, length
			if name == "take" {
				if n >= 0 {
					end = n
				} else {
					start = length + n
				}
			} else if n >= 0 {
				start = n
			} else {
				end = length + n
			}
			return &object.Array{Elements: append([]object.Object(nil), arr.Elements[start:end]...)}
		}}
	case "chunk":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("chunk expects exactly 1 integer argument")
			}
			size, ok := args[0].(*object.Integer)
			if !ok || size.Value <= 0 {
				return object.NewError("chunk size must be a positive INTEGER")
			}
			out := make([]object.Object, 0, (len(arr.Elements)+int(size.Value)-1)/int(size.Value))
			for start := 0; start < len(arr.Elements); start += int(size.Value) {
				end := start + int(size.Value)
				if end > len(arr.Elements) {
					end = len(arr.Elements)
				}
				out = append(out, &object.Array{Elements: append([]object.Object(nil), arr.Elements[start:end]...)})
			}
			return &object.Array{Elements: out}
		}}
	case "sum", "avg":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) > 1 {
				return object.NewError("%s expects zero arguments or one field", name)
			}
			field := ""
			if len(args) == 1 {
				fields, errObj := collectionStringFields(args, name, 1)
				if errObj != nil {
					return errObj
				}
				field = fields[0]
			}
			total := float64(0)
			allIntegers := true
			count := 0
			for _, element := range arr.Elements {
				value := element
				if field != "" {
					hash, ok := element.(*object.Hash)
					if !ok {
						return object.NewError("%s(%q) requires an array of HASH values", name, field)
					}
					var exists bool
					value, exists = collectionHashGetPath(hash, field)
					if !exists {
						return object.NewError("%s field %q is missing", name, field)
					}
				}
				switch number := value.(type) {
				case *object.Integer:
					total += float64(number.Value)
				case *object.Float:
					total += number.Value
					allIntegers = false
				default:
					return object.NewError("%s expects numeric values, got %s", name, value.Type())
				}
				count++
			}
			if name == "avg" {
				if count == 0 {
					return object.NULL
				}
				return &object.Float{Value: total / float64(count)}
			}
			if allIntegers {
				return &object.Integer{Value: int64(total)}
			}
			return &object.Float{Value: total}
		}}
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
		extendedEnv := extendFunctionEnv(fn, args, nil, nil)
		evaluated := Eval(fn.Body, extendedEnv)
		return unwrapReturnValue(evaluated)
	case *object.Builtin:
		return fn.Fn(args...)
	default:
		return object.NewError("not a function")
	}
}
