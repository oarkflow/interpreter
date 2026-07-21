# 30 — Scheduler & File Watching

Source: `pkg/builtins/scheduler` (`pkg/builtins/scheduler/scheduler.go`),
`pkg/builtins/watcher` (`pkg/builtins/watcher/watcher.go`). Both are linked
into every interpreter binary (not optional plugins).

## Cron jobs

```spl
let jobA = schedule("* * * * *", "heartbeat", function() {
    print "heartbeat";
});
```

`schedule(cronExpr, [name,] handler)` parses a standard 5-field cron
expression and returns a job id string.

## Interval jobs

```spl
let jobB = schedule_interval(50, "tick", function() { print "tick"; });
// or with a duration string: schedule_interval("2s", "cleanup", fn);
```

Accepts either a millisecond integer or a duration string (`"2s"`, `"1h30m"`).

## One-shot jobs

```spl
let jobC = schedule_once("* * * * *", "init", function() {
    print "runs once, then deactivates";
});
```

## Inspecting & controlling jobs

```spl
print schedule_list();
// [{id, name, active, run_count, next_run?, last_run?, cron?, interval?}, ...]

schedule_cancel(jobB); // returns bool
```

## Running due jobs synchronously (tests/demos)

```spl
let executed = schedule_run(5); // runs up to 5 due jobs synchronously, returns count run
```

`schedule_run` is the deterministic way to exercise scheduled jobs from a
test or a one-shot script, without needing wall-clock time to actually pass.
`schedule_worker([duration])` instead blocks, running due jobs continuously
until `duration` elapses — for long-lived processes.

## Persistence

```spl
schedule_persist("jobs.json");
let restored = schedule_restore("jobs.json"); // reloads jobs from disk
```

Matches the shape of `testdata/scheduled_jobs.json` — a JSON array of job
definitions (`id`, `name`, `active`, plus `cron`/`interval` as applicable).

## Timezone

```spl
schedule_timezone("UTC"); // affects cron evaluation timezone
```

## Fire-and-forget background work

```spl
let f = background(function() { return "async work done"; });
print await f; // "async work done"
```

`background(handler, ...args)` runs `handler` on a goroutine and returns a
`Future` — similar to `go(...)` (doc 12) but namespaced under the scheduler
module and gated by the `async` capability rather than requiring the
scheduler capability.

## Capability requirements

All `schedule_*` builtins require the `scheduler` capability;
`background(...)` requires the `async` capability. Under the default
`untrusted`/playground profile, `scheduler` is explicitly allow-listed
(so scheduling demos work in the playground sandbox) while other
host-mutating capabilities remain denied — see doc 45.

## File watching & hot reload

```spl
let id = watch("config.json", function(event) {
    print "config changed: " + event;
});
print unwatch(id); // true
```

`watch(path, handler|pattern[, handler]|optsHash)` accepts either a bare
handler, a `(pattern, handler)` pair, or an options hash
`{pattern, handler, debounce}` (default debounce 200ms). Requires the
`watch` capability.

```spl
hot_reload("script.spl");
```

`hot_reload(path)` watches a file and automatically invalidates its module
cache entry and re-evaluates it whenever it changes — useful in
development loops (this is also what the REPL's `:reload <file>` command
does on demand, doc 40).
