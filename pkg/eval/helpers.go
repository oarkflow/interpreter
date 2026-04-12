package eval

import (
	"fmt"
	"reflect"

	"github.com/oarkflow/interpreter/pkg/object"
)

// ---------------------------------------------------------------------------
// ToObject converts a Go value to an object.Object.
// ---------------------------------------------------------------------------

func ToObject(val interface{}) object.Object {
	if val == nil {
		return object.NULL
	}

	switch v := val.(type) {
	case object.Object:
		return v
	case bool:
		return object.NativeBoolToBooleanObject(v)
	case int:
		return &object.Integer{Value: int64(v)}
	case int8:
		return &object.Integer{Value: int64(v)}
	case int16:
		return &object.Integer{Value: int64(v)}
	case int32:
		return &object.Integer{Value: int64(v)}
	case int64:
		return &object.Integer{Value: v}
	case uint:
		return &object.Integer{Value: int64(v)}
	case uint8:
		return &object.Integer{Value: int64(v)}
	case uint16:
		return &object.Integer{Value: int64(v)}
	case uint32:
		return &object.Integer{Value: int64(v)}
	case uint64:
		return &object.Integer{Value: int64(v)}
	case float32:
		return &object.Float{Value: float64(v)}
	case float64:
		return &object.Float{Value: v}
	case string:
		return &object.String{Value: v}
	case []object.Object:
		return &object.Array{Elements: append([]object.Object(nil), v...)}
	case []string:
		elements := make([]object.Object, len(v))
		for i := range v {
			elements[i] = &object.String{Value: v[i]}
		}
		return &object.Array{Elements: elements}
	case []interface{}:
		elements := make([]object.Object, len(v))
		for i := range v {
			elements[i] = ToObject(v[i])
		}
		return &object.Array{Elements: elements}
	case map[string]interface{}:
		pairs := make(map[object.HashKey]object.HashPair, len(v))
		for k, vv := range v {
			key := &object.String{Value: k}
			pairs[key.HashKey()] = object.HashPair{Key: key, Value: ToObject(vv)}
		}
		return &object.Hash{Pairs: pairs}
	case map[string]string:
		pairs := make(map[object.HashKey]object.HashPair, len(v))
		for k, vv := range v {
			key := &object.String{Value: k}
			pairs[key.HashKey()] = object.HashPair{Key: key, Value: &object.String{Value: vv}}
		}
		return &object.Hash{Pairs: pairs}
	}

	rv := reflect.ValueOf(val)

	switch rv.Kind() {
	case reflect.Bool:
		return object.NativeBoolToBooleanObject(rv.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &object.Integer{Value: rv.Int()}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &object.Integer{Value: int64(rv.Uint())}
	case reflect.Float32, reflect.Float64:
		return &object.Float{Value: rv.Float()}
	case reflect.String:
		return &object.String{Value: rv.String()}
	case reflect.Slice, reflect.Array:
		elements := make([]object.Object, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			elements[i] = ToObject(rv.Index(i).Interface())
		}
		return &object.Array{Elements: elements}
	case reflect.Map:
		pairs := make(map[object.HashKey]object.HashPair)
		iter := rv.MapRange()
		for iter.Next() {
			key := ToObject(iter.Key().Interface())
			hashKey, ok := key.(object.Hashable)
			if !ok {
				continue
			}
			val := ToObject(iter.Value().Interface())
			pairs[hashKey.HashKey()] = object.HashPair{Key: key, Value: val}
		}
		return &object.Hash{Pairs: pairs}
	case reflect.Struct:
		pairs := make(map[object.HashKey]object.HashPair)
		t := rv.Type()
		for i := 0; i < rv.NumField(); i++ {
			field := t.Field(i)
			fieldName := field.Name
			key := &object.String{Value: fieldName}
			val := ToObject(rv.Field(i).Interface())
			pairs[key.HashKey()] = object.HashPair{Key: key, Value: val}
		}
		return &object.Hash{Pairs: pairs}
	default:
		return &object.String{Value: fmt.Sprintf("%v", val)}
	}
}

// ---------------------------------------------------------------------------
// InjectData injects Go values into an SPL environment as variables.
// ---------------------------------------------------------------------------

func InjectData(env *object.Environment, data map[string]interface{}) {
	for k, v := range data {
		obj := ToObject(v)
		env.Set(k, obj)
	}
}
