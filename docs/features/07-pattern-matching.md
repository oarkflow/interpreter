# 07 — Pattern Matching (`match`)

Source: `pkg/parser/parser.go:1286-1610` (parsing), `pkg/ast/ast.go:986-1168`,
`pkg/eval/match.go` (evaluation).

`match` is both a statement and an expression:

```spl
match (value) {
    case pattern [if guard] => body
    ...
}

let result = match (value) {
    case pattern => expr
    ...
};
```

A case body may be a single expression/statement or a `{ block }`. All 11
pattern kinds below can be freely nested and combined.

## 1. Literal patterns

```spl
match (x) {
    case 42 => "the answer"
    case "go" => "golang"
    case true => "yes"
    case null => "nothing"
}
```

## 2. Wildcard

```spl
case _ => "anything"
```

## 3. Variable binding

```spl
case n => "got " + n   // binds the whole matched value to n
```

## 4. Typed binding

```spl
case n: integer => "int " + n
case s: string  => "str " + s
case b: boolean => "bool"
case a: array   => "array of " + len(a)
case h: hash    => "hash"
case _: float   => "float"
```

Matches only if the value's runtime type matches, and binds it to the name.

## 5. Array destructuring

```spl
match ([1, 2, 3]) {
    case [a, b, c] => a + b + c
    case [head, ...tail] => "head=" + head
}
```

## 6. Object destructuring

```spl
match (event) {
    case {type: "click", target: t} => "clicked " + t
    case {name, age} => name + " is " + age   // shorthand bind
    case {a, ...rest} => rest                  // rest collects remaining keys
}
```

## 7. Guard clauses

```spl
case x if x > 0 => "positive"
```

## 8. OR patterns

```spl
case 1 | 2 | 3 => "small"
```

## 9. Range patterns

```spl
let grade = score => match (score) {
    case 90..100 => "A"
    case 80..89  => "B"
    case _       => "F"
};
```

## 10. Comparison patterns

```spl
match (15) {
    case > 100 => "huge"
    case > 10  => "big"
    case > 0   => "small"
    case _     => "non-positive"
}
```

`>`, `>=`, `<`, `<=`, `!=` are all supported as comparison-pattern operators.

## 11. Extractor patterns

```spl
case Some(x)   => "got " + x      // matches any non-null value, binds inner
case None      => "nothing"       // matches null
case All(p1,p2)=> ...             // every sub-pattern must match; bindings accumulate
case Any(p1,p2)=> ...             // first matching sub-pattern wins
case Tuple(p1,p2) => ...          // fixed-arity array match
case Regex("^[0-9]+$") => "numeric" // string regex test
```

```spl
let r10 = match ("Alice") {
    case Some(s: string) => "Hello, " + s
    case None => "no name"
};
print r10; // Hello, Alice

let r11 = match (null) {
    case Some(x) => "got " + x
    case None => "nothing"
};
print r11; // nothing
```

## Constructor patterns (algebraic data types)

Values built from a `type X = A(...) | B(...)` declaration (doc 10) are
matched with their constructor name:

```spl
type Result = Ok(value) | Err(error);
let outcome = Ok(42);
let msg = match (outcome) {
    case Ok(v)  => "ok: " + v
    case Err(e) => "err: " + e
};
```

### Exhaustiveness checking

If the matched value is an ADT value, the interpreter requires every
variant to be covered (by a constructor pattern, an OR-pattern of
constructors, a wildcard, or a plain binding pattern) — otherwise it's a
runtime error: `non-exhaustive ADT match for <Type>: missing <Variant>`.
`spltool check`'s static analysis also flags non-exhaustive matches at
edit time (doc 41).

## Nested patterns

Any of the above compose recursively:

```spl
case {users: [first, ...rest]} => first
case Ok({status: 200, body: b}) => b
```

## `match` as a statement vs. an expression

```spl
match (event) {                 // statement form: runs a matching case's side effects
    case {type: "click"} => print "clicked";
    case _ => print "ignored";
}

let label = match (n) {         // expression form: yields the case body's value
    case 0 => "zero"
    case _ => "nonzero"
};
```

If no case matches and there is no wildcard, evaluating a `match` expression
is a runtime error.
