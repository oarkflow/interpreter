// Package builtins contains the extracted builtin function implementations
// for the interpreter. Each file registers its builtins via eval.RegisterBuiltins
// in an init() function.
package builtins

import (
	"io"
	"os"

	"github.com/oarkflow/interpreter/pkg/object"
)

// runtimeOutput returns the writer that print-like builtins must use instead
// of writing to os.Stdout directly. Hosts that capture output (e.g. the LSP
// server's spl/evaluate, or any embedder running an untrusted script) set
// env.Output to redirect writes away from the real process stdout; writing
// straight to os.Stdout from a builtin bypasses that capture and, for stdio
// transports like the LSP server, corrupts the protocol stream itself.
func runtimeOutput(env *object.Environment) io.Writer {
	if env != nil && env.Output != nil {
		return env.Output
	}
	return os.Stdout
}

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
