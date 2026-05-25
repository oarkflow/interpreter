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
	Profile        string `json:"profile,omitempty"`
	TimeoutMS      int64  `json:"timeoutMs,omitempty"`
	MaxOutputBytes int64  `json:"maxOutputBytes,omitempty"`
	MaxSteps       int64  `json:"maxSteps,omitempty"`
	MaxDepth       int    `json:"maxDepth,omitempty"`
}

type EvaluationResult struct {
	OK       bool   `json:"ok"`
	Path     string `json:"path,omitempty"`
	Result   string `json:"result,omitempty"`
	Output   string `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`
	Duration int64  `json:"durationMs"`
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
	for _, sym := range SymbolsForSource(path, src) {
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
	for _, item := range CompletionItems(path, src, prefix) {
		add(item)
	}
	for _, doc := range splRuntimeDocs {
		add(CompletionItem{Label: doc.Name, Kind: "builtin", Detail: RuntimeDocMarkdown(doc.Name)})
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
		return ""
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
	if profile != "untrusted" {
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
	lines := strings.Split(src, "\n")
	if line <= 0 || line > len(lines) {
		return Location{}, false
	}
	text := lines[line-1]
	if !strings.Contains(text, "import") {
		return Location{}, false
	}
	idx := col - 1
	if idx < 0 {
		idx = 0
	}
	spans := quotedSpans(text)
	for _, span := range spans {
		if idx >= span.start && idx <= span.end {
			target := text[span.start+1 : span.end]
			if resolved, ok := resolveImportCandidate(path, target); ok {
				return Location{Path: resolved, Line: 1, Column: 1, Name: target}, true
			}
		}
	}
	return Location{}, false
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
