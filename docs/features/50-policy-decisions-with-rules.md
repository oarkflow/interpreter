# 50 — Policy-Driven Decisions (`rules`)

Source: `plugins/rules`, wrapping `github.com/oarkflow/rules` ("Condition"),
a BCL-backed policy/decision engine. Full example:
`examples/rules_all_in_one.spl`.

This plugin wraps only the **core publish → evaluate → activate/rollback
loop**. `rules.Service` itself is much larger (workflows, stateful chains,
canary releases, blast-radius analysis, approval gates, tamper-evident audit
chains, HTTP servers) — none of that is exposed here yet. Reach for it when
you want business/authorization decisions described declaratively in BCL
(e.g. "does this payment need manual review?") instead of scattered `if`
statements in script code.

`rules_service()` requires the `policy` capability under a restrictive
security policy (`--allow-cap policy`).

## `rules_service([opts])`

```spl
let svc = rules_service({"environment": "dev"});
```

Creates an in-process policy service backed by an in-memory store (no
file/SQL-backed storage in this pass — the service and every published
definition live only for the lifetime of the script). `opts`: `environment`,
`default_tenant`, `strict_validation`, `strict_evaluation`,
`require_activation_approval`, `require_tests` (all optional).

## `rules_publish(service, name, bcl_source_or_path[, opts])`

```spl
let policy = `module "access" {
  decision_schema "access" { effects [allow, deny] default deny strategy first_match }
  decision_table "access" {
    default deny
    hit_policy first
    row "allow-verified" {
      when { request.verified == true }
      then { decision allow reason "verified user" }
    }
  }
}`;

let [pub, err] = rules_publish(svc, "access-policy", policy, {"version": "1"});
if (err != null) { throw err; }
```

The third argument is auto-detected: if it resolves to a real file on disk
it's loaded from there (subject to the same filesystem-read sandboxing as
every other file-reading builtin — see [Security & Sandboxed
Execution](../../README.md#security--sandboxed-execution)); otherwise it's
parsed as an inline BCL block. `opts`: `version`, `environment`,
`run_tests`. Publishing auto-activates the new version unless the service
was created with `require_activation_approval: true`.

**Gotcha:** a `decision_table`'s own `default` clause does not override its
`decision_schema`'s `default` — if they disagree, the schema-level default
wins. Keep them in sync (see `examples/rules_all_in_one.spl` step 4 for a
worked example of exactly this).

## `rules_evaluate(service, name, decision, facts[, opts])`

```spl
let [result, err] = rules_evaluate(svc, "access-policy", "access", {"request": {"verified": true}});
if (err != null) { throw err; }
print result.Report.Decision.Effect;   // "allow"
print result.Report.Decision.Allowed;  // true
```

`opts`: `strict`. The return value is the library's full
`EvaluateResponse` (`Report`, `Shadow`, `Workflow`, `Audit`) converted
generically — struct field name becomes hash key **as-is**, so paths are
capitalized (`result.Report.Decision.Effect`), not the usual
snake_case/camelCase convention used elsewhere in this language. Pointer
fields that are `nil` convert to `null`; non-nil pointer fields are
dereferenced into a nested hash. `time.Time` fields convert to an RFC3339
string.

## `rules_activate(service, name, version[, environment])` / `rules_rollback(service, name, version[, environment])`

```spl
let [_pub2, _e] = rules_publish(svc, "access-policy", policyV2, {"version": "2"}); // auto-activates v2
let [_rb, rbErr] = rules_rollback(svc, "access-policy", "1", "dev");               // back to v1
if (rbErr != null) { throw rbErr; }
```

Thin wrappers around the library's version activation/rollback, for
promoting or reverting between already-published versions without
republishing.

## Not yet supported

Stateful chains/watches, lifecycle phases, workflows, canary/blast-radius
comparisons, approval workflows, route/action catalogs, PII redaction
config, signed bundle import/export, and the file/SQL-backed
`contrib/storage` adapters. All of these exist on `rules.Service` in the
underlying library and can be added as builtins later if needed — nothing
here is a hard architectural limit, just unwired surface.
