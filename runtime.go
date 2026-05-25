package interpreter

import (
	"context"
	"fmt"
	"io"
	"time"

	sessionpkg "github.com/oarkflow/interpreter/pkg/session"
)

type RuntimeOptions struct {
	Profile                string
	ModuleDir              string
	Args                   []string
	Security               *SecurityPolicy
	Sandbox                *SandboxConfig
	Observability          *ObservabilityHooks
	Output                 io.Writer
	WorkerCommand          []string
	RequireOSIsolation     bool
	AllowInProcessFallback bool
	MaxSourceBytes         int64
	MaxDepth               int
	MaxSteps               int64
	MaxHeapMB              int64
	MaxOutputBytes         int64
	MaxHTTPBodyBytes       int64
	MaxExecOutputBytes     int64
	MaxStringBytes         int64
	MaxArrayLength         int
	MaxHashEntries         int
	MaxImportDepth         int
	MaxImportCount         int
	Timeout                time.Duration
	Context                context.Context
	Plugins                []Plugin
}

type Runtime struct {
	opts RuntimeOptions
}

func NewRuntime(opts RuntimeOptions) (*Runtime, error) {
	if opts.Profile == "" {
		opts.Profile = "trusted"
	}
	if opts.Security == nil {
		policy, sandboxCfg, err := CapabilityPreset(opts.Profile, opts.ModuleDir)
		if err != nil {
			return nil, err
		}
		opts.Security = policy
		if opts.Sandbox == nil {
			opts.Sandbox = &sandboxCfg
		}
	}
	rt := &Runtime{opts: opts}
	for _, plugin := range opts.Plugins {
		if plugin == nil {
			continue
		}
		if err := plugin.Register(rt); err != nil {
			return nil, fmt.Errorf("register plugin %q: %w", plugin.Name(), err)
		}
	}
	return rt, nil
}

func MustRuntime(opts RuntimeOptions) *Runtime {
	rt, err := NewRuntime(opts)
	if err != nil {
		panic(err)
	}
	return rt
}

func (rt *Runtime) Exec(script string, data map[string]interface{}) (Object, error) {
	return ExecWithOptions(script, data, rt.execOptions())
}

func (rt *Runtime) ExecFile(filename string, data map[string]interface{}) (Object, error) {
	return ExecFileWithOptions(filename, data, rt.execOptions())
}

func (rt *Runtime) NewSession(opts SessionOptions) (*Session, error) {
	if rt != nil {
		rtOpts := rt.Options()
		if opts.Profile == "" {
			opts.Profile = rtOpts.Profile
		}
		if opts.ModuleDir == "" {
			opts.ModuleDir = rtOpts.ModuleDir
		}
		if opts.Security == nil {
			opts.Security = rtOpts.Security
		}
		if opts.Sandbox == nil {
			opts.Sandbox = rtOpts.Sandbox
		}
		if opts.Output == nil {
			opts.Output = rtOpts.Output
		}
		if opts.MaxDepth == 0 {
			opts.MaxDepth = rtOpts.MaxDepth
		}
		if opts.MaxSteps == 0 {
			opts.MaxSteps = rtOpts.MaxSteps
		}
		if opts.MaxHeapMB == 0 {
			opts.MaxHeapMB = rtOpts.MaxHeapMB
		}
		if opts.MaxOutputBytes == 0 {
			opts.MaxOutputBytes = rtOpts.MaxOutputBytes
		}
		if opts.Timeout == 0 {
			opts.Timeout = rtOpts.Timeout
		}
	}
	return sessionpkg.New(opts)
}

func (rt *Runtime) Options() RuntimeOptions {
	if rt == nil {
		return RuntimeOptions{}
	}
	return rt.opts
}

func (rt *Runtime) execOptions() ExecOptions {
	if rt == nil {
		return ExecOptions{}
	}
	return ExecOptions{
		Args:                   append([]string(nil), rt.opts.Args...),
		ModuleDir:              rt.opts.ModuleDir,
		Profile:                rt.opts.Profile,
		WorkerCommand:          append([]string(nil), rt.opts.WorkerCommand...),
		MaxSourceBytes:         rt.opts.MaxSourceBytes,
		MaxDepth:               rt.opts.MaxDepth,
		MaxSteps:               rt.opts.MaxSteps,
		MaxHeapMB:              rt.opts.MaxHeapMB,
		MaxOutputBytes:         rt.opts.MaxOutputBytes,
		MaxHTTPBodyBytes:       rt.opts.MaxHTTPBodyBytes,
		MaxExecOutputBytes:     rt.opts.MaxExecOutputBytes,
		MaxStringBytes:         rt.opts.MaxStringBytes,
		MaxArrayLength:         rt.opts.MaxArrayLength,
		MaxHashEntries:         rt.opts.MaxHashEntries,
		MaxImportDepth:         rt.opts.MaxImportDepth,
		MaxImportCount:         rt.opts.MaxImportCount,
		Timeout:                rt.opts.Timeout,
		Context:                rt.opts.Context,
		Output:                 rt.opts.Output,
		Security:               rt.opts.Security,
		Sandbox:                rt.opts.Sandbox,
		Observability:          rt.opts.Observability,
		RequireOSIsolation:     rt.opts.RequireOSIsolation,
		AllowInProcessFallback: rt.opts.AllowInProcessFallback,
	}
}
