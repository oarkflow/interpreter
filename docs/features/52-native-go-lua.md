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

## Go embedding

Blank-import the package to register the SPL module and embedded-language tag:

```go
import _ "github.com/oarkflow/interpreter/plugins/lua"
```

For direct use, create a state with `lua.NewState`, then use `Load`, `Call`, or
`DoString`. `CallInto` avoids allocating a result slice, and `CallNumber2` is a
specialized allocation-free path for numeric functions with two arguments.

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
GOMAXPROCS=1 go test ./lua -run '^$' \
  -bench 'Benchmark(BinaryTrees12WithCapturedOutput|FannkuchRedux8|SpectralNorm150|NBody20000|GoCallsLuaInto)$' \
  -benchmem -count=15
```

The VM uses compact values, register bytecode, iterative frames, dense-array
table paths, shape caches, a state-local table arena, typed native math calls,
and compiler-generated straight-line numeric formulas. Keep published
cross-runtime claims tied to results collected on the same host, Go version,
inputs, output oracle, warm-up, `GOGC`, and `GOMAXPROCS` settings.
