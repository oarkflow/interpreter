package interpreter

import (
	"fmt"
	"os"
)

func registerBuiltins(group map[string]*Builtin) {
	for name, fn := range group {
		if _, exists := builtins[name]; exists {
			fmt.Fprintf(os.Stderr, "warning: builtin %q already exists; skipping duplicate registration\n", name)
			continue
		}
		if fn != nil && fn.Fn == nil && fn.FnWithEnv != nil {
			captured := fn
			fn.Fn = func(args ...Object) Object {
				return captured.FnWithEnv(captured.Env, args...)
			}
		}
		builtins[name] = fn
	}
}
