# SPL — A Practical Scripting Language for Go

SPL ("Simple Programming Language") is a dynamically-typed, C-family
scripting language with a tree-walking interpreter written in Go. It ships
as an interactive REPL, a script runner, a browser playground, a developer
CLI (formatting, static checks, testing, packaging, an LSP server), and an
embedding API for Go programs that want to run user-supplied logic safely.

Out of the box it gives you closures, classes, algebraic data types,
pattern matching, macros, async/await, channels, generators/streams,
optional type annotations, and a full sandboxing/security-policy system —
plus a very large builtin library: strings, collections, time, math,
crypto, JSON/CSV/YAML, files, process execution, an HTTP server with SSE,
a scheduler, reactive state, a database layer with a query builder,
HTTP/SMTP/FTP/SFTP integrations, PDF generation/editing, a secrets vault,
email/phone/IP validation, fixed-point money arithmetic, natural-language
date parsing, sortable ID generation, and CSV/JSON type inference.

This guide covers everything you need to **use** SPL: installing it,
learning the language, working through the builtin library, and following
complete, realistic workflows that combine multiple features. It does not
describe the project's internal source layout — see the in-repo feature
documentation set if you need that level of detail.

## Table of Contents

- [Getting Started](#getting-started)
- [Language Guide](#language-guide)
  - [Comments & Literals](#comments--literals)
  - [Variables, Constants & Destructuring](#variables-constants--destructuring)
  - [Operators](#operators)
  - [Control Flow](#control-flow)
  - [Functions & Closures](#functions--closures)
  - [Pattern Matching (`match`)](#pattern-matching-match)
  - [Error Handling](#error-handling)
  - [Classes & Interfaces](#classes--interfaces)
  - [Algebraic Data Types](#algebraic-data-types)
  - [Macros](#macros)
  - [Concurrency](#concurrency-asyncawait-channels-generators--streams)
  - [Modules: Imports & Exports](#modules-imports--exports)
  - [Package Manifests](#package-manifests)
  - [Arrays & Hashes](#arrays--hashes)
  - [Strings & Templates](#strings--template-literals)
  - [Optional Typing](#optional-typing)
  - [Ownership & Immutability Helpers](#ownership--immutability-helpers)
- [Standard Builtin Library](#standard-builtin-library)
- [Server & Runtime Features](#server--runtime-features)
- [Data, Secrets & Integration Plugins](#data-secrets--integration-plugins)
- [Daily-Ops & Data-Validation Plugins](#daily-ops--data-validation-plugins)
- [Config Loading & Secrets Masking](#config-loading--secrets-masking)
- [Common Workflows](#common-workflows)
- [Developer Tooling](#developer-tooling)
- [The REPL](#the-repl)
- [Embedding SPL in a Go Program](#embedding-spl-in-a-go-program)
- [Security & Sandboxed Execution](#security--sandboxed-execution)
- [The Browser Playground](#the-browser-playground)

---

## Getting Started

### Requirements

- Go 1.25 or newer (the full-featured `cmd/interpreter` build requires
  Go 1.26+)

### Building

The full-featured CLI/REPL/playground binary is its own Go module (so a
plain `go get` of the library never pulls in database drivers, image
codecs, or other optional third-party dependencies):

```bash
go -C cmd/interpreter build -o interpreter .
./interpreter                       # start the REPL
./interpreter script.spl            # run a script
./interpreter --playground          # start the browser playground
```

The developer CLI (`spltool` — formatting, static checks, packaging,
testing, an LSP server) is part of the lightweight core and can be run
directly:

```bash
go run ./cmd/spltool fmt script.spl
```

A version of `spltool` that also understands every optional plugin builtin
(useful for IDE completion/diagnostics on scripts that use them) is built
the same way as `cmd/interpreter`:

```bash
go -C cmd/spltool-full build -o spltool-full .
```

### Hello, world

```spl
print "Hello, world!";
```

```spl
let name = "SPL";
print sprintf("Hello, %s!", name);
```

### Starting the REPL

```bash
./interpreter
```

```text
>> let x = 40;
>> x + 2
42
>> :help
```

See [The REPL](#the-repl) for the full command reference.

### Running a script

```bash
./interpreter script.spl
```

### Running untrusted / third-party code

For scripts you don't fully trust (user-submitted rules, plugin scripts,
multi-tenant workloads), use the `untrusted` execution profile. It applies
strict host protection, bounded runtime limits, worker-process execution,
and a default read-only filesystem capability rooted at the script's own
directory:

```bash
./interpreter --profile untrusted script.spl
./interpreter --profile untrusted --allow-cap network --allow-network example.com script.spl
```

On Linux, add `--require-os-isolation` for an additional OS-level sandbox
boundary (via `bubblewrap`). See [Security & Sandboxed
Execution](#security--sandboxed-execution) for the complete model.

### A slightly bigger taste

```spl
import "std/core" as core;

function fib(n) {
    if (n < 2) { return n; }
    return fib(n - 1) + fib(n - 2);
}

let numbers = [1, 2, 3, 4, 5];
let doubled = numbers.map(x => x * 2);
let total = doubled.reduce((a, b) => a + b, 0);

print core.sprintf("fib(10)=%d doubled=%v total=%d", fib(10), doubled, total);

type Result = Ok(value) | Err(error);
let safeDiv = function(a, b) {
    if (b == 0) { return Err("division by zero"); }
    return Ok(a / b);
};

let outcome = match (safeDiv(10, 2)) {
    case Ok(v)  => "result: " + v
    case Err(e) => "error: " + e
};
print outcome;
```

---

## Language Guide

### Comments & Literals

```spl
// line comment
# also a line comment
/* block comment, /* nesting */ too */
```

**Numeric literals**:

```spl
print 0b1010;    // 10  (binary)
print 0o755;     // 493 (octal)
print 0xFF;      // 255 (hex)
print 1_000_000; // digit separators anywhere between digits
print 3.14_15;   // 3.1415
```

**String literal forms**:

| Form | Syntax | Interpolation |
|---|---|---|
| Single/double-quoted | `'text'` / `"text"` | no |
| Triple-quoted (raw) | `'''text'''` / `"""text"""` | no |
| Heredoc (raw) | `<<MARKER ... MARKER` | no |
| Backtick template | `` `text ${expr}` `` | yes |
| Tagged block | `` tag`code` `` | dispatched to a registered language handler |

```spl
let name = "SPL";
let greeting = `Hello ${name}, 1 + 1 = ${1 + 1}`;
print greeting; // Hello SPL, 1 + 1 = 2
```

Tagged blocks (`` xql`...` ``) are lexed as a single unit and dispatched to
a host-registered handler — see [XQL](#xql-data-pipeline) for a real
example.

### Variables, Constants & Destructuring

```spl
let x = 10;
const PI = 3.14159;
let y: int = 10;          // optional type annotation
```

`const` bindings cannot be reassigned. Both accept the common
**`[value, error]` tuple** return convention used throughout the builtin
library and plugins:

```spl
let a, b = [1, 2];               // a = 1, b = 2

import "database" as database;
let conn, err = database.connect("sqlite", ":memory:");
```

**Object (hash) destructuring**:

```spl
let {name: n, age: a = 30, ...rest} = {"name": "Alice", "city": "NYC"};
print n;    // Alice
print a;    // 30 (default — "age" was missing)
print rest; // {city: "NYC"}
```

`key: newName` renames, `key = default` (or `key: newName = default`)
supplies a fallback, and `...rest` collects every remaining key.

**Array destructuring**:

```spl
let [head, ...tail] = [1, 2, 3, 4, 5];
print head; // 1
print tail; // [2, 3, 4, 5]
```

The same destructuring syntax is reused in function parameters, `match`
patterns, and `for (k, v in hash)` loop bindings.

### Operators

Precedence (lowest to highest):

```text
LOWEST < ASSIGN < ??  < || < && < | < ^ < & < ==/in/not
      < < > <= >=  <  << >>  <  +  <  ..  <  *
      < ** (right-assoc) < prefix (-, !, typeof) < call/index/dot < ++/--
```

```spl
print 2 ** 3 ** 2; // 512 — ** is right-associative
print 7 / 2;       // 3   — integer division truncates toward zero
print 7.0 / 2;     // 3.5

// Rule-style expressions with case-insensitive word aliases for && || !:
amount > 100000 and department in ["finance", "procurement"] and risk_score >= 70
status not in ["closed", "archived"] or not reviewed

let flags = 0;
flags |= 0b0010;               // compound bitwise assignment
let cfg = null;
cfg ??= "default";             // nullish-coalescing assignment
let label = (score >= 60) ? "pass" : "fail"; // ternary

print 1..5;      // [1,2,3,4,5]     — inclusive range
print "a".."e";  // ["a","b","c","d","e"]

// Pipeline: value |> fn prepends value as fn's first argument
let stageMap    = arr => arr.map(x => x * 2);
let stageFilter = arr => arr.filter(x => x > 5);
let result = [1, 2, 3, 4, 5] |> stageMap |> stageFilter;

let x = { a: { b: 1 } };
print x?.a?.b;   // 1 — optional chaining, short-circuits to null on a missing link
print x?.c?.d;   // null, no error

print typeof 42;    // "integer"
function sum(...nums) { return nums.reduce((a,b)=>a+b, 0); } // rest params
let combined = [0, ...[1,2,3], 4]; // spread
```

`a in b` / `a not in b` works over arrays (element check), hashes (key
check), strings (substring check), and ranges (inclusion check).

### Control Flow

`if`/`else` is an **expression**:

```spl
let label = if (score >= 60) { "pass" } else { "fail" };
```

```spl
while (i < 5) { ...; i = i + 1; }
do { ...; } while (n < 3);
for (let i = 0; i < 5; i = i + 1) { print i; }

for (v in [10, 20, 30]) { print v; }
for (i, v in [10, 20, 30]) { print sprintf("%d:%d", i, v); }
for (k, v in {"a": 1, "b": 2}) { print sprintf("%s=%d", k, v); }

switch (lang) {
    case "go": print "Go lang";
    case "js", "ts": print "JavaScript family"; // multiple values, no fallthrough
    default: print "unknown";
}
```

`break`/`continue` work inside every loop form. For richer branching over
shapes/types (rather than a flat `switch`), reach for `match` (below).

### Functions & Closures

```spl
function fact(n) {
    if (n <= 1) { return 1; }
    return n * fact(n - 1);
}

let makeAdder = function(x) {
    return function(y) { return x + y; };  // captures x by reference
};
let add10 = makeAdder(10);
print add10(5); // 15

let square = x => x * x;                // arrow function, implicit return
let add = (a, b) => a + b;
let blockFn = (x) => { let y = x * 2; return y + 1; }; // block body needs `return`

function greet(name, greeting = "Hello") { return greeting + ", " + name; }
function sum(...nums) { return nums.reduce((a, b) => a + b, 0); }
function describe({name, age = 0}) { return name + " is " + age; } // param destructuring

function sumTyped(values: Array<int>): int {   // optional type annotations
    let total: int = 0;
    for (value in values) { total += value; }
    return total;
}

async function fetchValue() { return 42; }     // returns a Future — see Concurrency
```

Array higher-order methods work the same way: `[1,2,3].map(x => x*2)`,
`.filter(...)`, `.reduce(...)`.

### Pattern Matching (`match`)

`match` is both a statement and an expression, and supports literal,
wildcard, variable-binding, typed-binding, array/object destructuring,
guard, OR, range, comparison, extractor, and ADT-constructor patterns —
freely nested:

```spl
let describe = value => match (value) {
    case 42                       => "the answer"
    case _ if typeof value == "string" && value == "go" => "golang"
    case n: integer                => "int " + n
    case [a, b, c]                 => a + b + c
    case {type: "click", target: t} => "clicked " + t
    case 1 | 2 | 3                 => "small"
    case 90..100                   => "A grade"
    case > 10                      => "big"
    case Some(x)                   => "got " + x
    case None                      => "nothing"
    case _                         => "anything else"
};
```

```spl
type Result = Ok(value) | Err(error);
let outcome = Ok(42);
let msg = match (outcome) {
    case Ok(v)  => "ok: " + v
    case Err(e) => "err: " + e
};
```

When the matched value is an [algebraic data type](#algebraic-data-types)
value, `match` requires every variant to be covered (by a constructor
pattern, an OR-pattern, a wildcard, or a plain binding) — a missing variant
is a runtime error, and the static checker (`spltool check`) flags it at
edit time too.

### Error Handling

`try`/`catch` is an **expression**:

```spl
let result = try {
    throw "boom";
} catch (e) {
    "caught: " + e;
};
print result; // caught: boom
```

```spl
let r = try {
    1 / 0;
} catch (e) {
    "div error: " + e;
} finally {
    print "always runs";
};
```

Structured errors carry a machine-checkable `code`:

```spl
let r = try {
    throw Error("bad input", {"code": "E_BAD"});
} catch (e: Error) {
    e;
};
print r; // {code: "E_RUNTIME", message: "[E_BAD] bad input", name: "Error", stack: ""}
```

Under a restrictive security policy, a denied operation raises a catchable
error rather than crashing the script — see
[Security](#security--sandboxed-execution).

### Classes & Interfaces

```spl
class User {
    init(name) { this.name = name; }
    greet() { return "Hello " + this.name; }
}
let u = User("SPL");    // classes are directly callable — `new` is optional
print u.greet();        // Hello SPL

abstract class Shape {
    abstract area();
    describe() { return "area=" + this.area(); }
}
class Circle extends Shape {
    init(radius) { this.radius = radius; }
    area() { return 3.14159 * this.radius * this.radius; }
}
print Circle(2).describe(); // area=12.56636

class Person {
    init(name) { this.name = name; }
    greet() { return "Hi, " + this.name; }
}
class Employee extends Person {
    init(name, role) {
        super(name);        // parent constructor
        this.role = role;
    }
}

class BankAccount {
    private balance = 0;
    init(initial) { this.balance = initial; }
    deposit(amount) { this.balance += amount; return this.balance; }
}

class InstanceCounter {
    static total = 0;
    init() { InstanceCounter.total += 1; }
    static getTotal() { return InstanceCounter.total; }
}
```

Interfaces (`interface Greetable { greet(); }`, `class X implements
Greetable`) record metadata for tooling/reflection but are not enforced at
runtime.

### Algebraic Data Types

```spl
type Result = Ok(value) | Err(error);
type Shape  = Circle(radius) | Rectangle(width, height) | Point();

function area(s) {
    return match (s) {
        case Circle(r)        => 3.14159 * r * r
        case Rectangle(w, h)  => w * h
        case Point()          => 0
    };
}
print area(Circle(2));       // 12.56636
print area(Rectangle(3, 4)); // 12
```

Reach for an ADT + exhaustive `match` when you want a missed case caught
for you; reach for a class when you want shared behavior, inheritance, or
mutable state.

### Macros

Macros splice their body into the call site at the AST level, with
hygienic renaming of internal locals — useful for deferred/repeated
evaluation of a block, or lvalue-style parameters no ordinary function
could have:

```spl
macro repeat(n, body) {
    let i = 0;
    while (i < n) { body; i += 1; }
}
repeat(2) { print "macro repeat"; }

macro swap(a, b) {
    let temp = a;
    a = b;
    b = temp;
}
let left = "left", right = "right";
swap(left, right);
print [left, right]; // ["right", "left"]
```

Most scripts never need one — the same effects usually come from functions
and closures.

### Concurrency: Async/Await, Channels, Generators & Streams

```spl
let asyncDouble = async function(x) { return x * 2; };
print await asyncDouble(21); // 42
```

> Prefer `let name = async function(...) {...};` over a bare
> `async function name() {...}` statement — the named-statement form
> doesn't currently bind `name` into the enclosing scope.

```spl
let ch = channel();          // unbuffered
let producer = go(function() {   // runs on a goroutine, returns a Future
    send(ch, 42);
    return "sent";
});
print recv(ch);       // 42
print await producer; // sent

let f1 = go(function() { sleep(10); return 1; });
let f2 = go(function() { sleep(5); return 2; });
print await_all([f1, f2]); // [1, 2]
```

`select` waits on multiple channels — its cases are **receive-only**:

```spl
let bus = channel(10);
let stop = channel();
let worker = go(function() {
    let count = 0;
    while (true) {
        select {
            case bus <- evt:  { count += 1; }
            case stop <- _:   { return "stopped after " + count; }
        }
    }
});
send(bus, "a"); send(bus, "b"); sleep(20);
send(stop, true);
print await worker; // stopped after 2
```

This is the standard pattern for a cleanly-stoppable background worker: a
dedicated stop channel alongside the real work channel.

```spl
let s = stream([1, 2, 3, 4, 5]);
let doubled = stream_map(s, x => x * 2);
let evens = stream_filter(doubled, x => x % 4 == 0);
print stream_to_array(evens); // [4, 8]

for await (v in stream([10, 20, 30])) { print v; }
```

`go_async(fn)` fires and forgets; `await_race([futures])` resolves with
whichever future finishes first.

### Modules: Imports & Exports

```spl
export const name = "mathlib";
export let answer = 42;
export function add(a, b) { return a + b; }
```

```spl
import "./mathlib.spl" as math;
print math.answer; print math.add(2, 3);

import {answer, add} from "./mathlib.spl";
import {a as c} from "./mathlib.spl";       // rename on import
import * as m2 from "./mathlib.spl";
```

- Relative paths resolve against the **importing file's own directory**.
- Re-importing the same resolved path returns cached exports; the cache
  auto-invalidates when the file's mtime changes (useful for hot reload).
- Circular imports are detected and rejected with a clear error.
- `SPL_MODULE_PATH` adds extra module lookup directories.

Beyond filesystem modules, a set of **virtual standard modules** are
available with no file on disk — `std/core`, `std/fs`, `std/render`,
`std/test`, `std/config`, `math`, `string`, `array`, `hash`, `time`,
`json`, `csv`, `crypto`, `path`, `random` (each with a `std/`-prefixed
alias too), plus every optional plugin's own module name once it's linked
into the running binary (`database`, `images`, `integrations`, `pdf`,
`xql`, `yaml`, `money`, `phone`, `ip`, `naturaldate`, `wuid`, `shamir`,
`metadata`, `cryptoextra`, `securetoken`, `rules`, `secretr`, `server`,
`tcpguard`, `emailvalidator`, and the `tools/*` family):

```spl
import "std/core" as core;
core.sprintf("value=%d", 42);

import "database" as database;
let conn, err = database.connect("sqlite", ":memory:");
```

The first group above (`std/*`, `math`, `string`, `array`, `hash`, `time`,
`json`, `csv`, `crypto`, `path`, `random`) is always linked in — those
functions (`sprintf`, `upper`, `sum`, `md5`, ...) also work as plain global
calls with **no import at all**; the `import ... as` form is just an
optional namespacing convenience for them.

Every **optional plugin module** (`database`, `pdf`, `images`,
`integrations`, `money`, `phone`, `ip`, `naturaldate`, `wuid`, `shamir`,
`metadata`, `cryptoextra`, `securetoken`, `yaml`, `xql`, `rules`,
`secretr`, `server`, `tcpguard`, `emailvalidator`) is different: its
functions are **only reachable through `import`** — there is no global
fallback, so calling e.g. `pdf_merge(...)` without importing `pdf` first
fails with `identifier not found`, even in the full `cmd/interpreter`
build. (The `tools/*` family is the one exception: those daily-ops
builtins are always linked into `cmd/interpreter` as core functions, so
`import "tools/files"; bulk_rename(...)` and a bare `bulk_rename(...)`
with no import at all behave identically — the import is there for
readability/namespacing, not because it's required.) Two ways to bring
plugin functions into scope:

```spl
import "pdf" as pdf;
pdf.merge("out.pdf", "a.pdf", "b.pdf");   // dot access, prefix stripped from pdf_merge

import "money";
let price, err = new("19.99", "USD");     // unaliased: names bound directly into scope
```

Each plugin builtin is registered under a prefixed global name
(`pdf_merge`, `money_new`, `db_connect`, ...). An **aliased** import
(`as pdf`) exposes them as `pdf.merge(...)` with the module's own prefix
stripped. An **unaliased** import binds the same stripped names directly
into the current scope (`merge(...)`), which is convenient but will
shadow any identically-named variable or builtin already in scope — this
guide uses the aliased form throughout to keep call sites unambiguous. If
a builtin's name doesn't start with its module's prefix (e.g. `database`'s
`query`/`lazy_query`), or if stripping the prefix would collide with
another export in the same module (`database`'s `db_query` would collide
with the already-unprefixed `query`), the full prefixed name is kept
instead, so `database.db_query(...)` (not `database.query`, which is the
separate fluent query builder) is correct.

If a script calls a plugin builtin that isn't linked into the running
binary, it fails with an actionable error naming the missing module rather
than a bare "identifier not found".

Import behavior can be locked down under a restrictive security policy:
`--allow-import-path`/`--deny-import-path`,
`--allow-import-package`/`--deny-import-package`,
`--deny-dynamic-imports` (rejects any `import` whose path isn't a string
literal), plus depth/count limits — see
[Security](#security--sandboxed-execution).

### Package Manifests

For deterministic **bare** (non-relative) module imports, similar in
spirit to a lockfile-based package manager:

```bash
spltool mod init example/app     # writes spl.mod
```

```json
{
  "module": "example/app",
  "dependencies": { "mathlib": "./deps/mathlib" }
}
```

```bash
spltool mod tidy      # resolves + checksums dependencies into spl.lock
spltool mod verify    # fails (non-zero exit) if on-disk content has drifted
```

```spl
import "mathlib/math.spl" as math; // resolved via the "mathlib" alias
print math.answer;
```

Commit both `spl.mod` and the regenerated `spl.lock`, and run
`spltool mod verify` as a CI/deploy gate.

### Arrays & Hashes

```spl
let arr = [1, 2, 3];
let hash = {"name": "SPL", "count": 3};

let name = "x";
let obj = { name };                 // shorthand for {"name": name}
let merged = { ...obj, extra: 1 };  // spread
let b = [0, ...[1, 2, 3], 4];        // array spread

hash["name"]; hash.name; person?.address?.city; // access forms
```

**Array methods** (dot syntax): `.length`, `.first()`, `.last()`, `.map(fn)`, `.filter(fn)`,
`.forEach(fn)`, `.find(fn)`, `.every(fn)`, `.some(fn)`, `.reduce(fn[,
init])`, `.indexOf(x)`, `.includes(x)`, `.join([sep])`, `.flat()`,
`.flatMap(fn)`, `.reverse()`, `.slice(start[, end])`, `.sort()` (all of
these return a **new** array), plus mutating `.push(v)`, `.pop()`,
`.shift()`, `.unshift(v)`, and `.at(i)` (negative indices count from the
end):

```spl
let scores = [5, 3, 1, 4, 2];
print scores.sort();               // [1,2,3,4,5] — new array, scores unchanged
print scores.reduce((a,b)=>a+b,0);  // 15
print scores.at(-1);                // 2

let arr2 = [1, 2, 3];
arr2.push(4);                       // mutates in place
```

Arrays of hashes also expose chainable, non-mutating collection operations:

```spl
let large = orders.filter(o => o.total >= 1000);
print large.pluck("id");             // [{id:"B-42"}, {id:"C-08"}]
print large.pluck("id", "total");   // projected hashes for many fields
print large.column("id");            // ["B-42", "C-08"]
print large.except("id");            // new hashes without id
print large.pluck();                  // shallow copy of the collection
```

The full set includes `.only(fields...)`, `.except(fields...)`,
`.where(field, value)`, `.where_in(field, values)`,
`.first_where(field, value)`, `.group_by(field)`, `.key_by(field)`,
`.sort_by(field[, "asc"|"desc"])`, `.unique_by(field)`, `.compact()`,
`.take(n)`, `.drop(n)`, `.chunk(size)`, `.sum([field])`, and `.avg([field])`.
Field-based methods accept dotted paths such as `"customer.region"`.
`pluck` preserves object shape; use `.column(field)` or `.values_of(field)`
when a flat array of scalar values is desired.

**Hash methods**: `.keys()`, `.values()`, `.entries()`, `.length`,
`.only(fields...)`, `.except(fields...)`, `.has(fields...)`, and
`.get(field[, fallback])` (with `pick`/`omit` aliases).

A large complementary set of **free** collection functions (`first`,
`last`, `sum`, `avg`, `group_by`, `merge`, `has_key`, `get`, `zip`,
`chunk`, `clamp`, `unique`, `sort_by`, `pluck`, `deep_equal`, ...) lives
alongside these — see [Collections](#collections).

### Strings & Template Literals

```spl
"  Hello  ".trim();              // "Hello"
"Hello".upper();  "Hello".lower();
"hello world".title();           // "Hello World"
"HelloWorld".snake_case();       // "hello_world"
"hello_world".camel_case();      // "helloWorld"
"Hello World!".slug();           // "hello-world"

"Hello".starts_with("He"); "Hello".ends_with("lo"); "Hello".includes("ll");
"Hello".index_of("l");           // 2
"aXaXa".count_substr("X");       // 2

"Hello".replace("l", "L");       // "HeLLo" — replaces all occurrences
"ab".repeat(3);                   // "ababab"
"Hello World".substring(0, 5);    // "Hello"
"5".pad_left(3, "0");             // "005"
"a,b,c".split(",");                // ["a","b","c"]
"Hello".at(-1);                    // "o"

"test123".regex_match("[0-9]+");           // true
"test123".regex_replace("[0-9]+", "#");    // "test#"
```

Template literals interpolate any expression:

```spl
let name = "SPL";
print `Hello ${name}, ${1 + 1}`; // Hello SPL, 2
```

For printf-style formatting and `{placeholder}`-style templating, see
[Formatting & Interpolation](#formatting--interpolation).

### Optional Typing

SPL is dynamically typed at runtime, but the parser accepts a gradual type
annotation syntax that's surfaced by tooling (hover/completion, static
checks) — it documents intent rather than statically rejecting mismatched
calls:

```spl
const DEFAULT_LIMIT: int = 10;
let typedIdentifiers: Array<int> = [1, 2, 3];
let typedScores: Map<string, float> = {"alice": 98.5, "bob": 91.0};
let optionalId: int? = null;               // nullable
let textOrNumber: string | int = "forty-two"; // union

function sumTyped(values: Array<int>): int {
    let total: int = 0;
    for (value in values) { total += value; }
    return total;
}
```

Combine with `typeof` and the `is_*` predicate builtins for actual runtime
type guards.

### Ownership & Immutability Helpers

Two Rust-inspired wrapper types layered over ordinary arrays/hashes:

```spl
let frozen = immutable({"a": 1});
let result = try { frozen.a = 2; "unexpected"; } catch (e) { e; };
print result; // cannot set property on HASH
```

`immutable(...)` makes mutation attempts raise a catchable error — useful
as a write-guard for shared config/constants passed into other scopes.
(Reading a property back out of a frozen value is not fully reliable in
the current build; keep an unfrozen reference if you need to read the
value later.)

```spl
let ownedData = move([1, 2, 3]); // an ownership-tracking marker;
print ownedData;                  // reads/prints transparently
```

`move(...)` is primarily a documentation/intent marker (there's no
compiler pass enforcing single ownership) — use it to signal that a value
is being handed off, e.g. into a background job.

---

## Standard Builtin Library

Every builtin below is always available, regardless of which optional
plugins are linked. Use `help()` to list every registered builtin in the
running binary, and `help("name")` for a one-line description.

### Core & Type Introspection

```spl
print len([1,2,3]); print len("hello"); print keys({"a":1,"b":2});
print typeof "x";           // "string" (friendlier than type(), which returns "STRING")
print is_int(1); print is_string("x"); print is_array([1]); print is_hash({});
print is_null(null); print is_number(1); print is_function(function(){});

print to_int("42"); print to_float("3.14"); print to_string(42);
print parse_bool("true"); print parse_int("ff", 16); // 255

print time();                 // unix timestamp
sleep(100);                    // block for 100ms
let name = input("Name: ");    // read a line from stdin
print random();                 // [0,1) float, or random(max) for an int
seed_random(42);                 // reproducible PRNG sequences
```

### Collections

```spl
print first([1,2,3]); print last([1,2,3]); print rest([1,2,3]);
print sum([1,2,3]); print avg([1,2,3]);
print mean([1,2,3,4]); print median([1,2,3,4]); print stddev([1,2,3,4]);

print merge({"a":1}, {"b":2});          // {a:1, b:2}
print pick({"a":1,"b":2,"c":3}, ["a","c"]);
print coalesce(null, null, "x");         // "x" — first non-null argument

print group_by([{"k":"a","v":1},{"k":"b","v":2},{"k":"a","v":3}], "k");
print zip([1,2], [3,4]);        // [[1,3],[2,4]]
print chunk([1,2,3,4,5], 2);    // [[1,2],[3,4],[5]]
print unique([1,1,2,3,3]);      // [1,2,3]
print sort_by([{"n":2},{"n":1}], "n");
print pluck([{"n":1},{"n":2}], "n"); // [1,2]
print clamp(15, 0, 10);           // 10
```

### Time & Dates

Timestamps are Unix seconds unless a function name says `_ms`:

```spl
print now(); print now_iso(); print now_format("YYYY-MM-DD");
print format_time(now(), "YYYY-MM-DD HH:mm:ss");
print parse_time("2024-01-15", "YYYY-MM-DD");
print iso_to_unix("2024-01-15T00:00:00Z");
print time_add(now(), 1, "day"); print time_diff(a, b, "day");
print start_of_day(now()); print end_of_month(now()); print is_weekend(now());
print to_timezone(now(), "America/New_York");

// Unix timestamps also expose the same operations as dot methods:
let ts = 1705276800;
print ts.to_iso(); print ts.add(1, "day"); print ts.format("YYYY-MM-DD");

print parse_duration("1h30m");   // milliseconds
print format_duration(5400000);  // "1h30m0s"-style
```

For calendar-free, human-phrased date parsing ("tomorrow at 9am", "next
monday"), see
[Money, Dates & Sortable IDs](#money-natural-language-dates--sortable-ids).

### Math

```spl
print abs(-5); print pow(2, 10); print sqrt(16); print min(3,1,2); print max(3,1,2);
print PI(); print E(); print sin(0); print log2(8); print hypot(3, 4);
print cbrt(27); print gcd(12, 18); print lcm(4, 6); print is_prime(17);
print round_to(3.14159, 2);  // 3.14
print lerp(0, 10, 0.5);      // 5
print map_range(5, 0, 10, 0, 100); // 50
print factorial(5);           // 120
print random_choice([1,2,3]); print shuffle([1,2,3,4,5]); print sample([1,2,3,4,5], 2);
```

### Formatting & Interpolation

```spl
let s = sprintf("name=%s n=%d ok=%t type=%T val=%v", "spl", 7, true, 3.14, {"a": 1});
// %T is SPL-specific: the argument's runtime type name

printf("user=%s age=%d\n", "alice", 30); // formats AND writes to stdout

print interpolate("Hello {name}, items={count}", {"name": "SPL", "count": 3});
print interpolate("{0} + {1} = {2}", null, 20, 22, 42); // positional form
print interpolate("literal {{brace}}", {}); // literal {brace}
```

Use `sprintf` for type-aware printf-style formatting, `interpolate` for
placeholder substitution against a data hash (e.g. translated message
templates), and backtick templates for inline expression interpolation in
source code.

### Filesystem, OS & Process Execution

```spl
let ok, err = write_file("test.txt", "Hello File System!");
let content, rerr = read_file("test.txt");
print file_exists("test.txt");
remove_file("test.txt");

print os_env("HOME");           // read
os_env("MY_VAR", "hello");      // write (subject to security policy)

let names, derr = readdir(".");
let matches, gerr = glob("*.txt");
mkdir("subdir"); rmdir("subdir");
let info, serr = stat("test.txt"); // {name, size, mode, mod_time, is_dir}
print basename("/a/b/c.txt"); print dirname("/a/b/c.txt");
print path_join("a", "b", "c.txt");

let output = exec("echo", "hello-exec", 1000); // captured stdout, 1000ms timeout
exit(0);
```

`exec` is command-whitelisted, can be globally disabled
(`SPL_DISABLE_EXEC=1`), and requires the `exec` capability under a
restrictive policy. All path-based builtins are sandboxed to the script's
module directory by default. See [Daily-Ops
Tools](#daily-ops--data-validation-plugins) for higher-level, preview-first
file/archive/media operations.

### Crypto & Encoding

```spl
print hash("sha256", "hello"); print md5("hello"); print sha256("hello");
print hmac("sha256", "key", "data"); print hmac_sha256("data", "key");
print password_hash("secret");
print password_verify("secret", password_hash("secret")); // true
print encrypt("aes_gcm", "0123456789abcdef", "hello");
print decrypt("aes_gcm", "0123456789abcdef", cipher);
print constant_time_eq("a", "a"); // timing-safe comparison

print base64_encode("hello"); print base64_decode("aGVsbG8=");
print hex_encode("hi"); print url_encode("a b");
print json_encode({"a": 1}); print json_decode('{"a":1}');

print uuid();      // v7 (time-ordered) by default
print uuid(4);      // v4 (fully random)
print random_bytes(4); print random_string(8, "abc");
print password_generate(16); print api_key("sk", 24);

print secret("x");                 // wraps as SECRET, prints as ***
print secret_reveal(secret("x"));  // "x"
print secret_mask("mypassword");   // "********rd"

print regex_find_all("a1b2c3", "[0-9]");  // ["1","2","3"]
print escape_html("<div>&\"'</div>");
print json_parse('{"a":1}'); print json_stringify({"a": 1}); // aliases
```

For bcrypt/JWT, Shamir secret sharing, and encrypted-token helpers, see
[Secrets & Extra Crypto](#secrets-vault--extra-crypto). For email/phone/IP
validation and money arithmetic, see [Daily-Ops
Plugins](#daily-ops--data-validation-plugins).

### Testing

```spl
assert_true(1 == 1, "one equals one");
assert_eq(2 + 2, 4);
assert_neq(2 + 2, 5);
assert_contains([1, 2, 3], 2);
let threw = assert_throws(function() { throw "x"; });
print test_summary(); // {failed: 0, passed: 4, total: 4}

test "basic math" {
    assert_eq(1 + 1, 2);
}

run_tests("tests/math_test.spl");
run_tests(["tests/a_test.spl", "tests/b_test.spl"]);
```

### Data Values: JSON, CSV & Files

```spl
let data = read_json("data.json");           // parsed value directly, no tuple
let table = read_csv("data.csv");             // returns a TableValue
write_json("out.json", {"x": 1});
write_csv("out.csv", table);

print table_rows(table);    // [{name:"Alice",age:"30"}, ...]
print table_columns(table); // ["name", "age"]
print table_filter(table, function(row) { return row.age > "25"; });

let decoded = csv_decode("a,b\n1,2\n3,4");    // decode CSV text with no file I/O
print csv_encode(decoded);

let f = file_load("data.json");
print file_name(f); print file_mime(f); print file_size(f); print file_text(f);
file_save(f, "copy.json"); file_copy("a.json", "b.json");

let art = file("data.json");   // wraps a path/URL/bytes as a display artifact
let r = render({"a": 1});       // generic value -> artifact (used by the playground/REPL)
```

### Image Processing (optional plugin)

Requires the full `cmd/interpreter` build (a lightweight custom embedding
host would need to import this plugin explicitly):

```spl
import "images" as images;

let img = images.load("logo.png");
print images.info(img); // {format, width, height, mime, name, size}
let resized = images.resize(img, 50, 50, {"filter": "linear"});
let cropped = images.crop(img, x, y, width, height);
let converted = images.convert(img, "jpeg"); // png|jpeg|jpg|gif
let artifact = images.render(converted);      // wrap for display
```

For batch, file-to-file image operations that don't require decoding into
memory, see [Daily-Ops Tools](#daily-ops--data-validation-plugins) — those
work in every build.

---

## Server & Runtime Features

### HTTP Server, Routing & SSE

Requires the full `cmd/interpreter` build.

```spl
import "server" as svr;

let app = svr.server(3099);   // or svr.server("localhost:3099")

svr.route(app, "GET", "/hello", function(req, res) {
    res.json({"msg": "hi"});
});

svr.middleware(app, function(req, res, next) {
    print "log: " + req.method + " " + req.path;
    next();
});

svr.middleware(app, "/api", function(req, res, next) {  // path-scoped
    if (req.get_header("Authorization") == null) {
        res.status(401).json({"error": "unauthorized"});
        return;                 // skipping next() short-circuits the chain
    }
    next();
});

svr.route_group(app, "/api", "GET", "/health", function(req, res) {
    res.json({"ok": true});
});

svr.static(app, "/public/", "./public");
svr.template_dir(app, "./views");
```

Request/response objects:

```spl
req.method; req.path; req.param("id"); req.get_header("Authorization"); req.json();

res.status(401); res.header("X-Custom", "value");
res.json({"ok": true}); res.text("plain"); res.html("<h1>hi</h1>");
res.redirect("/login", 302); res.file("path/to/file");
res.render("template.html", data);      // server-side render
res.render_ssr("template.html", data);  // SSR + client hydration payload
```

Server-Sent Events:

```spl
svr.route(app, "GET", "/events", function(req, res) {
    let sse = res.sse();
    sse.send("tick", json_encode({"seq": 1}));
    sse.close();
});
```

Stateful in-memory example (server-side closures give you request-scoped
mutable state with no database):

```spl
let users = {};
let nextID = 1;

svr.route(app, "POST", "/api/users", function(req, res) {
    let body = req.json();
    let id = nextID;
    nextID += 1;
    users[to_string(id)] = body;
    res.json({"id": id});
});

svr.route(app, "GET", "/api/users/:id", function(req, res) {
    res.json(users[req.param("id")]);
});

svr.listen(app, 3099);                     // blocks
let handle = svr.listen_async(app, 3099);  // non-blocking
svr.shutdown(app);
```

`listen`/`listen_async` require both `server` and `network` capabilities
under a restrictive security policy.

### HTTP Security Middleware (tcpguard)

Requires the full `cmd/interpreter` build. Wraps
`github.com/oarkflow/tcpguard`: describe request-level security rules (rate
abuse, bad user agents, sensitive-path protection, risk scoring) in BCL and
enforce them automatically on every request instead of hand-written `if`
checks:

```spl
import "tcpguard" as tcpguard;

let policy = `
guard "tcpguard-main" {
  mode enforce
  version "1"
}

rule "protect-admin" {
  scope {
    methods ["GET", "POST"]
    paths ["/admin/*"]
  }

  trigger {
    on request.received
  }

  when {
    any {
      request.user_agent equals ""
      request.user_agent contains "sqlmap"
    }
  }

  risk {
    base 90
  }

  actions {
    critical {
      run block
    }
  }
}
`;

let [bundle, err] = tcpguard.load(policy);       // inline block, or a file/dir path - auto-detected
if (err != null) { throw err; }
let [guard, gerr] = tcpguard.new(bundle);         // {"mode": "enforce"|"monitor", "geoip": bool}
if (gerr != null) { throw gerr; }

svr.route(app, "GET", "/admin/secret", function(req, res) { res.json({"ok": true}); });
tcpguard.guard_middleware(app, guard);   // attaches as global middleware; blocked requests never reach routes

// ad-hoc evaluation without a live server:
let [decision, everr] = tcpguard.evaluate(guard, {"method": "GET", "path": "/admin/secret", "headers": {"User-Agent": ["sqlmap/1.0"]}});
print decision.Effect; // "block"
```

`tcpguard.new()` requires the `policy` capability under a restrictive
security policy. GeoIP enrichment is opt-in (`{"geoip": true}`) since it
loads a sizeable in-memory dataset on first use. This wraps only the core
load/attach/evaluate loop — abuse-detector tuning, approval workflows,
Redis-backed stores, and the management server aren't exposed yet. See
[docs/features/51](docs/features/51-http-security-middleware-with-tcpguard.md)
and `examples/tcpguard_all_in_one.spl`.

**BCL formatting matters for enforcement, not just style**: some blocks
(e.g. `risk { base 90 }` written on one line) can silently fail to take
effect depending on what else is condensed around them, degrading a
rule's outcome (e.g. `block` silently becoming `monitor`) with no parse
error. Always write policy blocks one field per line as above, and verify
a new/edited policy with `tcpguard.evaluate` (or a live request) before
trusting it in production — don't assume a policy does what it says just
because it parses.

### Scheduler & File Watching

```spl
let jobA = schedule("* * * * *", "heartbeat", function() { print "heartbeat"; });
let jobB = schedule_interval(50, "tick", function() { print "tick"; }); // ms or "2s"/"1h30m"
let jobC = schedule_once("* * * * *", "init", function() { print "runs once"; });

print schedule_list(); // [{id, name, active, run_count, next_run?, ...}, ...]
schedule_cancel(jobB);

let executed = schedule_run(5);       // deterministically run up to 5 due jobs
schedule_worker("10s");                // block, running due jobs, for 10 seconds

schedule_persist("jobs.json");
let restored = schedule_restore("jobs.json");
schedule_timezone("UTC");

let f = background(function() { return "async work done"; }); // like go(), scheduler-namespaced
print await f;
```

```spl
let id = watch("config.json", function(event) { print "config changed: " + event; });
unwatch(id);
hot_reload("script.spl"); // auto re-evaluates a file whenever it changes on disk
```

### Reactive State

Server-side signals/computed-values/effects — useful for computed values
other route handlers read, or for driving `res.render_ssr(...)` templates:

```spl
let count = signal("count", 0);
count.set(5);
count.set(function(prev) { return prev + 1; }); // updater form
print count.get(); // 6

let multiplier = signal("multiplier", 3);
let tripled = computed(function() { return count.value * multiplier.value; });

let log = effect(function() {
    print sprintf("count=%d tripled=%d", count.value, tripled.value);
});
count.set(10);            // effect re-runs automatically
log.dispose();              // stop re-running

batch(function() {          // groups multiple updates as an organizational unit
    count.set(100);
    multiplier.set(2);
});
```

---

## Data, Secrets & Integration Plugins

Everything in this section requires the full `cmd/interpreter` build (a
custom embedding host would import the corresponding Go package
explicitly).

### Database

```spl
import "database" as database;

let conn, err = database.connect("sqlite", ":memory:"); // also: postgres, mysql

let _, cerr = database.exec(conn, "CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT, qty INTEGER)");
let _, e1 = database.exec(conn, "INSERT INTO items(name, qty) VALUES(?, ?)", ["apples", 3]);       // positional
let _, e2 = database.exec(conn, "INSERT INTO items(name, qty) VALUES(:name, :qty)", {"name": "pears", "qty": 4}); // named

let rows, qerr = database.db_query(conn, "SELECT name, qty FROM items ORDER BY qty ASC", null, "array");

let tx, tx_err = database.begin(conn);
database.exec(tx, "INSERT INTO items(name, qty) VALUES(:name, :qty)", {"name": "committed", "qty": 7});
database.commit(tx); // or database.rollback(tx)

print database.tables(conn);
database.close(conn);
```

Fluent query builder (`database.query`, not `database.db_query` — the
two are unrelated: `db_query` runs a raw SQL string, `query` returns a
`QueryBuilder`):

```spl
let rows, err = database.query(conn, "items")
    .where("qty", ">", 3)
    .order_by("qty DESC")
    .limit(2)
    .exec();

let qb = database.query(conn, "items").where("qty", ">", 3);
print qb.sql(); // SELECT * FROM items WHERE qty > ? ...

let matched = database.query(conn, "items").where_match("{kind: \"fruit\", qty: > 1}").decode_match();
let lazyRows = database.lazy_query(conn, "items"); // forces to [rows, err] on first access
```

Query builder methods: `.from`, `.select`, `.where`/`.where_raw`,
`.where_in`, `.where_between`, `.where_like`, `.where_null`/
`.where_not_null`, `.where_filter`, `.order_by`, `.limit`/`.offset`,
`.join`, `.group_by`, `.match`/`.where_match`, `.decode`/`.decode_match`,
`.exec()`, `.lazy()`, `.sql()`.

`database.connect` requires the `db` capability under a restrictive
policy. Every DB call follows the `[value, error]` tuple convention.

### HTTP, SMTP, FTP & SFTP Integrations

```spl
import "integrations";

let res, err = http_get(url, headers, timeout_ms);
let res2, err2 = http_post(url, body, headers, timeout_ms);
let res3, err3 = http_request("POST", url, {"a": 1}, {"X-Env": "test"}, 2000);
print res3.status_code; print res3.body; // {status, status_code, body, url, ok, duration_ms, headers}

let wres, werr = webhook(url, {"event": "test"}, headers, timeout_ms);
```

A non-string body is auto-JSON-encoded. Response body size is capped
(default 1 MiB, `SPL_HTTP_MAX_BODY_BYTES`); default timeout is 30s.

```spl
let ok, err = smtp_send({
    "host": "localhost", "port": 1025,
    "from": "noreply@localhost", "to": ["alice@localhost", "bob@localhost"],
    "subject": "Build status", "body": "Pipeline complete"
    // optional: "cc", "bcc", "html"
});
```

```spl
let cfg = {"host": "ftp.example.com", "port": 21, "username": "u", "password": "p"};
let list, lerr = ftp_list(cfg, "/incoming");
let ok1, gerr = ftp_get(cfg, "/incoming/a.txt", "local/a.txt");
let ok2, perr = ftp_put(cfg, "local/a.txt", "/outgoing/a.txt");

let sftpCfg = {"host": "sftp.example.com", "port": 22, "username": "u", "password": "p"}; // or "private_key"
let slist, slerr = sftp_list(sftpCfg, "/data");
```

All of the above require the `network` capability (plus file
read/write capability for transfers), and network targets can be
restricted with `--allow-network`/`SPL_NETWORK_ALLOW`/`SPL_NETWORK_DENY`.

### PDF Generation & Editing

```spl
import "pdf" as pdf;

pdf.quick("Page one text.", "page1.pdf");

let info = pdf.info("page1.pdf");           // {pages, encrypted, ...}
print pdf.validate("page1.pdf");             // {valid, path, pages, encrypted}
print pdf.to_text("page1.pdf");
// also: pdf.to_html, pdf.to_markdown, pdf.to_json, pdf.search, pdf.extract_images

pdf.from_markdown("# Title\n\nHello **world**", "report.pdf", {
    "title": "Demo", "author": "SPL", "theme": "modern", "toc": false
});
pdf.from_html("<h1>Hi</h1>", "from_html.pdf");
pdf.from_url("https://example.com", "from_url.pdf"); // requires network capability

pdf.merge("merged.pdf", "page1.pdf", "report.pdf");
pdf.split("merged.pdf", "first_page.pdf", "1");     // page specs: "1", "1-3", "3,2,1"
pdf.delete_pages("merged.pdf", "trimmed.pdf", "2");
pdf.reorder("merged.pdf", "reordered.pdf", "2,1");
pdf.rotate("merged.pdf", "rotated.pdf", "1", 90);
pdf.compress("merged.pdf", "compressed.pdf");

pdf.protect("merged.pdf", "protected.pdf", "user-pw", "owner-pw", "aes-128"); // rc4-128|aes-128 ("aes-256" is not implemented yet)
pdf.decrypt("protected.pdf", "decrypted.pdf", "user-pw");

pdf.watermark("merged.pdf", "watermarked.pdf", "DRAFT", {"opacity": 0.25, "angle": 45});
pdf.add_page_numbers("watermarked.pdf", "numbered.pdf", {"format": "Page %d of %d"});
pdf.set_metadata("numbered.pdf", "tagged.pdf", {"Title": "Report", "Author": "SPL"});

let fields = pdf.list_form_fields("form.pdf");
pdf.fill_form("form.pdf", "filled.pdf", {"name": "Alice", "email": "a@example.com"});
```

Every function checks file read/write capability; `pdf.from_url`
additionally requires the `network` capability.

### Secrets Vault & Extra Crypto

**Vault** (`secretr.*` — requires the `secrets` capability):

```spl
import "secretr" as secretr;

secretr.set("demo/api-key", "sk_live_demo_only_not_real");
let apiKey = secretr.get("demo/api-key");
print apiKey;                    // *** (masked)
print secret_reveal(apiKey);     // sk_live_demo_only_not_real

secretr.set("demo.database.password", "hunter2");  // dot-notation nests under one entry
secretr.set("demo.database.host", "localhost");
secretr.delete("demo/api-key");

let findings = secretr.scan("aws_key = \"AKIAABCDEFGHIJKLMNOP\"");
print findings; // [{pattern, redacted, severity}, ...] — scans text for hardcoded secrets
```

**bcrypt / JWT** (`cryptoextra` — no capability required):

```spl
import "cryptoextra" as cryptoextra;

let h = cryptoextra.bcrypt_hash("hunter2");
print cryptoextra.bcrypt_verify("hunter2", h); // true

let token = cryptoextra.jwt_encode({"sub": "user-123", "role": "admin"}, "signing-secret", {
    "alg": "HS256", "expires_in": 3600
});
let claims = cryptoextra.jwt_decode(token, "signing-secret");
print claims.sub; // user-123
```

Only HMAC algorithms are supported and `jwt_decode` always verifies
against the caller-specified `alg`, so `alg: none` / algorithm-confusion
forgeries are rejected. `jwt_encode`/`jwt_decode` (and
`bcrypt_hash`/`bcrypt_verify`) raise a catchable error on failure rather
than returning a tuple.

**Shamir secret sharing** (no capability required):

```spl
import "shamir" as shamir;

let [split, err] = shamir.split("db-master-key", 3, 5); // 5 shares, any 3 reconstruct
print split; // {auth_key, shares: [...]}

let [secret, cerr] = shamir.combine(slice(split.shares, 0, 3), split.auth_key);
print secret; // "db-master-key"
```

Every share is HMAC-tagged with the `auth_key` so tampered/mismatched
shares fail loudly at combine time. Distribute shares and the auth key
separately.

**Stateless encrypted tokens** (`securetoken.*` — no capability required):

```spl
import "securetoken" as securetoken;

let tok = securetoken.encrypt({"sub": "u1", "role": "admin"}, "sekret", {"footer": "v1"});
let claims = securetoken.decrypt(tok, "sekret", {"expected_footer": "v1"});
```

An AES-256-GCM alternative to JWT when you want the payload itself
encrypted, not just signed.

### YAML Config

```spl
import "yaml" as yaml;  // or "config/yaml"

let doc = yaml.encode({"name": "svc", "replicas": 3, "tags": ["a", "b"]}, {"indent": 2});
let parsed = yaml.decode(doc);
print parsed.replicas; // 3

let cfg = config_load("config/database.yaml", "yaml"); // extends config_load with yaml support
```

`yaml_encode` serializes the actual unwrapped value — it does not re-mask
secrets, so don't encode a hash of live secrets into a log/response body.

### Template Engine & Directives

A full `@directive` HTML templating engine (distinct from the SPL
scripting language itself), consumed via `res.render(...)`/
`res.render_ssr(...)`:

```html
<div>Hello ${name}!</div>
@if(count > 0) {
  <p>You have ${count} items</p>
} @else {
  <p>No items</p>
}
@for(item in items) {
  <li>${item}</li>
}
```

```spl
svr.route(app, "GET", "/", function(req, res) {
    res.render("directive.html", {"name": "World", "count": 3, "items": ["a","b","c"]});
});
```

Expressions support `${var}`, `${obj.prop}`, `${arr[0]}`, `${fn(a,b)}`,
`${cond ? a : b}`, and filters (`${text | uppercase}`,
`${text | lowercase | capitalize}`). The directive catalog covers state
(`@signal`, `@let`, `@computed`), binding/events (`@bind`, `@handler`,
`@click`), control flow (`@if/@elseif/@else`, `@for`, `@switch`, `@match`),
reactive client-side re-rendering (`@effect`, requires SSR hydration),
components/slots (`@component`, `@render`, `@slot`, `@fill`), layout
(`@extends`, `@define`, `@block`, `@include`), and streaming
(`@stream`, `@defer`/`@fallback`, `@lazy`/`@fallback`). HTML attribute
bindings (`on:submit.prevent`, `bind:value`, `data-spl-if`) wire hydration
without hand-written JS.

### XQL Data Pipeline

A small pipeline query DSL (`source |> stage |> stage`) over in-memory
data and, via connected integrations, external HTTP/REST/GraphQL/database
sources:

```spl
import "xql";

let items = [{"id": 1, "n": "a"}, {"id": 2, "n": "b"}];
let result, err = xql```
items
|> keep id, n
|> take 1
```;
print result; // [{id: 1, n: "a"}]
```

Any array-of-hashes variable in scope (`items` above) is automatically
available as a named source inside a tagged block — this is the
recommended form. The string form, `xql.run("items |> ...")`, does **not**
auto-see scope variables; connect a source explicitly with `xql.connect`
first if you need one from there:

```spl
import "xql" as xql;

xql.connect("alias", "http", {"base_url": "https://example.com"});
print xql.list_integrations();

let result, err = xql.run(`
call https://example.com/api { method: "GET" }
|> keep userId, id, title, body
|> take 5
`);
```

Provider types include `http.json`, `rest`, `graphql`, `webhook`, `hl7`,
`google_api`, `github_api`, `slack_api`, `database`, `webcrawler`, `smtp`,
and more; any `http`/`https` URI is auto-connectable. Use ordinary array
methods for straightforward in-language data manipulation, and reach for
XQL when you want a declarative pipeline over data that may come from an
external integration.

### Policy Decisions (rules)

Requires the full `cmd/interpreter` build. Wraps `github.com/oarkflow/rules`
("Condition"): describe business/authorization decisions declaratively in
BCL (e.g. "does this payment need manual review?") instead of scattered
`if` statements:

```spl
import "rules" as rules;

let svc = rules.service({"environment": "dev"});

let policy = `module "access" {
  decision_schema "access" { effects [allow, deny] default deny strategy first_match }
  decision_table "access" {
    default deny
    hit_policy first
    row "allow-verified" {
      when { request.verified == true }
      then { decision allow reason "verified user" }
    }
  }
}`;

let [pub, perr] = rules.publish(svc, "access-policy", policy, {"version": "1"}); // or a file path
if (perr != null) { throw perr; }

let [result, everr] = rules.evaluate(svc, "access-policy", "access", {"request": {"verified": true}});
if (everr != null) { throw everr; }
print result.Report.Decision.Effect;   // "allow" - hash keys mirror the Go struct field names as-is
print result.Report.Decision.Allowed;  // true

rules.activate(svc, "access-policy", "1", "dev");
rules.rollback(svc, "access-policy", "1", "dev");
```

`rules.service()` requires the `policy` capability under a restrictive
security policy. This wraps only the core publish/evaluate/activate loop —
workflows, stateful chains, canary releases, and approval gates aren't
exposed yet. See
[docs/features/50](docs/features/50-policy-decisions-with-rules.md) and
`examples/rules_all_in_one.spl`.

---

## Daily-Ops & Data-Validation Plugins

### Daily Tools (Files, Archive, Images, Media, Office, Secrets, System, Network)

Every mutating operation is **preview-first**: it returns a description of
what *would* happen (`status: "planned"`) unless `{"apply": true}` is
passed, in which case it actually runs (`status: "applied"`).

```spl
import "tools/files";

let plan = bulk_rename("photos", {"match": "*.jpg", "template": "{name}_{seq}.{ext}", "apply": false});
let found = file_search("photos", {"match": "*.jpg"}); // [{path, name, size, mode, mod_time, is_dir, mime}]
let sum = file_checksum("photos/a.jpg");                 // {path, sha256, size}
file_organize("./downloads", "./downloads/by-type", {"apply": false});
file_dedupe("./photos");

let results = file_finder("photos")
    .files().ext("jpg").size(0, 1000000).sort("size", true).limit(10).exec();
```

```spl
import "tools/archive";
archive_compress("photos", "photos.zip", {"format": "zip", "apply": true});
let list = archive_list("photos.zip");  // lists WITHOUT extracting
archive_extract("photos.zip", "restored/", {"apply": true});
```

```spl
import "tools/images";  // file-to-file, no in-memory codec required
image_convert_batch("./photos", "./web", {"to": "png", "apply": true});
image_resize_file(src, dst, {"width": 200, "apply": true});
image_thumbnail(src, dst, {"size": 256, "apply": true});
```

```spl
import "tools/media";
print ffmpeg_status(); // {ffmpeg, ffmpeg_path, ffprobe, ffprobe_path, install_command}
media_convert("input.mov", "output.mp4", {"install": true, "apply": true});
```

```spl
import "tools/office";
let text = office_text("report.docx"); // .txt/.md/.log/.csv/.json/.docx/.xlsx
let doc = office_read("data.csv");     // {path, name, size, ext, rows: [[...]]}
```

```spl
import "tools/secrets";
print secret_generate(16);                                    // masked SECRET
file_encrypt(src, dst, passphrase, {"apply": true});           // scrypt-derived key
file_decrypt(src, dst, passphrase, {"apply": true});
```

```spl
import "tools/system";
print system_info(); // {os, arch, cpus, hostname, cwd, go_version}

import "tools/network";
print dns_lookup("localhost");
print tcp_check("127.0.0.1:80", 500);
print http_probe("https://example.com");
```

Every one of these has a matching `spltool` subcommand — see [Developer
Tooling](#developer-tooling).

### Email, Phone & IP Validation

All three follow the same shape for validating a field across many
records at once (a `*_bulk` builtin accepting an array of hashes/strings
or a table value, returning `{total, valid_count, invalid_count,
results}`, never aborting a batch on one bad record).

**Email**:

```spl
import "emailvalidator" as email;

print email.validate_syntax("User@Example.COM"); // syntax/normalization, no network
print email.is_disposable("test@mailinator.com");   // true
print email.is_role_account("admin+ops@example.com"); // true
print email.is_free_provider("someone@gmail.com");    // true

let [result, err] = email.validate("user@example.com", {"check_dns": false});
print result.verdict; print result.risk_score; print result.reasons;

let signups = [
    {"name": "Ada", "email": "ada@example.com"},
    {"name": "Grace", "email": "grace@mailinator.com"},
    {"name": "Linus", "email": "not-an-email"}
];
let report = email.validate_bulk(signups, "email");
print sprintf("checked %d, %d valid, %d invalid", report.total, report.valid_count, report.invalid_count);
// write_json/write_csv/database.exec straight from report.results
```

`email.validate`'s DNS check (on by default) and optional SMTP probing
require the `network` capability; syntax/disposable/role/free-provider
checks never touch the network. `email.validate_bulk` defaults DNS/SMTP
off so batches stay fast and capability-free unless requested.

**Phone**:

```spl
import "phone" as phone;

let [parsed, err] = phone.parse("(650) 253-0000", "US"); // default_region for numbers w/o "+"
print parsed; // {valid, possible, e164, international, national, country_code,
               //  region, type, carrier, network, ...}
print phone.valid("not a phone number", "US"); // false — never throws
print phone.country("AU");        // {code, name, phone, currency, currency_symbol}
print phone.networks("US", {"status": "Operational"}); // full MCC/MNC/PLMN operator table

let report = phone.parse_bulk(contacts, "phone", {"default_region": "US", "region_field": "region"});
```

**IP**:

```spl
import "ip" as ip;

print ip.is_private("10.0.0.1"); // true
print ip.client_from_header("10.0.0.1", "203.0.113.5, 10.0.0.1"); // "203.0.113.5"
print ip.client_from_header(remote, header, {"trust_proxy": false}); // ignore header entirely

let [ok, err] = ip.geo_init(); // fetches/caches a geolocation dataset once (network + write capability)
print ip.country("8.8.8.8");   // "" until ip.geo_init() has run
print ip.lookup("8.8.8.8");    // {found, country_code, country, region, city, latitude, longitude}

let report = ip.lookup_bulk(requests, "ip");
```

### Money, Natural-Language Dates & Sortable IDs

**Money** — fixed-point (integer-minor-unit) arithmetic, so results never
drift like float math would:

```spl
import "money" as money;

let [price, err] = money.new("19.99", "USD"); // STRING amount avoids float rounding
let tax = money.percent(price, 8.5);           // 8.5%, rounds half up
let [total, addErr] = money.add(price, tax);
print money.format(price); print money.format(total); // "$19.99", "$21.69"

let [tripled, mulErr] = money.mul(price, 4);   // whole-number multiplier

let [eurPrice, _] = money.new("19.99", "EUR");
let [mismatch, mismatchErr] = money.add(price, eurPrice);
print mismatchErr; // "currency mismatch" — a hard error, not silently wrong math
```

`money.new`'s amount is always interpreted as a **major-unit** value
(dollars, not cents) regardless of whether you pass a `STRING`, `INTEGER`,
or `FLOAT` — pass the `STRING` form (`"19.99"`) to avoid float rounding;
there's no separate "from minor units" constructor.

**Natural-language dates**:

```spl
import "naturaldate" as naturaldate;

let [r, err] = naturaldate.parse("tomorrow at 9am");
print r; // {time, unix, direction, truncated, has_recur}
print naturaldate.parse_all("remind me tomorrow at 9am and again next friday"); // every expression found

print naturaldate.parse("next friday", {
    "reference": "2026-01-01T00:00:00Z", "location": "America/New_York"
});
```

**Sortable IDs**:

```spl
import "wuid" as wuid;

let id = wuid.new();         // 128-bit, time-ordered, base62-encoded
print wuid.new_uuid();        // same ID, standard dashed UUID format

let [parsed, err] = wuid.parse(id); // {hex, id, uuid, unix_ms, time}
```

None of these check any capability — they're pure in-memory computations.

### CSV/JSON Type Inference

Profile an unfamiliar data source before writing a schema/import pipeline
by hand:

```spl
import "metadata" as metadata;

let [types, err] = metadata.infer_csv_types("id,name,active,joined\n1,Ada,true,2020-01-15\n2,Grace,false,2021-06-30\n");
print types; // {active: "bool", id: "int", joined: "time.Time", name: "string"}

let [jtypes, jerr] = metadata.infer_json_types([{"id": 1, "score": 9.5}, {"id": 2, "score": 10}]);
print jtypes; // {id: "int", score: "float64"}

print metadata.infer_value_type("2026-01-01"); // "time.Time"
```

---

## Config Loading & Secrets Masking

```spl
let cfg = config_load(".env", "env");
let db = config_load("config/database.yaml", "yaml"); // requires the yaml plugin
let api = config_load("api.json", "json");
```

Format is auto-detected from the extension if omitted;
`config_parse(raw, format)` parses an in-memory string instead of a file.
Dot access works for nested keys: `db.auth.username`, `db.auth.password`.

Any hash key whose name matches a sensitive pattern (`password`, `secret`,
`token`, `api_key`, `private_key`, `access_key`, `credentials`, `auth`,
...) has its string value wrapped as a `SECRET`, which always prints as
`***`:

```spl
print db.auth.password;              // ***
print secret_reveal(db.auth.password); // actual plaintext value
print secret_mask("mypassword");      // ********rd (last 2 chars visible)
```

A `config_load`/`config_parse` failure (missing file, parse error) raises
a catchable runtime error — wrap in `try/catch`.

---

## Common Workflows

These walk through combining multiple features to accomplish a realistic
task end to end.

### 1. Validate and clean up a signup CSV, then load it into a database

```spl
import "database" as database;
import "emailvalidator" as email;
import "phone" as phone;
import "metadata" as metadata;

let csvText = read_file("signups.csv");
let [types, terr] = metadata.infer_csv_types(csvText);
print types; // spot-check column types before trusting the shape

let table = read_csv("signups.csv");
let rows = table_rows(table);

let emailReport = email.validate_bulk(rows, "email");
let phoneReport = phone.parse_bulk(rows, "phone", {"default_region": "US"});

let conn, dberr = database.connect("sqlite", "signups.db");
database.exec(conn, "CREATE TABLE IF NOT EXISTS signups (name TEXT, email TEXT, phone_e164 TEXT, valid_email BOOLEAN, valid_phone BOOLEAN)");

for (i, row in emailReport.results) {
    let phoneRow = phoneReport.results[i];
    database.exec(conn, "INSERT INTO signups(name, email, phone_e164, valid_email, valid_phone) VALUES(?, ?, ?, ?, ?)",
        [row.name, row.input, phoneRow.e164, row.valid, phoneRow.valid]);
}

write_json("signups_report.json", {"emails": emailReport, "phones": phoneReport}, {"pretty": true});
```

### 2. A small JSON API with auth middleware, JWT, and in-memory state

```spl
import "server" as svr;
import "cryptoextra" as cryptoextra;
import "money" as money;

let app = svr.server(3099);
let SECRET = "change-me";
let users = {};

svr.route(app, "POST", "/api/login", function(req, res) {
    let body = req.json();
    let token = cryptoextra.jwt_encode({"sub": body.username}, SECRET, {"alg": "HS256", "expires_in": 3600});
    res.json({"token": token});
});

svr.middleware(app, "/api", function(req, res, next) {
    if (req.path == "/api/login") { next(); return; }
    let auth = req.get_header("Authorization");
    if (auth == null) {
        res.status(401).json({"error": "missing token"});
        return;
    }
    let claims = try { cryptoextra.jwt_decode(auth, SECRET); } catch (e) { null; };
    if (claims == null) {
        res.status(401).json({"error": "invalid token"});
        return;
    }
    next();
});

svr.route(app, "POST", "/api/orders", function(req, res) {
    let body = req.json();
    let [price, perr] = money.new(body.amount, body.currency);
    if (perr != null) { res.status(400).json({"error": perr}); return; }
    let tax = money.percent(price, 8.5);
    let [total, aerr] = money.add(price, tax);
    res.json({"subtotal": money.format(price), "tax": money.format(tax), "total": money.format(total)});
});

svr.listen(app, 3099);
```

### 3. A scheduled data pipeline with reactive status and a live dashboard

```spl
import "database" as database;
import "server" as svr;

let conn, dberr = database.connect("sqlite", "pipeline.db");

let lastRun = signal("lastRun", null);
let recordsProcessed = signal("recordsProcessed", 0);
let status = computed(function() {
    return recordsProcessed.value > 0 ? "healthy" : "idle";
});

schedule_interval("5m", "sync", function() {
    let rows, err = database.query(conn, "pending_items").where("processed", false).limit(100).exec();
    if (err != null) { return; }
    for (row in rows) {
        database.exec(conn, "UPDATE pending_items SET processed = ? WHERE id = ?", [true, row.id]);
    }
    recordsProcessed.set(function(prev) { return prev + len(rows); });
    lastRun.set(now_iso());
});

let app = svr.server(3100);
svr.route(app, "GET", "/status", function(req, res) {
    let sse = res.sse();
    sse.send("status", json_encode({"status": status.value, "processed": recordsProcessed.value, "lastRun": lastRun.value}));
    sse.close();
});
svr.listen(app, 3100);
```

### 4. Generating a signed, encrypted report as a PDF

```spl
import "database" as database;
import "money" as money;
import "pdf" as pdf;

let conn, err = database.connect("postgres", "postgres://user:pass@localhost/reports");
let rows, qerr = database.query(conn, "monthly_totals").order_by("month DESC").limit(12).exec();

let body = "# Monthly Report\n\n";
for (row in rows) {
    // row.total is a decimal-string column (e.g. "1999.00") — pass amounts
    // as STRING to money.new to avoid float rounding; an INTEGER/FLOAT
    // amount is treated as a major-unit value, not minor units (cents).
    let [amount, aerr] = money.new(row.total, "USD");
    body += sprintf("- %s: %s\n", row.month, money.format(amount));
}

pdf.from_markdown(body, "report.pdf", {"title": "Monthly Report", "theme": "modern", "toc": false});
pdf.watermark("report.pdf", "report_wm.pdf", "CONFIDENTIAL", {"opacity": 0.2});
pdf.protect("report_wm.pdf", "report_final.pdf", "reader-pw", "owner-pw", "aes-128");
```

### 5. Running third-party scripts safely (multi-tenant / plugin scripts)

```bash
interpreter --profile untrusted \
    --allow-cap filesystem_read \
    --allow-cap network --allow-network api.example.com \
    --require-os-isolation \
    tenant_script.spl
```

```go
result, err := interpreter.ExecFileWithOptions("tenant_script.spl", nil, interpreter.ExecOptions{
    Profile: "untrusted",
    Security: &interpreter.SecurityPolicy{
        AllowedCapabilities: []string{"filesystem_read", "network"},
    },
    Timeout: 5 * time.Second,
})
var execErr *interpreter.ExecError
if errors.As(err, &execErr) && execErr.Kind == interpreter.ExecErrorPolicyDenied {
    log.Printf("tenant script denied: %s", execErr.Message)
}
```

The script itself never needs to know it's sandboxed — a denied
filesystem write, network call, or `exec` simply raises a catchable
error, which the script can handle with ordinary `try/catch` (see
[Security & Sandboxed Execution](#security--sandboxed-execution)).

### 6. Splitting and safely re-combining a master credential

```spl
import "shamir" as shamir;

let masterKey = secret_generate(32);
let [split, err] = shamir.split(secret_reveal(masterKey), 3, 5); // any 3 of 5 shares reconstruct

// Distribute split.shares[0..4] to 5 separate holders, and split.auth_key
// to a 6th party (or store it separately) — no single share (or the auth
// key alone) reveals anything about the secret.

let [recovered, cerr] = shamir.combine(
    [split.shares[0], split.shares[2], split.shares[4]],
    split.auth_key
);
print recovered == secret_reveal(masterKey); // true
```

---

## Developer Tooling

`spltool` is the standalone developer CLI: formatting, static checks,
package management, testing, daily-tools subcommands, IDE-support JSON
surfaces, and a JSON-RPC LSP server.

```bash
spltool fmt [-w] script.spl               # format
spltool check [-json] script.spl          # parser diagnostics + static analysis

spltool mod init example/app              # spl.mod / spl.lock workflow
spltool mod tidy
spltool mod verify

spltool config init                        # spl.config.json
spltool config show

spltool symbols [--json] script.spl
spltool complete --prefix pri script.spl
spltool hover --line 1 --col 1 script.spl
spltool docs script.spl

spltool test [-json] [-filter <substr>] [-profile trusted|untrusted] [targets...]
spltool conformance                        # canonical language-compatibility corpus

spltool session run --json --checkpoint baseline script.spl
spltool session debug --json script.spl

spltool files rename ./photos --match '*.jpg' --template '{date}_{seq}.{ext}'
spltool files organize ./downloads ./downloads/by-type --apply
spltool archive compress ./docs backup.zip --format zip --apply
spltool image convert ./photos ./web --to png --apply
spltool office read ./data/people.csv --json
spltool media ffmpeg-status
spltool secrets generate --length 24 [--token]

spltool lsp --stdio                        # full JSON-RPC 2.0 LSP server
```

`check` flags undefined identifiers, suspicious shadowing, unreachable
statements, missing/incorrect imports, deprecated builtins, and
non-exhaustive ADT matches. Every mutating daily-tools subcommand defaults
to a dry-run preview — add `--apply` to actually perform the operation.

`spl.config.json` (via `spltool config init`) sets project defaults for
runtime limits, the security profile, and which static checks are active,
discovered by walking up parent directories from wherever `spltool` runs.

### VS Code Extension

Contributes SPL syntax highlighting, code snippets, and commands
(`spl.runFile`, `spl.evaluateSelection`, session checkpoint/restore/
inspect, daily-tools helpers) backed by the same LSP server. Point its
`spl.toolPath` setting at a `cmd/spltool-full` build if your scripts use
any optional plugin builtin — otherwise the language server won't
recognize plugin functions in completion/diagnostics (they still run fine
at runtime via `cmd/interpreter`).

---

## The REPL

```bash
interpreter
```

```text
>> let x = 40;
>> x + 2
42
>> :vars
  x = 40
>> :type x
INTEGER
>> :checkpoint base
checkpoint saved: base
>> x = 99
>> :restore base
restored: base
>> x
40
```

Selected meta-commands:

| Command | Purpose |
|---|---|
| `:help` / `:tips` / `:commands [query]` | keybindings, tips, full command table |
| `:palette <query>` | fuzzy search across commands, builtins, variables, symbols |
| `:examples` / `:tools` | runnable example scripts / daily-tools modules |
| `!<shell command>` | run a shell command |
| `:vars` / `:reset` | list variables / clear session |
| `:checkpoint [name]` / `:restore <name>` / `:replay` | session snapshot workflow |
| `:inspect [name]` / `:metrics` / `:events` | inspect state / execution metrics / event log |
| `:type <expr>` / `:doc <name\|expr>` / `:methods <expr>` / `:fields <expr>` | inline documentation & introspection |
| `:diagnostics [source]` / `:symbols [query]` / `:def <name>` / `:refs <name>` | static analysis & navigation |
| `:format [source]` / `:ast <expr>` | formatting & AST dump |
| `:time <expr>` | evaluate and print elapsed wall time |
| `:debug <expr>` | interactive step-debugger (`step`, `locals`, `break N`, `continue`, `quit`) |
| `:load <file>` / `:reload [file]` | evaluate a file / invalidate module cache |
| `:rename` / `:move` | preview/apply bulk file operations |
| `:install <alias> <path>` | add a dependency to `spl.mod` |
| `:config ...` | runtime/security config viewer/editor |

`:config profile untrusted` applies hardened defaults directly in the
REPL: strict mode, host protection, and denial of `async, db, env_write,
exec, filesystem_write, network, policy, process_exit, scheduler, server,
watch`.

Every REPL execution runs inside a session that supports named
checkpoints, replay of recorded input, an event/metrics log, and
cancellation — the same session model the embedding API exposes to Go
hosts (below). History persists across restarts; tab completion is
semantic (after a `.`, it evaluates the base expression and offers that
value's real fields/methods); multiline input is auto-detected.

Secure config example:

```text
>> :config .env env
CONFIG loaded
>> CONFIG.DB_HOST
localhost
>> CONFIG.DB_PASSWORD
***
```

---

## Embedding SPL in a Go Program

```go
import "github.com/oarkflow/interpreter"

result, err := interpreter.Exec("let x = 40; let y = 2; x + y;", nil)
```

`Exec`/`ExecFile` accept a `data map[string]interface{}` injected into the
script's global scope (Go values are converted to SPL objects via
reflection).

### Options, limits & cancellation

```go
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()

result, err := interpreter.ExecWithOptions(
    "let x = 40; let y = 2; x + y;", nil,
    interpreter.ExecOptions{
        Context: ctx, MaxSteps: 1_000_000, MaxDepth: 200, MaxHeapMB: 128,
    },
)
```

`ExecOptions` covers `Profile` (`""`/`trusted`, `untrusted`, `readonly`,
`networked`, `data-processing`, `automation`, `server`), every runtime
limit (`MaxDepth`, `MaxSteps`, `MaxHeapMB`, `MaxOutputBytes`,
`MaxHTTPBodyBytes`, `MaxExecOutputBytes`, `MaxStringBytes`,
`MaxArrayLength`, `MaxHashEntries`, `MaxImportDepth`, `MaxImportCount`),
`Timeout`/`Context` for cancellation, `Output` (an `io.Writer` for printed
output), and explicit `Security`/`Sandbox` overrides. Setting `Profile` to
anything other than `""`/`"trusted"` routes execution through the
untrusted worker subprocess path.

```go
var execErr *interpreter.ExecError
if errors.As(err, &execErr) {
    switch execErr.Kind {
    case interpreter.ExecErrorPolicyDenied:   // a security policy denied an operation
    case interpreter.ExecErrorResourceLimit:  // a step/depth/heap/output limit was hit
    case interpreter.ExecErrorTimeout:        // the time budget elapsed
    case interpreter.ExecErrorCancelled:      // the caller's context was cancelled
    }
}
```

### `Runtime` — for hosts executing many scripts

```go
rt, err := interpreter.NewRuntime(interpreter.RuntimeOptions{
    Profile: "readonly", ModuleDir: "./scripts", MaxSteps: 500_000,
    Observability: &interpreter.ObservabilityHooks{
        OnFinish: func(m interpreter.ExecutionMetrics) {
            log.Printf("script=%s duration=%s err=%s", m.Path, m.Duration, m.Error)
        },
        OnPolicyDenied: func(category, detail string) {
            metrics.IncrCounter("spl.policy_denied", map[string]string{"category": category})
        },
    },
})
result, err := rt.ExecFile("scripts/job.spl", nil)
```

Passing only `Profile` is enough to derive a full capability preset if
`Security`/`Sandbox` aren't set explicitly.

### Sessions (checkpoints, replay, cancellation)

```go
sess, _ := rt.NewSession(interpreter.SessionOptions{ID: "workspace"})
res := sess.Execute(interpreter.ExecutionRequest{Source: `let x = 40; x + 2;`})
snap, _ := sess.Checkpoint("baseline")

go func() {
    res := sess.Execute(interpreter.ExecutionRequest{Source: `while (true) { 1 + 1; }`})
    fmt.Println(res.Metrics.ErrorKind) // ErrorKindCancelled
}()
time.Sleep(500 * time.Millisecond)
sess.Cancel() // stops the in-flight execution above
```

### Registering custom builtins & modules

```go
plugin := interpreter.PluginFunc{
    PluginName: "example",
    Fn: func(rt *interpreter.Runtime) error {
        interpreter.RegisterRuntimeBuiltins(map[string]*object.Builtin{
            "answer": {Fn: func(args ...object.Object) object.Object {
                return &object.Integer{Value: 42}
            }},
        })
        return interpreter.RegisterStdModule("std/example", map[string]interpreter.Object{
            "name": &interpreter.String{Value: "example"},
        })
    },
}
rt, _ := interpreter.NewRuntime(interpreter.RuntimeOptions{Plugins: []interpreter.Plugin{plugin}})
```

### Performance: pooled & sealed environments

```go
env := interpreter.NewPooledEnvironment()
defer interpreter.ReleasePooledEnvironment(env)
result := interpreter.Eval(program, env)
```

For prepared, expression-only workloads whose bindings never change after
setup:

```go
env := interpreter.NewEnvironment()
interpreter.InjectData(env, data)
env.SealBindings()          // lock-free reads, native short-circuit rule eval
result := interpreter.Eval(program, env)
```

Don't seal an environment used by scripts that declare or assign
variables — call `env.Reset()` to unseal and make it writable again.

---

## Security & Sandboxed Execution

### Execution profiles

- **`trusted`** (default): unrestricted beyond the sandbox's own default
  resource limits — preserves ordinary CLI/embedding behavior.
- **`untrusted`**: strict mode + host protection, only `filesystem_read`
  allowed (rooted at the script's directory), and tighter limits
  (`MaxDepth=128, MaxSteps=500,000, MaxHeapMB=64, output caps=64KiB,
  Timeout=2s`).

```bash
interpreter --profile untrusted script.spl
```

Under `untrusted`, host-mutating operations are denied by default:

```spl
let ok, err = write_file("hostile.txt", "pwned");
print err; // capability denied by host protection policy: filesystem_write

let r, e = http_get("http://example.com");
print e; // network policy denied request: capability denied by host protection policy: network
```

### CLI allow-list flags

```text
--profile trusted|untrusted
--require-os-isolation
--allow-in-process-fallback
--allow-cap <csv>              # capability allowlist, e.g. "network,db"
--allow-exec <csv>
--allow-network <csv>          # network HOST allowlist
--allow-db-driver <csv>
--allow-db-dsn <csv>
--allow-read <csv>
--allow-write <csv>
--allow-import-path <csv> / --deny-import-path <csv>
--allow-import-package <csv> / --deny-import-package <csv>
--deny-dynamic-imports
```

> **Important**: `--allow-network <hosts>` alone does **not** grant
> network access under `--profile untrusted` — it only populates the host
> allowlist. The `network` **capability** must also be granted, either via
> `--allow-cap network` or a preset that already includes it:
>
> ```bash
> # still denied — capability not granted:
> interpreter --profile untrusted --allow-network example.com script.spl
> # works — capability AND host both granted:
> interpreter --profile untrusted --allow-cap network --allow-network example.com script.spl
> ```

`--require-os-isolation` (Linux only, via `bubblewrap`) additionally wraps
execution in a fresh network/PID/UTS/IPC namespace with the module
directory bind-mounted read-only; it fails closed (rather than silently
degrading) if `bwrap` isn't available.

### Environment variables

| Env var | Effect |
|---|---|
| `SPL_SECURITY_MODE=strict` | default-deny for file/network/db/exec unless explicitly allowed |
| `SPL_PROTECT_HOST=1` | disables host-mutating capabilities (`exec`, `write_file`, `remove_file`, `os_env(key,value)`, `exit()`) |
| `SPL_ALLOW_ENV_WRITE` | controls whether `os_env(key, value)` can mutate env vars |
| `SPL_EXEC_ALLOW_CMDS` / `SPL_EXEC_DENY_CMDS` | exec command allow/deny lists |
| `SPL_NETWORK_ALLOW` / `SPL_NETWORK_DENY` | network host allow/deny lists |
| `SPL_DB_ALLOW_DRIVERS` / `SPL_DB_DENY_DRIVERS`, `SPL_DB_DSN_ALLOW` / `SPL_DB_DSN_DENY` | DB driver/DSN allow/deny lists |
| `SPL_FILE_READ_ALLOW` / `SPL_FILE_READ_DENY`, `SPL_FILE_WRITE_ALLOW` / `SPL_FILE_WRITE_DENY` | file access allow/deny lists |
| `SPL_IMPORT_PATH_ALLOW` / `_DENY`, `SPL_IMPORT_PACKAGE_ALLOW` / `_DENY`, `SPL_IMPORT_DENY_DYNAMIC` | import restrictions |
| `SPL_BLOCK_HARDCODED_SECRETS` | rejects scripts whose source looks like a hardcoded credential |
| `SPL_MODULE_PATH` | extra module lookup directories |
| `SPL_DISABLE_EXEC` | globally disables `exec(...)` |
| `SPL_HTTP_MAX_BODY_BYTES` | caps HTTP integration response body size |

> A normal script run under the default `trusted` profile always
> constructs its own permissive sandbox policy before evaluating, so bare
> env vars like `SPL_PROTECT_HOST=1`/`SPL_SECURITY_MODE=strict` are **not**
> guaranteed to restrict execution on that path. For guaranteed
> enforcement, use `--profile untrusted` or pass an explicit
> `ExecOptions.Security`/`SecurityPolicy` from Go.

### In-script policy adjustment

```spl
permissions({"strict": true, "allow_exec": ["echo"], "deny_http": ["*"]});
```

Requires the `policy` capability (and is itself denied under
`ProtectHost`) — treat this as a coarse, best-effort adjustment rather
than a hard security boundary; prefer CLI flags or `ExecOptions.Security`
for guarantees that must hold regardless of script content.

### Capability model

Capabilities: `async, db, env_read, env_write, exec, filesystem_read,
filesystem_write, network, policy, process_exit, scheduler, server,
secrets, system, watch`. Under host protection, every capability except
`filesystem_read` is denied unless explicitly allow-listed.
`SPL_`-prefixed and dynamic-linker environment variables (`PATH`,
`LD_PRELOAD`, ...) can never be mutated regardless of policy.

### Capability presets (for embedding)

| Preset | Grants (on top of the `untrusted` baseline where noted) |
|---|---|
| `trusted` | unrestricted (default) |
| `untrusted` / `readonly` | only `filesystem_read`, tight resource limits |
| `networked` | `untrusted` + `network` |
| `data-processing` | `untrusted` + `db` |
| `automation` | `untrusted` + `exec, filesystem_write, system` |
| `server` | `untrusted` + `server, network` |

---

## The Browser Playground

```bash
interpreter --playground
# open http://localhost:8080
```

```bash
PLAYGROUND_AUTH_SECRET=dev-secret interpreter --playground   # require sign-in
```

The playground runs submitted code through bounded evaluation, captures
printed output, typed results, diagnostics, and renderable artifacts
(files/images/tables), and ships with 40+ built-in examples spanning
language basics, modules, data values, runtime/session introspection,
resource limits, production-profile behavior, stateful servers,
middleware, scheduling, SSE, reactive state/HTML, and every optional
plugin (database query builder, email/phone/IP validation, money, natural
dates, sortable IDs, Shamir secret sharing, type inference).

### API

```text
GET  /api/health
GET  /api/ready
GET  /api/session
POST /api/login
POST /api/logout
GET  /api/examples
POST /api/execute
GET  /metrics       (Prometheus text format)
```

```bash
curl -X POST http://127.0.0.1:8080/api/execute \
    -H "Content-Type: application/json" \
    -d '{"code":"print \"hello\";"}'
# {"output":"hello\n","result":"null","result_type":"NULL","error":"", ...}
```

Sessions are opaque server-side tokens (checked via a cookie or
`Authorization: Bearer <token>` header, timing-safe compared); if
`PLAYGROUND_AUTH_SECRET` is unset, every endpoint is open — always set it
for any deployment reachable outside a trusted local network.

### Configuration (environment variables)

| Var | Default | Purpose |
|---|---|---|
| `PLAYGROUND_ADDR` | `:8080` | listen address |
| `PLAYGROUND_AUTH_SECRET` | unset (auth disabled) | shared login secret |
| `PLAYGROUND_EXECUTION_PROFILE` | `untrusted` | `trusted` or `untrusted` |
| `PLAYGROUND_MAX_BODY_BYTES` | `1048576` | max request body |
| `PLAYGROUND_RATE_LIMIT` / `_RATE_WINDOW_MS` | `60` / `60000` | rate limiting |
| `PLAYGROUND_COOKIE_SECURE` | `false` | force `Secure` cookie flag |
| `PLAYGROUND_SESSION_TTL_MS` | `43200000` (12h) | session lifetime |
| `PLAYGROUND_TRUST_PROXY_HEADERS` | `false` | honor `X-Forwarded-For`/`X-Real-IP` |
| `PLAYGROUND_EVAL_MAX_DEPTH` / `_MAX_STEPS` / `_MAX_HEAP_MB` / `_TIMEOUT_MS` | `200`/`2000000`/`256`/`8000` | per-script eval limits |
| `PLAYGROUND_RENDER_MODE` / `_ALLOW_URLS` / `_ALLOW_URL_HOSTS` / `_MAX_BYTES` | `auto`/`false`/`""`/`1048576` | artifact rendering |

CLI flags (`--render-allow-urls`, `--render-url-hosts`, `--render-mode`,
`--profile`) override env vars for local runs. The playground's default
profile grants `db` and `network` capabilities in addition to the
`untrusted` baseline, so its examples can exercise the database and
integration plugins; `listen`/`listen_async` still require both `server`
and `network`, so example scripts avoid actually opening sockets.

Put the playground behind a TLS-terminating reverse proxy for any
non-local deployment; set `PLAYGROUND_COOKIE_SECURE=true` once TLS is
terminated, and only set `PLAYGROUND_TRUST_PROXY_HEADERS=true` if that
proxy is trusted to strip client-supplied `X-Forwarded-*` headers first.
