# 04 — Operators Reference

Source: `pkg/parser/parser.go` (precedence table + parse functions),
`pkg/eval/infix.go`, `docs/OPERATOR_AUDIT.md`.

## Precedence (lowest to highest)

```
LOWEST < ASSIGN < NULLISH_COALESCE(??) < LOGICAL_OR < LOGICAL_AND
      < BIT_OR < BIT_XOR < BIT_AND < EQUALS/IN/NOT
      < LESSGREATER < BIT_SHIFT < SUM < RANGE(..) < PRODUCT
      < POWER(**, right-assoc) < PREFIX < CALL/INDEX/DOT < POSTFIX(++/--)
```

## Arithmetic

| Operator | Meaning |
|---|---|
| `+ - * / %` | standard; `+` also concatenates strings |
| unary `-` | negation |
| `**` | exponentiation, right-associative (`2 ** 3 ** 2 == 2 ** 9`) |

Integer `/` truncates toward zero when both operands are integers; mixed
int/float operations promote to float. Division by zero is a runtime error.

```spl
print 2 ** 3 ** 2; // 512
print 7 / 2;       // 3
print 7.0 / 2;     // 3.5
```

## Comparison

`== != < > <= >=` — numeric comparisons auto-promote int/float; strings
compare lexicographically.

## Logical

`&& || !`, plus case-insensitive word aliases `and`, `or`, `not` (useful for
rule-style expressions injected with external data):

```spl
amount > 100000 and department in ["finance", "procurement"] and risk_score >= 70
status not in ["closed", "archived"] or not reviewed
```

`&&`/`||` short-circuit; the word aliases canonicalize to the same
short-circuiting operators at parse time. Only operator *words* are
case-insensitive — language keywords (`let`, `if`, ...) remain case-sensitive.

## Membership

`a in b` / `a not in b`:
- Array: equality check against elements (numeric cross-type aware)
- Hash: key-existence check
- String: substring check
- Range (`a in low..high`): inclusion check

## Bitwise (integer operands only)

`& | ^ ~ << >>`

## Assignment & compound assignment

```
=
+= -= *= /= %= **=
&= |= ^= <<= >>=
&&= ||= ??=
```

`&&=`/`||=`/`??=` preserve short-circuit semantics (the right side is only
evaluated if needed).

```spl
let x = 5;
x **= 2;   // 25
let flags = 0;
flags |= 0b0010;
let cfg = null;
cfg ??= "default";
```

## Nullish coalescing

`??` returns the right operand only if the left is `null`:

```spl
let value = maybeNull ?? "fallback";
```

## Ternary

```spl
let label = (score >= 60) ? "pass" : "fail";
```

## Range

`a..b` produces an inclusive array, ascending or descending, for integers or
single-character strings:

```spl
print 1..5;      // [1,2,3,4,5]
print 5..1;      // [5,4,3,2,1]
print "a".."e";  // ["a","b","c","d","e"]
for (i in 1..5) { print i; }
```

## Pipeline

`value |> fn` calls `fn(value)`; `value |> fn(extra)` calls `fn(value, extra)`
(the left-hand value is prepended as the first argument):

```spl
let stageMap    = arr => arr.map(x => x * 2);
let stageFilter = arr => arr.filter(x => x > 5);
let stageReduce = arr => arr.reduce((a, b) => a + b, 0);
let result = [1, 2, 3, 4, 5] |> stageMap |> stageFilter |> stageReduce;
```

## Index / dot / optional-dot access

```spl
arr[0]
hash["key"]
obj.key
obj?.key       // short-circuits to null if obj is null (or the chain fails)
person?.address?.city
```

## Postfix increment/decrement

`++` / `--` on identifier, dot, or index targets:

```spl
let i = 0;
i++;
obj.count++;
arr[0]--;
```

## `typeof`

Prefix operator returning a type-name string:

```spl
print typeof 42;       // "integer"
print typeof "x";      // "string"
print typeof [1,2];    // "array"
```

## Spread / rest

```spl
function sum(...nums) { return nums.reduce((a,b)=>a+b, 0); }
let combined = [0, ...[1,2,3], 4]; // [0,1,2,3,4]
let merged = {...base, extra: 1};
```

## Channel operators

`ch <- value` sends; `ch <-` (postfix) receives. See doc 12.

## Not operators (by design)

- Regex matching is a builtin (`regex_match`, or the `Regex(...)` extractor
  pattern in `match`), not an infix operator.
- There is no dedicated `between` operator — express it as range membership
  (`x in 90..100`) or a `match` range pattern.
