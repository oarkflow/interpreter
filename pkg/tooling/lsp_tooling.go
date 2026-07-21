package tooling

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/lexer"
	"github.com/oarkflow/interpreter/pkg/parser"
)

var splIdentifierRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

var splKeywords = []string{
	"let", "const", "function", "return", "if", "else", "for", "while", "do", "in",
	"break", "continue", "print", "import", "export", "try", "catch", "throw",
	"switch", "case", "default", "match", "type", "class", "interface", "test",
	"init", "async", "await", "new", "true", "false", "null", "typeof", "and", "or",
	"not", "lazy", "as", "from",
}

type KeywordDoc struct {
	Name           string
	Category       string
	Syntax         string
	Interpretation string
	Example        string
}

type RuntimeDoc struct {
	Name           string
	Kind           string
	Signature      string
	Interpretation string
	Returns        string
	Example        string
}

var splKeywordDocs = map[string]KeywordDoc{
	"let":       {"let", "binding", "let name = expression; or let a, b = expression;", "Evaluates the right-hand expression and binds mutable local names in the current lexical scope.", "let total = price * qty;"},
	"const":     {"const", "binding", "const name = expression;", "Evaluates once and binds a constant name. Use it for values that should not be reassigned.", "const timeout_ms = 1500;"},
	"function":  {"function", "function", "function name(args) { ... } or function(args) { ... }", "Creates an SPL function with its own parameter scope. A named function declaration also binds the function name.", "function add(a, b) { return a + b; }"},
	"return":    {"return", "control flow", "return expression;", "Stops the current function and yields the expression as that function's value.", "return { ok: true, value: result };"},
	"if":        {"if", "control flow", "if (condition) { ... } else { ... }", "Evaluates the condition truthily and runs only the matching branch. The branch may produce a value when used as an expression.", "let label = if (ok) { \"ready\" } else { \"blocked\" };"},
	"else":      {"else", "control flow", "else { ... }", "Provides the fallback branch for an if expression or statement when the condition is falsey.", "if (count > 0) { print count; } else { print \"empty\"; }"},
	"while":     {"while", "loop", "while (condition) { ... }", "Repeats the body while the condition remains truthy. Runtime step limits still apply.", "while (i < len(items)) { i += 1; }"},
	"for":       {"for", "loop", "for (init; condition; post) { ... } or for key, value in iterable { ... }", "Runs a counted loop or iterates arrays, hashes, tables, and iterable values depending on the parsed form.", "for i, item in items { print item; }"},
	"do":        {"do", "loop", "do { ... } while (condition);", "Runs the body once before checking the loop condition.", "do { retry += 1; } while (retry < 3);"},
	"in":        {"in", "loop/operator", "for value in iterable { ... } or value in collection", "Introduces for-in iteration and can also test membership where supported by the evaluator.", "for name in names { print name; }"},
	"break":     {"break", "control flow", "break;", "Immediately exits the nearest enclosing loop or switch-like control context.", "if (done) { break; }"},
	"continue":  {"continue", "control flow", "continue;", "Skips the rest of the current loop iteration and proceeds to the next iteration.", "if (skip) { continue; }"},
	"print":     {"print", "output", "print expression;", "Evaluates the expression, writes its display form to the configured output, and continues execution.", "print \"created user\";"},
	"import":    {"import", "module", "import \"path\"; import { name } from \"path\"; import * as alias from \"path\";", "Loads another SPL module, evaluates its exports once through the module cache, and binds imported names or aliases.", "import { add } from \"./math.spl\";"},
	"export":    {"export", "module", "export let name = value; or export function name(...) { ... }", "Marks declarations as public exports for other modules to import.", "export let version = \"1.0\";"},
	"try":       {"try", "error handling", "try { ... } catch err { ... } finally { ... }", "Runs a protected block and routes thrown/runtime errors into catch/finally handling.", "try { risky(); } catch err { print err; }"},
	"catch":     {"catch", "error handling", "catch err { ... }", "Binds the thrown error for recovery after a try block fails.", "catch err { return { ok: false, err: err }; }"},
	"throw":     {"throw", "error handling", "throw expression;", "Raises an error value and unwinds until a catch block or the top-level runtime handles it.", "throw Error(\"missing config\");"},
	"switch":    {"switch", "control flow", "switch (value) { case x: { ... } default: { ... } }", "Compares a value against case labels and runs the matching case or default.", "switch (status) { case \"ok\": { print \"ready\"; } }"},
	"case":      {"case", "control flow/pattern", "case pattern => { ... } or case value: { ... }", "Defines one branch in match or switch. In match, patterns can bind names for that branch.", "case Ok(value) => { return value; }"},
	"default":   {"default", "control flow", "default: { ... }", "Fallback branch for switch when no explicit case matches.", "default: { print \"unknown\"; }"},
	"match":     {"match", "pattern matching", "match (value) { case pattern => { ... } }", "Destructures and tests a value against ordered patterns, with optional bound names scoped to each case.", "match (result) { case Ok(v) => { v; } case _ => { null; } }"},
	"type":      {"type", "type declaration", "type Name = Variant(fields) | Other;", "Declares algebraic-style constructors used by match and ordinary function calls.", "type Result = Ok(value) | Err(message);"},
	"class":     {"class", "type declaration", "class Name { ... }", "Declares a class-like SPL type parsed by the language frontend and surfaced to tooling.", "class User { }"},
	"interface": {"interface", "type declaration", "interface Name { ... }", "Declares an interface-like contract parsed by the language frontend and surfaced to tooling.", "interface Repository { }"},
	"test":      {"test", "testing", "test \"name\" { ... }", "Declares an executable test block discoverable by spltool test/conformance.", "test \"adds\" { assert_eq(add(1, 2), 3); }"},
	"init":      {"init", "module lifecycle", "init { ... }", "Declares initialization code collected during parsing and associated with module startup behavior.", "init { print \"loading\"; }"},
	"async":     {"async", "concurrency", "async function name(...) { ... }", "Marks a function form as asynchronous where supported by the parser/evaluator.", "async function fetch_user(id) { return http_get(id); }"},
	"await":     {"await", "concurrency", "await expression", "Waits for a future/asynchronous value to resolve before continuing.", "let value = await go(load);"},
	"new":       {"new", "construction", "new Type(args)", "Constructs a class-like value where the runtime supports that construct.", "let user = new User(\"Ada\");"},
	"true":      {"true", "literal", "true", "Boolean true literal.", "let enabled = true;"},
	"false":     {"false", "literal", "false", "Boolean false literal.", "let enabled = false;"},
	"null":      {"null", "literal", "null", "Null/empty value literal.", "let optional = null;"},
	"typeof":    {"typeof", "operator", "typeof expression", "Returns or participates in runtime type inspection depending on expression context.", "let kind = typeof value;"},
	"and":       {"and", "operator alias", "left and right", "Logical AND spelling recognized as a language word; prefer && when matching existing examples.", "if (ready and enabled) { print \"go\"; }"},
	"or":        {"or", "operator alias", "left or right", "Logical OR spelling recognized as a language word; prefer || when matching existing examples.", "if (cached or fresh) { return value; }"},
	"not":       {"not", "operator alias", "not expression", "Logical NOT spelling recognized as a language word; prefer ! when matching existing examples.", "if (not ok) { throw err; }"},
	"lazy":      {"lazy", "evaluation", "lazy expression", "Wraps an expression for delayed evaluation where the parser produces a lazy expression node.", "let later = lazy expensive();"},
	"as":        {"as", "import alias", "import \"path\" as alias", "Introduces an alias binding for module imports.", "import \"./math.spl\" as math;"},
	"from":      {"from", "import clause", "import { name } from \"path\"", "Separates imported names from the module path in named import syntax.", "import { parse } from \"./parser.spl\";"},
}

var splRuntimeDocs = map[string]RuntimeDoc{
	"schedule":          {"schedule", "scheduler builtin", "schedule(cron_expr, handler) or schedule(cron_expr, name, handler)", "Registers a recurring cron job. The handler must be a function or builtin and is executed by the scheduler when the cron expression matches.", "Job id string", "let job = schedule(\"*/5 * * * *\", \"health-check\", function() { print \"ok\"; });"},
	"schedule_once":     {"schedule_once", "scheduler builtin", "schedule_once(cron_expr, handler) or schedule_once(cron_expr, name, handler)", "Registers a cron job that removes itself after the first matching run.", "Job id string", "let job = schedule_once(\"0 12 * * *\", function() { print \"noon\"; });"},
	"schedule_interval": {"schedule_interval", "scheduler builtin", "schedule_interval(duration, handler) or schedule_interval(duration, name, handler)", "Registers a recurring interval job. Duration can be milliseconds as an integer or a duration string such as \"30s\", \"1m\", or \"2h\".", "Job id string", "let job = schedule_interval(\"30s\", \"poll\", function() { print \"tick\"; });"},
	"schedule_cancel":   {"schedule_cancel", "scheduler builtin", "schedule_cancel(job_id)", "Cancels a scheduled job by id. It is safe to call again; an already-cancelled or missing job returns false.", "Boolean", "let cancelled = schedule_cancel(job);"},
	"schedule_list":     {"schedule_list", "scheduler builtin", "schedule_list()", "Returns metadata for all scheduled jobs, including id, name, active state, and schedule timing fields.", "Array of hashes", "for job in schedule_list() { print job.name; }"},
	"schedule_persist":  {"schedule_persist", "scheduler builtin", "schedule_persist(path)", "Writes active scheduled jobs to a JSON file so they can be restored later.", "Boolean or error", "schedule_persist(\"./scheduled_jobs.json\");"},
	"schedule_restore":  {"schedule_restore", "scheduler builtin", "schedule_restore(path)", "Loads scheduled jobs from a JSON file created by schedule_persist.", "Boolean or count depending on runtime version", "schedule_restore(\"./scheduled_jobs.json\");"},
	"schedule_now":      {"schedule_now", "scheduler builtin", "schedule_now(handler)", "Runs a scheduler handler immediately through the scheduler execution path.", "Handler result", "schedule_now(function() { return \"ran\"; });"},
	"schedule_run":      {"schedule_run", "scheduler builtin", "schedule_run([limit])", "Manually runs due scheduled jobs, optionally limiting how many jobs execute.", "Number of jobs run", "let ran = schedule_run(10);"},
	"schedule_worker":   {"schedule_worker", "scheduler builtin", "schedule_worker([duration])", "Starts a scheduler worker loop for the requested duration or default runtime duration.", "Worker status/result", "schedule_worker(\"10s\");"},
	"schedule_timezone": {"schedule_timezone", "scheduler builtin", "schedule_timezone(name)", "Sets the scheduler timezone used to interpret cron expressions.", "Timezone name or status", "schedule_timezone(\"UTC\");"},
	"background":        {"background", "scheduler builtin", "background(handler)", "Runs a function asynchronously in a goroutine and resolves the result through a future.", "Future", "let future = background(function() { return 42; });"},
	"signal":            {"signal", "reactive builtin", "signal(initial_value) or signal(name, initial_value)", "Creates reactive state. Reading .value tracks dependencies for computed/effect; calling .set updates the value and notifies subscribers.", "Signal object", "let count = signal(\"count\", 0); count.set(count.value + 1);"},
	"setSignal":         {"setSignal", "reactive builtin", "setSignal(signal, value_or_updater)", "Updates a signal directly. The second argument can be a value or updater function receiving the previous value.", "Updated signal value", "setSignal(count, function(prev) { return prev + 1; });"},
	"computed":          {"computed", "reactive builtin", "computed(fn)", "Creates a cached derived value. Dependencies are tracked while fn runs and the computed value invalidates when dependencies change.", "Computed object", "let total = computed(function() { return price.value * qty.value; });"},
	"effect":            {"effect", "reactive builtin", "effect(fn)", "Runs a side-effect immediately and again whenever tracked signal/computed dependencies change.", "Effect object with dispose support", "let stop = effect(function() { print count.value; });"},
	"batch":             {"batch", "reactive builtin", "batch(fn)", "Runs updates as a batch so dependent effects/computeds observe the final state instead of intermediate changes.", "Callback result", "batch(function() { a.set(1); b.set(2); });"},
	"watch":             {"watch", "watcher builtin", "watch(path, handler) or watch(path, pattern, handler)", "Watches filesystem changes and calls the handler when matching files change.", "Watch id string", "let id = watch(\"./src\", \"*.spl\", function(event) { print event.path; });"},
	"unwatch":           {"unwatch", "watcher builtin", "unwatch(watch_id)", "Stops a filesystem watcher by id.", "Boolean", "unwatch(id);"},
	"hot_reload":        {"hot_reload", "watcher builtin", "hot_reload(path)", "Watches an SPL file and re-evaluates it on change.", "Watch id/status", "hot_reload(\"./app.spl\");"},
	"now":               {"now", "time builtin", "now()", "Returns the current Unix timestamp in seconds.", "Integer timestamp", "let ts = now();"},
	"time_now":          {"time_now", "time helper alias", "time_now()", "Scheduler examples use this as a current-time helper. In the core runtime, now() is the canonical builtin.", "Unix timestamp", "let ts = time_now();"},
	"time_ms":           {"time_ms", "time builtin", "time_ms()", "Returns the current Unix timestamp in milliseconds.", "Integer timestamp", "let ms = time_ms();"},
	"now_iso":           {"now_iso", "time builtin", "now_iso()", "Returns the current UTC time formatted as RFC3339.", "String", "print now_iso();"},
	"now_format":        {"now_format", "time builtin", "now_format(format)", "Formats the current UTC time. Supports SPL-friendly tokens like YYYY, MM, DD, HH, mm, ss.", "String", "print now_format(\"YYYY-MM-DD HH:mm:ss\");"},
	"format_time":       {"format_time", "time builtin", "format_time(unix_seconds, format)", "Formats a Unix timestamp in UTC using SPL-friendly format tokens.", "String", "print format_time(now(), \"HH:mm:ss\");"},
	"time_format":       {"time_format", "time helper alias", "time_format(unix_seconds, format)", "Scheduler examples use this as a formatting helper. In the core runtime, format_time(ts, format) is the canonical builtin.", "String", "print time_format(time_now(), \"15:04:05\");"},
	"server":            {"server", "web builtin", "server([opts])", "Creates an SPL web server instance used with route, middleware, static assets, template directories, and listen helpers.", "Server object", "let app = server();"},
	"route":             {"route", "web builtin", "route(server, method, path, handler) or route(group, method, path, handler)", "Registers an HTTP route. The handler receives request and response objects with params, query, headers, json/text/html/send helpers, and route metadata.", "Route metadata/status", "route(app, \"GET\", \"/users/:id\", function(req, res) { return res.json({ id: req.params.id }); });"},
	"middleware":        {"middleware", "web builtin", "middleware(server_or_group, handler)", "Registers middleware that can inspect or mutate request/response state before routes run.", "Server/group object or registration status", "middleware(app, function(req, res, next) { next(); });"},
	"static":            {"static", "web builtin", "static(server, url_prefix, directory)", "Serves static files from a filesystem directory under a URL prefix.", "Server object or registration status", "static(app, \"/assets\", \"./public\");"},
	"template_dir":      {"template_dir", "web builtin", "template_dir(server, directory)", "Configures the directory used by response render helpers and server-side template rendering.", "Server object or registration status", "template_dir(app, \"./templates\");"},
	"web_app":           {"web_app", "web builtin", "web_app([opts])", "Creates a convenience web application with server defaults suitable for route-oriented apps.", "Server object", "let app = web_app();"},
	"route_group":       {"route_group", "web builtin", "route_group(server, prefix[, middleware])", "Creates a grouped route scope with a shared path prefix and optional group middleware.", "Route group object", "let api = route_group(app, \"/api\");"},
	"listen":            {"listen", "web builtin", "listen(server, port_or_addr)", "Starts the HTTP server and blocks until it shuts down.", "Server status", "listen(app, 3000);"},
	"listen_async":      {"listen_async", "web builtin", "listen_async(server, port_or_addr)", "Starts the HTTP server in the background so the script can continue running.", "Server status/future", "let running = listen_async(app, 3000);"},
	"shutdown":          {"shutdown", "web builtin", "shutdown(server)", "Stops a running SPL web server and releases registered runtime cleanup resources.", "Boolean/status", "shutdown(app);"},
}

type StdModuleDoc struct {
	Name        string
	Category    string
	Purpose     string
	Exports     []string
	Example     string
	Recommended bool
}

var splStdModuleDocs = map[string]StdModuleDoc{
	"std/core":      {"std/core", "core", "Groups universal interpreter helpers for formatting, length/type inspection, interpolation, and builtin discovery.", []string{"help", "sprintf", "printf", "interpolate", "len", "type"}, "import \"std/core\" as core;\nprint core.len(items);", true},
	"core":          {"core", "core", "Short alias for std/core.", []string{"help", "sprintf", "printf", "interpolate", "len", "type"}, "import \"core\" as core;\nprint core.type(value);", false},
	"std/math":      {"std/math", "numeric computing", "Groups numeric, trigonometry, range-mapping, and statistics helpers for scripts that transform measurements, scores, prices, or other number-heavy data.", []string{"round_to", "median", "stddev", "percentile", "gcd", "lerp"}, "import \"std/math\" as math;\nlet p95 = math.percentile(latencies, 95);", true},
	"math":          {"math", "numeric computing", "Short alias for std/math. Useful in quick scripts; prefer std/math in shared code for clarity.", []string{"round_to", "median", "stddev", "percentile", "gcd", "lerp"}, "import \"math\" as math;\nprint math.gcd(18, 24);", false},
	"std/random":    {"std/random", "random data", "Collects pseudo-random numbers, deterministic seeding, sampling, shuffling, and secure random byte/string helpers.", []string{"random", "seed_random", "random_float", "random_choice", "shuffle", "sample"}, "import \"std/random\" as random;\nlet picked = random.random_choice(items);", true},
	"random":        {"random", "random data", "Short alias for std/random.", []string{"random_float", "random_choice", "shuffle", "sample"}, "import \"random\" as random;\nlet hand = random.sample(deck, 5);", false},
	"std/string":    {"std/string", "text processing", "Collects text cleanup, casing, regex, encoding, URL, HTML, and string predicate helpers.", []string{"trim", "slug", "words", "reverse_string", "is_blank", "url_encode"}, "import \"std/string\" as str;\nlet key = str.slug(title);", true},
	"string":        {"string", "text processing", "Short alias for std/string.", []string{"trim", "slug", "words", "reverse_string", "is_blank"}, "import \"string\" as str;\nprint str.reverse_string(\"abc\");", false},
	"std/array":     {"std/array", "array/data transforms", "Groups array selection, ordering, statistics, grouping-adjacent helpers, and random sampling for data-processing scripts.", []string{"unique", "sort_by", "pluck", "index_by", "take", "drop"}, "import \"std/array\" as array;\nlet names = array.pluck(users, \"name\");", true},
	"array":         {"array", "array/data transforms", "Short alias for std/array.", []string{"unique", "sort_by", "pluck", "take", "drop"}, "import \"array\" as array;\nlet top = array.take(scores, 10);", false},
	"std/hash":      {"std/hash", "hash/object transforms", "Groups object/hash key selection, lookup, conversion, grouping, and deep comparison helpers.", []string{"get", "pick", "omit", "entries", "from_entries", "deep_equal"}, "import \"std/hash\" as hash;\nlet public = hash.omit(user, [\"password\"]);", true},
	"hash":          {"hash", "hash/object transforms", "Short alias for std/hash.", []string{"get", "pick", "omit", "entries"}, "import \"hash\" as hash;\nprint hash.get(config, \"port\", 3000);", false},
	"std/time":      {"std/time", "time/date handling", "Groups Unix timestamp, ISO, timezone, calendar boundary, duration, and date-part helpers.", []string{"format_time_tz", "parse_duration", "start_of_month", "end_of_week", "weekday", "year"}, "import \"std/time\" as time;\nlet ms = time.parse_duration(\"1h30m\");", true},
	"time":          {"time", "time/date handling", "Short alias for std/time.", []string{"now", "format_time", "parse_duration", "year"}, "import \"time\" as time;\nprint time.year(time.now());", false},
	"std/json":      {"std/json", "JSON data", "Groups JSON file IO and text encode/decode helpers for API payloads and config-shaped data.", []string{"read_json", "write_json", "json_parse", "json_stringify"}, "import \"std/json\" as json;\nlet payload = json.json_parse(raw);", true},
	"json":          {"json", "JSON data", "Short alias for std/json.", []string{"json_parse", "json_stringify"}, "import \"json\" as json;\nprint json.json_stringify({ ok: true });", false},
	"std/csv":       {"std/csv", "CSV/table data", "Groups CSV file IO, CSV text encode/decode, and table row/column transformation helpers.", []string{"read_csv", "csv_decode", "table_rows", "table_select", "table_filter"}, "import \"std/csv\" as csv;\nlet rows = csv.table_rows(csv.read_csv(\"people.csv\"));", true},
	"csv":           {"csv", "CSV/table data", "Short alias for std/csv.", []string{"read_csv", "csv_decode", "table_rows"}, "import \"csv\" as csv;\nlet table = csv.csv_decode(raw);", false},
	"std/crypto":    {"std/crypto", "crypto/encoding", "Groups hashing, HMAC, password hashing, authenticated encryption, constant-time comparison, and byte encoders.", []string{"sha256", "hmac_sha256", "password_hash", "constant_time_eq", "base64_encode"}, "import \"std/crypto\" as crypto;\nlet digest = crypto.sha256(body);", true},
	"crypto":        {"crypto", "crypto/encoding", "Short alias for std/crypto.", []string{"sha256", "hmac_sha256", "base64_encode"}, "import \"crypto\" as crypto;\nprint crypto.sha256(\"hello\");", false},
	"std/path":      {"std/path", "path utilities", "Groups path joining, basename/dirname aliases, extension lookup, cleaning, and absolute path conversion.", []string{"path_join", "path_base", "path_dir", "path_ext", "path_clean", "path_abs"}, "import \"std/path\" as path;\nlet out = path.path_join(\"dist\", \"app.json\");", true},
	"path":          {"path", "path utilities", "Short alias for std/path.", []string{"path_join", "path_ext", "path_base"}, "import \"path\" as path;\nprint path.path_ext(\"app.spl\");", false},
	"tools/files":   {"tools/files", "daily file chores", "Groups preview-first file operations plus rich glob/regex file and directory search, chainable finders, bulk renaming, organizing, checksums, moving, copying, removal, and duplicate detection.", []string{"bulk_rename", "file_search", "file_finder", "file_organize", "file_checksum", "file_remove_plan"}, "import \"tools/files\" as files;\nlet matches = files.file_finder(\"testdata\").files().regex(\"^examples_.*\\\\.spl$\").content_regex(\"print\\\\s+\").limit(5).exec();", true},
	"tools/archive": {"tools/archive", "archives", "Groups archive listing, compression, and extraction helpers. Mutating operations preview by default.", []string{"archive_compress", "archive_extract", "archive_list"}, "import \"tools/archive\" as archive;\nlet plan = archive.archive_compress(\"docs\", \"backup.zip\", {\"format\": \"zip\", \"apply\": false});", true},
	"tools/images":  {"tools/images", "image chores", "Groups batch image conversion, optimization previews, file metadata, resize, thumbnail, and crop helpers for photographer and asset workflows.", []string{"image_convert_batch", "image_info_file", "image_resize_file", "image_thumbnail", "image_crop_file"}, "import \"tools/images\" as images;\nlet plan = images.image_convert_batch(\"photos\", \"web\", {\"to\": \"png\", \"apply\": false});", true},
	"tools/office":  {"tools/office", "office files", "Groups safe office-like text and structured extraction helpers for plain text, Markdown, CSV, JSON, DOCX, and XLSX files.", []string{"office_text", "office_read"}, "import \"tools/office\" as office;\nprint office.office_text(\"README.md\");", true},
	"tools/secrets": {"tools/secrets", "secret chores", "Groups secret generation, token generation, and preview-first file encryption/decryption helpers.", []string{"secret_generate", "token_generate", "file_encrypt", "file_decrypt"}, "import \"tools/secrets\" as secrets;\nlet token = secrets.token_generate(32);", true},
	"tools/media":   {"tools/media", "audio/video chores", "Groups ffmpeg-backed media probing, conversion, status, and install helpers. Conversion previews by default.", []string{"media_info", "media_convert", "ffmpeg_status", "ffmpeg_install"}, "import \"tools/media\" as media;\nprint media.ffmpeg_status();", true},
	"tools/system":  {"tools/system", "system inspection", "Groups safe host/runtime inspection helpers gated by the system capability.", []string{"system_info"}, "import \"tools/system\" as system;\nprint system.system_info();", true},
	"tools/network": {"tools/network", "network checks", "Groups DNS, TCP, and HTTP probe helpers gated by network policy.", []string{"dns_lookup", "tcp_check", "http_probe"}, "import \"tools/network\" as net;\nprint net.dns_lookup(\"example.com\");", true},
	"native/os":     {"native/os", "native OS adapter", "Exposes policy-gated direct command execution, command discovery, platform metadata, and capability inspection without defining every OS function in SPL.", []string{"run", "which", "list", "platform", "capabilities"}, "import \"native/os\" as os;\nlet result = os.run(\"go\", [\"version\"]);", true},
}

var builtinSignatureHints = map[string]string{
	"abs":                 "abs(n)",
	"acos":                "acos(n)",
	"all":                 "all(array)",
	"any":                 "any(array)",
	"asin":                "asin(n)",
	"atan":                "atan(n)",
	"atan2":               "atan2(y, x)",
	"avg":                 "avg(array)",
	"ceil":                "ceil(n)",
	"chunk":               "chunk(array, size)",
	"clamp":               "clamp(value, min, max)",
	"coalesce":            "coalesce(...values)",
	"compact":             "compact(array)",
	"contains":            "contains(string_or_array, needle)",
	"cos":                 "cos(n)",
	"cosh":                "cosh(n)",
	"count_substr":        "count_substr(s, substr)",
	"default":             "default(value, fallback)",
	"delete_key":          "delete_key(hash, key)",
	"ends_with":           "ends_with(s, suffix)",
	"find":                "find(array, value)",
	"first":               "first(array)",
	"flatten":             "flatten(array)",
	"floor":               "floor(n)",
	"get":                 "get(hash, key[, default])",
	"group_by":            "group_by(array, key)",
	"has_key":             "has_key(hash, key)",
	"hypot":               "hypot(a, b)",
	"index_of":            "index_of(s, substr)",
	"is_even":             "is_even(n)",
	"is_inf":              "is_inf(n)",
	"is_nan":              "is_nan(n)",
	"is_odd":              "is_odd(n)",
	"join":                "join(array, separator)",
	"keys":                "keys(hash)",
	"last":                "last(array)",
	"len":                 "len(value)",
	"log":                 "log(n)",
	"log10":               "log10(n)",
	"log2":                "log2(n)",
	"lower":               "lower(s)",
	"max":                 "max(...numbers)",
	"merge":               "merge(left, right)",
	"min":                 "min(...numbers)",
	"partition":           "partition(array, key, value)",
	"pow":                 "pow(base, exponent)",
	"push":                "push(array, value)",
	"random":              "random([max])",
	"random_range":        "random_range(min, max)",
	"range":               "range(end) or range(start, end[, step])",
	"reduce":              "reduce(array, operator)",
	"repeat":              "repeat(s, count)",
	"replace":             "replace(s, old, new)",
	"rest":                "rest(array)",
	"reverse":             "reverse(array)",
	"round":               "round(n)",
	"sin":                 "sin(n)",
	"sinh":                "sinh(n)",
	"slice":               "slice(array, start, end)",
	"sort":                "sort(array)",
	"split":               "split(s, separator)",
	"split_lines":         "split_lines(s)",
	"sqrt":                "sqrt(n)",
	"starts_with":         "starts_with(s, prefix)",
	"substring":           "substring(s, start, end)",
	"sum":                 "sum(array)",
	"tan":                 "tan(n)",
	"tanh":                "tanh(n)",
	"to_degrees":          "to_degrees(radians)",
	"to_radians":          "to_radians(degrees)",
	"trim":                "trim(s)",
	"trim_prefix":         "trim_prefix(s, prefix)",
	"trim_suffix":         "trim_suffix(s, suffix)",
	"uniq":                "uniq(array)",
	"upper":               "upper(s)",
	"values":              "values(hash)",
	"zip":                 "zip(left, right)",
	"bulk_rename":         "bulk_rename(dir[, opts])",
	"file_search":         "file_search(root[, opts])",
	"file_locate":         "file_locate(root[, opts])",
	"file_finder":         "file_finder(root)",
	"file_move_plan":      "file_move_plan(src, dst[, opts])",
	"file_copy_plan":      "file_copy_plan(src, dst[, opts])",
	"file_dedupe":         "file_dedupe(root[, opts])",
	"archive_compress":    "archive_compress(src, dst[, opts])",
	"archive_extract":     "archive_extract(src, dst[, opts])",
	"archive_list":        "archive_list(path)",
	"image_convert_batch": "image_convert_batch(src_dir, dst_dir[, opts])",
	"image_optimize":      "image_optimize(src_dir, dst_dir[, opts])",
	"image_crop_file":     "image_crop_file(src, dst[, opts])",
	"office_text":         "office_text(path)",
	"secret_generate":     "secret_generate([length][, alphabet])",
	"token_generate":      "token_generate([bytes])",
	"file_encrypt":        "file_encrypt(src, dst, passphrase[, opts])",
	"file_decrypt":        "file_decrypt(src, dst, passphrase[, opts])",
	"media_info":          "media_info(path)",
	"media_convert":       "media_convert(src, dst[, opts])",
	"ffmpeg_status":       "ffmpeg_status()",
	"ffmpeg_install":      "ffmpeg_install([opts])",
	"system_info":         "system_info()",
	"dns_lookup":          "dns_lookup(host)",
	"tcp_check":           "tcp_check(address[, timeout_ms])",
	"http_probe":          "http_probe(url[, timeout_ms])",
	"run":                 "run(command, args[, opts])",
	"which":               "which(command)",
	"list":                "list([opts])",
	"platform":            "platform()",
	"capabilities":        "capabilities()",
	"xql_run":             "xql_run(query)",
	"xql_connect":         "xql_connect(alias, type, config[, source])",
	"xql_list_integrations": "xql_list_integrations()",
}

var builtinPurposeHints = map[string]string{
	"abs":                 "Returns the absolute value of an integer or float.",
	"acos":                "Returns the arccosine of a number in radians.",
	"all":                 "Returns true when every element in an array is truthy. Empty arrays return true.",
	"any":                 "Returns true when at least one element in an array is truthy.",
	"asin":                "Returns the arcsine of a number in radians.",
	"atan":                "Returns the arctangent of a number in radians.",
	"atan2":               "Returns the arctangent of y/x using both coordinates to determine the quadrant.",
	"avg":                 "Returns the arithmetic average of numeric array values.",
	"ceil":                "Rounds a float up to the nearest integer.",
	"chunk":               "Splits an array into sub-arrays with at most size elements each.",
	"clamp":               "Constrains a numeric value to an inclusive minimum and maximum. Values below min become min; values above max become max.",
	"coalesce":            "Returns the first argument that is not null, or null when every argument is null.",
	"compact":             "Returns a copy of an array with null values removed.",
	"contains":            "For strings, reports whether the substring exists. For arrays, reports whether an equal inspected value exists.",
	"cos":                 "Returns the cosine of a number in radians.",
	"cosh":                "Returns the hyperbolic cosine of a number.",
	"count_substr":        "Counts non-overlapping occurrences of a substring in a string.",
	"default":             "Returns fallback when value is null; otherwise returns value unchanged.",
	"delete_key":          "Returns a copy of a hash without the requested key.",
	"ends_with":           "Reports whether a string ends with the given suffix.",
	"find":                "Returns the first array element equal to value, or null when no element matches.",
	"first":               "Returns the first array element, or null for an empty array.",
	"flatten":             "Flattens one level of nested arrays into a new array.",
	"floor":               "Rounds a float down to the nearest integer.",
	"get":                 "Looks up a hash key. When the key is missing, returns the optional default or null.",
	"group_by":            "Groups an array of hashes by the value stored at key, returning a hash of arrays.",
	"has_key":             "Reports whether a hash contains the requested key.",
	"hypot":               "Returns sqrt(a*a + b*b) without intermediate overflow for large float values.",
	"index_of":            "Returns the first byte index of substr in s, or -1 when missing.",
	"is_even":             "Reports whether an integer is divisible by 2.",
	"is_inf":              "Reports whether a numeric value is positive or negative infinity.",
	"is_nan":              "Reports whether a numeric value is NaN.",
	"is_odd":              "Reports whether an integer is not divisible by 2.",
	"join":                "Converts array elements to strings and joins them with separator.",
	"keys":                "Returns the keys of a hash as an array.",
	"last":                "Returns the last array element, or null for an empty array.",
	"len":                 "Returns the number of bytes in a string, elements in an array, or pairs in a hash.",
	"log":                 "Returns the natural logarithm of a number.",
	"log10":               "Returns the base-10 logarithm of a number.",
	"log2":                "Returns the base-2 logarithm of a number.",
	"lower":               "Returns a lower-case copy of a string.",
	"max":                 "Returns the largest numeric argument.",
	"merge":               "Returns a hash containing all pairs from left and right. Values from right replace duplicate keys.",
	"min":                 "Returns the smallest numeric argument.",
	"partition":           "Splits an array of hashes into matching and non-matching groups based on key == value.",
	"pow":                 "Raises base to exponent. Integer non-negative exponent cases preserve integer results when exact.",
	"push":                "Returns a copy of an array with value appended.",
	"random":              "Returns a pseudo-random integer from 0 up to max-1. Without max, uses a large integer range.",
	"random_range":        "Returns a pseudo-random integer from min up to max-1.",
	"range":               "Builds an integer sequence suitable for loops, generated test data, and array operations.",
	"reduce":              "Reduces an array with a named built-in operation such as sum or concat.",
	"repeat":              "Repeats a string count times.",
	"replace":             "Replaces every occurrence of old with new in a string.",
	"rest":                "Returns all array elements except the first.",
	"reverse":             "Returns a copy of an array with elements in reverse order.",
	"round":               "Rounds a number to the nearest integer.",
	"sin":                 "Returns the sine of a number in radians.",
	"sinh":                "Returns the hyperbolic sine of a number.",
	"slice":               "Returns a copy of array elements in the half-open range start..end.",
	"sort":                "Returns a sorted copy of a homogeneous integer or string array.",
	"split":               "Splits a string around every occurrence of separator.",
	"split_lines":         "Splits a string into lines, normalizing CRLF to LF first.",
	"sqrt":                "Returns the square root. Integer inputs preserve the legacy truncated integer result; float inputs return float precision.",
	"starts_with":         "Reports whether a string starts with the given prefix.",
	"substring":           "Returns the rune-based substring in the half-open range start..end.",
	"sum":                 "Adds numeric array values.",
	"tan":                 "Returns the tangent of a number in radians.",
	"tanh":                "Returns the hyperbolic tangent of a number.",
	"to_degrees":          "Converts radians to degrees.",
	"to_radians":          "Converts degrees to radians.",
	"trim":                "Removes leading and trailing Unicode whitespace.",
	"trim_prefix":         "Removes prefix from the start of a string when present.",
	"trim_suffix":         "Removes suffix from the end of a string when present.",
	"uniq":                "Returns the first occurrence of each distinct array value.",
	"upper":               "Returns an upper-case copy of a string.",
	"values":              "Returns the values of a hash as an array.",
	"zip":                 "Pairs elements from two arrays up to the shorter length.",
	"bulk_rename":         "Builds a preview or apply plan for renaming files in a directory using a glob match and template placeholders.",
	"file_search":         "Searches files or directories by glob, literal, or regex patterns plus name, path, extension, content, size, modified time, sort, and limit filters within the allowed filesystem root.",
	"file_locate":         "Alias for file_search when a script reads more naturally as a locate operation.",
	"file_finder":         "Creates a chainable filesystem finder with files, dirs, match, pattern_type, regex, name, path_regex, content_regex, size, modified, recursive, max_depth, sort, limit, and exec methods.",
	"file_move_plan":      "Builds a preview or apply plan for moving one file to another path.",
	"file_copy_plan":      "Builds a preview or apply plan for copying one file to another path.",
	"file_dedupe":         "Scans files and reports duplicate-content candidates without deleting anything.",
	"archive_compress":    "Builds a preview or apply plan for creating a zip, tar, or gzip archive.",
	"archive_extract":     "Builds a preview or apply plan for extracting a supported archive safely.",
	"archive_list":        "Lists entries in a supported archive without extracting it.",
	"image_convert_batch": "Builds a preview or apply plan for converting matching image files into an output directory.",
	"image_optimize":      "Re-encodes images through the batch conversion path so users can preview optimized outputs.",
	"image_crop_file":     "Builds a preview or apply plan for cropping a single image file.",
	"office_text":         "Extracts text from supported office-like/plain data files.",
	"secret_generate":     "Generates a masked secret value suitable for passwords or one-off credentials.",
	"token_generate":      "Generates a masked URL-safe random token.",
	"file_encrypt":        "Builds a preview or apply plan for AES-GCM file encryption.",
	"file_decrypt":        "Builds a preview or apply plan for AES-GCM file decryption.",
	"media_info":          "Returns media metadata using ffprobe when available, with a Go-native fallback for basic image metadata.",
	"media_convert":       "Builds a preview or apply plan for converting audio/video with ffmpeg, optionally installing ffmpeg first.",
	"ffmpeg_status":       "Reports ffmpeg and ffprobe availability plus the detected installer command.",
	"ffmpeg_install":      "Builds a preview or apply plan for installing ffmpeg with the detected OS package manager.",
	"system_info":         "Returns safe host/runtime metadata when system capability is allowed.",
	"dns_lookup":          "Resolves host addresses under the active network policy.",
	"tcp_check":           "Checks TCP connectivity to an address under the active network policy.",
	"http_probe":          "Sends an HTTP HEAD probe under the active network policy.",
	"run":                 "Runs an allowed executable directly and returns structured stdout, stderr, status, timeout, and truncation details.",
	"which":               "Resolves an allowed executable from PATH.",
	"list":                "Lists executable names from PATH after applying exec policy.",
	"platform":            "Returns host OS, architecture, cwd, separators, and shell hints.",
	"capabilities":        "Reports currently available native OS, exec, filesystem, network, system, and environment capabilities.",
	"xql_run":             "Executes an XQL query against integration-bound data sources and returns (result, err).",
	"xql_connect":         "Connects an integration alias with the given type and configuration hash. When a 4th source argument is provided, also binds the source name for XQL queries.",
	"xql_list_integrations": "Returns the names of all bound integration sources.",
}

var builtinReturnHints = map[string]string{
	"abs":                 "Integer or float",
	"acos":                "Float",
	"all":                 "Boolean",
	"any":                 "Boolean",
	"asin":                "Float",
	"atan":                "Float",
	"atan2":               "Float",
	"avg":                 "Float",
	"ceil":                "Integer",
	"chunk":               "Array",
	"clamp":               "Integer",
	"coalesce":            "First non-null value, or null",
	"compact":             "Array",
	"contains":            "Boolean",
	"cos":                 "Float",
	"cosh":                "Float",
	"count_substr":        "Integer",
	"default":             "Original value or fallback",
	"delete_key":          "Hash",
	"ends_with":           "Boolean",
	"find":                "Matching value or null",
	"first":               "First value or null",
	"flatten":             "Array",
	"floor":               "Integer",
	"get":                 "Value, default, or null",
	"group_by":            "Hash of arrays",
	"has_key":             "Boolean",
	"hypot":               "Float",
	"index_of":            "Integer",
	"is_even":             "Boolean",
	"is_inf":              "Boolean",
	"is_nan":              "Boolean",
	"is_odd":              "Boolean",
	"join":                "String",
	"keys":                "Array",
	"last":                "Last value or null",
	"len":                 "Integer",
	"log":                 "Float",
	"log10":               "Float",
	"log2":                "Float",
	"lower":               "String",
	"max":                 "Integer or float",
	"merge":               "Hash",
	"min":                 "Integer or float",
	"partition":           "Array containing [matching, rest]",
	"pow":                 "Integer or float",
	"push":                "Array",
	"random":              "Integer",
	"random_range":        "Integer",
	"range":               "Array of integers",
	"reduce":              "Reduced value",
	"repeat":              "String",
	"replace":             "String",
	"rest":                "Array",
	"reverse":             "Array",
	"round":               "Integer",
	"sin":                 "Float",
	"sinh":                "Float",
	"slice":               "Array",
	"sort":                "Array",
	"split":               "Array of strings",
	"split_lines":         "Array of strings",
	"sqrt":                "Integer or float",
	"starts_with":         "Boolean",
	"substring":           "String",
	"sum":                 "Integer or float",
	"tan":                 "Float",
	"tanh":                "Float",
	"to_degrees":          "Float",
	"to_radians":          "Float",
	"trim":                "String",
	"trim_prefix":         "String",
	"trim_suffix":         "String",
	"uniq":                "Array",
	"upper":               "String",
	"values":              "Array",
	"zip":                 "Array of pairs",
	"bulk_rename":         "Array of operation plans",
	"file_search":         "Array of file metadata hashes",
	"file_finder":         "Chainable file finder object",
	"file_locate":         "Array of file metadata hashes",
	"file_move_plan":      "Operation plan hash",
	"file_copy_plan":      "Operation plan hash",
	"file_dedupe":         "Array of operation plans",
	"archive_compress":    "Operation plan hash",
	"archive_extract":     "Operation plan hash",
	"archive_list":        "Array of archive entry hashes",
	"image_convert_batch": "Array of operation plans",
	"image_optimize":      "Array of operation plans",
	"image_crop_file":     "Operation plan hash",
	"office_text":         "String",
	"secret_generate":     "SECRET",
	"token_generate":      "SECRET",
	"file_encrypt":        "Operation plan hash",
	"file_decrypt":        "Operation plan hash",
	"media_info":          "Hash",
	"media_convert":       "Operation plan hash",
	"ffmpeg_status":       "Hash",
	"ffmpeg_install":      "Operation plan hash",
	"system_info":         "Hash",
	"dns_lookup":          "Array of strings",
	"tcp_check":           "Hash",
	"http_probe":          "Hash",
	"run":                 "Hash",
	"which":               "String or null",
	"list":                "Array of strings",
	"platform":            "Hash",
	"capabilities":        "Hash",
	"xql_run":             "Tuple of (result, error)",
	"xql_connect":         "Boolean",
	"xql_list_integrations": "Array of strings",
}

var builtinExampleHints = map[string]string{
	"clamp":               "let capped = clamp(score, 0, 100);",
	"len":                 "print len([\"a\", \"b\", \"c\"]);",
	"bulk_rename":         "let plan = bulk_rename(\"testdata\", {\"match\": \"*.spl\", \"template\": \"{date}_{seq}.{ext}\", \"apply\": false});",
	"file_finder":         "let matches = file_finder(\"testdata\").files().regex(\"^examples_.*\\\\.spl$\").content_regex(\"print\\\\s+\").limit(5).exec();",
	"archive_compress":    "let plan = archive_compress(\"docs\", \"backup.zip\", {\"format\": \"zip\", \"apply\": false});",
	"image_convert_batch": "let plan = image_convert_batch(\"photos\", \"web\", {\"to\": \"png\", \"apply\": false});",
	"secret_generate":     "let password = secret_generate(32);",
	"media_convert":       "let plan = media_convert(\"input.mov\", \"output.mp4\", {\"codec\": \"libx264\", \"apply\": false});",
	"ffmpeg_install":      "let install = ffmpeg_install({\"apply\": false});",
	"ffmpeg_status":       "print ffmpeg_status();",
}

func init() {
	for name, doc := range enhancementRuntimeDocs {
		splRuntimeDocs[name] = doc
	}
}

var enhancementRuntimeDocs = map[string]RuntimeDoc{
	"cbrt":            {"cbrt", "math builtin", "cbrt(n)", "Returns the cube root of a number.", "Float", "print cbrt(27);"},
	"mod":             {"mod", "math builtin", "mod(a, b)", "Returns numeric remainder. Integer inputs return an integer; float inputs use floating-point modulo.", "Integer or float", "print mod(10, 3);"},
	"sign":            {"sign", "math builtin", "sign(n)", "Classifies a number as negative, zero, or positive.", "-1, 0, or 1", "print sign(-42);"},
	"trunc":           {"trunc", "math builtin", "trunc(n)", "Removes the fractional part of a number without rounding.", "Integer or float", "print trunc(4.9);"},
	"round_to":        {"round_to", "math builtin", "round_to(n, decimals)", "Rounds a number to a fixed number of decimal places.", "Float", "print round_to(3.14159, 2);"},
	"lerp":            {"lerp", "math builtin", "lerp(a, b, t)", "Interpolates between a and b where t is usually between 0 and 1.", "Float", "print lerp(10, 20, 0.25);"},
	"normalize":       {"normalize", "math builtin", "normalize(value, min, max)", "Maps a value inside a range to a 0..1 position.", "Float", "print normalize(75, 50, 100);"},
	"map_range":       {"map_range", "math builtin", "map_range(value, in_min, in_max, out_min, out_max)", "Maps a value from one numeric range into another.", "Float", "print map_range(score, 0, 100, 1, 5);"},
	"percent":         {"percent", "math builtin", "percent(value, total)", "Returns value as a percentage of total.", "Float", "print percent(25, 200);"},
	"factorial":       {"factorial", "math builtin", "factorial(n)", "Returns n! for non-negative integers.", "Integer", "print factorial(5);"},
	"gcd":             {"gcd", "math builtin", "gcd(a, b)", "Returns the greatest common divisor of two integers.", "Integer", "print gcd(18, 24);"},
	"lcm":             {"lcm", "math builtin", "lcm(a, b)", "Returns the least common multiple of two integers.", "Integer", "print lcm(6, 8);"},
	"is_finite":       {"is_finite", "math predicate", "is_finite(n)", "Reports whether a number is neither NaN nor infinite.", "Boolean", "if is_finite(value) { print value; }"},
	"is_integer":      {"is_integer", "math predicate", "is_integer(n)", "Reports whether a numeric value has no fractional part.", "Boolean", "print is_integer(10.0);"},
	"is_prime":        {"is_prime", "math predicate", "is_prime(n)", "Reports whether an integer is prime.", "Boolean", "print is_prime(17);"},
	"random_float":    {"random_float", "random builtin", "random_float()", "Returns a pseudo-random float in the half-open range [0, 1).", "Float", "print random_float();"},
	"random_choice":   {"random_choice", "random builtin", "random_choice(array)", "Returns one random element from an array, or null for an empty array.", "Any value", "let winner = random_choice(users);"},
	"shuffle":         {"shuffle", "random builtin", "shuffle(array)", "Returns a shuffled copy of an array without mutating the input.", "Array", "let shuffled = shuffle(deck);"},
	"sample":          {"sample", "random builtin", "sample(array, count)", "Returns up to count random elements from an array without replacement.", "Array", "let hand = sample(deck, 5);"},
	"mean":            {"mean", "statistics builtin", "mean(array)", "Returns the arithmetic mean of numeric array values. Alias-style companion to avg.", "Float", "print mean([2, 4, 6]);"},
	"median":          {"median", "statistics builtin", "median(array)", "Returns the middle numeric value after sorting, or the average of the two middle values.", "Number or null", "print median([9, 1, 5]);"},
	"mode":            {"mode", "statistics builtin", "mode(array)", "Returns the most frequently occurring value in an array.", "Any value or null", "print mode([\"a\", \"b\", \"a\"]);"},
	"variance":        {"variance", "statistics builtin", "variance(array)", "Returns population variance for numeric array values.", "Float", "print variance(scores);"},
	"stddev":          {"stddev", "statistics builtin", "stddev(array)", "Returns population standard deviation for numeric array values.", "Float", "print round_to(stddev(scores), 2);"},
	"percentile":      {"percentile", "statistics builtin", "percentile(array, p)", "Returns the pth percentile for numeric array values, where p is 0..100.", "Float or null", "let p95 = percentile(latencies, 95);"},
	"unique":          {"unique", "array builtin", "unique(array)", "Returns the first occurrence of each distinct value. This is the clear-name alias of uniq.", "Array", "print unique([1, 1, 2]);"},
	"sort_by":         {"sort_by", "array builtin", "sort_by(array, key)", "Sorts an array of hashes by the inspected value at key.", "Array", "let ordered = sort_by(users, \"age\");"},
	"take":            {"take", "array builtin", "take(array, n)", "Returns the first n elements of an array.", "Array", "let top = take(scores, 10);"},
	"drop":            {"drop", "array builtin", "drop(array, n)", "Skips the first n elements of an array.", "Array", "let rest = drop(items, 1);"},
	"pluck":           {"pluck", "array builtin", "pluck(array, key)", "Extracts key from every hash in an array, returning null where the key is absent.", "Array", "let names = pluck(users, \"name\");"},
	"index_by":        {"index_by", "array/hash builtin", "index_by(array, key)", "Builds a hash where each hash element is indexed by its key value.", "Hash", "let by_id = index_by(users, \"id\");"},
	"pick":            {"pick", "hash builtin", "pick(hash, keys)", "Returns a hash containing only the requested keys.", "Hash", "let public = pick(user, [\"id\", \"name\"]);"},
	"omit":            {"omit", "hash builtin", "omit(hash, keys)", "Returns a hash without the requested keys.", "Hash", "let public = omit(user, [\"password\"]);"},
	"entries":         {"entries", "hash builtin", "entries(hash)", "Converts a hash to an array of [key, value] pairs.", "Array", "print entries({ a: 1 });"},
	"from_entries":    {"from_entries", "hash builtin", "from_entries(entries)", "Converts [key, value] pairs back into a hash.", "Hash", "print from_entries([[\"a\", 1]]);"},
	"deep_equal":      {"deep_equal", "comparison builtin", "deep_equal(a, b)", "Compares scalars, arrays, and hashes recursively. Numeric int/float values compare by numeric value.", "Boolean", "assert_true(deep_equal({ a: [1] }, { a: [1.0] }));"},
	"last_index_of":   {"last_index_of", "string builtin", "last_index_of(s, substr)", "Returns the last byte index of substr in s, or -1 when missing.", "Integer", "print last_index_of(\"banana\", \"na\");"},
	"replace_n":       {"replace_n", "string builtin", "replace_n(s, old, new, n)", "Replaces at most n occurrences of old with new.", "String", "print replace_n(\"a-a-a\", \"-\", \"_\", 1);"},
	"trim_chars":      {"trim_chars", "string builtin", "trim_chars(s, chars)", "Trims all leading and trailing runes contained in chars.", "String", "print trim_chars(\"..spl..\", \".\");"},
	"words":           {"words", "string builtin", "words(s)", "Splits text into normalized lower-case words using punctuation and case boundaries.", "Array of strings", "print words(\"helloWorld SPL\");"},
	"chars":           {"chars", "string builtin", "chars(s)", "Splits a string into Unicode character strings.", "Array of strings", "print chars(\"spl\");"},
	"reverse_string":  {"reverse_string", "string builtin", "reverse_string(s)", "Returns a string with Unicode characters in reverse order.", "String", "print reverse_string(\"abc\");"},
	"is_blank":        {"is_blank", "string predicate", "is_blank(s)", "Reports whether a string is empty or only whitespace.", "Boolean", "print is_blank(\"  \");"},
	"is_numeric":      {"is_numeric", "string predicate", "is_numeric(s)", "Reports whether a string parses as a number.", "Boolean", "print is_numeric(\"42.5\");"},
	"is_alpha":        {"is_alpha", "string predicate", "is_alpha(s)", "Reports whether a string contains only letters and is not empty.", "Boolean", "print is_alpha(\"SPL\");"},
	"is_alnum":        {"is_alnum", "string predicate", "is_alnum(s)", "Reports whether a string contains only letters/digits and is not empty.", "Boolean", "print is_alnum(\"SPL3\");"},
	"escape_html":     {"escape_html", "string builtin", "escape_html(s)", "Escapes special HTML characters for safe text output.", "String", "print escape_html(\"<b>\");"},
	"unescape_html":   {"unescape_html", "string builtin", "unescape_html(s)", "Decodes HTML entities back to text.", "String", "print unescape_html(\"&lt;b&gt;\");"},
	"json_parse":      {"json_parse", "JSON builtin", "json_parse(text)", "Parses JSON text into SPL arrays, hashes, strings, numbers, booleans, and null.", "SPL value", "let data = json_parse(raw);"},
	"json_stringify":  {"json_stringify", "JSON builtin", "json_stringify(value[, opts])", "Serializes an SPL value to JSON text. opts can include pretty and indent.", "String", "print json_stringify({ ok: true });"},
	"path_base":       {"path_base", "path builtin", "path_base(path)", "Returns the final path element.", "String", "print path_base(\"dir/file.spl\");"},
	"path_dir":        {"path_dir", "path builtin", "path_dir(path)", "Returns the parent directory portion of a path.", "String", "print path_dir(\"dir/file.spl\");"},
	"path_ext":        {"path_ext", "path builtin", "path_ext(path)", "Returns the file extension, including the leading dot.", "String", "print path_ext(\"file.spl\");"},
	"path_clean":      {"path_clean", "path builtin", "path_clean(path)", "Normalizes redundant separators and dot segments in a path.", "String", "print path_clean(\"a/../b\");"},
	"path_abs":        {"path_abs", "path builtin", "path_abs(path)", "Converts a path to an absolute path according to the host process working directory.", "String or error", "print path_abs(\".\");"},
	"parse_duration":  {"parse_duration", "time builtin", "parse_duration(text)", "Parses Go-style duration text such as 1h30m or 250ms into milliseconds.", "Integer milliseconds", "let timeout = parse_duration(\"1h30m\");"},
	"format_duration": {"format_duration", "time builtin", "format_duration(milliseconds)", "Formats a millisecond duration as compact duration text.", "String", "print format_duration(5400000);"},
	"start_of_month":  {"start_of_month", "time builtin", "start_of_month(unix_seconds)", "Returns the UTC Unix timestamp for the first second of that month.", "Integer timestamp", "print start_of_month(now());"},
	"end_of_week":     {"end_of_week", "time builtin", "end_of_week(unix_seconds)", "Returns the UTC Unix timestamp for the last second of the ISO-style week.", "Integer timestamp", "print end_of_week(now());"},
	"is_weekend":      {"is_weekend", "time predicate", "is_weekend(unix_seconds)", "Reports whether a UTC timestamp falls on Saturday or Sunday.", "Boolean", "print is_weekend(now());"},
	"weekday":         {"weekday", "time builtin", "weekday(unix_seconds)", "Returns UTC weekday number where Sunday is 0.", "Integer", "print weekday(now());"},
	"month":           {"month", "time builtin", "month(unix_seconds)", "Returns UTC month number, 1 through 12.", "Integer", "print month(now());"},
	"year":            {"year", "time builtin", "year(unix_seconds)", "Returns UTC year.", "Integer", "print year(now());"},
}

type Location struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Name   string `json:"name,omitempty"`
}

type Reference struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Name   string `json:"name"`
}

type IndexedDocument struct {
	Path    string
	Source  string
	Symbols []Symbol
}

type WorkspaceIndex struct {
	Root      string
	Documents map[string]IndexedDocument
}

type EvaluationOptions struct {
	Profile               string   `json:"profile,omitempty"`
	TimeoutMS             int64    `json:"timeoutMs,omitempty"`
	MaxOutputBytes        int64    `json:"maxOutputBytes,omitempty"`
	MaxExecOutputBytes    int64    `json:"maxExecOutputBytes,omitempty"`
	MaxSteps              int64    `json:"maxSteps,omitempty"`
	MaxDepth              int      `json:"maxDepth,omitempty"`
	AllowedCapabilities   []string `json:"allowedCapabilities,omitempty"`
	AllowedExecCommands   []string `json:"allowedExecCommands,omitempty"`
	AllowedNativeModules  []string `json:"allowedNativeModules,omitempty"`
	DeniedNativeModules   []string `json:"deniedNativeModules,omitempty"`
	AllowedFileReadPaths  []string `json:"allowedFileReadPaths,omitempty"`
	AllowedFileWritePaths []string `json:"allowedFileWritePaths,omitempty"`
}

type EvaluationResult struct {
	OK          bool             `json:"ok"`
	Path        string           `json:"path,omitempty"`
	Result      string           `json:"result,omitempty"`
	Output      string           `json:"output,omitempty"`
	Error       string           `json:"error,omitempty"`
	Duration    int64            `json:"durationMs"`
	Diagnostics []string         `json:"diagnostics,omitempty"`
	Metrics     map[string]any   `json:"metrics,omitempty"`
	Artifacts   []map[string]any `json:"artifacts,omitempty"`
	Events      []map[string]any `json:"events,omitempty"`
}

func NewWorkspaceIndex(root string) *WorkspaceIndex {
	abs := root
	if strings.TrimSpace(abs) == "" {
		if wd, err := os.Getwd(); err == nil {
			abs = wd
		}
	}
	if real, err := filepath.Abs(abs); err == nil {
		abs = real
	}
	idx := &WorkspaceIndex{Root: abs, Documents: map[string]IndexedDocument{}}
	idx.Refresh()
	return idx
}

func (idx *WorkspaceIndex) Refresh() {
	if idx == nil || idx.Root == "" {
		return
	}
	_ = filepath.WalkDir(idx.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "dist", "out", ".vscode-test":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".spl") {
			if data, err := os.ReadFile(path); err == nil {
				idx.Update(path, string(data))
			}
		}
		return nil
	})
}

func (idx *WorkspaceIndex) Update(path, src string) {
	if idx == nil || strings.TrimSpace(path) == "" {
		return
	}
	path = cleanAbsPath(path)
	idx.Documents[path] = IndexedDocument{Path: path, Source: src, Symbols: SymbolsForSource(path, src)}
}

func (idx *WorkspaceIndex) Remove(path string) {
	if idx == nil {
		return
	}
	delete(idx.Documents, cleanAbsPath(path))
}

func (idx *WorkspaceIndex) AllSymbols() []Symbol {
	if idx == nil {
		return nil
	}
	out := []Symbol{}
	for _, doc := range idx.Documents {
		out = append(out, doc.Symbols...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].Path < out[j].Path
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (idx *WorkspaceIndex) Definition(path, src string, line, col int) (Location, bool) {
	if loc, ok := importLocationAt(path, src, line, col); ok {
		return loc, true
	}
	word := wordAt(src, line, col)
	if word == "" {
		return Location{}, false
	}
	for _, sym := range VisibleSymbolsForSource(path, src) {
		if sym.Name == word {
			return Location{Path: sym.Path, Line: sym.Line, Column: sym.Column, Name: sym.Name}, true
		}
	}
	if idx != nil {
		for _, sym := range idx.AllSymbols() {
			if sym.Name == word {
				return Location{Path: sym.Path, Line: sym.Line, Column: sym.Column, Name: sym.Name}, true
			}
		}
	}
	return Location{}, false
}

func (idx *WorkspaceIndex) References(path, src string, line, col int) []Reference {
	word := wordAt(src, line, col)
	if word == "" {
		return nil
	}
	docs := map[string]string{cleanAbsPath(path): src}
	if idx != nil {
		for p, doc := range idx.Documents {
			docs[p] = doc.Source
		}
	}
	out := []Reference{}
	for p, text := range docs {
		lines := strings.Split(text, "\n")
		for i, ln := range lines {
			for _, m := range splIdentifierRe.FindAllStringIndex(ln, -1) {
				if ln[m[0]:m[1]] == word {
					out = append(out, Reference{Path: p, Line: i + 1, Column: runeColumn(ln, m[0]) + 1, Name: word})
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path == out[j].Path {
			if out[i].Line == out[j].Line {
				return out[i].Column < out[j].Column
			}
			return out[i].Line < out[j].Line
		}
		return out[i].Path < out[j].Path
	})
	return out
}

func WorkspaceCompletionItems(idx *WorkspaceIndex, path, src, prefix string) []CompletionItem {
	seen := map[string]struct{}{}
	items := []CompletionItem{}
	add := func(item CompletionItem) {
		if item.Label == "" || !strings.HasPrefix(item.Label, prefix) {
			return
		}
		key := item.Label + "\x00" + item.Kind
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		items = append(items, item)
	}
	for _, kw := range splKeywords {
		add(CompletionItem{Label: kw, Kind: "keyword", Detail: KeywordMarkdown(kw)})
	}
	for _, doc := range splRuntimeDocs {
		add(CompletionItem{Label: doc.Name, Kind: "builtin", Detail: RuntimeDocMarkdown(doc.Name)})
	}
	for _, item := range CompletionItems(path, src, prefix) {
		if md := RuntimeDocMarkdown(item.Label); md != "" {
			item.Detail = md
		}
		add(item)
	}
	if idx != nil {
		for _, sym := range idx.AllSymbols() {
			add(CompletionItem{Label: sym.Name, Kind: sym.Kind, Detail: sym.Detail})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
	return items
}

func KeywordMarkdown(name string) string {
	doc, ok := splKeywordDocs[name]
	if !ok {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "**keyword** `%s`\n\n", doc.Name)
	fmt.Fprintf(&b, "Category: %s\n\n", doc.Category)
	fmt.Fprintf(&b, "Syntax: `%s`\n\n", doc.Syntax)
	b.WriteString(doc.Interpretation)
	if doc.Example != "" {
		fmt.Fprintf(&b, "\n\nExample:\n```spl\n%s\n```", doc.Example)
	}
	return b.String()
}

func HoverMarkdown(idx *WorkspaceIndex, path, src string, line, col int) string {
	if target, ok := importTargetAt(src, line, col); ok {
		if md := StdModuleMarkdown(target); md != "" {
			return md
		}
	}
	if loc, ok := importLocationAt(path, src, line, col); ok {
		return fmt.Sprintf("**import** `%s`\n\nResolves to `%s`.\n\nGo to definition opens the target module.", loc.Name, loc.Path)
	}
	word := wordAt(src, line, col)
	if word == "" {
		return blockHoverMarkdown(src, line, col)
	}
	if md := KeywordMarkdown(word); md != "" {
		return withSemanticHoverContext(md, path, src, line, col, word)
	}
	if md := RuntimeDocMarkdown(word); md != "" {
		return withUsageContext(md, src, line, word)
	}
	if info := HoverAt(path, src, line, col); info.Name != "" && info.Kind == "builtin" {
		return withUsageContext(fmt.Sprintf("**builtin** `%s`\n\n%s", info.Name, info.Detail), src, line, word)
	}
	if sym, ok := bestSymbolForWord(idx, path, src, word); ok {
		refs := 0
		if idx != nil {
			refs = len(idx.References(path, src, line, col))
		}
		var b strings.Builder
		fmt.Fprintf(&b, "**%s** `%s`\n\n", sym.Kind, sym.Name)
		if sym.Detail != "" {
			fmt.Fprintf(&b, "%s\n\n", sym.Detail)
		}
		if sym.Path != "" && sym.Line > 0 {
			fmt.Fprintf(&b, "Declared at `%s:%d:%d`.\n\n", sym.Path, sym.Line, sym.Column)
		}
		if refs > 0 {
			fmt.Fprintf(&b, "Workspace references: %d.\n\n", refs)
		}
		if value, ok := inferHoverValues(path, src)[word]; ok && value.Known {
			fmt.Fprintf(&b, "Current inferred value: `%s`.\n\n", value.Repr)
		}
		if snippet := sourceLine(src, line); snippet != "" {
			fmt.Fprintf(&b, "Current context:\n```spl\n%s\n```", snippet)
		}
		return strings.TrimSpace(b.String())
	}
	if value, ok := inferHoverValues(path, src)[word]; ok && value.Known {
		var b strings.Builder
		fmt.Fprintf(&b, "**identifier** `%s`\n\nCurrent inferred value: `%s`.", word, value.Repr)
		if snippet := sourceLine(src, line); snippet != "" {
			fmt.Fprintf(&b, "\n\nCurrent context:\n```spl\n%s\n```", snippet)
		}
		return b.String()
	}
	if decl, ok := lexicalDeclarationContext(src, word, line, col); ok {
		return decl
	}
	if call := callContextMarkdown(src, line, col, word); call != "" {
		return call
	}
	refs := 0
	if idx != nil {
		refs = len(idx.References(path, src, line, col))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "**identifier** `%s`\n\n", word)
	b.WriteString("No declaration was found in the current file or workspace index.")
	if refs > 0 {
		fmt.Fprintf(&b, "\n\nWorkspace occurrences: %d.", refs)
	}
	if snippet := sourceLine(src, line); snippet != "" {
		fmt.Fprintf(&b, "\n\nCurrent context:\n```spl\n%s\n```", snippet)
	}
	return b.String()
}

func RuntimeDocMarkdown(name string) string {
	doc, ok := splRuntimeDocs[name]
	if !ok {
		doc, ok = inferredRuntimeDoc(name)
		if !ok {
			return ""
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "**%s** `%s`\n\n", doc.Kind, doc.Name)
	fmt.Fprintf(&b, "Signature: `%s`\n\n", doc.Signature)
	b.WriteString(doc.Interpretation)
	if doc.Returns != "" {
		fmt.Fprintf(&b, "\n\nReturns: %s.", doc.Returns)
	}
	if doc.Example != "" {
		fmt.Fprintf(&b, "\n\nExample:\n```spl\n%s\n```", doc.Example)
	}
	return b.String()
}

func inferredRuntimeDoc(name string) (RuntimeDoc, bool) {
	modules := modulesForBuiltin(name)
	if !eval.HasBuiltin(name) {
		if builtinSignatureHints[name] == "" && len(modules) == 0 {
			return RuntimeDoc{}, false
		}
	}
	signature := builtinSignatureHints[name]
	helpText := ""
	if help, ok := eval.BuiltinHelpDescriptions[name]; ok {
		helpText = strings.TrimSpace(help)
	}
	if signature == "" {
		if parsedSignature, parsedPurpose := splitBuiltinHelp(name, helpText); parsedSignature != "" {
			signature = parsedSignature
			if builtinPurposeHints[name] == "" {
				helpText = parsedPurpose
			}
		} else {
			signature = name + "(...)"
		}
	}

	kind := "builtin"
	category := ""
	if len(modules) > 0 {
		category = moduleCategory(modules[0])
		if category != "" {
			kind = category + " builtin"
		}
	}

	purpose := builtinPurposeHints[name]
	if purpose == "" {
		if helpText != "" {
			purpose = helpText
		}
	}
	if purpose == "" {
		if len(modules) > 0 {
			purpose = fmt.Sprintf("Registered SPL builtin exported by `%s`.", modules[0])
			if category != "" {
				purpose += " It belongs to the " + category + " helper set."
			}
		} else {
			purpose = "Registered SPL builtin provided by the interpreter runtime. It is available globally in scripts."
		}
	}
	if len(modules) > 0 {
		purpose += fmt.Sprintf("\n\nAvailable via: `%s`.", strings.Join(modules, "`, `"))
	}
	returns := builtinReturnHints[name]
	if returns == "" {
		returns = "SPL value"
	}
	return RuntimeDoc{
		Name:           name,
		Kind:           kind,
		Signature:      signature,
		Interpretation: purpose,
		Returns:        returns,
		Example:        inferredExample(name, modules),
	}, true
}

func splitBuiltinHelp(name, help string) (signature, purpose string) {
	help = strings.TrimSpace(help)
	if help == "" {
		return "", ""
	}
	idx := strings.Index(help, ")")
	if idx <= 0 {
		return "", help
	}
	first := help[:idx+1]
	if !strings.HasPrefix(first, name+"(") {
		return "", help
	}
	return first, strings.TrimSpace(help[idx+1:])
}

func modulesForBuiltin(name string) []string {
	modules := []string{}
	seen := map[string]struct{}{}
	for moduleName, exports := range knownStdModuleExports {
		for _, export := range exports {
			if export == name {
				if _, ok := seen[moduleName]; !ok {
					seen[moduleName] = struct{}{}
					modules = append(modules, moduleName)
				}
				break
			}
		}
	}
	sort.Slice(modules, func(i, j int) bool {
		ri := moduleDocRank(modules[i])
		rj := moduleDocRank(modules[j])
		if ri != rj {
			return ri < rj
		}
		return modules[i] < modules[j]
	})
	return modules
}

func moduleDocRank(moduleName string) int {
	switch moduleName {
	case "std/core":
		return 0
	case "std/math", "std/string", "std/array", "std/hash", "std/time", "std/json", "std/csv", "std/crypto", "std/path", "std/random":
		return 1
	case "tools/files", "tools/archive", "tools/images", "tools/office", "tools/secrets", "tools/media", "tools/system", "tools/network", "native/os":
		return 2
	default:
		return 3
	}
}

func moduleCategory(moduleName string) string {
	if doc, ok := splStdModuleDocs[moduleName]; ok {
		return doc.Category
	}
	return ""
}

func inferredExample(name string, modules []string) string {
	if example := builtinExampleHints[name]; example != "" {
		return example
	}
	if len(modules) > 0 {
		alias := strings.TrimPrefix(modules[0], "std/")
		if strings.Contains(alias, "/") {
			alias = filepath.Base(alias)
		}
		signature := builtinSignatureHints[name]
		call := name + "(...)"
		if signature != "" {
			call = signature
		}
		return fmt.Sprintf("import %q as %s;\nprint %s.%s;", modules[0], alias, alias, call)
	}
	return fmt.Sprintf("print %s(...);", name)
}

func StdModuleMarkdown(name string) string {
	doc, ok := splStdModuleDocs[name]
	if !ok {
		return ""
	}
	var b strings.Builder
	label := "standard module"
	if strings.HasPrefix(doc.Name, "tools/") {
		label = "tools module"
	}
	fmt.Fprintf(&b, "**%s** `%s`\n\n", label, doc.Name)
	fmt.Fprintf(&b, "Category: %s\n\n", doc.Category)
	b.WriteString(doc.Purpose)
	if doc.Recommended && !strings.HasPrefix(doc.Name, "tools/") {
		b.WriteString("\n\nUse this namespaced import in shared scripts so readers can see where helper functions come from.")
	} else if strings.HasPrefix(doc.Name, "tools/") {
		b.WriteString("\n\nDaily tool operations are preview-first by default; pass `apply: true` only when you want to mutate files or install dependencies.")
	}
	if len(doc.Exports) > 0 {
		fmt.Fprintf(&b, "\n\nKey exports: `%s`.", strings.Join(doc.Exports, "`, `"))
	}
	if doc.Example != "" {
		fmt.Fprintf(&b, "\n\nExample:\n```spl\n%s\n```", doc.Example)
	}
	return b.String()
}

func withUsageContext(markdown, src string, line int, word string) string {
	var b strings.Builder
	b.WriteString(markdown)
	if call := callLineContext(src, line, word); call != "" {
		fmt.Fprintf(&b, "\n\nCall context:\n```spl\n%s\n```", call)
	} else if snippet := sourceLine(src, line); snippet != "" {
		fmt.Fprintf(&b, "\n\nCurrent context:\n```spl\n%s\n```", snippet)
	}
	return b.String()
}

func withSemanticHoverContext(markdown, path, src string, line, col int, word string) string {
	var b strings.Builder
	b.WriteString(markdown)
	if ctx := keywordHoverContext(path, src, line, col, word); ctx != "" {
		fmt.Fprintf(&b, "\n\n%s", ctx)
	}
	return b.String()
}

func keywordHoverContext(path, src string, line, col int, word string) string {
	switch word {
	case "match":
		if ctx := matchHoverContext(path, src, line, col); ctx != "" {
			return "Current match:\n" + ctx
		}
	case "print":
		if ctx := printHoverContext(path, src, line); ctx != "" {
			return "Current print:\n" + ctx
		}
	}
	if ctx := grammarUsageContext(path, src, line, word); ctx != "" {
		return "Current usage:\n" + ctx
	}
	return ""
}

type hoverValue struct {
	Known bool
	Repr  string
}

type hoverMatchSummary struct {
	Subject     string
	SubjectVal  hoverValue
	Cases       []hoverCaseSummary
	MatchedCase int
	Result      hoverValue
}

type hoverCaseSummary struct {
	Pattern string
	Guard   string
	Body    string
	Matches bool
	Result  hoverValue
}

func matchHoverContext(path, src string, line, col int) string {
	matches := matchSummaries(path, src)
	if len(matches) == 0 {
		if snippet := blockSnippetAroundLine(src, line); snippet != "" {
			return fmt.Sprintf("```spl\n%s\n```\n\nSPL evaluates the subject once, checks cases from top to bottom, and returns the body of the first matching case.", snippet)
		}
		return ""
	}
	idx := matchOrdinalAtLine(src, line)
	if idx < 0 || idx >= len(matches) {
		idx = 0
	}
	m := matches[idx]
	var b strings.Builder
	if snippet := blockSnippetAroundLine(src, line); snippet != "" {
		fmt.Fprintf(&b, "```spl\n%s\n```\n\n", snippet)
	}
	fmt.Fprintf(&b, "Evaluation steps:\n")
	fmt.Fprintf(&b, "- Subject `%s`", m.Subject)
	if m.SubjectVal.Known {
		fmt.Fprintf(&b, " resolves to `%s`", m.SubjectVal.Repr)
	}
	b.WriteString(".\n")
	for i, c := range m.Cases {
		status := "does not match"
		if c.Matches {
			status = "matches"
		}
		fmt.Fprintf(&b, "- Case %d pattern `%s` %s", i+1, c.Pattern, status)
		if c.Guard != "" {
			fmt.Fprintf(&b, "; guard `%s` is checked after the pattern", c.Guard)
		}
		if c.Matches && c.Result.Known {
			fmt.Fprintf(&b, " and returns `%s`", c.Result.Repr)
		}
		b.WriteString(".\n")
	}
	if m.Result.Known {
		fmt.Fprintf(&b, "- Match result is `%s`.", m.Result.Repr)
	} else {
		b.WriteString("- If no case matches, SPL returns `null`.")
	}
	return strings.TrimSpace(b.String())
}

func printHoverContext(path, src string, line int) string {
	lineText := sourceLine(src, line)
	if strings.TrimSpace(lineText) == "" {
		return ""
	}
	env := inferHoverValues(path, src)
	expr := printExpressionForLine(lineText)
	var b strings.Builder
	fmt.Fprintf(&b, "```spl\n%s\n```\n\n", lineText)
	b.WriteString("Evaluation steps:\n")
	if expr == nil {
		b.WriteString("- `print` evaluates the expression after the keyword, writes it to output, and returns `null`.")
		return b.String()
	}
	exprText := expr.String()
	value := resolveHoverExpression(expr, env)
	fmt.Fprintf(&b, "- Argument `%s` is evaluated", exprText)
	if value.Known {
		fmt.Fprintf(&b, " to `%s`", value.Repr)
	}
	b.WriteString(".\n")
	if value.Known {
		fmt.Fprintf(&b, "- Output line: `%s`.\n", value.Repr)
	}
	b.WriteString("- `print` returns `null` after writing output.")
	return strings.TrimSpace(b.String())
}

func grammarUsageContext(path, src string, line int, word string) string {
	lineText := sourceLine(src, line)
	trimmed := strings.TrimSpace(stripLineComment(lineText))
	if trimmed == "" {
		return ""
	}
	env := inferHoverValues(path, src)
	var b strings.Builder
	fmt.Fprintf(&b, "```spl\n%s\n```\n\n", strings.TrimSpace(lineText))
	b.WriteString("Interpretation:\n")
	switch word {
	case "let", "const":
		names, valueExpr := bindingPartsFromLine(trimmed)
		if len(names) == 0 {
			fmt.Fprintf(&b, "- This %s declaration creates binding(s) in the current lexical scope.", word)
			return b.String()
		}
		value := resolveHoverSnippet(valueExpr, env)
		for _, name := range names {
			fmt.Fprintf(&b, "- `%s` is bound", name)
			if valueExpr != "" {
				fmt.Fprintf(&b, " from `%s`", valueExpr)
			}
			if value.Known {
				fmt.Fprintf(&b, ", currently inferred as `%s`", value.Repr)
			}
			b.WriteString(".\n")
		}
		if word == "const" {
			b.WriteString("- The binding is intended to be immutable after initialization.")
		} else {
			b.WriteString("- The binding can be reassigned later in the same reachable scope.")
		}
	case "function", "async":
		name, params := functionPartsFromLine(trimmed)
		if name == "" {
			name = "anonymous function"
		}
		fmt.Fprintf(&b, "- `%s` defines callable code with %d parameter(s)", name, len(params))
		if len(params) > 0 {
			fmt.Fprintf(&b, ": `%s`", strings.Join(params, "`, `"))
		}
		b.WriteString(".\n")
		if word == "async" || strings.HasPrefix(trimmed, "async ") {
			b.WriteString("- Calls produce asynchronous work that should be consumed with `await` or future helpers.")
		} else {
			b.WriteString("- The body runs only when the function is called.")
		}
	case "return":
		expr := strings.TrimSpace(strings.TrimPrefix(trimmed, "return"))
		expr = strings.TrimSuffix(expr, ";")
		value := resolveHoverSnippet(expr, env)
		fmt.Fprintf(&b, "- Returns `%s` from the current function/callback", expr)
		if value.Known {
			fmt.Fprintf(&b, ", currently inferred as `%s`", value.Repr)
		}
		b.WriteString(".\n- Statements after this point in the same block are unreachable.")
	case "if", "while", "do":
		cond := conditionFromLine(trimmed)
		if cond == "" && word == "do" {
			b.WriteString("- The body runs once before its trailing `while` condition is checked.")
			return b.String()
		}
		value := resolveHoverSnippet(cond, env)
		fmt.Fprintf(&b, "- Condition `%s` is evaluated for truthiness", cond)
		if value.Known {
			fmt.Fprintf(&b, "; current inferred value is `%s`", value.Repr)
		}
		if word == "while" {
			b.WriteString(".\n- The body repeats while the condition remains truthy.")
		} else {
			b.WriteString(".\n- Only the truthy branch runs; otherwise `else` is considered if present.")
		}
	case "else":
		b.WriteString("- This branch runs only when the preceding `if` condition was falsey.")
	case "for":
		if strings.Contains(trimmed, " in ") {
			names, iterable := forInPartsFromLine(trimmed)
			fmt.Fprintf(&b, "- Iterates `%s`", iterable)
			if len(names) > 0 {
				fmt.Fprintf(&b, " and binds `%s` on each iteration", strings.Join(names, "`, `"))
			}
			b.WriteString(".")
		} else {
			b.WriteString("- Runs initializer, checks condition before each iteration, and runs post-expression after each body execution.")
		}
	case "in":
		if names, iterable := forInPartsFromLine(trimmed); iterable != "" {
			fmt.Fprintf(&b, "- `in` connects loop binding `%s` to iterable `%s`.", strings.Join(names, "`, `"), iterable)
		} else {
			b.WriteString("- `in` checks membership or introduces an iterable binding depending on context.")
		}
	case "break":
		b.WriteString("- Exits the nearest surrounding loop or switch-like control structure immediately.")
	case "continue":
		b.WriteString("- Skips the remaining statements in this loop iteration and proceeds to the next one.")
	case "import":
		target := quotedStringFromLine(trimmed)
		if target != "" {
			fmt.Fprintf(&b, "- Loads module `%s`", target)
			if loc, ok := importPathLocation(path, target); ok {
				fmt.Fprintf(&b, ", resolved to `%s`", loc)
			}
			b.WriteString(".")
		} else {
			b.WriteString("- Loads another module and binds imported names or aliases.")
		}
	case "export":
		b.WriteString("- Makes the following declaration available to importing modules.")
	case "try":
		b.WriteString("- Runs the protected block; thrown/runtime errors jump to `catch`, then `finally` runs if present.")
	case "catch":
		name := catchNameFromLine(trimmed)
		if name != "" {
			fmt.Fprintf(&b, "- Binds the caught error to `%s` inside this recovery block.", name)
		} else {
			b.WriteString("- Handles an error raised by the preceding `try` block.")
		}
	case "throw":
		expr := strings.TrimSpace(strings.TrimPrefix(trimmed, "throw"))
		expr = strings.TrimSuffix(expr, ";")
		value := resolveHoverSnippet(expr, env)
		fmt.Fprintf(&b, "- Raises `%s` and unwinds to the nearest matching `catch`", expr)
		if value.Known {
			fmt.Fprintf(&b, "; current inferred value is `%s`", value.Repr)
		}
		b.WriteString(".")
	case "switch":
		cond := conditionFromLine(trimmed)
		value := resolveHoverSnippet(cond, env)
		fmt.Fprintf(&b, "- Compares switch value `%s` against case labels in order", cond)
		if value.Known {
			fmt.Fprintf(&b, "; current inferred value is `%s`", value.Repr)
		}
		b.WriteString(".")
	case "case":
		pattern := casePatternFromLine(trimmed)
		fmt.Fprintf(&b, "- This branch is selected when the active match/switch value satisfies `%s`.", pattern)
	case "default":
		b.WriteString("- This fallback runs when no previous `case` matched.")
	case "type":
		b.WriteString("- Declares constructors/variants that can later be called and destructured by `match`.")
	case "class":
		name := secondWord(trimmed)
		fmt.Fprintf(&b, "- Declares class-like type `%s` and methods inside its block.", name)
	case "interface":
		name := secondWord(trimmed)
		fmt.Fprintf(&b, "- Declares interface-like contract `%s` with method signatures.", name)
	case "test":
		name := quotedStringFromLine(trimmed)
		fmt.Fprintf(&b, "- Registers test block `%s` for SPL test tooling.", name)
	case "init":
		b.WriteString("- Registers module initialization code that runs as part of module startup/loading.")
	case "await":
		expr := strings.TrimSpace(strings.TrimPrefix(trimmed, "await"))
		fmt.Fprintf(&b, "- Waits for `%s` to resolve before evaluating the surrounding expression.", expr)
	case "new":
		b.WriteString("- Constructs a class-like value from the named type and supplied arguments.")
	case "true", "false", "null":
		fmt.Fprintf(&b, "- Literal value is `%s`.", word)
	case "typeof":
		expr := strings.TrimSpace(strings.TrimPrefix(trimmed, "typeof"))
		fmt.Fprintf(&b, "- Inspects the SPL runtime type of `%s`.", expr)
	case "and", "or", "not":
		b.WriteString(operatorWordContext(trimmed, word, env))
	case "lazy":
		expr := strings.TrimSpace(strings.TrimPrefix(trimmed, "lazy"))
		fmt.Fprintf(&b, "- Delays evaluation of `%s` until the lazy value is forced.", expr)
	case "as":
		b.WriteString("- Introduces an alias name for the imported module or symbol in this import clause.")
	case "from":
		target := quotedStringFromLine(trimmed)
		fmt.Fprintf(&b, "- Reads named imports from module `%s`.", target)
	default:
		b.WriteString("- This grammar construct is interpreted according to the surrounding statement/expression.")
	}
	return strings.TrimSpace(b.String())
}

func matchSummaries(path, src string) []hoverMatchSummary {
	program := parseProgramForHover(src)
	if program == nil {
		return nil
	}
	env := map[string]hoverValue{}
	out := []hoverMatchSummary{}
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.LetStatement:
			if s.Value == nil {
				continue
			}
			value := resolveHoverExpression(s.Value, env)
			if match, ok := s.Value.(*ast.MatchExpression); ok {
				summary := summarizeMatch(match, env)
				out = append(out, summary)
				value = summary.Result
			}
			for _, name := range letStatementNames(s) {
				if value.Known {
					env[name] = value
				}
			}
		case *ast.ExpressionStatement:
			if match, ok := s.Expression.(*ast.MatchExpression); ok {
				out = append(out, summarizeMatch(match, env))
			}
		case *ast.MatchExpression:
			out = append(out, summarizeMatch(s, env))
		}
	}
	return out
}

func summarizeMatch(match *ast.MatchExpression, env map[string]hoverValue) hoverMatchSummary {
	summary := hoverMatchSummary{
		Subject:     safeNodeString(match.Value),
		SubjectVal:  resolveHoverExpression(match.Value, env),
		MatchedCase: -1,
	}
	for i, mc := range match.Cases {
		item := hoverCaseSummary{
			Pattern: safePatternString(mc.Pattern),
			Guard:   safeNodeString(mc.Guard),
			Body:    blockResultString(mc.Body),
		}
		item.Matches = hoverPatternMatches(mc.Pattern, summary.SubjectVal)
		item.Result = resolveHoverBlock(mc.Body, env)
		if item.Matches && summary.MatchedCase == -1 {
			summary.MatchedCase = i
			summary.Result = item.Result
		}
		summary.Cases = append(summary.Cases, item)
	}
	return summary
}

func inferHoverValues(path, src string) map[string]hoverValue {
	_ = path
	env := inferHoverValuesFromLines(src)
	program := parseProgramForHover(src)
	if program == nil {
		return env
	}
	for _, stmt := range program.Statements {
		ls, ok := stmt.(*ast.LetStatement)
		if !ok || ls.Value == nil {
			continue
		}
		value := resolveHoverExpression(ls.Value, env)
		if match, ok := ls.Value.(*ast.MatchExpression); ok {
			value = summarizeMatch(match, env).Result
		}
		if !value.Known {
			continue
		}
		for _, name := range letStatementNames(ls) {
			env[name] = value
		}
	}
	return env
}

func inferHoverValuesFromLines(src string) map[string]hoverValue {
	env := map[string]hoverValue{}
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(stripLineComment(line))
		if !(strings.HasPrefix(trimmed, "let ") || strings.HasPrefix(trimmed, "const ")) {
			continue
		}
		names, valueExpr := bindingPartsFromLine(trimmed)
		if len(names) == 0 || valueExpr == "" || strings.HasPrefix(valueExpr, "match ") {
			continue
		}
		value := resolveHoverSnippet(valueExpr, env)
		if !value.Known {
			continue
		}
		for _, name := range names {
			env[name] = value
		}
	}
	return env
}

func resolveHoverExpression(expr ast.Expression, env map[string]hoverValue) hoverValue {
	switch e := expr.(type) {
	case nil:
		return hoverValue{}
	case *ast.IntegerLiteral:
		return hoverValue{Known: true, Repr: e.String()}
	case *ast.FloatLiteral:
		return hoverValue{Known: true, Repr: e.String()}
	case *ast.StringLiteral:
		return hoverValue{Known: true, Repr: e.Value}
	case *ast.BooleanLiteral:
		return hoverValue{Known: true, Repr: e.String()}
	case *ast.NullLiteral:
		return hoverValue{Known: true, Repr: "null"}
	case *ast.Identifier:
		if v, ok := env[e.Name]; ok {
			return v
		}
	case *ast.PrefixExpression:
		right := resolveHoverExpression(e.Right, env)
		if !right.Known {
			return hoverValue{}
		}
		switch e.Operator {
		case "!":
			return hoverBool(!hoverTruthy(right))
		case "-":
			if strings.HasPrefix(right.Repr, "-") {
				return hoverValue{Known: true, Repr: strings.TrimPrefix(right.Repr, "-")}
			}
			return hoverValue{Known: true, Repr: "-" + right.Repr}
		}
	case *ast.InfixExpression:
		left := resolveHoverExpression(e.Left, env)
		right := resolveHoverExpression(e.Right, env)
		return resolveHoverInfix(left, e.Operator, right)
	case *ast.MatchExpression:
		return summarizeMatch(e, env).Result
	}
	return hoverValue{}
}

func resolveHoverSnippet(expr string, env map[string]hoverValue) hoverValue {
	expr = strings.TrimSpace(strings.TrimSuffix(expr, ";"))
	if expr == "" {
		return hoverValue{}
	}
	program := parseProgramForHover("let __hover = " + expr)
	if program == nil || len(program.Statements) == 0 {
		return hoverValue{}
	}
	if ls, ok := program.Statements[0].(*ast.LetStatement); ok {
		return resolveHoverExpression(ls.Value, env)
	}
	return hoverValue{}
}

func resolveHoverInfix(left hoverValue, operator string, right hoverValue) hoverValue {
	if !left.Known || !right.Known {
		return hoverValue{}
	}
	switch operator {
	case "==":
		return hoverBool(left.Repr == right.Repr)
	case "!=":
		return hoverBool(left.Repr != right.Repr)
	case "&&":
		return hoverBool(hoverTruthy(left) && hoverTruthy(right))
	case "||":
		return hoverBool(hoverTruthy(left) || hoverTruthy(right))
	case "+":
		if li, lok := parseHoverInt(left); lok {
			if ri, rok := parseHoverInt(right); rok {
				return hoverValue{Known: true, Repr: fmt.Sprintf("%d", li+ri)}
			}
		}
		return hoverValue{Known: true, Repr: left.Repr + right.Repr}
	case "-", "*", "/", "%", "<", "<=", ">", ">=":
		li, lok := parseHoverInt(left)
		ri, rok := parseHoverInt(right)
		if !lok || !rok {
			return hoverValue{}
		}
		switch operator {
		case "-":
			return hoverValue{Known: true, Repr: fmt.Sprintf("%d", li-ri)}
		case "*":
			return hoverValue{Known: true, Repr: fmt.Sprintf("%d", li*ri)}
		case "/":
			if ri == 0 {
				return hoverValue{}
			}
			return hoverValue{Known: true, Repr: fmt.Sprintf("%d", li/ri)}
		case "%":
			if ri == 0 {
				return hoverValue{}
			}
			return hoverValue{Known: true, Repr: fmt.Sprintf("%d", li%ri)}
		case "<":
			return hoverBool(li < ri)
		case "<=":
			return hoverBool(li <= ri)
		case ">":
			return hoverBool(li > ri)
		case ">=":
			return hoverBool(li >= ri)
		}
	}
	return hoverValue{}
}

func parseHoverInt(value hoverValue) (int64, bool) {
	var n int64
	if _, err := fmt.Sscanf(value.Repr, "%d", &n); err != nil {
		return 0, false
	}
	return n, true
}

func hoverBool(v bool) hoverValue {
	if v {
		return hoverValue{Known: true, Repr: "true"}
	}
	return hoverValue{Known: true, Repr: "false"}
}

func hoverTruthy(value hoverValue) bool {
	switch value.Repr {
	case "", "false", "null":
		return false
	default:
		return true
	}
}

func resolveHoverBlock(block *ast.BlockStatement, env map[string]hoverValue) hoverValue {
	if block == nil || len(block.Statements) == 0 {
		return hoverValue{}
	}
	last := block.Statements[len(block.Statements)-1]
	switch s := last.(type) {
	case *ast.ExpressionStatement:
		return resolveHoverExpression(s.Expression, env)
	case *ast.ReturnStatement:
		return resolveHoverExpression(s.ReturnValue, env)
	}
	return hoverValue{}
}

func hoverPatternMatches(pattern ast.Pattern, value hoverValue) bool {
	if !value.Known || pattern == nil {
		return false
	}
	switch p := pattern.(type) {
	case *ast.LiteralPattern:
		return resolveHoverExpression(p.Value, map[string]hoverValue{}).Repr == value.Repr
	case *ast.WildcardPattern:
		return true
	case *ast.OrPattern:
		for _, sub := range p.Patterns {
			if hoverPatternMatches(sub, value) {
				return true
			}
		}
	}
	return false
}

func printExpressionForLine(lineText string) ast.Expression {
	trimmed := strings.TrimSpace(stripLineComment(lineText))
	program := parseProgramForHover(trimmed)
	if program != nil {
		for _, stmt := range program.Statements {
			if ps, ok := stmt.(*ast.PrintStatement); ok {
				return ps.Expression
			}
		}
	}
	// `print(r1)` is parsed as a print statement by the real parser, but keep a
	// tiny fallback for incomplete editor states while the user is typing.
	if strings.HasPrefix(trimmed, "print(") && strings.HasSuffix(trimmed, ")") {
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "print("), ")"))
		if splIdentifierRe.MatchString(name) {
			return &ast.Identifier{Name: name}
		}
	}
	return nil
}

func bindingPartsFromLine(line string) ([]string, string) {
	line = strings.TrimSpace(strings.TrimSuffix(line, ";"))
	line = strings.TrimPrefix(line, "let ")
	line = strings.TrimPrefix(line, "const ")
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return splitBindingNames(line), ""
	}
	return splitBindingNames(parts[0]), strings.TrimSpace(parts[1])
}

func functionPartsFromLine(line string) (string, []string) {
	line = strings.TrimSpace(strings.TrimPrefix(line, "async "))
	if !strings.HasPrefix(line, "function") {
		return "", nil
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, "function"))
	name := ""
	if idx := strings.Index(rest, "("); idx >= 0 {
		name = strings.TrimSpace(rest[:idx])
	}
	params, _ := paramsFromFunctionHeader(line)
	return name, params
}

func conditionFromLine(line string) string {
	open := strings.Index(line, "(")
	if open < 0 {
		return ""
	}
	depth := 0
	for i, r := range line[open:] {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return strings.TrimSpace(line[open+1 : open+i])
			}
		}
	}
	return ""
}

func forInPartsFromLine(line string) ([]string, string) {
	line = strings.TrimSpace(strings.TrimPrefix(line, "for "))
	line = strings.Trim(line, "() {")
	parts := strings.SplitN(line, " in ", 2)
	if len(parts) != 2 {
		return nil, ""
	}
	names := splitBindingNames(parts[0])
	iterable := strings.TrimSpace(strings.TrimSuffix(parts[1], "{"))
	if idx := strings.Index(iterable, "{"); idx >= 0 {
		iterable = strings.TrimSpace(iterable[:idx])
	}
	return names, iterable
}

func quotedStringFromLine(line string) string {
	for _, quote := range []string{`"`, `'`} {
		start := strings.Index(line, quote)
		if start < 0 {
			continue
		}
		end := strings.Index(line[start+1:], quote)
		if end >= 0 {
			return line[start+1 : start+1+end]
		}
	}
	return ""
}

func importPathLocation(path, target string) (string, bool) {
	if target == "" || strings.Contains(target, "://") {
		return "", false
	}
	candidates := []string{target}
	if path != "" && path != DefaultStdinPath() {
		candidates = append([]string{filepath.Join(filepath.Dir(path), target)}, candidates...)
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			if abs, err := filepath.Abs(candidate); err == nil {
				return abs, true
			}
			return candidate, true
		}
	}
	return "", false
}

func catchNameFromLine(line string) string {
	open := strings.Index(line, "(")
	close := strings.Index(line, ")")
	if open >= 0 && close > open {
		return strings.TrimSpace(strings.Split(line[open+1:close], ":")[0])
	}
	fields := strings.Fields(line)
	if len(fields) >= 2 {
		return strings.Trim(fields[1], "{}")
	}
	return ""
}

func casePatternFromLine(line string) string {
	line = strings.TrimSpace(strings.TrimPrefix(line, "case "))
	if idx := strings.Index(line, "=>"); idx >= 0 {
		return strings.TrimSpace(line[:idx])
	}
	if idx := strings.Index(line, ":"); idx >= 0 {
		return strings.TrimSpace(line[:idx])
	}
	return strings.TrimSpace(line)
}

func secondWord(line string) string {
	fields := strings.Fields(line)
	if len(fields) >= 2 {
		return strings.Trim(fields[1], "{(")
	}
	return ""
}

func operatorWordContext(line, word string, env map[string]hoverValue) string {
	switch word {
	case "not":
		expr := strings.TrimSpace(strings.TrimPrefix(line, "not"))
		value := resolveHoverSnippet(expr, env)
		if value.Known {
			return fmt.Sprintf("- Negates `%s`; current inferred result is `%t`.", expr, !hoverTruthy(value))
		}
		return fmt.Sprintf("- Negates the truthiness of `%s`.", expr)
	case "and", "or":
		parts := strings.Split(line, " "+word+" ")
		if len(parts) >= 2 {
			left := resolveHoverSnippet(parts[0], env)
			right := resolveHoverSnippet(parts[1], env)
			if left.Known && right.Known {
				result := hoverTruthy(left) && hoverTruthy(right)
				if word == "or" {
					result = hoverTruthy(left) || hoverTruthy(right)
				}
				return fmt.Sprintf("- Evaluates `%s` %s `%s`; current inferred result is `%t`.", strings.TrimSpace(parts[0]), word, strings.TrimSpace(parts[1]), result)
			}
		}
		return fmt.Sprintf("- `%s` combines boolean/truthy expressions with short-circuit behavior.", word)
	default:
		return ""
	}
}

func parseProgramForHover(src string) *ast.Program {
	l := lexer.NewLexer(src)
	p := parser.NewParser(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		return nil
	}
	return program
}

func letStatementNames(stmt *ast.LetStatement) []string {
	if stmt == nil {
		return nil
	}
	names := []string{}
	if stmt.Name != nil {
		names = append(names, stmt.Name.Name)
	}
	for _, name := range stmt.Names {
		if name != nil {
			names = append(names, name.Name)
		}
	}
	return names
}

func blockResultString(block *ast.BlockStatement) string {
	if block == nil || len(block.Statements) == 0 {
		return ""
	}
	return strings.TrimSuffix(block.Statements[len(block.Statements)-1].String(), ";")
}

func safeNodeString(node ast.Node) string {
	if node == nil {
		return ""
	}
	return node.String()
}

func safePatternString(pattern ast.Pattern) string {
	if pattern == nil {
		return ""
	}
	return pattern.String()
}

func stripLineComment(line string) string {
	if idx := strings.Index(line, "//"); idx >= 0 {
		return line[:idx]
	}
	return line
}

func blockSnippetAroundLine(src string, line int) string {
	lines := strings.Split(src, "\n")
	if line <= 0 || line > len(lines) {
		return ""
	}
	start := line - 1
	for start > 0 && !strings.Contains(lines[start], "match") {
		start--
	}
	end := line - 1
	depth := 0
	seenBrace := false
	for i := start; i < len(lines); i++ {
		for _, r := range lines[i] {
			switch r {
			case '{':
				depth++
				seenBrace = true
			case '}':
				depth--
			}
		}
		end = i
		if seenBrace && depth <= 0 {
			break
		}
	}
	return strings.TrimSpace(strings.Join(lines[start:end+1], "\n"))
}

func matchOrdinalAtLine(src string, line int) int {
	if line <= 0 {
		return 0
	}
	lines := strings.Split(src, "\n")
	limit := min(line, len(lines))
	count := 0
	for i := 0; i < limit; i++ {
		text := stripLineComment(lines[i])
		for _, loc := range splIdentifierRe.FindAllStringIndex(text, -1) {
			if text[loc[0]:loc[1]] == "match" {
				count++
			}
		}
	}
	if count == 0 {
		return 0
	}
	return count - 1
}

func lexicalDeclarationContext(src, word string, line, col int) (string, bool) {
	if word == "" {
		return "", false
	}
	lines := strings.Split(src, "\n")
	for i := min(line-1, len(lines)-1); i >= 0; i-- {
		text := lines[i]
		if declarationLineContains(text, word) {
			return declarationMarkdown("local binding", word, i+1, text, src, line), true
		}
		if params, header, ok := functionHeaderBefore(src, i); ok {
			for _, p := range params {
				if p == word {
					return parameterMarkdown(word, i+1, header, src, line), true
				}
			}
		}
	}
	if paramLine, header, ok := nearestInlineFunctionParameter(src, word, line, col); ok {
		return parameterMarkdown(word, paramLine, header, src, line), true
	}
	return "", false
}

func declarationLineContains(line, word string) bool {
	patterns := []string{
		`\blet\s+([^=;]+)`,
		`\bconst\s+([^=;]+)`,
		`\bfunction\s+` + regexp.QuoteMeta(word) + `\b`,
		`\bfor\s*\((?:\s*let\s+)?` + regexp.QuoteMeta(word) + `\b`,
		`\bfor\s+` + regexp.QuoteMeta(word) + `\s+in\b`,
		`\bfor\s+[^{}\n,]+,\s*` + regexp.QuoteMeta(word) + `\s+in\b`,
	}
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if len(m) == 1 {
			return true
		}
		for _, name := range splitBindingNames(m[1]) {
			if name == word {
				return true
			}
		}
	}
	return false
}

func declarationMarkdown(kind, word string, declLine int, declText, src string, hoverLine int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**%s** `%s`\n\n", kind, word)
	fmt.Fprintf(&b, "Declared on line %d. SPL evaluates the declaration expression and binds `%s` in the surrounding lexical block.\n\n", declLine, word)
	fmt.Fprintf(&b, "Declaration:\n```spl\n%s\n```", strings.TrimSpace(declText))
	if snippet := sourceLine(src, hoverLine); snippet != "" && strings.TrimSpace(snippet) != strings.TrimSpace(declText) {
		fmt.Fprintf(&b, "\n\nCurrent use:\n```spl\n%s\n```", snippet)
	}
	return b.String()
}

func parameterMarkdown(word string, line int, header, src string, hoverLine int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**function parameter** `%s`\n\n", word)
	fmt.Fprintf(&b, "`%s` is bound when this callback/function is invoked. It is scoped to the function body and shadows outer bindings with the same name.\n\n", word)
	if header != "" {
		fmt.Fprintf(&b, "Function/callback:\n```spl\n%s\n```", strings.TrimSpace(header))
	}
	if snippet := sourceLine(src, hoverLine); snippet != "" {
		fmt.Fprintf(&b, "\n\nCurrent context:\n```spl\n%s\n```", snippet)
	}
	if line > 0 {
		fmt.Fprintf(&b, "\n\nParameter source line: %d.", line)
	}
	return b.String()
}

func callContextMarkdown(src string, line, col int, word string) string {
	text := sourceLine(src, line)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	idx := col - 1
	if idx < 0 || idx > len(runes) {
		return ""
	}
	after := strings.TrimLeft(string(runes[min(idx+len([]rune(word)), len(runes)):]), " \t")
	if !strings.HasPrefix(after, "(") {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "**function call** `%s(...)`\n\n", word)
	b.WriteString("This is a call expression. SPL evaluates the callee, evaluates each argument from left to right, then invokes the function/builtin with those argument values.")
	fmt.Fprintf(&b, "\n\nCall context:\n```spl\n%s\n```", text)
	return b.String()
}

func blockHoverMarkdown(src string, line, col int) string {
	ch := charAtPosition(src, line, col)
	text := sourceLine(src, line)
	switch ch {
	case '{':
		return fmt.Sprintf("**block** `{ ... }`\n\nA block groups statements under a control structure, function, callback, test, init block, or object/hash literal context. Bindings declared inside statement blocks are scoped to that block.\n\nCurrent context:\n```spl\n%s\n```", text)
	case '}':
		return fmt.Sprintf("**block end** `}`\n\nCloses the nearest SPL block or hash literal. If this belongs to a function/callback, execution returns to the caller after the block completes unless `return`, `throw`, `break`, or `continue` changes control flow.\n\nCurrent context:\n```spl\n%s\n```", text)
	case '(':
		return fmt.Sprintf("**argument/condition list** `(`\n\nStarts a grouped expression, call argument list, function parameter list, or control-flow condition depending on the preceding token.\n\nCurrent context:\n```spl\n%s\n```", text)
	case ')':
		return fmt.Sprintf("**argument/condition list end** `)`\n\nCloses the current grouped expression, parameter list, call arguments, or condition.\n\nCurrent context:\n```spl\n%s\n```", text)
	case '[':
		return fmt.Sprintf("**array/index context** `[`\n\nStarts an array literal or index expression depending on the value before it.\n\nCurrent context:\n```spl\n%s\n```", text)
	case ']':
		return fmt.Sprintf("**array/index context end** `]`\n\nCloses an array literal or index expression.\n\nCurrent context:\n```spl\n%s\n```", text)
	default:
		return ""
	}
}

func bestSymbolForWord(idx *WorkspaceIndex, path, src, word string) (Symbol, bool) {
	for _, sym := range SymbolsForSource(path, src) {
		if sym.Name == word {
			return sym, true
		}
	}
	if idx != nil {
		cleanPath := cleanAbsPath(path)
		for _, sym := range idx.AllSymbols() {
			if sym.Name == word && cleanAbsPath(sym.Path) == cleanPath {
				return sym, true
			}
		}
		for _, sym := range idx.AllSymbols() {
			if sym.Name == word {
				return sym, true
			}
		}
	}
	return Symbol{}, false
}

func sourceLine(src string, line int) string {
	lines := strings.Split(src, "\n")
	if line <= 0 || line > len(lines) {
		return ""
	}
	return strings.TrimRight(lines[line-1], "\r")
}

func completionDetail(item CompletionItem) string {
	if item.Kind == "keyword" {
		if doc, ok := splKeywordDocs[item.Label]; ok {
			return doc.Category
		}
	}
	if item.Detail != "" && item.Kind != "" {
		return item.Kind + " - " + firstLine(item.Detail)
	}
	return item.Kind
}

func CompletionDetail(item CompletionItem) string {
	return completionDetail(item)
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

func splitBindingNames(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "let ")
	raw = strings.TrimPrefix(raw, "const ")
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		part = strings.TrimPrefix(part, "...")
		if idx := strings.IndexAny(part, ":=; \t"); idx >= 0 {
			part = part[:idx]
		}
		part = strings.Trim(part, "()[]{} ")
		if splIdentifierRe.MatchString(part) {
			out = append(out, part)
		}
	}
	return out
}

func functionHeaderBefore(src string, lineIdx int) ([]string, string, bool) {
	lines := strings.Split(src, "\n")
	if lineIdx < 0 || lineIdx >= len(lines) {
		return nil, "", false
	}
	for i := lineIdx; i >= 0 && i >= lineIdx-12; i-- {
		line := lines[i]
		if !strings.Contains(line, "function") {
			continue
		}
		header := line
		for j := i + 1; j <= lineIdx && !strings.Contains(header, "{"); j++ {
			header += " " + strings.TrimSpace(lines[j])
		}
		params, ok := paramsFromFunctionHeader(header)
		if ok {
			return params, header, true
		}
	}
	return nil, "", false
}

func nearestInlineFunctionParameter(src, word string, line, col int) (int, string, bool) {
	lines := strings.Split(src, "\n")
	if line <= 0 || line > len(lines) {
		return 0, "", false
	}
	current := lines[line-1]
	if !strings.Contains(current, "function") {
		return 0, "", false
	}
	params, ok := paramsFromFunctionHeader(current)
	if !ok {
		return 0, "", false
	}
	for _, p := range params {
		if p == word {
			return line, current, true
		}
	}
	return 0, "", false
}

func paramsFromFunctionHeader(header string) ([]string, bool) {
	idx := strings.Index(header, "function")
	if idx < 0 {
		return nil, false
	}
	rest := header[idx+len("function"):]
	open := strings.Index(rest, "(")
	close := strings.Index(rest, ")")
	if open < 0 || close < 0 || close < open {
		return nil, false
	}
	paramsRaw := rest[open+1 : close]
	params := []string{}
	for _, p := range strings.Split(paramsRaw, ",") {
		p = strings.TrimSpace(strings.TrimPrefix(p, "..."))
		if eq := strings.Index(p, "="); eq >= 0 {
			p = p[:eq]
		}
		if colon := strings.Index(p, ":"); colon >= 0 {
			p = p[:colon]
		}
		p = strings.TrimSpace(p)
		if splIdentifierRe.MatchString(p) {
			params = append(params, p)
		}
	}
	return params, true
}

func callLineContext(src string, line int, word string) string {
	text := sourceLine(src, line)
	if text == "" || word == "" {
		return ""
	}
	idx := strings.Index(text, word)
	for idx >= 0 {
		afterStart := idx + len(word)
		if beforeOK(text, idx) && afterStart < len(text) && strings.HasPrefix(strings.TrimLeft(text[afterStart:], " \t"), "(") {
			return text
		}
		next := strings.Index(text[afterStart:], word)
		if next < 0 {
			break
		}
		idx = afterStart + next
	}
	return ""
}

func beforeOK(text string, idx int) bool {
	if idx <= 0 {
		return true
	}
	r, _ := utf8LastRune(text[:idx])
	return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_')
}

func utf8LastRune(s string) (rune, bool) {
	runes := []rune(s)
	if len(runes) == 0 {
		return 0, false
	}
	return runes[len(runes)-1], true
}

func charAtPosition(src string, line, col int) rune {
	lines := strings.Split(src, "\n")
	if line <= 0 || line > len(lines) {
		return 0
	}
	runes := []rune(lines[line-1])
	idx := col - 1
	if idx < 0 || idx >= len(runes) {
		return 0
	}
	return runes[idx]
}

type ExecuteSPLFunc func(path, src string, opts EvaluationOptions, output io.Writer) (result string, err error)

var ExecuteSPLFn ExecuteSPLFunc

func EvaluateSPL(path, src string, opts EvaluationOptions) EvaluationResult {
	start := time.Now()
	profile := strings.ToLower(strings.TrimSpace(opts.Profile))
	if profile == "" {
		profile = "untrusted"
	}
	if profile != "untrusted" && profile != "native" && profile != "trusted" {
		profile = "untrusted"
	}
	timeout := time.Duration(opts.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 1500 * time.Millisecond
	}
	maxOutput := opts.MaxOutputBytes
	if maxOutput <= 0 {
		maxOutput = 64 * 1024
	}
	maxSteps := opts.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 200_000
	}
	maxDepth := opts.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 128
	}
	opts.Profile = profile
	opts.TimeoutMS = timeout.Milliseconds()
	opts.MaxOutputBytes = maxOutput
	opts.MaxSteps = maxSteps
	opts.MaxDepth = maxDepth
	var output bytes.Buffer
	result := EvaluationResult{Path: path, Duration: time.Since(start).Milliseconds()}
	if ExecuteSPLFn == nil {
		result.OK = false
		result.Error = "SPL evaluation is not configured"
		return result
	}
	inspect, err := ExecuteSPLFn(path, src, opts, &output)
	result.OK = err == nil
	result.Output = output.String()
	result.Duration = time.Since(start).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Result = inspect
	return result
}

func importLocationAt(path, src string, line, col int) (Location, bool) {
	if target, ok := importTargetAt(src, line, col); ok {
		if _, isStd := splStdModuleDocs[target]; isStd {
			return Location{}, false
		}
		if _, isStd := knownStdModules[target]; isStd {
			return Location{}, false
		}
		if resolved, ok := resolveImportCandidate(path, target); ok {
			return Location{Path: resolved, Line: 1, Column: 1, Name: target}, true
		}
		return Location{}, false
	}
	return Location{}, false
}

func importTargetAt(src string, line, col int) (string, bool) {
	lines := strings.Split(src, "\n")
	if line <= 0 || line > len(lines) {
		return "", false
	}
	text := lines[line-1]
	if !strings.Contains(text, "import") {
		return "", false
	}
	idx := col - 1
	if idx < 0 {
		idx = 0
	}
	spans := quotedSpans(text)
	for _, span := range spans {
		if idx >= span.start && idx <= span.end {
			target := text[span.start+1 : span.end]
			return target, true
		}
	}
	return "", false
}

type quoteSpan struct {
	start int
	end   int
}

func quotedSpans(text string) []quoteSpan {
	out := []quoteSpan{}
	var quote rune
	start := -1
	escaped := false
	for i, r := range text {
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == quote {
				out = append(out, quoteSpan{start: start, end: i})
				quote = 0
				start = -1
			}
			continue
		}
		if r == '"' || r == '\'' {
			quote = r
			start = i
		}
	}
	return out
}

func resolveImportCandidate(path, target string) (string, bool) {
	if strings.TrimSpace(target) == "" || strings.Contains(target, "://") {
		return "", false
	}
	candidates := []string{target}
	if path != "" && path != DefaultStdinPath() {
		candidates = append([]string{filepath.Join(filepath.Dir(path), target)}, candidates...)
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil {
			if info.IsDir() {
				for _, name := range []string{"index.spl", "main.spl"} {
					nested := filepath.Join(candidate, name)
					if _, err := os.Stat(nested); err == nil {
						return cleanAbsPath(nested), true
					}
				}
				continue
			}
			return cleanAbsPath(candidate), true
		}
	}
	return "", false
}

func cleanAbsPath(path string) string {
	if path == "" || path == DefaultStdinPath() {
		return path
	}
	if abs, err := filepath.Abs(path); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(path)
}

func CleanAbsPath(path string) string {
	return cleanAbsPath(path)
}

func moduleDirForPath(path string) string {
	if path == "" || path == DefaultStdinPath() {
		return "."
	}
	return filepath.Dir(path)
}

func ModuleDirForPath(path string) string {
	return moduleDirForPath(path)
}

func pathToURI(path string) string {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(cleanAbsPath(path))}
	return u.String()
}

func PathToURI(path string) string {
	return pathToURI(path)
}

func uriToPath(uri string) string {
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "file" {
		return uri
	}
	path := parsed.Path
	if parsed.Host != "" {
		path = "//" + parsed.Host + parsed.Path
	}
	if p, err := url.PathUnescape(path); err == nil {
		path = p
	}
	return cleanAbsPath(filepath.FromSlash(path))
}

func URIToPath(uri string) string {
	return uriToPath(uri)
}

func prefixAt(src string, line, col int) string {
	lines := strings.Split(src, "\n")
	if line <= 0 || line > len(lines) {
		return ""
	}
	runes := []rune(lines[line-1])
	idx := col - 1
	if idx < 0 {
		return ""
	}
	if idx > len(runes) {
		idx = len(runes)
	}
	start := idx
	for start > 0 && (unicode.IsLetter(runes[start-1]) || unicode.IsDigit(runes[start-1]) || runes[start-1] == '_') {
		start--
	}
	return string(runes[start:idx])
}

func PrefixAt(src string, line, col int) string {
	return prefixAt(src, line, col)
}

func runeColumn(s string, byteOffset int) int {
	if byteOffset <= 0 {
		return 0
	}
	if byteOffset > len(s) {
		byteOffset = len(s)
	}
	return len([]rune(s[:byteOffset]))
}

func RuneColumn(s string, byteOffset int) int {
	return runeColumn(s, byteOffset)
}

func symbolSummary(sym Symbol) string {
	if sym.Detail != "" {
		return fmt.Sprintf("%s %s - %s", sym.Kind, sym.Name, sym.Detail)
	}
	return fmt.Sprintf("%s %s", sym.Kind, sym.Name)
}

func SymbolSummary(sym Symbol) string {
	return symbolSummary(sym)
}
