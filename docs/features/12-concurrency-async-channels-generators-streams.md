# 12 — Concurrency: Async/Await, Channels, Generators & Streams

Source: `pkg/builtins/concurrency.go`, `pkg/eval/eval.go` (channel/select/spawn
evaluation), `pkg/object/object.go` (`Future`, `Channel`, `GeneratorValue`,
`Stream`), `pkg/eval/builtins.go` (stream builtins).

SPL's concurrency model is Go-flavored: goroutine-backed `Future`s, buffered
channels with blocking send/receive, and a `select` statement — layered with
lighter-weight `async`/`await` sugar and array-like generators/streams.

## `async` functions and `await`

```spl
let asyncDouble = async function(x) { return x * 2; };
let futureVal = asyncDouble(21);
print await futureVal; // 42

let asyncSquare = async (x) => x * x;
print await asyncSquare(7); // 49
```

> Prefer `let name = async function(...) {...};` or `async (...) => ...`
> over a bare `async function name() {...}` statement — the named-statement
> form does not currently bind `name` into the enclosing scope the way a
> plain `function name() {}` declaration does.

Calling an `async` function returns a `Future` immediately; `await` blocks
until it resolves (or re-throws if the async body threw):

```spl
let asyncFail = async function() { throw "async boom"; };
let caught = try {
    await asyncFail();
} catch (e) {
    "caught async: " + e;
};
print caught; // caught async: async boom
```

## `go(fn, ...args)` — explicit goroutine

```spl
let ch = channel();
let producer = go(function() {
    send(ch, 42);
    return "sent";
});
print recv(ch);      // 42
print await producer; // sent
```

`go` runs `fn` on a new goroutine and returns a `Future` for its return
value, just like calling an `async function` does.

## `go_async(fn, ...args)` — fire-and-forget

```spl
go_async(function() { print "background work"; });
```

Like `go`, but discards the resulting `Future` — no way to `await` its
result; useful for side-effecting background work.

## `await_all` / `await_race`

```spl
let f1 = go(function() { sleep(10); return 1; });
let f2 = go(function() { sleep(5); return 2; });
print await_all([f1, f2]);  // [1, 2] — resolves once every future is done

let raceA = async function() { return "A wins"; };
let raceB = async function() { return "B wins"; };
print await_race([raceA(), raceB()]); // whichever resolves first
```

## Channels

```spl
let ch = channel();     // unbuffered
let buffered = channel(10); // buffered with capacity 10
send(ch, value);
let v = recv(ch);
```

Channel send/receive also have dedicated expression syntax: `ch <- value`
(send) and `ch <-` (postfix receive expression), in addition to the
`send()`/`recv()` builtins.

## `select`

`select` waits on multiple channel operations at once. **In this language,
`select` cases are receive-only** (no non-blocking send case) — the
`case channel <- bindingName:` form means "receive from `channel`, bind the
received value to `bindingName`", not "send".

```spl
let bus = channel(10);
let stop = channel();

let worker = go(function() {
    let count = 0;
    while (true) {
        select {
            case bus <- evt: {
                count += 1;
                print sprintf("got: %v (count=%d)", evt, count);
            }
            case stop <- _: {
                return "stopped after " + count;
            }
        }
    }
});

send(bus, "a");
send(bus, "b");
sleep(20);
send(stop, true);
print await worker; // stopped after 2
```

This is the standard pattern for a cleanly-stoppable background worker: a
dedicated `stop` channel is select-ed alongside the real work channel so the
loop can exit on demand instead of being killed abruptly. See
`examples/app/app/support/events.spl` for a full production-style version
(an in-memory activity bus with bounded history).

A `select` can also include a `default:` case that runs immediately if no
channel is ready, for a non-blocking poll.

## `spawn`

`spawn f(x)` schedules a call without eagerly evaluating it inline (parsed
specially so the call expression itself isn't evaluated before scheduling).

## Generators

```spl
let gen = generator(function() { return [1, 2, 3]; });
```

`generator(fn)` wraps `fn`'s array result as a `GeneratorValue` for use with
the stream pipeline below (its runtime `Type()` reports as `ARRAY_OBJ`, so
it interoperates with array-oriented code).

## Streams

```spl
let s = stream([1, 2, 3, 4, 5]);
let doubled = stream_map(s, x => x * 2);
let evens = stream_filter(doubled, x => x % 4 == 0);
print stream_to_array(evens); // [4, 8]
```

Stream builtins: `stream(arr)`, `stream_map(s, fn)`, `stream_filter(s, fn)`,
`stream_reduce(s, fn, init)`, `stream_collect(s)`, `stream_to_array(s)`.

## `for await` — async iteration

```spl
for await (v in stream([10, 20, 30])) {
    print v;
}
```

Iterates a `Stream`/`GeneratorValue` source, suitable for pipelines that may
yield asynchronously.

## Runtime permission interaction

Some concurrency primitives (`async`, background work) are governed by the
`async` capability under a restrictive security policy — see doc 44.
