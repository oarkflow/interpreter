# 44 — Security Policy & Sandboxed Execution

Source: `pkg/security/security.go`, `pkg/sandbox/sandbox.go`, `untrusted.go`,
`presets_plugins.go`. This is the model that lets a host run untrusted or
semi-trusted SPL scripts safely.

## Execution profiles

- **`trusted`** (default): preserves existing CLI/embedding behavior —
  effectively unrestricted beyond the sandbox's own default resource limits.
- **`untrusted`**: routes execution through the untrusted worker path with
  strict defaults — `StrictMode + ProtectHost`, only `filesystem_read`
  allowed (rooted at the script/module directory), and tighter resource
  limits (`MaxDepth=128, MaxSteps=500_000, MaxHeapMB=64,
  MaxOutputBytes/MaxHTTPBodyBytes/MaxExecOutputBytes=64KiB, Timeout=2s`).

```bash
spl-interpreter --profile untrusted script.spl
```

## Verified: `untrusted` denies host mutation by default

```spl
let ok, err = write_file("hostile.txt", "pwned");
print err;
// capability denied by host protection policy: filesystem_write

let r, e = http_get("http://example.com");
print e;
// network policy denied request: capability denied by host protection policy: network

let out = exec("echo", "hi", 1000);
// ERROR: exec denied by host protection policy
```

Reading a file that exists inside the script's own directory still works
(`filesystem_read` is allow-listed by default under `untrusted`).

## CLI allow-list flags

```
--profile trusted|untrusted
--require-os-isolation
--allow-in-process-fallback
--allow-cap <csv>            # capability allowlist, e.g. "network,db"
--allow-exec <csv>
--allow-network <csv>        # network HOST allowlist
--allow-db-driver <csv>
--allow-db-dsn <csv>
--allow-read <csv>
--allow-write <csv>
--allow-import-path <csv> / --deny-import-path <csv>
--allow-import-package <csv> / --deny-import-package <csv>
--deny-dynamic-imports
```

> **Verified gotcha**: `--allow-network <hosts>` alone does **not** grant
> network access under `--profile untrusted` — it only populates the host
> allowlist. The `network` **capability** must also be granted, either via
> `--allow-cap network` or by using a preset that already includes it (the
> `networked` preset, doc 43). Confirmed:
> ```bash
> # still denied — capability not granted:
> spl-interpreter --profile untrusted --allow-network example.com script.spl
> # works — capability + host both granted:
> spl-interpreter --profile untrusted --allow-cap network --allow-network example.com script.spl
> ```

## `--require-os-isolation`

```bash
spl-interpreter --profile untrusted --require-os-isolation script.spl
```

Only implemented on Linux via `bubblewrap` (`bwrap`). **Verified fail-closed
behavior on macOS**:

```text
ERROR: OS isolation is only implemented on linux
```

On Linux, this wraps the untrusted worker subprocess as:

```text
bwrap --die-with-parent --unshare-net --unshare-pid --unshare-uts --unshare-ipc \
      --new-session --proc /proc --dev /dev --tmpfs /tmp \
      --ro-bind <absModuleDir> <absModuleDir> --chdir <absModuleDir> \
      <workerCommand...>
```

Full network/PID/UTS/IPC namespace isolation, a fresh session, minimal
`/proc`/`/dev`, a writable `tmpfs` at `/tmp`, and the module directory
bind-mounted **read-only**. If `bwrap` isn't on `PATH`, this fails closed
rather than silently degrading — leave `--allow-in-process-fallback` unset
(false) for genuinely untrusted workloads so a missing `bwrap` is a hard
error, not a silent downgrade to no isolation.

## Environment variables

| Env var | Effect |
|---|---|
| `SPL_SECURITY_MODE=strict` | default-deny for file/network/db/exec unless explicitly allowed |
| `SPL_PROTECT_HOST=1` | disables host-mutating capabilities (`exec`, `write_file`, `remove_file`, `os_env(key,value)`, `exit()`) |
| `SPL_ALLOW_ENV_WRITE` | controls whether `os_env(key, value)` can mutate env vars |
| `SPL_EXEC_ALLOW_CMDS` / `SPL_EXEC_DENY_CMDS` | exec command allow/deny lists |
| `SPL_NETWORK_ALLOW` / `SPL_NETWORK_DENY` | network host allow/deny lists |
| `SPL_DB_ALLOW_DRIVERS` / `SPL_DB_DENY_DRIVERS`, `SPL_DB_DSN_ALLOW` / `SPL_DB_DSN_DENY` | DB driver/DSN allow/deny lists |
| `SPL_FILE_READ_ALLOW` / `SPL_FILE_READ_DENY`, `SPL_FILE_WRITE_ALLOW` / `SPL_FILE_WRITE_DENY` | file access allow/deny lists |
| `SPL_IMPORT_PATH_ALLOW` / `SPL_IMPORT_PATH_DENY`, `SPL_IMPORT_PACKAGE_ALLOW` / `SPL_IMPORT_PACKAGE_DENY`, `SPL_IMPORT_DENY_DYNAMIC` | import restrictions |
| `SPL_BLOCK_HARDCODED_SECRETS` | rejects scripts whose source looks like a hardcoded credential (requires `plugins/secretr` linked, doc 35) |

`security.LoadSecurityPolicyFromEnv()` reads all of these into a
`*SecurityPolicy`, but it is only consulted when
`security.ActiveSecurityPolicy()` has **no active process-wide override**
in effect for the current call.

> **Fixed, verified**: a normal script run (`spl-interpreter script.spl`,
> default `trusted` profile, no `--profile` flag) now honors
> `SPL_SECURITY_MODE=strict` and `SPL_PROTECT_HOST=1` — setting
> `SPL_PROTECT_HOST=1` denies `exec(...)` under the default trusted CLI
> path. This previously did not work: `DefaultExecSandboxConfig`
> (`pkg/sandbox/sandbox.go`) and `CapabilityPreset`'s `"trusted"` branch
> (`presets_plugins.go`) hardcoded a permissive policy
> (`AllowEnvWrite: true`, no `ProtectHost`) without ever consulting these
> env vars; they now apply the same hardening
> `security.LoadSecurityPolicyFromEnv` reads (see
> `applyEnvSecurityHardening` in `pkg/sandbox/sandbox.go`). See
> `TestSPLProtectHostEnvVarRestrictsDefaultTrustedExec` and
> `TestCapabilityPresetTrustedSyncsPolicyWithEnvHardening` in
> `tests/exec/env_security_hardening_test.go`. An explicit
> `ExecOptions.Security`/`SecurityPolicy` passed by the embedder (doc 42)
> still takes precedence over ambient env vars, unchanged — `--profile
> untrusted` (verified above) remains the strongest guarantee for
> genuinely hostile input.

## `permissions(policyHash)` builtin (in-script)

```spl
permissions({"strict": true, "allow_exec": ["echo"], "deny_http": ["*"]});
```

Sets `env.SecurityPolicy` on the current environment (`strict`,
`protect_host`, `allow_env_write`, `allow_exec`/`deny_exec`,
`allow_http`/`deny_http` keys). Requires the `policy` capability and is
itself denied under `ProtectHost`. Treat this as a coarse, best-effort,
in-script policy adjustment rather than a hard security boundary — prefer
CLI flags / `ExecOptions.Security` for guarantees that must hold regardless
of script content.

## Capability model

Constants: `async, db, env_read, env_write, exec, filesystem_read,
filesystem_write, network, policy, process_exit, scheduler, server,
secrets, system, watch`. Under `ProtectHost`, every capability except
`filesystem_read` is denied unless explicitly present in
`AllowedCapabilities`. `EnvWriteAllowed` always refuses to mutate
`SPL_`-prefixed vars or dynamic-linker variables
(`PATH`, `LD_PRELOAD`, ...) regardless of policy — these are hardcoded,
non-overridable protections.

## `ExecErrorKind` (embedding-level)

`ExecErrorPolicyDenied`, `ExecErrorResourceLimit`, `ExecErrorTimeout`,
`ExecErrorCancelled` (plus `ExecErrorParser`/`Validation`/`IO`/`Runtime`) —
let a host distinguish *why* execution stopped and react accordingly
(retry, alert, bill differently). See doc 42.

## Sandbox VM defaults by execution path

| Path | Defaults |
|---|---|
| REPL | strict policy + host protection + bounded runtime limits, always |
| `Exec`/`ExecFile` (trusted) | bounded sandbox, host mutation allowed unless restricted by policy |
| `ExecWithOptions(Profile: "untrusted")` | strict defaults: max source size, output caps, lower runtime limits, host protection, worker-process execution |

Module/file access is always rooted to the sandbox base directory
(`ModuleDir` for embedding, the file's directory for `ExecFile`/CLI runs).

## Module lock verification

```bash
spltool mod verify
```

Confirms locked dependency content (`spl.lock`, doc 14) hasn't changed
since `spltool mod tidy` — run this in CI/deployment gates.

## Production checklist

See `docs/PRODUCTION_CHECKLIST.md` for the full pre-deployment checklist
(capability allowlists, resource limits, OS isolation, `spltool mod
verify`, exercising denied-exec/denied-write/timeout scenarios).
