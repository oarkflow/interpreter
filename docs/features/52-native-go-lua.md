# Native Go Lua plugin

Source: `plugins/lua`. This optional plugin implements Lua 5.1 source parsing,
bytecode compilation, and execution in this repository. It does not link the C
Lua runtime, use cgo, or wrap a third-party Go Lua interpreter.

## SPL entry points

Import the virtual module when the plugin is linked:

```spl
import "lua" as lua;

let [answer, err] = lua.eval("6 * 7");
let [values, run_err] = lua.run("return 20, 22");
```

`lua.run(source[, globals])` executes a chunk. `lua.eval(expression[, globals])`
evaluates an expression. Both return `[result, error]`; multiple Lua results
become an SPL array.

`lua.load(source[, globals])` creates a persistent state and executes the
chunk once. A returned module table is used before globals when resolving a
method:

```spl
let [counter, load_err] = lua.load(`
  local n = 0
  return {
    next = function(step) n = n + step; return n end
  }
`);
let [first, call_err] = counter.call("next", 2);
```

Persistent script objects expose `call(name, ...args)`, `get(name)`, and
`set(name, value)`. Calls are serialized because each object reuses its Lua VM
stack and table arena.

Lua can also be embedded directly:

```spl
lua`
  local total = 0
  for i = 1, 100 do total = total + i end
  return total
`;
```

Single backtick blocks interpolate `${expr}` as host-language expressions
before the code reaches the Lua compiler, the same as any other SPL template
string. For Lua source containing a literal `$` or `${` — currency
formatting, or code that just happens to contain that character sequence —
use triple backtick instead; it reads as a raw, non-interpolated block:

```spl
lua```
  local price = "$" .. tostring(total)
  return price
```;
```

## Go embedding

Blank-import the package to register the SPL module and embedded-language tag:

```go
import _ "github.com/oarkflow/interpreter/plugins/lua"
```

For direct use, create a state with `lua.NewState`, then use `Load`, `Call`, or
`DoString`. `CallInto` avoids allocating a result slice, and `CallNumber2` is a
specialized allocation-free path for numeric functions with two arguments.

`plugins/lua/examples/main.go` is a runnable walkthrough of the Go API
(`Load`/`Call`/`CallInto`/`CallNumber2`, registering a Go callback with
`Native`/`NativeBinary`, a persistent `Script` via `LoadScript`) and the SPL
layer (`lua.eval`/`run`/`load`, single- and triple-backtick tagged blocks):

```sh
cd plugins && go run ./lua/examples
```

The implementation includes closures and upvalues, varargs and multiple
returns, numeric and generic loops, metatables, protected calls, function
environments, coroutines, dynamic chunks, filesystem modules, and the core
Lua 5.1 libraries. Filesystem functions (`loadfile`, `dofile`, and filesystem
`require`) execute with the host process's filesystem authority; do not expose
them to untrusted scripts without a host sandbox.

## Performance checks

Run the established-program and embedding benchmarks on a fixed CPU:

```sh
cd plugins
GOMAXPROCS=1 go test ./lua -pgo=lua/default.pgo -run '^$' \
  -bench 'Benchmark(BinaryTrees12WithCapturedOutput|FannkuchRedux8|SpectralNorm150|NBody20000|GoCallsLuaInto)$' \
  -benchmem -count=15
```

`plugins/lua/default.pgo` is a profile captured from the full benchmark suite
(`go test -bench .`) and is used for profile-guided inlining of the hot
interpreter loop. `go test` does not auto-detect a package-local
`default.pgo` the way `go build` of a main package does, so pass `-pgo`
explicitly when benchmarking this package directly; regenerate the profile
with `go test -pgo=off -bench . -cpuprofile=default.pgo .` after VM changes
that materially shift where time is spent. A host application that vendors
this VM should generate its own `default.pgo` from its own production
workload and place it next to its main package, per the standard Go PGO
workflow — the profile here is a development/benchmarking aid, not a
substitute for that.

The VM uses compact values, register bytecode, iterative frames, dense-array
table paths, shape caches, a state-local table arena, typed native math calls,
and compiler-generated straight-line numeric formulas. Keep published
cross-runtime claims tied to results collected on the same host, Go version,
inputs, output oracle, warm-up, `GOGC`, and `GOMAXPROCS` settings.
