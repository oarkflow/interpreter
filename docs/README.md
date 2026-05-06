# SPL Interpreter (Go)

Simple Programming Language (SPL) interpreter written in Go with:

- interactive REPL
- script execution
- module system (`import` / `export`)
- rich builtin library (strings, collections, time, crypto, file IO, exec, database)
- embedding API for Go projects

This repository currently includes a full feature showcase script at `testdata/complete_feature_showcase.spl`.

## Quick Start

### Requirements

- Go `1.25+`
- Network access for integration demos (optional)
- Local SMTP sandbox (optional): MailHog/Mailpit on `localhost:1025`

### Run REPL

```bash
go run ./cmd/interpreter
```

### Run a script

```bash
go run ./cmd/interpreter testdata/complete_feature_showcase.spl
```

### Common showcase output (expected)

The following lines are expected in the showcase because they intentionally
demonstrate error handling and builtin help:

- `expected caught: manual error`
- `expected runtime: identifier not found: unknown_identifier`
- `help(assert_eq):`
- `test_summary:`

These are demo validations, not failures.

### Run tests

```bash
go test ./...
```

### CLI Tooling

Use the standalone tooling command for source checks and formatting:

```bash
go run ./cmd/spltool check --json testdata/hello.spl
go run ./cmd/spltool fmt testdata/hello.spl
go run ./cmd/spltool mod init example/app
go run ./cmd/spltool mod tidy
```

`check` reports machine-readable diagnostics, `fmt` emits a canonical formatted version of the source, and `mod` manages `spl.mod` / `spl.lock` files for reproducible package-style imports.

Developer-experience helpers are also available for editors and CI:

```bash
go run ./cmd/spltool config init
go run ./cmd/spltool config show
go run ./cmd/spltool symbols --json testdata/hello.spl
go run ./cmd/spltool complete --prefix pri testdata/hello.spl
go run ./cmd/spltool hover --line 1 --col 1 testdata/hello.spl
go run ./cmd/spltool docs testdata/hello.spl
go run ./cmd/spltool test --json tests
go run ./cmd/spltool lsp
```

`check` includes parser diagnostics plus conservative static warnings for undefined identifiers, suspicious shadowing, unreachable statements, missing imports, deprecated builtins, and non-exhaustive match fallbacks. `symbols`, `complete`, and `hover` provide stable JSON surfaces that can back IDE/LSP integrations.

### Run benchmarks

```bash
go test ./... -run ^$ -bench . -benchmem
```

Or use the benchmark runner:

```bash
go run ./cmd/bench
```

## Language Features

### Variables and constants

```spl
let x = 10;
const PI = 3.14159;
```

Supports tuple-style assignment for functions/builtins returning arrays:

```spl
let db, err = db_connect("sqlite", ":memory:");
```

### Functions and closures

```spl
let makeAdder = function(x) {
  return function(y) { x + y; };
};
let add10 = makeAdder(10);
add10(5);
```

### Control flow

- `if / else`
- `while`
- `for (init; cond; post)`
- `break`, `continue`

### Collections and methods

- arrays: `map`, `filter`, `find`, `reduce`
- hashes (object-like maps), dot property access (`obj.key`)
- string and number method forms (`"x".upper()`, `(10).is_even()`)

### Module system

Supported import forms:

```spl
import "path/to/mod.spl";
import "path/to/mod.spl" as mod;
import {a, b} from "path/to/mod.spl";
import * as mod from "path/to/mod.spl";
```

Supported exports:

```spl
export let value = 42;
export const name = "math";
```

Module behavior:

- module cache enabled
- cache invalidation on file mod-time change
- circular import detection
- relative import resolution from importer directory
- package-style bare imports via `spl.mod` / `spl.lock`
- additional module lookup paths via `SPL_MODULE_PATH`

### Package manifests

SPL supports a lightweight manifest and lock flow for deterministic bare imports.

Example `spl.mod`:

```json
{
  "module": "example/app",
  "dependencies": {
    "mathlib": "./deps/mathlib"
  }
}
```

Then sync the lock file:

```bash
go run ./cmd/spltool mod tidy
```

And import with the dependency alias:

```spl
import "mathlib/math.spl" as math;
math.answer;
```

### Error handling

```spl
let result = try {
  throw "boom";
} catch (e) {
  "caught: " + e;
};
```

- `throw expr;`
- `try { ... } catch (e) { ... }`

## Formatting and Interpolation

New builtins for formatted output and template-style replacement:

### `sprintf(format, ...args)`

Returns formatted string using printf-like verbs.

```spl
let s = sprintf("name=%s n=%d ok=%t type=%T val=%v", "spl", 7, true, 3.14, {"a": 1});
```

Notes:

- supports common printf verbs (`%s`, `%d`, `%f`, `%t`, `%v`, ...)
- supports SPL type verb `%T` (returns SPL type name)
- validates argument count and returns clear `ERROR:` messages

### `printf(format, ...args)`

Prints formatted output and returns the formatted string.

```spl
printf("user=%s age=%d\n", "alice", 30);
```

### `interpolate(template, data[, ...positional])`

Replaces placeholders in `{key}` / `{index}` form.

```spl
interpolate("Hello {name}, items={count}", {"name": "SPL", "count": 3});
interpolate("{0} + {1} = {2}", null, 20, 22, 42);
```

Supports escaped braces `{{` and `}}`.

## Builtin Library Overview

Use `help()` to list builtins and `help("name")` for details.

Key groups:

- Core: `len`, `type`, `keys`, `puts`, conversions and parsing
- String: `trim`, `replace`, `substring`, casing transforms, regex helpers
- Collections: `first`, `last`, `slice`, `sum`, `avg`, `merge`, `group_by`, `clamp`
- Time: current time, formatting/parsing, timezone conversion, date arithmetic
- Crypto: hash, hmac, random bytes/string, UUID, password/hash helpers, AES-GCM
- JSON/Encoding: `json_encode`, `json_decode`, base64/hex/url encode/decode
- File/OS: `read_file`, `write_file`, `file_exists`, `remove_file`, `os_env`
- Process exec: `exec` with whitelist + timeout
- Database: `db_connect`, `db_query`, `db_exec`, `db_begin`, `db_commit`, `db_rollback`, `db_tables`, `db_close`
- Integrations:
  - HTTP: `http_request`, `http_get`, `http_post`, `webhook`
  - SMTP: `smtp_send`
  - FTP: `ftp_list`, `ftp_get`, `ftp_put`
  - SFTP: `sftp_list`, `sftp_get`, `sftp_put`
- Testing helpers: `assert_true`, `assert_eq`, `test_summary`, `run_tests`
- Formatting: `sprintf`, `printf`, `interpolate`

## Integrations Reference

All integration builtins return tuple-style responses for robust handling.

- Network calls generally return `[result, error]`
- Mutating operations generally return `[ok_bool, error]`

### HTTP

```spl
let res, err = http_get("https://httpbin.org/get");
if (err == null) {
  print res.status_code;
}

let payload = {"event": "build_done", "ok": true};
let wres, werr = webhook("https://example.com/hook", payload, {"X-Token": "abc"}, 5000);
```

### Database

`db_query` and `db_exec` now support both positional and named parameters, and transactions are available via `db_begin` / `db_commit` / `db_rollback`.

```spl
let db, err = db_connect("sqlite", ":memory:");
let _, _ = db_exec(db, "CREATE TABLE items (name TEXT, qty INTEGER)");
let _, _ = db_exec(db, "INSERT INTO items(name, qty) VALUES(?, ?)", ["apples", 3]);
let _, _ = db_exec(db, "INSERT INTO items(name, qty) VALUES(:name, :qty)", {"name": "pears", "qty": 4});

let tx, tx_err = db_begin(db);
let _, _ = db_exec(tx, "INSERT INTO items(name, qty) VALUES(:name, :qty)", {"name": "committed", "qty": 7});
let ok, commit_err = db_commit(tx);

let rows, query_err = db_query(db, "SELECT name, qty FROM items ORDER BY qty ASC", null, "array");
```

### SMTP

```spl
let ok, err = smtp_send({
  "host": "localhost",
  "port": 1025,
  "from": "noreply@localhost",
  "to": ["alice@localhost"],
  "subject": "Hello from SPL",
  "body": "Mail from SPL via local SMTP sandbox"
});
```

For local testing, run MailHog/Mailpit and keep SMTP on `localhost:1025`.

### FTP

```spl
let cfg = {"host": "ftp.example.com", "port": 21, "username": "u", "password": "p"};
let list, lerr = ftp_list(cfg, "/incoming");
let ok1, gerr = ftp_get(cfg, "/incoming/a.txt", "testdata/a.txt");
let ok2, perr = ftp_put(cfg, "testdata/a.txt", "/outgoing/a.txt");
```

### SFTP

```spl
let cfg = {
  "host": "sftp.example.com",
  "port": 22,
  "username": "u",
  "password": "p"
};
let list, lerr = sftp_list(cfg, "/data");
let ok1, gerr = sftp_get(cfg, "/data/in.csv", "testdata/in.csv");
let ok2, perr = sftp_put(cfg, "testdata/in.csv", "/data/out.csv");
```

## REPL

Interactive meta commands:

- `:help`
- `:builtins`
- `:search <text>`
- `:history`
- `:clear`
- `:vars`
- `:type <expr>`
- `:doc <name|expr>`
- `:methods <expr>`
- `:fields <expr>`
- `:ast <expr>`
- `:time <expr>`
- `:debug <expr>`
- `:mem`
- `:load <file>`
- `:reload [file]`
- `:install <alias> <path>`
- `:config <file> [json|yaml|env]`
- `!<shell command>`
- `:reset`

REPL supports history, semantic tab completion (including object members), inline suggestions,
call tips, parser-aware multiline input, persistent history, and enhanced runtime error display.

### Secure Config and Credentials

Use config helpers to load credentials without exposing values in output:

```spl
let cfg = config_load(".env", "env");
let db = config_load("config/database.yaml", "yaml");
let api = config_load("config/api.json", "json");
```

Access loaded keys using dot notation (nested too):

```spl
let db = config_load("config/database.yaml", "yaml");

// plain keys
print db.host;
print db.port;

// nested keys
print db.auth.username;

// secret keys are masked when printed
print db.auth.password;      // ***

// reveal only when explicitly needed
let raw_password = secret_reveal(db.auth.password);
```

`.env` files are loaded as a hash map, so keys are also accessible with dot notation:

```spl
let env_cfg = config_load(".env", "env");

print env_cfg.APP_NAME;
print env_cfg.DB_HOST;
print env_cfg.DB_PASSWORD;   // ***

let db_password = secret_reveal(env_cfg.DB_PASSWORD);
```

From REPL:

```text
spl> :config .env env
CONFIG loaded
spl> CONFIG.DB_HOST
localhost
spl> CONFIG.DB_PASSWORD
***
```

Sensitive keys (`password`, `secret`, `token`, `api_key`, `private_key`, etc.) are wrapped as
`SECRET` values and render as `***` in REPL/prints/docs. Use `secret_reveal(...)` only when
you explicitly need to pass plain values to external systems.

Related helpers:

- `config_load(path[, format])`
- `config_parse(raw, format)`
- `secret(value)`
- `secret_reveal(secret_value)`
- `secret_mask(value[, visible])`

## Embedding in Go

### Execute script string

```go
result, err := interpreter.Exec("let x = 40; let y = 2; x + y;", nil)
```

### Execute script with limits and cancellation

```go
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()

result, err := interpreter.ExecWithOptions(
  "let x = 40; let y = 2; x + y;",
  nil,
  interpreter.ExecOptions{
    Context:   ctx,
    MaxSteps:  1_000_000,
    MaxDepth:  200,
    MaxHeapMB: 128,
  },
)
if err != nil {
  var execErr *interpreter.ExecError
  if errors.As(err, &execErr) {
    fmt.Println(execErr.Kind, execErr.Message)
  }
}
```

### Execute script file

```go
result, err := interpreter.ExecFile("testdata/modules/entry_relative_import.spl", nil)
```

Or with options:

```go
result, err := interpreter.ExecFileWithOptions(
  "testdata/modules/entry_relative_import.spl",
  nil,
  interpreter.ExecOptions{Timeout: 3 * time.Second},
)
```

`ExecFile` resolves module-relative imports using the directory of the input file.

## Runtime Safety Controls

### CLI flags

- `--timeout`
- `--max-depth`
- `--max-steps`
- `--max-heap-mb`

### Environment variables

- `SPL_MAX_RECURSION`
- `SPL_MAX_STEPS`
- `SPL_EVAL_TIMEOUT_MS`
- `SPL_MAX_HEAP_MB`
- `SPL_MODULE_PATH`
- `SPL_DISABLE_EXEC`
- `SPL_EXEC_TIMEOUT_MS`
- `SPL_INT_CACHE_MAX` (optional integer interning cache upper bound, default `1000000`)

## Playground Production Configuration

`cmd/playground` now requires an explicit auth secret and supports server hardening env vars:

- `PLAYGROUND_AUTH_SECRET` (required; `PLAYGROUND_API_KEY` is accepted as a compatibility fallback)
- `PLAYGROUND_ADDR` (default `:8080`)
- `PLAYGROUND_MAX_BODY_BYTES` (default `1048576`)
- `PLAYGROUND_RATE_LIMIT` (default `60`)
- `PLAYGROUND_RATE_WINDOW_MS` (default `60000`)
- `PLAYGROUND_RATE_CLEANUP_MS` (default `120000`)
- `PLAYGROUND_COOKIE_SECURE` (default `false`)
- `PLAYGROUND_SESSION_TTL_MS` (default `43200000`)
- `PLAYGROUND_READ_TIMEOUT_MS` (default `15000`)
- `PLAYGROUND_WRITE_TIMEOUT_MS` (default `15000`)
- `PLAYGROUND_IDLE_TIMEOUT_MS` (default `30000`)
- `PLAYGROUND_SHUTDOWN_TIMEOUT_MS` (default `10000`)
- `PLAYGROUND_TRUST_PROXY_HEADERS` (default `false`)
- `PLAYGROUND_EVAL_MAX_DEPTH` (default `200`)
- `PLAYGROUND_EVAL_MAX_STEPS` (default `2000000`)
- `PLAYGROUND_EVAL_MAX_HEAP_MB` (default `256`)
- `PLAYGROUND_EVAL_TIMEOUT_MS` (default `8000`)

Playground endpoints:

- `GET /api/health`
- `GET /api/ready`
- `GET /api/session`
- `POST /api/login`
- `POST /api/logout`
- `GET /api/examples`
- `GET /metrics`
- `POST /api/execute` (requires `X-API-Key` or `Authorization: Bearer <key>`)

Security behavior for playground:

- per-client in-memory rate limiting with periodic stale-entry cleanup
- strict JSON request validation (`application/json`, unknown fields rejected)
- panic recovery middleware + structured request logs
- security headers (`X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, `Cache-Control`)
- proxy header trust is disabled by default and must be explicitly enabled

## CI and Container

- CI workflow: `.github/workflows/ci.yml`
- Container image build: `Dockerfile` (playground server)
- Docker context reduction: `.dockerignore`

## Governance and Release

- Production checklist: `docs/PRODUCTION_CHECKLIST.md`
- Compatibility policy: `docs/COMPATIBILITY_POLICY.md`
- Versioning policy: `docs/VERSIONING_POLICY.md`
- Release notes template: `.github/RELEASE_NOTES_TEMPLATE.md`

## Interpreter Security Policy

The interpreter supports optional policy-based controls for sensitive capabilities.

- `SPL_SECURITY_MODE=strict` enables default-deny for file/network/db/exec unless explicitly allowed.
- `SPL_PROTECT_HOST=1` disables host-mutating capabilities such as `exec`, `write_file`, `remove_file`, `os_env(key, value)`, and `exit()`.
- `SPL_ALLOW_ENV_WRITE` controls whether `os_env(key, value)` can mutate env vars.
- `SPL_EXEC_ALLOW_CMDS`, `SPL_EXEC_DENY_CMDS` control `exec` command policy.
- `SPL_NETWORK_ALLOW`, `SPL_NETWORK_DENY` control network targets for HTTP/SMTP/FTP/SFTP.
- `SPL_DB_ALLOW_DRIVERS`, `SPL_DB_DENY_DRIVERS`, `SPL_DB_DSN_ALLOW`, `SPL_DB_DSN_DENY` control `db_connect`.
- `SPL_FILE_READ_ALLOW`, `SPL_FILE_READ_DENY`, `SPL_FILE_WRITE_ALLOW`, `SPL_FILE_WRITE_DENY` control file and import access.

Embedding callers can also pass policy via `ExecOptions.Security`.

### Sandbox VM defaults

All execution paths now create a sandbox VM first:

- REPL runs with sandbox defaults: strict policy + host protection + bounded runtime limits.
- `Exec`/`ExecFile` run inside a bounded sandbox VM by default, with host mutation allowed unless explicitly restricted by policy.
- Module/file access is rooted to the sandbox base directory (`ModuleDir` for embedding, file directory for `ExecFile`).

Embedding callers can customize sandbox behavior via `ExecOptions.Sandbox`.

## Security Notes

- file operations use path sanitization to keep access inside project root
- `exec` is command-whitelisted and can be disabled globally (`SPL_DISABLE_EXEC=1`)
- playground evaluation enables host protection by default so browser-submitted code cannot mutate the host process or filesystem

## Performance Notes

Recent work included:

- Eval short-circuiting (`&&`, `||`)
- integer object interning for common integer values
- lower-allocation identifier lookup and expression evaluation paths
- benchmark coverage for lexer/parser/eval/import and full showcase parsing

For current numbers, run the benchmark commands in this README on your machine.

## Feature Showcase Files

- `testdata/complete_feature_showcase.spl`
- `testdata/modules/*`
- `testdata/tests/*`

These scripts demonstrate modules, builtins, formatting/interpolation, database usage, error handling, and test helpers end-to-end.

## Integration Showcase Examples

Use these snippets as copy-paste templates.

### HTTP request

```spl
let res, err = http_request(
  "POST",
  "https://httpbin.org/post",
  {"event": "deploy", "ok": true},
  {"X-Env": "staging"},
  5000
);
if (err == null) {
  print res.status_code;
  print res.body;
}
```

### Webhook

```spl
let wres, werr = webhook(
  "https://example.com/webhook",
  {"event": "build_done", "ts": now_iso()},
  {"Authorization": "Bearer token"},
  3000
);
```

### SMTP

```spl
let ok, err = smtp_send({
  "host": "localhost",
  "port": 1025,
  "from": "noreply@localhost",
  "to": ["alice@localhost", "bob@localhost"],
  "subject": "Build status",
  "body": "Pipeline complete"
});
```

## Troubleshooting

- If you see `identifier not found: unknown_identifier` in showcase output,
  it is intentional and caught by `try/catch`.
- If `smtp_send` fails locally, ensure your SMTP sandbox is listening on
  `localhost:1025`.
- FTP/SFTP examples in showcase are disabled by default and require real
  reachable endpoints before enabling.

### FTP

```spl
let cfg = {"host":"ftp.example.com","port":21,"username":"u","password":"p"};
let files, ferr = ftp_list(cfg, "/incoming");
let ok1, gerr = ftp_get(cfg, "/incoming/a.csv", "testdata/a.csv");
let ok2, perr = ftp_put(cfg, "testdata/a.csv", "/archive/a.csv");
```

### SFTP

```spl
let cfg = {"host":"sftp.example.com","port":22,"username":"u","password":"p"};
let files, serr = sftp_list(cfg, "/data");
let ok1, gerr = sftp_get(cfg, "/data/in.json", "testdata/in.json");
let ok2, perr = sftp_put(cfg, "testdata/in.json", "/data/out.json");
```
