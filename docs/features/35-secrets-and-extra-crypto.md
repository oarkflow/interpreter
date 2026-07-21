# 35 — Secrets Vault & Extra Crypto (secretr, bcrypt, JWT, Shamir, securetoken)

Source: `plugins/secretr` (private dependency, requires a local sibling
checkout of `github.com/oarkflow/secretr` + `-tags dev`) and
`plugins/crypto` (wraps `golang.org/x/crypto/bcrypt`,
`github.com/golang-jwt/jwt/v5`, and `github.com/oarkflow/securetoken`), plus
`plugins/shamir` (wraps `github.com/oarkflow/shamir`). All are optional
packages linked only into `cmd/interpreter`. `cryptoextra` registers virtual
module `"cryptoextra"`; `securetoken` and `shamir` each register a virtual
module of the same name; `secretr` registers its builtins globally with no
dedicated module namespace.

**Verified:** only `secretr_*` requires the `secrets` capability (confirmed
denied under `--profile untrusted` with no `--allow-cap`, then allowed with
`--allow-cap secrets`) — `bcrypt_*`, `jwt_*`, `shamir_*`, and
`securetoken_*` all run under **any** profile with no capability grant at
all, since none of them touch the filesystem, network, or a stored secret
vault; they're pure in-memory cryptographic transforms over their explicit
arguments.

## Secrets vault (`secretr_*`)

```spl
secretr_set("demo/api-key", "sk_live_demo_only_not_real");
let apiKey = secretr_get("demo/api-key");
print apiKey;                   // *** (masked SECRET value)
print secret_reveal(apiKey);    // sk_live_demo_only_not_real
```

Values are stored in a local encrypted vault (`SECRETR_DATA_DIR`, default
`./.secretr-data`) and always returned wrapped as `SECRET` (doc 19) so
they never print in plaintext by accident.

### Dot-notation nesting

```spl
secretr_set("demo.database.password", "hunter2");
secretr_set("demo.database.host", "localhost");
print secretr_list("demo"); // ["demo", "demo/api-key"]
```

Dotted names nest fields under one top-level vault entry rather than
creating separate top-level secrets — useful for grouping related
credentials (`demo.database.password`, `demo.database.host`) under a single
logical namespace.

```spl
secretr_delete("demo/api-key"); // remove a secret
```

### Scanning text for hardcoded secrets

```spl
let findings = secretr_scan("aws_key = \"AKIAABCDEFGHIJKLMNOP\"");
print findings;
// [{pattern: "aws_access_key", redacted: "AK****************OP", severity: "critical"}]
```

`secretr_scan` runs the same detection patterns (AWS keys, GitHub tokens,
PEM headers, JWTs, ...) used to enforce
`SecurityPolicy.BlockHardcodedSecrets` — importing `plugins/secretr`
registers this scanner with `pkg/security`, so setting
`SPL_BLOCK_HARDCODED_SECRETS=true` (or `BlockHardcodedSecrets: true` on a
policy) causes the interpreter to **reject scripts** whose literal source
text looks like a hardcoded credential, in addition to letting you call
`secretr_scan` yourself against arbitrary text for a non-rejecting report.

## bcrypt

```spl
let h = bcrypt_hash("hunter2"[, cost]);
print h; // $2a$10$...
print bcrypt_verify("hunter2", h); // true
print bcrypt_verify("wrong", h);   // false
```

## JWT

```spl
let token = jwt_encode({"sub": "user-123", "role": "admin"}, "signing-secret", {
    "alg": "HS256", "expires_in": 3600
});
let claims = jwt_decode(token, "signing-secret");
print claims.sub;  // user-123
print claims.role; // admin
```

Only HMAC algorithms (`HS256`/`HS384`/`HS512`) are supported. `jwt_decode`
always verifies against the caller-specified `alg`, so a token can't pick
its own verification algorithm — `alg: none` and algorithm-confusion
forgeries are rejected.

`jwt_encode`/`jwt_decode` (like `bcrypt_hash`/`bcrypt_verify`) **raise a
catchable error** rather than returning a `[value, error]` tuple:

```spl
let bad = try {
    jwt_decode(token, "wrong-secret");
} catch (e) {
    "rejected: " + e;
};
print bad;
// rejected: jwt_decode: token signature is invalid: signature is invalid
```

## Shamir secret sharing (`shamir_*`)

`plugins/shamir` wraps `github.com/oarkflow/shamir`: split a secret into N
shares such that any T of them reconstruct it, but fewer reveal nothing at
all — useful for distributing a master key/password across multiple
holders so no single person can recover it alone (e.g. a database
encryption key split among on-call engineers, or a root credential among
founders).

```spl
let [split, err] = shamir_split("db-master-key", 3, 5); // 5 shares, any 3 reconstruct
print err;
print split;
// {auth_key: "...", shares: ["...", "...", "...", "...", "..."]}

let [secret, cerr] = shamir_combine(slice(split.shares, 0, 3), split.auth_key);
print cerr;
print secret; // "db-master-key"
```

Every share is HMAC-tagged with an `auth_key` so tampered or mismatched
shares are rejected at combine time rather than silently producing garbage.
`shamir_split`'s optional 4th argument reuses an existing base64 `auth_key`
(e.g. to re-split under the same key); when omitted, a fresh random one is
generated and returned alongside the shares — keep it (distributed
separately from the shares, since its own secrecy is what makes them
tamper-evident) and pass it to `shamir_combine`. Fewer than `threshold`
shares, a wrong `auth_key`, or a tampered share all fail with a
`[null, err]` result rather than returning garbage.

## Stateless encrypted tokens (`securetoken_*`)

`plugins/crypto`'s `securetoken_encrypt`/`securetoken_decrypt` wrap
`github.com/oarkflow/securetoken`, encrypting a HASH of claims into an
AES-256-GCM `"s1.local."`-prefixed token using a key derived (SHA-256) from
a plain secret string — a JWT alternative when you want the payload itself
encrypted (not just signed and base64-readable):

```spl
let tok = securetoken_encrypt({"sub": "u1", "role": "admin"}, "sekret"[, {"footer": "v1"}]);
print tok; // "s1.local.<base64>..."
let claims = securetoken_decrypt(tok, "sekret"[, {"expected_footer": "v1"}]);
print claims.sub; // "u1"
```

`opts.footer` (encrypt) attaches cleartext-but-authenticated metadata (e.g.
a key-version tag); `opts.expected_footer` (decrypt) must match exactly or
decryption fails. Like `jwt_decode`, `securetoken_decrypt` **raises a
catchable error** (rather than a `[value, error]` tuple) on any AEAD
authentication failure — tampering or a wrong secret.

## Build note

`secretr` uniquely requires a **local, private sibling checkout** of
`github.com/oarkflow/secretr` (not published) plus a `-tags dev` build —
see `plugins/secretr/README.md`. Without that sibling checkout,
`cmd/interpreter` cannot link `plugins/secretr` at all (a build error,
not a runtime "not linked" message like other optional builtins).
`cryptoextra` and `plugins/shamir` have no such requirement and build
normally as part of `cmd/interpreter`.
