package interpreter

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type SandboxConfig struct {
	Enabled bool

	StrictMode    bool
	ProtectHost   bool
	AllowEnvWrite bool

	MaxDepth  int
	MaxSteps  int64
	MaxHeapMB int64
	Timeout   time.Duration

	BaseDir string

	AllowedExecCommands   []string
	DeniedExecCommands    []string
	AllowedNetworkHosts   []string
	DeniedNetworkHosts    []string
	AllowedDBDrivers      []string
	DeniedDBDrivers       []string
	AllowedFileReadPaths  []string
	DeniedFileReadPaths   []string
	AllowedFileWritePaths []string
	DeniedFileWritePaths  []string
	AllowedDBDSNPatterns  []string
	DeniedDBDSNPatterns   []string
}

func DefaultExecSandboxConfig() SandboxConfig {
	return SandboxConfig{
		Enabled:       parseBoolEnvDefault("SPL_SANDBOX", true),
		StrictMode:    false,
		ProtectHost:   false,
		AllowEnvWrite: true,
		MaxDepth:      256,
		MaxSteps:      2_000_000,
		MaxHeapMB:     256,
		Timeout:       8 * time.Second,
	}
}

func DefaultReplSandboxConfig() SandboxConfig {
	return SandboxConfig{
		Enabled:       parseBoolEnvDefault("SPL_SANDBOX", true),
		StrictMode:    true,
		ProtectHost:   true,
		AllowEnvWrite: false,
		MaxDepth:      256,
		MaxSteps:      2_000_000,
		MaxHeapMB:     256,
		Timeout:       0,
	}
}

type SandboxVM struct {
	env    *Environment
	policy *SecurityPolicy
	config SandboxConfig
}

var sandboxRootOverride struct {
	mu   sync.RWMutex
	root string
}

func NewSandboxVM(args []string, sourcePath string, moduleDir string, cfg SandboxConfig) (*SandboxVM, error) {
	env := NewGlobalEnvironment(args)
	if sourcePath == "" {
		sourcePath = "<memory>"
	}
	env.sourcePath = sourcePath

	if !cfg.Enabled {
		if moduleDir == "" {
			moduleDir = "."
		}
		env.moduleDir = moduleDir
		return &SandboxVM{env: env, policy: nil, config: cfg}, nil
	}

	baseDir, err := sandboxBaseDir(moduleDir)
	if err != nil {
		return nil, err
	}
	if cfg.BaseDir == "" {
		cfg.BaseDir = baseDir
	}
	env.moduleDir = baseDir
	env.runtimeLimits = sandboxRuntimeLimits(cfg)
	env.securityPolicy = sandboxSecurityPolicy(cfg)

	return &SandboxVM{env: env, policy: env.securityPolicy, config: cfg}, nil
}

func sandboxBaseDir(moduleDir string) (string, error) {
	base := moduleDir
	if base == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		base = cwd
	}
	abs, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("resolve sandbox base directory: %w", err)
	}
	return abs, nil
}

func sandboxRuntimeLimits(cfg SandboxConfig) *RuntimeLimits {
	rl := &RuntimeLimits{heapCheckEvery: 128}
	if cfg.MaxDepth > 0 {
		rl.MaxDepth = cfg.MaxDepth
	}
	if cfg.MaxSteps > 0 {
		rl.MaxSteps = cfg.MaxSteps
	}
	if cfg.MaxHeapMB > 0 {
		rl.MaxHeapBytes = uint64(cfg.MaxHeapMB) * 1024 * 1024
	}
	if cfg.Timeout > 0 {
		rl.Deadline = time.Now().Add(cfg.Timeout)
	}
	if rl.MaxDepth == 0 && rl.MaxSteps == 0 && rl.MaxHeapBytes == 0 && rl.Deadline.IsZero() {
		return nil
	}
	return rl
}

func sandboxSecurityPolicy(cfg SandboxConfig) *SecurityPolicy {
	readPaths := append([]string(nil), cfg.AllowedFileReadPaths...)
	writePaths := append([]string(nil), cfg.AllowedFileWritePaths...)
	if cfg.BaseDir != "" {
		if len(readPaths) == 0 {
			readPaths = append(readPaths, cfg.BaseDir)
		}
		if len(writePaths) == 0 {
			writePaths = append(writePaths, cfg.BaseDir)
		}
	}

	return &SecurityPolicy{
		StrictMode:            cfg.StrictMode,
		ProtectHost:           cfg.ProtectHost,
		AllowEnvWrite:         cfg.AllowEnvWrite,
		AllowedExecCommands:   append([]string(nil), cfg.AllowedExecCommands...),
		DeniedExecCommands:    append([]string(nil), cfg.DeniedExecCommands...),
		AllowedNetworkHosts:   append([]string(nil), cfg.AllowedNetworkHosts...),
		DeniedNetworkHosts:    append([]string(nil), cfg.DeniedNetworkHosts...),
		AllowedDBDrivers:      append([]string(nil), cfg.AllowedDBDrivers...),
		DeniedDBDrivers:       append([]string(nil), cfg.DeniedDBDrivers...),
		AllowedFileReadPaths:  readPaths,
		DeniedFileReadPaths:   append([]string(nil), cfg.DeniedFileReadPaths...),
		AllowedFileWritePaths: writePaths,
		DeniedFileWritePaths:  append([]string(nil), cfg.DeniedFileWritePaths...),
		AllowedDBDSNPatterns:  append([]string(nil), cfg.AllowedDBDSNPatterns...),
		DeniedDBDSNPatterns:   append([]string(nil), cfg.DeniedDBDSNPatterns...),
	}
}

func (vm *SandboxVM) Environment() *Environment {
	if vm == nil {
		return nil
	}
	return vm.env
}

func (vm *SandboxVM) Policy() *SecurityPolicy {
	if vm == nil {
		return nil
	}
	return vm.policy
}

func resetRuntimeLimitCounters(env *Environment) {
	if env == nil || env.runtimeLimits == nil {
		return
	}
	env.runtimeLimits.Steps = 0
	env.runtimeLimits.CurrentDepth = 0
}

func runProgramSandboxed(program *Program, env *Environment, policy *SecurityPolicy) Object {
	resetRuntimeLimitCounters(env)
	baseDir := ""
	if env != nil {
		baseDir = env.moduleDir
	}
	if policy == nil {
		return withSandboxRootOverride(baseDir, func() Object {
			return runProgram(program, env)
		})
	}
	obj := withSandboxRootOverride(baseDir, func() Object {
		res, _ := withSecurityPolicyOverride(policy, func() (Object, error) {
			return runProgram(program, env), nil
		})
		return res
	})
	if obj == nil {
		return NULL
	}
	return obj
}

func withSandboxRootOverride(root string, fn func() Object) Object {
	if stringsTrimSpace(root) == "" {
		return fn()
	}
	sandboxRootOverride.mu.Lock()
	prev := sandboxRootOverride.root
	sandboxRootOverride.root = root
	sandboxRootOverride.mu.Unlock()

	defer func() {
		sandboxRootOverride.mu.Lock()
		sandboxRootOverride.root = prev
		sandboxRootOverride.mu.Unlock()
	}()

	return fn()
}

func activeSandboxBaseDir() string {
	sandboxRootOverride.mu.RLock()
	root := sandboxRootOverride.root
	sandboxRootOverride.mu.RUnlock()
	return root
}

func stringsTrimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	if start == 0 && end == len(s) {
		return s
	}
	return s[start:end]
}
