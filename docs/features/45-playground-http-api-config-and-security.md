# 45 — Playground: HTTP API, Configuration & Security

Source: `pkg/playground` (execution engine), `pkg/playgroundserver` (HTTP
server), `pkg/ide` (Projects/IDE mode), `cmd/interpreter --playground` (the
sole shipped playground entry point — its `main.go` calls
`playgroundserver.Run` with `Variant{Name: "full", ExtraCapabilities:
[db, network]}` unconditionally, since `cmd/interpreter` always links every
`plugins/*` package).

**Verified:** earlier revisions shipped this as two separate binaries,
lightweight `cmd/playground` and full `cmd/playground-full`; both were
merged into `cmd/interpreter`'s `--playground` flag, which now always runs
with the full plugin set (there is no more lightweight playground variant
in a shipped binary — building one would mean writing your own `main.go`
against `pkg/playgroundserver` without blank-importing `plugins`).

## Running it

```bash
go -C cmd/interpreter run . --playground
# open http://localhost:8080
```

```bash
PLAYGROUND_AUTH_SECRET=dev-secret go -C cmd/interpreter run . --playground   # require sign-in
```

## Verified endpoints

```text
$ curl http://127.0.0.1:8099/api/health
{"ok":true,"service":"spl-playground"}

$ curl http://127.0.0.1:8099/api/ready
{"ok":true,"ready":true}

$ curl http://127.0.0.1:8099/api/session
{"auth_enabled":false,"authenticated":true,"render":{"allow_url_hosts":null,"allow_urls":false,"max_bytes":1048576,"mode":"auto"},"session_ttl_ms":43200000}

$ curl -X POST http://127.0.0.1:8099/api/execute -H "Content-Type: application/json" -d '{"code":"print \"hello\";"}'
{"output":"hello\n","result":"null","result_type":"NULL","error":"","error_kind":"","duration_ms":0}
```

`print` is a statement (doc 05), so `result`/`result_type` reflect the
script's *last expression value* (`null` here), while the printed text
lands in `output`.

## Full route table

| Method | Path | Auth required |
|---|---|---|
| GET | `/api/health` | no |
| GET | `/api/ready` | no |
| GET | `/api/session` | no |
| POST | `/api/login` | no (rate-limited 10/5min) |
| POST | `/api/logout` | no |
| GET | `/metrics` | no |
| GET | `/api/examples` | no |
| POST | `/api/execute` | yes, if auth enabled |
| GET/POST/DELETE | `/api/projects...` (Projects/IDE mode) | yes, if auth enabled |
| GET | `/` + static assets | no |

## `POST /api/execute` request/response

Request:

```json
{
  "code": "print \"hello\";",
  "render_mode": "auto",
  "render_allow_urls": false,
  "render_url_hosts": [],
  "render_max_bytes": 1048576
}
```

Response:

```json
{
  "output": "hello\n",
  "result": "null",
  "result_type": "NULL",
  "error": "",
  "error_kind": "",
  "diagnostics": [],
  "artifacts": [],
  "duration_ms": 1
}
```

Client-supplied `render_allow_urls`/`render_url_hosts`/`render_max_bytes`
can only **narrow**, never widen, what the server already permits.

## Verified authentication flow

```text
$ curl -X POST http://127.0.0.1:8098/api/execute -d '{"code":"print 1;"}'
{"error":"unauthorized"}   # HTTP 401 — PLAYGROUND_AUTH_SECRET is set

$ curl -c cookies.txt -X POST http://127.0.0.1:8098/api/login \
    -H "Content-Type: application/json" -d '{"secret":"testsecret"}'
{"authenticated":true,"ok":true,"session_ttl_ms":43200000,"token":"...","token_type":"bearer"}

$ curl -b cookies.txt -X POST http://127.0.0.1:8098/api/execute -d '{"code":"print 1+1;"}'
{"output":"2\n", ...}
```

Sessions are server-side opaque tokens (not JWTs), checked via either the
`spl_playground_session` cookie or an `Authorization: Bearer <token>`
header, compared with a timing-safe (`crypto/subtle`) constant-time check.
If `PLAYGROUND_AUTH_SECRET` (or its fallback `PLAYGROUND_API_KEY`) is
unset, **every endpoint is open** — always set it for any deployment
reachable outside a trusted local network.

## Environment variables

| Var | Default | Purpose |
|---|---|---|
| `PLAYGROUND_ADDR` | `:8080` | listen address |
| `PLAYGROUND_AUTH_SECRET` (`PLAYGROUND_API_KEY` fallback) | unset (auth disabled) | shared login secret |
| `PLAYGROUND_EXECUTION_PROFILE` | `untrusted` | `trusted` or `untrusted` |
| `PLAYGROUND_MAX_BODY_BYTES` | `1048576` | max request body |
| `PLAYGROUND_RATE_LIMIT` / `_RATE_WINDOW_MS` / `_RATE_CLEANUP_MS` | `60` / `60000` / `120000` | rate limiting |
| `PLAYGROUND_COOKIE_SECURE` | `false` | force `Secure` cookie flag |
| `PLAYGROUND_SESSION_TTL_MS` | `43200000` (12h) | session lifetime |
| `PLAYGROUND_READ/WRITE/IDLE/SHUTDOWN_TIMEOUT_MS` | `15000`/`15000`/`30000`/`10000` | HTTP server timeouts |
| `PLAYGROUND_TRUST_PROXY_HEADERS` | `false` | honor `X-Forwarded-For`/`X-Real-IP` |
| `PLAYGROUND_EVAL_MAX_DEPTH` / `_MAX_STEPS` / `_MAX_HEAP_MB` / `_TIMEOUT_MS` | `200`/`2000000`/`256`/`8000` | per-script eval limits |
| `PLAYGROUND_RENDER_ALLOW_URLS` / `_ALLOW_URL_HOSTS` / `_MODE` / `_MAX_BYTES` | `false`/`""`/`auto`/`1048576` | artifact render controls |
| `PLAYGROUND_INTERPRETER_BIN` / `_REPO_ROOT` / `_WORKSPACE_ROOT` | unset/unset/`./workspace` | Projects (IDE) mode config |

CLI flags (`--render-allow-urls`, `--render-url-hosts`, `--render-mode`,
`--render-max-bytes`, `--profile`) override env vars for local runs.

## Execution security profile

- `trusted`: `{AllowEnvWrite: true}`, no `ProtectHost`, plus allowed URL
  hosts if URL rendering is enabled.
- `untrusted` (default): `{ProtectHost: true, AllowedCapabilities:
  [filesystem_read, async, scheduler, server] + variant extras}`,
  `AllowedFileReadPaths` rooted at the request's working directory.
  `cmd/interpreter --playground`'s `"full"` variant adds `db` and `network`
  to the extras (see the note above — this is now the only shipped
  variant).
- `listen`/`listen_async` (doc 29) require both the `server` and `network`
  capabilities — the default example set avoids opening real sockets for
  this reason (the SSE/route-group examples are exercised via
  `http_probe`/local test servers, not by literally binding a port in the
  shipped demo scripts).

## Rate limiting

Fixed-window counters per client key (IP, or `X-Forwarded-For`/`X-Real-IP`
if `PLAYGROUND_TRUST_PROXY_HEADERS=true` and the proxy is trusted to strip
client-supplied values first). `/api/execute` uses the configured
`PLAYGROUND_RATE_LIMIT`/`_WINDOW_MS`; `/api/login` has its own fixed
10-requests-per-5-minutes limiter. Exceeding either returns
`429 {"error":"rate limit exceeded"}`.

## Metrics (`GET /metrics`, Prometheus text format)

`spl_playground_http_requests_total{route,method,status}`,
`spl_playground_auth_events_total{event}`,
`spl_playground_sessions_active`, `spl_playground_rate_limited_total`,
`spl_playground_http_request_duration_seconds{route,method,le}` (histogram),
`spl_playground_execution_duration_seconds{kind,le}` — all verified present
in a live `/metrics` scrape above.

## Render artifact modes

`render_mode`/`PLAYGROUND_RENDER_MODE`: `off` (drop artifacts entirely),
`metadata` (shape/kind info only, no bytes fetched), `auto`/`inline`
(resolve bytes, base64-encode images as data URLs, decode text content) —
see doc 27 for the artifact types this resolves (`file()`, `image()`,
`render()`).

## TLS

The playground serves plain HTTP; put it behind a TLS-terminating reverse
proxy for any non-local deployment, set `PLAYGROUND_COOKIE_SECURE=true`
once TLS is terminated, and only set `PLAYGROUND_TRUST_PROXY_HEADERS=true`
if the proxy is trusted to strip client-supplied `X-Forwarded-*` headers
(otherwise rate limiting and client-key derivation can be spoofed).

## Example menu

`/api/examples` returns a flat `{key: sourceCode}` map — 40+ examples
spanning language basics, modules, data values (files/images/CSV/tables),
runtime/session introspection, resource limits, production-profile
behavior, the query-builder template, stateful servers, middleware,
scheduling, SSE, reactive state, reactive HTML, and a full end-to-end tour.
The `"full"` variant's `ExampleOverrides`/`ExtraExamples` (see
`cmd/interpreter/examples.go`) override/extend a few of these (live SQLite
query-builder demo, real image codec demo, a PDF-tools example category)
and add examples for the newer plugins (email/phone/IP validation, money,
natural-language dates, sortable IDs, Shamir secret sharing, type
inference).

## Projects / IDE mode

Beyond single-script execution, the playground also exposes a small
project-workspace API (`/api/projects...`) backed by `pkg/ide`: file tree
CRUD (rooted/symlink-safe via `SafeJoin`), a real per-project OS subprocess
(never in-process, to avoid the shutdown-hook registry corrupting across
projects), SSE log streaming, and tooling endpoints
(`completions`/`hover`/`diagnostics`) that wrap the same `pkg/tooling`
engine used by `spltool lsp` (doc 41).
