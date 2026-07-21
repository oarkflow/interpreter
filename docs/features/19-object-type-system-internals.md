# 19 — Object & Type System Internals

Source: `pkg/object/object.go`.

This is a reference to the runtime value types underlying everything in
docs 01–18 — useful when embedding SPL in Go (doc 42) or extending it with
custom builtins/plugins (doc 43).

## `ObjectType` enum

```
INTEGER FLOAT BOOLEAN STRING NULL ERROR RETURN_VALUE BREAK CONTINUE
FUNCTION BUILTIN ARRAY HASH DB DB_TX FUTURE INTERFACE ADT_TYPE ADT_VALUE
LAZY OWNED SECRET RENDER_ARTIFACT FILE_VALUE IMAGE_VALUE TABLE_VALUE MACRO
```

Plus extended IDs (100-109) used by optional plugin subsystems outside the
core `pkg/` tree: `SERVER REQUEST RESPONSE SSE_WRITER QUERY_BUILDER
LAZY_DB_QUERY SIGNAL COMPUTED EFFECT FILE_FINDER`.

Every value type implements `Type() ObjectType` and `Inspect() string` (used
by `print`, default formatting, and REPL output).

## Scalar types

| Go type | Notes |
|---|---|
| `Integer{Value int64}` | |
| `Float{Value float64}` | |
| `Boolean{Value bool}` | |
| `String{Value string}` | |
| `Null{}` | singleton `NULL` |
| `Secret{Value string}` | `Inspect()` always returns `"***"` — never leaks the value; unwrap with `secret_reveal()` (doc 46) |

## Collections

| Go type | Notes |
|---|---|
| `Array{Elements []Object}` | doc 15 |
| `Hash{Pairs map[HashKey]HashPair}` | keys must implement `Hashable` (Integer/Float/String/Boolean); doc 15 |

## Callables

| Go type | Notes |
|---|---|
| `Function{Name, Parameters, ParamTypes, Defaults, ReturnType, HasRest, Body, Env, IsAsync, IsGenerator}` | user-defined closures (doc 06) |
| `Builtin{Fn, Fn1, FnWithEnv, Env}` | native functions; three call shapes exist for zero-allocation fast paths depending on arity/whether env access is needed |
| `Macro{Name, Parameters, Body}` | AST-substitution macros (doc 11) |

## OOP / ADT types

| Go type | Notes |
|---|---|
| `ClassObject{Name, Parent, Methods, StaticMethods, Fields, InstanceFieldDefs, PrivateMembers, IsAbstract, Implements}` | doc 09 |
| `ClassInstance{Class, Fields, PrivateOwner}` | `Type()` reports `HASH_OBJ` so instances interoperate with hash-oriented code |
| `ADTTypeDef{TypeName, Variants, Order}` / `ADTValue{TypeName, VariantName, FieldNames, Values, AllVariants}` | doc 10 |
| `InterfaceLiteral{Methods}` | runtime metadata only, not enforced (doc 09) |

## Control-flow / error sentinels

| Go type | Notes |
|---|---|
| `Error{Message, Code, Path, Line, Column, Stack []CallFrame, ModuleChain, LeafMessage}` | thrown/runtime errors, with a call-stack trace (doc 08) |
| `ReturnValue`, `Break`, `Continue` | internal control-flow sentinels, not user-constructible |

## Concurrency types

| Go type | Notes |
|---|---|
| `Future{Ch chan Object}` | result of `async function`/`go(...)` (doc 12) |
| `Channel{Ch chan Object}` | `Type()` reports `BUILTIN_OBJ`; from `channel(...)` (doc 12) |
| `LazyValue{Env, Expr}` | from `lazy expr`, forced on demand |
| `GeneratorValue{Elements, Env, Body, Func}` / `Stream{Elements}` | both report `Type() == ARRAY_OBJ`; doc 12 |

## Ownership wrappers

| Go type | Notes |
|---|---|
| `OwnedValue{OwnerID, Inner}` | from `move(x)` (doc 18) |
| `ImmutableValue{Inner}` | from `immutable(x)` (doc 18) |

## Data-value types

| Go type | Notes |
|---|---|
| `RenderArtifact{Kind, Source, SourceTyp, MIME, Name, Alt, Width, Height, MaxBytes, Mode}` | from `file()`/`image()`/`render()` (doc 27) |
| `FileValue{Name, Path, MIME, Encoding, SourceType, Size, Data}` | doc 27 |
| `ImageValue{Name, Path, MIME, Format, SourceType, Width, Height, Data, Image}` | doc 27 |
| `TableValue{Name, Path, MIME, SourceType, Columns, Rows}` | doc 27 |

## Extending the type system: `DotExpressionHook`

Plugin subsystems (server/database/image/reactive objects, etc.) that live
outside `pkg/object` add their own dot-accessible methods via a
`DotExpressionHook` extension point rather than modifying the core type
switch — this is how `plugins/database`'s `QueryBuilder`,
`pkg/builtins/server`'s `Request`/`Response`, and `pkg/builtins/reactive`'s
`Signal`/`Computed` objects all support `.method()` call syntax without
being part of `pkg/object` itself. See doc 43 for the Go-level plugin API
(`RegisterRuntimeBuiltins`, `RegisterStdModule`).

## Per-type dot-method dispatch

Runtime dot-method resolution for the built-in scalar/collection types
(String, Integer, Float, Array, Hash) is centralized in
`pkg/eval/properties.go`'s `evalDotExpression` — see docs 15 and 16 for the
user-facing method tables it exposes.
