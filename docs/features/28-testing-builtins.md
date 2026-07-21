# 28 — Testing Builtins

Source: `pkg/builtins/test.go`, `pkg/parser/parser.go` (`parseTestStatement`).

SPL has lightweight, built-in test assertions and a native `test { }` block
— no separate test framework/import required.

## Assertions

```spl
assert_true(1 == 1, "one equals one");
assert_eq(2 + 2, 4);
assert_neq(2 + 2, 5);
assert_contains([1, 2, 3], 2);
assert_contains("hello world", "world"); // string or array haystack
let threw = assert_throws(function() { throw "x"; });
print threw; // true
```

Every assertion increments an internal pass/fail counter and, on failure,
raises a descriptive error (optionally with a custom `message` argument).

## `test_summary()`

```spl
print test_summary();
// {failed: 0, passed: 6, total: 6}
```

Reports cumulative counts of assertions run so far in the current
script/session.

## `test "name" { ... }` blocks

```spl
test "basic math" {
    assert_eq(1 + 1, 2);
}
```

A native test statement — its body executes like an ordinary block, using
the same `assert_*` builtins; useful for grouping related assertions with a
readable label (surfaced by tooling: `spltool symbols`/`docs` list `test`
blocks alongside functions/types, doc 42).

## `run_tests(pathOrArrayOfPaths)`

```spl
run_tests("tests/math_test.spl");
run_tests(["tests/a_test.spl", "tests/b_test.spl"]);
```

Parses and evaluates one or more SPL files purely as test suites (each
file's `test { }` blocks and assertions run in sequence).

## `help([name])`

```spl
print help("assert_eq");
// assert_eq(actual, expected[, message]) fails test when values differ
print help(); // lists every builtin name known to the running binary
```

`help()` with no argument enumerates every registered builtin (core plus
any optional plugin packages linked into the running binary); `help(name)`
shows that builtin's one-line description.

## CLI test runners

For running whole test suites/directories from the command line rather than
inline in a script, see `spltool test` and `spltool conformance` in doc 42.
