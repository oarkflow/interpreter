# 47 — Email, Phone & IP Validation

Source: `plugins/emailvalidator` (wraps `github.com/oarkflow/ev`),
`plugins/phone` (wraps `github.com/oarkflow/phone`, a
libphonenumber-equivalent parser), and `plugins/ip` (wraps
`github.com/oarkflow/ip`). All three are optional packages linked only into
`cmd/interpreter`. `phone` and `ip` each register a virtual module of the
same name; `emailvalidator`'s builtins register directly and have **no**
virtual module at all (`import "emailvalidator"` doesn't work — the
functions are simply always-global once the package is linked).

All three share the same shape for validating a field across many records
at once: a `*_bulk` builtin that accepts an `ARRAY` (of `HASH` rows, or
plain `STRING` values) or a `TABLE_VALUE` (as returned by `read_csv`),
returns `{total, valid_count, invalid_count, results}`, and never aborts a
batch on one bad record — each `results[i]` is a *flat* row (the original
record's fields, if any, plus `input`/`valid`/`error` and, on success, the
same fields the single-record function would return) ready to hand
straight to `write_json`/`write_csv`/`db_exec`.

## Email (`email_*`)

Syntax/normalization, disposable-domain, role-account, and free-provider
checks never touch the network. DNS (on by default) and SMTP mailbox
probing (opt-in) do, and require the `network` capability — confirmed
denied under `--profile untrusted` with no `--allow-cap`, then allowed with
`--allow-cap network`:

```spl
let syntax = email_validate_syntax("User@Example.COM");
print syntax;
// {domain: "example.com", domain_literal: false, error: "", local: "User",
//  normalized: "User@example.com", smtp_utf8: false, status: "pass"}
// Verified: only the domain is lowercased by normalization; the local
// part's case is preserved (email locals are technically case-sensitive).

print email_is_disposable("test@mailinator.com");   // true
print email_is_role_account("admin+ops@example.com"); // true (local part before "+tag")
print email_is_free_provider("someone@gmail.com");    // true

let [result, err] = email_validate("user@example.com", {"check_dns": false});
if (err != null) { throw err; }
print result.verdict;     // "unknown" (no DNS checked, so not enough signal either way)
print result.risk_score;  // 5
print result.reasons;     // [{code: "dns_not_checked", message: "...", weight: 5}]
```

`email_validate(email[, opts])` runs the full layered check: syntax,
disposable/role/free-provider detection, DNS (`opts.check_dns`, default
**true**), and optional SMTP RCPT probing (`opts.check_smtp`, default
false, slower and can be rate-limited by mail servers). `opts.timeout_ms`
(default 5000) bounds the whole call. It returns a `[result, err]` tuple
where `err` is non-null only for cancellation/timeout or a denied
capability — never for a simply-invalid address (that shows up in
`result.verdict`/`result.reasons` instead). `result` mirrors `ev.Result`:
`input`, `normalized`, `verdict` (`"pass"`/`"risky"`/`"fail"`/`"unknown"`),
`risk_score`, `syntax`, `dns`, `smtp`, `disposable`, `role_account`,
`free_provider`, `reputation`, `reasons`, `checked_at`, `duration_ms`.

### Bulk validation (`email_validate_bulk`)

```spl
let signups = [
    {"name": "Ada", "email": "ada@example.com"},
    {"name": "Grace", "email": "grace@mailinator.com"},
    {"name": "Linus", "email": "not-an-email"}
];

let report = email_validate_bulk(signups, "email");
print sprintf("checked %d, %d valid, %d invalid", report.total, report.valid_count, report.invalid_count);
print report.results;
// results[i]: {name, email, input, valid, error, ...email_validate's fields on success}
```

`email_validate_bulk(records[, field][, opts])` defaults `check_dns`/
`check_smtp` to **false** (unlike `email_validate`, so large batches stay
fast and capability-free unless explicitly requested) and runs with
`opts.workers` concurrent goroutines (default 1 serial when neither DNS nor
SMTP is checked, else 8; capped at 64). `valid` reflects syntax validity
only, matching `phone_parse_bulk`/`ip_lookup_bulk`'s convention. Omit
`field` (or pass `null`) when `records` are plain email strings rather than
`HASH` rows, e.g. `email_validate_bulk(["a@x.com", "b@y.com"])`.

## Phone (`phone_*`)

```spl
let [parsed, err] = phone_parse("(650) 253-0000", "US");
if (err != null) { throw err; }
print parsed;
// {carrier: "", country_code: 1, e164: "+16502530000",
//  international: "+1 650-253-0000", national: "(650) 253-0000",
//  national_number: "6502530000", network: null, possible: true,
//  region: "US", type: "fixed_line_or_mobile", valid: true}
```

`phone_parse(number[, default_region])` parses and validates a number.
`default_region` (a 2-letter ISO country code) is used to interpret numbers
without a leading `+`; it's ignored — but the underlying library still
requires *some* value, so pass `""` or a best guess — once the number
already includes a country calling code (e.g. `"+16502530000"`). Mobile
numbers additionally get a best-effort `carrier` name and a `network`
record (that carrier's MCC/MNC/PLMN/status entry, cross-referenced from the
`phone_networks` table by fuzzy name match — a carrier name doesn't
uniquely identify one network record, so this is approximate); both are
`""`/`null` for non-mobile numbers.

```spl
print phone_valid("(650) 253-0000", "US");   // true — never throws
print phone_valid("not a phone number", "US"); // false

print phone_country("AU");
// {code: "AU", currency: "AUD", currency_symbol: "$", name: "Australia", phone: "+61"}

print phone_networks("US", {"status": "Operational"});
// [{mcc, mnc, plmn, country_code, country_name, region, type, brand,
//   operator, status, bands, latitude, longitude}, ...] — 175+ entries for US
```

`phone_networks(country_code[, opts])` lists the full, unfiltered
per-country MCC/MNC/PLMN operator table directly — useful when you need
every known network for a country, not just the one guessed for a single
parsed number. `opts.status` filters to an exact (case-insensitive) status
match, e.g. `"Operational"`, to drop the retired/reserved/historical
entries the raw dataset otherwise includes.

### Bulk validation (`phone_parse_bulk`)

```spl
let contacts = [
    {"name": "Ada", "phone": "(650) 253-0000"},
    {"name": "Grace", "phone": "+61 2 9374 4000", "region": "AU"},
    {"name": "Linus", "phone": "not a phone number"}
];

let report = phone_parse_bulk(contacts, "phone", {"default_region": "US", "region_field": "region"});
print sprintf("checked %d, %d valid, %d invalid", report.total, report.valid_count, report.invalid_count);
```

`opts.region_field` looks up a per-record region column (e.g. `"AU"` for
Grace above), falling back to `opts.default_region` when the record has no
region or the field is absent — handy for international contact lists
where region isn't uniform.

## IP (`ip_*`)

```spl
print ip_is_private("10.0.0.1");  // true
print ip_is_private("8.8.8.8");   // false

// Client-IP extraction from a proxy-chain header, preferring the first
// public address; pass trust_proxy: false to ignore the header entirely
// (the safe default when it could be spoofed by an untrusted client).
print ip_client_from_header("10.0.0.1", "203.0.113.5, 10.0.0.1");        // "203.0.113.5"
print ip_client_from_header("10.0.0.1", "203.0.113.5", {"trust_proxy": false}); // "10.0.0.1"
```

`ip_country(ip)` and `ip_lookup(ip)` (full country/region/city/lat-long
record) need a local geolocation dataset that isn't fetched automatically —
call `ip_geo_init()` first, which is gated behind **both** the `network`
and `filesystem_write` capabilities (it downloads/caches a multi-megabyte
third-party dataset under `~/.ipdata` on first use) and returns `[ok,
err]`. Until then, `ip_country` returns `""` and `ip_lookup` returns
`found: false`:

```spl
print ip_country("8.8.8.8");  // "" — geo dataset not loaded yet
print ip_lookup("8.8.8.8");
// {city: "", country: "", country_code: "", found: false, latitude: 0, longitude: 0, region: ""}

// let [ok, err] = ip_geo_init();
// if (err != null) { throw err; }
// print ip_country("8.8.8.8");  // now resolves a real country code
```

### Bulk validation (`ip_lookup_bulk`)

```spl
let requests = [
    {"user": "ada", "ip": "8.8.8.8"},
    {"user": "grace", "ip": "10.0.0.5"},
    {"user": "linus", "ip": "not-an-ip"}
];

let report = ip_lookup_bulk(requests, "ip");
print sprintf("checked %d, %d valid, %d invalid", report.total, report.valid_count, report.invalid_count);
// results[i] also includes is_private alongside ip_lookup's fields
```

`ip_lookup_bulk(records[, field])` has no `opts` argument (unlike the email/
phone bulk builtins) — `valid_count` counts syntactically valid IP
addresses, not geolocation matches (call `ip_geo_init()` first for that).

## Capability summary

| Builtin | Capability required |
|---|---|
| `email_validate_syntax`, `email_is_disposable`, `email_is_role_account`, `email_is_free_provider` | none |
| `email_validate` (default opts, or `email_validate_bulk` with `check_dns`/`check_smtp` requested) | `network` |
| `phone_parse`, `phone_valid`, `phone_country`, `phone_networks`, `phone_parse_bulk` | none |
| `ip_is_private`, `ip_client_from_header`, `ip_country`, `ip_lookup`, `ip_lookup_bulk` | none |
| `ip_geo_init` | `network` **and** `filesystem_write` |

**Verified:** all of the "none" rows above were confirmed to run under
`--profile untrusted` with no `--allow-cap` flags at all; `email_validate`
with its DNS-checking default was confirmed **denied** under the same
profile until `--allow-cap network` was added.
