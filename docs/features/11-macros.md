# 11 — Macros

Source: `pkg/parser/parser.go` (`parseMacroDefinition`), `pkg/eval/macros.go`,
`pkg/object/object.go` (`Macro`).

Macros are **AST-substitution** constructs: unlike functions, a macro's body
is spliced into the call site at the AST level (with hygienic renaming of
any internal local variables), rather than being called as a closure over
its own values.

## Defining a macro

```spl
macro when(condition, body) {
    if (condition) { body; }
}
```

## Calling a macro (trailing-block call form)

Macros are typically invoked with a trailing `{ ... }` block that becomes
the macro's `body` parameter:

```spl
let macroValue = 10;
when(macroValue > 5) { print "macro condition matched"; }
```

```spl
macro repeat(n, body) {
    let i = 0;
    while (i < n) {
        body;
        i += 1;
    }
}
repeat(2) { print "macro repeat"; }
```

## Hygienic local bindings

A macro's own internal `let` bindings (like `temp` in `swap` or `i` in
`repeat` above) are renamed at expansion time so they never collide with —
or leak into — the caller's scope:

```spl
macro swap(a, b) {
    let temp = a;
    a = b;
    b = temp;
}
let macroLeft = "left";
let macroRight = "right";
swap(macroLeft, macroRight);
print [macroLeft, macroRight]; // ["right", "left"]
print typeof temp;             // ERROR: identifier not found: temp
```

`temp` is not visible outside `swap`'s expansion, even though the macro body
was spliced directly into the call site — this is the "hygienic" part of
macro hygiene.

## When to reach for a macro vs. a function

- Use a **function** for ordinary reusable logic — it's a closure, has its
  own call semantics, and values are always evaluated before the call.
- Use a **macro** when you need to defer/repeat evaluation of a whole block
  (as `when`/`repeat` do above) or need lvalue-style parameters that get
  assigned to (as `swap` does, which no ordinary function could do since
  function arguments are passed by value).

Macros are a niche, advanced feature — most SPL scripts never need one; the
same effects can usually be achieved with functions/closures and control
flow.
