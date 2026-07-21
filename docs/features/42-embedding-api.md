# 42 — Embedding API (Go)

Source: `interpreter.go`. This is the root Go package's public API for
running SPL from a host Go program. Verified end-to-end with a standalone
Go module in this doc.

## Quick verification

```go
package main

import (
    "fmt"
    "github.com/oarkflow/interpreter"
)

func main() {
    result, err := interpreter.Exec("let x = 40; let y = 2; x + y;", nil)
    fmt.Println(result, err) // &{42} <nil>
}
```

## `Exec` / `ExecFile`

```go
result, err := interpreter.Exec("let x = 40; let y = 2; x + y;", nil)
result, err := interpreter.ExecFile("script.spl", nil)
```

Both accept a `data map[string]interface{}` injected into the script's
global scope as variables (Go primitives/slices/maps/structs are converted
to SPL objects via reflection). `Exec`/`ExecFile` are trivial wrappers over
the `*WithOptions` variants below with a zero `ExecOptions{}`.

## `ExecWithOptions` / `ExecFileWithOptions`

```go
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()

result, err := interpreter.ExecWithOptions(
    "let x = 40; let y = 2; x + y;", nil,
    interpreter.ExecOptions{
        Context:  ctx,
        MaxSteps: 1_000_000,
        MaxDepth: 200,
        MaxHeapMB: 128,
    },
)
```

### `ExecOptions` fields

```go
type ExecOptions struct {
    Args                   []string
    ModuleDir              string
    Profile                string // "", "trusted", "untrusted", "readonly",
                                   // "networked", "data-processing", "automation", "server"
    WorkerCommand          []string
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
    Output                 io.Writer
    Security               *SecurityPolicy
    Sandbox                *SandboxConfig
    Observability          *ObservabilityHooks
    RequireOSIsolation     bool
    AllowInProcessFallback bool
}
```

Every numeric field must be `>= 0` or the call fails validation
(`ExecErrorValidation`); a `0` value means "leave the sandbox/preset
default in place," not "unlimited." Setting `Profile` to anything other
than `""`/`"trusted"` routes execution through the **untrusted worker
subprocess path** (doc 45) instead of running trusted in-process.

### `ExecError` and `ExecErrorKind`

```go
var execErr *interpreter.ExecError
if errors.As(err, &execErr) {
    fmt.Println(execErr.Kind, execErr.Message)
}
```

```go
const (
    ExecErrorIO             ExecErrorKind = "io"
    ExecErrorParser         ExecErrorKind = "parser"
    ExecErrorRuntime        ExecErrorKind = "runtime"
    ExecErrorValidation     ExecErrorKind = "validation"
    ExecErrorPolicyDenied   ExecErrorKind = "policy_denied"
    ExecErrorResourceLimit  ExecErrorKind = "resource_limit"
    ExecErrorTimeout        ExecErrorKind = "timeout"
    ExecErrorCancelled      ExecErrorKind = "cancelled"
)
```

`ExecError` also carries `Path`, `Diagnostics`/`StructuredDiagnostics`
(parser-friendly, JSON-taggable), `Stack []CallFrame`, `ModuleChain`, and
source location fields — its `Error()` renders paths relative to `BaseDir`
so absolute disk paths never leak into error text.

```go
result, err := interpreter.ExecWithOptions(script, nil, interpreter.ExecOptions{
    Profile:  "untrusted",
    Security: &interpreter.SecurityPolicy{AllowedCapabilities: []string{"filesystem_read"}},
})
var execErr *interpreter.ExecError
if errors.As(err, &execErr) && execErr.Kind == interpreter.ExecErrorPolicyDenied {
    log.Printf("tenant script denied: %s", execErr.Message)
}
```

## `Runtime` — for hosts executing many scripts

```go
rt, err := interpreter.NewRuntime(interpreter.RuntimeOptions{
    Profile:   "readonly",
    ModuleDir: "./scripts",
    MaxSteps:  500_000,
    Observability: &interpreter.ObservabilityHooks{
        OnFinish: func(m interpreter.ExecutionMetrics) {
            log.Printf("script=%s profile=%s duration=%s err=%s", m.Path, m.Profile, m.Duration, m.Error)
        },
    },
})
result, err := rt.ExecFile("scripts/job.spl", nil)
```

`RuntimeOptions` mirrors `ExecOptions` plus `Plugins []Plugin`. If
`Security`/`Sandbox` aren't set, `NewRuntime` derives them from
`CapabilityPreset(Profile, ModuleDir)` — so passing only `Profile` is
enough to get a full preset's capabilities (doc 45).

**Verified** (`rt.NewSession`):

```go
rt, _ := interpreter.NewRuntime(interpreter.RuntimeOptions{Profile: "readonly", AllowInProcessFallback: true})
sess, _ := rt.NewSession(interpreter.SessionOptions{ID: "workspace"})
res := sess.Execute(interpreter.ExecutionRequest{Source: `let x = 40; x + 2;`})
fmt.Println(res.ResultText) // "42"
snap, _ := sess.Checkpoint("baseline")
```

`Runtime.NewSession` back-fills any zero-valued `SessionOptions` fields
from the runtime's own configuration — see doc 40 for the full session
API (checkpoints, replay, debug, cancellation).

### `ObservabilityHooks`

```go
type ObservabilityHooks struct {
    OnStart        func(ExecutionEvent)
    OnFinish       func(ExecutionMetrics)
    OnPolicyDenied func(category, detail string)
}
```

`OnPolicyDenied` is installed as a **process-wide** hook when set through
`NewRuntime` — last writer wins across concurrently constructed
`Runtime`s; there is no per-Runtime scoping for it.

## Cancelling an in-flight execution

```go
sess, _ := rt.NewSession(interpreter.SessionOptions{ID: "long-job", Timeout: 30 * time.Second})
go func() {
    res := sess.Execute(interpreter.ExecutionRequest{Source: `while (true) { 1 + 1; }`})
    fmt.Println(res.Metrics.ErrorKind) // session.ErrorKindCancelled
}()
time.Sleep(500 * time.Millisecond)
sess.Cancel() // stops the in-flight execution above
```

## Performance: pooled environments

```go
env := interpreter.NewPooledEnvironment()
defer interpreter.ReleasePooledEnvironment(env)
result := interpreter.Eval(program, env)
```

For prepared, expression-only workloads whose bindings never change after
setup, seal the environment for lock-free reads and native short-circuit
rule evaluation:

```go
env := interpreter.NewEnvironment()
interpreter.InjectData(env, data)
env.SealBindings()
result := interpreter.Eval(program, env)
```

Do **not** seal an environment used by scripts that declare or assign
variables — `Set`/`SetConst`/`Assign` panic on a sealed environment; call
`env.Reset()` (which also unseals) to make a pooled environment writable
again. See doc 44 for the rest of the runtime/plugin/performance API
(`RegisterRuntimeBuiltins`, `RegisterStdModule`, capability presets).
