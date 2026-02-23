package interpreter

import "fmt"

func registerBuiltins(group map[string]*Builtin) {
	for name, fn := range group {
		if _, exists := builtins[name]; exists {
			panic(fmt.Sprintf("builtin %q already exists", name))
		}
		builtins[name] = fn
	}
}
