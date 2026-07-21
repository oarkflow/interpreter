# 08 — Error Handling

Source: `pkg/parser/parser.go` (`parseThrowStatement`, `parseTryCatchExpression`),
`pkg/builtins/core.go` (`Error` builtin), `pkg/object/object.go` (`Error`).

## `throw` / `try` / `catch`

`try/catch` is an **expression** — it evaluates to whichever branch ran:

```spl
let result = try {
    throw "boom";
} catch (e) {
    "caught: " + e;
};
print result; // caught: boom
```

- `throw expr;` raises any value (string, hash, structured `Error`, ...).
- `catch (e) { ... }` binds the thrown value to `e`.
- The `catch` clause is optional if a `finally` block is present.

## `finally`

Runs unconditionally after the `try`/`catch`, whether or not an exception
was thrown:

```spl
let r = try {
    1 / 0;
} catch (e) {
    "div error: " + e;
} finally {
    print "always runs";
};
print r; // div error: division by zero
```

## Structured errors: the `Error(...)` builtin

```spl
let r = try {
    throw Error("bad input", {"code": "E_BAD"});
} catch (e: Error) {
    e;
};
print r;
// {code: "E_RUNTIME", message: "[E_BAD] bad input", name: "Error", stack: ""}
```

`Error(message[, details])` builds a structured hash (`name`, `message`,
`code`, `stack`) rather than a bare string, so calling code can branch on
`e.code` instead of parsing message text. `catch (e: Error)` is a typed
catch clause — the parser converts the thrown value into this structured
shape when the catch variable declares type `Error`.

## Runtime errors

Uncaught runtime errors (missing identifiers, type mismatches, division by
zero, security policy denials, resource-limit violations, ...) propagate as
the same underlying `Error` object and are catchable with a plain
`catch (e) { ... }`. At the top level (script/REPL), an uncaught error is
printed with a call stack and the process/REPL reports it as a runtime
failure. From the embedding API, uncaught script errors surface as an
`*interpreter.ExecError` with a `Kind` (see doc 42) distinguishing plain
runtime bugs from security denials, resource limits, timeouts, and
cancellations.

## Testing that an error was thrown

```spl
function risky() { throw "nope"; }
let threw = assert_throws(function() { risky(); });
print threw; // true
```

See doc 28 for the full testing-builtin reference (`assert_true`,
`assert_eq`, `assert_neq`, `assert_contains`, `assert_throws`, `test_summary`).

## Interaction with permissions/sandboxing

Under a restrictive security policy (e.g. `permissions({"strict": true})` or
the `untrusted` execution profile), a denied operation raises a catchable
runtime error rather than crashing the whole script:

```spl
permissions({"strict": true});
let denied = try {
    exec("date", 1000);
} catch (e) {
    e;
};
print denied; // policy-denial message
permissions({"strict": false});
```

See doc 44 for the full security-policy model.
