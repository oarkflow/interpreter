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
	if name == "builtins" {
		names := eval.BuiltinNames()
		out := make(map[string]Object, len(names))
		for _, builtinName := range names {
			if fn, ok := eval.BuiltinByName(builtinName); ok {
				out[builtinName] = fn
			}
		}
		return out, true
	}
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
			} else if optionalBuiltinModule(name) {
				out[builtinName] = unavailableBuiltin(name, builtinName)
			}
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	}
	return nil, false
}

func optionalBuiltinModule(name string) bool {
	switch name {
	case "database", "images", "integrations", "cryptoextra", "yaml", "config/yaml":
		return true
	default:
		return false
	}
}

func unavailableBuiltin(moduleName, builtinName string) *object.Builtin {
	return &object.Builtin{Fn: func(args ...object.Object) object.Object {
		return object.NewError("optional builtin %q from module %q is not linked into this interpreter; use cmd/interpreter-full, import the optional Go package in your embedding host, or build a custom preset", builtinName, moduleName)
	}}
}

var stdModules = struct {
	mu           sync.RWMutex
	items        map[string]map[string]Object
	builtinItems map[string][]string
}{items: map[string]map[string]Object{}, builtinItems: map[string][]string{}}

func init() {
	_ = RegisterStdBuiltinModule("std/core", "help", "sprintf", "printf", "interpolate", "len", "type")
	_ = RegisterStdBuiltinModule("core", "help", "sprintf", "printf", "interpolate", "len", "type")
	_ = RegisterStdBuiltinModule("std/fs", "read_file", "write_file", "file_exists", "remove_file", "readdir", "glob", "mkdir", "rmdir", "stat")
	_ = RegisterStdBuiltinModule("fs", "read_file", "write_file", "file_exists", "remove_file", "readdir", "glob", "mkdir", "rmdir", "stat")
	_ = RegisterStdBuiltinModule("std/render", "file", "image", "render")
	_ = RegisterStdBuiltinModule("render", "file", "image", "render")
	_ = RegisterStdBuiltinModule("std/test", "assert_true", "assert_eq", "assert_neq", "assert_contains", "assert_throws", "test_summary", "run_tests")
	_ = RegisterStdBuiltinModule("test", "assert_true", "assert_eq", "assert_neq", "assert_contains", "assert_throws", "test_summary", "run_tests")
	_ = RegisterStdBuiltinModule("std/config", "config_load", "config_parse", "secret", "secret_reveal", "secret_mask")
	_ = RegisterStdBuiltinModule("config", "config_load", "config_parse", "secret", "secret_reveal", "secret_mask")
	_ = RegisterStdBuiltinModule("database", "db_connect", "db_query", "db_exec", "db_begin", "db_commit", "db_rollback", "db_tables", "db_close", "query", "lazy_query")
	_ = RegisterStdBuiltinModule("images", "image_load", "image_resize", "image_crop", "image_rotate", "image_convert", "image_save", "image_info", "image_render", "image_resize_file", "image_convert_file")
	_ = RegisterStdBuiltinModule("integrations", "http_request", "http_get", "http_post", "webhook", "smtp_send", "ftp_list", "ftp_get", "ftp_put", "sftp_list", "sftp_get", "sftp_put")
	_ = RegisterStdBuiltinModule("cryptoextra", "bcrypt_hash", "bcrypt_verify")
	_ = RegisterStdBuiltinModule("yaml", "config_load", "config_parse")
	_ = RegisterStdBuiltinModule("config/yaml", "config_load", "config_parse")
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
