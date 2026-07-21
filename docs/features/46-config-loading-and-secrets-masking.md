# 46 — Config Loading & Secrets Masking

Source: `pkg/config/config.go` (always available), `config/yaml` (optional,
adds YAML support — doc 36).

## Loading config files

```spl
let cfg = config_load(".env", "env");
let db = config_load("config/database.yaml", "yaml"); // requires config/yaml (doc 36)
let api = config_load("config/api.json", "json");
```

`config_load(path[, format])` auto-detects format from the file extension
(`.json`→json, `.yaml`/`.yml`→yaml, `.env`→env) if `format` is omitted.
`config_parse(raw, format)` parses an in-memory string instead of reading a
file (same format support, no path/read-permission checks).

## `.env` files

```text
APP_NAME=demo
DB_HOST=localhost
DB_PASSWORD=hunter2
```

```spl
let cfg = config_load(".env", "env");
print cfg.APP_NAME;    // demo
print cfg.DB_HOST;     // localhost
print cfg.DB_PASSWORD; // ***  (masked, see below)
print secret_reveal(cfg.DB_PASSWORD); // hunter2
```

`.env` parsing: skips blank lines and `#` comments, strips a leading
`export ` prefix, splits on the first `=`, and strips matching quotes from
values. A malformed line (no `=`) is a parse error citing the line number.

## JSON / access via dot notation

```spl
let api = config_load("api.json", "json");
print api.base_url;           // https://api.example.com
print secret_reveal(api.api_key); // reveals a key named "api_key"
```

Nested keys work the same way: `db.auth.username`, `db.auth.password`.

## Secret masking

Any hash key whose name matches a sensitive-key pattern
(`password`, `passwd`, `secret`, `token`, `api_key`, `apikey`,
`private_key`, `secret_key`, `access_key`, `refresh_token`,
`client_secret`, `credentials`/`credential`, `auth` — case-insensitive
substring match) has its **string value** wrapped as a `SECRET` object
(doc 19), which always prints as `***` regardless of context (print,
default formatting, REPL output, JSON encoding of the wrapped value's
`Inspect()`).

```spl
print db.auth.password;      // ***
let raw = secret_reveal(db.auth.password);
print raw;                   // actual plaintext value
```

> Once any key within a nested hash matches a sensitive pattern, **sibling
> string values in that same nested hash may also be wrapped** as secrets
> (verified in doc 36 with a YAML example — `username` printed masked
> alongside `password` under the same `auth` section). Don't assume only
> the literally-matching key is masked; check what actually printed if it
> matters.

## Reveal / re-mask helpers

```spl
secret_reveal(value)         // Secret -> plain String; String passes through unchanged
secret_mask(value[, visible=2]) // masks all but the last N characters
secret(value)                 // wraps a plain value as SECRET yourself
```

```spl
print secret_mask("mypassword");    // ********rd  (last 2 chars visible)
print secret_mask("mypassword", 4); // ******word
```

## Error handling — verified behavior

```spl
let cfg = try {
    config_load("nonexistent.json", "json");
} catch (e) {
    "caught: " + e;
};
print cfg; // caught: open .../nonexistent.json: no such file or directory
```

**Verified**: a `config_load`/`config_parse` failure (missing file, parse
error, unsupported format) raises a **catchable runtime error** — wrap the
call in `try/catch` rather than expecting a sentinel return value or an
`"ERROR: ..."`-prefixed string to check for. An unwrapped failing call
aborts the enclosing statement (and, at the top level, the script) just
like any other uncaught error (doc 08).

## REPL usage

```text
spl> :config .env env
CONFIG loaded
spl> CONFIG.DB_HOST
localhost
spl> CONFIG.DB_PASSWORD
***
```

`:config <file> [json|yaml|env]` loads a file into the REPL's global
`CONFIG` variable for interactive exploration (doc 40).

## Related builtins quick reference

| Builtin | Purpose |
|---|---|
| `config_load(path[, format])` | load + parse a config file, with secret wrapping |
| `config_parse(raw, format)` | parse an in-memory string the same way |
| `secret(value)` | wrap a value as `SECRET` |
| `secret_reveal(value)` | unwrap a `SECRET` (or pass through a plain string) |
| `secret_mask(value[, visible])` | partially mask an arbitrary string |
