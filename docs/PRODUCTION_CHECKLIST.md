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

## Concurrency Notes

- Security policy overrides (`pkg/security`'s `policyOverride`), the sandbox
  root override (`pkg/sandbox`'s `sandboxRootOverride`), and the denial hook
  (`pkg/security`'s `denialHook`, set via `security.SetDenialHook`, e.g. from
  `Runtime`'s `Observability.OnPolicyDenied`) are all process-wide global
  state, not per-execution/per-Runtime state.
- The policy and sandbox-root overrides hold a mutex for the full duration of
  the guarded evaluation, so concurrent in-process evaluations (multiple
  `Runtime`s, multiple playground requests, etc.) correctly serialize on them
  rather than leaking one execution's policy/root into another's window - but
  that means concurrent evaluations throughput-bottleneck on this lock rather
  than running truly in parallel in-process.
- The denial hook is "last write wins, process-wide": if multiple `Runtime`s
  with distinct denial hooks are constructed concurrently, only the most
  recently installed hook fires for all of them. It is synchronized
  internally (safe to set/read concurrently, no data race), but that
  synchronization does not change this last-write-wins semantic.
- A per-execution-context refactor (threading policy/root/hook through an
  explicit context object instead of process-wide globals) is planned as a
  follow-up to remove the serialization bottleneck and the hook's last-write-
  wins collision; until then, avoid running many differently-configured
  evaluations concurrently in a single process if you need real parallelism
  or per-request denial observability, and consider `--require-os-isolation`
  (separate OS processes) as an alternative isolation boundary.

## Continuous Integration

- `.github/workflows/ci.yml` now runs on every push/PR to `main`:
  - A `test` job matrixed over `ubuntu-latest`, `macos-latest`, and
    `windows-latest` runs `go vet`/`go test` for every module in this repo
    (root, `plugins`, `cmd/interpreter`, `cmd/spltool-full`,
    `examples/app`, `benchmarks/exprcompare`) - via `make vet-all` /
    `make test-race` on Linux/macOS, and direct per-module `go vet`/`go
    test` on Windows (no `-race` there; see the workflow comments for why).
  - A Linux-only `sandbox-bwrap` job installs `bubblewrap` and runs the
    root module's tests with `-tags=ignore`, so the `--require-os-isolation`
    / `bwrap` fail-closed path is actually exercised in CI, not just
    documented.
  - A `govulncheck` job runs `golang.org/x/vuln/cmd/govulncheck` per
    module. It is intentionally `continue-on-error` for now (an initial
    rollout choice, not yet triaged/baselined) - treat it as informative
    until a follow-up tightens it into a hard gate.
