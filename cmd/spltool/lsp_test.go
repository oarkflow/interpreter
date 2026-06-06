package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLSPInitializeAndDiagnosticsFlow(t *testing.T) {
	dir := t.TempDir()
	var out, err bytes.Buffer
	s := newLSPServer(strings.NewReader(""), &out, &err)

	result, dispatchErr := s.dispatch("initialize", mustRaw(map[string]any{"rootUri": pathToURI(dir)}))
	if dispatchErr != nil {
		t.Fatalf("initialize failed: %v", dispatchErr)
	}
	raw, _ := json.Marshal(result)
	if !strings.Contains(string(raw), "completionProvider") {
		t.Fatalf("expected capabilities, got %s", raw)
	}
	if strings.Contains(string(raw), "executeCommandProvider") {
		t.Fatalf("server must not advertise VS Code extension-owned commands, got %s", raw)
	}

	uri := pathToURI(filepath.Join(dir, "bad.spl"))
	s.dispatch("textDocument/didOpen", mustRaw(map[string]any{
		"textDocument": map[string]any{"uri": uri, "text": "let x = ;"},
	}))
	if !strings.Contains(out.String(), "publishDiagnostics") || !strings.Contains(out.String(), "expected") {
		t.Fatalf("expected diagnostic notification, got %q stderr=%q", out.String(), err.String())
	}
}

func TestLSPNullHoverResponseIncludesResult(t *testing.T) {
	dir := t.TempDir()
	uri := pathToURI(filepath.Join(dir, "blank.spl"))
	input := append(encodeRPC("initialize", 1, map[string]any{"rootUri": pathToURI(dir)}), encodeRPC("textDocument/hover", 2, posParams(uri, 0, 0))...)
	var out, errb bytes.Buffer
	code := runLSPServer(bytes.NewReader(input), &out, &errb)
	if code != 0 {
		t.Fatalf("server failed: code=%d stderr=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), `"id":2`) || !strings.Contains(out.String(), `"result":null`) {
		t.Fatalf("expected explicit null result for empty hover, got %q", out.String())
	}
}

func TestLSPCompletionHoverDefinitionReferencesAndFormatting(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.spl")
	libPath := filepath.Join(dir, "lib.spl")
	mainSrc := "import \"./lib.spl\";\nlet add = function(a, b) { return a + b; };\nlet total = add(1, 2);\n"
	if err := os.WriteFile(mainPath, []byte(mainSrc), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(libPath, []byte("export let helper = 1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	s := newLSPServer(strings.NewReader(""), &out, &errb)
	if _, err := s.dispatch("initialize", mustRaw(map[string]any{"rootUri": pathToURI(dir)})); err != nil {
		t.Fatal(err)
	}
	uri := pathToURI(mainPath)
	s.dispatch("textDocument/didOpen", mustRaw(map[string]any{
		"textDocument": map[string]any{"uri": uri, "text": mainSrc},
	}))

	completion, _ := s.dispatch("textDocument/completion", mustRaw(posParams(uri, 2, 15)))
	completionJSON, _ := json.Marshal(completion)
	if !strings.Contains(string(completionJSON), `"label":"add"`) {
		t.Fatalf("expected add completion, got %s", completionJSON)
	}

	hover, _ := s.dispatch("textDocument/hover", mustRaw(posParams(uri, 1, 5)))
	hoverJSON, _ := json.Marshal(hover)
	if !strings.Contains(string(hoverJSON), "Declared at") || !strings.Contains(string(hoverJSON), "Current context") {
		t.Fatalf("expected contextual function hover, got %s", hoverJSON)
	}

	keywordHover, _ := s.dispatch("textDocument/hover", mustRaw(posParams(uri, 0, 1)))
	keywordJSON, _ := json.Marshal(keywordHover)
	if !strings.Contains(string(keywordJSON), "Syntax:") || !strings.Contains(string(keywordJSON), "Loads another SPL module") {
		t.Fatalf("expected keyword documentation hover, got %s", keywordJSON)
	}

	definition, _ := s.dispatch("textDocument/definition", mustRaw(posParams(uri, 2, 13)))
	defJSON, _ := json.Marshal(definition)
	if !strings.Contains(string(defJSON), pathToURI(mainPath)) {
		t.Fatalf("expected local definition, got %s", defJSON)
	}

	importDef, _ := s.dispatch("textDocument/definition", mustRaw(posParams(uri, 0, 10)))
	importJSON, _ := json.Marshal(importDef)
	if !strings.Contains(string(importJSON), pathToURI(libPath)) {
		t.Fatalf("expected import definition, got %s", importJSON)
	}

	refs, _ := s.dispatch("textDocument/references", mustRaw(posParams(uri, 1, 5)))
	refsJSON, _ := json.Marshal(refs)
	if strings.Count(string(refsJSON), pathToURI(mainPath)) < 2 {
		t.Fatalf("expected references in main file, got %s", refsJSON)
	}

	formatted, _ := s.dispatch("textDocument/formatting", mustRaw(map[string]any{"textDocument": map[string]any{"uri": uri}}))
	fmtJSON, _ := json.Marshal(formatted)
	if !strings.Contains(string(fmtJSON), "newText") {
		t.Fatalf("expected formatting edit, got %s", fmtJSON)
	}
}

func TestLSPSessionEvaluationCheckpointRestoreInspect(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.spl")
	uri := pathToURI(path)
	var out, errb bytes.Buffer
	s := newLSPServer(strings.NewReader(""), &out, &errb)
	s.dispatch("initialize", mustRaw(map[string]any{"rootUri": pathToURI(dir)}))
	s.dispatch("textDocument/didOpen", mustRaw(map[string]any{
		"textDocument": map[string]any{"uri": uri, "text": "let x = 1; x;"},
	}))

	evaluated, _ := s.dispatch("spl/evaluate", mustRaw(map[string]any{
		"uri":     uri,
		"text":    "let x = 1; x;",
		"options": map[string]any{"profile": "trusted"},
	}))
	evalJSON, _ := json.Marshal(evaluated)
	if !strings.Contains(string(evalJSON), `"ok":true`) || !strings.Contains(string(evalJSON), `"metrics"`) {
		t.Fatalf("expected session evaluation metrics, got %s", evalJSON)
	}

	checkpoint, _ := s.dispatch("spl/sessionCheckpoint", mustRaw(map[string]any{"uri": uri, "name": "base"}))
	checkpointJSON, _ := json.Marshal(checkpoint)
	if !strings.Contains(string(checkpointJSON), `"ok":true`) || !strings.Contains(string(checkpointJSON), "base") {
		t.Fatalf("expected checkpoint response, got %s", checkpointJSON)
	}

	s.dispatch("spl/evaluate", mustRaw(map[string]any{
		"uri":     uri,
		"text":    "x = 2; x;",
		"options": map[string]any{"profile": "trusted"},
	}))
	restored, _ := s.dispatch("spl/sessionRestore", mustRaw(map[string]any{"uri": uri, "name": "base"}))
	restoreJSON, _ := json.Marshal(restored)
	if !strings.Contains(string(restoreJSON), `"ok":true`) {
		t.Fatalf("expected restore response, got %s", restoreJSON)
	}

	inspect, _ := s.dispatch("spl/sessionInspect", mustRaw(map[string]any{"uri": uri}))
	inspectJSON, _ := json.Marshal(inspect)
	if !strings.Contains(string(inspectJSON), `"x":"1"`) {
		t.Fatalf("expected inspect to show restored variable, got %s", inspectJSON)
	}
}

func TestLSPHoverDocumentsRuntimeCallbacksParametersAndBlocks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scheduler.spl")
	src := "let job = schedule_interval(\"30s\", \"poll\", function(n) {\n  return n;\n});\n"
	uri := pathToURI(path)
	var out, errb bytes.Buffer
	s := newLSPServer(strings.NewReader(""), &out, &errb)
	s.dispatch("initialize", mustRaw(map[string]any{"rootUri": pathToURI(dir)}))
	s.dispatch("textDocument/didOpen", mustRaw(map[string]any{
		"textDocument": map[string]any{"uri": uri, "text": src},
	}))

	runtimeHover, _ := s.dispatch("textDocument/hover", mustRaw(posParams(uri, 0, 11)))
	runtimeJSON, _ := json.Marshal(runtimeHover)
	if !strings.Contains(string(runtimeJSON), "Registers a recurring interval job") || !strings.Contains(string(runtimeJSON), "Call context") {
		t.Fatalf("expected runtime API hover, got %s", runtimeJSON)
	}

	paramHover, _ := s.dispatch("textDocument/hover", mustRaw(posParams(uri, 1, 9)))
	paramJSON, _ := json.Marshal(paramHover)
	if !strings.Contains(string(paramJSON), "function parameter") || !strings.Contains(string(paramJSON), "scoped to the function body") {
		t.Fatalf("expected callback parameter hover, got %s", paramJSON)
	}

	blockHover, _ := s.dispatch("textDocument/hover", mustRaw(posParams(uri, 0, len([]rune(src[:strings.Index(src, "{")+1]))-1)))
	blockJSON, _ := json.Marshal(blockHover)
	if !strings.Contains(string(blockJSON), "block") || !strings.Contains(string(blockJSON), "Bindings declared inside") {
		t.Fatalf("expected block hover, got %s", blockJSON)
	}
}

func TestLSPCoversFirstPartyRuntimePackages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runtime.spl")
	src := strings.Join([]string{
		"let app = server();",
		"route(app, \"GET\", \"/health\", function(req, res) { return res.json({ ok: true }); });",
		"let job = schedule_interval(\"30s\", function() { return now(); });",
		"let count = signal(\"count\", 0);",
		"watch(\"./src\", \"*.spl\", function(event) { return event.path; });",
		"let formatted = time_format(time_now(), \"15:04:05\");",
	}, "\n")

	report := CheckSource(path, src)
	for _, diag := range report.Diagnostics {
		if diag.Code == "undefined" {
			t.Fatalf("expected first-party runtime builtin coverage, got undefined diagnostic: %#v", diag)
		}
	}

	items := WorkspaceCompletionItems(NewWorkspaceIndex(dir), path, src, "listen")
	foundListenAsync := false
	for _, item := range items {
		if item.Label == "listen_async" && item.Kind == "builtin" {
			foundListenAsync = true
			break
		}
	}
	if !foundListenAsync {
		t.Fatalf("expected web runtime completion for listen_async, got %#v", items)
	}
}

func TestLSPSemanticHoverExplainsMatchAndPrint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pattern_matching.spl")
	src := strings.Join([]string{
		"let x = 42",
		"let r1 = match (x) {",
		"    case 1 => \"one\"",
		"    case 42 => \"forty-two\"",
		"    case 100 => \"hundred\"",
		"}",
		"print(r1) // forty-two",
	}, "\n")
	uri := pathToURI(path)
	var out, errb bytes.Buffer
	s := newLSPServer(strings.NewReader(""), &out, &errb)
	s.dispatch("initialize", mustRaw(map[string]any{"rootUri": pathToURI(dir)}))
	s.dispatch("textDocument/didOpen", mustRaw(map[string]any{
		"textDocument": map[string]any{"uri": uri, "text": src},
	}))

	matchHover, _ := s.dispatch("textDocument/hover", mustRaw(posParams(uri, 1, 10)))
	matchJSON, _ := json.Marshal(matchHover)
	for _, want := range []string{"Syntax:", "Evaluation steps", "Subject `x` resolves to `42`", "Case 2 pattern `42` matches", "Match result is `forty-two`"} {
		if !strings.Contains(string(matchJSON), want) {
			t.Fatalf("expected match hover to contain %q, got %s", want, matchJSON)
		}
	}

	printHover, _ := s.dispatch("textDocument/hover", mustRaw(posParams(uri, 6, 1)))
	printJSON, _ := json.Marshal(printHover)
	for _, want := range []string{"Syntax:", "Current print", "Argument `r1` is evaluated to `forty-two`", "Output line: `forty-two`"} {
		if !strings.Contains(string(printJSON), want) {
			t.Fatalf("expected print hover to contain %q, got %s", want, printJSON)
		}
	}
}

func TestLSPSemanticHoverExplainsGrammarConstructs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grammar.spl")
	src := strings.Join([]string{
		"let x = 40 + 2",
		"const ready = x == 42",
		"if (ready) {",
		"  print(x)",
		"} else {",
		"  throw \"bad\"",
		"}",
		"function add(a, b) { return a + b; }",
		"for item in [1, 2, 3] { print(item); }",
	}, "\n")
	uri := pathToURI(path)
	var out, errb bytes.Buffer
	s := newLSPServer(strings.NewReader(""), &out, &errb)
	s.dispatch("initialize", mustRaw(map[string]any{"rootUri": pathToURI(dir)}))
	s.dispatch("textDocument/didOpen", mustRaw(map[string]any{
		"textDocument": map[string]any{"uri": uri, "text": src},
	}))

	cases := []struct {
		name string
		line int
		col  int
		want []string
	}{
		{"let", 0, 1, []string{"Current usage", "`x` is bound from `40 + 2`, currently inferred as `42`"}},
		{"const", 1, 1, []string{"`ready` is bound from `x == 42`, currently inferred as `true`", "immutable"}},
		{"if", 2, 1, []string{"Condition `ready` is evaluated for truthiness", "current inferred value is `true`"}},
		{"else", 4, 3, []string{"This branch runs only when the preceding `if` condition was falsey"}},
		{"throw", 5, 3, []string{"Raises", "current inferred value is `bad`"}},
		{"function", 7, 1, []string{"`add` defines callable code with 2 parameter(s)", "`a`, `b`"}},
		{"for", 8, 1, []string{"Iterates `[1, 2, 3]`", "binds `item`"}},
		{"identifier", 3, 9, []string{"Current inferred value: `42`"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hover, _ := s.dispatch("textDocument/hover", mustRaw(posParams(uri, tc.line, tc.col)))
			raw, _ := json.Marshal(hover)
			for _, want := range tc.want {
				if !strings.Contains(string(raw), want) {
					t.Fatalf("expected hover to contain %q, got %s", want, raw)
				}
			}
		})
	}
}

func TestVSCodeGrammarMentionsRuntimeBuiltinsAndOperators(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "vscode-extension", "syntaxes", "spl.tmLanguage.json"))
	if err != nil {
		t.Fatal(err)
	}
	grammar := string(data)
	for _, want := range []string{
		"schedule_interval",
		"server",
		"route_group",
		"signal",
		"db_query",
		"image_resize",
		"http_request",
		`\\?\\?=`,
		"<<=",
		`\\*\\*`,
	} {
		if !strings.Contains(grammar, want) {
			t.Fatalf("expected VS Code grammar to cover %q", want)
		}
	}
}

func TestLSPSafeEvaluationUsesUntrustedLimits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eval.spl")
	uri := pathToURI(path)
	var out, errb bytes.Buffer
	s := newLSPServer(strings.NewReader(""), &out, &errb)
	s.dispatch("initialize", mustRaw(map[string]any{"rootUri": pathToURI(dir)}))

	result, _ := s.dispatch("spl/evaluate", mustRaw(map[string]any{
		"uri":  uri,
		"text": `print "hello"; 40 + 2;`,
		"options": map[string]any{
			"profile":        "trusted",
			"timeoutMs":      500,
			"maxOutputBytes": 1024,
		},
	}))
	raw, _ := json.Marshal(result)
	if !strings.Contains(string(raw), `"ok":true`) || !strings.Contains(string(raw), "hello") || !strings.Contains(string(raw), "42") {
		t.Fatalf("unexpected evaluation result: %s", raw)
	}
}

func TestLSPNativeOSEvaluationUsesExplicitPolicy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "native.spl")
	uri := pathToURI(path)
	var out, errb bytes.Buffer
	s := newLSPServer(strings.NewReader(""), &out, &errb)
	s.dispatch("initialize", mustRaw(map[string]any{"rootUri": pathToURI(dir)}))

	result, _ := s.dispatch("spl/evaluate", mustRaw(map[string]any{
		"uri": uri,
		"text": `import "native/os" as os;
let result = os.run("echo", ["Hellow"]);
result["stdout"];`,
		"options": map[string]any{
			"profile":              "native",
			"timeoutMs":            500,
			"maxOutputBytes":       1024,
			"maxExecOutputBytes":   1024,
			"allowedExecCommands":  []string{"echo"},
			"allowedNativeModules": []string{"native/os"},
		},
	}))
	raw, _ := json.Marshal(result)
	if !strings.Contains(string(raw), `"ok":true`) || !strings.Contains(string(raw), "Hellow") {
		t.Fatalf("unexpected native evaluation result: %s", raw)
	}
}

func TestRunLSPInfoMentionsStdio(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"lsp"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("lsp info failed: %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "spltool lsp --stdio") {
		t.Fatalf("expected stdio hint, got %s", stdout.String())
	}
}

func posParams(uri string, line, character int) map[string]any {
	return map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": line, "character": character},
	}
}
