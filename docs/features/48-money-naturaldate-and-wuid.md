# 48 — Money, Natural-Language Dates & Sortable IDs

Source: `plugins/money` (wraps `github.com/oarkflow/money`), `plugins/naturaldate`
(wraps `github.com/oarkflow/naturaldate`), and `plugins/wuid` (wraps
`github.com/oarkflow/wuid`). All three are small, optional, dependency-light
utility packages linked only into `cmd/interpreter`, each following the same
plugin pattern as `plugins/xql`/`plugins/database`. `money` and `naturaldate`
register virtual modules of the same name; `wuid` does too.

## Money (`money_*`)

Amounts are stored as **integer minor units** (e.g. cents) so arithmetic
never drifts the way `float64` math would:

```spl
let [price, err] = money_new("19.99", "USD");
if (err != null) { throw err; }

let tax = money_percent(price, 8.5);       // 8.5% sales tax, rounds half up
let [total, addErr] = money_add(price, tax);
if (addErr != null) { throw addErr; }

print money_format(price); // $19.99
print money_format(tax);   // $1.70
print money_format(total); // $21.69
```

`money_new(amount, currency_code)` accepts a `STRING` amount (e.g.
`"19.99"` — preferred, since it avoids float rounding entirely) or an
`INTEGER`/`FLOAT`, plus an ISO 4217 currency code; it returns `[money,
err]`. A `money` value round-trips through SPL as a plain `HASH`: `{amount,
currency, decimals, display}` — `amount` is the integer minor-unit value,
so it survives JSON/HASH conversion (`write_json`, `db_exec`, ...) without
floating-point drift.

```spl
print money_format(price, {"without_symbol": true}); // "19.99"
```

`money_format(money[, opts])` supports `opts.locale` (e.g. `"en_US"`,
`"de_DE"`), `opts.without_symbol`, and `opts.without_comma` (thousands
separators).

```spl
let [m, _] = money_new(3, "USD");
let [tripled, mulErr] = money_mul(m, 4);   // whole-number multiplier
print money_format(tripled);               // $12.00
```

`money_add`/`money_sub` require both operands to share a currency —
mismatched currencies are a **hard error**, not silently wrong math:

```spl
let [eurPrice, _] = money_new("19.99", "EUR");
let [mismatch, mismatchErr] = money_add(price, eurPrice);
print mismatch;      // null
print mismatchErr;   // "currency mismatch"
```

`money_mul(money, factor)` takes a whole-number `INTEGER` multiplier (e.g.
quantity × unit price); use `money_percent(money, pct)` for fractional
multipliers instead.

## Natural-language dates (`naturaldate_*`)

```spl
let [r, err] = naturaldate_parse("tomorrow at 9am");
if (err != null) { throw err; }
print r;
// {direction: "future", has_recur: false, time: "...", truncated: "hour", unix: ...}

print naturaldate_parse("next monday");
print naturaldate_parse("in 3 business days");
```

`naturaldate_parse(text[, opts])` returns `[result, err]`; on unparseable
input it returns `[null, err]` rather than throwing — useful for scripts
that scan free-form input where only some of it is a date:

```spl
let [bad, badErr] = naturaldate_parse("this is not a date at all");
print bad;    // null
print badErr; // naturaldate_parse: could not parse "this is not a date at all" as a date/time expression
```

`result` has `time` (RFC3339), `unix`, `direction` (`"past"`/`"present"`/
`"future"`), `truncated` (the coarsest unit actually specified — `"hour"`
for "tomorrow at 9am", `"day"` for "next friday"), and `has_recur` (with a
`recur` field when the expression describes a repeating schedule, e.g.
"every first monday of the month").

```spl
print naturaldate_parse_all("remind me tomorrow at 9am and again next friday");
// [{...9am tomorrow...}, {...next friday...}] — every date/time expression found in the text
```

`naturaldate_parse_all(text[, opts])` scans free-form text and extracts
**every** date/time expression it finds, rather than requiring the whole
string to be one expression.

```spl
print naturaldate_parse("next friday", {
    "reference": "2026-01-01T00:00:00Z",
    "location": "America/New_York"
});
// time resolves relative to 2026-01-01 in America/New_York, not the wall clock
```

`opts` (a `HASH`, both functions) pins the parse away from real wall-clock
time and locale for reproducible results: `reference` (RFC3339 `STRING`,
the "now" to resolve relative expressions against), `location` (IANA
timezone name), `weekday_dir` (`"past"`/`"present"`/`"future"`, which
direction an ambiguous weekday like "monday" resolves), `allow_embedded`
(`BOOLEAN`), and `holidays` (`ARRAY` of `"YYYY-MM-DD"` strings, for
business-day arithmetic).

## Sortable IDs (`wuid_*`)

```spl
let id = wuid_new();
print id; // e.g. "033sqLuRamwvCCpmubTdAG" — base62, fixed-width, sorts by creation time

print wuid_new_uuid(); // same underlying ID, formatted as a standard dashed UUID
```

`wuid_new()` generates a 128-bit, time-ordered (UUIDv7-compatible) ID
encoded as fixed-width base62 text — about a third shorter than a dashed
UUID while preserving sort order, a sortable-ID counterpart to the
`uuid()`/`token_generate()` daily-ops builtins (doc 39). `wuid_new_uuid()`
generates the same kind of ID but formatted as a standard dashed UUID
string for systems that expect that shape.

```spl
let [parsed, err] = wuid_parse(id);
if (err != null) { throw err; }
print parsed;
// {hex, id, time (RFC3339Nano), unix_ms, uuid}

let [bad, badErr] = wuid_parse("not-a-valid-id");
print bad;    // null
print badErr; // wuid: invalid id: invalid Base62 character '-'
```

`wuid_parse(id)` decodes a base62, Crockford base32, hex, or dashed-UUID
string and returns its embedded creation time; garbage input returns
`[null, err]` rather than throwing.

## Capability notes

None of `money_*`, `naturaldate_*`, or `wuid_*` check any capability at
all — verified running under `--profile untrusted` with no `--allow-cap`
flags. They're pure in-memory computations over their explicit arguments,
with no filesystem or network access.
