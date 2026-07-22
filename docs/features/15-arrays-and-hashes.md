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
| `.first()` / `.last()` | no | first or last element, or `null` |
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
| `.pluck([fields...])` | no | no fields: copy; otherwise: projected hashes |
| `.column(field)` / `.values_of(field)` | no | extract one field as scalar values |
| `.only(fields...)` / `.select(fields...)` | no | keep fields in each hash element |
| `.except(fields...)` / `.omit(fields...)` | no | remove fields from each hash element |
| `.where(field, value)` | no | hashes whose field equals value |
| `.where_in(field, values)` | no | hashes whose field is in an array or variadic values |
| `.first_where(field, value)` | no | first matching hash, or `null` |
| `.group_by(field)` / `.key_by(field)` | no | group or index hashes by a field |
| `.sort_by(field[, direction])` | no | stable field sort; direction is `"asc"` or `"desc"` |
| `.unique_by(field)` | no | first hash for each unique field value |
| `.compact()` | no | remove `null` elements |
| `.take(n)` / `.drop(n)` | no | take or drop from either end (negative `n` uses the end) |
| `.chunk(size)` | no | split into arrays of at most `size` elements |
| `.sum([field])` / `.avg([field])` | no | aggregate numbers or a numeric hash field |

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

Record-collection methods are chainable and accept dotted field paths:

```spl
let orders = [
  { id: "A-17", total: 680, customer: { region: "west" } },
  { id: "B-42", total: 1400, customer: { region: "east" } },
  { id: "C-08", total: 2200, customer: { region: "east" } }
];

let large = orders.where("customer.region", "east").sort_by("total", "desc");
print large.pluck("id");            // [{id:"C-08"}, {id:"B-42"}]
print large.pluck("id", "total");  // projected records
print large.column("id");           // ["C-08", "B-42"]
print large.except("id");           // records without id
print large.pluck();                 // non-mutating shallow copy
```

`pluck` always preserves record shape when fields are supplied. Use `column`
or `values_of` (`valuesOf`) when only scalar field values are wanted. With no
fields, `pluck` returns a shallow copy of the array. These operations do not
mutate the source collection. Camel-case aliases are available for compound
names: `whereIn`, `firstWhere`, `groupBy`, `keyBy`, `sortBy`, and `uniqueBy`.

## Hash methods

```spl
let h = {"a": 1, "b": 2};
print h.keys();    // ["a", "b"]
print h.values();  // [1, 2]
print h.entries(); // [["a",1], ["b",2]] (order not guaranteed)
print h.length;    // 2
print h.only("a");
print h.except("b");
print h.has("a");
print h.get("missing", "fallback");
```

Dot access first checks for a literal key match (`h.a`), then falls back to
these built-in methods if no such key exists. Hash selection methods support
dotted paths and also provide `pick`/`omit` aliases.

## Free collection builtins

A large complementary set of collection builtins (`first`, `last`, `rest`,
`sum`, `avg`, `group_by`, `merge`, `has_key`, `get`, `zip`, `chunk`,
`clamp`, ...) is documented in doc 21 (Collection Builtins) — those are
called as plain functions (`sum(arr)`) rather than dot methods.

## Destructuring

Array/hash destructuring in `let`, function parameters, and `match` patterns
is covered in docs 03 and 07.
