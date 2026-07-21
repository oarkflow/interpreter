# builtins/secretr

SPL builtins backed by [`github.com/oarkflow/secretr`](https://github.com/oarkflow/secretr)
(private, local-only): a real encrypted secrets vault plus a DLP-based
scanner that can refuse to run scripts embedding hardcoded secrets.

This plugin is only usable in a checkout that also has `secretr` on disk
next to this repo (`../../../secretr` relative to this directory, i.e. a
sibling of the `interpreter` repo root) - its module path is private and
not published anywhere resolvable by `go get`.

## Build with `-tags dev`

secretr's CLI/API surface enforces licensing through `pkg/authz`, gated by
`pkg/securitymode.IsDevBuild()` (a Go build tag, not an env var). Building
with `-tags dev` makes that function return `true`, which is what lets
`pkg/authz.EnvEntitlementProvider` hand back a synthetic full-access license
instead of requiring a real license file - see `pkg/securitymode/mode_dev.go`
and `pkg/authz/provider.go` in the secretr checkout.

This plugin's own code path (`pkg/core/identity`, `pkg/core/secrets`,
`pkg/core/compliance`, `pkg/storage`) does **not** currently go through
`pkg/authz` at all, so it happens to work either way today - but build
every binary that links this plugin with `-tags dev` anyway, so future
secretr versions that extend licensing checks into these packages don't
suddenly require a license file for local/embedded use:

```sh
cd cmd/interpreter-full && go build -tags dev -o ../../bin/interpreter-full .
```

## What this plugin does

- **`secretr_get(name)`** - fetch a secret's value, wrapped in a SPL SECRET
  (masks as `***` on print/log; unwrap with `secret_reveal()`).
- **`secretr_set(name, value)`** - create or update a secret. `value` may be
  a SECRET or a plain STRING.
- **`secretr_delete(name)`** - remove a secret.
- **`secretr_list([prefix])`** - list secret *names* (never values). Only
  top-level secret names are listed - see "Dot notation" below for why a
  nested key like `"app.database.password"` won't show up as its own list
  entry.
- **`secretr_scan(text)`** - run the same DLP scan used automatically by
  `BlockHardcodedSecrets` against arbitrary text, returning
  `[{pattern, redacted, severity}, ...]` findings without rejecting
  anything - useful for checking user-submitted config before storing it.

All five are gated behind the `secrets` capability
(`security.CapabilitySecrets`), which `ProtectHost` policies block by
default like every other host-mutating capability - add `"secrets"` to
`AllowedCapabilities` (or use a trusted profile) to use them.

## Dot notation ("json - dot notation")

`secretr_get`/`secretr_set` pass names straight through to
`secrets.Vault.Create`/`Get`, which already treats any name containing `.`
as addressing a *nested* JSON-like structure (see secretr's
`pkg/core/secrets/vault_nested.go`) - no extra plumbing was needed here:

```spl
secretr_set("app.database.password", "hunter2");
secretr_set("app.database.host", "localhost");
print secret_reveal(secretr_get("app.database.password")); // "hunter2"
```

Both calls above store into **one** top-level secret named `"app"` (a
JSON object with a `database` key inside it) - `secretr_list()` will show
`"app"`, not `"app.database.password"` or `"app.database.host"`
individually. This is secretr's own nested-secret model, not a limitation
of this plugin: dot notation groups related values under one vault entry
rather than creating N independent ones.

## Preventing hardcoded secrets in SPL source

Set `SecurityPolicy.BlockHardcodedSecrets = true` (or the
`SPL_BLOCK_HARDCODED_SECRETS=true` env var, which
`security.LoadSecurityPolicyFromEnv()` reads) and importing this plugin
causes `interpreter.Exec*`, the CLI, and the playground's quick-eval mode to
all refuse scripts whose source text contains something that looks like a
real AWS access/secret key, GitHub token, PEM private key header, or JWT
(the pattern set is intentionally narrow - see `hardcodedSecretPatterns` in
`vault.go` - to keep false positives low; `generic_api_key`, which matches
any 32-64 character alphanumeric run, is deliberately excluded from the
automatic check but still available to `secretr_scan()`):

```spl
// Rejected when BlockHardcodedSecrets is on:
let key = "AKIAABCDEFGHIJKLMNOP";

// Runs fine - fetch it instead:
let key = secretr_get("aws.access_key");
```

With no scanner plugin linked in, `BlockHardcodedSecrets` is a harmless
no-op (see `security.ScanForHardcodedSecrets`) - this plugin is what
actually implements the check.

## Local vault storage

Data lives under `SECRETR_DATA_DIR` (default `./.secretr-data`, relative to
the process's working directory): a `velocity`-backed encrypted store, one
bootstrap identity (created once, persisted to `identity.id`), and a
32-byte vault key (`vault.key`, 0600). This is a local/dev-mode key
management scheme, not a production KMS - see secretr's own docs for
stronger `KeyProvider` implementations if you need one.
