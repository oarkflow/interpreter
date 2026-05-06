package main

import (
	"encoding/json"
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
match (x) { case n: integer => { n; } }
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
