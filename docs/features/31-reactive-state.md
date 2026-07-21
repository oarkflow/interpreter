# 31 — Reactive State

Source: `pkg/builtins/reactive` (`pkg/builtins/reactive/reactive.go`). Linked
into every interpreter binary (not an optional plugin). This is a
server-side/script-level reactive primitive — distinct from the separate
HTML template engine's `@signal`/`@effect` **directives** (doc 38), though
conceptually similar (auto-tracked dependencies, re-run on change).

## Signals

```spl
let count = signal("count", 0);       // named signal with initial value
let anon = signal(0);                  // unnamed form also works
print count.value;                     // 0 (read; also tracks the current effect/computed)
count.set(5);                          // direct set
count.set(function(prev) { return prev + 1; }); // updater form
print count.get();                     // 6
```

## Computed values

```spl
let multiplier = signal("multiplier", 3);
let tripled = computed(function() { return count.value * multiplier.value; });
print tripled.value; // auto-recomputes from its tracked dependencies
```

`computed(fn)` automatically tracks every signal read inside `fn` (via a
dependency-tracking mechanism active while `fn` runs) and recomputes lazily
when any dependency changes.

## Effects

```spl
let log = effect(function() {
    print sprintf("count=%d multiplier=%d tripled=%d",
        count.value, multiplier.value, tripled.value);
});
count.set(5);       // effect re-runs automatically
multiplier.set(10); // effect re-runs again
```

`effect(fn)` runs `fn` immediately (to establish its dependency set) and
re-runs it every time a tracked signal/computed changes. `log.dispose()`
stops it from re-running.

## Full example

```spl
let count = signal("count", 0);
let multiplier = signal("multiplier", 3);
let tripled = computed(function() { return count.value * multiplier.value; });
let log = effect(function() {
    print sprintf("count=%d multiplier=%d tripled=%d",
        count.value, multiplier.value, tripled.value);
});
count.set(5);
multiplier.set(10);
count.set(function(prev) { return prev + 1; });
print count.get(); // 6
```

Output:

```text
count=0 multiplier=3 tripled=0
count=5 multiplier=3 tripled=15
count=5 multiplier=10 tripled=50
count=6 multiplier=10 tripled=60
6
```

Each individual `.set()` call is observed to trigger dependent effects
immediately and independently — plan for effects to fire once per `set()`
call rather than assuming updates are deduplicated within a `batch(...)`
call.

## `batch(fn)`

```spl
batch(function() {
    count.set(100);
    multiplier.set(2);
});
```

Groups multiple signal updates inside one call. Treat it as an organizational
grouping construct rather than relying on it to suppress every intermediate
effect re-run — verify actual re-run counts for your use case if that
distinction matters to you.

## Typical use

Reactive state is most useful for **server-side computed values that other
route handlers read**, or for driving `res.render_ssr(...)` templates that
need to re-render when underlying signals change (see doc 38 for the
templating layer that consumes this on the client after hydration).
