# 43 — Runtime Plugin System & Capability Presets

Source: `presets_plugins.go`.

## The `Plugin` interface

```go
type Plugin interface {
    Name() string
    Register(*Runtime) error
}

type PluginFunc struct {
    PluginName string
    Fn         func(*Runtime) error
}
```

`RuntimeOptions.Plugins []Plugin` runs every plugin's `Register(rt)` once,
in order, during `NewRuntime` — a plugin error short-circuits construction.

## Registering builtins and virtual modules

**Verified end-to-end**:

```go
plugin := interpreter.PluginFunc{
    PluginName: "example",
    Fn: func(rt *interpreter.Runtime) error {
        interpreter.RegisterRuntimeBuiltins(map[string]*object.Builtin{
            "answer": {Fn: func(args ...object.Object) object.Object {
                return &object.Integer{Value: 42}
            }},
        })
        return interpreter.RegisterStdModule("std/example", map[string]interpreter.Object{
            "name": &interpreter.String{Value: "example"},
        })
    },
}
rt, _ := interpreter.NewRuntime(interpreter.RuntimeOptions{Plugins: []interpreter.Plugin{plugin}})

res, _ := rt.Exec(`answer() + 1;`, nil)          // 43
res2, _ := rt.Exec(`import "std/example" as ex; ex.name;`, nil) // "example"
```

```go
func RegisterRuntimeBuiltins(group map[string]*object.Builtin)
func RegisterStdModule(name string, exports map[string]Object) error
func RegisterStdBuiltinModule(name string, builtinNames ...string) error
func LookupStdModule(name string) (map[string]Object, bool)
```

- `RegisterRuntimeBuiltins` adds names to the process-wide builtin table
  (callable globally, no import needed).
- `RegisterStdModule(name, exports)` registers a fixed export map that
  `import "<name>" as x` resolves to.
- `RegisterStdBuiltinModule(name, builtinNames...)` instead registers a
  *list of already-registered builtin names* to expose as a module's
  exports — this is how the optional plugin packages (`database`,
  `images`, `integrations`, `tools/*`, `cryptoextra`, `securetoken`, `yaml`,
  `xql`, `naturaldate`, `wuid`, `money`, `phone`, `ip`, `shamir`,
  `metadata`) wire their std module aliases (see `optionalBuiltinModule` in
  `presets_plugins.go` for the exact list). If a named builtin isn't
  actually linked in and the module name is one of these known-optional
  ones, the export becomes a stub that raises: `"<name> is not linked into
  this interpreter; use cmd/interpreter (built with the full plugin set),
  import the optional Go package in your embedding host, or build a custom
  preset"` — verified in doc 32/34/38. Notably, `emailvalidator`'s builtins
  register directly via `eval.RegisterBuiltins` rather than through this
  stub mechanism, so they simply don't exist at all (rather than existing
  as an error-raising stub) under a binary that doesn't link
  `plugins/emailvalidator`.
- `LookupStdModule("builtins")` is special-cased to return every currently
  registered builtin as a pseudo-module.
- Around 25 standard-library modules are pre-registered at package `init()`
  under both a bare name and a `std/`-prefixed alias (`math`/`std/math`,
  `string`/`std/string`, `time`/`std/time`, `json`/`std/json`, `fs`/`std/fs`,
  `render`/`std/render`, `test`/`std/test`, `config`/`std/config`, `core`/
  `std/core`, and more), plus optional-only modules registered the same way
  once their Go package is linked.

## Embedded language handlers

```go
func RegisterEmbeddedLanguage(tag string, handler EmbeddedLanguageHandler) error
func EmbeddedLanguageTags() []string

type EmbeddedLanguageContext struct { Tag string; Code string; Env *object.Environment }
type EmbeddedLanguageHandler func(EmbeddedLanguageContext) object.Object
```

Registers a handler for a tagged block literal tag (`` tag`code` ``, doc
02). `plugins/xql` uses this to implement `` xql`...` `` (doc 38).

## Capability presets

```go
func CapabilityPreset(name string, moduleDir string) (*SecurityPolicy, SandboxConfig, error)
```

| Preset | Grants (on top of `untrusted` baseline where noted) |
|---|---|
| `trusted` | unrestricted — the default, preserves existing CLI/embedding behavior |
| `untrusted` / `readonly` | `StrictMode + ProtectHost`; only `filesystem_read` allowed, rooted at `moduleDir`; `MaxDepth=128, MaxSteps=500_000, MaxHeapMB=64, MaxOutputBytes/MaxHTTPBodyBytes/MaxExecOutputBytes=64KiB, Timeout=2s` |
| `networked` | `untrusted` + `network` |
| `data-processing` | `untrusted` + `db` |
| `automation` | `untrusted` + `exec, filesystem_write, system` |
| `server` | `untrusted` + `server, network` |

`NewRuntime`/`ExecOptions.Profile` use this to derive
`Security`/`Sandbox` defaults automatically when the caller doesn't supply
its own `Security`/`Sandbox` — see doc 45 for the full policy model this
feeds into.

## Native vs. JS/WASM build tags

- `repl_bridge_native.go` (`//go:build !js`) wires the full interactive
  REPL (lexer/parser/eval/sandbox/config/builtins, native command
  execution via `os/exec`).
- `repl_bridge_js.go` (`//go:build js`) reduces `initReplBridge()` to a
  no-op — under `GOOS=js` (browser/WASM embedding), the OS-dependent REPL
  bridge and native command execution aren't available/desired.
- `scheduler_types_native.go`/`scheduler_types_js.go` currently declare
  identical placeholder types on both tags (`Scheduler{}`,
  `ScheduledJob{}`) — reserved for a future native/JS scheduler split, not
  yet a real behavioral divergence.
- The untrusted worker subprocess path (`untrusted.go`) uses `os/exec`,
  `syscall`, and (on Linux) `bubblewrap` unconditionally, with no `js`-tag
  variant — a WASM/browser embedding would need
  `InProcess: true`/`AllowInProcessFallback: true` to avoid depending on
  subprocess spawning.

## Dependency isolation guarantee

Importing the bare root package (`github.com/oarkflow/interpreter`) pulls
in **zero third-party Go dependencies** beyond one small vendored utility
package — enforced by a `dependency_guard_test.go` check via `go list
-deps`. Everything that reaches a database driver, image codec, extra
crypto library, or network/SMTP/FTP client lives in a separate optional Go
package (`plugins/database`, `plugins/images`, `plugins/integrations`,
`plugins/pdf`, `plugins/crypto`, `plugins/yaml`, `plugins/emailvalidator`,
`plugins/phone`, `plugins/ip`, `plugins/money`, `plugins/naturaldate`,
`plugins/wuid`, `plugins/shamir`, `plugins/metadata`, ...) that an
embedding host opts into explicitly, or gets automatically via
`cmd/interpreter` (which blank-imports every one of them via
`plugins/builtins.go`).
