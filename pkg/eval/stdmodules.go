package eval

import "sync"

// StdModuleInfo mirrors a std/plugin module's registration: the prefix
// stripped from builtin names to form its members, and the full builtin
// names backing it.
type StdModuleInfo struct {
	Prefix       string
	BuiltinNames []string
}

var stdModuleRegistry = struct {
	mu      sync.RWMutex
	modules map[string]StdModuleInfo
}{modules: make(map[string]StdModuleInfo)}

// RegisterStdModule records that a std/plugin module named name (e.g.
// "secretr", "pdf", "std/core") exports builtinNames (optionally stripped
// of prefix to form member names). It exists so static-analysis tooling
// (pkg/tooling) can recognize `import "<name>" as alias` without a
// hand-maintained, easily-stale duplicate of the module list - see
// RegisterStdModuleFn in the root interpreter package's
// RegisterStdBuiltinModuleWithPrefix, which calls this for every module it
// registers, keeping this registry automatically in sync.
func RegisterStdModule(name, prefix string, builtinNames []string) {
	stdModuleRegistry.mu.Lock()
	defer stdModuleRegistry.mu.Unlock()
	stdModuleRegistry.modules[name] = StdModuleInfo{
		Prefix:       prefix,
		BuiltinNames: append([]string(nil), builtinNames...),
	}
}

// StdModules returns a snapshot of every module registered via
// RegisterStdModule, keyed by module name.
func StdModules() map[string]StdModuleInfo {
	stdModuleRegistry.mu.RLock()
	defer stdModuleRegistry.mu.RUnlock()
	out := make(map[string]StdModuleInfo, len(stdModuleRegistry.modules))
	for name, info := range stdModuleRegistry.modules {
		out[name] = info
	}
	return out
}
