package builtins

import (
	"math"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

func toFloat64(obj object.Object) *float64 {
	switch v := obj.(type) {
	case *object.Integer:
		f := float64(v.Value)
		return &f
	case *object.Float:
		return &v.Value
	default:
		return nil
	}
}

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{
		// Constants
		"PI":  {Fn: func(args ...object.Object) object.Object { return &object.Float{Value: math.Pi} }},
		"E":   {Fn: func(args ...object.Object) object.Object { return &object.Float{Value: math.E} }},
		"INF": {Fn: func(args ...object.Object) object.Object { return &object.Float{Value: math.Inf(1)} }},
		"NAN": {Fn: func(args ...object.Object) object.Object { return &object.Float{Value: math.NaN()} }},

		// Trig functions
		"sin": {Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("sin expects 1 argument")
			}
			v := toFloat64(args[0])
			if v == nil {
				return object.NewError("sin expects a number")
			}
			return &object.Float{Value: math.Sin(*v)}
		}},
		"cos": {Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("cos expects 1 argument")
			}
			v := toFloat64(args[0])
			if v == nil {
				return object.NewError("cos expects a number")
			}
			return &object.Float{Value: math.Cos(*v)}
		}},
		"tan": {Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("tan expects 1 argument")
			}
			v := toFloat64(args[0])
			if v == nil {
				return object.NewError("tan expects a number")
			}
			return &object.Float{Value: math.Tan(*v)}
		}},
		"asin": {Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("asin expects 1 argument")
			}
			v := toFloat64(args[0])
			if v == nil {
				return object.NewError("asin expects a number")
			}
			return &object.Float{Value: math.Asin(*v)}
		}},
		"acos": {Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("acos expects 1 argument")
			}
			v := toFloat64(args[0])
			if v == nil {
				return object.NewError("acos expects a number")
			}
			return &object.Float{Value: math.Acos(*v)}
		}},
		"atan": {Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("atan expects 1 argument")
			}
			v := toFloat64(args[0])
			if v == nil {
				return object.NewError("atan expects a number")
			}
			return &object.Float{Value: math.Atan(*v)}
		}},
		"atan2": {Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return object.NewError("atan2 expects 2 arguments")
			}
			y := toFloat64(args[0])
			x := toFloat64(args[1])
			if y == nil || x == nil {
				return object.NewError("atan2 expects numbers")
			}
			return &object.Float{Value: math.Atan2(*y, *x)}
		}},

		// Log functions
		"log": {Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("log expects 1 argument")
			}
			v := toFloat64(args[0])
			if v == nil {
				return object.NewError("log expects a number")
			}
			return &object.Float{Value: math.Log(*v)}
		}},
		"log2": {Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("log2 expects 1 argument")
			}
			v := toFloat64(args[0])
			if v == nil {
				return object.NewError("log2 expects a number")
			}
			return &object.Float{Value: math.Log2(*v)}
		}},
		"log10": {Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("log10 expects 1 argument")
			}
			v := toFloat64(args[0])
			if v == nil {
				return object.NewError("log10 expects a number")
			}
			return &object.Float{Value: math.Log10(*v)}
		}},

		// Hyperbolic
		"sinh": {Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("sinh expects 1 argument")
			}
			v := toFloat64(args[0])
			if v == nil {
				return object.NewError("sinh expects a number")
			}
			return &object.Float{Value: math.Sinh(*v)}
		}},
		"cosh": {Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("cosh expects 1 argument")
			}
			v := toFloat64(args[0])
			if v == nil {
				return object.NewError("cosh expects a number")
			}
			return &object.Float{Value: math.Cosh(*v)}
		}},
		"tanh": {Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("tanh expects 1 argument")
			}
			v := toFloat64(args[0])
			if v == nil {
				return object.NewError("tanh expects a number")
			}
			return &object.Float{Value: math.Tanh(*v)}
		}},

		// Other math
		"exp": {Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("exp expects 1 argument")
			}
			v := toFloat64(args[0])
			if v == nil {
				return object.NewError("exp expects a number")
			}
			return &object.Float{Value: math.Exp(*v)}
		}},
		"hypot": {Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return object.NewError("hypot expects 2 arguments")
			}
			a := toFloat64(args[0])
			b := toFloat64(args[1])
			if a == nil || b == nil {
				return object.NewError("hypot expects numbers")
			}
			return &object.Float{Value: math.Hypot(*a, *b)}
		}},
		"is_nan": {Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("is_nan expects 1 argument")
			}
			v := toFloat64(args[0])
			if v == nil {
				return object.FALSE
			}
			return object.NativeBoolToBooleanObject(math.IsNaN(*v))
		}},
		"is_inf": {Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("is_inf expects 1 argument")
			}
			v := toFloat64(args[0])
			if v == nil {
				return object.FALSE
			}
			return object.NativeBoolToBooleanObject(math.IsInf(*v, 0))
		}},
		"to_radians": {Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("to_radians expects 1 argument")
			}
			v := toFloat64(args[0])
			if v == nil {
				return object.NewError("to_radians expects a number")
			}
			return &object.Float{Value: *v * math.Pi / 180}
		}},
		"to_degrees": {Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.NewError("to_degrees expects 1 argument")
			}
			v := toFloat64(args[0])
			if v == nil {
				return object.NewError("to_degrees expects a number")
			}
			return &object.Float{Value: *v * 180 / math.Pi}
		}},
	})
}
