# 17 — Optional Typing & Structured Types

Source: `pkg/parser/parser.go` (type-annotation parsing on `let`/`const`/
function parameters/return types), `pkg/object/object.go` (`Function.ParamTypes`,
`ReturnType`).

SPL is dynamically typed at runtime, but the parser accepts a gradual/
optional type-annotation syntax useful for documentation, IDE tooling
(hover/completion, doc 41), and static checks (`spltool check`).

## Basic annotations

```spl
let x: int = 10;
const DEFAULT_LIMIT: int = 10;

function sumTyped(values: Array<int>): int {
    let total: int = 0;
    for (value in values) { total += value; }
    return total;
}
```

## Generic/collection types

```spl
let typedIdentifiers: Array<int> = [1, 2, 3];
let typedScores: Map<string, float> = {"alice": 98.5, "bob": 91.0};
```

## Nullable types

```spl
let optionalId: int? = null;
```

## Union types

```spl
let textOrNumber: string | int = "forty-two";
```

## Full example

```spl
const DEFAULT_LIMIT: int = 10;
let typedIdentifiers: Array<int> = [1, 2, 3];
let typedScores: Map<string, float> = {"alice": 98.5, "bob": 91.0};
let optionalId: int? = null;
let textOrNumber: string | int = "forty-two";

function sumTyped(values: Array<int>): int {
    let total: int = 0;
    for (value in values) { total += value; }
    return total;
}

print sumTyped(typedIdentifiers); // 6
print typedScores.alice;          // 98.5
print optionalId;                 // null
print textOrNumber;                // forty-two
```

## What type annotations do (and don't do)

- They are attached to the AST/objects (`Function.ParamTypes`, `ReturnType`)
  and surfaced by tooling: hover info, completion detail strings, and
  `spltool docs` output all display declared types.
- They are **not** a full static type-checker — SPL remains dynamically
  typed at the value level; annotations primarily document intent and
  drive editor/CLI tooling rather than rejecting mismatched calls outright.
- Combine with `typeof` (doc 04) and the runtime `is_*` predicate builtins
  (doc 20) for actual runtime type checks/guards.

## Relationship to `match` typed bindings

The same type vocabulary (`int`, `string`, `boolean`, `array`, `hash`,
`float`) is reused by `match`'s typed-binding pattern (doc 07:
`case n: integer => ...`), giving a consistent way to talk about types
across declarations, signatures, and pattern matches.
