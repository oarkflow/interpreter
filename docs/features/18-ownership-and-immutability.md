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
print result; // cannot set property on HASH
```

Wrapping a hash/array with `immutable(...)` makes subsequent **mutation**
attempts raise a catchable runtime error instead of silently succeeding —
useful for defensively freezing shared configuration or constant data
structures passed into other scopes.

> **Caveat observed while verifying this feature**: reading a property back
> out of a frozen value (`frozen.a`, `frozen["a"]`, `frozen.keys()`) is
> unreliable in the current build — it may return `null`/an error, or panic
> with an internal type-assertion mismatch, rather than transparently
> proxying the read to the wrapped value. Treat `immutable(...)` as a
> write-guard for values you pass onward and then don't need to read back
> through the wrapper yourself (read the original hash/array before
> freezing it, or take a defensive copy first) until this is hardened.

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
