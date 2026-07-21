# 51 — HTTP Security Middleware (`tcpguard`)

Source: `plugins/tcpguard`, wrapping `github.com/oarkflow/tcpguard`, a
runtime HTTP security policy engine. Full example:
`examples/tcpguard_all_in_one.spl`. Requires the [HTTP server
plugin](29-http-server-routing-and-sse.md) (`plugins/server`) for
`guard_middleware`.

This plugin wraps only the **core load → build guard → attach middleware
loop**. `tcpguard.Guard` itself is much larger (abuse detection tuning,
approval-gated actions, Redis-backed distributed stores, threat intel
feeds/enrichers, a hardened management server for hot-reload) — none of
that is exposed here yet. Reach for it when you want request-level security
rules (rate abuse, bad user agents, sensitive-path protection, risk
scoring) described declaratively in BCL and enforced automatically on every
request, instead of hand-written `if` checks in every route/middleware.

`tcpguard_new()` requires the `policy` capability under a restrictive
security policy (`--allow-cap policy`).

## `tcpguard_load(source_or_path)`

```spl
let policy = `
pack "example-security-pack" {
  version "1.0.0"
  mode enforce
}

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

let [bundle, err] = tcpguard_load(policy);
if (err != null) { throw err; }
```

**Write policy blocks one field per line.** Condensing certain blocks onto
a single line (`risk { base 90 }` in particular) can silently fail to take
effect depending on what else is condensed around it — the policy still
parses without error, but enforcement quietly degrades (e.g. `block`
becomes `monitor`). Always verify a new/edited policy with
`tcpguard_evaluate` before trusting it.

Auto-detected like `rules_publish`'s source argument: a real file loads via
`tcpguard.LoadTCPGuardBundleFile`, a real directory via
`tcpguard.LoadTCPGuardBundleDir` (following its own `include` globs), and
anything else parses as an inline BCL block. File/directory access goes
through the same filesystem-read sandboxing as every other file-reading
builtin.

## `tcpguard_new(bundle[, opts])`

```spl
let [guard, err] = tcpguard_new(bundle, {"mode": "enforce"});
if (err != null) { throw err; }
```

`opts`: `mode` (`"enforce"` (default) or `"monitor"`), `geoip` (bool,
default `false`). **GeoIP enrichment is opt-in on purpose**: the library's
default context builder loads a sizeable in-memory IP-geolocation dataset
on the first evaluated request, which is an expensive and surprising
default inside an embedded script runtime unless a policy actually
references `network.country`/`network.city`/etc. facts. Pass
`{"geoip": true}` if your rules need it.

## `guard_middleware(server, guard)`

```spl
let app = server();
route(app, "GET", "/admin/secret", function(req, res) { res.json({"ok": true}); });
guard_middleware(app, guard);
listen_async(app, 8080);
```

Attaches `guard` as global middleware on an SPL `server()` handle. Every
request is bridged into a synthetic `*http.Request` (method, path, query,
headers, body, remote IP) and evaluated through the guard; a request the
guard decides to enforce against gets the guard's own decision response
(status/headers/body) written back and the route/middleware chain is
short-circuited — matching routes never run. Everything else passes
through untouched.

## `tcpguard_evaluate(guard, facts)`

```spl
let [decision, err] = tcpguard_evaluate(guard, {
    "method": "GET",
    "path": "/admin/secret",
    "headers": {"User-Agent": ["sqlmap/1.0"]}
});
if (err != null) { throw err; }
print decision.Effect;   // "block"
print decision.Allowed;  // false
```

Ad-hoc scripted evaluation without a live server: `facts` describes a
synthetic request (`method`, `path`, `body`, `ip`, and a nested `headers`
hash of `name -> string | [string]`). Useful for testing a policy pack or
making a one-off authorization decision outside the HTTP request path.
Returned hash keys mirror the library's `Decision` struct field names
as-is (capitalized) — see the `rules` doc's note on the same generic
struct-to-hash conversion.

## Not yet supported

The `AbuseDetector` configuration surface, approval-gated actions,
`RedisStore`/distributed state, threat-intel feeds and enrichers,
baseline/anomaly detectors, the `oarkflow/authz` DSL integration, and
`NewManagementServer` (hot-reload, RBAC-protected simulate/canary
endpoints). All of these exist on `tcpguard.Guard`/`Option` in the
underlying library and can be added as builtins later if needed — nothing
here is a hard architectural limit, just unwired surface.
