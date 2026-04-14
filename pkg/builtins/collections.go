package builtins

import (
	"fmt"
	"math"
	mrand "math/rand"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

func hashContainsKey(h *object.Hash, key object.Object) (bool, object.Object) {
	hashKey, ok := key.(object.Hashable)
	if !ok {
		return false, &object.String{Value: fmt.Sprintf("ERROR: unusable as hash key: %s", key.Type())}
	}
	_, exists := h.Pairs[hashKey.HashKey()]
	return exists, nil
}

func sumArrayValues(arr *object.Array) (object.Object, object.Object) {
	var totalFloat float64
	allInt := true
	for _, el := range arr.Elements {
		switch el.Type() {
		case object.INTEGER_OBJ:
			totalFloat += float64(el.(*object.Integer).Value)
		case object.FLOAT_OBJ:
			allInt = false
			totalFloat += el.(*object.Float).Value
		default:
			return nil, &object.String{Value: fmt.Sprintf("ERROR: sum supports INTEGER/FLOAT values only, got %s", el.Type())}
		}
	}
	if allInt {
		return &object.Integer{Value: int64(totalFloat)}, nil
	}
	return &object.Float{Value: totalFloat}, nil
}

func stableObjectKey(obj object.Object) string {
	return obj.Type().String() + "::" + obj.Inspect()
}

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{
		"first": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `first` must be ARRAY, got %s", args[0].Type())}
				}
				arr := args[0].(*object.Array)
				if len(arr.Elements) == 0 {
					return object.NULL
				}
				return arr.Elements[0]
			},
		},
		"last": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `last` must be ARRAY, got %s", args[0].Type())}
				}
				arr := args[0].(*object.Array)
				if len(arr.Elements) == 0 {
					return object.NULL
				}
				return arr.Elements[len(arr.Elements)-1]
			},
		},
		"rest": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `rest` must be ARRAY, got %s", args[0].Type())}
				}
				arr := args[0].(*object.Array)
				if len(arr.Elements) <= 1 {
					return &object.Array{Elements: []object.Object{}}
				}
				out := make([]object.Object, len(arr.Elements)-1)
				copy(out, arr.Elements[1:])
				return &object.Array{Elements: out}
			},
		},
		"reverse": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `reverse` must be ARRAY, got %s", args[0].Type())}
				}
				arr := args[0].(*object.Array)
				out := make([]object.Object, len(arr.Elements))
				for i := range arr.Elements {
					out[i] = arr.Elements[len(arr.Elements)-1-i]
				}
				return &object.Array{Elements: out}
			},
		},
		"slice": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 3 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=3", len(args))}
				}
				if args[0].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `slice` must be ARRAY, got %s", args[0].Type())}
				}
				start, errObj := asInt(args[1], "start")
				if errObj != nil {
					return errObj
				}
				end, errObj := asInt(args[2], "end")
				if errObj != nil {
					return errObj
				}
				arr := args[0].(*object.Array)
				if start < 0 || end < start || end > int64(len(arr.Elements)) {
					return &object.String{Value: "ERROR: invalid slice bounds"}
				}
				out := make([]object.Object, end-start)
				copy(out, arr.Elements[start:end])
				return &object.Array{Elements: out}
			},
		},
		"sum": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `sum` must be ARRAY, got %s", args[0].Type())}
				}
				arr := args[0].(*object.Array)
				sumObj, errObj := sumArrayValues(arr)
				if errObj != nil {
					return errObj
				}
				return sumObj
			},
		},
		"avg": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `avg` must be ARRAY, got %s", args[0].Type())}
				}
				arr := args[0].(*object.Array)
				if len(arr.Elements) == 0 {
					return &object.Float{Value: 0}
				}
				sumObj, errObj := sumArrayValues(arr)
				if errObj != nil {
					return errObj
				}
				switch sumObj.Type() {
				case object.INTEGER_OBJ:
					return &object.Float{Value: float64(sumObj.(*object.Integer).Value) / float64(len(arr.Elements))}
				case object.FLOAT_OBJ:
					return &object.Float{Value: sumObj.(*object.Float).Value / float64(len(arr.Elements))}
				default:
					return &object.String{Value: "ERROR: unexpected avg internal type"}
				}
			},
		},
		"compact": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `compact` must be ARRAY, got %s", args[0].Type())}
				}
				arr := args[0].(*object.Array)
				out := make([]object.Object, 0, len(arr.Elements))
				for _, el := range arr.Elements {
					if el.Type() != object.NULL_OBJ {
						out = append(out, el)
					}
				}
				return &object.Array{Elements: out}
			},
		},
		"flatten": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `flatten` must be ARRAY, got %s", args[0].Type())}
				}
				arr := args[0].(*object.Array)
				out := make([]object.Object, 0, len(arr.Elements))
				for _, el := range arr.Elements {
					if el.Type() == object.ARRAY_OBJ {
						out = append(out, el.(*object.Array).Elements...)
					} else {
						out = append(out, el)
					}
				}
				return &object.Array{Elements: out}
			},
		},
		"values": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.HASH_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `values` must be HASH, got %s", args[0].Type())}
				}
				h := args[0].(*object.Hash)
				out := make([]object.Object, 0, len(h.Pairs))
				for _, pair := range h.Pairs {
					out = append(out, pair.Value)
				}
				return &object.Array{Elements: out}
			},
		},
		"has_key": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				if args[0].Type() != object.HASH_OBJ {
					return &object.String{Value: fmt.Sprintf("first argument to `has_key` must be HASH, got %s", args[0].Type())}
				}
				ok, errObj := hashContainsKey(args[0].(*object.Hash), args[1])
				if errObj != nil {
					return errObj
				}
				return object.NativeBoolToBooleanObject(ok)
			},
		},
		"get": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 2 || len(args) > 3 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2 or 3", len(args))}
				}
				if args[0].Type() != object.HASH_OBJ {
					return &object.String{Value: fmt.Sprintf("first argument to `get` must be HASH, got %s", args[0].Type())}
				}
				h := args[0].(*object.Hash)
				hashKey, ok := args[1].(object.Hashable)
				if !ok {
					return &object.String{Value: fmt.Sprintf("ERROR: unusable as hash key: %s", args[1].Type())}
				}
				if pair, exists := h.Pairs[hashKey.HashKey()]; exists {
					return pair.Value
				}
				if len(args) == 3 {
					return args[2]
				}
				return object.NULL
			},
		},
		"merge": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				if args[0].Type() != object.HASH_OBJ || args[1].Type() != object.HASH_OBJ {
					return &object.String{Value: fmt.Sprintf("arguments to `merge` must be HASH, got %s and %s", args[0].Type(), args[1].Type())}
				}
				left := args[0].(*object.Hash)
				right := args[1].(*object.Hash)
				out := make(map[object.HashKey]object.HashPair, len(left.Pairs)+len(right.Pairs))
				for k, pair := range left.Pairs {
					out[k] = pair
				}
				for k, pair := range right.Pairs {
					out[k] = pair
				}
				return &object.Hash{Pairs: out}
			},
		},
		"delete_key": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				if args[0].Type() != object.HASH_OBJ {
					return &object.String{Value: fmt.Sprintf("first argument to `delete_key` must be HASH, got %s", args[0].Type())}
				}
				hashKey, ok := args[1].(object.Hashable)
				if !ok {
					return &object.String{Value: fmt.Sprintf("ERROR: unusable as hash key: %s", args[1].Type())}
				}
				src := args[0].(*object.Hash)
				out := make(map[object.HashKey]object.HashPair, len(src.Pairs))
				for k, pair := range src.Pairs {
					if k != hashKey.HashKey() {
						out[k] = pair
					}
				}
				return &object.Hash{Pairs: out}
			},
		},
		"any": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `any` must be ARRAY, got %s", args[0].Type())}
				}
				arr := args[0].(*object.Array)
				for _, el := range arr.Elements {
					if object.IsTruthy(el) {
						return object.TRUE
					}
				}
				return object.FALSE
			},
		},
		"all": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `all` must be ARRAY, got %s", args[0].Type())}
				}
				arr := args[0].(*object.Array)
				for _, el := range arr.Elements {
					if !object.IsTruthy(el) {
						return object.FALSE
					}
				}
				return object.TRUE
			},
		},
		"group_by": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				if args[0].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("first argument to `group_by` must be ARRAY, got %s", args[0].Type())}
				}
				keyName, errObj := asString(args[1], "key")
				if errObj != nil {
					return errObj
				}

				arr := args[0].(*object.Array)
				grouped := make(map[object.HashKey]object.HashPair)
				for _, el := range arr.Elements {
					var groupKeyObj object.Object = object.NULL
					if h, ok := el.(*object.Hash); ok {
						key := &object.String{Value: keyName}
						if pair, exists := h.Pairs[key.HashKey()]; exists {
							groupKeyObj = pair.Value
						}
					}

					if _, ok := groupKeyObj.(object.Hashable); !ok {
						groupKeyObj = &object.String{Value: stableObjectKey(groupKeyObj)}
					}
					hashableKey := groupKeyObj.(object.Hashable)
					hk := hashableKey.HashKey()
					current, exists := grouped[hk]
					if !exists {
						current = object.HashPair{Key: groupKeyObj, Value: &object.Array{Elements: []object.Object{}}}
					}
					bucket := current.Value.(*object.Array)
					bucket.Elements = append(bucket.Elements, el)
					grouped[hk] = object.HashPair{Key: current.Key, Value: bucket}
				}

				return &object.Hash{Pairs: grouped}
			},
		},
		"clamp": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 3 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=3", len(args))}
				}
				val, errObj := asInt(args[0], "value")
				if errObj != nil {
					return errObj
				}
				minVal, errObj := asInt(args[1], "min")
				if errObj != nil {
					return errObj
				}
				maxVal, errObj := asInt(args[2], "max")
				if errObj != nil {
					return errObj
				}
				if minVal > maxVal {
					return &object.String{Value: "ERROR: min cannot be greater than max"}
				}
				if val < minVal {
					return &object.Integer{Value: minVal}
				}
				if val > maxVal {
					return &object.Integer{Value: maxVal}
				}
				return &object.Integer{Value: val}
			},
		},
		"round": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				switch args[0].Type() {
				case object.INTEGER_OBJ:
					return args[0]
				case object.FLOAT_OBJ:
					return &object.Integer{Value: int64(math.Round(args[0].(*object.Float).Value))}
				default:
					return &object.String{Value: fmt.Sprintf("argument to `round` must be INTEGER/FLOAT, got %s", args[0].Type())}
				}
			},
		},
		"floor": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.FLOAT_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `floor` must be FLOAT, got %s", args[0].Type())}
				}
				return &object.Integer{Value: int64(math.Floor(args[0].(*object.Float).Value))}
			},
		},
		"ceil": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				if args[0].Type() != object.FLOAT_OBJ {
					return &object.String{Value: fmt.Sprintf("argument to `ceil` must be FLOAT, got %s", args[0].Type())}
				}
				return &object.Integer{Value: int64(math.Ceil(args[0].(*object.Float).Value))}
			},
		},
		"coalesce": {
			Fn: func(args ...object.Object) object.Object {
				for _, arg := range args {
					if arg.Type() != object.NULL_OBJ {
						return arg
					}
				}
				return object.NULL
			},
		},
		"default": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				if args[0].Type() == object.NULL_OBJ {
					return args[1]
				}
				return args[0]
			},
		},
		"is_even": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				n, errObj := asInt(args[0], "n")
				if errObj != nil {
					return errObj
				}
				return object.NativeBoolToBooleanObject(n%2 == 0)
			},
		},
		"is_odd": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
				}
				n, errObj := asInt(args[0], "n")
				if errObj != nil {
					return errObj
				}
				return object.NativeBoolToBooleanObject(n%2 != 0)
			},
		},
		"random_range": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				minVal, errObj := asInt(args[0], "min")
				if errObj != nil {
					return errObj
				}
				maxVal, errObj := asInt(args[1], "max")
				if errObj != nil {
					return errObj
				}
				if maxVal <= minVal {
					return &object.String{Value: "ERROR: max must be greater than min"}
				}
				return &object.Integer{Value: minVal + mrand.Int63n(maxVal-minVal)}
			},
		},

		// zip(arr1, arr2) — pairs elements from two arrays.
		// Returns array of [a, b] pairs with length = min(len(arr1), len(arr2)).
		"zip": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				if args[0].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("first argument to `zip` must be ARRAY, got %s", args[0].Type())}
				}
				if args[1].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("second argument to `zip` must be ARRAY, got %s", args[1].Type())}
				}
				a := args[0].(*object.Array).Elements
				b := args[1].(*object.Array).Elements
				minLen := len(a)
				if len(b) < minLen {
					minLen = len(b)
				}
				out := make([]object.Object, minLen)
				for i := 0; i < minLen; i++ {
					out[i] = &object.Array{Elements: []object.Object{a[i], b[i]}}
				}
				return &object.Array{Elements: out}
			},
		},

		// chunk(arr, size) — splits an array into sub-arrays of the given size.
		"chunk": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
				}
				if args[0].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("first argument to `chunk` must be ARRAY, got %s", args[0].Type())}
				}
				size, errObj := asInt(args[1], "size")
				if errObj != nil {
					return errObj
				}
				if size <= 0 {
					return &object.String{Value: "ERROR: chunk size must be > 0"}
				}
				arr := args[0].(*object.Array).Elements
				var out []object.Object
				for i := 0; i < len(arr); i += int(size) {
					end := i + int(size)
					if end > len(arr) {
						end = len(arr)
					}
					chunk := make([]object.Object, end-i)
					copy(chunk, arr[i:end])
					out = append(out, &object.Array{Elements: chunk})
				}
				if out == nil {
					out = []object.Object{}
				}
				return &object.Array{Elements: out}
			},
		},

		// partition(arr, key, value) — splits array of hashes into two groups:
		// those where hash[key] == value, and those where it doesn't.
		// Returns [[matching], [non-matching]].
		"partition": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 3 {
					return &object.String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=3", len(args))}
				}
				if args[0].Type() != object.ARRAY_OBJ {
					return &object.String{Value: fmt.Sprintf("first argument to `partition` must be ARRAY, got %s", args[0].Type())}
				}
				keyName, errObj := asString(args[1], "key")
				if errObj != nil {
					return errObj
				}
				matchValue := args[2]

				arr := args[0].(*object.Array).Elements
				var matching, rest []object.Object
				keyObj := &object.String{Value: keyName}
				hk := keyObj.HashKey()

				for _, el := range arr {
					matched := false
					if h, ok := el.(*object.Hash); ok {
						if pair, exists := h.Pairs[hk]; exists {
							if pair.Value.Inspect() == matchValue.Inspect() {
								matched = true
							}
						}
					}
					if matched {
						matching = append(matching, el)
					} else {
						rest = append(rest, el)
					}
				}
				if matching == nil {
					matching = []object.Object{}
				}
				if rest == nil {
					rest = []object.Object{}
				}
				return &object.Array{Elements: []object.Object{
					&object.Array{Elements: matching},
					&object.Array{Elements: rest},
				}}
			},
		},
	})
}
