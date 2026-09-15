# Security Policy

## Supported Versions

This project does not yet publish tagged releases with a formal support
matrix. Until a versioned release process exists, security fixes are applied
to the `main` branch only.

| Version | Supported          |
| ------- | ------------------ |
| main    | :white_check_mark: |
| other   | :x:                |

This table will be updated once tagged releases begin.

## Reporting a Vulnerability

**Placeholder — maintainer to fill in a real contact before publishing this
policy externally.** Until then, please report suspected vulnerabilities
using one of the following:

- Open a [GitHub Security Advisory](https://github.com/oarkflow/interpreter/security/advisories/new)
  on this repository (preferred — keeps the report private until a fix is
  available).
- Or email **security@REPLACE-ME.example** (placeholder address — replace
  with a monitored maintainer/security contact before relying on this).

Please do not open a public GitHub issue for suspected vulnerabilities.

When reporting, include:

- A description of the vulnerability and its potential impact.
- Steps to reproduce (a minimal SPL/script snippet, CLI invocation, or code
  sample is ideal).
- The execution profile in use (`trusted` vs `untrusted`), whether
  `--require-os-isolation` / `bwrap` was enabled, and which entrypoint was
  involved (`cmd/interpreter`, `cmd/spltool`, `cmd/splworker`, embedding via
  the Go API, etc.).

### Expected Response Times

These are targets, not guarantees, given this is currently a small/unfunded
project:

- **Acknowledgment:** within 5 business days of a report.
- **Initial assessment / triage:** within 10 business days.
- **Fix or mitigation timeline communicated:** within 30 days of
  acknowledgment, depending on severity and complexity.

Critical remote-code-execution or sandbox-escape issues affecting the
`untrusted` execution profile will be prioritized over lower-severity
findings.

## Scope and Known Boundaries

This project is a scripting language interpreter designed to run
potentially untrusted code, so its security model is central to its design.
Please read this section before reporting, as some behaviors described here
are **known current limitations rather than novel findings** — they are
still useful to report if you find a concrete exploit, but are not
surprises.

- **Sandbox isolation model.** The interpreter offers a `trusted` execution
  profile (full host access, intended for scripts you already trust) and an
  `untrusted` profile (capability allowlists, resource/step/heap/timeout
  limits, and restricted I/O). The `untrusted` profile is a
  language-level sandbox: it restricts what the interpreter's built-in
  APIs expose, but it is **not** currently a hardened boundary suitable for
  running fully hostile, adversarial code with no other isolation. Treat
  the language-level sandbox alone as defense-in-depth, not as a
  security boundary on its own.
- **OS-level isolation (`bwrap`).** On Linux hosts with `bubblewrap`
  installed, `--require-os-isolation` layers OS-level (namespace-based)
  isolation underneath the language-level sandbox. This is the boundary
  that should be relied on for genuinely hostile workloads today; running
  untrusted code without OS-level isolation (e.g., on hosts without `bwrap`,
  or with `--allow-in-process-fallback` enabled) should be treated as a
  weaker guarantee.
- **Worker-process execution model.** `cmd/splworker` implements an
  out-of-process worker protocol intended for isolating untrusted script
  execution from the host process. Vulnerabilities that allow escaping the
  worker process, escalating privileges relative to the worker's intended
  restrictions, or bypassing the worker protocol's boundaries are in scope
  and of high interest.
- **In scope:** sandbox/capability bypass in the `untrusted` profile,
  resource-limit bypass (heap/step/timeout/output limits), filesystem or
  network escapes from the untrusted profile, privilege escalation from the
  worker process, and memory-safety issues in the Go runtime reachable from
  interpreted code.
- **Out of scope (for now, but still useful as issues):** the `trusted`
  profile behaving as a full-access execution mode (this is intentional —
  it is meant for code you already trust), and denial-of-service via
  obviously pathological scripts run under the `trusted` profile without
  any limits configured.

If you are unsure whether something qualifies as a security issue versus a
general bug, err on the side of reporting it privately via the channels
above and we will triage.
