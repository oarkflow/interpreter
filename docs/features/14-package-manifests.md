# 14 — Package Manifests (`spl.mod` / `spl.lock`)

Source: `interpreter.go` (`InitModuleManifest`, `SyncModuleLock`,
`VerifyModuleLock`), `pkg/pkgmgr`, `cmd/spltool` (`mod` subcommand).

SPL supports a lightweight manifest + lock-file flow for deterministic
**bare** (non-relative) module imports — similar in spirit to `go.mod`/
`go.sum` but far simpler.

## `spltool mod init`

```bash
spltool mod init example/app
```

Writes `spl.mod`:

```json
{
  "module": "example/app"
}
```

## Declaring a dependency

Edit `spl.mod` to add a dependency alias pointing at a local path:

```json
{
  "module": "example/app",
  "dependencies": {
    "mathlib": "./deps/mathlib"
  }
}
```

## `spltool mod tidy`

```bash
spltool mod tidy
```

Resolves every dependency to an absolute path, checksums its contents, and
writes `spl.lock`:

```json
{
  "module": "example/app",
  "dependencies": {
    "mathlib": {
      "source": "./deps/mathlib",
      "resolved_path": "/abs/path/to/deps/mathlib",
      "checksum": "f59df894f731824bfffd9328de5fe69f2da1428287cd97c634aa25fdbc18ca92"
    }
  }
}
```

## Importing via the dependency alias

```spl
import "mathlib/math.spl" as math;
print math.answer;
```

The `mathlib` alias declared in `spl.mod` is resolved by the module loader
without needing a relative (`./` / `../`) path — `mathlib/math.spl` maps to
`<resolved_path>/math.spl`.

## `spltool mod verify`

```bash
spltool mod verify
```

Recomputes each locked dependency's checksum and fails (non-zero exit) if
the on-disk content has drifted from what `spl.lock` recorded — intended for
CI/deployment gates so a locally-tidied lock file can't silently drift from
what's actually shipped.

```text
$ spltool mod verify
verified spl.lock
```

## Recommended workflow

1. `spltool mod init <module-name>` once, at project setup.
2. Add dependency aliases to `spl.mod` as needed.
3. `spltool mod tidy` after any dependency change, and commit both
   `spl.mod` and the regenerated `spl.lock`.
4. `spltool mod verify` in CI/deploy pipelines before running the app.

See doc 41 for the rest of `spltool`'s subcommands.
