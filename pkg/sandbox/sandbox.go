package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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

	MaxDepth           int
	MaxSteps           int64
	MaxHeapMB          int64
	MaxOutputBytes     int64
	MaxHTTPBodyBytes   int64
	MaxExecOutputBytes int64
	Timeout            time.Duration

	BaseDir string

	AllowedCapabilities   []string
	DeniedCapabilities    []string
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
	AllowedImportPaths    []string
	DeniedImportPaths     []string
	AllowedImportPackages []string
	DeniedImportPackages  []string
	AllowedNativeModules  []string
	DeniedNativeModules   []string
	DenyDynamicImports    bool
}

// DefaultExecSandboxConfig returns a SandboxConfig suitable for running
// scripts from the command line.
//
// Timeout is intentionally 0 (no wall-clock deadline): trusted CLI/embedding
// scripts routinely run long-lived servers, schedulers, and watchers via a
// blocking call (listen(), schedule_run(), watch(), ...), and a fixed
// deadline would silently stop evaluating everything running under that
// script's environment once the wall clock passed it - including future
// request/job/event callbacks - without the process crashing or erroring
// visibly. Per-call step/depth/heap limits (MaxSteps/MaxDepth/MaxHeapMB)
// still bound any single evaluation; callers that want a hard wall-clock cap
// on trusted scripts should set ExecOptions.Timeout or SandboxConfig.Timeout
// explicitly (e.g. via --timeout on the CLI).
func DefaultExecSandboxConfig() SandboxConfig {
	return SandboxConfig{
		Enabled:       ParseBoolEnvDefaultFn("SPL_SANDBOX", true),
		StrictMode:    false,
		ProtectHost:   false,
		AllowEnvWrite: true,
		MaxDepth:      256,
		MaxSteps:      2_000_000,
		MaxHeapMB:     256,
		Timeout:       0,
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

// sandboxRootOverride holds the process-wide active sandbox root override.
// `current` is an atomic value so ActiveSandboxBaseDir (called repeatedly
// from within the same goroutine that may be holding `mu` for the duration of
// WithSandboxRootOverride's fn() call) can read it lock-free without risking
// self-deadlock on a non-reentrant sync.Mutex. `mu` is used purely to
// serialize/queue concurrent WithSandboxRootOverride callers so overlapping
// override scopes don't interleave.
var sandboxRootOverride struct {
	mu      sync.Mutex
	current atomic.Value // string
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
	if cfg.MaxOutputBytes > 0 {
		rl.MaxOutputBytes = cfg.MaxOutputBytes
	}
	if cfg.MaxHTTPBodyBytes > 0 {
		rl.MaxHTTPBodyBytes = cfg.MaxHTTPBodyBytes
	}
	if cfg.MaxExecOutputBytes > 0 {
		rl.MaxExecOutputBytes = cfg.MaxExecOutputBytes
	}
	if cfg.Timeout > 0 {
		rl.Deadline = time.Now().Add(cfg.Timeout)
	}
	if rl.MaxDepth == 0 && rl.MaxSteps == 0 && rl.MaxHeapBytes == 0 && rl.MaxOutputBytes == 0 && rl.MaxHTTPBodyBytes == 0 && rl.MaxExecOutputBytes == 0 && rl.Deadline.IsZero() {
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
		AllowedCapabilities:   append([]string(nil), cfg.AllowedCapabilities...),
		DeniedCapabilities:    append([]string(nil), cfg.DeniedCapabilities...),
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
		AllowedImportPaths:    append([]string(nil), cfg.AllowedImportPaths...),
		DeniedImportPaths:     append([]string(nil), cfg.DeniedImportPaths...),
		AllowedImportPackages: append([]string(nil), cfg.AllowedImportPackages...),
		DeniedImportPackages:  append([]string(nil), cfg.DeniedImportPackages...),
		AllowedNativeModules:  append([]string(nil), cfg.AllowedNativeModules...),
		DeniedNativeModules:   append([]string(nil), cfg.DeniedNativeModules...),
		DenyDynamicImports:    cfg.DenyDynamicImports,
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
	env.RuntimeLimits.OutputBytes = 0
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
//
// The mutex is held for the FULL DURATION of fn(), not just for the swap, so
// that concurrent in-process callers (e.g. the playground's per-request
// EvalForPlayground calls) are fully serialized and can never observe or be
// affected by another request's sandbox root during the overlap window.
func WithSandboxRootOverride(root string, fn func() object.Object) object.Object {
	if stringsTrimSpace(root) == "" {
		return fn()
	}
	sandboxRootOverride.mu.Lock()
	defer sandboxRootOverride.mu.Unlock()

	prev, _ := sandboxRootOverride.current.Load().(string)
	sandboxRootOverride.current.Store(root)
	defer sandboxRootOverride.current.Store(prev)

	return fn()
}

// ActiveSandboxBaseDir returns the currently active sandbox root directory,
// if any override is in effect.
func ActiveSandboxBaseDir() string {
	root, _ := sandboxRootOverride.current.Load().(string)
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
