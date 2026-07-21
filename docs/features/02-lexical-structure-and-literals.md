# 02 — Lexical Structure & Literals

Source: `pkg/lexer`, `pkg/token`.

## Comments

```spl
// line comment
# also a line comment
/* block comment,
   supports /* nesting */ too */
```

## Numeric literals

| Form | Example | Notes |
|---|---|---|
| Decimal integer | `42` | |
| Decimal float | `3.14` | |
| Digit separators | `1_000_000`, `3.14_15` | underscores anywhere between digits |
| Hex | `0xFF`, `0Xff` | integer only |
| Binary | `0b1010`, `0B1010` | integer only |
| Octal | `0o755`, `0O755` | integer only |

```spl
print 0b1010;   // 10
print 0o755;    // 493
print 0xFF;     // 255
print 1_000_000;// 1000000
print 3.14_15;  // 3.1415
```

A number literal followed immediately by another `.` is *not* parsed greedily
as a float — this is what makes the range operator work: `1..5` lexes as
`1`, `..`, `5`, not `1.` followed by `.5`.

## String literal forms

| Form | Syntax | Escapes | Interpolation |
|---|---|---|---|
| Single-quoted | `'text'` | `\n \r \t \" \' \\` | no |
| Double-quoted | `"text"` | same | no |
| Triple-quoted | `'''text'''` / `"""text"""` | none (raw) | no |
| Heredoc | `<<MARKER ... MARKER` / `<<-MARKER ... MARKER` | none (raw) | no |
| Backtick template | `` `text ${expr}` `` | `` \` ``, `\$`, plus standard escapes | yes, via `${...}` |
| Triple-backtick tagged block | ``` ```code``` ``` or `` tag`code` `` | none (raw) | no (passed to a language handler) |

```spl
let a = 'single quoted';
let b = "double quoted";
let c = '''raw
multi-line, no escaping''';

let name = "SPL";
let greeting = `Hello ${name}, 1 + 1 = ${1 + 1}`;
print greeting; // Hello SPL, 1 + 1 = 2
```

Triple-backtick / tagged blocks (`` sql`SELECT 1` ``) are lexed as a single
`TaggedBlockLiteral{Tag, Code}` node and dispatched to a host-registered
**embedded language handler** — see the XQL doc (38) for a real example
(`` xql`...` ``). If no handler is registered for the tag, evaluating the
literal is an error.

## Keywords

```
let const function return if else while for do break continue
print import export try catch throw switch case default in
typeof match async await init new type lazy yield macro
for_await select spawn abstract extends super class interface
```

`and`, `or`, `not` are recognized case-insensitively as word-aliases for
`&&`, `||`, `!` (useful for rule-style expressions); see doc 04. `type` and
`lazy` lex as plain identifiers and are reinterpreted contextually by the
parser, so they remain usable as ordinary variable names outside their
special forms.

## Delimiters and operators

Delimiters: `( ) { } [ ] , ; : .`

The full operator set (arithmetic, comparison, logical, bitwise, assignment,
nullish, ternary, range, pipeline, spread/rest, channel send/receive, etc.)
is documented in doc 04.

## Whitespace and statement terminators

Statements are terminated with `;`. Whitespace (including newlines) is
otherwise insignificant to the grammar, though the REPL's multiline-input
detector (doc 40) uses indentation/bracket-balance heuristics to decide when
to prompt for a continuation line.
