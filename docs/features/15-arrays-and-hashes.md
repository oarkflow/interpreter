# 15 — Arrays & Hashes

Source: `pkg/eval/properties.go` (per-type method dispatch), `pkg/object/object.go`
(`Array`, `Hash`).

## Literals

```spl
let arr = [1, 2, 3];
let nested = [[1, 2], [3, 4]];
let hash = {"name": "SPL", "count": 3};
```

### Object literal shorthand, computed keys, and spread

```spl
let name = "x";
let obj = { name };                 // shorthand for {"name": name}
let dyn = { [ "c" + "" ]: 3 };      // computed key
let merged = { ...obj, extra: 1 };  // object spread
```

### Array spread

```spl
let a = [1, 2, 3];
let b = [0, ...a, 4]; // [0, 1, 2, 3, 4]
```

## Access

```spl
arr[0]
hash["name"]
hash.name        // dot access
person?.address?.city  // optional chaining, short-circuits to null
```

## Array methods

All array methods are called with dot syntax: `arr.method(...)`.

| Method | Mutates? | Description |
|---|---|---|
| `.length` | — | element count (property, not a call) |
| `.map(fn)` | no | transform each element |
| `.filter(fn)` | no | keep elements where `fn` is truthy |
| `.forEach(fn)` | no | side-effect iteration, returns `null` |
| `.find(fn)` | no | first element where `fn` is truthy |
| `.every(fn)` | no | true if `fn` is truthy for all elements |
| `.some(fn)` | no | true if `fn` is truthy for any element |
| `.reduce(fn[, init])` | no | fold to a single value |
| `.indexOf(x)` | no | first index of `x`, or `-1` |
| `.includes(x)` | no | membership test |
| `.join([sep])` | no | string-join elements |
| `.flat()` | no | flattens one level of nested arrays |
| `.flatMap(fn)` | no | map then flatten one level |
| `.reverse()` | **no** | returns a new reversed array |
| `.slice(start[, end])` | no | sub-array |
| `.sort()` | **no** | returns a new sorted array (homogeneous int/string) |
| `.push(v)` | **yes** | appends in place, returns new length |
| `.pop()` | yes | removes/returns the last element |
| `.shift()` | yes | removes/returns the first element |
| `.unshift(v)` | yes | prepends in place |
| `.at(i)` | no | element at index, negative indices count from the end |

```spl
let arr = [5, 3, 1, 4, 2];
print arr.sort();              // [1,2,3,4,5] (new array; arr unchanged)
print arr;                     // [5,3,1,4,2] — still original
print arr.reverse();           // [2,4,1,3,5] (reverse of original, new array)
print arr.slice(1, 3);         // [3,1]
print arr.every(x => x > 0);   // true
print arr.some(x => x > 4);    // true
print arr.reduce((a,b)=>a+b,0);// 15
print arr.join("-");           // 5-3-1-4-2
print arr.at(-1);              // 2 (last element)

let arr2 = [1, 2, 3];
arr2.push(4);
print arr2; // [1,2,3,4] — push mutates in place
```

There's also a free `push(arr, v)` **builtin** (doc 20) distinct from the
`.push()` method — the builtin form is a pure function returning a new
array rather than mutating.

## Hash methods

```spl
let h = {"a": 1, "b": 2};
print h.keys();    // ["a", "b"]
print h.values();  // [1, 2]
print h.entries(); // [["a",1], ["b",2]] (order not guaranteed)
print h.length;    // 2
```

Dot access first checks for a literal key match (`h.a`), then falls back to
these built-in methods if no such key exists.

## Free collection builtins

A large complementary set of collection builtins (`first`, `last`, `rest`,
`sum`, `avg`, `group_by`, `merge`, `has_key`, `get`, `zip`, `chunk`,
`clamp`, ...) is documented in doc 21 (Collection Builtins) — those are
called as plain functions (`sum(arr)`) rather than dot methods.

## Destructuring

Array/hash destructuring in `let`, function parameters, and `match` patterns
is covered in docs 03 and 07.
