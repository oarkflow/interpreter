package interpreter

import (
	"fmt"
	"math"
	mrand "math/rand"
)

func hashContainsKey(h *Hash, key Object) (bool, Object) {
	hashKey, ok := key.(Hashable)
	if !ok {
		return false, &String{Value: fmt.Sprintf("ERROR: unusable as hash key: %s", key.Type())}
	}
	_, exists := h.Pairs[hashKey.HashKey()]
	return exists, nil
}

func sumArrayValues(arr *Array) (Object, Object) {
	var totalFloat float64
	allInt := true
	for _, el := range arr.Elements {
		switch el.Type() {
		case INTEGER_OBJ:
			totalFloat += float64(el.(*Integer).Value)
		case FLOAT_OBJ:
			allInt = false
			totalFloat += el.(*Float).Value
		default:
			return nil, &String{Value: fmt.Sprintf("ERROR: sum supports INTEGER/FLOAT values only, got %s", el.Type())}
		}
	}
	if allInt {
		return &Integer{Value: int64(totalFloat)}, nil
	}
	return &Float{Value: totalFloat}, nil
}

func stableObjectKey(obj Object) string {
	return obj.Type().String() + "::" + obj.Inspect()
}

var collectionBuiltins = map[string]*Builtin{
	"first": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != ARRAY_OBJ {
				return &String{Value: fmt.Sprintf("argument to `first` must be ARRAY, got %s", args[0].Type())}
			}
			arr := args[0].(*Array)
			if len(arr.Elements) == 0 {
				return NULL
			}
			return arr.Elements[0]
		},
	},
	"last": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != ARRAY_OBJ {
				return &String{Value: fmt.Sprintf("argument to `last` must be ARRAY, got %s", args[0].Type())}
			}
			arr := args[0].(*Array)
			if len(arr.Elements) == 0 {
				return NULL
			}
			return arr.Elements[len(arr.Elements)-1]
		},
	},
	"rest": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != ARRAY_OBJ {
				return &String{Value: fmt.Sprintf("argument to `rest` must be ARRAY, got %s", args[0].Type())}
			}
			arr := args[0].(*Array)
			if len(arr.Elements) <= 1 {
				return &Array{Elements: []Object{}}
			}
			out := make([]Object, len(arr.Elements)-1)
			copy(out, arr.Elements[1:])
			return &Array{Elements: out}
		},
	},
	"reverse": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != ARRAY_OBJ {
				return &String{Value: fmt.Sprintf("argument to `reverse` must be ARRAY, got %s", args[0].Type())}
			}
			arr := args[0].(*Array)
			out := make([]Object, len(arr.Elements))
			for i := range arr.Elements {
				out[i] = arr.Elements[len(arr.Elements)-1-i]
			}
			return &Array{Elements: out}
		},
	},
	"slice": {
		Fn: func(args ...Object) Object {
			if len(args) != 3 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=3", len(args))}
			}
			if args[0].Type() != ARRAY_OBJ {
				return &String{Value: fmt.Sprintf("argument to `slice` must be ARRAY, got %s", args[0].Type())}
			}
			start, errObj := asInt(args[1], "start")
			if errObj != nil {
				return errObj
			}
			end, errObj := asInt(args[2], "end")
			if errObj != nil {
				return errObj
			}
			arr := args[0].(*Array)
			if start < 0 || end < start || end > int64(len(arr.Elements)) {
				return &String{Value: "ERROR: invalid slice bounds"}
			}
			out := make([]Object, end-start)
			copy(out, arr.Elements[start:end])
			return &Array{Elements: out}
		},
	},
	"sum": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != ARRAY_OBJ {
				return &String{Value: fmt.Sprintf("argument to `sum` must be ARRAY, got %s", args[0].Type())}
			}
			arr := args[0].(*Array)
			sumObj, errObj := sumArrayValues(arr)
			if errObj != nil {
				return errObj
			}
			return sumObj
		},
	},
	"avg": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != ARRAY_OBJ {
				return &String{Value: fmt.Sprintf("argument to `avg` must be ARRAY, got %s", args[0].Type())}
			}
			arr := args[0].(*Array)
			if len(arr.Elements) == 0 {
				return &Float{Value: 0}
			}
			sumObj, errObj := sumArrayValues(arr)
			if errObj != nil {
				return errObj
			}
			switch sumObj.Type() {
			case INTEGER_OBJ:
				return &Float{Value: float64(sumObj.(*Integer).Value) / float64(len(arr.Elements))}
			case FLOAT_OBJ:
				return &Float{Value: sumObj.(*Float).Value / float64(len(arr.Elements))}
			default:
				return &String{Value: "ERROR: unexpected avg internal type"}
			}
		},
	},
	"compact": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != ARRAY_OBJ {
				return &String{Value: fmt.Sprintf("argument to `compact` must be ARRAY, got %s", args[0].Type())}
			}
			arr := args[0].(*Array)
			out := make([]Object, 0, len(arr.Elements))
			for _, el := range arr.Elements {
				if el.Type() != NULL_OBJ {
					out = append(out, el)
				}
			}
			return &Array{Elements: out}
		},
	},
	"flatten": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != ARRAY_OBJ {
				return &String{Value: fmt.Sprintf("argument to `flatten` must be ARRAY, got %s", args[0].Type())}
			}
			arr := args[0].(*Array)
			out := make([]Object, 0, len(arr.Elements))
			for _, el := range arr.Elements {
				if el.Type() == ARRAY_OBJ {
					out = append(out, el.(*Array).Elements...)
				} else {
					out = append(out, el)
				}
			}
			return &Array{Elements: out}
		},
	},
	"values": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != HASH_OBJ {
				return &String{Value: fmt.Sprintf("argument to `values` must be HASH, got %s", args[0].Type())}
			}
			h := args[0].(*Hash)
			out := make([]Object, 0, len(h.Pairs))
			for _, pair := range h.Pairs {
				out = append(out, pair.Value)
			}
			return &Array{Elements: out}
		},
	},
	"has_key": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			if args[0].Type() != HASH_OBJ {
				return &String{Value: fmt.Sprintf("first argument to `has_key` must be HASH, got %s", args[0].Type())}
			}
			ok, errObj := hashContainsKey(args[0].(*Hash), args[1])
			if errObj != nil {
				return errObj
			}
			return nativeBoolToBooleanObject(ok)
		},
	},
	"get": {
		Fn: func(args ...Object) Object {
			if len(args) < 2 || len(args) > 3 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2 or 3", len(args))}
			}
			if args[0].Type() != HASH_OBJ {
				return &String{Value: fmt.Sprintf("first argument to `get` must be HASH, got %s", args[0].Type())}
			}
			h := args[0].(*Hash)
			hashKey, ok := args[1].(Hashable)
			if !ok {
				return &String{Value: fmt.Sprintf("ERROR: unusable as hash key: %s", args[1].Type())}
			}
			if pair, exists := h.Pairs[hashKey.HashKey()]; exists {
				return pair.Value
			}
			if len(args) == 3 {
				return args[2]
			}
			return NULL
		},
	},
	"merge": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			if args[0].Type() != HASH_OBJ || args[1].Type() != HASH_OBJ {
				return &String{Value: fmt.Sprintf("arguments to `merge` must be HASH, got %s and %s", args[0].Type(), args[1].Type())}
			}
			left := args[0].(*Hash)
			right := args[1].(*Hash)
			out := make(map[HashKey]HashPair, len(left.Pairs)+len(right.Pairs))
			for k, pair := range left.Pairs {
				out[k] = pair
			}
			for k, pair := range right.Pairs {
				out[k] = pair
			}
			return &Hash{Pairs: out}
		},
	},
	"delete_key": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			if args[0].Type() != HASH_OBJ {
				return &String{Value: fmt.Sprintf("first argument to `delete_key` must be HASH, got %s", args[0].Type())}
			}
			hashKey, ok := args[1].(Hashable)
			if !ok {
				return &String{Value: fmt.Sprintf("ERROR: unusable as hash key: %s", args[1].Type())}
			}
			src := args[0].(*Hash)
			out := make(map[HashKey]HashPair, len(src.Pairs))
			for k, pair := range src.Pairs {
				if k != hashKey.HashKey() {
					out[k] = pair
				}
			}
			return &Hash{Pairs: out}
		},
	},
	"any": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != ARRAY_OBJ {
				return &String{Value: fmt.Sprintf("argument to `any` must be ARRAY, got %s", args[0].Type())}
			}
			arr := args[0].(*Array)
			for _, el := range arr.Elements {
				if isTruthy(el) {
					return TRUE
				}
			}
			return FALSE
		},
	},
	"all": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != ARRAY_OBJ {
				return &String{Value: fmt.Sprintf("argument to `all` must be ARRAY, got %s", args[0].Type())}
			}
			arr := args[0].(*Array)
			for _, el := range arr.Elements {
				if !isTruthy(el) {
					return FALSE
				}
			}
			return TRUE
		},
	},
	"group_by": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			if args[0].Type() != ARRAY_OBJ {
				return &String{Value: fmt.Sprintf("first argument to `group_by` must be ARRAY, got %s", args[0].Type())}
			}
			keyName, errObj := asString(args[1], "key")
			if errObj != nil {
				return errObj
			}

			arr := args[0].(*Array)
			grouped := make(map[HashKey]HashPair)
			for _, el := range arr.Elements {
				var groupKeyObj Object = NULL
				if h, ok := el.(*Hash); ok {
					key := &String{Value: keyName}
					if pair, exists := h.Pairs[key.HashKey()]; exists {
						groupKeyObj = pair.Value
					}
				}

				// Stringify non-hashable keys to keep grouping robust.
				if _, ok := groupKeyObj.(Hashable); !ok {
					groupKeyObj = &String{Value: stableObjectKey(groupKeyObj)}
				}
				hashableKey := groupKeyObj.(Hashable)
				hk := hashableKey.HashKey()
				current, exists := grouped[hk]
				if !exists {
					current = HashPair{Key: groupKeyObj, Value: &Array{Elements: []Object{}}}
				}
				bucket := current.Value.(*Array)
				bucket.Elements = append(bucket.Elements, el)
				grouped[hk] = HashPair{Key: current.Key, Value: bucket}
			}

			return &Hash{Pairs: grouped}
		},
	},
	"clamp": {
		Fn: func(args ...Object) Object {
			if len(args) != 3 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=3", len(args))}
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
				return &String{Value: "ERROR: min cannot be greater than max"}
			}
			if val < minVal {
				return &Integer{Value: minVal}
			}
			if val > maxVal {
				return &Integer{Value: maxVal}
			}
			return &Integer{Value: val}
		},
	},
	"round": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			switch args[0].Type() {
			case INTEGER_OBJ:
				return args[0]
			case FLOAT_OBJ:
				return &Integer{Value: int64(math.Round(args[0].(*Float).Value))}
			default:
				return &String{Value: fmt.Sprintf("argument to `round` must be INTEGER/FLOAT, got %s", args[0].Type())}
			}
		},
	},
	"floor": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != FLOAT_OBJ {
				return &String{Value: fmt.Sprintf("argument to `floor` must be FLOAT, got %s", args[0].Type())}
			}
			return &Integer{Value: int64(math.Floor(args[0].(*Float).Value))}
		},
	},
	"ceil": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != FLOAT_OBJ {
				return &String{Value: fmt.Sprintf("argument to `ceil` must be FLOAT, got %s", args[0].Type())}
			}
			return &Integer{Value: int64(math.Ceil(args[0].(*Float).Value))}
		},
	},
	"coalesce": {
		Fn: func(args ...Object) Object {
			for _, arg := range args {
				if arg.Type() != NULL_OBJ {
					return arg
				}
			}
			return NULL
		},
	},
	"default": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			if args[0].Type() == NULL_OBJ {
				return args[1]
			}
			return args[0]
		},
	},
	"is_even": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			n, errObj := asInt(args[0], "n")
			if errObj != nil {
				return errObj
			}
			return nativeBoolToBooleanObject(n%2 == 0)
		},
	},
	"is_odd": {
		Fn: func(args ...Object) Object {
			if len(args) != 1 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			n, errObj := asInt(args[0], "n")
			if errObj != nil {
				return errObj
			}
			return nativeBoolToBooleanObject(n%2 != 0)
		},
	},
	"random_range": {
		Fn: func(args ...Object) Object {
			if len(args) != 2 {
				return &String{Value: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
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
				return &String{Value: "ERROR: max must be greater than min"}
			}
			return &Integer{Value: minVal + mrand.Int63n(maxVal-minVal)}
		},
	},
}

func init() {
	registerBuiltins(collectionBuiltins)
}
