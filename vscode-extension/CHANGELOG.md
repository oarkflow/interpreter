# Changelog

All notable changes to the SPL VS Code extension are documented here.

## 0.3.0

### Fixed

- **The language server process was never killed on stop/restart/reload**,
  because the custom stream-based `ServerOptions` only hands the client
  library a `{reader, writer}` pair, not the underlying `ChildProcess` — the
  library has nothing to manage lifecycle-wise beyond closing those streams.
  Each restart (including a plain window reload) could leave a `go run
  ./cmd/spltool lsp --stdio` process orphaned in the background. The
  extension now tracks the spawned process itself and explicitly kills it
  on stop, restart, and deactivate; `client.stop()` failures (e.g.
  "Stopping server timed out") no longer prevent that cleanup or block a
  restart.

### Added

- **Inline CodeLens actions on every `.spl` file**, shown above the first
  line the same way `go.mod` shows "Check for upgrades | Upgrade direct
  dependencies": `▶ Run`, `Evaluate Selection`, and `Restart Language
  Server`, wired to the existing `spl.runFile` / `spl.evaluateSelection` /
  `spl.restartLanguageServer` commands so they're one click away instead of
  needing the command palette.
- **`SPL: Clear Output` command.** `Run Current File` and `Evaluate
  Selection` also now clear the SPL output channel before printing a new
  result, instead of appending underneath every previous run.

## 0.2.0

### Fixed

- **`import "secretr" as secretr;` (and other plugin-only modules) showed
  as `undefined identifier` / `import path ... was not found`.** This was a
  language-server bug (`pkg/tooling`), not an extension bug: the static
  checker's known-module list predated the plugin system and was never
  updated for `secretr`, `pdf`, `money`, `phone`, `ip`, `wuid`, `shamir`,
  `naturaldate`, `rules`, `tcpguard`, `emailvalidator`, `server`, `lua`,
  `metadata`, or `securetoken`. The checker now also consults the
  interpreter's runtime module registry, so it stays correct as new
  modules are added instead of needing a matching manual edit every time.
- **Syntax highlighting was missing several real keywords**: `yield`,
  `macro`, `for_await`, `select`, `spawn`, `abstract`, `extends`, `super`
  (plus `this`, which isn't a reserved word at the lexer level but is
  still special) now highlight correctly.
- **Plugin builtins had no syntax highlighting at all** (`secretr_get`,
  `pdf_protect`, `money_new`, `phone_parse`, `xql_run`, `lua_eval`,
  `yaml_encode`, `naturaldate_parse`, `rules_evaluate`, `tcpguard_load`,
  `securetoken_encrypt`, `wuid_new`, `shamir_split`, `ip_lookup`,
  `email_validate`, `infer_csv_types`, and similar) - they fell through to
  the generic "any identifier followed by `(`" function-call highlighting
  instead of being recognized as builtins. Added prefix-based matching for
  these so new additions to a plugin module don't need a grammar update.

### Changed

- Reinstalled and pinned the dev toolchain (`npm install` / `npm audit
  fix`) - the checked-in `node_modules` had a broken/incomplete
  `typescript` install (`tsc` was missing entirely) and a high-severity
  transitive `brace-expansion` advisory; both are resolved.

## 0.1.0

Initial release: syntax highlighting, language server integration (via
`spltool lsp --stdio`), manual evaluation commands, session
checkpoint/restore/inspect, and FFmpeg/bulk-rename tooling commands.
