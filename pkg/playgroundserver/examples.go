package playgroundserver

func baseCodeExamples() map[string]string {
	return map[string]string{
		"hello": `print "Hello SPL Playground";`,
		"functions": `function add(a, b) {
	return a + b;
}

let makeMultiplier = function(factor) {
	return function(value) {
		return value * factor;
	};
};

let fib = function fib(n) {
	if (n < 2) {
		return n;
	}
	return fib(n - 1) + fib(n - 2);
};

let scores = [6, 7, 8];
let fibScores = scores.map(function fibScore(n) {
	if (n < 2) {
		return n;
	}
	return fibScore(n - 1) + fibScore(n - 2);
});

let double = makeMultiplier(2);
print add(20, 22);
print double(21);
print fib(8);
print fibScores;`,
		"formatting": `let payload = {"name": "spl", "ok": true, "count": 3};
print sprintf("name=%s type=%T val=%v", payload.name, payload, payload);
print interpolate("Hello {name}, items={count}", {"name": "Playground", "count": 3});`,
		"artifacts": `// Renderable artifact commands
// The playground collects these values and opens them in Preview/Artifacts.

// Load a repository file as a previewable artifact.
print file("docs/README.md", {
	"name": "README.md",
	"mime": "text/markdown",
	"max_bytes": 200000
});

// Load an inline image artifact.
let logo = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSIyNDAiIGhlaWdodD0iMTIwIj48cmVjdCB3aWR0aD0iMjQwIiBoZWlnaHQ9IjEyMCIgZmlsbD0iIzA4OTFiMiIvPjx0ZXh0IHg9IjI0IiB5PSI3MCIgZm9udC1zaXplPSIzMCIgZmlsbD0id2hpdGUiPlNQTCBhcnRpZmFjdDwvdGV4dD48L3N2Zz4=";
print image(logo, {
	"name": "spl-artifact.svg",
	"alt": "SPL artifact badge",
	"width": 240,
	"height": 120
});

// Render inline HTML directly into the browser preview.
print render("<!doctype html><html><body style=\"font-family:system-ui;padding:2rem\"><h1>SPL artifact</h1><p>Loaded with render(...).</p></body></html>", {
	"name": "artifact.html",
	"mime": "text/html"
});

// URL artifacts are intentionally disabled by default in the playground.
// Enable PLAYGROUND_RENDER_ALLOW_URLS and PLAYGROUND_RENDER_ALLOW_URL_HOSTS
// before loading remote artifacts such as:
// print image("https://example.com/image.png", {"name": "remote.png"});`,
		"file-values": `// File values load content into variables so you can inspect and transform it.
// This example stays read-only, so it runs cleanly inside the browser playground.

let note = file_load("testdata/test_io.txt");
print sprintf("name=%s mime=%s size=%d", file_name(note), file_mime(note), file_size(note));
print sprintf("base64-prefix=%s", substring(file_bytes(note), 0, 20));

let profile = file_load(file("testdata/data/profile.json", {
	"name": "profile.json",
	"mime": "application/json"
}));

print file_text(note);
print render(profile, {
	"name": "profile.preview.json",
	"mime": "application/json"
});`,
		"image-values": `// Core image artifacts work without optional image-codec plugins.
// Use cmd/interpreter or import builtins/images in an embedded host for image_load/image_resize.

let tiny = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAAEElEQVR4nGL6z8AACAAA//8DCQECWLbVUAAAAABJRU5ErkJggg==";
let rendered = image(tiny, {"mime": "image/png", "name": "tiny.png", "alt": "Inline playground image", "width": 48, "height": 48});
let binary = file_load(rendered);

print sprintf("rendered mime=%s size=%d", file_mime(binary), file_size(binary));
print rendered;`,
		"json-csv-values": `// Structured data helpers work with JSON files, CSV files, and in-memory tables.

let profile = read_json("testdata/data/profile.json");
let roster = read_csv("testdata/data/people.csv");
let engineers = table_filter(roster, function(row) {
	return row.role == "engineer";
});
let labels = table_map(engineers, function(row) {
	return {
		"name": row.name,
		"label": row.name + " (" + row.role + ")",
		"location": row.location
	};
});
let compact = table_select(labels, ["name", "label"]);
let adhoc = csv_decode("name,score\nAda,9\nLinus,8");

print sprintf("team=%s columns=%v", profile.team, table_columns(compact));
print table_rows(compact);
print csv_encode(compact);
print render(compact, {"name": "engineers.csv"});
print render(adhoc, {"name": "scores.csv"});`,
		"write-ops": `// Write-oriented helpers are available in SPL, but the browser playground runs with
// ProtectHost enabled, so filesystem writes are intentionally blocked here.
// Use this script in the CLI or REPL when filesystem write access is allowed.

let payload = file_load("testdata/test_io.txt");
let tiny = image("data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAAEElEQVR4nGL6z8AACAAA//8DCQECWLbVUAAAAABJRU5ErkJggg==", {
	"mime": "image/png",
	"name": "tiny.png"
});

// file_save(payload, "tmp/test_io_copy.txt");
// file_copy("testdata/test_io.txt", "tmp/test_io_copy_2.txt");
// file_move("tmp/test_io_copy_2.txt", "tmp/test_io_moved.txt");
// file_rename("tmp/test_io_moved.txt", "test_io_renamed.txt");
// write_json("tmp/profile.json", {"team": "playground", "ok": true}, {"pretty": true});
// write_csv("tmp/people.csv", [{"name": "Ada", "role": "engineer"}]);
// Optional images module examples in cmd/interpreter:
// import "images" as images;
// let loaded = images.load(tiny);
// import "tools/images" as toolsImages;
// toolsImages.image_resize_file("tmp/tiny.png", "tmp/tiny-large.png", {"width": 48, "height": 48, "apply": true});
// toolsImages.image_convert_batch("tmp", "tmp/converted", {"to": "jpeg", "apply": true});

print "Write helpers are documented here for CLI/REPL workflows.";`,
		"tools-files": `// Daily file tools are preview-first. This example only reads repository files
// and produces operation plans; it does not rename, move, or write anything.

import "tools/files";

let rename_plan = bulk_rename("testdata", {
	"match": "*.spl",
	"template": "{date}_{seq}.{ext}",
	"apply": false
});

let matches = file_search("testdata", {
	"ext": "spl",
	"name": "examples",
	"content": "print",
	"sort": "name",
	"limit": 3
});

let finder_matches = file_finder("testdata")
	.files()
	.regex("^examples_.*\\.spl$")
	.content_regex("print\\s+")
	.limit(2)
	.exec();

print rename_plan;
print matches;
print finder_matches;`,
		"tools-images": `// Image tools expose batch operations for CLI/REPL workflows.
// The browser playground blocks writes, so the mutating example stays commented.

import "tools/images";

// let convert_plan = image_convert_batch("photos", "web", {
// 	"from": ["jpg", "png"],
// 	"to": "png",
// 	"apply": false
// });

print "Use image_convert_batch(..., {\"apply\": false}) to preview image chores.";`,
		"tools-media": `// Media tools use ffmpeg/ffprobe when available.
// Install and conversion operations are explicit and preview-first.

import "tools/media";

let status = ffmpeg_status();
// let install_preview = ffmpeg_install({"apply": false});
// let convert_preview = media_convert("input.mov", "output.mp4", {
// 	"codec": "libx264",
// 	"crf": "23",
// 	"install": true,
// 	"apply": false
// });

print status;`,
		"tools-secrets": `// Secret tools return masked SECRET values by default.

import "tools/secrets";

let password = secret_generate(32);
let token = token_generate(24);

print secret_mask(password);
print secret_mask(token);`,
		"tools-network": `// System and network probes are gated by the system/network capabilities.
// The default browser sandbox restricts both, so this example stays
// preview-first; run it in cmd/interpreter or the REPL (trusted
// profile), or grant the capabilities explicitly, to execute live.

import "tools/system";
import "tools/network";

// let info = system_info();
// let ips = dns_lookup("example.com");
// let tcp = tcp_check("example.com:443", 2000);
// let probe = http_probe("https://example.com", 5000);

print "system_info, dns_lookup, tcp_check, and http_probe need system/network capability grants - see comments above.";`,
		"modules": `// Standard modules are virtual and need no companion files.
import "std/math" as math;

print math.gcd(18, 24);
print math.round_to(math.percent(1, 4), 1);`,
		"std-modules": `// Virtual std modules group builtins into capability-oriented namespaces.
// The global builtins still work; std modules make larger programs easier to scan.

import "std/core" as core;
import "std/fs" as fs;
import "std/test" as testing;
import "std/config" as config;

print "=== std module aliases ===";
print core.sprintf("runtime=%s answer=%d", "spl", 42);

let exists = fs.file_exists("testdata/test_io.txt");
print core.sprintf("test file exists=%t", exists);

let content, readErr = fs.read_file("testdata/test_io.txt");
if (readErr != null) {
	throw readErr;
}
print core.sprintf("test file bytes=%d", len(content));

let parsed = config.config_parse("{\"name\":\"spl\",\"ok\":true}", "json");
print core.sprintf("parsed=%s ok=%t", parsed.name, parsed.ok);

assert_eq(core.sprintf("%s-%d", "std", 1), "std-1");
assert_true(exists, "expected repository test file to exist");
print testing.test_summary();`,
		"package-imports": `// Package-style imports resolve through spl.mod / spl.lock.
// A package may expose files such as "vendor/math.spl".
// import "vendor/math.spl" as pkgmath;
// print pkgmath.answer;
print "Add a package mapping before enabling the import above.";`,
		"collections": `let nums = [1, 2, 3, 4, 5, 6];
let doubled = nums.map(function(x) { return x * 2; });
let evens = nums.filter(function(x) { return x % 2 == 0; });
let total = nums.reduce(function(acc, x) { return acc + x; }, 0);
let chained = nums
	.map(function(x) { return x * 3; })
	.filter(function(x) { return x > 10; })
	.reduce(function(acc, x) { return acc + x; }, 0);

print doubled;
print evens;
print total;
print chained;
print {"first_even": evens[0], "count": len(nums)};`,
		"error-handling": `let payload = {"name": "spl", "count": 3};
print sprintf("payload=%v type=%T", payload, payload);

let recovered = try {
	throw "demo error";
} catch (e) {
	"caught: " + e;
};

print recovered;`,
		"pattern-matching": `// Match works as an expression and supports literals, wildcards,
// variable bindings, destructuring, guards, ranges, OR patterns, and extractors.

let badge = match ("active") {
	case "active" => "green"
	case "pending" => "yellow"
	case _ => "gray"
};
print sprintf("badge=%s", badge);

let classify = function(value) {
	return match (value) {
		case n: integer if n >= 90 => "high integer"
		case n: integer => "integer " + n
		case s: string => "string " + s
		case [head, ...tail] => sprintf("array head=%v tail=%d", head, len(tail))
		case {type: "click", target: t} => "clicked " + t
		case null => "nothing"
		case _ => "unknown"
	};
};

print classify(42);
print classify("spl");
print classify([10, 20, 30]);
print classify({"type": "click", "target": "button"});
print classify(null);

let grade = match (86) {
	case 90..100 => "A"
	case 80..89 => "B"
	case 70..79 => "C"
	case _ => "needs review"
};
print sprintf("grade=%s", grade);

let maybeName = "Ada";
let greeting = match (maybeName) {
	case Some(name: string) => "hello " + name
	case None => "no name"
};
print greeting;`,
		"loops": `// for loops with range
for (let i = 0; i < 5; i = i + 1) {
	print sprintf("i = %d", i);
}

// while loop
let count = 3;
while (count > 0) {
	print sprintf("countdown: %d", count);
	count = count - 1;
}
print "liftoff!";`,
		"math": `// Math constants (called as functions)
print sprintf("PI = %v", PI());
print sprintf("E  = %v", E());

// Trigonometry
print sprintf("sin(PI/2) = %v", sin(PI() / 2));
print sprintf("cos(0)    = %v", cos(0));
print sprintf("atan2(1,1)= %v", atan2(1, 1));

// Logarithms and powers
print sprintf("log(E)    = %v", log(E()));
print sprintf("log2(8)   = %v", log2(8));
print sprintf("log10(100)= %v", log10(100));
print sprintf("sqrt(144) = %v", sqrt(144));
print sprintf("pow(2,10) = %v", pow(2, 10));
print sprintf("hypot(3,4)= %v", hypot(3, 4));

// Utility
print sprintf("abs(-42)  = %v", abs(-42));
print sprintf("round(3.7)= %v", round(3.7));
print sprintf("min(5,2)  = %v", min(5, 2));
print sprintf("max(5,2)  = %v", max(5, 2));
print sprintf("clamp(15, 0, 10) = %v", clamp(15, 0, 10));`,
		"strings": `// Case transforms
print upper("hello spl");
print lower("HELLO SPL");
print title("hello world");
print snake_case("helloWorld");
print kebab_case("helloWorld");
print camel_case("hello_world");
print pascal_case("hello_world");

// Search and manipulation
let text = "SPL is a scripting language";
print starts_with(text, "SPL");
print ends_with(text, "language");
print index_of(text, "scripting");
print count_substr(text, "a");
print replace(text, "scripting", "template");
print substring(text, 0, 3);

// Trim and pad
print trim("  hello  ");
print pad_left("42", 6, "0");
print pad_right("hi", 8, ".");
print truncate(text, 15, "...");
print repeat("ha", 3);

// Regex
print regex_match("abc123", "^[a-z]+[0-9]+$");
print regex_replace("hello 2024 world 42", "[0-9]+", "#");`,
		"collections-advanced": `// Array helpers
let nums = [5, 3, 1, 4, 2];
print sprintf("first=%v last=%v", first(nums), last(nums));
print sprintf("rest=%v", rest(nums));
print sprintf("sorted=%v", sort(nums));
print sprintf("reversed=%v", reverse(nums));
print sprintf("sum=%v avg=%v", sum(nums), avg(nums));
print sprintf("slice(1,3)=%v", slice(nums, 1, 3));

let nested = [[1, 2], [3, [4, 5]]];
print sprintf("flatten=%v", flatten(nested));

let mixed = [0, "", null, "ok", 42, false];
print sprintf("compact=%v", compact(mixed));

// group_by groups array of hashes by a string key
let people = [
	{"name": "alice", "dept": "eng"},
	{"name": "bob", "dept": "sales"},
	{"name": "carol", "dept": "eng"},
	{"name": "dave", "dept": "sales"}
];
let byDept = group_by(people, "dept");
print sprintf("grouped=%v", byDept);

// Hash helpers
let user = {"name": "alice", "role": "admin", "active": true};
print sprintf("keys=%v", keys(user));
print sprintf("values=%v", values(user));
print sprintf("has role=%v", has_key(user, "role"));

let defaults = {"theme": "light", "lang": "en"};
let prefs = {"theme": "dark"};
print sprintf("merged=%v", merge(defaults, prefs));

// any/all check truthiness of elements
print sprintf("any truthy=%v", any([0, false, 1]));
print sprintf("all truthy=%v", all([1, true, "ok"]));

// Method-style callbacks on arrays
let scores = [85, 92, 78, 95];
let high = scores.filter(function(s) { return s > 90; });
let found = scores.find(function(s) { return s > 80; });
print sprintf("high scores=%v", high);
print sprintf("first >80=%v", found);`,
		"crypto": `// Hashing
let msg = "Hello SPL";
print sprintf("sha256=%s", hash("sha256", msg));
print sprintf("md5=%s", hash("md5", msg));

// HMAC (algo, key, data)
let secret = "my-secret-key";
print sprintf("hmac=%s", hmac("sha256", secret, msg));

// Hex encoding
let encoded = hex_encode("SPL");
print sprintf("hex_encode=%s", encoded);
print sprintf("hex_decode=%s", hex_decode(encoded));

// UUID
print sprintf("uuid=%s", uuid());

// Random generation (length, alphabet)
print sprintf("random_string=%s", random_string(16, "abcdefghijklmnopqrstuvwxyz0123456789"));

// URL encoding
let url = "hello world&foo=bar";
let urlEnc = url_encode(url);
print sprintf("url_encode=%s", urlEnc);
print sprintf("url_decode=%s", url_decode(urlEnc));

// JSON round-trip
let data = {"key": "value", "nums": [1, 2, 3]};
let json = json_encode(data);
print sprintf("json=%s", json);
print sprintf("decoded=%v", json_decode(json));`,
		"time": `// Current time
print sprintf("now=%v", now());
print sprintf("now_iso=%s", now_iso());
print sprintf("time_ms=%v", time_ms());

// Parsing and formatting
let stamp = iso_to_unix("2024-06-15T14:30:00Z");
print sprintf("unix=%v", stamp);
print sprintf("formatted=%s", format_time(stamp, "YYYY-MM-DD HH:mm:ss"));
print sprintf("back_to_iso=%s", unix_to_iso(stamp));

// Time arithmetic (unix, amount, unit)
let future = time_add(stamp, 1, "h");
print sprintf("+1 hour=%s", format_time(future, "HH:mm:ss"));

let diff = time_diff(future, stamp);
print sprintf("diff=%v seconds", diff);

// Day boundaries
let dayStart = start_of_day(stamp);
let dayEnd = end_of_day(stamp);
print sprintf("day_start=%s", format_time(dayStart, "HH:mm:ss"));
print sprintf("day_end=%s", format_time(dayEnd, "HH:mm:ss"));

// Add months
let nextMonth = add_months(stamp, 1);
print sprintf("next_month=%s", format_time(nextMonth, "YYYY-MM-DD"));`,
		"testing": `// Built-in assertion functions
assert_true(1 + 1 == 2);
assert_eq(len("hello"), 5);
assert_neq("foo", "bar");
assert_contains([1, 2, 3], 2);

// Test that errors are thrown correctly
assert_throws(function() {
	throw "expected error";
});

// More assertions
assert_eq(upper("hi"), "HI");
assert_eq(abs(-42), 42);
assert_true(starts_with("hello", "hel"));
assert_contains("hello world", "world");

// View test summary
let summary = test_summary();
print sprintf("total=%v passed=%v failed=%v", summary.total, summary.passed, summary.failed);`,
		"type-casting": `// === Type Conversion ===
print sprintf("to_int(\"42\")     = %v", to_int("42"));
print sprintf("to_int(3.14)     = %v", to_int(3.14));
print sprintf("to_int(true)     = %v", to_int(true));
print sprintf("to_float(\"3.14\") = %v", to_float("3.14"));
print sprintf("to_float(42)     = %v", to_float(42));
print sprintf("to_string(42)    = %v", to_string(42));
print sprintf("to_string(true)  = %v", to_string(true));

// === Parsing ===
print sprintf("parse_float(\"3.14\") = %v", parse_float("3.14"));
print sprintf("parse_bool(\"true\")  = %v", parse_bool("true"));

// === Type Checking ===
print sprintf("typeof(42)    = %s", typeof(42));
print sprintf("typeof(3.14)  = %s", typeof(3.14));
print sprintf("typeof(\"hi\")  = %s", typeof("hi"));
print sprintf("typeof(true)  = %s", typeof(true));
print sprintf("typeof([1,2]) = %s", typeof([1,2]));
print sprintf("typeof(null)  = %s", typeof(null));

// === Type Predicates ===
print sprintf("is_int(42)      = %t", is_int(42));
print sprintf("is_float(3.14)  = %t", is_float(3.14));
print sprintf("is_number(42)   = %t", is_number(42));
print sprintf("is_number(3.14) = %t", is_number(3.14));
print sprintf("is_string(\"hi\") = %t", is_string("hi"));
print sprintf("is_bool(true)   = %t", is_bool(true));
print sprintf("is_array([1])   = %t", is_array([1]));
print sprintf("is_hash({})     = %t", is_hash({}));
print sprintf("is_null(null)   = %t", is_null(null));
print sprintf("is_function(len)= %t", is_function(len));

// === Numeric with mixed types ===
print sprintf("abs(-3.14)       = %v", abs(-3.14));
print sprintf("min(3.14, 2.71)  = %v", min(3.14, 2.71));
print sprintf("max(42, 3.14)    = %v", max(42, 3.14));

// === sprintf verbs ===
print sprintf("%%t: %t", true);
print sprintf("%%c: %c", 65);
print sprintf("%%q: %q", "hello world");
print sprintf("%%d from float: %d", 3.14);
print sprintf("%%f from int:   %f", 42);`,
		"stateful-server": `// SPL Stateful Server
// The server is a long-lived, stateful process — not a fragment or API stub.
// It manages routes, middleware, sessions, and in-memory state.

import "server" as svr;

let app = svr.server(3000);

// In-memory state lives on the server; the client never touches it.
let users = {};
let nextID = 1;

// Middleware runs server-side on every request.
svr.middleware(app, function(req, res, next) {
	print sprintf("[%s] %s %s", now_iso(), req.method, req.path);
	res.header("X-Powered-By", "SPL");
	next();
});

// All flow logic is server-side: create, read, lookup, delete.
svr.route(app, "GET", "/api/users", function(req, res) {
	res.json(values(users));
});

svr.route(app, "POST", "/api/users", function(req, res) {
	let body = req.json();
	let id = to_string(nextID);
	nextID = nextID + 1;
	let user = {"id": id, "name": body.name, "email": body.email, "created": now_iso()};
	users[id] = user;
	res.status(201).json(user);
});

svr.route(app, "GET", "/api/users/:id", function(req, res) {
	let id = req.param("id");
	if (has_key(users, id)) {
		res.json(users[id]);
	} else {
		res.status(404).json({"error": "user not found"});
	}
});

svr.route(app, "DELETE", "/api/users/:id", function(req, res) {
	let id = req.param("id");
	if (has_key(users, id)) {
		let removed = users[id];
		delete(users, id);
		res.json({"deleted": removed.name});
	} else {
		res.status(404).json({"error": "user not found"});
	}
});

svr.route(app, "GET", "/api/stats", function(req, res) {
	let fib = function fib(n) {
		if (n < 2) {
			return n;
		}
		return fib(n - 1) + fib(n - 2);
	};
	res.json({
		"total_users": len(users),
		"next_id": nextID,
		"uptime": now_iso(),
		"fib_8": fib(8)
	});
});

print "Stateful server defined with " + to_string(len(app.routes)) + " routes";
print "State (users, nextID) lives in server memory — client is a thin shell";
print "All create/read/delete decisions happen server-side";`,

		"server-middleware": `// Server Middleware Chain
// Middleware runs on the server, wrapping every handler.
// The client sends plain requests; the server decides auth, logging, CORS.

import "server" as svr;

let app = svr.server(3001);

// Request counter — server-side state, invisible to client.
let requestCount = 0;

// Logging middleware — server decides what to log.
svr.middleware(app, function(req, res, next) {
	requestCount = requestCount + 1;
	print sprintf("#%d %s %s", requestCount, req.method, req.path);
	next();
});

// Auth middleware on /api — server controls access, not the client.
svr.middleware(app, "/api", function(req, res, next) {
	let token = req.get_header("Authorization");
	if (token == null) {
		res.status(401).json({"error": "missing token"});
		return null;
	}
	if (!starts_with(token, "Bearer ")) {
		res.status(401).json({"error": "invalid token format"});
		return null;
	}
	// Server validates the token — client just sends it.
	next();
});

// CORS middleware — server sets the policy.
svr.middleware(app, function(req, res, next) {
	res.header("Access-Control-Allow-Origin", "*");
	res.header("Access-Control-Allow-Methods", "GET, POST, DELETE");
	res.header("Access-Control-Allow-Headers", "Authorization, Content-Type");
	next();
});

svr.route(app, "GET", "/health", function(req, res) {
	res.json({"ok": true, "requests_served": requestCount});
});

svr.route(app, "GET", "/api/protected", function(req, res) {
	res.json({"message": "you passed server-side auth", "total_requests": requestCount});
});

print "Middleware chain defined: logging -> auth -> CORS -> handler";
print "Client sends plain HTTP — server owns the entire request pipeline";`,

		"reactive-state": `// Reactive State Management (Server-Side Signals)
// Signals, computed values, and effects run on the server.
// The client never manages derived state or triggers recalculations.

let count = signal("count", 0);
let multiplier = signal("multiplier", 3);

// Computed value auto-tracks its signal dependencies on the server.
let tripled = computed(function() {
	return count.value * multiplier.value;
});

// Effects run server-side when their tracked signals change.
let log = effect(function() {
	print sprintf("count=%d multiplier=%d tripled=%d", count.value, multiplier.value, tripled.value);
});

// Mutate state — effects fire automatically on the server.
print "--- Setting count to 5 ---";
count.set(5);

print "--- Setting multiplier to 10 ---";
multiplier.set(10);

// Updater function receives previous value — server computes new state.
print "--- Incrementing count via updater ---";
count.set(function(prev) { return prev + 1; });

print sprintf("Final: count=%d tripled=%d", count.value, tripled.value);
print "All reactive computation happened server-side";`,

		"scheduler": `// Server-Side Job Scheduling
// The scheduler is a stateful server component.
// Jobs, timers, and execution are managed entirely on the server.

let runLog = [];

// Schedule a recurring job using cron — server keeps the schedule.
let jobA = schedule("* * * * *", "heartbeat", function() {
	let entry = sprintf("heartbeat at %s", now_iso());
	print entry;
});

// Interval-based job — server tracks timing.
let jobB = schedule_interval("2s", "cleanup", function() {
	print sprintf("cleanup tick at %s", now_iso());
});

// One-shot job — server fires it once and deactivates.
let jobC = schedule_once("* * * * *", "init", function() {
	print "one-time initialization complete";
});

// List all jobs — server owns the registry.
let jobs = schedule_list();
print sprintf("Scheduled %d jobs:", len(jobs));
for (let i = 0; i < len(jobs); i = i + 1) {
	let j = jobs[i];
	print sprintf("  %s: %s (active=%v)", j.id, j.name, j.active);
}

// Run due jobs synchronously for demo purposes.
let executed = schedule_run(1);
print sprintf("Executed %d due jobs", executed);

// Cancel a job — server removes it from the registry.
schedule_cancel(jobB);
print sprintf("Cancelled job %s", jobB);

// Background task — server runs it in a goroutine.
let future = background(function() {
	return "async work done on server";
});

print "Scheduler and background tasks are server-managed";
print "Client never polls, retries, or sequences — server does it all";`,

		"runtime-session": `// Runtime workspace events power spltool session, the REPL, and embedded hosts.
// In the playground the script still emits metric/trace events and render artifacts.

let orders = [
	{"id": "A-100", "region": "east", "amount": 120},
	{"id": "A-101", "region": "west", "amount": 80},
	{"id": "A-102", "region": "east", "amount": 240}
];

let large = orders.filter(function(order) {
	return order.amount >= 100;
});

let total = large.reduce(function(sum, order) {
	return sum + order.amount;
}, 0);

metric("workspace.large_orders", len(large), {"source": "playground"});
trace("workspace.total", {"total": total});

print sprintf("large_orders=%d total=%d", len(large), total);
print render(large, {"name": "large-orders.json"});
total;`,

		"config-secrets": `// Config helpers parse JSON, YAML, and .env text.
// Secret-like keys are wrapped and print as *** until explicitly revealed.

let raw = "APP_NAME=spl\nDB_HOST=localhost\nDB_PASSWORD=super-secret\nAPI_TOKEN=token-123";
let cfg = config_parse(raw, "env");

print cfg.APP_NAME;
print cfg.DB_HOST;
print cfg.DB_PASSWORD;
print secret_mask(secret_reveal(cfg.DB_PASSWORD), 2);

let jsonCfg = config_parse("{\"service\":{\"name\":\"playground\"},\"api_key\":\"abc123\"}", "json");
print jsonCfg.service.name;
print jsonCfg.api_key;
print sprintf("revealed length=%d", len(secret_reveal(jsonCfg.api_key)));`,

		"resource-limits": `// The playground already runs with bounded depth, steps, heap, source size,
// request body size, and wall-clock timeout. CLI/embedding callers can add
// stricter string, array, hash, and import limits.

import "std/core" as core;

let smallArray = [1, 2, 3];
print core.sprintf("array length=%d", len(smallArray));

let smallHash = {"name": "limits", "kind": "example"};
print core.sprintf("hash keys=%d", len(keys(smallHash)));

let message = "resource limit example";
print core.sprintf("message length=%d", len(message));

// CLI examples:
// go run ./cmd/interpreter --max-array-length 2 examples/all_in_one.spl
// go run ./cmd/interpreter --max-hash-entries 1 examples/all_in_one.spl
// go run ./cmd/interpreter --max-string-bytes 8 examples/all_in_one.spl

print "resource limit example complete";`,

		"production-profile": `// The playground normally uses the untrusted execution profile.
// Read-only work is allowed, while host mutation is blocked unless explicitly enabled.

import "std/core" as core;
import "std/fs" as fs;

print "=== production profile smoke ===";

let content, err = fs.read_file("testdata/test_io.txt");
if (err != null) {
	throw err;
}

print core.sprintf("read-only file access ok, bytes=%d", len(content));
print core.sprintf("string helpers still work: %s", trim("  production  ").upper());

// These intentionally stay commented in the browser playground:
// exec("echo", "blocked");
// fs.write_file("blocked.txt", "should not be written");
// fs.read_file("/etc/passwd");

print "untrusted profile smoke complete";`,

		"query-builder": `// Query builder and lazy query templates.
// These require a configured database connection, so this playground version
// prints the SPL you would use in CLI, REPL, or an embedded host.

let template = "import \"database\" as database;\n" +
"let db, err = database.connect(\"sqlite\", \":memory:\");\n" +
"let ok, _ = database.exec(db, \"CREATE TABLE users (id INTEGER, name TEXT, active BOOLEAN)\");\n" +
"let _, _ = database.exec(db, \"INSERT INTO users(id, name, active) VALUES(?, ?, ?)\", [1, \"Ada\", true]);\n" +
"let rows, qerr = database.query(db, \"users\")\n" +
"  .select(\"id\", \"name\")\n" +
"  .where(\"active\", true)\n" +
"  .where_in(\"id\", [1, 2, 3])\n" +
"  .where_like(\"name\", \"%a%\")\n" +
"  .order_by(\"name ASC\")\n" +
"  .limit(20)\n" +
"  .exec();\n" +
"print rows;";

print "=== query builder template ===";
print template;
print "Use database.lazy_query(db, \"users\") or database.query(db, \"users\").lazy() to defer execution.";`,

		"server-sse": `// Server-Sent Events (SSE)
// The server pushes events to the client over a long-lived connection.
// The client just listens — no polling, no flow control.

import "server" as svr;

let app = svr.server(3002);

// In-memory event log — server state.
let eventLog = [];

svr.route(app, "GET", "/events", function(req, res) {
	// Server initiates the SSE stream.
	let sse = res.sse();

	// Server decides what to send and when.
	let i = 0;
	while (i < 5) {
		let payload = json_encode({"seq": i, "time": now_iso()});
		sse.send("tick", payload);
		let entry = sprintf("sent event %d", i);
		eventLog = append(eventLog, entry);
		print entry;
		i = i + 1;
	}

	sse.send("done", json_encode({"total": i}));
	sse.close();
});

svr.route(app, "GET", "/log", function(req, res) {
	res.json({"events": eventLog, "count": len(eventLog)});
});

print "SSE server defined — server pushes events, client only listens";
print "Event sequence, timing, and payload are all server decisions";`,

		"server-route-groups": `// Route Groups and Web App Pattern
// All URL structure and handler mapping is server-side.
// The client hits URLs — it does not decide which handler runs.

import "server" as svr;

let app = svr.web_app("./templates");

// API group — server organizes routes by prefix.
svr.route_group(app, "/api/v1", "GET", "/health", function(req, res) {
	res.json({"status": "ok", "version": "1.0"});
});

svr.route_group(app, "/api/v1", "GET", "/users", function(req, res) {
	res.json([
		{"id": 1, "name": "Alice"},
		{"id": 2, "name": "Bob"}
	]);
});

svr.route_group(app, "/api/v1", "GET", "/users/:id", function(req, res) {
	let id = req.param("id");
	// Server resolves the user — client just requested a URL.
	res.json({"id": to_int(id), "name": "User " + id});
});

// Admin group — server decides access, not the client.
svr.route_group(app, "/admin", "GET", "/dashboard", function(req, res) {
	res.json({"section": "dashboard", "stats": {"users": 42, "active": 18}});
});

svr.route_group(app, "/admin", "GET", "/settings", function(req, res) {
	res.json({"section": "settings", "features": ["auth", "logging", "rate-limit"]});
});

// Static files served by the server.
svr.static(app, "/assets/", "./public");

print "Web app with route groups:";
let routes = app.routes;
for (let i = 0; i < len(routes); i = i + 1) {
	print sprintf("  %s %s", routes[i].method, routes[i].pattern);
}
print "URL dispatch, grouping, and static serving are all server-owned";`,

		"reactive-html": `// Reactive HTML — Server-Rendered with Embedded Reactivity
// The server builds the entire HTML page including reactive behavior.
// The client receives a self-contained document — zero API round-trips.
// All state, logic, and rendering decisions are made server-side.

let title = "Task Manager";
let theme_color = "#6366f1";

// Server builds the items and renders them into HTML at response time.
let tasks = [
	{"id": 1, "text": "Build stateful server", "done": true},
	{"id": 2, "text": "Move flow logic to server", "done": true},
	{"id": 3, "text": "Remove client-side state", "done": false},
	{"id": 4, "text": "Add reactive HTML rendering", "done": false}
];

// Server computes stats — client never calculates these.
let total = len(tasks);
let completed = 0;
for (let i = 0; i < total; i = i + 1) {
	if (tasks[i].done) {
		completed = completed + 1;
	}
}
let pct = 0;
if (total > 0) {
	pct = round((completed * 100) / total);
}

// Build task list HTML server-side.
let task_items = "";
for (let i = 0; i < total; i = i + 1) {
	let t = tasks[i];
	let checked = "";
	let style = "";
	if (t.done) {
		checked = " checked";
		style = ' style="text-decoration:line-through;opacity:.6"';
	}
	task_items = task_items + sprintf(<<HTML
<li class="task"%s><label><input type="checkbox" data-id="%d"%s> %s</label></li>
HTML
	, style, t.id, checked, t.text);
}

// Server produces the complete HTML document with embedded reactive JS.
// The reactive behavior (checkbox toggle, add task) runs client-side but
// operates only on the rendered DOM — no API calls, no server round-trips.
let html = sprintf(<<HTML
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:system-ui,-apple-system,sans-serif;background:#f8fafc;color:#1e293b;min-height:100vh;display:flex;justify-content:center;padding:2rem 1rem}
.app{width:100%%;max-width:480px}
h1{font-size:1.5rem;font-weight:700;color:%s;margin-bottom:1.5rem;display:flex;align-items:center;gap:.5rem}
h1::before{content:'\2713';background:%s;color:white;width:2rem;height:2rem;border-radius:.5rem;display:flex;align-items:center;justify-content:center;font-size:.875rem}
.stats{display:flex;gap:1rem;margin-bottom:1.5rem}
.stat{flex:1;background:white;border:1px solid #e2e8f0;border-radius:.75rem;padding:.75rem 1rem;text-align:center}
.stat-value{font-size:1.5rem;font-weight:700;color:%s}
.stat-label{font-size:.75rem;color:#64748b;text-transform:uppercase;letter-spacing:.05em}
.progress{height:.5rem;background:#e2e8f0;border-radius:.25rem;margin-bottom:1.5rem;overflow:hidden}
.progress-bar{height:100%%;background:linear-gradient(90deg,%s,%s);border-radius:.25rem;transition:width .3s ease}
.task-list{list-style:none;display:flex;flex-direction:column;gap:.5rem;margin-bottom:1.5rem}
.task{background:white;border:1px solid #e2e8f0;border-radius:.75rem;padding:.75rem 1rem;transition:all .2s}
.task:hover{border-color:%s;box-shadow:0 1px 3px rgba(0,0,0,.08)}
.task label{display:flex;align-items:center;gap:.75rem;cursor:pointer;font-size:.9375rem}
.task input[type=checkbox]{width:1.125rem;height:1.125rem;accent-color:%s}
.add-form{display:flex;gap:.5rem}
.add-form input{flex:1;padding:.625rem .875rem;border:1px solid #e2e8f0;border-radius:.5rem;font-size:.875rem;outline:none}
.add-form input:focus{border-color:%s;box-shadow:0 0 0 3px rgba(99,102,241,.15)}
.add-form button{padding:.625rem 1.25rem;background:%s;color:white;border:none;border-radius:.5rem;font-weight:600;cursor:pointer;font-size:.875rem}
.add-form button:hover{opacity:.9}
.footer{margin-top:1.5rem;text-align:center;font-size:.75rem;color:#94a3b8}
</style>
</head>
<body>
<div class="app">
<h1>%s</h1>
<div class="stats">
 <div class="stat"><div class="stat-value" id="totalCount">%d</div><div class="stat-label">Total</div></div>
 <div class="stat"><div class="stat-value" id="doneCount">%d</div><div class="stat-label">Done</div></div>
 <div class="stat"><div class="stat-value" id="pctDisplay">%d%%</div><div class="stat-label">Progress</div></div>
</div>
<div class="progress"><div class="progress-bar" id="progressBar" style="width:%d%%"></div></div>
<ul class="task-list" id="taskList">%s</ul>
<form class="add-form" id="addForm">
 <input type="text" id="newTask" placeholder="Add a new task..." required>
 <button type="submit">Add</button>
</form>
<div class="footer">Server-rendered by SPL &mdash; no API calls needed</div>
</div>
<script>
// Minimal client-side JS for interactivity — operates on the DOM only.
// No fetch(), no API endpoints, no server round-trips.
let nextId=%d;
function updateStats(){
 const items=document.querySelectorAll('.task');
 const done=document.querySelectorAll('.task input:checked').length;
 const total=items.length;
 const pct=total?Math.round(done/total*100):0;
 document.getElementById('totalCount').textContent=total;
 document.getElementById('doneCount').textContent=done;
 document.getElementById('pctDisplay').textContent=pct+'%%';
 document.getElementById('progressBar').style.width=pct+'%%';
 items.forEach(li=>{
  const cb=li.querySelector('input');
  li.style.textDecoration=cb.checked?'line-through':'none';
  li.style.opacity=cb.checked?'.6':'1';
 });
}
document.getElementById('taskList').addEventListener('change',updateStats);
document.getElementById('addForm').addEventListener('submit',function(e){
 e.preventDefault();
 const input=document.getElementById('newTask');
 const text=input.value.trim();
 if(!text)return;
 const li=document.createElement('li');
 li.className='task';
 li.innerHTML='<label><input type="checkbox" data-id="'+nextId+'"> '+text+'</label>';
 document.getElementById('taskList').appendChild(li);
 nextId++;
 input.value='';
 updateStats();
});
</script>
</body>
</html>
HTML
	, title, theme_color, theme_color, theme_color,
	theme_color, theme_color, theme_color, theme_color,
	theme_color, theme_color,
	title, total, completed, pct, pct,
	task_items, total + 1
);

print html;`,

		"complete-tour": `// Complete SPL playground tour: closures, loops, collections,
// formatting, JSON, crypto/time helpers, and structured error handling.
print "=== Complete SPL Playground Tour ===";
print sprintf("language=%s total=%d", "SPL", 40 + 2);

const RATE = 1.13;
let prices = [12, 18, 25, 31];

let makeScaler = function(multiplier) {
	return function(value) {
		return value * multiplier;
	};
};

let scale = makeScaler(3);
print sprintf("closure scale(7) = %d", scale(7));

let adjusted = prices.map(function(price) {
	return round(price * RATE);
});
let premium = adjusted.filter(function(price) {
	return price >= 20;
});
let total = premium.reduce(function(acc, price) {
	return acc + price;
}, 0);

print sprintf("adjusted=%v", adjusted);
print sprintf("premium=%v", premium);
print sprintf("total=%d", total);

let summary = {
	"module": "complete-tour",
	"items": len(prices),
	"premium_count": len(premium),
	"total": total,
	"ok": true
};

let encoded = json_encode(summary);
let decoded = json_decode(encoded);
print sprintf("json total=%d items=%d", decoded.total, decoded.items);

let stamp = iso_to_unix("2020-01-01T00:00:00Z");
print sprintf("time=%s", format_time(stamp, "YYYY-MM-DD HH:mm:ss"));
print sprintf("sha256=%s", hash("sha256", encoded));

let finalMessage = try {
	if (decoded.total < 50) {
		throw "total too small";
	}
	interpolate("{name} processed {count} premium items", {"name": "SPL", "count": decoded.premium_count});
} catch (e) {
	"recovered: " + e;
};

print finalMessage;
print "=== Tour Complete ===";`,
	}
}

// buildExamples merges the shared base example set with a variant's
// overrides (replacing specific keys, e.g. image-values/query-builder
// which differ based on which builtins are linked) and extras (additional
// keys unique to the variant, e.g. pdf-tools). This is the single source of
// truth for cmd/interpreter --playground's example content - see
// Variant.ExampleOverrides/ExtraExamples.
func buildExamples(v Variant) map[string]string {
	base := baseCodeExamples()
	out := make(map[string]string, len(base)+len(v.ExtraExamples))
	for k, val := range base {
		out[k] = val
	}
	for k, val := range v.ExampleOverrides {
		out[k] = val
	}
	for k, val := range v.ExtraExamples {
		out[k] = val
	}
	return out
}
