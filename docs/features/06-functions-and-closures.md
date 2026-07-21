# 06 — Functions & Closures

Source: `pkg/parser/parser.go` (`parseFunctionLiteral`, `parseFunctionDeclaration`,
arrow-function parsing), `pkg/object/object.go` (`Function`).

## Named function declarations

```spl
function fact(n) {
    if (n <= 1) { return 1; }
    return n * fact(n - 1);
}
print fact(5); // 120
```

A named function declaration is sugar for `let fact = function fact(n) {...};`
— the name is available for recursive self-reference either way.

## Anonymous functions and closures

```spl
let makeAdder = function(x) {
    return function(y) { return x + y; };
};
let add10 = makeAdder(10);
print add10(5); // 15
```

Functions capture their defining environment by reference. Each call to
`makeAdder` creates a fresh closure over its own `x`.

## Arrow functions

```spl
let square = x => x * x;              // single param, no parens needed
let add = (a, b) => a + b;            // multiple params
let blockFn = (x) => {                // block body needs explicit return
    let y = x * 2;
    return y + 1;
};
print square(5); // 25
print add(2, 3);  // 5
print blockFn(5); // 11
```

An arrow function with an expression body implicitly returns that
expression's value; a block body (`{ ... }`) requires an explicit `return`.

## Default parameters

```spl
function greet(name, greeting = "Hello") {
    return greeting + ", " + name;
}
print greet("Alice");        // Hello, Alice
print greet("Bob", "Hi");    // Hi, Bob
```

Default expressions are evaluated at call time when the argument is omitted
(or explicitly `null`), in the function's own scope.

## Rest parameters

```spl
function sum(...nums) {
    return nums.reduce((a, b) => a + b, 0);
}
print sum(1, 2, 3, 4); // 10
```

`...name` collects any trailing positional arguments into an array. Combine
with spread at the call site: `sum(...someArray)`.

## Parameter destructuring

Function parameters accept the same object/array destructuring patterns as
`let` (doc 03):

```spl
function describe({name, age = 0}) {
    return name + " is " + age;
}
```

## Optional type annotations

```spl
function sumTyped(values: Array<int>): int {
    let total: int = 0;
    for (value in values) { total += value; }
    return total;
}
```

Types are checked/used where the interpreter supports it but are primarily
documentation and tooling metadata; see doc 17.

## `async` functions

```spl
async function fetchValue() { return 42; }
let asyncSquare = async (x) => x * x;
```

`async function`/`async (...) => ...` return a `Future`, resolved via
`await`. See doc 12 for the full concurrency model (channels, `go`, `await_all`).

## Higher-order use with array methods

```spl
let doubled = [1, 2, 3].map(x => x * 2);
let evens = [1, 2, 3, 4].filter(x => x % 2 == 0);
let total = [1, 2, 3].reduce((a, b) => a + b, 0);
```

See doc 16 for the full array method reference.
