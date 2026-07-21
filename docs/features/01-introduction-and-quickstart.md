# 01 — Introduction & Quickstart

## What is SPL?

SPL ("Simple Programming Language") is a dynamically-typed, C-family scripting
language implemented as a tree-walking interpreter in Go
(`github.com/oarkflow/interpreter`). It ships as:

- an **interactive REPL** with tab completion, call tips, and session workspaces
- a **script runner** (`cmd/interpreter script.spl`, built from its own
  Go module — see "Installing / building" below)
- an **embedding API** for Go programs (`interpreter.Exec`, `interpreter.Runtime`, ...)
- a **CLI toolchain** (`spltool`) for formatting, static checks, testing, packaging, and an LSP server
- a **browser playground** (`cmd/interpreter --playground`) with a Monaco editor and a small JSON API
- a rich **builtin library**: strings, collections, time, crypto, JSON, files,
  process exec, database access, HTTP/SMTP/FTP/SFTP integrations, PDF
  generation, image processing, a secrets vault, email/phone/IP validation,
  money arithmetic, natural-language date parsing, and more

The language itself supports closures, classes, algebraic data types, pattern
matching, macros, async/await, channels, generators/streams, optional type
annotations, and a full sandboxing/security-policy system for running
untrusted scripts safely.

## The root module vs. `cmd/interpreter`

| Module | What it links | Use case |
|---|---|---|
| `github.com/oarkflow/interpreter` (root) | Core language + `tools/*` (file/archive/media/secret chores) + server/scheduler/reactive/watcher builtins | Lightweight; the default `go get github.com/oarkflow/interpreter` dependency footprint, meant for embedding (doc 42) |
| `cmd/interpreter` (its own Go module, separate `go.mod`) | Everything the root module has **plus** every package under `plugins/` — `database`, `images`, `integrations` (HTTP/SMTP/FTP/SFTP), `pdf`, `secretr`, `cryptoextra` (bcrypt/JWT), `securetoken`, `template`, `xql`, `config/yaml`, `emailvalidator`, `phone`, `ip`, `money`, `naturaldate`, `wuid`, `shamir`, and `metadata` | The full-featured, batteries-included CLI/REPL/playground binary this project ships |

`cmd/interpreter` is its own Go module (not part of the root module) precisely
*because* several of those `plugins/*` packages pull in heavy or private
third-party dependencies (DB drivers, image codecs, a private `secretr`
sibling checkout, ...) that `go get github.com/oarkflow/interpreter` should
never have to carry. Building your own lightweight CLI is just a `main.go`
that imports the root module without blank-importing `plugins` (see doc 42);
if a script calls a builtin that isn't linked into whatever binary is running
(e.g. `db_connect` in such a custom build), it fails with an actionable error
naming the missing module rather than "identifier not found".

**Verified:** older revisions of this project shipped separate
`cmd/interpreter-full`, `cmd/playground`, and `cmd/playground-full` binaries;
these have since been merged into the single `cmd/interpreter` binary (the
browser playground is now `cmd/interpreter --playground`, always with the
full plugin set — see doc 45). If you see those names in older notes or
issues, read them as `cmd/interpreter`.

There's also `cmd/splworker` — a minimal, single-purpose binary that only
implements the untrusted worker subprocess protocol (see doc 44); `cmd/bench`
— a thin wrapper that runs the project's benchmark suites; and `cmd/spltool`
/ `cmd/spltool-full` — the CLI/LSP tool in lightweight and full-plugin
flavors respectively (doc 41).

## Installing / building

Requires Go 1.25+.

```bash
git clone <this repo>
cd interpreter
go build -o spltool ./cmd/spltool
go -C cmd/interpreter build -o /tmp/interpreter .   # its own module, build from its own dir
```

`cmd/interpreter`, `cmd/spltool-full`, and the various `plugins/*` packages
are each their own Go module, so `go get github.com/oarkflow/interpreter`
alone never pulls in database drivers, image codecs, or other third-party
dependencies.

## Running the REPL

`cmd/interpreter` is its own Go module, so `go run`/`go build` for it must be
invoked from (or targeted at) its own directory, not `./cmd/interpreter` from
the repo root:

```bash
go -C cmd/interpreter run .
```

```text
spl> let x = 40;
spl> x + 2
42
spl> :help
```

See doc 40 for the full REPL command reference.

## Running a script

Build once, then run the binary directly against a repo-root-relative script
path (since `go run`'s working directory would otherwise be `cmd/interpreter`,
not the repo root):

```bash
go -C cmd/interpreter build -o /tmp/interpreter .
/tmp/interpreter examples/all_in_one.spl
```

`examples/all_in_one.spl` is the canonical, self-contained language tour —
every feature documented across these documents has a runnable example
there. Static-check it without running:

```bash
go run ./cmd/spltool check examples/all_in_one.spl
```

## Running untrusted / third-party code

```bash
/tmp/interpreter --profile untrusted script.spl
/tmp/interpreter --profile untrusted --allow-network example.com script.spl
```

The `untrusted` profile applies strict host protection, bounded runtime
limits, worker-process execution, and a default read-only filesystem
capability rooted at the script's directory. Add `--require-os-isolation` on
Linux to additionally sandbox with `bubblewrap`. See docs 44 and 45.

## Hello, world

```spl
print "Hello, world!";
```

```spl
let name = "SPL";
print sprintf("Hello, %s!", name);
```

## A slightly bigger taste

```spl
import "std/core" as core;

function fib(n) {
    if (n < 2) { return n; }
    return fib(n - 1) + fib(n - 2);
}

let numbers = [1, 2, 3, 4, 5];
let doubled = numbers.map(x => x * 2);
let total = doubled.reduce((a, b) => a + b, 0);

print core.sprintf("fib(10)=%d doubled=%v total=%d", fib(10), doubled, total);

type Result = Ok(value) | Err(error);
let safeDiv = function(a, b) {
    if (b == 0) { return Err("division by zero"); }
    return Ok(a / b);
};

let outcome = match (safeDiv(10, 2)) {
    case Ok(v)  => "result: " + v
    case Err(e) => "error: " + e
};
print outcome;
```

## Where to go next

- Language core: docs 02–19 (lexical structure through the object model)
- Builtin library: docs 20–31 (core/collection/string/time/math/crypto/data builtins)
- Server & runtime features: docs 29–31, 39 (server/SSE, scheduler, reactive, tools)
- Optional plugin packages: docs 32–38, 47–49 (database, integrations, PDF, secrets, YAML, templates, XQL, email/phone/IP, money/dates/IDs, type inference)
- Tooling: docs 40–41 (REPL, spltool, LSP, VS Code)
- Embedding & operations: docs 42–46 (embedding API, runtime/plugins, security/sandbox, playground, config)

Every code sample in this documentation set was run against a locally built
`cmd/interpreter` binary while writing these docs, unless
marked otherwise (some integration examples that require live network/SMTP/DB
endpoints are shown as reference snippets rather than verified runs).
