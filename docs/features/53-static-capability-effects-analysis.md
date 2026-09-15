# 53 — Static Capability/Effects Analysis (`spltool check --effects`)

Source: `pkg/security/effects.go`, `pkg/tooling/effects.go`,
`pkg/spltoolcli/cli.go` (`runCheck`/`runCheckEffects`).

Capability enforcement (see doc 44) happens at **runtime**: a builtin calls
`security.CheckCapabilityAllowed` (or one of its wrapper helpers) and the
call fails if the active `SecurityPolicy` denies it. That tells you nothing
about a script until you run it. This feature adds a **static** pass that
answers "what might this script try to do" — network access, filesystem
reads/writes, subprocess exec, database access, secrets access, dynamic
imports, scheduler/async/server/watch/process-exit usage — by walking the
parsed AST and matching call expressions against a builtin → capability
registry, without executing anything.

## The registry

`security.BuiltinCapabilities` (`pkg/security/effects.go`) maps a registered
builtin name to the `security.Capability*` constant(s) it may exercise, built
by reading every `security.Check*Allowed`/`CheckCapabilityAllowed` call site
in `pkg/builtins/**`, `pkg/render`, and `plugins/**`. It is intentionally not
guaranteed 100% exhaustive — extend it whenever a new builtin gains a
capability check. `TestAllCapabilitiesAreRegistered` (in
`pkg/security/effects_test.go`) fails the build if a `Capability*` constant
has no entry at all, so a newly introduced capability can't silently go
unmapped.

Module-call resolution (`import "pdf" as pdf; pdf.to_docx(...)`) is handled
by `security.ResolveModuleBuiltinName`, using the same prefix convention as
`RegisterStdBuiltinModuleWithPrefix` in `presets_plugins.go` (so
`pdf.to_docx` resolves to the registry key `pdf_to_docx`, `db.query` to
`db_query`, etc.).

Dynamic imports (`import some_variable;` or any import whose path isn't a
string literal) are flagged as their own synthetic category,
`"dynamic-import"` — mirroring the `DenyDynamicImports` runtime check in
`pkg/eval/eval.go`'s `evalImportStatement`, which denies exactly the same
non-literal-path imports when that policy flag is set.

## CLI usage

```bash
spltool check --effects script.spl
spltool check --effects --json script.spl   # machine-readable
```

Human-readable output groups by capability, then lists each usage:

```
script.spl
  exec:
    - native/os.run (line 6, col 4)
  filesystem_read:
    - pdf_to_docx (line 7, col 5)
  filesystem_write:
    - write_file (line 5, col 1)
    - pdf_to_docx (line 7, col 5)
  network:
    - http_get (line 4, col 12)
  dynamic-import:
    - import (line 10, col 1)
```

`--effects` replaces the ordinary lint diagnostics for that invocation
rather than adding to them (`spltool check` without `--effects` is
unchanged). Multiple files may be given; each gets its own section.

This is meant for a "what does this script need before I run it" workflow —
e.g. a CI or admission-control step that runs `spltool check --effects
--json untrusted.spl` and rejects scripts that request capabilities beyond
an allowlist, before ever executing them under a `SecurityPolicy`.
