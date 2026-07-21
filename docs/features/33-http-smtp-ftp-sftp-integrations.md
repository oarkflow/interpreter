# 33 — HTTP, SMTP, FTP & SFTP Integrations

Source: `plugins/integrations` (optional package; linked only into
`cmd/interpreter`, including its `--playground` mode). Virtual module name:
`"integrations"`. All network calls require the `network` capability (doc
45); file transfers also require file read/write capability as applicable.

```spl
import "integrations";
```

## HTTP

```spl
let res, err = http_get("http://127.0.0.1:3097/echo"[, headers][, timeout_ms]);
if (err == null) {
    print res.status_code;
    print res.body;
}

let res2, err2 = http_post(url, body[, headers][, timeout_ms]);

let res3, err3 = http_request("POST", url, {"a": 1}, {"X-Env": "test"}, 2000);
print res3.status_code; // 200
print res3.body;         // '{"a":1}'

let wres, werr = webhook(url, {"event": "test"}[, headers][, timeout_ms]);
```

Response shape: `{status, status_code, body, url, ok, duration_ms, headers}`.
A non-string `body` argument is auto-JSON-encoded with
`Content-Type: application/json`; `webhook` defaults to that content type.
Default per-call timeout is 30 seconds if not specified. Response body size
is capped by `SPL_HTTP_MAX_BODY_BYTES` (default 1 MiB).

Verified live against a local SPL server (doc 29): `http_request`,
`http_post`, `webhook`, and `http_get` all round-trip correctly, including
a `404` for a method the route doesn't handle.

## SMTP

```spl
let ok, err = smtp_send({
    "host": "localhost",
    "port": 1025,
    "from": "noreply@localhost",
    "to": ["alice@localhost", "bob@localhost"],
    "subject": "Build status",
    "body": "Pipeline complete"
    // optional: "cc": [...], "bcc": [...], "html": "<b>...</b>"
});
```

Sends via `net/smtp`; header values are sanitized against CRLF injection.
For local testing, run a sandbox SMTP server (MailHog/Mailpit) listening on
`localhost:1025`. *(Not exercised live while writing this doc — no local
SMTP sandbox was running; the call shape above matches
`examples/all_in_one.spl`'s guarded integration section.)*

## FTP

```spl
let cfg = {"host": "ftp.example.com", "port": 21, "username": "u", "password": "p", "timeout_ms": 10000};
let list, lerr = ftp_list(cfg, "/incoming");
// [{name, size, is_dir, modified}, ...]
let ok1, gerr = ftp_get(cfg, "/incoming/a.txt", "testdata/a.txt");
let ok2, perr = ftp_put(cfg, "testdata/a.txt", "/outgoing/a.txt");
```

## SFTP

```spl
let cfg = {
    "host": "sftp.example.com", "port": 22, "username": "u",
    "password": "p" // or "private_key": "..."
};
let list, lerr = sftp_list(cfg, "/data");
let ok1, gerr = sftp_get(cfg, "/data/in.csv", "testdata/in.csv");
let ok2, perr = sftp_put(cfg, "testdata/in.csv", "/data/out.csv");
```

FTP/SFTP config defaults: FTP port `21`, SFTP port `22`, both with a
10-second default `timeout_ms`; transfers are internally bounded to a
5-minute default transfer timeout. SFTP `username` is required;
authenticate with either `password` or `private_key`.

*(FTP/SFTP examples require reachable real endpoints and were not exercised
live while writing this doc — signatures and response shapes are taken
directly from `plugins/integrations/integrations.go` and
`examples/all_in_one.spl`'s disabled-by-default integration showcase.)*

## Return-value convention

Every integration builtin follows the `[value, error]` tuple pattern:
network calls return `[result, error]`; mutating operations (`ftp_put`,
`sftp_put`) return `[ok_bool, error]`.

## Capability & policy notes

- All of the above require the `network` capability.
- Network targets can be further restricted with
  `--allow-network`/`SPL_NETWORK_ALLOW`/`SPL_NETWORK_DENY` (doc 45).
- Under a custom embedding host built against only the root
  `github.com/oarkflow/interpreter` module, none of these builtins are
  linked in — calling them fails with an actionable "not linked into this
  interpreter" error pointing at `cmd/interpreter`.
