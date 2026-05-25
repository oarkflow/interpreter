package interpreter

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/security"
)

type Plugin interface {
	Name() string
	Register(*Runtime) error
}

type PluginFunc struct {
	PluginName string
	Fn         func(*Runtime) error
}

func (p PluginFunc) Name() string {
	if strings.TrimSpace(p.PluginName) == "" {
		return "anonymous"
	}
	return p.PluginName
}

func (p PluginFunc) Register(rt *Runtime) error {
	if p.Fn == nil {
		return nil
	}
	return p.Fn(rt)
}

func RegisterRuntimeBuiltins(group map[string]*object.Builtin) {
	eval.RegisterBuiltins(group)
}

func RegisterStdModule(name string, exports map[string]Object) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("std module name cannot be empty")
	}
	if exports == nil {
		exports = map[string]Object{}
	}
	stdModules.mu.Lock()
	defer stdModules.mu.Unlock()
	stdModules.items[name] = exports
	return nil
}

func RegisterStdBuiltinModule(name string, builtinNames ...string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("std module name cannot be empty")
	}
	stdModules.mu.Lock()
	defer stdModules.mu.Unlock()
	stdModules.builtinItems[name] = append([]string(nil), builtinNames...)
	return nil
}

func LookupStdModule(name string) (map[string]Object, bool) {
	name = strings.TrimSpace(name)
	stdModules.mu.RLock()
	exports, ok := stdModules.items[name]
	builtinNames, builtinOK := stdModules.builtinItems[name]
	stdModules.mu.RUnlock()
	if ok {
		out := make(map[string]Object, len(exports))
		for k, v := range exports {
			out[k] = v
		}
		return out, true
	}
	if builtinOK {
		out := make(map[string]Object, len(builtinNames))
		for _, builtinName := range builtinNames {
			if fn, ok := eval.BuiltinByName(builtinName); ok {
				out[builtinName] = fn
			}
		}
		return out, true
	}
	return nil, false
}

var stdModules = struct {
	mu           sync.RWMutex
	items        map[string]map[string]Object
	builtinItems map[string][]string
}{items: map[string]map[string]Object{}, builtinItems: map[string][]string{}}

func init() {
	_ = RegisterStdBuiltinModule("std/core", "help", "sprintf", "printf", "interpolate", "len", "type")
	_ = RegisterStdBuiltinModule("std/fs", "read_file", "write_file", "file_exists", "remove_file", "readdir", "glob", "mkdir", "rmdir", "stat")
	_ = RegisterStdBuiltinModule("std/render", "file", "image", "render")
	_ = RegisterStdBuiltinModule("std/test", "assert_true", "assert_eq", "assert_neq", "assert_contains", "assert_throws", "test_summary", "run_tests")
	_ = RegisterStdBuiltinModule("std/config", "config_load", "config_parse", "secret", "secret_reveal", "secret_mask")
}

func CapabilityPreset(name string, moduleDir string) (*SecurityPolicy, SandboxConfig, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		name = "trusted"
	}
	if strings.TrimSpace(moduleDir) == "" {
		moduleDir = "."
	}
	cfg := DefaultExecSandboxConfig()
	cfg.BaseDir = moduleDir
	policy := &SecurityPolicy{AllowEnvWrite: true}

	switch name {
	case "trusted":
		return policy, cfg, nil
	case "untrusted", "readonly":
		cfg.StrictMode = true
		cfg.ProtectHost = true
		cfg.AllowEnvWrite = false
		cfg.MaxDepth = 128
		cfg.MaxSteps = 500_000
		cfg.MaxHeapMB = 64
		cfg.MaxOutputBytes = 64 * 1024
		cfg.MaxHTTPBodyBytes = 64 * 1024
		cfg.MaxExecOutputBytes = 64 * 1024
		cfg.Timeout = 2 * time.Second
		policy = &SecurityPolicy{
			StrictMode:          true,
			ProtectHost:         true,
			AllowedCapabilities: []string{security.CapabilityFilesystemRead},
			AllowedFileReadPaths: []string{
				moduleDir,
			},
		}
	case "networked":
		policy, cfg, _ = CapabilityPreset("untrusted", moduleDir)
		policy.AllowedCapabilities = append(policy.AllowedCapabilities, security.CapabilityNetwork)
	case "data-processing":
		policy, cfg, _ = CapabilityPreset("untrusted", moduleDir)
		policy.AllowedCapabilities = append(policy.AllowedCapabilities, security.CapabilityDB)
	case "automation":
		policy, cfg, _ = CapabilityPreset("untrusted", moduleDir)
		policy.AllowedCapabilities = append(policy.AllowedCapabilities, security.CapabilityExec, security.CapabilityFilesystemWrite)
	case "server":
		policy, cfg, _ = CapabilityPreset("untrusted", moduleDir)
		policy.AllowedCapabilities = append(policy.AllowedCapabilities, security.CapabilityServer, security.CapabilityNetwork)
	default:
		return nil, SandboxConfig{}, fmt.Errorf("unknown capability preset %q", name)
	}
	return policy, cfg, nil
}
