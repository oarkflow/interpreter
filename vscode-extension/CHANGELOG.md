# Changelog

All notable changes to the SPL VS Code extension are documented here.

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
