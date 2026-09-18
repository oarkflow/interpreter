# 18 — Object Model: Ownership & Immutability

Source: `pkg/builtins/concurrency.go` (`move`, `ImmutableValue`),
`pkg/object/object.go` (`OwnedValue`, `ImmutableValue`).

These are two experimental, Rust-inspired wrapper types layered over
ordinary arrays/hashes.

## `immutable(value)` — deep-frozen values

```spl
let frozen = immutable({"a": 1});
let result = try {
    frozen.a = 2;
    "unexpected";
} catch (e) {
    e;
};
print result; // cannot mutate immutable value
```

Wrapping a hash/array with `immutable(...)` makes subsequent **mutation**
attempts raise a catchable runtime error instead of silently succeeding —
useful for defensively freezing shared configuration or constant data
structures passed into other scopes.

Reading through a frozen value works transparently: `frozen.a`, `frozen["a"]`,
and iteration all proxy to the wrapped value, and nested arrays/hashes stay
individually frozen (mutating `frozen.nested.x` fails the same way mutating
`frozen.a` does). A prior build had a bug here — an internal duplicate of the
`ImmutableValue` type meant reads could panic with a type-assertion mismatch
or return the wrong error — this has been fixed and is covered by
`TestImmutableValuesDoNotPanicAndAreGuarded` in `pkg/eval/language_gaps_test.go`.

## `move(value)` — ownership marker

```spl
let ownedData = move([1, 2, 3]);
print ownedData;       // [1, 2, 3]
print typeof ownedData; // "array"
```

`move(...)` wraps a value as an `OwnedValue`, an ownership-tracking marker
inspired by Rust's move semantics. Reading and printing an owned value works
transparently (`typeof`/iteration/print all see through to the inner value)
— it's primarily a documentation/intent marker in the current implementation
rather than an enforced single-owner discipline (there's no compiler pass
that rejects using a value after it's been "moved" elsewhere).

## When to use these

- Reach for `move(...)` purely as an in-code signal that a value is meant to
  be handed off and not mutated further by the original owner, when
  reviewing or documenting data flow (e.g. handing a large collection into a
  background job via `go(...)`/channels).
- Reach for `immutable(...)` when you want a hash/array's *write* path to
  fail loudly if some downstream code accidentally tries to mutate shared
  data — but keep your own reference to the pre-frozen value if you'll need
  to read it later, given the read-path caveat above.
