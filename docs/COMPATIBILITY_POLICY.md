# Compatibility Policy

Stable surfaces:

- Go embedding APIs: `Exec`, `ExecWithOptions`, `ExecFile`, `ExecFileWithOptions`
- Core SPL syntax and semantics for existing language constructs
- Builtin names and argument contracts unless marked experimental

Breaking changes require:

- Major version bump
- Migration notes in release notes
- Compatibility section documenting behavior changes

Non-breaking changes:

- Performance improvements
- New builtins
- Additive `ExecOptions` fields
