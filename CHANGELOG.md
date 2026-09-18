# Changelog

All notable changes to this project are documented here. This project does
not yet publish tagged releases with a formal support matrix (see
`SECURITY.md`), so everything below is grouped under **Unreleased**.

> **Note on tags**: the `v0.0.1`–`v0.0.12` git tags in this repository point
> at a lineage with a different root commit than the current `main` branch
> (i.e. the two histories share no common ancestor — this is a full history
> replacement, not an ordinary rebase or divergence). None of those tags are
> reachable from `main`. If you relied on that old tagged history, it is
> preserved at those tag refs, but it is not part of `main`'s history and is
> not reflected below. This was flagged rather than silently resolved —
> reconciling it (e.g. deciding whether any of v0.0.12's unique commits,
> such as its XQL tagged-block-literal feature, VS Code extension work, or
> REPL/session improvements, should be reintroduced) needs a maintainer
> decision, not an automated one.

## Unreleased

### Fixed

- **`SanitizePathLocal` resolved relative paths (`mkdir("pdf_demo")`,
  `file_exists(...)`, and anything built on it, e.g. `pkg/builtins`'s and
  `plugins/pdf`'s file-writing builtins) against the host process's global
  working directory instead of the active sandbox root
  (`sandbox.ActiveSandboxBaseDir()`, already set correctly to the script's
  own module directory by `pkg/session`).** A single OS process has one
  cwd shared across every concurrently-evaluated file, so any host that
  evaluates a script from outside that script's own directory — the LSP
  server chief among them, since it runs from the workspace root — resolved
  relative paths to the wrong place and then rejected them as outside the
  sandbox jail. `SanitizePathLocal` now resolves a relative path against
  the active sandbox root when one is set, falling back to the process cwd
  only outside a sandboxed evaluation (e.g. plain CLI usage from the
  script's own directory).
- **`printf`, `puts`, and `input`'s prompt echo wrote straight to the real
  process `os.Stdout` instead of the running session's configured output
  (`env.Output`).** For any host that captures output — most critically the
  `spltool lsp --stdio` server, whose real stdout *is* the JSON-RPC
  transport — this corrupted the protocol stream mid-message and manifested
  as the VS Code extension logging `Header must provide a Content-Length
  property.` and hanging until the connection was torn down. `print`
  already respected `env.Output`; these three builtins now do too (via the
  existing `FnWithEnv` mechanism), falling back to `os.Stdout` only when no
  session output is configured (e.g. plain CLI usage).
- **`immutable()` values could panic or return misleading errors on read.**
  Two independent, non-interoperable `ImmutableValue` types existed (one in
  `pkg/object`, an unexported duplicate in `pkg/builtins`); reads through a
  frozen array/hash now transparently proxy to the wrapped value (including
  through nested containers, which stay individually frozen), and the
  underlying index-expression evaluators no longer panic on a type-assertion
  mismatch.
- **`async function name() {...}` didn't bind `name` into scope.** The
  named-statement form now binds like a plain `function name() {}`
  declaration, matching what the concurrency docs already described.
- **`go_async()` discarded its result.** It now returns an awaitable
  `Future`, like `go()`.
- **Interface `implements` only checked method names, not
  parameter/return types.** Declaring a class whose method signature
  conflicts with its interface's declared types is now rejected at
  class-declaration time (an untyped implementation of a typed interface
  method is still accepted, to avoid breaking previously-valid code that
  has no annotations).
- **A denial hook set on one `Runtime` could fire for another,
  concurrently-executing `Runtime`.** `Observability.OnPolicyDenied` is now
  scoped per-call instead of being installed as "last write wins"
  process-wide global state.
- **`SPL_PROTECT_HOST`/`SPL_SECURITY_MODE` env vars had no effect under the
  default trusted profile.** `DefaultExecSandboxConfig` and
  `CapabilityPreset`'s `"trusted"` branch previously hardcoded a permissive
  policy regardless of these env vars; they're now honored.
- **`plugins`, `cmd/interpreter`, `cmd/spltool-full`, and `examples/app`
  failed to build.** Their `go.sum` files never picked up the
  `aws-sdk-go-v2`/`mongo-driver` transitive dependencies pulled in through
  the `xql` module.
- **The bytecode VM was entirely unreachable.** A dispatch filter excluded
  every statement type the VM's compiler could actually handle, and
  `BuiltinLookupFn`/`ApplyFunctionFn` were never wired up - so it never ran
  for any real program. Fixed, and extended to cover call expressions, `if`
  expressions, and short-circuit `&&`/`||` (previously rejected outright).
- **`Compress(..., {"password": ...})` always failed.** Password-protected
  zip archives are now supported (AES-256, via `github.com/yeka/zip`), wired
  through `spltool archive compress/extract --password`.
- **WebP encoding always failed.** Lossless WebP encoding now works (via
  `github.com/HugoSmits86/nativewebp`); lossy encoding remains unsupported
  since no pure-Go lossy WebP encoder exists.
- **AES-256 PDF encryption always failed.** `github.com/oarkflow/pdf` is
  bumped to v0.0.4, which implements the Standard Security Handler
  revision 5 algorithm end-to-end (the previous code was non-functional
  placeholder logic with no way to ever recover the file key).

### Changed

- `SECURITY.md`'s vulnerability-reporting section now points solely at
  GitHub Security Advisories; the previous unfilled placeholder email
  (`security@REPLACE-ME.example`) was removed rather than left in place.

### Security

See the "Fixed" items above on `SPL_PROTECT_HOST`/`SPL_SECURITY_MODE` and
denial-hook scoping - both are security-relevant behavior changes for
embedders relying on the trusted-profile default or on
`Observability.OnPolicyDenied`.
