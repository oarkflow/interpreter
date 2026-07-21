# 10 — Algebraic Data Types (`type ... = A(...) | B(...)`)

Source: `pkg/parser/parser.go` (`parseTypeDeclarationStatement`),
`pkg/object/object.go` (`ADTTypeDef`, `ADTValue`), `pkg/eval/match.go`
(`ensureExhaustiveMatchADT`).

## Declaring a type

```spl
type Result = Ok(value) | Err(error);
type Shape  = Circle(radius) | Rectangle(width, height) | Point();
```

Each variant (`Ok`, `Err`, `Circle`, ...) becomes a callable **constructor**
in scope. Calling it builds an `ADTValue` tagging which variant was used and
holding its field values:

```spl
let c = Circle(5);
print c; // Circle(5)

let outcome = Ok(42);
```

## Consuming ADT values with `match`

```spl
function area(s) {
    return match (s) {
        case Circle(r)        => 3.14159 * r * r
        case Rectangle(w, h)  => w * h
        case Point()          => 0
    };
}
print area(Circle(2));       // 12.56636
print area(Rectangle(3, 4)); // 12
print area(Point());         // 0
```

Constructor patterns bind each variant's fields by position
(`Circle(r)` binds `radius` to `r`).

## Exhaustiveness checking

`match` over an ADT value **requires every variant to be covered** — by a
constructor pattern, an OR-pattern of constructors, a wildcard (`_`), or a
plain binding pattern. Missing a variant is a runtime error:

```spl
type Shape = Circle(radius) | Rectangle(width, height) | Point();
let s = Rectangle(3, 4);
let area = match (s) {
    case Circle(r) => 3.14159 * r * r
};
// ERROR: non-exhaustive ADT match for Shape: missing Rectangle, Point
```

`spltool check` also flags non-exhaustive matches statically
(diagnostic code `match-exhaustiveness`, doc 41) so this can be caught
before running the script.

## Typical use: modeling success/failure without exceptions

```spl
type Result = Ok(value) | Err(error);

let safeDiv = function(a, b) {
    if (b == 0) { return Err("division by zero"); }
    return Ok(a / b);
};

let describe = function(r) {
    return match (r) {
        case Ok(v)  => "result: " + v
        case Err(e) => "error: " + e
    };
};

print describe(safeDiv(10, 2)); // result: 5
print describe(safeDiv(10, 0)); // error: division by zero
```

## Relationship to classes

ADTs and classes (doc 09) are separate, complementary features: classes
model mutable, method-bearing objects with inheritance; ADTs model closed
sets of tagged, immutable-shaped data variants meant to be consumed
exhaustively via `match`. Reach for an ADT when you want the compiler/tooling
to catch a missed case; reach for a class when you want shared behavior,
inheritance, or mutable state.
