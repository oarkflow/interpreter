# 24 — Crypto & Encoding Builtins

Source: `pkg/builtins/crypto.go`, plus the core crypto/encoding builtins in
`pkg/builtins/core.go` (see doc 20 for `hash`, `hmac`, `uuid`,
`password_hash`/`verify`, `encrypt`/`decrypt`, `base64_*`, `hex_*`,
`url_*`, `json_encode`/`decode`, `secret*`, `random_bytes`/`random_string`,
`constant_time_eq`, `api_key`, `password_generate`).

This document covers the small, fixed-algorithm digest helpers in
`crypto.go` plus regex/HTML/JSON string helpers that round out the
encoding story.

## Fixed-algorithm digests (`crypto.go`)

```spl
print md5("hello");
// 5d41402abc4b2a76b9719d911017c592

print sha256("hello");
// 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b982

print sha512("hello");
// 9b71d224bd62f3785d96d46ad3ea3d73319bfbc2890caadae2dff72519673ca...

print hmac_sha256("data", "key");
print hmac_sha512("data", "key");
```

These duplicate what the generic `hash(algo, data)`/`hmac(algo, key, data)`
builtins (doc 20) already cover for `"md5"`/`"sha256"`/`"sha512"` — use
whichever reads better at the call site; behavior is identical.

## Regex helpers (complementing the string dot-methods in doc 16)

```spl
print regex_find_all("a1b2c3", "[0-9]");  // ["1","2","3"]
print regex_split("a1b2c3", "[0-9]");     // ["a","b","c",""]
```

(`"x".regex_match(pattern)` and `"x".regex_replace(pattern, repl)` are
dot-methods, documented in doc 16.)

## HTML escaping

```spl
print escape_html("<div>&\"'</div>");
// &lt;div&gt;&amp;&#34;&#39;&lt;/div&gt;
print unescape_html("&lt;div&gt;"); // <div>
```

## JSON helpers

```spl
print json_parse('{"a":1}');       // {a: 1}
print json_stringify({"a": 1});    // '{"a":1}'
```

These are aliases of `json_decode`/`json_encode` (doc 20) with slightly
different names for readability in different contexts (`json_stringify`
also accepts formatting options — see its `opts` parameter for indent
control).

## Where the rest lives

| Feature | Doc |
|---|---|
| `hash`, `hmac`, `password_hash`/`verify`, `encrypt`/`decrypt` (AES-GCM), `constant_time_eq`, `uuid`, `random_bytes`, `random_string`, `password_generate`, `api_key` | doc 20 (Core Builtins) |
| `base64_encode`/`decode`, `hex_encode`/`decode`, `url_encode`/`decode`, `json_encode`/`decode` | doc 20 |
| `secret(v)`, `secret_reveal(v)`, `secret_mask(v[, visible])`, `config_load`/`config_parse` secret-wrapping | doc 46 (Config Loading & Secrets Masking) |
| `bcrypt_hash`/`verify`, `jwt_encode`/`decode` (optional `cryptoextra` plugin) | doc 36 (Secrets & Extra Crypto) |
| `secretr_get`/`set`/`delete`/`list`/`scan` (optional `secretr` plugin) | doc 36 |
