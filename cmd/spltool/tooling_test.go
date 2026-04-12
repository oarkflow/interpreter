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
