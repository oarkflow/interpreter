# 03 — Variables, Constants & Destructuring

Source: `pkg/parser/parser.go` (`parseLetStatement`, `parseConstStatement`,
`parseDestructureLetStatement`), `pkg/ast/ast.go`.

## `let` and `const`

```spl
let x = 10;
const PI = 3.14159;
```

`const` bindings cannot be reassigned; attempting to assign to one is a
runtime error. Both `let` and `const` accept an optional type annotation
(doc 17):

```spl
let x: int = 10;
const LIMIT: int = 100;
```

## Tuple-style assignment

`let a, b = expr;` destructures an array-returning expression positionally.
This is the idiomatic way to consume SPL's common `[value, error]` tuple
return convention:

```spl
let a, b = [1, 2];       // a = 1, b = 2
import "database";
let db, err = db_connect("sqlite", ":memory:");
```

`const` supports the same array-positional form:

```spl
const [c1, c2] = [100, 200];
```

## Object (hash) destructuring

```spl
let {name: n, age: a = 30, ...rest} = {"name": "Alice", "city": "NYC"};
print n;    // Alice
print a;    // 30 (default, since "age" wasn't present)
print rest; // {city: "NYC"}
```

- `key: newName` renames a bound variable.
- `key = default` (or `key: newName = default`) supplies a default when the
  key is missing or `null`.
- `...rest` collects every remaining, not-yet-destructured key into a new hash.
- **Shorthand**: `let {pName, pAge} = hash;` binds `pName`/`pAge` directly
  from matching keys (no renaming needed).

## Array destructuring

```spl
let [head, ...tail] = [1, 2, 3, 4, 5];
print head; // 1
print tail; // [2, 3, 4, 5]
```

Array destructuring also supports positional defaults (`[a, b = 2]`) and can
be nested/combined with object destructuring patterns.

## Scoping

Every `{ ... }` block, function body, and `for`/`while` loop body introduces
a new lexical scope (`NewEnclosedEnvironment`) that closes over its parent.
Closures capture their defining environment by reference — see doc 06.

## Where destructuring also appears

The same destructuring syntax used in `let`/`const` is reused in:

- function parameters (doc 06)
- `match` patterns (doc 07)
- `for (k, v in hash)` loop bindings (doc 05)
