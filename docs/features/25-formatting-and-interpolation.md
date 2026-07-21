# 25 — Formatting & Interpolation

Source: `pkg/builtins/core.go` (`sprintf`, `printf`, `interpolate`).

## `sprintf(format, ...args)`

Printf-style formatting, returning the formatted string:

```spl
let s = sprintf("name=%s n=%d ok=%t type=%T val=%v", "spl", 7, true, 3.14, {"a": 1});
print s;
// name=spl n=7 ok=true type=FLOAT val={a: 1}
```

- Supports common printf verbs: `%s`, `%d`, `%f`, `%t`, `%v`, and more.
- `%T` is an SPL-specific verb returning the argument's SPL type name (its
  `ObjectType`, e.g. `FLOAT`, `STRING`, `ARRAY` — see doc 19).
- `%v` renders any value with its default `Inspect()` representation.
- Mismatched argument counts return a clear `ERROR: ...` string rather than
  panicking.

## `printf(format, ...args)`

Same formatting as `sprintf`, but also writes the result to stdout (and
still returns the formatted string):

```spl
printf("user=%s age=%d\n", "alice", 30); // prints "user=alice age=30"
```

## `interpolate(template, data[, ...positional])`

Replaces `{key}` (named) or `{index}` (positional) placeholders:

```spl
print interpolate("Hello {name}, items={count}", {"name": "SPL", "count": 3});
// Hello SPL, items=3

print interpolate("{0} + {1} = {2}", null, 20, 22, 42);
// 20 + 22 = 42
```

For the positional form, pass `null` as the `data` argument and supply the
values as trailing positional arguments (`{0}` refers to the first trailing
argument, etc.).

Escaped braces:

```spl
print interpolate("literal {{brace}}", {}); // literal {brace}
```

`{{` and `}}` produce literal `{` and `}` rather than being treated as
placeholder delimiters.

## Choosing between `sprintf`/`interpolate` and template literals

- Use **`sprintf`** for printf-style, type-aware formatting (numeric width/
  precision, `%T` type introspection).
- Use **`interpolate`** for simple named/positional placeholder substitution
  against a data hash — handy for user-facing message templates loaded from
  config or translated strings.
- Use **backtick template literals** (`` `Hello ${name}` ``, doc 16) for
  inline string-building with full expression evaluation directly in
  source code.
