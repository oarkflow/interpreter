package interpreter_test

import (
	"strings"
	"testing"

	. "github.com/oarkflow/interpreter"
)

func TestEvalForPlaygroundCapturesOutputAndResult(t *testing.T) {
	res := EvalForPlayground(`print "hello"; 40 + 2;`, PlaygroundOptions{TimeoutMS: 2000})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if !strings.Contains(res.Output, "hello") {
		t.Fatalf("expected print output, got %q", res.Output)
	}
	if res.Result != "42" {
		t.Fatalf("expected result 42, got %q", res.Result)
	}
	if res.ResultTy != "INTEGER" {
		t.Fatalf("expected INTEGER result type, got %q", res.ResultTy)
	}
}

func TestEvalForPlaygroundReturnsParserErrors(t *testing.T) {
	res := EvalForPlayground(`let x = ;`, PlaygroundOptions{TimeoutMS: 2000})
	if res.Error == "" {
		t.Fatalf("expected parser error")
	}
	if !strings.Contains(res.Error, "Parser error(s):") {
		t.Fatalf("expected friendly parser heading, got %q", res.Error)
	}
	if !strings.Contains(res.Error, "Line") {
		t.Fatalf("expected line details in error, got %q", res.Error)
	}
	if !strings.Contains(res.Error, "Source:") {
		t.Fatalf("expected source context in error, got %q", res.Error)
	}
	if res.ErrorKind != "parser" {
		t.Fatalf("expected parser error kind, got %q", res.ErrorKind)
	}
	if len(res.Diagnostics) == 0 {
		t.Fatalf("expected diagnostics to be populated")
	}
}
