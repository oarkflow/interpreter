# SPL Operator Audit

This audit tracks operator support across lexing, parsing, evaluation, bytecode
fallback, tests, and documentation.

| Area | Operators / Forms | Status | Notes |
| --- | --- | --- | --- |
| Arithmetic | `+`, `-`, `*`, `/`, `%`, unary `-` | Supported | Integers and floats; `+` also concatenates strings. |
| Exponentiation | `**`, `**=` | Supported | `**` is right-associative. |
| Comparison | `<`, `>`, `<=`, `>=`, `==`, `!=` | Supported | Numeric cross-type comparison is supported for integer/float pairs. |
| Logical | `&&`, `||`, `!`, `and`, `or`, `not` | Supported | Word aliases are case-insensitive and canonicalize to symbolic operators. `&&`/`||` short-circuit. |
| Membership | `in`, `not in` | Supported | Case-insensitive word operator. Arrays/ranges use typed equality with numeric cross-type matching; hashes check key existence; strings check substring membership. |
| Bitwise | `&`, `|`, `^`, `~`, `<<`, `>>` | Supported | Integer operands. |
| Compound assignment | `+=`, `-=`, `*=`, `/=`, `%=`, `&=`, `|=`, `^=`, `<<=`, `>>=`, `**=` | Supported | Uses the corresponding infix operation before assignment. |
| Logical assignment | `&&=`, `||=`, `??=` | Supported | Preserves short-circuit assignment behavior. |
| Nullish | `??`, `??=` | Supported | Returns/evaluates the right side only when the left side is `null`. |
| Ternary | `cond ? a : b` | Supported | Parses at assignment-level precedence. |
| Range | `a..b` | Supported | Produces integer or single-character string arrays, ascending or descending. |
| Pipeline | `value |> fn(...)` | Supported | Rewrites the left value into the first call argument. |
| Index / access | `arr[i]`, `hash[key]`, `obj.key`, `obj?.key` | Supported | Optional dot returns `null` when the left side is `null` or access fails. |
| Update | postfix `++`, postfix `--` | Supported | Works on assignable integer/float targets. |
| Type | `typeof expr` | Supported | Prefix operator returning SPL type names. |
| Spread/rest | `...expr`, `function(...args)` | Supported | Spread works in arrays, hashes, and calls where implemented; rest works in parameter/destructure forms. |
| Regex | `regex_match`, `regex_replace`, `Regex(...)` patterns | Builtin / pattern support | No dedicated regex infix operator; use builtins or match extractors. |
| Between | `x in low..high`, range patterns | Supported through range membership/patterns | No separate `between` keyword. |
| Pattern matching | `match`, `case`, `if` guards, `|` OR patterns, ranges, comparisons | Supported | Pattern syntax is separate from expression infix operators. |

## Newly Added

- Case-insensitive word logical aliases: `and`, `or`, `not`.
- Expression-level membership: `x in container`.
- Negated membership: `x not in container`.
- Rule-style expressions now work with injected data, for example:

```spl
amount > 100000 and department in ["finance", "procurement"] and risk_score >= 70
```

## Intentional Non-Operators

- Regex remains available through builtins and match extractors instead of an
  infix operator.
- Between checks are expressed as `x in low..high`.
- General SPL keywords remain case-sensitive; only operator words are
  case-insensitive.
