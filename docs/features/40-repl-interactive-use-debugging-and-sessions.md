# 40 — REPL: Interactive Use, Debugging & Sessions

Source: `pkg/repl/repl.go`, `pkg/session/session.go`.

## Starting the REPL

```bash
go -C cmd/interpreter run .   # cmd/interpreter is its own Go module
```

```text
spl> let x = 40;
spl> x + 2
42
```

Every REPL execution round-trips through a `pkg/session.Session`
(profile `trusted`, 256-event ring buffer) created lazily per REPL
environment — this is what backs `:checkpoint`/`:restore`/`:replay`/
`:inspect`/`:metrics`/`:events` below.

## Verified interaction

```text
>> let x = 40;
>> x + 2
42
>> :vars
  x = 40
>> :type x
INTEGER
>> :checkpoint base
checkpoint saved: base
>> x = 99
99
>> :restore base
restored: base
>> x
40
```

## Meta-command reference

| Command | Purpose |
|---|---|
| `:help` | keybindings + full command list |
| `:tips` | practical usage tips |
| `:commands [query]` | table of all commands |
| `:palette <query>` | fuzzy search across commands, builtins, variables, workspace symbols |
| `:examples` | list runnable example scripts |
| `:tools` | list `tools/*` daily-chore modules with preview-first examples |
| `:hint <expr\|name>` | hover-style doc |
| `!<shell command>` | run a shell command |
| `:builtins` | list all builtin names |
| `:history` | numbered editor history |
| `:clear` | clear screen |
| `:search <text>` | substring search over completion candidates |
| `:vars` | list all environment variables with values |
| `:reset` | clear all variables and the REPL's session |
| `:checkpoint [name]` | save a session snapshot |
| `:restore <name>` | restore a saved snapshot |
| `:replay` | replay all recorded input since the `"initial"` checkpoint |
| `:inspect [name]` | show one variable, or the full session state |
| `:metrics` | per-execution duration/steps/output/result-type |
| `:events` | last 20 session events |
| `:trace on\|off` | toggle a trace display flag |
| `:ask` / `:explain` / `:fix` | query a configured AI assistant provider (host-specific; errors if none configured) |
| `:type <expr>` | evaluate and print the expression's type |
| `:doc <name\|expr>` | show builtin/symbol/object documentation |
| `:diagnostics [source]` | static diagnostics for given/typed-so-far source |
| `:symbols [query]` | list REPL + workspace symbols |
| `:def <name>` | jump to a symbol's definition |
| `:refs <name>` | find references to a symbol |
| `:format [source]` | format source |
| `:methods <expr>` | list dot-methods available on a value |
| `:fields <expr>` | list fields available on a value |
| `:ast <expr>` | dump the parsed AST |
| `:time <expr>` | evaluate and print elapsed wall time |
| `:debug <expr>` | interactive step-debugger (see below) |
| `:mem` | Go runtime memory stats |
| `:load <file>` | read and evaluate a file |
| `:reload [file]` | no arg: clear the whole module cache; with arg: invalidate just that module + fire hot-reload hooks |
| `:rename <pattern> <replacement> [dir] [--apply]` | preview/apply a bulk rename (`pkg/renamer`) |
| `:move <src> <dst> [--apply]` | preview/apply a file move |
| `:install <alias> <path>` | add a dependency to `spl.mod`, sync `spl.lock` |
| `:config ...` | runtime/security config viewer/editor (see below) |

## `:methods`/`:doc` example

```text
>> :doc sprintf
**core builtin** `sprintf`

Signature: `sprintf(format, ...args)`

formats values with printf-style placeholders; supports %T for SPL type

Available via: `std/core`, `core`.

>> :methods "hi"
methods:
- at
- camel_case
- charAt
- ends_with
- ...
```

## `:config` subcommands

```text
:config                          # print full config table
:config get <key>
:config set <key> <value>
:config profile trusted|untrusted
:config reset
:config load <file> [json|yaml|env]
```

`:config profile untrusted` applies hardened defaults: strict mode + host
protection, denies `async, db, env_write, exec, filesystem_write, network,
policy, process_exit, scheduler, server, watch`, and caps recursion/steps/
heap/timeout/output size. Configurable keys span
`execution.profile`, `module.dir`, `security.*` (strict, protect_host,
allow_env_write, allow/deny capabilities/exec/native/network/db-drivers/
db-dsn/file-read/file-write), `runtime.*` (max_depth, max_steps,
max_heap_mb, timeout_ms, max_output_bytes, max_http_body_bytes,
max_exec_output_bytes), and `render.*` (mode, terminal_protocol, max_bytes,
allow_urls, allow_url_hosts).

## `:debug` — interactive step debugger

```text
>> :debug someExpressionOrBlock
dbg> step      # (or "next"/"n"/"s"/empty) execute one statement
dbg> locals    # (or "vars") dump current environment
dbg> break 3   # set a breakpoint at statement index 3
dbg> continue  # (or "c") run to next breakpoint or error
dbg> quit      # (or "q"/"exit")
```

Debug executions use the evaluator directly rather than going through the
session, so they are **not** recorded in session history/`:replay`.

## Session workspace concepts (`pkg/session`)

- **Checkpoints**: named snapshots of the environment's variable store plus
  history length (`:checkpoint`/`:restore`), auto-created as `"initial"` at
  session start.
- **Replay**: resets to the `"initial"` checkpoint and re-executes every
  previously recorded input source in order (`:replay`).
- **Events**: a ring buffer (default 256) of
  `execution.start/finish, output, diagnostic, metric, trace,
  render_artifact, error, debug, checkpoint, restore, replay` events
  (`:events`).
- **Metrics**: per-execution `Duration, Steps, OutputBytes, ResultType,
  Error, ErrorKind` (`:metrics`).
- **Cancellation**: `Session.Cancel()` stops an in-flight execution from
  another goroutine — useful for a host exposing an admin "kill this
  session's current job" action (see doc 44 for the embedding-level API).
- **Persistence** (opt-in, `Persist: true`): writes `metadata.json`, an
  append-only `inputs.spl` execution log, and `checkpoints/<id>.json` under
  `.spl/sessions/<id>/`.

## Completion, call tips, and history

- Tab completion is semantic: after a `.`, it evaluates the base expression
  and offers that value's actual fields/methods; otherwise it merges REPL
  commands, builtins, keywords, env vars, and workspace symbols.
- Inline "ghost text" suggestions render in gray as you type.
- Call tips show the active function/builtin's signature and per-argument
  hint while inside a call's parentheses.
- History persists across sessions at `~/.interpreter_repl_history`; `Ctrl-R`
  does a reverse history search.
- Multiline input is auto-detected (unterminated strings, unbalanced
  brackets, trailing operators/keywords, or a parser "unexpected EOF")
  and prompts with a `..` continuation line.

## CLI equivalents for session workflows

```bash
spltool session run --json --checkpoint baseline examples/all_in_one.spl
spltool session debug --json examples/all_in_one.spl
```

See doc 42 for the full `spltool` CLI reference.
