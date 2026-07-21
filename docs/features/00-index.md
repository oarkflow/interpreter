# SPL Interpreter — Feature Documentation Index

This directory contains 49 comprehensive, numbered feature documents
covering the entire `github.com/oarkflow/interpreter` project: the SPL
scripting language, its builtin library, server/runtime features, optional
plugin packages, developer tooling, and the embedding/security/operations
model.

Every code example across these documents was either run directly against
a locally built `cmd/interpreter` (and, where relevant, `cmd/spltool` /
`cmd/spltool-full`) binary while writing them, or is explicitly marked where
live verification wasn't possible (e.g. examples requiring a real
SMTP/FTP endpoint). Several documents call out specific, verified behavioral
nuances that differ from what a first read of the README might suggest —
these are flagged inline with "Verified" callouts.

**Note on binary names:** `cmd/interpreter` is the single, full-featured CLI/
REPL/playground binary this project ships (its own Go module, blank-imports
every package under `plugins/`; the playground is `cmd/interpreter
--playground`). Older notes or issues may reference now-retired
`cmd/interpreter-full`, `cmd/playground`, or `cmd/playground-full` binaries —
those were merged into `cmd/interpreter` by a later refactor; read them as
`cmd/interpreter`. The root `github.com/oarkflow/interpreter` module remains
the lightweight, plugin-free embedding target (doc 42).

## Language Core

1. [Introduction & Quickstart](01-introduction-and-quickstart.md)
2. [Lexical Structure & Literals](02-lexical-structure-and-literals.md)
3. [Variables, Constants & Destructuring](03-variables-constants-destructuring.md)
4. [Operators Reference](04-operators-reference.md)
5. [Control Flow](05-control-flow.md)
6. [Functions & Closures](06-functions-and-closures.md)
7. [Pattern Matching (`match`)](07-pattern-matching.md)
8. [Error Handling](08-error-handling.md)
9. [Classes & Interfaces](09-classes-and-interfaces.md)
10. [Algebraic Data Types](10-algebraic-data-types.md)
11. [Macros](11-macros.md)
12. [Concurrency: Async/Await, Channels, Generators & Streams](12-concurrency-async-channels-generators-streams.md)
13. [Modules: Imports & Exports](13-modules-imports-and-exports.md)
14. [Package Manifests (`spl.mod`/`spl.lock`)](14-package-manifests.md)
15. [Arrays & Hashes](15-arrays-and-hashes.md)
16. [Strings & Template Literals](16-strings-and-template-literals.md)
17. [Optional Typing & Structured Types](17-optional-typing-and-structured-types.md)
18. [Object Model: Ownership & Immutability](18-ownership-and-immutability.md)
19. [Object & Type System Internals](19-object-type-system-internals.md)

## Builtin Library

20. [Core Builtins](20-core-builtins.md)
21. [Collection Builtins](21-collection-builtins.md)
22. [Time & Date Builtins](22-time-and-date-builtins.md)
23. [Math Builtins](23-math-builtins.md)
24. [Crypto & Encoding Builtins](24-crypto-and-encoding-builtins.md)
25. [Formatting & Interpolation](25-formatting-and-interpolation.md)
26. [Filesystem, OS & Process Execution](26-filesystem-os-and-process-execution.md)
27. [Data Values & Image Processing](27-data-values-and-image-processing.md)
28. [Testing Builtins](28-testing-builtins.md)

## Server & Runtime Features

29. [HTTP Server, Routing & SSE](29-http-server-routing-and-sse.md)
30. [Scheduler & File Watching](30-scheduler-and-file-watching.md)
31. [Reactive State](31-reactive-state.md)

## Optional Plugin Packages (require `cmd/interpreter`, or blank-importing the package in your own embedding host)

32. [Database](32-database.md)
33. [HTTP/SMTP/FTP/SFTP Integrations](33-http-smtp-ftp-sftp-integrations.md)
34. [PDF Generation & Editing](34-pdf-generation-and-editing.md)
35. [Secrets Vault & Extra Crypto (secretr, bcrypt, JWT, Shamir secret sharing, securetoken)](35-secrets-and-extra-crypto.md)
36. [YAML Config](36-yaml-config.md)
37. [Template Engine & Directives](37-template-engine-and-directives.md)
38. [XQL Data Pipeline](38-xql-data-pipeline.md)

## Daily Tools & Developer Tooling

39. [Daily Tools (Files/Archive/Images/Media/Office/Secrets/System/Network)](39-daily-tools.md)
40. [REPL: Interactive Use, Debugging & Sessions](40-repl-interactive-use-debugging-and-sessions.md)
41. [spltool CLI, LSP & VS Code Extension](41-spltool-cli-lsp-and-vscode-extension.md)

## Embedding, Runtime & Operations

42. [Embedding API (Go)](42-embedding-api.md)
43. [Runtime Plugin System & Capability Presets](43-runtime-plugin-system-and-capability-presets.md)
44. [Security Policy & Sandboxed Execution](44-security-policy-and-sandboxed-execution.md)
45. [Playground: HTTP API, Configuration & Security](45-playground-http-api-config-and-security.md)
46. [Config Loading & Secrets Masking](46-config-loading-and-secrets-masking.md)

## Data-Validation & Daily-Ops Plugins

47. [Email, Phone & IP Validation](47-email-phone-and-ip-validation.md)
48. [Money, Natural-Language Dates & Sortable IDs](48-money-naturaldate-and-wuid.md)
49. [CSV/JSON Type Inference (metadata)](49-type-inference.md)

## Reading order suggestions

- **New to SPL?** Start at 01, then read 02–08 for the language core before
  jumping to whichever builtin/feature area you need.
- **Building a server-side app?** 01 → 06 → 13 → 29 → 30 → 31 → 37, then
  the relevant plugin docs (32–36, 38) for any external systems you touch.
- **Embedding SPL in a Go host?** 42 → 43 → 44, then skim the language docs
  as a reference for what scripts your host will run.
- **Operating a multi-tenant/untrusted deployment?** 44 → 45 → 14
  (`spltool mod verify`) → `docs/PRODUCTION_CHECKLIST.md` in the parent
  `docs/` directory.
- **Contributing tooling/editor support?** 40 → 41.
- **Validating/cleaning up user-submitted data (forms, CSV imports)?**
  47 (email/phone/IP) → 49 (type inference) → 48 (money) for the
  arithmetic once fields are validated.

## Known nuances discovered during verification

A few behaviors were confirmed to differ from a naive reading of the
project's top-level README; each is documented in place with a "Verified"
note, but the highlights are:

- `--allow-network <hosts>` alone does **not** grant network access under
  `--profile untrusted` — the `network` capability must also be granted via
  `--allow-cap network` (doc 44).
- Bare `SPL_PROTECT_HOST=1`/`SPL_SECURITY_MODE=strict` env vars were not
  observed to restrict a default **trusted**-profile run, because trusted
  execution always installs its own (permissive) sandbox policy override
  first — use `--profile untrusted` or explicit `ExecOptions.Security` for
  guaranteed enforcement (doc 44).
- `config_load`/`config_parse` failures raise a catchable runtime error
  (not a sentinel `"ERROR: ..."` string) — wrap in `try/catch` (doc 46).
- `immutable(...)`'s write-guard works reliably, but reading a property
  back out through the wrapper is unreliable in the current build (doc 18).
- `async function name() {...}` as a bare named statement doesn't bind
  `name`; prefer `let name = async function() {...};` (doc 12).
- `{{var}}`-style templates only render when `plugins/template` isn't
  linked in; `cmd/interpreter` links it, replacing the renderer with the
  `${...}`/`@directive` template engine (doc 37).
- `shamir_split`/`shamir_combine`/`securetoken_encrypt`/`securetoken_decrypt`
  run without any capability grant at all (unlike `secretr_*`, which
  requires the `secrets` capability) — verified under `--profile untrusted`
  with no `--allow-cap` flags (doc 35).
