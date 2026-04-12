package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/oarkflow/interpreter/pkg/object"
)

// ---------------------------------------------------------------------------
// Function variables – plugged by the host (evaluator) package at init time.
// ---------------------------------------------------------------------------

// EvalProgramFn evaluates an AST program in the given environment. This must
// be set by the host package before calling RunProgramSandboxed.
var EvalProgramFn func(program any, env *object.Environment) object.Object

// WithSecurityPolicyOverrideFn temporarily sets a security policy and runs fn.
// Set by the host package.
var WithSecurityPolicyOverrideFn func(policy *object.SecurityPolicy, fn func() (object.Object, error)) (object.Object, error)

// ParseBoolEnvDefaultFn reads a boolean from the environment with a fallback.
var ParseBoolEnvDefaultFn func(name string, def bool) bool = func(name string, def bool) bool {
	v := os.Getenv(name)
	switch v {
	case "1", "true", "yes":
		return true
	case "0", "false", "no":
		return false
	default:
		return def
	}
}

// ---------------------------------------------------------------------------
// SandboxConfig
// ---------------------------------------------------------------------------

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

// DefaultExecSandboxConfig returns a SandboxConfig suitable for running
// scripts from the command line.
func DefaultExecSandboxConfig() SandboxConfig {
	return SandboxConfig{
		Enabled:       ParseBoolEnvDefaultFn("SPL_SANDBOX", true),
		StrictMode:    false,
		ProtectHost:   false,
		AllowEnvWrite: true,
		MaxDepth:      256,
		MaxSteps:      2_000_000,
		MaxHeapMB:     256,
		Timeout:       8 * time.Second,
	}
}

// DefaultReplSandboxConfig returns a SandboxConfig suitable for the
// interactive REPL.
func DefaultReplSandboxConfig() SandboxConfig {
	return SandboxConfig{
		Enabled:       ParseBoolEnvDefaultFn("SPL_SANDBOX", true),
		StrictMode:    true,
		ProtectHost:   true,
		AllowEnvWrite: false,
		MaxDepth:      256,
		MaxSteps:      2_000_000,
		MaxHeapMB:     256,
		Timeout:       0,
	}
}

// ---------------------------------------------------------------------------
// SandboxVM
// ---------------------------------------------------------------------------

type SandboxVM struct {
	env    *object.Environment
	policy *object.SecurityPolicy
	config SandboxConfig
}

var sandboxRootOverride struct {
	mu   sync.RWMutex
	root string
}

// NewSandboxVM creates a new sandbox-isolated VM with the given
// configuration, returning a SandboxVM ready for evaluation.
func NewSandboxVM(args []string, sourcePath string, moduleDir string, cfg SandboxConfig) (*SandboxVM, error) {
	env := object.NewGlobalEnvironment(args)
	if sourcePath == "" {
		sourcePath = "<memory>"
	}
	env.SourcePath = sourcePath

	if !cfg.Enabled {
		if moduleDir == "" {
			moduleDir = "."
		}
		env.ModuleDir = moduleDir
		return &SandboxVM{env: env, policy: nil, config: cfg}, nil
	}

	baseDir, err := sandboxBaseDir(moduleDir)
	if err != nil {
		return nil, err
	}
	if cfg.BaseDir == "" {
		cfg.BaseDir = baseDir
	}
	env.ModuleDir = baseDir
	env.RuntimeLimits = sandboxRuntimeLimits(cfg)
	env.SecurityPolicy = sandboxSecurityPolicy(cfg)

	return &SandboxVM{env: env, policy: env.SecurityPolicy, config: cfg}, nil
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

func sandboxRuntimeLimits(cfg SandboxConfig) *object.RuntimeLimits {
	rl := &object.RuntimeLimits{HeapCheckEvery: 128}
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

func sandboxSecurityPolicy(cfg SandboxConfig) *object.SecurityPolicy {
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

	return &object.SecurityPolicy{
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

// Environment returns the underlying Environment.
func (vm *SandboxVM) Environment() *object.Environment {
	if vm == nil {
		return nil
	}
	return vm.env
}

// Policy returns the SecurityPolicy used by this sandbox, if any.
func (vm *SandboxVM) Policy() *object.SecurityPolicy {
	if vm == nil {
		return nil
	}
	return vm.policy
}

// ResetRuntimeLimitCounters zeroes the step/depth counters on the
// environment's RuntimeLimits so a fresh evaluation can run.
func ResetRuntimeLimitCounters(env *object.Environment) {
	if env == nil || env.RuntimeLimits == nil {
		return
	}
	env.RuntimeLimits.Steps = 0
	env.RuntimeLimits.CurrentDepth = 0
}

// RunProgramSandboxed evaluates an AST program inside the sandbox, resetting
// counters and applying policy/root overrides.
// The program parameter is typed as `any` because the ast.Program type may
// live in a different package; callers should pass *ast.Program.
func RunProgramSandboxed(program any, env *object.Environment, policy *object.SecurityPolicy) object.Object {
	ResetRuntimeLimitCounters(env)
	if EvalProgramFn == nil {
		return object.NewError("sandbox: EvalProgramFn not set")
	}
	baseDir := ""
	if env != nil {
		baseDir = env.ModuleDir
	}
	if policy == nil {
		return WithSandboxRootOverride(baseDir, func() object.Object {
			return EvalProgramFn(program, env)
		})
	}
	obj := WithSandboxRootOverride(baseDir, func() object.Object {
		if WithSecurityPolicyOverrideFn == nil {
			return EvalProgramFn(program, env)
		}
		res, _ := WithSecurityPolicyOverrideFn(policy, func() (object.Object, error) {
			return EvalProgramFn(program, env), nil
		})
		return res
	})
	if obj == nil {
		return object.NULL
	}
	return obj
}

// WithSandboxRootOverride temporarily sets the sandbox root directory while
// running fn, then restores the previous value.
func WithSandboxRootOverride(root string, fn func() object.Object) object.Object {
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

// ActiveSandboxBaseDir returns the currently active sandbox root directory,
// if any override is in effect.
func ActiveSandboxBaseDir() string {
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
