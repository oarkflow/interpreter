// Package builtins contains the extracted builtin function implementations
// for the interpreter. Each file registers its builtins via eval.RegisterBuiltins
// in an init() function.
package builtins

import (
	"github.com/oarkflow/interpreter/pkg/object"
)

// asString extracts a string value from an object, supporting Secret types.
func asString(arg object.Object, name string) (string, object.Object) {
	if s, ok := arg.(*object.Secret); ok {
		return s.Value, nil
	}
	if arg.Type() != object.STRING_OBJ {
		return "", object.NewError("argument `%s` must be STRING, got %s", name, arg.Type())
	}
	return arg.(*object.String).Value, nil
}

// asInt extracts an int64 value from an object.
func asInt(arg object.Object, name string) (int64, object.Object) {
	if arg.Type() != object.INTEGER_OBJ {
		return 0, object.NewError("argument `%s` must be INTEGER, got %s", name, arg.Type())
	}
	return arg.(*object.Integer).Value, nil
}
