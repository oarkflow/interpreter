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
