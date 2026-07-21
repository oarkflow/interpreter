# Production Checklist

Use this checklist before running SPL where scripts may come from users or
other untrusted sources.

## Execution Profile

- Use `--profile untrusted` for user-submitted code.
- Use `ExecWithOptions` / `ExecFileWithOptions` with `Profile: "untrusted"` for embedding.
- Use `--require-os-isolation` on Linux hosts where `bwrap` is installed and an OS-level boundary is required.
- Keep `--allow-in-process-fallback` disabled for hostile workloads.

## Capability Allowlists

- Grant only the capabilities needed by the workload with `--allow-cap`.
- Prefer read-only file access; use `--allow-read` for explicit roots.
- Allow file writes only for disposable directories with both `--allow-cap filesystem_write` and `--allow-write`.
- Allow network and database access only through explicit host, driver, and DSN allowlists.
- Avoid enabling `exec`, `process_exit`, `policy`, `server`, `scheduler`, `watch`, or `async` for untrusted scripts unless the host process is disposable.

## Runtime Limits

- Set source, step, depth, heap, output, HTTP body, exec output, and timeout limits for production traffic.
- Set object/import limits for hostile workloads: max string bytes, array length, hash entries, import depth, and import count.
- Keep default untrusted limits unless the workload has a measured need for larger bounds.
- Monitor timeout and limit errors; repeated failures usually indicate abusive or incorrectly sized workloads.

## Modules and Extensions

- Run `spltool mod verify` before deployment when using `spl.lock`.
- Allow imports with explicit path/package policy for untrusted workloads.
- Register plugins and standard modules at process startup only; avoid loading extension code from untrusted scripts.
- Use `Runtime` instances to keep profile, limits, plugins, and observability hooks together per host workflow.

## Deployment Surfaces

- `cmd/interpreter` and `cmd/interpreter-full` default to `trusted`; pass `--profile untrusted` explicitly for hardened runs.
- `cmd/spltool test` defaults to `trusted`; pass `--profile untrusted` when testing untrusted-compatible scripts.
- `cmd/splworker` is only the untrusted worker protocol entrypoint.
- The playground defaults to `PLAYGROUND_EXECUTION_PROFILE=untrusted` and host protection.

## Verification

- Run `go test ./...` before release.
- Run `spltool conformance` before release to catch language compatibility regressions.
- Exercise at least one denied `exec`, denied write, filesystem escape, and timeout scenario in the deployment environment.
- If `--require-os-isolation` is part of the deployment, verify startup fails on hosts without `bwrap` and succeeds on hosts with `bwrap`.
