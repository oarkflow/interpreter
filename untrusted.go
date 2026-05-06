package interpreter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	defaultUntrustedMaxSourceBytes = 1 << 20
	defaultUntrustedMaxOutputBytes = 64 << 10
)

type UntrustedExecOptions struct {
	Args      []string
	ModuleDir string

	MaxSourceBytes     int64
	MaxDepth           int
	MaxSteps           int64
	MaxHeapMB          int64
	MaxOutputBytes     int64
	MaxHTTPBodyBytes   int64
	MaxExecOutputBytes int64
	Timeout            time.Duration

	AllowedCapabilities   []string
	AllowedExecCommands   []string
	AllowedNetworkHosts   []string
	AllowedDBDrivers      []string
	AllowedDBDSNPatterns  []string
	AllowedFileReadPaths  []string
	AllowedFileWritePaths []string

	WorkerCommand          []string
	RequireOSIsolation     bool
	AllowInProcessFallback bool
	InProcess              bool
}

type untrustedWorkerRequest struct {
	Script  string                 `json:"script"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Options UntrustedExecOptions   `json:"options"`
}

type untrustedWorkerResponse struct {
	Result        interface{}   `json:"result,omitempty"`
	ResultInspect string        `json:"result_inspect,omitempty"`
	ResultType    string        `json:"result_type,omitempty"`
	Output        string        `json:"output,omitempty"`
	Error         string        `json:"error,omitempty"`
	ErrorKind     ExecErrorKind `json:"error_kind,omitempty"`
}

func ExecUntrusted(script string, data map[string]interface{}) (Object, error) {
	return ExecUntrustedWithOptions(script, data, UntrustedExecOptions{})
}

func ExecFileUntrusted(filename string, data map[string]interface{}) (Object, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, &ExecError{Kind: ExecErrorIO, Message: err.Error(), Path: filename}
	}
	opts := UntrustedExecOptions{ModuleDir: filepath.Dir(filename)}
	return ExecUntrustedWithOptions(string(content), data, opts)
}

func ExecUntrustedWithOptions(script string, data map[string]interface{}, opts UntrustedExecOptions) (Object, error) {
	opts = normalizeUntrustedOptions(opts)
	if int64(len(script)) > opts.MaxSourceBytes {
		return nil, &ExecError{Kind: ExecErrorValidation, Message: fmt.Sprintf("source exceeds %d bytes", opts.MaxSourceBytes)}
	}
	if opts.InProcess {
		return execUntrustedInProcess(script, data, opts)
	}
	obj, err := execUntrustedWorker(script, data, opts)
	if err == nil || !opts.AllowInProcessFallback {
		return obj, err
	}
	return execUntrustedInProcess(script, data, opts)
}

func ExecFileUntrustedWithOptions(filename string, data map[string]interface{}, opts UntrustedExecOptions) (Object, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, &ExecError{Kind: ExecErrorIO, Message: err.Error(), Path: filename}
	}
	if opts.ModuleDir == "" {
		opts.ModuleDir = filepath.Dir(filename)
	}
	return ExecUntrustedWithOptions(string(content), data, opts)
}

func normalizeUntrustedOptions(opts UntrustedExecOptions) UntrustedExecOptions {
	if opts.MaxSourceBytes <= 0 {
		opts.MaxSourceBytes = defaultUntrustedMaxSourceBytes
	}
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = 128
	}
	if opts.MaxSteps <= 0 {
		opts.MaxSteps = 500_000
	}
	if opts.MaxHeapMB <= 0 {
		opts.MaxHeapMB = 64
	}
	if opts.MaxOutputBytes <= 0 {
		opts.MaxOutputBytes = defaultUntrustedMaxOutputBytes
	}
	if opts.MaxHTTPBodyBytes <= 0 {
		opts.MaxHTTPBodyBytes = defaultUntrustedMaxOutputBytes
	}
	if opts.MaxExecOutputBytes <= 0 {
		opts.MaxExecOutputBytes = defaultUntrustedMaxOutputBytes
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Second
	}
	if opts.ModuleDir == "" {
		opts.ModuleDir = "."
	}
	if len(opts.AllowedCapabilities) == 0 {
		opts.AllowedCapabilities = []string{"filesystem_read"}
	}
	if len(opts.AllowedFileReadPaths) == 0 {
		opts.AllowedFileReadPaths = []string{opts.ModuleDir}
	}
	return opts
}

func untrustedSecurityPolicy(opts UntrustedExecOptions) *SecurityPolicy {
	return &SecurityPolicy{
		StrictMode:            true,
		ProtectHost:           true,
		AllowEnvWrite:         false,
		AllowedCapabilities:   append([]string(nil), opts.AllowedCapabilities...),
		AllowedExecCommands:   append([]string(nil), opts.AllowedExecCommands...),
		AllowedNetworkHosts:   append([]string(nil), opts.AllowedNetworkHosts...),
		AllowedDBDrivers:      append([]string(nil), opts.AllowedDBDrivers...),
		AllowedDBDSNPatterns:  append([]string(nil), opts.AllowedDBDSNPatterns...),
		AllowedFileReadPaths:  append([]string(nil), opts.AllowedFileReadPaths...),
		AllowedFileWritePaths: append([]string(nil), opts.AllowedFileWritePaths...),
	}
}

func untrustedSandboxConfig(opts UntrustedExecOptions) SandboxConfig {
	return SandboxConfig{
		Enabled:               true,
		StrictMode:            true,
		ProtectHost:           true,
		AllowEnvWrite:         false,
		MaxDepth:              opts.MaxDepth,
		MaxSteps:              opts.MaxSteps,
		MaxHeapMB:             opts.MaxHeapMB,
		MaxOutputBytes:        opts.MaxOutputBytes,
		MaxHTTPBodyBytes:      opts.MaxHTTPBodyBytes,
		MaxExecOutputBytes:    opts.MaxExecOutputBytes,
		Timeout:               opts.Timeout,
		BaseDir:               opts.ModuleDir,
		AllowedCapabilities:   append([]string(nil), opts.AllowedCapabilities...),
		AllowedExecCommands:   append([]string(nil), opts.AllowedExecCommands...),
		AllowedNetworkHosts:   append([]string(nil), opts.AllowedNetworkHosts...),
		AllowedDBDrivers:      append([]string(nil), opts.AllowedDBDrivers...),
		AllowedDBDSNPatterns:  append([]string(nil), opts.AllowedDBDSNPatterns...),
		AllowedFileReadPaths:  append([]string(nil), opts.AllowedFileReadPaths...),
		AllowedFileWritePaths: append([]string(nil), opts.AllowedFileWritePaths...),
	}
}

func execUntrustedInProcess(script string, data map[string]interface{}, opts UntrustedExecOptions) (Object, error) {
	output := &limitWriter{limit: opts.MaxOutputBytes}
	return ExecWithOptions(script, data, ExecOptions{
		Args:               opts.Args,
		ModuleDir:          opts.ModuleDir,
		MaxDepth:           opts.MaxDepth,
		MaxSteps:           opts.MaxSteps,
		MaxHeapMB:          opts.MaxHeapMB,
		MaxOutputBytes:     opts.MaxOutputBytes,
		MaxHTTPBodyBytes:   opts.MaxHTTPBodyBytes,
		MaxExecOutputBytes: opts.MaxExecOutputBytes,
		Timeout:            opts.Timeout,
		Output:             output,
		Security:           untrustedSecurityPolicy(opts),
		Sandbox:            ptrSandboxConfig(untrustedSandboxConfig(opts)),
	})
}

func execUntrustedWorker(script string, data map[string]interface{}, opts UntrustedExecOptions) (Object, error) {
	command, args, err := untrustedWorkerCommand(opts)
	if err != nil {
		return nil, err
	}
	req := untrustedWorkerRequest{Script: script, Data: data, Options: opts}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, &ExecError{Kind: ExecErrorValidation, Message: err.Error()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout+time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Env = workerEnvironment()
	cmd.Dir = opts.ModuleDir
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	var stdout, stderr limitWriter
	stdout.limit = opts.MaxOutputBytes
	stderr.limit = opts.MaxOutputBytes
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, &ExecError{Kind: ExecErrorRuntime, Message: fmt.Sprintf("start sandbox worker: %v", err)}
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		killProcessGroup(cmd.Process)
		err = <-done
		return nil, &ExecError{Kind: ExecErrorRuntime, Message: "sandbox worker timed out"}
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, &ExecError{Kind: ExecErrorRuntime, Message: msg}
	}
	var resp untrustedWorkerResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, &ExecError{Kind: ExecErrorRuntime, Message: fmt.Sprintf("invalid sandbox worker response: %v", err)}
	}
	if resp.Error != "" {
		kind := resp.ErrorKind
		if kind == "" {
			kind = ExecErrorRuntime
		}
		return nil, &ExecError{Kind: kind, Message: resp.Error}
	}
	return toObject(resp.Result), nil
}

func untrustedWorkerCommand(opts UntrustedExecOptions) (string, []string, error) {
	command := append([]string(nil), opts.WorkerCommand...)
	if len(command) == 0 {
		self, err := os.Executable()
		if err != nil {
			return "", nil, &ExecError{Kind: ExecErrorRuntime, Message: fmt.Sprintf("resolve worker executable: %v", err)}
		}
		command = []string{self, "--spl-worker"}
	}
	if !opts.RequireOSIsolation {
		return command[0], command[1:], nil
	}
	if runtime.GOOS != "linux" {
		return "", nil, &ExecError{Kind: ExecErrorRuntime, Message: "OS isolation is only implemented on linux"}
	}
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		return "", nil, &ExecError{Kind: ExecErrorRuntime, Message: "OS isolation requested but bubblewrap (bwrap) was not found"}
	}
	absModuleDir, err := filepath.Abs(opts.ModuleDir)
	if err != nil {
		return "", nil, &ExecError{Kind: ExecErrorValidation, Message: err.Error()}
	}
	args := []string{
		"--die-with-parent",
		"--unshare-net",
		"--new-session",
		"--proc", "/proc",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
		"--ro-bind", absModuleDir, absModuleDir,
		"--chdir", absModuleDir,
	}
	args = append(args, command...)
	return bwrap, args, nil
}

func workerEnvironment() []string {
	allow := map[string]bool{
		"PATH":   true,
		"HOME":   true,
		"TMPDIR": true,
		"TZ":     true,
	}
	env := make([]string, 0, len(allow))
	for _, pair := range os.Environ() {
		key, _, _ := strings.Cut(pair, "=")
		if allow[key] {
			env = append(env, pair)
		}
	}
	return env
}

func killProcessGroup(p *os.Process) {
	if p == nil {
		return
	}
	if runtime.GOOS != "windows" {
		_ = syscall.Kill(-p.Pid, syscall.SIGKILL)
		return
	}
	_ = p.Kill()
}

func ptrSandboxConfig(cfg SandboxConfig) *SandboxConfig {
	return &cfg
}

type limitWriter struct {
	buf   bytes.Buffer
	limit int64
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if w == nil {
		return len(p), nil
	}
	if w.limit <= 0 {
		return w.buf.Write(p)
	}
	if int64(w.buf.Len()) >= w.limit {
		return len(p), nil
	}
	remaining := int(w.limit - int64(w.buf.Len()))
	if remaining < len(p) {
		_, _ = w.buf.Write(p[:remaining])
		return len(p), nil
	}
	return w.buf.Write(p)
}

func (w *limitWriter) Bytes() []byte {
	if w == nil {
		return nil
	}
	return w.buf.Bytes()
}

func (w *limitWriter) String() string {
	if w == nil {
		return ""
	}
	return w.buf.String()
}

var _ io.Writer = (*limitWriter)(nil)

func RunUntrustedWorker(r io.Reader, w io.Writer) int {
	var req untrustedWorkerRequest
	if err := json.NewDecoder(r).Decode(&req); err != nil {
		_ = json.NewEncoder(w).Encode(untrustedWorkerResponse{Error: err.Error(), ErrorKind: ExecErrorValidation})
		return 1
	}
	opts := normalizeUntrustedOptions(req.Options)
	opts.InProcess = true
	obj, err := execUntrustedInProcess(req.Script, req.Data, opts)
	resp := untrustedWorkerResponse{}
	if err != nil {
		resp.Error = err.Error()
		if execErr, ok := err.(*ExecError); ok {
			resp.ErrorKind = execErr.Kind
		} else {
			resp.ErrorKind = ExecErrorRuntime
		}
	} else if obj != nil {
		resp.Result = objectToJSONValue(obj)
		resp.ResultInspect = obj.Inspect()
		resp.ResultType = obj.Type().String()
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		return 1
	}
	return 0
}

func objectToJSONValue(obj Object) interface{} {
	switch v := obj.(type) {
	case *Integer:
		return v.Value
	case *Float:
		return v.Value
	case *Boolean:
		return v.Value
	case *String:
		return v.Value
	case *Null:
		return nil
	case *Array:
		out := make([]interface{}, len(v.Elements))
		for i, el := range v.Elements {
			out[i] = objectToJSONValue(el)
		}
		return out
	case *Hash:
		out := make(map[string]interface{}, len(v.Pairs))
		for _, pair := range v.Pairs {
			out[pair.Key.Inspect()] = objectToJSONValue(pair.Value)
		}
		return out
	default:
		return obj.Inspect()
	}
}
