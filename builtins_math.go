package interpreter

import "math"

func init() {
	registerBuiltins(mathBuiltins)
}

var mathBuiltins = map[string]*Builtin{
	// Constants
	"PI":  {Fn: func(args ...Object) Object { return &Float{Value: math.Pi} }},
	"E":   {Fn: func(args ...Object) Object { return &Float{Value: math.E} }},
	"INF": {Fn: func(args ...Object) Object { return &Float{Value: math.Inf(1)} }},
	"NAN": {Fn: func(args ...Object) Object { return &Float{Value: math.NaN()} }},

	// Trig functions
	"sin": {Fn: func(args ...Object) Object {
		if len(args) != 1 {
			return newError("sin expects 1 argument")
		}
		v := toFloat64(args[0])
		if v == nil {
			return newError("sin expects a number")
		}
		return &Float{Value: math.Sin(*v)}
	}},
	"cos": {Fn: func(args ...Object) Object {
		if len(args) != 1 {
			return newError("cos expects 1 argument")
		}
		v := toFloat64(args[0])
		if v == nil {
			return newError("cos expects a number")
		}
		return &Float{Value: math.Cos(*v)}
	}},
	"tan": {Fn: func(args ...Object) Object {
		if len(args) != 1 {
			return newError("tan expects 1 argument")
		}
		v := toFloat64(args[0])
		if v == nil {
			return newError("tan expects a number")
		}
		return &Float{Value: math.Tan(*v)}
	}},
	"asin": {Fn: func(args ...Object) Object {
		if len(args) != 1 {
			return newError("asin expects 1 argument")
		}
		v := toFloat64(args[0])
		if v == nil {
			return newError("asin expects a number")
		}
		return &Float{Value: math.Asin(*v)}
	}},
	"acos": {Fn: func(args ...Object) Object {
		if len(args) != 1 {
			return newError("acos expects 1 argument")
		}
		v := toFloat64(args[0])
		if v == nil {
			return newError("acos expects a number")
		}
		return &Float{Value: math.Acos(*v)}
	}},
	"atan": {Fn: func(args ...Object) Object {
		if len(args) != 1 {
			return newError("atan expects 1 argument")
		}
		v := toFloat64(args[0])
		if v == nil {
			return newError("atan expects a number")
		}
		return &Float{Value: math.Atan(*v)}
	}},
	"atan2": {Fn: func(args ...Object) Object {
		if len(args) != 2 {
			return newError("atan2 expects 2 arguments")
		}
		y := toFloat64(args[0])
		x := toFloat64(args[1])
		if y == nil || x == nil {
			return newError("atan2 expects numbers")
		}
		return &Float{Value: math.Atan2(*y, *x)}
	}},

	// Log functions
	"log": {Fn: func(args ...Object) Object {
		if len(args) != 1 {
			return newError("log expects 1 argument")
		}
		v := toFloat64(args[0])
		if v == nil {
			return newError("log expects a number")
		}
		return &Float{Value: math.Log(*v)}
	}},
	"log2": {Fn: func(args ...Object) Object {
		if len(args) != 1 {
			return newError("log2 expects 1 argument")
		}
		v := toFloat64(args[0])
		if v == nil {
			return newError("log2 expects a number")
		}
		return &Float{Value: math.Log2(*v)}
	}},
	"log10": {Fn: func(args ...Object) Object {
		if len(args) != 1 {
			return newError("log10 expects 1 argument")
		}
		v := toFloat64(args[0])
		if v == nil {
			return newError("log10 expects a number")
		}
		return &Float{Value: math.Log10(*v)}
	}},

	// Hyperbolic
	"sinh": {Fn: func(args ...Object) Object {
		if len(args) != 1 {
			return newError("sinh expects 1 argument")
		}
		v := toFloat64(args[0])
		if v == nil {
			return newError("sinh expects a number")
		}
		return &Float{Value: math.Sinh(*v)}
	}},
	"cosh": {Fn: func(args ...Object) Object {
		if len(args) != 1 {
			return newError("cosh expects 1 argument")
		}
		v := toFloat64(args[0])
		if v == nil {
			return newError("cosh expects a number")
		}
		return &Float{Value: math.Cosh(*v)}
	}},
	"tanh": {Fn: func(args ...Object) Object {
		if len(args) != 1 {
			return newError("tanh expects 1 argument")
		}
		v := toFloat64(args[0])
		if v == nil {
			return newError("tanh expects a number")
		}
		return &Float{Value: math.Tanh(*v)}
	}},

	// Other math
	"exp": {Fn: func(args ...Object) Object {
		if len(args) != 1 {
			return newError("exp expects 1 argument")
		}
		v := toFloat64(args[0])
		if v == nil {
			return newError("exp expects a number")
		}
		return &Float{Value: math.Exp(*v)}
	}},
	"hypot": {Fn: func(args ...Object) Object {
		if len(args) != 2 {
			return newError("hypot expects 2 arguments")
		}
		a := toFloat64(args[0])
		b := toFloat64(args[1])
		if a == nil || b == nil {
			return newError("hypot expects numbers")
		}
		return &Float{Value: math.Hypot(*a, *b)}
	}},
	"is_nan": {Fn: func(args ...Object) Object {
		if len(args) != 1 {
			return newError("is_nan expects 1 argument")
		}
		v := toFloat64(args[0])
		if v == nil {
			return FALSE
		}
		return nativeBoolToBooleanObject(math.IsNaN(*v))
	}},
	"is_inf": {Fn: func(args ...Object) Object {
		if len(args) != 1 {
			return newError("is_inf expects 1 argument")
		}
		v := toFloat64(args[0])
		if v == nil {
			return FALSE
		}
		return nativeBoolToBooleanObject(math.IsInf(*v, 0))
	}},
	"to_radians": {Fn: func(args ...Object) Object {
		if len(args) != 1 {
			return newError("to_radians expects 1 argument")
		}
		v := toFloat64(args[0])
		if v == nil {
			return newError("to_radians expects a number")
		}
		return &Float{Value: *v * math.Pi / 180}
	}},
	"to_degrees": {Fn: func(args ...Object) Object {
		if len(args) != 1 {
			return newError("to_degrees expects 1 argument")
		}
		v := toFloat64(args[0])
		if v == nil {
			return newError("to_degrees expects a number")
		}
		return &Float{Value: *v * 180 / math.Pi}
	}},
}

func toFloat64(obj Object) *float64 {
	switch v := obj.(type) {
	case *Integer:
		f := float64(v.Value)
		return &f
	case *Float:
		return &v.Value
	default:
		return nil
	}
}
