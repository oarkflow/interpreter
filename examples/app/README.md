# SPL App Scaffold

A minimal, opinionated folder layout for building a "real" application in
SPL, similar in spirit to a Laravel or Express project: one entry point,
config/models/controllers/routes/views split by responsibility, and a
`spl.mod` manifest identifying the app as a module.

It implements a small Todo app, both as server-rendered HTML pages
(`/`, `/todos`, `/activity`) and as a JSON REST API (`/api/todos`,
`/api/activity`), persisted to **SQLite** and backed by the same model, to
show how one route layer can reuse the other. Views use a full
component/layout/signal templating engine (`builtins/template`, see
[Templates](#templates)), not the interpreter's built-in `{{var}}`-only
renderer. Along the way the app also exercises config loading, pattern
matching, custom types (classes and algebraic types), ES6-style arrow
functions, transactions, and channels/goroutines - see
[Language features used](#language-features-used).

## Run it from source

```bash
cd examples/app
go run ../../cmd/interpreter main.spl
```

`app/models/todo.spl` uses the `database` plugin module (`import
"database"; connect(...); exec(...); ...`), which is **not** linked into
the lightweight `cmd/interpreter` binary — it pulls in a real SQLite
driver dependency that most embedders don't need (see the root
`README.md`'s Quick Start). This app needs the full `cmd/interpreter`
build.

Run it from inside `examples/app/`: `app/config/app.spl`'s `VIEWS_DIR`,
`PUBLIC_DIR`, and `DB_PATH` are plain relative paths, resolved against the
process's working directory (there's no script-directory builtin to
anchor them to `main.spl`'s own location), so `res.render()`/`static()`/
`connect()` need the app directory as cwd to find `app/views`, `public`,
and `storage/app.db`.

No flags needed — `cmd/interpreter`'s default sandbox no longer
applies a wall-clock deadline to trusted scripts, so a server started
with `listen()` keeps running (and keeps handling requests) indefinitely.
Press `Ctrl+C` to stop it; the server drains in-flight requests and closes
its listener before the process exits (press `Ctrl+C` twice to force an
immediate quit). If you do want a hard lifetime cap, pass
`--timeout <duration>` explicitly, e.g. `--timeout 1h`.

Then visit `http://localhost:8080/` (HTML UI) or:

```bash
curl http://localhost:8080/api/todos
curl -X POST http://localhost:8080/api/todos -d '{"title":"write docs","priority":"high"}'
curl -X PUT http://localhost:8080/api/todos/1 -d '{"done":true}'
curl -X DELETE http://localhost:8080/api/todos/1
curl http://localhost:8080/api/activity
```

Data persists in `storage/app.db` (SQLite) between runs — restart the
process and `GET /api/todos` still returns what you created before.

## Configuration

`app/config/app.spl` loads settings from a `.env` file via the
interpreter's built-in `config_load(".env", "env")`, with defaults for
everything so the app still boots with no `.env` present at all:

```bash
cp .env.example .env
```

```text
APP_NAME=SPL Todo App
APP_PORT=8080
APP_ENV=development
VIEWS_DIR=app/views
PUBLIC_DIR=public
DB_PATH=storage/app.db
ADMIN_TOKEN=change-me-admin-token
```

`DB_PATH` is the SQLite file `app/models/todo.spl` connects to
(`db_connect("sqlite", config.DB_PATH)`); use `:memory:` for a throwaway
database, or an absolute path for the single-binary build (see below).

`config_load()` doesn't throw on a missing file - it returns an
error-sentinel value instead - so the load is wrapped in `try/catch` and
treated the same as "no config" (see `app/config/app.spl`). Any config key
containing `token`, `secret`, `password`, etc. is automatically wrapped as
a `SECRET` value by the interpreter: it prints as `***` everywhere
(`print`, logs, error messages) and has to be unwrapped explicitly with
`secret_reveal()` at the point of use. `ADMIN_TOKEN` demonstrates this end
to end: set it in `.env`, then call the protected admin route:

```bash
curl -X POST http://localhost:8080/api/admin/worker/stop \
  -H "Authorization: Bearer change-me-admin-token"
```

`app/middleware/auth.spl` checks the header against
`secret_reveal(config.ADMIN_TOKEN)` — the raw value is never logged or
returned to the client.

## Import aliases

Every import in the app is rooted at `app/` with a webpack/React-style
alias instead of relative `../../..` chains:

```spl
import "@/models/todo.spl" as Todo;
import "@/support/http.spl" as http;
```

This needed no interpreter changes — it's just `spl.mod`'s existing
bare-import dependency map (the same one `spltool mod tidy` fills in for
real external dependencies, see the root `README.md`'s "Package
manifests"), pointed at the app's own `app/` directory:

```json
{
  "module": "examples/app",
  "dependencies": {
    "@": "app",
    "~": "app"
  }
}
```

An import path with no leading `.` or `/` is treated as bare; the
segment before the first `/` is looked up in `dependencies`, and the rest
of the path is resolved under whatever directory that maps to. Both `@`
and `~` are defined here pointing at the same `app/` directory, so
`import "~/models/todo.spl"` works identically to `import
"@/models/todo.spl"` — pick whichever convention you prefer, or add more
aliases (e.g. `"config": "app/config"`) the same way. Relative imports
(`import "../x.spl"`) still work everywhere too; aliasing is additive, not
a replacement.

## Single-binary build

`examples/app` is its own Go module (a separate `go.mod`, the same
pattern `cmd/interpreter` uses) so it can link `builtins/database`
directly - the root module intentionally doesn't carry that dependency.
`main.go` embeds the entire app (`main.spl`, `app/`, `public/`,
`storage/`) into one Go binary via `go:embed`, so the compiled executable
needs no source tree at runtime:

```bash
cd examples/app
go build -o spl-todo-app .
./spl-todo-app
```

The binary extracts its embedded files to a temp directory, `cd`s into it,
and runs `main.spl` exactly like running from source — it's a thin
wrapper, not a rewrite. It picks up a `.env` from wherever it's launched
(copied in before startup) if one exists, and runs on the built-in
defaults otherwise. It has its own `Ctrl+C`/`SIGTERM` handling (mirroring
`pkg/eval/cli.go`'s), so shutdown is graceful and the temp directory is
always cleaned up, whether the process exits normally or via signal.

```bash
# run from anywhere - no examples/app/ directory needed nearby
cp /tmp/spl-todo-app ~/somewhere/
cd ~/somewhere && ./spl-todo-app
```

**The default `DB_PATH=storage/app.db` is relative to the extracted temp
directory, which is removed on exit — data does *not* persist across runs
of the compiled binary unless you override it.** Set an absolute
`DB_PATH` in a `.env` next to the binary to get real persistence:

```bash
echo "DB_PATH=/home/you/spl-todo-app.db" >> ~/somewhere/.env
```

## Templates

The interpreter's built-in template runtime only replaces `{{var}}`
placeholders — no loops, conditionals, layouts, or components. This app
instead links `builtins/template`, a plugin (own `go.mod`, same pattern as
`builtins/database`) that blank-imports `github.com/oarkflow/spl` — a
templating engine with `${expr}` interpolation, `@if`/`@for`/`@match`,
layouts (`@extends`/`@block`/`@define`), reusable `@component`s, and
client-reactive `@signal`/`@watch`/`@reactive`/`on:click` with automatic
SSR hydration. It self-registers via the interpreter's
`RegisterTemplateRuntimeFactory` hook (see `builtins/template/template.go`)
— importing it for its side effect is the entire integration.

```spl
res.render("home.html", {...});          // plain SSR - no hydration payload
res.render_ssr("todos/index.html", {...}); // SSR + hydration for @signal/on:click
```

Use `render_ssr` (not `render`) for any view using `@signal`/`@watch`/
`@reactive`/`on:click` — plain `render` still parses those directives but
skips the hydration payload and client runtime, so `on:click` stays an
inert HTML attribute instead of becoming a wired-up event listener.

- `app/views/layout.html` — shared shell (`@block("content")`, etc.);
  every page `@extends` it.
- `app/views/components/todo_components.html` — `PriorityBadge` and
  `TodoRow` components, `@import`ed into `app/views/todos/index.html` and
  rendered per-item with `@render("TodoRow", t)`. Component props are
  matched by name straight from the passed hash — no manual destructuring.
- `app/views/todos/index.html` — the fullest example: a `@signal`-backed
  live toggle button plus a `@watch` block that re-renders client-side
  with **zero server round-trip** (verified via the SSR output and the
  injected hydration payload/runtime — see the caveat below), server-side
  priority filter links (reusing `TodoFilter`, see
  [Custom types](#custom-types)), and `@for`/`@empty` over the todo list
  rendered through the `TodoRow` component.

**Known limitation (this template engine version, verified by testing):**
`@reactive(...) { @for(...) {...} }` - wrapping a loop in a reactive block
- fails with `cannot iterate over STRING`. Plain `@for` (outside
`@reactive`) works fine and is what's used here; the todo *list* itself is
therefore re-rendered via the server-side filter links, not live-updated
client-side. The `@signal`/`@watch`/`on:click` toggle button in
`todos/index.html` doesn't hit this path and works as documented.

**Verification note:** this environment has no headless browser, so
`on:click` → signal write → `@watch` re-render was verified structurally
rather than by clicking a real button: the SSR output correctly compiles
`on:click="..."` to `data-spl-on-click="..."`, the hydration
`<script type="application/json" data-spl-hydration>` payload correctly
embeds the initial signal value, and the injected runtime script contains
real, unminified-enough-to-read `addEventListener`/`SPL.write`/
`SPL.subscribe` wiring that reads those attributes and payload. Treat
browser-side interactivity as strongly-implied-correct rather than
click-tested.

## Language features used

Beyond the basics (functions, closures, modules, the HTTP server
builtins), the app deliberately exercises a few more corners of the
language as real, load-bearing code rather than toy snippets:

- **`match` with type patterns + guards** — `app/support/http.spl`'s
  `parse_id()`, shared by every controller that takes a `:id` path param,
  turns a raw param into a validated positive integer or `null` in one
  expression (`case n: integer if n > 0 => n`).
- **`match` with OR patterns and wildcards** —
  `app/models/todo.spl`'s `normalize_priority()` folds any input into a
  valid priority level, falling back to `"medium"` for anything
  unrecognized.
- **Channels + `go()` + `select`** — `app/support/events.spl` runs a
  background worker that receives on one channel (published activity
  events) and a second "stop" channel via `select`, so it can be shut
  down cleanly through the admin route instead of only being killed with
  the process.
- **`config_load()` + secrets** — see [Configuration](#configuration)
  above.
- **SQLite + transactions** — `app/models/todo.spl` connects with
  `db_connect("sqlite", ...)`, creates its schema with `db_exec()`, reads
  with `db_query(..., "array")`, and runs `destroy()` inside an explicit
  `db_begin()`/`db_commit()`/`db_rollback()` transaction.
- **Custom types (classes + algebraic types)** and **arrow functions** —
  see the dedicated sections below.

One gotcha worth calling out because it's easy to hit by accident:
`parse_int()` (and several other builtins) signal a bad input by
returning an `"ERROR: ..."` string rather than throwing - but the
language's own error-propagation convention treats *any* string with that
prefix as an error, so feeding it straight into `match` as the subject
aborts the expression instead of falling through to the wildcard case.
`parse_id()` wraps the call in `try/catch` first for exactly this reason;
see the comment there for the full explanation.

### Custom types

SPL has two distinct ways to declare a custom type, both used in
`app/support/types.spl`:

- **`class`** — stateful, with fields and methods; construct by calling
  the class name directly (no `new` keyword). `TodoInput`, `TodoFilter`,
  and `BearerToken` are typed parsers request data decodes into — see
  [Typed request parsing](#typed-request-parsing) below.
- **`type X = A(...) | B(...)`** — an algebraic/tagged-union type.
  Construct a variant by calling it (`Valid(x)`); take it apart with
  `match`/`case Valid(v) => ...` *anywhere* the value ends up, including a
  different module than the one that declared the type (`match` only
  inspects the value's tag, it doesn't need the constructor in scope).
  `ValidationResult = Valid(value) | Invalid(reason)` is what
  `TodoInput.validate()` returns, matched on in
  `app/controllers/api_todo_controller.spl`'s `store()`.

```spl
export type ValidationResult = Valid(value) | Invalid(reason);

export class TodoInput {
  init(data) {
    let {title, priority, done} = data;   // see the note below
    this.title = title;
    this.priority = priority;
    this.done = done;
  }
  validate() {
    if (type(this.title) != "STRING" or len(this.title) == 0) {
      return Invalid("title is required");
    }
    return Valid(this);
  }
}
```

**Note:** SPL's function/method parameter lists only bind plain
identifiers — object/array destructuring (`{a, b}`, `[a, b]`) is valid
only in `let`/`const` declarations and `match` patterns, not in a
parameter list. So `init({title, priority}) {...}` doesn't parse; each
constructor above destructures its single hash argument as the *first
line of the body* instead, which is the idiom used throughout
`app/support/types.spl`.

### Typed request parsing

`app/support/http.spl`'s `body_as`/`query_as`/`headers_as` decode a
request's body/query/headers through any callable — typically one of the
classes above — instead of controllers reading loose hash fields by hand:

```spl
let input = http.body_as(req, types.TodoInput);   // req.json() -> TodoInput
let filter = http.query_as(req, types.TodoFilter); // req.query -> TodoFilter
let bearer = http.headers_as(req, types.BearerToken); // req.headers -> BearerToken
```

- `api_todo_controller.spl`'s `store()` parses the JSON body into
  `TodoInput`, then `match`es its `validate()` result.
- `api_todo_controller.spl`'s `index()` and `todo_controller.spl`'s
  `index()` both parse `?priority=&done=` into `TodoFilter` and call
  `.matches(todo)` to filter the list — the same typed filter backs both
  the JSON API and the server-rendered page's filter links.
- `app/middleware/auth.spl`'s `require_admin` parses the `Authorization`
  header into `BearerToken` and calls `.matches(expectedToken)`.

### Arrow functions

Arrow functions (`(a, b) => expr` and `(a, b) => { ...; return x; }`) work
anywhere a `function` expression does, including as a `let`/`const`
binding — but `export name = (a) => expr;` (no `let`/`const`) is **not**
valid syntax; use `export let name = (a) => expr;` or
`export const name = (a) => expr;`. `app/models/todo.spl`'s
`normalize_priority`/`priority_rank` use the expression-body form (the
body is a single `match` expression).

One gotcha: an arrow's *implicit* (braceless) body can't be a bare hash
literal - `(x) => {"a": x}` parses as a block statement (then fails to
parse `"a": x` as a statement), not a returned hash. Use an explicit
block instead: `(x) => { return {"a": x}; }`. `app/models/todo.spl`'s
`row_to_todo` is written this way for exactly this reason.

## Folder structure

```text
examples/app/
├── main.spl                 single entry point: boots config, background
│                             workers, middleware, routes, then listen()
├── main.go                  optional: builds main.spl + app/ + public/ +
│                             storage/ into a single Go binary (go:embed)
├── go.mod                   separate module (like cmd/interpreter)
│                             so main.go can link builtins/database and
│                             builtins/template
├── spl.mod                  module manifest + the @/ and ~/ import aliases
├── .env.example              config template (copy to .env)
├── app/
│   ├── config/               app-wide settings, loaded via config_load()
│   ├── models/                data access (SQLite), one file per domain
│   │                           concept
│   ├── middleware/            request/response middleware (req, res, next)
│   ├── controllers/           request handlers (req, res); one file per
│   │                           resource, split web (HTML) vs api (JSON)
│   ├── routes/                route registration, grouped by concern
│   │                           (web.spl for HTML, api.spl for JSON)
│   ├── support/                small shared helpers: form parsing +
│   │                            typed request parsers (http.spl), custom
│   │                            types (types.spl), the channel-based
│   │                            activity event bus (events.spl)
│   └── views/                  templates (layout, components, pages) -
│                                see Templates below
├── public/                   static assets served at /public/*
└── storage/                  writable app data: app.db (SQLite), logs/
```

## Conventions

- **Single entry point.** Everything boots from `main.spl`. It never
  contains business logic itself — it only wires config, background
  workers, middleware, and routes together and calls `listen()`.
- **Routes call controllers, controllers call models.** Route files never
  contain handler bodies inline; they import a controller module and pass
  its exported functions to `route()` / `route_group()`.
- **Models are the only thing that touches storage.** Controllers never
  build SQL or reach into a raw data store directly — they call exported
  model functions (`Todo.all()`, `Todo.create()`, ...), so storage can be
  swapped later without touching routes/controllers/views.
- **`@/`-rooted imports everywhere, no `../../..` chains.** Every import in
  the app is written `import "@/models/todo.spl"` etc. — see
  [Import aliases](#import-aliases) below for how that's wired up and why
  it doesn't need any interpreter changes.
- **Views own their markup; controllers only gather data.** Row/badge
  HTML lives in `app/views/components/`, not built as strings in
  controllers — see [Templates](#templates).

## Adding a new resource

1. `app/models/<name>.spl` — exported CRUD-style functions.
2. `app/controllers/<name>_controller.spl` (and/or
   `api_<name>_controller.spl`) — exported `(req, res)` handlers that call
   the model.
3. Register routes in `app/routes/web.spl` and/or `app/routes/api.spl`.
4. Add any views under `app/views/`.

Nothing in `main.spl` needs to change unless the new route file itself is
new (in which case, import and call its `register(app)` there). If you add
a new top-level file/directory that should ship in the single-binary
build, add it to `main.go`'s `//go:embed` directive too.
