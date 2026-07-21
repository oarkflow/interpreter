# 21 — Collection Builtins

Source: `pkg/builtins/collections.go`, `pkg/builtins/enhancements.go`. These
are free functions (`fn(arr)`), complementary to the array/hash dot-methods
in doc 15.

## Basics

```spl
print first([1,2,3]);   // 1
print last([1,2,3]);    // 3
print rest([1,2,3]);    // [2,3]
print reverse([1,2,3]); // [3,2,1]
print slice([1,2,3,4,5], 1, 3); // [2,3]
print compact([1, null, 2, null]); // [1,2] — drops nulls
print flatten([[1,2],[3,4]]);      // [1,2,3,4] — one level
```

## Aggregation

```spl
print sum([1,2,3]); // 6
print avg([1,2,3]); // 2
print any([false, false, true]); // true — truthy check, no callback
print all([true, true]);          // true
```

## Statistics (`enhancements.go`)

```spl
print mean([1,2,3,4]);
print median([1,2,3,4]);
print mode([1,1,2,3]);
print variance([1,2,3,4]);
print stddev([1,2,3,4]);
print percentile([1,2,3,4,5], 90);
```

## Hash helpers

```spl
print values({"a":1,"b":2});           // [1,2]
print has_key({"a":1}, "a");           // true
print get({"a":1}, "b", "fallback");   // "fallback"
print merge({"a":1}, {"b":2});         // {a:1, b:2}
print delete_key({"a":1,"b":2}, "a");  // {b:2}
print entries({"a":1});                // [["a",1]]
print from_entries([["a",1],["b",2]]); // {a:1, b:2}
print pick({"a":1,"b":2,"c":3}, ["a","c"]); // {a:1, c:3}
print omit({"a":1,"b":2,"c":3}, ["b"]);     // {a:1, c:3}
```

> `default(value, fallback)` also exists as a builtin, but `default` is a
> reserved keyword (used by `switch`/`match`), so it **cannot be called by
> its bare name** as an ordinary function call expression. Use
> `coalesce(...)` instead for the common "null fallback" case.

```spl
print coalesce(null, null, "x"); // "x" — first non-null argument
```

## Grouping / partitioning

```spl
print group_by(
  [{"k":"a","v":1},{"k":"b","v":2},{"k":"a","v":3}], "k"
);
// {a: [{k:a,v:1},{k:a,v:3}], b: [{k:b,v:2}]}

print partition([{"k":"a"},{"k":"b"},{"k":"a"}], "k", "a");
// [[matching "k"=="a"...], [everything else]]

print zip([1,2], [3,4]);        // [[1,3],[2,4]]
print chunk([1,2,3,4,5], 2);    // [[1,2],[3,4],[5]]
```

## Sorting / dedup / selection helpers

```spl
print unique([1,1,2,3,3]);       // [1,2,3] — generalized dedupe
print sort_by([{"n":2},{"n":1}], "n"); // sorted by field
print take([1,2,3,4], 2);         // [1,2]
print drop([1,2,3,4], 2);         // [3,4]
print pluck([{"n":1},{"n":2}], "n"); // [1,2]
print index_by([{"id":"a"},{"id":"b"}], "id"); // {a:{...}, b:{...}}
print deep_equal({"a":1}, {"a":1}); // true
```

## Numeric helpers

```spl
print clamp(15, 0, 10);  // 10
print round(3.6); print floor(3.6); print ceil(3.2); // 4, 3, 4
print is_even(4); print is_odd(3); // true, true
print random_range(1, 10);
```

See doc 23 (Math Builtins) for the full trig/log/rounding library
(`cbrt`, `mod`, `sign`, `trunc`, `round_to`, `lerp`, `normalize`,
`map_range`, `percent`, `factorial`, `gcd`, `lcm`, `is_prime`, ...) and doc
22 (Time & Date Builtins) for time-specific helpers layered on this file
(`parse_duration`, `format_duration`, `start_of_month`, `weekday`, ...).
