package main

// fullExampleOverrides replaces the shared base example set's entries for
// scripts whose content depends on builtins only this binary links
// (builtins/images, builtins/database) - see
// pkg/playgroundserver.Variant.ExampleOverrides.
var fullExampleOverrides = map[string]string{
	"image-values": `// Full playground image values can be decoded, transformed, inspected, and rendered.
// This uses an inline PNG so it works without extra files or URL access.

let tiny = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAAEElEQVR4nGL6z8AACAAA//8DCQECWLbVUAAAAABJRU5ErkJggg==";
import "images";
let original = image_load(file(tiny, {"mime": "image/png", "name": "tiny.png"}));
let resized = image_resize(original, 48, 48);
let cropped = image_crop(resized, 0, 0, 24, 24);
let rotated = image_rotate(cropped, 90);
let converted = image_convert(rotated, "jpeg");
let rendered = image_render(converted, {"name": "tiny-ops.jpg", "alt": "Transformed playground image"});
let binary = file_load(rendered);

print image_info(converted);
print sprintf("rendered mime=%s size=%d", file_mime(binary), file_size(binary));
print rendered;`,

	"query-builder": `// Full playground query builder with an in-memory SQLite database.

import "database" as database;

let db, err = database.db_connect("sqlite", ":memory:");
if (err != null) { throw err; }

let _, createErr = database.db_exec(db, "CREATE TABLE users (id INTEGER, name TEXT, active BOOLEAN)");
if (createErr != null) { throw createErr; }

let _, insertErr = database.db_exec(db, "INSERT INTO users(id, name, active) VALUES(?, ?, ?)", [1, "Ada", true]);
if (insertErr != null) { throw insertErr; }
let _, insertErr2 = database.db_exec(db, "INSERT INTO users(id, name, active) VALUES(?, ?, ?)", [2, "Linus", true]);
if (insertErr2 != null) { throw insertErr2; }

let rows, qerr = database.query(db, "users")
	.select("id", "name")
	.where("active", true)
	.where_in("id", [1, 2, 3])
	.where_like("name", "%a%")
	.order_by("name ASC")
	.limit(20)
	.exec();
if (qerr != null) { throw qerr; }

print rows;

let lazyRows = database.lazy_query(db, "users").where("active", true);
print lazyRows;`,
}

// fullExtraExamples adds example keys unique to this binary (only
// meaningful when their backing builtins are linked in) - see
// pkg/playgroundserver.Variant.ExtraExamples.
var fullExtraExamples = map[string]string{
	"shamir": `// shamir splits a secret into N shares so that any T of them
// reconstruct it, but fewer reveal nothing at all (plugins/shamir, backed
// by github.com/oarkflow/shamir) - useful for distributing a master
// key/credential across multiple holders so no single person can recover
// it alone, e.g. splitting a database encryption key among on-call
// engineers.

let split, splitErr = shamir_split("db-master-key", 3, 5);
if (splitErr != null) { throw splitErr; }
print sprintf("%d shares generated, need any 3 to reconstruct", len(split.shares));

// Any 3 of the 5 shares, plus the matching auth_key, reconstruct the
// secret. The auth_key HMAC-authenticates shares so tampered or
// mismatched ones are rejected at combine time rather than silently
// producing garbage - keep it, but distribute it separately from the
// shares themselves.
let combined, combineErr = shamir_combine([split.shares[0], split.shares[2], split.shares[4]], split.auth_key);
if (combineErr != null) { throw combineErr; }
print combined;

// Too few shares - or the wrong auth_key - fails instead of returning
// wrong data.
let tooFew, tooFewErr = shamir_combine([split.shares[0], split.shares[1]], split.auth_key);
print tooFew;
print tooFewErr;`,

	"metadata": `// infer_csv_types/infer_json_types profile an unfamiliar CSV/JSON data
// source (plugins/metadata) - useful before writing a schema/import
// pipeline by hand: "is this column really numeric, or does it have stray
// text rows?"

let csvTypes, csvErr = infer_csv_types("id,name,active,joined\n1,Ada,true,2020-01-15\n2,Grace,false,2021-06-30\n");
if (csvErr != null) { throw csvErr; }
print csvTypes;

// Works on already-decoded JSON too - the same shape read_json/json_decode
// return - merging types across every row/array element.
let jsonTypes, jsonErr = infer_json_types([
	{"id": 1, "score": 9.5},
	{"id": 2, "score": 10}
]);
if (jsonErr != null) { throw jsonErr; }
print jsonTypes;

// infer_value_type checks a single value directly.
print infer_value_type(42);
print infer_value_type("2026-01-01");
print infer_value_type("not a date");`,

	"ip": `// ip does client-IP extraction and private-address detection
// (plugins/ip, backed by github.com/oarkflow/ip) - useful for request
// logging, access control, and reverse-proxy header parsing.

print ip_is_private("10.0.0.1");
print ip_is_private("8.8.8.8");

// Extracts the real client IP from a proxy-chain header, preferring the
// first public address. Pass trust_proxy: false to ignore the header
// entirely (the safe default when it could be spoofed by the client).
print ip_client_from_header("10.0.0.1", "203.0.113.5, 10.0.0.1");
print ip_client_from_header("10.0.0.1", "203.0.113.5", {"trust_proxy": false});

// Geolocation needs a local dataset fetched once via ip_geo_init(), which
// requires network + filesystem-write capability grants - it's not run
// automatically. Until then, country/lookup report unknown.
print ip_country("8.8.8.8");
print ip_lookup("8.8.8.8");

// let ok, err = ip_geo_init();
// if (err != null) { throw err; }
// print ip_country("8.8.8.8");

// ip_lookup_bulk validates/geolocates an IP field across many records in
// one call. It accepts an ARRAY (of HASH rows, like these, or plain
// strings) or a TABLE_VALUE - the same shape read_json/read_csv/db_query/
// xql_run/table_rows already return, so it drops straight into a pipeline
// like: let rows = table_rows(read_csv("visits.csv")); ip_lookup_bulk(rows, "ip");
let requests = [
	{"user": "ada", "ip": "8.8.8.8"},
	{"user": "grace", "ip": "10.0.0.5"},
	{"user": "linus", "ip": "not-an-ip"}
];

let report = ip_lookup_bulk(requests, "ip");
print sprintf("checked %d, %d valid, %d invalid", report.total, report.valid_count, report.invalid_count);
print report.results;

// Each results[i] is a flat row (original fields + verification fields),
// so it drops straight into the normal save helpers - no special-casing
// needed. The browser playground runs with ProtectHost enabled, so
// filesystem/DB writes are blocked here; use the CLI, REPL, or
// cmd/interpreter for a trusted profile that allows them.
// write_json("checked_ips.json", report.results, {"pretty": true});
// write_csv("checked_ips.csv", report.results);
// import "database" as database;
// let db, _ = database.db_connect("sqlite", "audit.db");
// for (let i = 0; i < len(report.results); i = i + 1) {
//   let row = report.results[i];
//   database.db_exec(db, "INSERT INTO ip_checks(user, ip, valid, country) VALUES(?, ?, ?, ?)",
//     [row.user, row.input, row.valid, row.country]);
// }`,

	"wuid": `// wuid generates short, sortable, time-ordered unique IDs
// (plugins/wuid, backed by github.com/oarkflow/wuid) - a compact
// alternative to a dashed UUID that still sorts by creation time.

let id = wuid_new();
print id;

// Same underlying ID, formatted as a standard dashed UUID.
print wuid_new_uuid();

let parsed, err = wuid_parse(id);
if (err != null) { throw err; }
print parsed;

// Garbage input returns (null, err) instead of throwing.
let bad, badErr = wuid_parse("not-a-valid-id");
print bad;
print badErr;`,

	"money": `// money does fixed-point currency arithmetic
// (plugins/money, backed by github.com/oarkflow/money) - amounts are
// stored as integer minor units (cents) so results never drift like
// float64 math would.

let price, err = money_new("19.99", "USD");
if (err != null) { throw err; }

let tax = money_percent(price, 8.5);
let total, addErr = money_add(price, tax);
if (addErr != null) { throw addErr; }

print money_format(price);
print money_format(tax);
print money_format(total);

// Adding two different currencies is a hard error, not silently wrong math.
let eurPrice, _ = money_new("19.99", "EUR");
let mismatch, mismatchErr = money_add(price, eurPrice);
print mismatch;
print mismatchErr;`,

	"phone": `// phone parses, validates, and formats phone numbers
// (plugins/phone, backed by github.com/oarkflow/phone, a
// libphonenumber-equivalent parser) - useful for form validation and
// data cleanup.

let parsed, err = phone_parse("(650) 253-0000", "US");
if (err != null) { throw err; }
print parsed;

// Mobile numbers additionally get "carrier" (best-effort guessed name)
// and "network" (that carrier's MCC/MNC/PLMN cross-referenced from the
// phone_networks table below) - both are "" / null for this US landline-
// style number, since fixed lines aren't tied to one mobile network.
let mobile, mobileErr = phone_parse("9856034616", "NP");
if (mobileErr != null) { throw mobileErr; }
print mobile;

// phone_networks(region[, opts]) lists the full MCC/MNC/PLMN operator
// table for a country directly, unfiltered by any specific number -
// useful when you need every known network, not just the one guessed
// for a single parsed number. opts.status narrows out retired/reserved
// entries (the raw dataset includes plenty of historical noise).
print phone_networks("NP", {"status": "Operational"});

// Convenience check: never throws, just true/false.
print phone_valid("(650) 253-0000", "US");
print phone_valid("not a phone number", "US");

print phone_country("AU");

// phone_parse_bulk validates a phone field across many records in one
// call. It accepts an ARRAY (of HASH rows, like these, or plain strings)
// or a TABLE_VALUE - the same shape read_json/read_csv/db_query/xql_run/
// table_rows already return, so it drops straight into a pipeline like:
// let rows = table_rows(read_csv("contacts.csv")); phone_parse_bulk(rows, "phone", {"default_region": "US"});
let contacts = [
	{"name": "Ada", "phone": "(650) 253-0000"},
	{"name": "Grace", "phone": "+61 2 9374 4000", "region": "AU"},
	{"name": "Linus", "phone": "not a phone number"}
];

// region_field looks up a per-record region column (falling back to
// default_region) - handy for international contact lists.
let report = phone_parse_bulk(contacts, "phone", {"default_region": "US", "region_field": "region"});
print sprintf("checked %d, %d valid, %d invalid", report.total, report.valid_count, report.invalid_count);
print report.results;

// Each results[i] is a flat row (original fields + verification fields),
// so it drops straight into the normal save helpers - no special-casing
// needed. The browser playground runs with ProtectHost enabled, so
// filesystem/DB writes are blocked here; use the CLI, REPL, or
// cmd/interpreter for a trusted profile that allows them.
// write_json("verified_contacts.json", report.results, {"pretty": true});
// write_csv("verified_contacts.csv", report.results);
// import "database" as database;
// let db, _ = database.db_connect("sqlite", "contacts.db");
// for (let i = 0; i < len(report.results); i = i + 1) {
//   let row = report.results[i];
//   database.db_exec(db, "INSERT INTO contacts(name, phone, valid, e164) VALUES(?, ?, ?, ?)",
//     [row.name, row.input, row.valid, row.e164]);
// }`,

	"email": `// email validates addresses (plugins/emailvalidator, backed by
// github.com/oarkflow/ev) - syntax/normalization, disposable-domain,
// role-account, and free-provider detection never touch the network;
// DNS/SMTP deliverability checks do, and need the "network" capability
// the browser playground doesn't grant, so this example passes
// check_dns: false explicitly (the CLI/REPL/cmd/interpreter default it
// to true).

let syntax = email_validate_syntax("User@Example.COM");
print syntax;

print email_is_disposable("test@mailinator.com");
print email_is_role_account("admin+ops@example.com");
print email_is_free_provider("someone@gmail.com");

let result, err = email_validate("user@example.com", {"check_dns": false});
if (err != null) { throw err; }
print result.verdict;
print result.risk_score;
print result.reasons;

// email_validate_bulk checks an email field across many records in one
// call. It accepts an ARRAY (of HASH rows, like these, or plain strings)
// or a TABLE_VALUE - the same shape read_json/read_csv/db_query/xql_run/
// table_rows already return, so it drops straight into a pipeline like:
// let rows = table_rows(read_csv("signups.csv")); email_validate_bulk(rows, "email");
// Bulk defaults check_dns/check_smtp to false (unlike email_validate)
// so large batches stay fast and capability-free unless requested.
let signups = [
	{"name": "Ada", "email": "ada@example.com"},
	{"name": "Grace", "email": "grace@mailinator.com"},
	{"name": "Linus", "email": "not-an-email"}
];

let report = email_validate_bulk(signups, "email");
print sprintf("checked %d, %d valid, %d invalid", report.total, report.valid_count, report.invalid_count);
print report.results;

// Each results[i] is a flat row (original fields + verification fields),
// so it drops straight into the normal save helpers - the browser
// playground runs with ProtectHost enabled, so filesystem/DB writes are
// blocked here; use the CLI, REPL, or cmd/interpreter for a trusted
// profile that allows them.
// write_json("verified_signups.json", report.results, {"pretty": true});
// write_csv("verified_signups.csv", report.results);`,

	"securetoken": `// securetoken issues authenticated, encrypted tokens
// (plugins/crypto, backed by github.com/oarkflow/securetoken) - an
// AES-256-GCM "s1.local." token as a lighter-weight alternative to
// jwt_encode/jwt_decode above.

let token = securetoken_encrypt({"sub": "user123", "role": "admin"}, "top-secret");
print token;

let claims = securetoken_decrypt(token, "top-secret");
print claims;

// Decrypting with the wrong secret is a hard error - the AEAD tag won't
// verify, so tampered or mismatched tokens are rejected outright.
// let rejected = securetoken_decrypt(token, "wrong-secret");`,

	"naturaldate": `// naturaldate parses natural-language date/time expressions
// (plugins/naturaldate, backed by github.com/oarkflow/naturaldate).

let r, err = naturaldate_parse("tomorrow at 9am");
if (err != null) { throw err; }
print r;

print naturaldate_parse("next monday");
print naturaldate_parse("in 3 business days");

// A soft failure returns (null, err) instead of throwing - useful for
// scripts that scan free-form input and only some of it is a date.
let bad, badErr = naturaldate_parse("this is not a date at all");
print bad;
print badErr;

// Extract every date/time expression embedded in free-form text.
print naturaldate_parse_all("remind me tomorrow at 9am and again next friday");

// Pin the "now" reference and timezone via opts instead of the wall clock.
print naturaldate_parse("next friday", {
	"reference": "2026-01-01T00:00:00Z",
	"location": "America/New_York"
});`,

	"xql-basics": `// XQL is a federated query language over local values, HTTP calls, and
// connected data sources (builtins/xql). A tagged xql code block queries
// SPL arrays/hashes directly by variable name.

import "xql";

let users = [
	{"id": 1, "name": "Ada", "active": true},
	{"id": 2, "name": "Linus", "active": true},
	{"id": 3, "name": "Grace", "active": false}
];

let active, err = xql` + "```" + `
users
|> filter active == true
|> keep id, name
` + "```" + `;

if (err != null) { throw err; }
print active;

// xql_list_integrations() lists data sources registered via xql_connect().
print xql_list_integrations();`,

	"pdf-tools": `// PDF tools (builtins/pdf, backed by github.com/oarkflow/pdf).
// PDF generation/manipulation writes files, so it's preview-first here:
// the browser playground runs the untrusted profile, which blocks
// filesystem writes by default. Run these in cmd/interpreter or the
// REPL (trusted profile) to actually create/modify files.

// Inspect an existing PDF (read-only, works under the untrusted profile
// once PLAYGROUND_ALLOWED_FILE_READ_PATHS or --profile trusted grants it):
// let info = pdf_info("report.pdf");
// print sprintf("pages=%d encrypted=%t", info.pages, info.encrypted);
// print pdf_validate("report.pdf");
// print pdf_to_text("report.pdf");
// print pdf_search("report.pdf", "invoice", {"case_sensitive": false});
// print pdf_list_form_fields("report.pdf");

// Generate / manipulate PDFs (writes - trusted profile only):
// pdf_quick("Hello from SPL", "out/hello.pdf");
// pdf_from_html("<h1>Invoice</h1><p>Total: $42</p>", "out/invoice.pdf");
// pdf_from_markdown("# Report\n\nGenerated by SPL.", "out/report.pdf", {"theme": "modern", "toc": true});
// pdf_merge("out/combined.pdf", "out/hello.pdf", "out/invoice.pdf");
// pdf_split("out/combined.pdf", "out/first.pdf", "1");
// pdf_delete_pages("out/combined.pdf", "out/trimmed.pdf", "2");
// pdf_reorder("out/combined.pdf", "out/reordered.pdf", "2,1");
// pdf_rotate("out/combined.pdf", "out/rotated.pdf", "1", 90);
// pdf_watermark("out/combined.pdf", "out/watermarked.pdf", "DRAFT", {"opacity": 0.25, "angle": 45});
// pdf_add_page_numbers("out/combined.pdf", "out/numbered.pdf", {"format": "Page %d of %d"});
// pdf_set_metadata("out/combined.pdf", "out/tagged.pdf", {"Title": "Q1 Report", "Author": "SPL"});
// pdf_stamp_image("out/combined.pdf", "out/stamped.pdf", "logo.png", {"x": 36, "y": 36, "width": 80, "height": 40});
// pdf_compress("out/combined.pdf", "out/compressed.pdf");
// pdf_protect("out/combined.pdf", "out/protected.pdf", "user-pw", "owner-pw", "aes-128");
// pdf_decrypt("out/protected.pdf", "out/decrypted.pdf", "user-pw");
// pdf_extract_images("out/combined.pdf", "out/images");
// pdf_images_to_pdf("out/photos.pdf", ["a.jpg", "b.jpg"], {"size": "a4", "image_fit": "contain"});
// pdf_fill_form("form.pdf", "out/filled.pdf", {"name": "Ada Lovelace"});

print "PDF builtins are documented here for CLI/REPL/cmd/interpreter workflows.";`,

	"rules": `// rules is a BCL-backed policy/decision engine (plugins/rules, wrapping
// github.com/oarkflow/rules) - describe business/authorization decisions
// declaratively ("does this payment need manual review?") instead of
// scattered if statements. This wraps the core publish/evaluate/
// activate/rollback loop; see docs/features/50 for the full surface.

let svc = rules_service({"environment": "dev"});

let policy = ` + "`" + `module "access" {
  decision_schema "access" { effects [allow, deny] default deny strategy first_match }
  decision_table "access" {
    default deny
    hit_policy first
    row "allow-verified" {
      when { request.verified == true }
      then { decision allow reason "verified user" }
    }
  }
}` + "`" + `;

let [pub, perr] = rules_publish(svc, "access-policy", policy, {"version": "1"});
if (perr != null) { throw perr; }
print sprintf("published version=%s", pub.Definition.Version);

let [allowed, e1] = rules_evaluate(svc, "access-policy", "access", {"request": {"verified": true}});
if (e1 != null) { throw e1; }
// Hash keys mirror the Go struct field names as-is (capitalized), not this
// language's usual snake_case/camelCase convention - see docs/features/50.
print sprintf("verified=true  -> effect=%s allowed=%t", allowed.Report.Decision.Effect, allowed.Report.Decision.Allowed);

let [denied, e2] = rules_evaluate(svc, "access-policy", "access", {"request": {"verified": false}});
if (e2 != null) { throw e2; }
print sprintf("verified=false -> effect=%s allowed=%t", denied.Report.Decision.Effect, denied.Report.Decision.Allowed);`,

	"tcpguard": `// tcpguard is a runtime HTTP security policy engine (plugins/tcpguard,
// wrapping github.com/oarkflow/tcpguard) - describe request-level
// security rules (rate abuse, bad user agents, sensitive-path
// protection, risk scoring) in BCL and evaluate requests against them.
// guard_middleware(server, guard) attaches this automatically to every
// request on a real server() - see examples/tcpguard_all_in_one.spl and
// docs/features/51 for that end-to-end flow; this playground-safe
// version sticks to ad-hoc evaluation so it doesn't open a real socket.

// Write policy blocks one field per line - condensing certain blocks onto
// a single line (risk { base 90 } in particular) can silently degrade
// enforcement (block -> monitor) with no parse error. See docs/features/51.
let policy = ` + "`" + `
pack "example-security-pack" {
  version "1.0.0"
  mode enforce
}

guard "tcpguard-main" {
  mode enforce
  version "1"
}

rule "protect-admin" {
  scope {
    methods ["GET", "POST"]
    paths ["/admin/*"]
  }

  trigger {
    on request.received
  }

  when {
    any {
      request.user_agent equals ""
      request.user_agent contains "sqlmap"
    }
  }

  risk {
    base 90
  }

  actions {
    critical {
      run block
    }
  }
}
` + "`" + `;

let [bundle, lerr] = tcpguard_load(policy); // inline block; a file/dir path also works, auto-detected
if (lerr != null) { throw lerr; }
let [guard, gerr] = tcpguard_new(bundle, {"mode": "enforce"});   // {"mode": "enforce"|"monitor", "geoip": bool}
if (gerr != null) { throw gerr; }

let [blocked, e1] = tcpguard_evaluate(guard, {"method": "GET", "path": "/admin/secret", "headers": {"User-Agent": ["sqlmap/1.0"]}});
if (e1 != null) { throw e1; }
print sprintf("sqlmap UA on /admin/secret -> effect=%s allowed=%t", blocked.Effect, blocked.Allowed);

let [allowed, e2] = tcpguard_evaluate(guard, {"method": "GET", "path": "/hello", "headers": {"User-Agent": ["normal-client/1.0"]}});
if (e2 != null) { throw e2; }
print sprintf("normal UA on /hello         -> effect=%s allowed=%t", allowed.Effect, allowed.Allowed);`,
}
