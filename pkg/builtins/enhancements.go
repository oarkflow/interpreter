package builtins

import (
	"html"
	"math"
	mrand "math/rand"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

func numberAsFloat(arg object.Object, name string) (float64, bool, object.Object) {
	switch v := arg.(type) {
	case *object.Integer:
		return float64(v.Value), true, nil
	case *object.Float:
		return v.Value, false, nil
	default:
		return 0, false, object.NewError("argument `%s` must be INTEGER or FLOAT, got %s", name, arg.Type())
	}
}

func numberArray(arg object.Object, name string) ([]float64, bool, object.Object) {
	arr, ok := arg.(*object.Array)
	if !ok {
		return nil, false, object.NewError("argument `%s` must be ARRAY, got %s", name, arg.Type())
	}
	values := make([]float64, 0, len(arr.Elements))
	allInt := true
	for i, el := range arr.Elements {
		v, isInt, errObj := numberAsFloat(el, name)
		if errObj != nil {
			return nil, false, object.NewError("%s[%d] must be INTEGER or FLOAT, got %s", name, i, el.Type())
		}
		allInt = allInt && isInt
		values = append(values, v)
	}
	return values, allInt, nil
}

func maybeIntFloat(value float64, preferInt bool) object.Object {
	if preferInt && !math.IsNaN(value) && !math.IsInf(value, 0) && math.Trunc(value) == value {
		return &object.Integer{Value: int64(value)}
	}
	return &object.Float{Value: value}
}

func objectIdentityKey(obj object.Object) string {
	if obj == nil {
		return "NULL::null"
	}
	return obj.Type().String() + "::" + obj.Inspect()
}

func objectsSameValue(a, b object.Object) bool {
	if a == nil || b == nil {
		return a == b
	}
	if af, _, aErr := numberAsFloat(a, "a"); aErr == nil {
		if bf, _, bErr := numberAsFloat(b, "b"); bErr == nil {
			return af == bf
		}
	}
	if a.Type() != b.Type() {
		return false
	}
	switch av := a.(type) {
	case *object.String:
		return av.Value == b.(*object.String).Value
	case *object.Boolean:
		return av.Value == b.(*object.Boolean).Value
	case *object.Null:
		return true
	case *object.Array:
		bv := b.(*object.Array)
		if len(av.Elements) != len(bv.Elements) {
			return false
		}
		for i := range av.Elements {
			if !objectsSameValue(av.Elements[i], bv.Elements[i]) {
				return false
			}
		}
		return true
	case *object.Hash:
		bv := b.(*object.Hash)
		if len(av.Pairs) != len(bv.Pairs) {
			return false
		}
		for hk, pair := range av.Pairs {
			other, ok := bv.Pairs[hk]
			if !ok || !objectsSameValue(pair.Value, other.Value) {
				return false
			}
		}
		return true
	}
	return objectIdentityKey(a) == objectIdentityKey(b)
}

func hashStringPair(key string, value object.Object) (object.HashKey, object.HashPair) {
	k := &object.String{Value: key}
	return k.HashKey(), object.HashPair{Key: k, Value: value}
}

func hashGetString(h *object.Hash, key string) (object.Object, bool) {
	k := (&object.String{Value: key}).HashKey()
	pair, ok := h.Pairs[k]
	if !ok {
		return nil, false
	}
	return pair.Value, true
}

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{
		"cbrt": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("cbrt expects 1 argument")
				}
				v, _, errObj := numberAsFloat(args[0], "n")
				if errObj != nil {
					return errObj
				}
				return &object.Float{Value: math.Cbrt(v)}
			},
		},
		"mod": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("mod expects 2 arguments")
				}
				a, aInt, errObj := numberAsFloat(args[0], "a")
				if errObj != nil {
					return errObj
				}
				b, bInt, errObj := numberAsFloat(args[1], "b")
				if errObj != nil {
					return errObj
				}
				if b == 0 {
					return object.NewError("mod divisor must not be zero")
				}
				if aInt && bInt {
					return &object.Integer{Value: args[0].(*object.Integer).Value % args[1].(*object.Integer).Value}
				}
				return &object.Float{Value: math.Mod(a, b)}
			},
		},
		"sign": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("sign expects 1 argument")
				}
				v, _, errObj := numberAsFloat(args[0], "n")
				if errObj != nil {
					return errObj
				}
				switch {
				case v > 0:
					return &object.Integer{Value: 1}
				case v < 0:
					return &object.Integer{Value: -1}
				default:
					return &object.Integer{Value: 0}
				}
			},
		},
		"trunc": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("trunc expects 1 argument")
				}
				v, isInt, errObj := numberAsFloat(args[0], "n")
				if errObj != nil {
					return errObj
				}
				return maybeIntFloat(math.Trunc(v), isInt)
			},
		},
		"round_to": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("round_to expects 2 arguments")
				}
				v, _, errObj := numberAsFloat(args[0], "n")
				if errObj != nil {
					return errObj
				}
				places, errObj := asInt(args[1], "decimals")
				if errObj != nil {
					return errObj
				}
				factor := math.Pow(10, float64(places))
				return &object.Float{Value: math.Round(v*factor) / factor}
			},
		},
		"lerp": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 3 {
					return object.NewError("lerp expects 3 arguments")
				}
				a, _, errObj := numberAsFloat(args[0], "a")
				if errObj != nil {
					return errObj
				}
				b, _, errObj := numberAsFloat(args[1], "b")
				if errObj != nil {
					return errObj
				}
				t, _, errObj := numberAsFloat(args[2], "t")
				if errObj != nil {
					return errObj
				}
				return &object.Float{Value: a + (b-a)*t}
			},
		},
		"normalize": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 3 {
					return object.NewError("normalize expects 3 arguments")
				}
				v, _, errObj := numberAsFloat(args[0], "value")
				if errObj != nil {
					return errObj
				}
				minVal, _, errObj := numberAsFloat(args[1], "min")
				if errObj != nil {
					return errObj
				}
				maxVal, _, errObj := numberAsFloat(args[2], "max")
				if errObj != nil {
					return errObj
				}
				if maxVal == minVal {
					return object.NewError("normalize range must not be zero")
				}
				return &object.Float{Value: (v - minVal) / (maxVal - minVal)}
			},
		},
		"map_range": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 5 {
					return object.NewError("map_range expects 5 arguments")
				}
				v, _, errObj := numberAsFloat(args[0], "value")
				if errObj != nil {
					return errObj
				}
				inMin, _, errObj := numberAsFloat(args[1], "in_min")
				if errObj != nil {
					return errObj
				}
				inMax, _, errObj := numberAsFloat(args[2], "in_max")
				if errObj != nil {
					return errObj
				}
				outMin, _, errObj := numberAsFloat(args[3], "out_min")
				if errObj != nil {
					return errObj
				}
				outMax, _, errObj := numberAsFloat(args[4], "out_max")
				if errObj != nil {
					return errObj
				}
				if inMax == inMin {
					return object.NewError("map_range input range must not be zero")
				}
				return &object.Float{Value: outMin + (v-inMin)*(outMax-outMin)/(inMax-inMin)}
			},
		},
		"percent": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("percent expects 2 arguments")
				}
				v, _, errObj := numberAsFloat(args[0], "value")
				if errObj != nil {
					return errObj
				}
				total, _, errObj := numberAsFloat(args[1], "total")
				if errObj != nil {
					return errObj
				}
				if total == 0 {
					return object.NewError("percent total must not be zero")
				}
				return &object.Float{Value: (v / total) * 100}
			},
		},
		"factorial": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("factorial expects 1 argument")
				}
				n, errObj := asInt(args[0], "n")
				if errObj != nil {
					return errObj
				}
				if n < 0 {
					return object.NewError("factorial n must be >= 0")
				}
				var out int64 = 1
				for i := int64(2); i <= n; i++ {
					out *= i
				}
				return &object.Integer{Value: out}
			},
		},
		"gcd": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("gcd expects 2 arguments")
				}
				a, errObj := asInt(args[0], "a")
				if errObj != nil {
					return errObj
				}
				b, errObj := asInt(args[1], "b")
				if errObj != nil {
					return errObj
				}
				if a < 0 {
					a = -a
				}
				if b < 0 {
					b = -b
				}
				for b != 0 {
					a, b = b, a%b
				}
				return &object.Integer{Value: a}
			},
		},
		"lcm": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("lcm expects 2 arguments")
				}
				a, errObj := asInt(args[0], "a")
				if errObj != nil {
					return errObj
				}
				b, errObj := asInt(args[1], "b")
				if errObj != nil {
					return errObj
				}
				if a == 0 || b == 0 {
					return &object.Integer{Value: 0}
				}
				aa, bb := a, b
				if aa < 0 {
					aa = -aa
				}
				if bb < 0 {
					bb = -bb
				}
				for bb != 0 {
					aa, bb = bb, aa%bb
				}
				out := (a / aa) * b
				if out < 0 {
					out = -out
				}
				return &object.Integer{Value: out}
			},
		},
		"is_finite": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("is_finite expects 1 argument")
				}
				v, _, errObj := numberAsFloat(args[0], "n")
				if errObj != nil {
					return object.FALSE
				}
				return object.NativeBoolToBooleanObject(!math.IsNaN(v) && !math.IsInf(v, 0))
			},
		},
		"is_integer": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("is_integer expects 1 argument")
				}
				v, _, errObj := numberAsFloat(args[0], "n")
				if errObj != nil {
					return object.FALSE
				}
				return object.NativeBoolToBooleanObject(math.Trunc(v) == v)
			},
		},
		"is_prime": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("is_prime expects 1 argument")
				}
				n, errObj := asInt(args[0], "n")
				if errObj != nil {
					return errObj
				}
				if n < 2 {
					return object.FALSE
				}
				if n == 2 {
					return object.TRUE
				}
				if n%2 == 0 {
					return object.FALSE
				}
				for i := int64(3); i*i <= n; i += 2 {
					if n%i == 0 {
						return object.FALSE
					}
				}
				return object.TRUE
			},
		},
		"random_float": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 0 {
					return object.NewError("random_float expects 0 arguments")
				}
				return &object.Float{Value: mrand.Float64()}
			},
		},
		"random_choice": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("random_choice expects 1 argument")
				}
				arr, ok := args[0].(*object.Array)
				if !ok {
					return object.NewError("argument `array` must be ARRAY, got %s", args[0].Type())
				}
				if len(arr.Elements) == 0 {
					return object.NULL
				}
				return arr.Elements[mrand.Intn(len(arr.Elements))]
			},
		},
		"shuffle": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("shuffle expects 1 argument")
				}
				arr, ok := args[0].(*object.Array)
				if !ok {
					return object.NewError("argument `array` must be ARRAY, got %s", args[0].Type())
				}
				out := append([]object.Object(nil), arr.Elements...)
				mrand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
				return &object.Array{Elements: out}
			},
		},
		"sample": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("sample expects 2 arguments")
				}
				arr, ok := args[0].(*object.Array)
				if !ok {
					return object.NewError("argument `array` must be ARRAY, got %s", args[0].Type())
				}
				count, errObj := asInt(args[1], "count")
				if errObj != nil {
					return errObj
				}
				if count < 0 {
					return object.NewError("sample count must be >= 0")
				}
				out := append([]object.Object(nil), arr.Elements...)
				mrand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
				if int(count) < len(out) {
					out = out[:int(count)]
				}
				return &object.Array{Elements: out}
			},
		},
		"mean": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("mean expects 1 argument")
				}
				values, _, errObj := numberArray(args[0], "array")
				if errObj != nil {
					return errObj
				}
				if len(values) == 0 {
					return &object.Float{Value: 0}
				}
				var sum float64
				for _, v := range values {
					sum += v
				}
				return &object.Float{Value: sum / float64(len(values))}
			},
		},
		"median": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("median expects 1 argument")
				}
				values, allInt, errObj := numberArray(args[0], "array")
				if errObj != nil {
					return errObj
				}
				if len(values) == 0 {
					return object.NULL
				}
				sort.Float64s(values)
				mid := len(values) / 2
				if len(values)%2 == 1 {
					return maybeIntFloat(values[mid], allInt)
				}
				return &object.Float{Value: (values[mid-1] + values[mid]) / 2}
			},
		},
		"mode": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("mode expects 1 argument")
				}
				arr, ok := args[0].(*object.Array)
				if !ok {
					return object.NewError("argument `array` must be ARRAY, got %s", args[0].Type())
				}
				if len(arr.Elements) == 0 {
					return object.NULL
				}
				counts := map[string]int{}
				values := map[string]object.Object{}
				bestKey := ""
				bestCount := 0
				for _, el := range arr.Elements {
					key := objectIdentityKey(el)
					counts[key]++
					values[key] = el
					if counts[key] > bestCount {
						bestKey = key
						bestCount = counts[key]
					}
				}
				return values[bestKey]
			},
		},
		"variance": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("variance expects 1 argument")
				}
				values, _, errObj := numberArray(args[0], "array")
				if errObj != nil {
					return errObj
				}
				if len(values) == 0 {
					return &object.Float{Value: 0}
				}
				var sum float64
				for _, v := range values {
					sum += v
				}
				mean := sum / float64(len(values))
				var total float64
				for _, v := range values {
					diff := v - mean
					total += diff * diff
				}
				return &object.Float{Value: total / float64(len(values))}
			},
		},
		"stddev": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("stddev expects 1 argument")
				}
				values, _, errObj := numberArray(args[0], "array")
				if errObj != nil {
					return errObj
				}
				if len(values) == 0 {
					return &object.Float{Value: 0}
				}
				var sum float64
				for _, v := range values {
					sum += v
				}
				mean := sum / float64(len(values))
				var total float64
				for _, v := range values {
					diff := v - mean
					total += diff * diff
				}
				return &object.Float{Value: math.Sqrt(total / float64(len(values)))}
			},
		},
		"percentile": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("percentile expects 2 arguments")
				}
				values, _, errObj := numberArray(args[0], "array")
				if errObj != nil {
					return errObj
				}
				p, _, errObj := numberAsFloat(args[1], "p")
				if errObj != nil {
					return errObj
				}
				if len(values) == 0 {
					return object.NULL
				}
				if p < 0 || p > 100 {
					return object.NewError("percentile p must be between 0 and 100")
				}
				sort.Float64s(values)
				pos := (p / 100) * float64(len(values)-1)
				lo := int(math.Floor(pos))
				hi := int(math.Ceil(pos))
				if lo == hi {
					return &object.Float{Value: values[lo]}
				}
				weight := pos - float64(lo)
				return &object.Float{Value: values[lo]*(1-weight) + values[hi]*weight}
			},
		},
		"unique": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("unique expects 1 argument")
				}
				arr, ok := args[0].(*object.Array)
				if !ok {
					return object.NewError("argument `array` must be ARRAY, got %s", args[0].Type())
				}
				seen := map[string]bool{}
				out := []object.Object{}
				for _, el := range arr.Elements {
					key := objectIdentityKey(el)
					if !seen[key] {
						seen[key] = true
						out = append(out, el)
					}
				}
				return &object.Array{Elements: out}
			},
		},
		"sort_by": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("sort_by expects 2 arguments")
				}
				arr, ok := args[0].(*object.Array)
				if !ok {
					return object.NewError("argument `array` must be ARRAY, got %s", args[0].Type())
				}
				key, errObj := asString(args[1], "key")
				if errObj != nil {
					return errObj
				}
				out := append([]object.Object(nil), arr.Elements...)
				sort.SliceStable(out, func(i, j int) bool {
					hi, iok := out[i].(*object.Hash)
					hj, jok := out[j].(*object.Hash)
					if !iok || !jok {
						return out[i].Inspect() < out[j].Inspect()
					}
					vi, _ := hashGetString(hi, key)
					vj, _ := hashGetString(hj, key)
					if vi == nil {
						vi = object.NULL
					}
					if vj == nil {
						vj = object.NULL
					}
					return vi.Inspect() < vj.Inspect()
				})
				return &object.Array{Elements: out}
			},
		},
		"take": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("take expects 2 arguments")
				}
				arr, ok := args[0].(*object.Array)
				if !ok {
					return object.NewError("argument `array` must be ARRAY, got %s", args[0].Type())
				}
				n, errObj := asInt(args[1], "n")
				if errObj != nil {
					return errObj
				}
				if n < 0 {
					n = 0
				}
				if int(n) > len(arr.Elements) {
					n = int64(len(arr.Elements))
				}
				return &object.Array{Elements: append([]object.Object(nil), arr.Elements[:int(n)]...)}
			},
		},
		"drop": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("drop expects 2 arguments")
				}
				arr, ok := args[0].(*object.Array)
				if !ok {
					return object.NewError("argument `array` must be ARRAY, got %s", args[0].Type())
				}
				n, errObj := asInt(args[1], "n")
				if errObj != nil {
					return errObj
				}
				if n < 0 {
					n = 0
				}
				if int(n) > len(arr.Elements) {
					n = int64(len(arr.Elements))
				}
				return &object.Array{Elements: append([]object.Object(nil), arr.Elements[int(n):]...)}
			},
		},
		"pluck": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("pluck expects 2 arguments")
				}
				arr, ok := args[0].(*object.Array)
				if !ok {
					return object.NewError("argument `array` must be ARRAY, got %s", args[0].Type())
				}
				key, errObj := asString(args[1], "key")
				if errObj != nil {
					return errObj
				}
				out := make([]object.Object, 0, len(arr.Elements))
				for _, el := range arr.Elements {
					if h, ok := el.(*object.Hash); ok {
						if v, exists := hashGetString(h, key); exists {
							out = append(out, v)
							continue
						}
					}
					out = append(out, object.NULL)
				}
				return &object.Array{Elements: out}
			},
		},
		"index_by": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("index_by expects 2 arguments")
				}
				arr, ok := args[0].(*object.Array)
				if !ok {
					return object.NewError("argument `array` must be ARRAY, got %s", args[0].Type())
				}
				key, errObj := asString(args[1], "key")
				if errObj != nil {
					return errObj
				}
				out := map[object.HashKey]object.HashPair{}
				for _, el := range arr.Elements {
					h, ok := el.(*object.Hash)
					if !ok {
						continue
					}
					v, exists := hashGetString(h, key)
					if !exists {
						continue
					}
					hashable, ok := v.(object.Hashable)
					if !ok {
						continue
					}
					out[hashable.HashKey()] = object.HashPair{Key: v, Value: el}
				}
				return &object.Hash{Pairs: out}
			},
		},
		"pick": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("pick expects 2 arguments")
				}
				h, ok := args[0].(*object.Hash)
				if !ok {
					return object.NewError("argument `hash` must be HASH, got %s", args[0].Type())
				}
				keys, ok := args[1].(*object.Array)
				if !ok {
					return object.NewError("argument `keys` must be ARRAY, got %s", args[1].Type())
				}
				out := map[object.HashKey]object.HashPair{}
				for _, keyObj := range keys.Elements {
					key, errObj := asString(keyObj, "key")
					if errObj != nil {
						return errObj
					}
					if val, exists := hashGetString(h, key); exists {
						hk, pair := hashStringPair(key, val)
						out[hk] = pair
					}
				}
				return &object.Hash{Pairs: out}
			},
		},
		"omit": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("omit expects 2 arguments")
				}
				h, ok := args[0].(*object.Hash)
				if !ok {
					return object.NewError("argument `hash` must be HASH, got %s", args[0].Type())
				}
				keys, ok := args[1].(*object.Array)
				if !ok {
					return object.NewError("argument `keys` must be ARRAY, got %s", args[1].Type())
				}
				omitSet := map[object.HashKey]bool{}
				for _, keyObj := range keys.Elements {
					hashable, ok := keyObj.(object.Hashable)
					if !ok {
						return object.NewError("keys must be hashable, got %s", keyObj.Type())
					}
					omitSet[hashable.HashKey()] = true
				}
				out := map[object.HashKey]object.HashPair{}
				for hk, pair := range h.Pairs {
					if !omitSet[hk] {
						out[hk] = pair
					}
				}
				return &object.Hash{Pairs: out}
			},
		},
		"entries": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("entries expects 1 argument")
				}
				h, ok := args[0].(*object.Hash)
				if !ok {
					return object.NewError("argument `hash` must be HASH, got %s", args[0].Type())
				}
				out := make([]object.Object, 0, len(h.Pairs))
				for _, pair := range h.Pairs {
					out = append(out, &object.Array{Elements: []object.Object{pair.Key, pair.Value}})
				}
				return &object.Array{Elements: out}
			},
		},
		"from_entries": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("from_entries expects 1 argument")
				}
				arr, ok := args[0].(*object.Array)
				if !ok {
					return object.NewError("argument `entries` must be ARRAY, got %s", args[0].Type())
				}
				out := map[object.HashKey]object.HashPair{}
				for _, el := range arr.Elements {
					pairArr, ok := el.(*object.Array)
					if !ok || len(pairArr.Elements) != 2 {
						return object.NewError("from_entries entries must be [key, value] arrays")
					}
					hashable, ok := pairArr.Elements[0].(object.Hashable)
					if !ok {
						return object.NewError("entry key must be hashable, got %s", pairArr.Elements[0].Type())
					}
					out[hashable.HashKey()] = object.HashPair{Key: pairArr.Elements[0], Value: pairArr.Elements[1]}
				}
				return &object.Hash{Pairs: out}
			},
		},
		"deep_equal": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("deep_equal expects 2 arguments")
				}
				return object.NativeBoolToBooleanObject(objectsSameValue(args[0], args[1]))
			},
		},
		"last_index_of": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("last_index_of expects 2 arguments")
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				sub, errObj := asString(args[1], "substr")
				if errObj != nil {
					return errObj
				}
				return &object.Integer{Value: int64(strings.LastIndex(s, sub))}
			},
		},
		"replace_n": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 4 {
					return object.NewError("replace_n expects 4 arguments")
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				old, errObj := asString(args[1], "old")
				if errObj != nil {
					return errObj
				}
				newVal, errObj := asString(args[2], "new")
				if errObj != nil {
					return errObj
				}
				n, errObj := asInt(args[3], "n")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: strings.Replace(s, old, newVal, int(n))}
			},
		},
		"trim_chars": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("trim_chars expects 2 arguments")
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				cutset, errObj := asString(args[1], "chars")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: strings.Trim(s, cutset)}
			},
		},
		"words": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("words expects 1 argument")
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				words := splitWords(s)
				out := make([]object.Object, len(words))
				for i, word := range words {
					out[i] = &object.String{Value: word}
				}
				return &object.Array{Elements: out}
			},
		},
		"chars": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("chars expects 1 argument")
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				runes := []rune(s)
				out := make([]object.Object, len(runes))
				for i, r := range runes {
					out[i] = &object.String{Value: string(r)}
				}
				return &object.Array{Elements: out}
			},
		},
		"reverse_string": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("reverse_string expects 1 argument")
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				runes := []rune(s)
				for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
					runes[i], runes[j] = runes[j], runes[i]
				}
				return &object.String{Value: string(runes)}
			},
		},
		"is_blank": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("is_blank expects 1 argument")
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				return object.NativeBoolToBooleanObject(strings.TrimSpace(s) == "")
			},
		},
		"is_numeric": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("is_numeric expects 1 argument")
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				if strings.TrimSpace(s) == "" {
					return object.FALSE
				}
				_, err := strconvParseFloat(strings.TrimSpace(s))
				return object.NativeBoolToBooleanObject(err == nil)
			},
		},
		"is_alpha": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("is_alpha expects 1 argument")
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				if s == "" {
					return object.FALSE
				}
				for _, r := range s {
					if !unicode.IsLetter(r) {
						return object.FALSE
					}
				}
				return object.TRUE
			},
		},
		"is_alnum": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("is_alnum expects 1 argument")
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				if s == "" {
					return object.FALSE
				}
				for _, r := range s {
					if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
						return object.FALSE
					}
				}
				return object.TRUE
			},
		},
		"escape_html": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("escape_html expects 1 argument")
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: html.EscapeString(s)}
			},
		},
		"unescape_html": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("unescape_html expects 1 argument")
				}
				s, errObj := asString(args[0], "s")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: html.UnescapeString(s)}
			},
		},
		"json_parse": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("json_parse expects 1 argument")
				}
				s, errObj := asString(args[0], "text")
				if errObj != nil {
					return errObj
				}
				out, err := ParseJSONToObject(s)
				if err != nil {
					return object.NewError("json decode failed: %v", err)
				}
				return out
			},
		},
		"json_stringify": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return object.NewError("json_stringify expects 1 or 2 arguments")
				}
				opts := map[string]object.Object(nil)
				if len(args) == 2 {
					var errObj object.Object
					opts, errObj = parseOptionalHash(args[1], "opts")
					if errObj != nil {
						return errObj
					}
				}
				data, errObj := jsonMarshalValue(args[0], opts)
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: string(data)}
			},
		},
		"path_base": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("path_base expects 1 argument")
				}
				path, errObj := asString(args[0], "path")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: filepath.Base(path)}
			},
		},
		"path_dir": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("path_dir expects 1 argument")
				}
				path, errObj := asString(args[0], "path")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: filepath.Dir(path)}
			},
		},
		"path_ext": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("path_ext expects 1 argument")
				}
				path, errObj := asString(args[0], "path")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: filepath.Ext(path)}
			},
		},
		"path_clean": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("path_clean expects 1 argument")
				}
				path, errObj := asString(args[0], "path")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: filepath.Clean(path)}
			},
		},
		"path_abs": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("path_abs expects 1 argument")
				}
				path, errObj := asString(args[0], "path")
				if errObj != nil {
					return errObj
				}
				out, err := filepath.Abs(path)
				if err != nil {
					return object.NewError("%s", err)
				}
				return &object.String{Value: out}
			},
		},
		"parse_duration": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("parse_duration expects 1 argument")
				}
				s, errObj := asString(args[0], "duration")
				if errObj != nil {
					return errObj
				}
				d, err := time.ParseDuration(s)
				if err != nil {
					return object.NewError("%s", err)
				}
				return &object.Integer{Value: d.Milliseconds()}
			},
		},
		"format_duration": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("format_duration expects 1 argument")
				}
				ms, errObj := asInt(args[0], "milliseconds")
				if errObj != nil {
					return errObj
				}
				return &object.String{Value: (time.Duration(ms) * time.Millisecond).String()}
			},
		},
		"start_of_month": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("start_of_month expects 1 argument")
				}
				ts, errObj := asInt(args[0], "unix_seconds")
				if errObj != nil {
					return errObj
				}
				tm := time.Unix(ts, 0).UTC()
				return &object.Integer{Value: time.Date(tm.Year(), tm.Month(), 1, 0, 0, 0, 0, time.UTC).Unix()}
			},
		},
		"end_of_week": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("end_of_week expects 1 argument")
				}
				ts, errObj := asInt(args[0], "unix_seconds")
				if errObj != nil {
					return errObj
				}
				tm := time.Unix(ts, 0).UTC()
				weekday := int(tm.Weekday())
				if weekday == 0 {
					weekday = 7
				}
				start := time.Date(tm.Year(), tm.Month(), tm.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(weekday - 1))
				return &object.Integer{Value: start.AddDate(0, 0, 7).Unix() - 1}
			},
		},
		"is_weekend": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("is_weekend expects 1 argument")
				}
				ts, errObj := asInt(args[0], "unix_seconds")
				if errObj != nil {
					return errObj
				}
				day := time.Unix(ts, 0).UTC().Weekday()
				return object.NativeBoolToBooleanObject(day == time.Saturday || day == time.Sunday)
			},
		},
		"weekday": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("weekday expects 1 argument")
				}
				ts, errObj := asInt(args[0], "unix_seconds")
				if errObj != nil {
					return errObj
				}
				return &object.Integer{Value: int64(time.Unix(ts, 0).UTC().Weekday())}
			},
		},
		"month": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("month expects 1 argument")
				}
				ts, errObj := asInt(args[0], "unix_seconds")
				if errObj != nil {
					return errObj
				}
				return &object.Integer{Value: int64(time.Unix(ts, 0).UTC().Month())}
			},
		},
		"year": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("year expects 1 argument")
				}
				ts, errObj := asInt(args[0], "unix_seconds")
				if errObj != nil {
					return errObj
				}
				return &object.Integer{Value: int64(time.Unix(ts, 0).UTC().Year())}
			},
		},
	})
}

func strconvParseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}
