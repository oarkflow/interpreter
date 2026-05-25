package tooling

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckSourceProducesStructuredDiagnostics(t *testing.T) {
	report := CheckSource("sample.spl", "let x = ;")
	if report.OK {
		t.Fatalf("expected check to fail")
	}
	if len(report.Diagnostics) == 0 {
		t.Fatalf("expected diagnostics")
	}
	diag := report.Diagnostics[0]
	if diag.Path != "sample.spl" {
		t.Fatalf("unexpected path: %q", diag.Path)
	}
	if diag.Line == 0 {
		t.Fatalf("expected line information")
	}
	if diag.Message == "" {
		t.Fatalf("expected diagnostic message")
	}
	if diag.Snippet == "" {
		t.Fatalf("expected snippet")
	}

	raw, err := json.Marshal(report.Diagnostics)
	if err != nil {
		t.Fatalf("marshal diagnostics: %v", err)
	}
	if !strings.Contains(string(raw), "\"severity\":\"error\"") {
		t.Fatalf("expected machine-readable severity, got %s", raw)
	}
}

func TestParsePartialReturnsDiagnosticsAndProgram(t *testing.T) {
	result := ParsePartial("partial.spl", "let x = ;\nlet y = 2;")
	if result.Complete {
		t.Fatalf("expected incomplete parse")
	}
	if result.Program == nil {
		t.Fatalf("expected best-effort program")
	}
	if len(result.Diagnostics) == 0 {
		t.Fatalf("expected diagnostics")
	}
}

func TestFormatSourceReturnsCanonicalText(t *testing.T) {
	report := FormatSource("sample.spl", "let x=1; let y=2; if (x) { y; }")
	if !report.OK {
		t.Fatalf("expected format to succeed: %#v", report.Diagnostics)
	}
	if report.Formatted == "" {
		t.Fatalf("expected formatted source")
	}
	if !strings.Contains(report.Formatted, "\n") {
		t.Fatalf("expected multiline formatted output, got %q", report.Formatted)
	}
	if !report.Changed {
		t.Fatalf("expected report to mark source as changed")
	}
}

func TestCheckSourceStaticWarnings(t *testing.T) {
	report := CheckSource("sample.spl", `
let x = 1;
let x = 2;
print missingName;
function f() { return 1; print "dead"; }
type Maybe = Present(value) | Absent();
let maybe = Present(1);
match (maybe) { case Present(n) => { n; } }
`)
	if !report.OK {
		t.Fatalf("warnings should not fail check: %#v", report.Diagnostics)
	}
	joined := ""
	for _, d := range report.Diagnostics {
		joined += d.Code + " " + d.Message + "\n"
	}
	for _, want := range []string{"shadow", "undefined", "unreachable", "match-exhaustiveness"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %s warning in diagnostics:\n%s", want, joined)
		}
	}
}

func TestCheckSourceIgnoresCommentedCodeForStaticWarnings(t *testing.T) {
	src := `
/*
let n = 1;
let n = 2;
let greetPerson = function(name) { return name; };
let greetPerson = function(other) { return other; };
print missingName;
*/
// let sameLine = 1; let sameLine = 2; print alsoMissing;
let ok = 1;
print ok;
`
	report := CheckSource("sample.spl", src)
	if !report.OK {
		t.Fatalf("comments should not produce parse errors: %#v", report.Diagnostics)
	}
	for _, diag := range report.Diagnostics {
		if diag.Code == "shadow" || diag.Code == "undefined" {
			t.Fatalf("commented code should not be validated, got diagnostic: %#v", diag)
		}
	}
}

func TestCheckSourceWarningLocationsSkipComments(t *testing.T) {
	src := `
// let greetPerson = 1;
let greetPerson = function(name) { return name; };
let greetPerson = function(other) { return other; };
`
	report := CheckSource("sample.spl", src)
	found := false
	for _, diag := range report.Diagnostics {
		if diag.Code == "shadow" && strings.Contains(diag.Message, "greetPerson") {
			found = true
			if strings.Contains(diag.Snippet, "//") || diag.Line == 2 {
				t.Fatalf("warning location should point to code, not comment: %#v", diag)
			}
		}
	}
	if !found {
		t.Fatalf("expected duplicate declaration warning, got %#v", report.Diagnostics)
	}
}

func TestCheckSourceAllowsShortForInLoopBindings(t *testing.T) {
	src := `
let n = 10;
let nums = [1, 2, 3];
for (n in nums) { print n; }
`
	report := CheckSource("sample.spl", src)
	for _, diag := range report.Diagnostics {
		if diag.Code == "shadow" && strings.Contains(diag.Message, `"n"`) {
			t.Fatalf("for-in loop binding should not warn as noisy shadowing: %#v", diag)
		}
	}
}

func TestCheckSourceNamedFunctionDeclarationIsSingleBinding(t *testing.T) {
	src := `
function greetPerson(name) {
	return "Hello, " + name;
}
print greetPerson("World");
`
	report := CheckSource("sample.spl", src)
	for _, diag := range report.Diagnostics {
		if diag.Code == "shadow" && strings.Contains(diag.Message, "greetPerson") {
			t.Fatalf("named function declaration should not be reported as duplicate: %#v", diag)
		}
	}
}

func TestCheckSourceAvoidsKnownFalsePositiveWarnings(t *testing.T) {
	src := `
let tiny = "data:image/png;base64,abc";
let original = image_load(tiny);
let compact = table_select([], ["name"]);
let x = 42;
let classified = match (x) {
	case value => value
};
type Result = Ok(value) | Err(message);
let res = Ok(42);
let out = match (res) {
	case Ok(v) => v
	case Err(e) => e
};
`
	report := CheckSource("sample.spl", src)
	for _, diag := range report.Diagnostics {
		if diag.Code == "undefined" && strings.Contains(diag.Message, "image_load") {
			t.Fatalf("image_load should be recognized as a documented builtin: %#v", diag)
		}
		if diag.Code == "shadow" && strings.Contains(diag.Message, "compact") {
			t.Fatalf("declaring compact should not conflict with the builtin seed: %#v", diag)
		}
		if diag.Code == "shadow" && strings.Contains(diag.Message, "value") {
			t.Fatalf("match binding should not warn as shadowing/noisy duplicate: %#v", diag)
		}
		if diag.Code == "match-exhaustiveness" {
			t.Fatalf("binding fallback and covered ADT variants should not warn as non-exhaustive: %#v", diag)
		}
	}
}

func TestCheckSourceAcceptsStdManifestAndModulePathImports(t *testing.T) {
	dir := t.TempDir()
	depDir := filepath.Join(dir, "deps", "showpkg")
	modulePathDir := filepath.Join(dir, "module-path")
	if err := os.MkdirAll(depDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(modulePathDir, "extras"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "math.spl"), []byte("export let answer = 42;"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modulePathDir, "extras", "util.spl"), []byte("export let util = 1;"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := `{"module":"sample","dependencies":{"showpkg":"./deps/showpkg"}}`
	if err := os.WriteFile(filepath.Join(dir, "spl.mod"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SPL_MODULE_PATH", modulePathDir)

	src := strings.Join([]string{
		`import "std/core" as core;`,
		`import "showpkg/math.spl" as math;`,
		`import "extras/util.spl" as util;`,
		`print core.sprintf("%d", math.answer + util.util);`,
	}, "\n")
	report := CheckSource(filepath.Join(dir, "main.spl"), src)
	for _, diag := range report.Diagnostics {
		if diag.Code == "missing-import" {
			t.Fatalf("expected import to resolve, got diagnostic: %#v", diag)
		}
	}
}

func TestCheckSourceResolvesBareImportExports(t *testing.T) {
	dir := t.TempDir()
	commonPath := filepath.Join(dir, "common.spl")
	mathPath := filepath.Join(dir, "math.spl")
	if err := os.WriteFile(commonPath, []byte("export const offset = 2;"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := `import "common.spl";

export let answer = 40 + offset;
`
	if err := os.WriteFile(mathPath, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	report := CheckSource(mathPath, src)
	for _, diag := range report.Diagnostics {
		if diag.Code == "undefined" && strings.Contains(diag.Message, "offset") {
			t.Fatalf("bare import should declare exported offset, got diagnostic: %#v", diag)
		}
	}
}

func TestCheckSourceResolvesNamedImportExports(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "common.spl"), []byte("export const offset = 2;"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := `import {offset} from "common.spl";
let answer = 40 + offset;
`
	report := CheckSource(filepath.Join(dir, "math.spl"), src)
	for _, diag := range report.Diagnostics {
		if diag.Code == "undefined" && strings.Contains(diag.Message, "offset") {
			t.Fatalf("named import should declare exported offset, got diagnostic: %#v", diag)
		}
		if diag.Code == "missing-import" {
			t.Fatalf("named import should resolve existing export, got diagnostic: %#v", diag)
		}
	}
}

func TestImportedExportsAreVisibleToEditorFeatures(t *testing.T) {
	dir := t.TempDir()
	commonPath := filepath.Join(dir, "common.spl")
	mathPath := filepath.Join(dir, "math.spl")
	if err := os.WriteFile(commonPath, []byte("export const offset = 2;"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := `import "common.spl";
let answer = 40 + offset;
`
	if err := os.WriteFile(mathPath, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	items := CompletionItems(mathPath, src, "off")
	if len(items) == 0 || items[0].Label != "offset" {
		t.Fatalf("expected imported offset completion, got %#v", items)
	}
	hover := HoverAt(mathPath, src, 2, 19)
	if hover.Name != "offset" || hover.Kind != "variable" {
		t.Fatalf("expected hover for imported offset, got %#v", hover)
	}
	idx := NewWorkspaceIndex(dir)
	loc, ok := idx.Definition(mathPath, src, 2, 19)
	if !ok || loc.Path != commonPath || loc.Name != "offset" {
		t.Fatalf("expected definition in common.spl, got ok=%v loc=%#v", ok, loc)
	}
}

func TestSymbolsCompletionHoverAndDocs(t *testing.T) {
	src := `
let add = function(a: integer, b: integer): integer { return a + b; };
type Result = Ok(value) | Err(message);
test "adds" { assert_eq(add(1, 2), 3); }
`
	symbols := SymbolsForSource("sample.spl", src)
	names := []string{}
	for _, sym := range symbols {
		names = append(names, sym.Name+":"+sym.Kind)
	}
	joined := strings.Join(names, " ")
	if !strings.Contains(joined, "add:function") || !strings.Contains(joined, "Result:type") || !strings.Contains(joined, "adds:test") {
		t.Fatalf("unexpected symbols: %#v", symbols)
	}
	completions := CompletionItems("sample.spl", src, "ad")
	if len(completions) == 0 || completions[0].Label != "add" {
		t.Fatalf("expected add completion, got %#v", completions)
	}
	hover := HoverAt("sample.spl", src, 2, 6)
	if hover.Name != "add" || hover.Kind != "function" {
		t.Fatalf("unexpected hover: %#v", hover)
	}
	docs := DocsMarkdown("sample.spl", src)
	if !strings.Contains(docs, "# sample.spl") || !strings.Contains(docs, "`add`") {
		t.Fatalf("unexpected docs:\n%s", docs)
	}
}
