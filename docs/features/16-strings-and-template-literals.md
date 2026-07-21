# 16 — Strings & Template Literals

Source: `pkg/eval/properties.go` (`getStringMethod`), `pkg/builtins/strings.go`,
`pkg/builtins/enhancements.go`.

String literal forms (single/double/triple-quoted, heredoc, backtick
templates) are covered in doc 02. This document covers the string **method**
API reached via dot syntax.

## Case & whitespace

```spl
"  Hello  ".trim();              // "Hello"
"Hello".upper();                 // "HELLO"  (alias: .toUpperCase())
"Hello".lower();                 // "hello"  (alias: .toLowerCase())
"hello world".title();           // "Hello World"
"Hello".swap_case();             // "hELLO"
```

## Case-style conversions

```spl
"HelloWorld".snake_case();  // "hello_world"
"hello_world".camel_case(); // "helloWorld"
"hello_world".pascal_case();// "HelloWorld"
"hello_world".kebab_case(); // "hello-world"
"Hello World!".slug();      // "hello-world"
```

## Searching & testing

```spl
"Hello".starts_with("He");   // true  (alias: .startsWith())
"Hello".ends_with("lo");     // true  (alias: .endsWith())
"Hello".includes("ll");      // true
"Hello".index_of("l");       // 2     (alias: .indexOf())
"aXaXa".count_substr("X");   // 2
```

## Transforming

```spl
"Hello".replace("l", "L");        // "HeLLo" (replaces all occurrences)
"ab".repeat(3);                    // "ababab"
"Hello World".substring(0, 5);     // "Hello" (rune-based)
"Hello World".truncate(5);         // "Hello..." (default suffix "...")
"prefix-value".trim_prefix("prefix-"); // "value"
"value-suffix".trim_suffix("-suffix"); // "value"
"5".pad_left(3, "0");   // "005"  (alias: .padStart())
"5".pad_right(3, "0");  // "500"  (alias: .padEnd())
```

## Splitting

```spl
"a,b,c".split(",");        // ["a","b","c"]
"a\nb\nc".split_lines();   // ["a","b","c"]
```

## Indexing

```spl
"Hello".charAt(1);  // "e"
"Hello".at(-1);      // "o" (negative index counts from the end)
```

## Regex helpers

```spl
"test123".regex_match("[0-9]+");           // true
"test123".regex_replace("[0-9]+", "#");    // "test#"
```

Additional regex builtins (`regex_find_all`, `regex_split`) are available as
free functions (doc 22).

## Template (backtick) literals

```spl
let name = "SPL";
print `Hello ${name}, ${1 + 1}`; // Hello SPL, 2
```

`${...}` interpolates any expression; escape a literal `` ` `` with `` \` ``
and a literal `$` with `\$`. Template literals are parsed into alternating
literal/expression parts at parse time (each `${...}` span is re-lexed and
re-parsed as its own sub-expression).

## `sprintf`/`printf`/`interpolate`

For printf-style formatting and `{placeholder}`-style templating (as
opposed to `${expr}` interpolation), see doc 25.
