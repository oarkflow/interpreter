package builtins

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/security"
)

type nativeCommandOptions struct {
	Timeout       time.Duration
	Cwd           string
	Env           map[string]string
	Stdin         string
	MaxOutput     int64
	SplitStreams  bool
	CheckDisabled bool
	CheckNative   bool
}

type nativeCommandResult struct {
	Command   string
	Args      []string
	Stdout    string
	Stderr    string
	Output    string
	ExitCode  int
	OK        bool
	TimedOut  bool
	Cancelled bool
	Truncated bool
	Err       error
}

type NativeCommandRunResult struct {
	Command   string
	Args      []string
	Stdout    string
	Stderr    string
	ExitCode  int
	OK        bool
	TimedOut  bool
	Cancelled bool
	Truncated bool
	Error     string
}

// NativeOSModule returns the import-only native/os module exports.
func NativeOSModule() map[string]object.Object {
	return map[string]object.Object{
		"run":          &object.Builtin{FnWithEnv: nativeOSRun},
		"which":        &object.Builtin{Fn: nativeOSWhich},
		"list":         &object.Builtin{Fn: nativeOSList},
		"platform":     &object.Builtin{Fn: nativeOSPlatform},
		"capabilities": &object.Builtin{Fn: nativeOSCapabilities},
	}
}

func RunNativeCommandForRepl(env *object.Environment, command string, args []string) NativeCommandRunResult {
	result := runNativeCommand(env, command, args, nativeCommandOptions{
		SplitStreams:  true,
		CheckDisabled: true,
		CheckNative:   true,
	})
	out := NativeCommandRunResult{
		Command:   result.Command,
		Args:      append([]string(nil), result.Args...),
		Stdout:    result.Stdout,
		Stderr:    result.Stderr,
		ExitCode:  result.ExitCode,
		OK:        result.OK,
		TimedOut:  result.TimedOut,
		Cancelled: result.Cancelled,
		Truncated: result.Truncated,
	}
	if result.Err != nil {
		out.Error = result.Err.Error()
	}
	return out
}

func nativeOSRun(env *object.Environment, args ...object.Object) object.Object {
	if len(args) < 2 || len(args) > 3 {
		return object.NewError("os.run(command, args[, opts]) expects 2 or 3 arguments, got %d", len(args))
	}
	command, errObj := asString(args[0], "command")
	if errObj != nil {
		return errObj
	}
	cmdArgs, errObj := stringArray(args[1], "args")
	if errObj != nil {
		return errObj
	}
	opts := nativeCommandOptions{
		SplitStreams:  true,
		CheckDisabled: true,
		CheckNative:   true,
	}
	if len(args) == 3 {
		parsed, errObj := nativeCommandOptionsFromObject(env, args[2])
		if errObj != nil {
			return errObj
		}
		parsed.SplitStreams = true
		parsed.CheckDisabled = true
		parsed.CheckNative = true
		opts = parsed
	}
	result := runNativeCommand(env, command, cmdArgs, opts)
	return eval.ToObject(map[string]interface{}{
		"ok":        result.OK,
		"exit_code": result.ExitCode,
		"stdout":    result.Stdout,
		"stderr":    result.Stderr,
		"timed_out": result.TimedOut,
		"cancelled": result.Cancelled,
		"truncated": result.Truncated,
		"command":   result.Command,
		"args":      result.Args,
		"error":     nativeCommandErrorString(result),
	})
}

func nativeOSWhich(args ...object.Object) object.Object {
	if len(args) != 1 {
		return object.NewError("os.which(command) expects 1 argument, got %d", len(args))
	}
	command, errObj := asString(args[0], "command")
	if errObj != nil {
		return errObj
	}
	if err := security.CheckNativeModuleAllowed("native/os"); err != nil {
		return object.NewError("%s", err)
	}
	if isTruthyEnvVar("SPL_DISABLE_EXEC") {
		return object.NewError("exec is disabled by SPL_DISABLE_EXEC")
	}
	if err := security.CheckExecAllowed(command); err != nil {
		return object.NewError("%s", err)
	}
	path, err := exec.LookPath(command)
	if err != nil {
		return object.NULL
	}
	return &object.String{Value: path}
}

func nativeOSList(args ...object.Object) object.Object {
	if len(args) > 1 {
		return object.NewError("os.list([opts]) expects at most 1 argument, got %d", len(args))
	}
	if err := security.CheckNativeModuleAllowed("native/os"); err != nil {
		return object.NewError("%s", err)
	}
	if isTruthyEnvVar("SPL_DISABLE_EXEC") {
		return &object.Array{Elements: []object.Object{}}
	}
	limit := int64(0)
	if len(args) == 1 {
		h, ok := args[0].(*object.Hash)
		if !ok {
			return object.NewError("argument `opts` must be HASH, got %s", args[0].Type())
		}
		if v, ok := hashGetString(h, "limit"); ok {
			n, errObj := asInt(v, "limit")
			if errObj != nil {
				return errObj
			}
			if n < 0 {
				return object.NewError("limit must be >= 0")
			}
			limit = n
		}
	}
	seen := map[string]struct{}{}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if _, exists := seen[name]; exists {
				continue
			}
			if err := security.CheckExecAllowed(name); err != nil {
				continue
			}
			seen[name] = struct{}{}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	if limit > 0 && int64(len(names)) > limit {
		names = names[:limit]
	}
	return eval.ToObject(names)
}

func nativeOSPlatform(args ...object.Object) object.Object {
	if len(args) != 0 {
		return object.NewError("os.platform() expects 0 arguments, got %d", len(args))
	}
	cwd, _ := os.Getwd()
	return eval.ToObject(map[string]interface{}{
		"os":             runtime.GOOS,
		"arch":           runtime.GOARCH,
		"cwd":            cwd,
		"path_separator": string(os.PathSeparator),
		"path_list_sep":  string(os.PathListSeparator),
		"shell":          defaultShellHint(),
		"env_available":  true,
	})
}

func nativeOSCapabilities(args ...object.Object) object.Object {
	if len(args) != 0 {
		return object.NewError("os.capabilities() expects 0 arguments, got %d", len(args))
	}
	allowed := func(cap string) bool {
		return security.CheckCapabilityAllowed(cap) == nil
	}
	return eval.ToObject(map[string]interface{}{
		"native_os":          security.CheckNativeModuleAllowed("native/os") == nil,
		"exec":               !isTruthyEnvVar("SPL_DISABLE_EXEC") && security.CheckCapabilityAllowed(security.CapabilityExec) == nil,
		"filesystem_read":    allowed(security.CapabilityFilesystemRead),
		"filesystem_write":   allowed(security.CapabilityFilesystemWrite),
		"network":            allowed(security.CapabilityNetwork),
		"system":             allowed(security.CapabilitySystem),
		"env_write":          allowed(security.CapabilityEnvWrite),
		"spl_disable_exec":   isTruthyEnvVar("SPL_DISABLE_EXEC"),
		"direct_invocation":  true,
		"shell_invocation":   false,
		"structured_results": true,
	})
}

func runNativeCommand(env *object.Environment, command string, args []string, opts nativeCommandOptions) nativeCommandResult {
	result := nativeCommandResult{
		Command:  command,
		Args:     append([]string(nil), args...),
		ExitCode: -1,
	}
	if opts.CheckDisabled && isTruthyEnvVar("SPL_DISABLE_EXEC") {
		result.Err = fmt.Errorf("exec is disabled by SPL_DISABLE_EXEC")
		return result
	}
	if opts.CheckNative {
		if err := security.CheckNativeModuleAllowed("native/os"); err != nil {
			result.Err = err
			return result
		}
	}
	if err := security.CheckExecAllowed(command); err != nil {
		result.Err = err
		return result
	}
	if opts.Timeout == 0 {
		timeout, timeoutErr := execTimeoutFromArgsAndEnv(0)
		if timeoutErr != nil {
			result.Err = errors.New(strings.TrimPrefix(timeoutErr.Inspect(), "ERROR: "))
			return result
		}
		opts.Timeout = timeout
	}
	ctx, cancel := runtimeContextWithTimeout(env, opts.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	if opts.Cwd != "" {
		cmd.Dir = opts.Cwd
	}
	if len(opts.Env) > 0 {
		cmd.Env = append(os.Environ(), envMapToList(opts.Env)...)
	}
	if opts.Stdin != "" {
		cmd.Stdin = strings.NewReader(opts.Stdin)
	}

	limit := execOutputLimit(env)
	if opts.MaxOutput > 0 && (limit <= 0 || opts.MaxOutput < limit) {
		limit = opts.MaxOutput
	}
	if opts.SplitStreams {
		var stdout, stderr cappedBuffer
		stdout.limit = limit
		stderr.limit = limit
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		result.Stdout = string(stdout.Bytes())
		result.Stderr = string(stderr.Bytes())
		result.Truncated = stdout.Truncated() || stderr.Truncated()
		finishNativeCommandResult(ctx, err, &result)
		return result
	}

	var output cappedBuffer
	output.limit = limit
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	result.Output = string(output.Bytes())
	result.Stdout = result.Output
	result.Truncated = output.Truncated()
	finishNativeCommandResult(ctx, err, &result)
	return result
}

func finishNativeCommandResult(ctx context.Context, err error, result *nativeCommandResult) {
	if err == nil {
		result.OK = true
		result.ExitCode = 0
		return
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
		result.Err = fmt.Errorf("command timed out")
		return
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		result.Cancelled = true
		result.Err = fmt.Errorf("command cancelled")
		return
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		result.Err = err
		return
	}
	result.Err = err
}

func nativeCommandOptionsFromObject(env *object.Environment, obj object.Object) (nativeCommandOptions, object.Object) {
	opts := nativeCommandOptions{}
	h, ok := obj.(*object.Hash)
	if !ok {
		return opts, object.NewError("argument `opts` must be HASH, got %s", obj.Type())
	}
	if v, ok := hashGetString(h, "timeout_ms"); ok {
		ms, errObj := asInt(v, "timeout_ms")
		if errObj != nil {
			return opts, errObj
		}
		timeout, timeoutErr := execTimeoutFromArgsAndEnv(ms)
		if timeoutErr != nil {
			return opts, timeoutErr
		}
		opts.Timeout = timeout
	}
	if v, ok := hashGetString(h, "cwd"); ok {
		cwd, errObj := asString(v, "cwd")
		if errObj != nil {
			return opts, errObj
		}
		safePath, err := SanitizePathLocal(cwd)
		if err != nil {
			return opts, object.NewError("%s", err)
		}
		if err := security.CheckFileReadAllowed(safePath); err != nil {
			return opts, object.NewError("%s", err)
		}
		opts.Cwd = safePath
	}
	if v, ok := hashGetString(h, "stdin"); ok {
		stdin, errObj := asString(v, "stdin")
		if errObj != nil {
			return opts, errObj
		}
		opts.Stdin = stdin
	}
	if v, ok := hashGetString(h, "max_output_bytes"); ok {
		maxOutput, errObj := asInt(v, "max_output_bytes")
		if errObj != nil {
			return opts, errObj
		}
		if maxOutput < 0 {
			return opts, object.NewError("max_output_bytes must be >= 0")
		}
		opts.MaxOutput = maxOutput
	}
	if v, ok := hashGetString(h, "env"); ok {
		extraEnv, errObj := stringMap(v, "env")
		if errObj != nil {
			return opts, errObj
		}
		for key := range extraEnv {
			if err := security.EnvWriteAllowed(key); err != nil {
				if sysErr := security.CheckCapabilityAllowed(security.CapabilitySystem); sysErr != nil {
					return opts, object.NewError("%s", err)
				}
			}
		}
		opts.Env = extraEnv
		_ = env
	}
	return opts, nil
}

func stringArray(obj object.Object, name string) ([]string, object.Object) {
	arr, ok := obj.(*object.Array)
	if !ok {
		return nil, object.NewError("argument `%s` must be ARRAY, got %s", name, obj.Type())
	}
	out := make([]string, 0, len(arr.Elements))
	for i, el := range arr.Elements {
		value, errObj := asString(el, fmt.Sprintf("%s[%d]", name, i))
		if errObj != nil {
			return nil, errObj
		}
		out = append(out, value)
	}
	return out, nil
}

func stringMap(obj object.Object, name string) (map[string]string, object.Object) {
	h, ok := obj.(*object.Hash)
	if !ok {
		return nil, object.NewError("argument `%s` must be HASH, got %s", name, obj.Type())
	}
	out := make(map[string]string, len(h.Pairs))
	for _, pair := range h.Pairs {
		key, errObj := asString(pair.Key, name+" key")
		if errObj != nil {
			return nil, errObj
		}
		value, errObj := asString(pair.Value, key)
		if errObj != nil {
			return nil, errObj
		}
		out[key] = value
	}
	return out, nil
}

func envMapToList(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for key, value := range values {
		out = append(out, key+"="+value)
	}
	sort.Strings(out)
	return out
}

func nativeCommandErrorString(result nativeCommandResult) interface{} {
	if result.Err == nil {
		return nil
	}
	return result.Err.Error()
}

func defaultShellHint() string {
	if shell := strings.TrimSpace(os.Getenv("SHELL")); shell != "" {
		return shell
	}
	if runtime.GOOS == "windows" {
		return "cmd.exe"
	}
	return "sh"
}

func legacyExecString(result nativeCommandResult, timeout time.Duration) object.Object {
	if result.Err == nil {
		return &object.String{Value: result.Output}
	}
	prefix := result.Err.Error()
	if result.TimedOut {
		prefix = fmt.Sprintf("command timed out after %dms", timeout/time.Millisecond)
	} else if result.Cancelled {
		prefix = "command cancelled"
	}
	if result.Output == "" {
		return &object.String{Value: "ERROR: " + prefix}
	}
	return &object.String{Value: fmt.Sprintf("ERROR: %s\n%s", prefix, result.Output)}
}
