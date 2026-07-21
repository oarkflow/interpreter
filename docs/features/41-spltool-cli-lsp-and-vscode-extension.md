# 41 — spltool CLI, LSP & VS Code Extension

Source: `cmd/spltool/{main.go,cli.go,lsp.go,tooling_aliases.go}`,
`pkg/tooling`, `vscode-extension/`.

`spltool` is the standalone developer-tooling binary: format/check,
package management, testing, the daily-tools family, IDE-support JSON
surfaces, and a JSON-RPC LSP server.

## `spltool fmt` / `spltool check`

```bash
spltool fmt [-w] [-json] [files... | -]
spltool check [-json] [files... | -]
```

```text
$ spltool fmt fmt_test.spl
let x=1;
print x;
```

`check` runs parser diagnostics plus conservative static analysis:
undefined identifiers, suspicious shadowing, unreachable statements after
`return`/`throw`/`break`/`continue`, missing/incorrect imports, deprecated
builtins (currently just `puts`), and non-exhaustive `match` over known ADT
types. `-w` writes formatted output back to the file(s); both default to
stdin (`-`) if no files are given, and exit non-zero on any error-severity
diagnostic.

## `spltool mod init|tidy|verify`

See doc 14 (Package Manifests) for the full `spl.mod`/`spl.lock` workflow.

## `spltool config init|show`

```bash
spltool config init [path]   # writes a DefaultProjectConfig() to spl.config.json
spltool config show [path]   # prints the discovered/default config as JSON
```

```text
$ spltool config show
{
  "config": {
    "runtime": {"max_depth": 256, "max_steps": 2000000, "max_heap_mb": 256, ...},
    "security": {"profile": "trusted"},
    "tooling": {"undefined_variables": true, "shadowing": true, "unreachable": true},
    "test": {"patterns": [...]}
  }
}
```

`LoadProjectConfig` walks up parent directories looking for
`spl.config.json`; unset fields fall back to these defaults.

## `spltool symbols` / `complete` / `hover` / `docs`

```bash
spltool symbols [--json] [files...]
spltool complete --prefix <text> [file]
spltool hover --line <n> --col <n> [file]
spltool docs [files...]
```

```text
$ spltool symbols fmt_test.spl
[{"name":"x","kind":"variable","path":"fmt_test.spl","line":1,"column":5}]

$ spltool hover --line 1 --col 5 fmt_test.spl
{"name":"x","kind":"variable","line":1,"column":5}

$ spltool docs fmt_test.spl
# fmt_test.spl
## Variables
- `x` _(line 1)_
```

These are stable JSON surfaces meant to back IDE/LSP integrations —
`complete`'s items include full builtin documentation strings (signature,
description, which std modules expose it, an example snippet).

## `spltool test` / `spltool conformance`

```bash
spltool test [-json] [-filter <substr>] [-profile trusted|untrusted] [targets... default "."]
spltool conformance [-json] [-profile trusted|untrusted] [targets... default "testdata/conformance"]
```

Discovers `*_test.spl` files and anything under a `tests/` directory,
running each with `MaxSteps: 2,000,000`, `MaxDepth: 256`, `Timeout: 10s`,
printing `PASS/FAIL <path> (<ms>)` plus a summary; exits 1 on any failure.
`conformance` is the same runner defaulting its target to
`testdata/conformance` — a project-specific canonical language-compatibility
corpus that may or may not exist in every checkout (add cases there when
changing parser/evaluator/module/builtin behavior).

## `spltool session run|debug`

```bash
spltool session run --json --checkpoint baseline examples/all_in_one.spl
spltool session debug --json examples/all_in_one.spl
```

`session run` builds a `Runtime` + `Session`, executes each target file,
optionally saves a named checkpoint, and (with `--json`) reports
`{ok, output, results, checkpoint, session}` where `session` is the full
`sess.Inspect()` snapshot (every variable's rendered value, e.g.
`"class BankAccount { methods=2 }"` for a class, `"builtin function"` for a
builtin). `session debug` steps through a file statement-by-statement via
`sess.Debug(...)`.

## Daily-tools subcommands

```bash
spltool files rename ./photos --match '*.jpg' --template '{date}_{seq}.{ext}'
spltool files organize ./downloads ./downloads/by-type --apply
spltool files checksum ./docs/report.pdf
spltool files search ./photos --match '*.jpg'
spltool files dedupe ./photos
spltool files move|copy <src> <dst> [--apply]
spltool files remove <path> [--recursive] [--apply]
spltool archive compress ./docs backup.zip --format zip --apply
spltool archive extract backup.zip ./restore --apply
spltool archive list backup.zip
spltool image convert ./photos ./web --to png --apply
spltool image resize ./photo.jpg ./photo-small.jpg --width 1200 --apply
spltool image crop|thumbnail|info ...
spltool secrets generate --length 24 [--token]
spltool secrets encrypt|decrypt <src> <dst> --passphrase '...' --apply
spltool office read ./data/people.csv --json
spltool office text ./report.docx
spltool media info ./clip.mov
spltool media ffmpeg-status
spltool media convert input.mov output.mp4 --install --apply
spltool media install-ffmpeg --apply
```

Every mutating subcommand defaults to a dry-run preview; add `--apply` to
actually perform the operation. `--json` emits machine-readable output
instead of the human-readable one-line-per-operation format.

## `spltool lsp` — JSON-RPC language server

```bash
spltool lsp             # prints a JSON capability blob, doesn't start a server
spltool lsp --stdio     # runs a full JSON-RPC 2.0 LSP server over stdin/stdout
```

Supported LSP methods: `initialize`, `initialized`, `shutdown`, `exit`,
`textDocument/didOpen|didChange|didSave|didClose`,
`workspace/didChangeWatchedFiles`,
`textDocument/completion|hover|definition|references|documentSymbol|formatting`,
`workspace/symbol`, plus SPL-specific extensions: `spl/evaluate`,
`spl/sessionCheckpoint`, `spl/sessionRestore`, `spl/sessionInspect`,
`spl/refreshIndex`. Diagnostics are pushed via
`textDocument/publishDiagnostics` on open/change/save.

`spl/evaluate` runs code through a cached `*interpreter.Session` (keyed by
path + profile + options) using a `CapabilityPreset`-derived security
policy; a `"native"` evaluation profile maps to `untrusted` plus an
auto-allow for the `exec` capability and `native/os` module, specifically to
support the VS Code extension's native-OS snippets/commands.

## VS Code extension (`vscode-extension/`)

Contributes:

- Language registration for `.spl` (TextMate grammar `syntaxes/spl.tmLanguage.json`,
  `language-configuration.json`: line comments `//`, block comments `/* */`,
  bracket/quote pairs including backtick, `// #region`/`// #endregion` folding).
- 7 code snippets (`snippets/spl.json`): `native-os-import`, `native-os-run`,
  `native-os-platform`, `native-os-capabilities`, `tools-file-finder`,
  `tools-file-search`, `db-query-filters`.
- Commands: `spl.runFile`, `spl.evaluateSelection`, `spl.sessionCheckpoint`/
  `Restore`/`Inspect`, `spl.toolsFfmpegStatus`, `spl.toolsInstallFfmpeg`,
  `spl.toolsPreviewBulkRename`, `spl.insertNativeOSExample`,
  `spl.restartLanguageServer`, `spl.showOutput`.
- Settings: `spl.toolPath`, `spl.serverMode` (`auto|toolPath|goRun`),
  `spl.evaluation.profile` (`untrusted|native|trusted`, default
  `untrusted`), `timeoutMs` (1500), `maxOutputBytes`/`maxExecOutputBytes`
  (65536), `allowedCapabilities`, `allowedExecCommands`,
  `allowedNativeModules` (default `["native/os"]`), `deniedNativeModules`.
- LSP client wiring (`src/extension.ts`): spawns the configured
  `spl.toolPath` binary with `['lsp', '--stdio']`, or falls back to
  `go run ./cmd/spltool lsp --stdio` if no tool path is configured.

Install/build the extension from `vscode-extension/` per its own
`README.md` (`npm install`, `npm run compile`, then load it as an
unpacked/dev extension in VS Code).

### `cmd/spltool` vs `cmd/spltool-full`

The CLI/LSP implementation itself lives in the importable package
`pkg/spltoolcli` (`spltoolcli.Run(args, stdin, stdout, stderr)`); both
binaries below are thin `main.go` wrappers around it, so they behave
identically except for which builtins are linked in and therefore visible
to completion, hover, and the "undefined identifier" diagnostic (both of
those read the global `eval.Builtins`/`eval.BuiltinNames()` registry at
runtime, populated by each blank-imported package's `init()` -
`pkg/tooling/tooling.go` has no per-builtin knowledge, so anything
registered in the running process is picked up automatically):

- **`cmd/spltool`** (part of the root module, package main) - the
  lightweight default. It only has the root module's own builtins (core
  language, `pkg/builtins/*` reactive/scheduler/tools/watcher, etc.). It
  cannot import `github.com/oarkflow/interpreter/plugins` - that's a
  separate Go module that itself depends back on the root module, so a
  direct import would be a module cycle.
- **`cmd/spltool-full`** (its own Go module, mirroring `cmd/interpreter`'s
  module setup) - the same `pkg/spltoolcli` implementation, but also
  blank-imports `github.com/oarkflow/interpreter/plugins`, which pulls in
  every optional plugin: the HTTP server/router builtins, `xql`, `images`,
  `database`, `money`, `phone`, `ip`, `wuid`, `naturaldate`, `pdf`, extra
  `crypto` helpers (including `securetoken`), `yaml`, `template`, `secretr`,
  `integrations`, `metadata`, `shamir`, and `emailvalidator`. Because
  completion/diagnostics are registry-driven, none of those needed any
  per-builtin tooling code to show up - linking the package in is
  sufficient.

If your `.spl` scripts use any plugin builtin, point the VS Code
extension's `spl.toolPath` setting (see above) at a built
`cmd/spltool-full` binary instead of relying on the default
`go run ./cmd/spltool lsp --stdio` fallback, e.g.:

```sh
go build -o spltool-full ./cmd/spltool-full   # from the repo root
# or: cd cmd/spltool-full && go build -o spltool-full .
```

then set `spl.toolPath` to the resulting binary's path. Otherwise the
language server will flag plugin builtins (`route`, `server`, `xql_run`,
`money_format`, ...) as "undefined identifier" and won't offer them in
completion, even though they work fine at runtime via `cmd/interpreter`.
