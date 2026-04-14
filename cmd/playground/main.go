package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/oarkflow/interpreter"
)

//go:embed static/*
var staticFS embed.FS

type executeRequest struct {
	Code string `json:"code"`
}

type executeResponse struct {
	Output      string   `json:"output"`
	Result      string   `json:"result"`
	ResultType  string   `json:"result_type"`
	Error       string   `json:"error"`
	ErrorKind   string   `json:"error_kind"`
	Diagnostics []string `json:"diagnostics,omitempty"`
	DurationMS  int64    `json:"duration_ms"`
}

func builtinCodeExamples() map[string]string {
	return map[string]string{
		"hello": `print "Hello SPL Playground";`,
		"functions": `let add = function(a, b) {
	return a + b;
};

let makeMultiplier = function(factor) {
	return function(value) {
		return value * factor;
	};
};

let double = makeMultiplier(2);
print add(20, 22);
print double(21);`,
		"formatting": `let payload = {"name": "spl", "ok": true, "count": 3};
print sprintf("name=%s type=%T val=%v", payload.name, payload, payload);
print interpolate("Hello {name}, items={count}", {"name": "Playground", "count": 3});`,
		"modules": `import "testdata/modules/math.spl" as math;
import {label} from "testdata/modules/math.spl";

print label;
print math.base + math.increment;`,
		"collections": `let nums = [1, 2, 3, 4, 5, 6];
let doubled = nums.map(function(x) { return x * 2; });
let evens = nums.filter(function(x) { return x % 2 == 0; });
let total = nums.reduce(function(acc, x) { return acc + x; }, 0);

print doubled;
print evens;
print total;
print {"first_even": evens[0], "count": len(nums)};`,
		"error-handling": `let payload = {"name": "spl", "count": 3};
print sprintf("payload=%v type=%T", payload, payload);

let recovered = try {
	throw "demo error";
} catch (e) {
	"caught: " + e;
};

print recovered;`,
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

let app = server(3000);

// In-memory state lives on the server; the client never touches it.
let users = {};
let nextID = 1;

// Middleware runs server-side on every request.
middleware(app, function(req, res, next) {
	print sprintf("[%s] %s %s", now_iso(), req.method, req.path);
	res.header("X-Powered-By", "SPL");
	next();
});

// All flow logic is server-side: create, read, lookup, delete.
route(app, "GET", "/api/users", function(req, res) {
	res.json(values(users));
});

route(app, "POST", "/api/users", function(req, res) {
	let body = req.json();
	let id = to_string(nextID);
	nextID = nextID + 1;
	let user = {"id": id, "name": body.name, "email": body.email, "created": now_iso()};
	users[id] = user;
	res.status(201).json(user);
});

route(app, "GET", "/api/users/:id", function(req, res) {
	let id = req.param("id");
	if (has_key(users, id)) {
		res.json(users[id]);
	} else {
		res.status(404).json({"error": "user not found"});
	}
});

route(app, "DELETE", "/api/users/:id", function(req, res) {
	let id = req.param("id");
	if (has_key(users, id)) {
		let removed = users[id];
		delete(users, id);
		res.json({"deleted": removed.name});
	} else {
		res.status(404).json({"error": "user not found"});
	}
});

route(app, "GET", "/api/stats", function(req, res) {
	res.json({
		"total_users": len(users),
		"next_id": nextID,
		"uptime": now_iso()
	});
});

print "Stateful server defined with " + to_string(len(app.routes)) + " routes";
print "State (users, nextID) lives in server memory — client is a thin shell";
print "All create/read/delete decisions happen server-side";`,

		"server-middleware": `// Server Middleware Chain
// Middleware runs on the server, wrapping every handler.
// The client sends plain requests; the server decides auth, logging, CORS.

let app = server(3001);

// Request counter — server-side state, invisible to client.
let requestCount = 0;

// Logging middleware — server decides what to log.
middleware(app, function(req, res, next) {
	requestCount = requestCount + 1;
	print sprintf("#%d %s %s", requestCount, req.method, req.path);
	next();
});

// Auth middleware on /api — server controls access, not the client.
middleware(app, "/api", function(req, res, next) {
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
middleware(app, function(req, res, next) {
	res.header("Access-Control-Allow-Origin", "*");
	res.header("Access-Control-Allow-Methods", "GET, POST, DELETE");
	res.header("Access-Control-Allow-Headers", "Authorization, Content-Type");
	next();
});

route(app, "GET", "/health", function(req, res) {
	res.json({"ok": true, "requests_served": requestCount});
});

route(app, "GET", "/api/protected", function(req, res) {
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

		"server-sse": `// Server-Sent Events (SSE)
// The server pushes events to the client over a long-lived connection.
// The client just listens — no polling, no flow control.

let app = server(3002);

// In-memory event log — server state.
let eventLog = [];

route(app, "GET", "/events", function(req, res) {
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

route(app, "GET", "/log", function(req, res) {
	res.json({"events": eventLog, "count": len(eventLog)});
});

print "SSE server defined — server pushes events, client only listens";
print "Event sequence, timing, and payload are all server decisions";`,

		"server-route-groups": `// Route Groups and Web App Pattern
// All URL structure and handler mapping is server-side.
// The client hits URLs — it does not decide which handler runs.

let app = web_app("./templates");

// API group — server organizes routes by prefix.
route_group(app, "/api/v1", "GET", "/health", function(req, res) {
	res.json({"status": "ok", "version": "1.0"});
});

route_group(app, "/api/v1", "GET", "/users", function(req, res) {
	res.json([
		{"id": 1, "name": "Alice"},
		{"id": 2, "name": "Bob"}
	]);
});

route_group(app, "/api/v1", "GET", "/users/:id", function(req, res) {
	let id = req.param("id");
	// Server resolves the user — client just requested a URL.
	res.json({"id": to_int(id), "name": "User " + id});
});

// Admin group — server decides access, not the client.
route_group(app, "/admin", "GET", "/dashboard", function(req, res) {
	res.json({"section": "dashboard", "stats": {"users": 42, "active": 18}});
});

route_group(app, "/admin", "GET", "/settings", function(req, res) {
	res.json({"section": "settings", "features": ["auth", "logging", "rate-limit"]});
});

// Static files served by the server.
static(app, "/assets/", "./public");

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
		style = " style=\"text-decoration:line-through;opacity:.6\"";
	}
	task_items = task_items + sprintf(
		"<li class=\"task\"%s><label><input type=\"checkbox\" data-id=\"%d\"%s> %s</label></li>\n",
		style, t.id, checked, t.text
	);
}

// Server produces the complete HTML document with embedded reactive JS.
// The reactive behavior (checkbox toggle, add task) runs client-side but
// operates only on the rendered DOM — no API calls, no server round-trips.
let html = sprintf("<!DOCTYPE html>
<html lang=\"en\">
<head>
<meta charset=\"utf-8\">
<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">
<title>%s</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:system-ui,-apple-system,sans-serif;background:#f8fafc;color:#1e293b;min-height:100vh;display:flex;justify-content:center;padding:2rem 1rem}
.app{width:100%%;max-width:480px}
h1{font-size:1.5rem;font-weight:700;color:%s;margin-bottom:1.5rem;display:flex;align-items:center;gap:.5rem}
h1::before{content:'\\2713';background:%s;color:white;width:2rem;height:2rem;border-radius:.5rem;display:flex;align-items:center;justify-content:center;font-size:.875rem}
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
<div class=\"app\">
<h1>%s</h1>
<div class=\"stats\">
 <div class=\"stat\"><div class=\"stat-value\" id=\"totalCount\">%d</div><div class=\"stat-label\">Total</div></div>
 <div class=\"stat\"><div class=\"stat-value\" id=\"doneCount\">%d</div><div class=\"stat-label\">Done</div></div>
 <div class=\"stat\"><div class=\"stat-value\" id=\"pctDisplay\">%d%%</div><div class=\"stat-label\">Progress</div></div>
</div>
<div class=\"progress\"><div class=\"progress-bar\" id=\"progressBar\" style=\"width:%d%%\"></div></div>
<ul class=\"task-list\" id=\"taskList\">%s</ul>
<form class=\"add-form\" id=\"addForm\">
 <input type=\"text\" id=\"newTask\" placeholder=\"Add a new task...\" required>
 <button type=\"submit\">Add</button>
</form>
<div class=\"footer\">Server-rendered by SPL &mdash; no API calls needed</div>
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
 li.innerHTML='<label><input type=\"checkbox\" data-id=\"'+nextId+'\"> '+text+'</label>';
 document.getElementById('taskList').appendChild(li);
 nextId++;
 input.value='';
 updateStats();
});
</script>
</body>
</html>",
	title, theme_color, theme_color, theme_color,
	theme_color, theme_color, theme_color, theme_color,
	theme_color, theme_color,
	title, total, completed, pct, pct,
	task_items, total + 1
);

print html;`,

		"complete-tour": `// Complete SPL playground tour: modules, closures, loops, collections,
// formatting, JSON, crypto/time helpers, and structured error handling.
import "testdata/modules/math.spl" as math;
import {label} from "testdata/modules/math.spl";

print "=== Complete SPL Playground Tour ===";
print sprintf("module=%s total=%d", label, math.base + math.increment);

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
	"module": label,
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

type playgroundConfig struct {
	Addr            string
	AuthSecret      string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	MaxBodyBytes    int64
	RateLimit       int
	RateWindow      time.Duration
	RateCleanup     time.Duration
	TrustProxy      bool
	CookieSecure    bool
	SessionTTL      time.Duration
	EvalMaxDepth    int
	EvalMaxSteps    int64
	EvalMaxHeapMB   int64
	EvalTimeoutMS   int64
}

func loadConfig() (playgroundConfig, error) {
	cfg := playgroundConfig{
		Addr:            envString("PLAYGROUND_ADDR", ":8080"),
		AuthSecret:      envString("PLAYGROUND_AUTH_SECRET", envString("PLAYGROUND_API_KEY", "")),
		ReadTimeout:     envDurationMS("PLAYGROUND_READ_TIMEOUT_MS", 15000),
		WriteTimeout:    envDurationMS("PLAYGROUND_WRITE_TIMEOUT_MS", 15000),
		IdleTimeout:     envDurationMS("PLAYGROUND_IDLE_TIMEOUT_MS", 30000),
		ShutdownTimeout: envDurationMS("PLAYGROUND_SHUTDOWN_TIMEOUT_MS", 10000),
		MaxBodyBytes:    envInt64("PLAYGROUND_MAX_BODY_BYTES", 1<<20),
		RateLimit:       envInt("PLAYGROUND_RATE_LIMIT", 60),
		RateWindow:      envDurationMS("PLAYGROUND_RATE_WINDOW_MS", 60000),
		RateCleanup:     envDurationMS("PLAYGROUND_RATE_CLEANUP_MS", 120000),
		TrustProxy:      envBool("PLAYGROUND_TRUST_PROXY_HEADERS", false),
		CookieSecure:    envBool("PLAYGROUND_COOKIE_SECURE", false),
		SessionTTL:      envDurationMS("PLAYGROUND_SESSION_TTL_MS", 12*60*60*1000),
		EvalMaxDepth:    envInt("PLAYGROUND_EVAL_MAX_DEPTH", 200),
		EvalMaxSteps:    envInt64("PLAYGROUND_EVAL_MAX_STEPS", 2_000_000),
		EvalMaxHeapMB:   envInt64("PLAYGROUND_EVAL_MAX_HEAP_MB", 256),
		EvalTimeoutMS:   envInt64("PLAYGROUND_EVAL_TIMEOUT_MS", 8_000),
	}

	if cfg.MaxBodyBytes <= 0 {
		return playgroundConfig{}, errors.New("PLAYGROUND_MAX_BODY_BYTES must be > 0")
	}
	if cfg.RateLimit <= 0 {
		return playgroundConfig{}, errors.New("PLAYGROUND_RATE_LIMIT must be > 0")
	}
	if cfg.RateWindow <= 0 {
		return playgroundConfig{}, errors.New("PLAYGROUND_RATE_WINDOW_MS must be > 0")
	}
	if cfg.RateCleanup <= 0 {
		return playgroundConfig{}, errors.New("PLAYGROUND_RATE_CLEANUP_MS must be > 0")
	}
	if cfg.ReadTimeout <= 0 || cfg.WriteTimeout <= 0 || cfg.IdleTimeout <= 0 || cfg.ShutdownTimeout <= 0 {
		return playgroundConfig{}, errors.New("timeout values must be > 0")
	}
	if cfg.EvalMaxDepth <= 0 || cfg.EvalMaxSteps <= 0 || cfg.EvalMaxHeapMB <= 0 || cfg.EvalTimeoutMS <= 0 {
		return playgroundConfig{}, errors.New("playground eval limits must be > 0")
	}
	// AuthSecret is optional – when unset the playground runs without authentication.
	if cfg.SessionTTL <= 0 {
		return playgroundConfig{}, errors.New("PLAYGROUND_SESSION_TTL_MS must be > 0")
	}
	return cfg, nil
}

func envString(name, fallback string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	return v
}

func envInt(name string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envInt64(name string, fallback int64) int64 {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func envDurationMS(name string, fallbackMS int64) time.Duration {
	ms := envInt64(name, fallbackMS)
	if ms <= 0 {
		ms = fallbackMS
	}
	return time.Duration(ms) * time.Millisecond
}

func envBool(name string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

type rateLimiter struct {
	mu      sync.Mutex
	clients map[string]*clientCounter
	limit   int
	window  time.Duration
}

type clientCounter struct {
	count int
	reset time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{clients: make(map[string]*clientCounter), limit: limit, window: window}
}

func (rl *rateLimiter) allow(key string, now time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cc, ok := rl.clients[key]
	if !ok || now.After(cc.reset) {
		rl.clients[key] = &clientCounter{count: 1, reset: now.Add(rl.window)}
		return true
	}
	if cc.count >= rl.limit {
		return false
	}
	cc.count++
	return true
}

func (rl *rateLimiter) prune(now time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for key, cc := range rl.clients {
		if now.After(cc.reset) {
			delete(rl.clients, key)
		}
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := loadConfig()
	if err != nil {
		logger.Error("invalid configuration", slog.String("error", err.Error()))
		os.Exit(2)
	}

	rl := newRateLimiter(cfg.RateLimit, cfg.RateWindow)
	var auth *authManager
	if cfg.AuthSecret != "" {
		auth = newAuthManager(cfg.AuthSecret, cfg.SessionTTL)
		go startAuthCleanup(auth, cfg.RateCleanup)
	}
	authEnabled := auth != nil
	metrics := newPlaygroundMetrics()
	go startRateLimiterCleanup(rl, cfg.RateCleanup)
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		status := http.StatusOK
		defer func() {
			metrics.recordRequest(r.URL.Path, r.Method, status, time.Since(start))
		}()
		if r.Method != http.MethodGet {
			status = http.StatusMethodNotAllowed
			writeJSON(w, status, map[string]any{"error": "method not allowed"})
			return
		}
		writeJSON(w, status, map[string]any{"ok": true, "service": "spl-playground"})
	})
	mux.HandleFunc("/api/ready", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		status := http.StatusOK
		defer func() {
			metrics.recordRequest(r.URL.Path, r.Method, status, time.Since(start))
		}()
		if r.Method != http.MethodGet {
			status = http.StatusMethodNotAllowed
			writeJSON(w, status, map[string]any{"error": "method not allowed"})
			return
		}
		writeJSON(w, status, map[string]any{"ok": true, "ready": true})
	})

	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		status := http.StatusOK
		defer func() {
			metrics.recordRequest(r.URL.Path, r.Method, status, time.Since(start))
		}()

		if r.Method != http.MethodGet {
			status = http.StatusMethodNotAllowed
			writeJSON(w, status, map[string]any{"error": "method not allowed"})
			return
		}
		if !authEnabled {
			writeJSON(w, status, map[string]any{
				"authenticated":  true,
				"auth_enabled":   false,
				"session_ttl_ms": cfg.SessionTTL.Milliseconds(),
			})
			return
		}
		token := tokenFromRequest(r)
		authed := auth.validate(token)
		metrics.recordAuth("session_check")
		if authed {
			metrics.setActiveSessions(auth.activeSessions())
		}
		writeJSON(w, status, map[string]any{
			"authenticated":  authed,
			"auth_enabled":   true,
			"session_ttl_ms": cfg.SessionTTL.Milliseconds(),
		})
	})

	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		status := http.StatusOK
		defer func() {
			metrics.recordRequest(r.URL.Path, r.Method, status, time.Since(start))
		}()

		if r.Method != http.MethodPost {
			status = http.StatusMethodNotAllowed
			writeJSON(w, status, map[string]any{"error": "method not allowed"})
			return
		}
		if ct := strings.TrimSpace(r.Header.Get("Content-Type")); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "application/json") {
			status = http.StatusUnsupportedMediaType
			writeJSON(w, status, map[string]any{"error": "content type must be application/json"})
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, cfg.MaxBodyBytes)
		var req struct {
			Secret string `json:"secret"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			status = http.StatusBadRequest
			writeJSON(w, status, map[string]any{"error": "invalid json payload"})
			return
		}
		if !auth.verifySecret(req.Secret) {
			metrics.recordAuth("login_failure")
			status = http.StatusUnauthorized
			writeJSON(w, status, map[string]any{"error": "unauthorized"})
			return
		}
		token, _, err := auth.issue()
		if err != nil {
			status = http.StatusInternalServerError
			writeJSON(w, status, map[string]any{"error": "failed to create session"})
			return
		}
		writeSessionCookie(w, token, cfg.CookieSecure || r.TLS != nil, cfg.SessionTTL)
		metrics.recordAuth("login_success")
		metrics.setActiveSessions(auth.activeSessions())
		writeJSON(w, status, map[string]any{
			"ok":             true,
			"authenticated":  true,
			"token":          token,
			"token_type":     "bearer",
			"session_ttl_ms": cfg.SessionTTL.Milliseconds(),
		})
	})

	mux.HandleFunc("/api/logout", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		status := http.StatusOK
		defer func() {
			metrics.recordRequest(r.URL.Path, r.Method, status, time.Since(start))
		}()

		if r.Method != http.MethodPost {
			status = http.StatusMethodNotAllowed
			writeJSON(w, status, map[string]any{"error": "method not allowed"})
			return
		}
		token := tokenFromRequest(r)
		auth.revoke(token)
		clearSessionCookie(w, cfg.CookieSecure || r.TLS != nil)
		metrics.recordAuth("logout")
		metrics.setActiveSessions(auth.activeSessions())
		writeJSON(w, status, map[string]any{"ok": true})
	})

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		status := http.StatusOK
		defer func() {
			metrics.recordRequest(r.URL.Path, r.Method, status, time.Since(start))
		}()

		if r.Method != http.MethodGet {
			status = http.StatusMethodNotAllowed
			http.Error(w, "method not allowed", status)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(metrics.renderPrometheus()))
	})

	mux.HandleFunc("/api/examples", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		status := http.StatusOK
		defer func() {
			metrics.recordRequest(r.URL.Path, r.Method, status, time.Since(start))
		}()
		if r.Method != http.MethodGet {
			status = http.StatusMethodNotAllowed
			writeJSON(w, status, map[string]any{"error": "method not allowed"})
			return
		}
		examples := builtinCodeExamples()
		writeJSON(w, status, map[string]any{"examples": examples})
	})

	mux.HandleFunc("/api/execute", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		status := http.StatusOK
		defer func() {
			metrics.recordRequest(r.URL.Path, r.Method, status, time.Since(start))
		}()
		if r.Method != http.MethodPost {
			status = http.StatusMethodNotAllowed
			writeJSON(w, status, map[string]any{"error": "method not allowed"})
			return
		}
		if !isAuthenticated(r, auth) {
			metrics.recordAuth("unauthorized")
			status = http.StatusUnauthorized
			writeJSON(w, status, map[string]any{"error": "unauthorized"})
			return
		}
		clientID := clientKey(r, cfg.TrustProxy)
		if !rl.allow(clientID, time.Now()) {
			metrics.recordRateLimited()
			status = http.StatusTooManyRequests
			writeJSON(w, status, map[string]any{"error": "rate limit exceeded"})
			return
		}

		if ct := strings.TrimSpace(r.Header.Get("Content-Type")); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "application/json") {
			status = http.StatusUnsupportedMediaType
			writeJSON(w, status, map[string]any{"error": "content type must be application/json"})
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, cfg.MaxBodyBytes)
		var req executeRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				status = http.StatusBadRequest
				writeJSON(w, status, map[string]any{"error": "request body is empty"})
				return
			}
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				status = http.StatusRequestEntityTooLarge
				writeJSON(w, status, map[string]any{"error": "payload too large"})
				return
			}
			status = http.StatusBadRequest
			writeJSON(w, status, map[string]any{"error": "invalid json payload"})
			return
		}
		var trailing any
		if err := dec.Decode(&trailing); err != io.EOF {
			status = http.StatusBadRequest
			writeJSON(w, status, map[string]any{"error": "invalid json payload"})
			return
		}
		if strings.TrimSpace(req.Code) == "" {
			status = http.StatusBadRequest
			writeJSON(w, status, map[string]any{"error": "code is required"})
			return
		}

		cwd, err := os.Getwd()
		if err != nil {
			status = http.StatusInternalServerError
			writeJSON(w, status, map[string]any{"error": "failed to resolve working directory"})
			return
		}
		execStart := time.Now()
		result := interpreter.EvalForPlayground(req.Code, interpreter.PlaygroundOptions{
			Args:      []string{},
			MaxDepth:  cfg.EvalMaxDepth,
			MaxSteps:  cfg.EvalMaxSteps,
			MaxHeapMB: cfg.EvalMaxHeapMB,
			TimeoutMS: cfg.EvalTimeoutMS,
			ModuleDir: cwd,
			Security: &interpreter.SecurityPolicy{
				ProtectHost: true,
			},
		})

		if result.Error != "" {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, executeResponse{
			Output:      result.Output,
			Result:      result.Result,
			ResultType:  result.ResultTy,
			Error:       result.Error,
			ErrorKind:   result.ErrorKind,
			Diagnostics: result.Diagnostics,
			DurationMS:  result.Duration,
		})
		metrics.recordExecution("execute", time.Since(execStart))
	})

	fileServer, err := fsSub()
	if err != nil {
		logger.Error("failed to load embedded static files", slog.String("error", err.Error()))
		os.Exit(2)
	}
	mux.Handle("/", fileServer)

	handler := withRecovery(logger, withSecurityHeaders(loggingMiddleware(logger, cfg.TrustProxy, mux)))
	server := &http.Server{
		Addr:         cfg.Addr,
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	ctx, stop := signalNotifyContext()
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed", slog.String("error", err.Error()))
		}
	}()

	logger.Info("SPL Playground running",
		slog.String("addr", cfg.Addr),
		slog.Int64("max_body_bytes", cfg.MaxBodyBytes),
		slog.Int("rate_limit", cfg.RateLimit),
		slog.String("rate_window", cfg.RateWindow.String()),
		slog.Bool("trust_proxy_headers", cfg.TrustProxy),
		slog.Int("eval_max_depth", cfg.EvalMaxDepth),
		slog.Int64("eval_max_steps", cfg.EvalMaxSteps),
		slog.Int64("eval_max_heap_mb", cfg.EvalMaxHeapMB),
		slog.Int64("eval_timeout_ms", cfg.EvalTimeoutMS),
	)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server terminated", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func signalNotifyContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func isAuthenticated(r *http.Request, auth *authManager) bool {
	if auth == nil {
		return true // no auth configured – open access
	}
	token := tokenFromRequest(r)
	return auth.validate(token)
}

func clientKey(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if ip := strings.TrimSpace(strings.Split(strings.TrimSpace(r.Header.Get("X-Forwarded-For")), ",")[0]); ip != "" {
			if parsed := net.ParseIP(ip); parsed != nil {
				return parsed.String()
			}
		}
		if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
			if parsed := net.ParseIP(ip); parsed != nil {
				return parsed.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if strings.TrimSpace(r.RemoteAddr) != "" {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return "unknown"
}

func loggingMiddleware(logger *slog.Logger, trustProxy bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		logger.Info("request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", sw.status),
			slog.String("remote", clientKey(r, trustProxy)),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)
	})
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func withRecovery(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("panic recovered", slog.Any("panic", rec), slog.String("path", r.URL.Path))
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func startRateLimiterCleanup(rl *rateLimiter, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for now := range ticker.C {
		rl.prune(now)
	}
}

func startAuthCleanup(auth *authManager, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for now := range ticker.C {
		auth.cleanup(now)
	}
}

func fsSub() (http.Handler, error) {
	fsys, err := staticFS.ReadFile("static/index.html")
	if err != nil || len(fsys) == 0 {
		return nil, err
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		path = filepath.Clean(path)
		if strings.Contains(path, "..") {
			http.NotFound(w, r)
			return
		}
		if path == "index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(fsys)
			return
		}
		content, err := staticFS.ReadFile("static/" + path)
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(fsys)
			return
		}
		switch {
		case strings.HasSuffix(path, ".js"):
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		case strings.HasSuffix(path, ".css"):
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case strings.HasSuffix(path, ".html"):
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		}
		_, _ = w.Write(content)
	}), nil
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
