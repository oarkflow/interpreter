# 05 — Control Flow

Source: `pkg/parser/parser.go` (`parseIfExpression`, `parseWhileStatement`,
`parseForStatement`, `parseDoWhileStatement`, `parseSwitchStatement`).

## `if` / `else` (as an expression)

`if`/`else` is parsed as an **expression**, so it can be used as a value:

```spl
if (x > 0) {
    print "positive";
} else if (x < 0) {
    print "negative";
} else {
    print "zero";
}

let label = if (score >= 60) { "pass" } else { "fail" };
```

## `while`

```spl
let i = 0;
while (i < 5) {
    print i;
    i = i + 1;
}
```

## `do { } while ()`

```spl
let n = 0;
do {
    print n;
    n = n + 1;
} while (n < 3);
```

## C-style `for`

```spl
for (let i = 0; i < 5; i = i + 1) {
    print i;
}
```

## `for ... in ...`

Iterates arrays (value, or index+value), hashes (key+value), and strings
(index+character):

```spl
for (v in [10, 20, 30]) { print v; }
for (i, v in [10, 20, 30]) { print sprintf("%d:%d", i, v); }
for (k, v in {"a": 1, "b": 2}) { print sprintf("%s=%d", k, v); }
for (i, ch in "abc") { print sprintf("%d:%s", i, ch); }
```

## `break` / `continue`

Standard loop control, valid inside `while`, `do/while`, `for`, `for-in`.

```spl
for (let i = 0; i < 10; i = i + 1) {
    if (i == 3) { continue; }
    if (i == 6) { break; }
    print i;
}
```

## `switch` / `case`

```spl
switch (lang) {
    case "go": print "Go lang";
    case "spl": print "SPL lang";
    case "js", "ts": print "JavaScript family"; // multiple values per case
    default: print "unknown lang";
}
```

Cases do **not** fall through automatically — each case body runs to
completion (or `break`s out early) and the switch ends; there is no implicit
fallthrough to the next case as in C.

## `match` (pattern matching)

`match`/`case` is a much richer alternative to `switch` — literals, type
bindings, destructuring, guards, ranges, comparisons, OR-patterns, and
extractor patterns. It is documented in full in doc 07.

## Ternary as lightweight control flow

```spl
let status = (code == 200) ? "ok" : "error";
```

## Related: exception-based control flow

`throw`/`try`/`catch`/`finally` are documented in doc 08.
