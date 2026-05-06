package interpreter_test

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	. "github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/repl"

	_ "github.com/oarkflow/interpreter/pkg/builtins"
)

func captureReplStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = orig
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	return buf.String()
}

func TestReplCallTipUsesCompactFunctionSignature(t *testing.T) {
	env := NewGlobalEnvironment(nil)
	env.Set("testType", &Function{
		Name: "testType",
		Parameters: []*ast.Identifier{
			{Name: "val"},
		},
		ParamTypes: []string{"any"},
		ReturnType: "string",
	})

	tip := repl.ReplCallTip("testType(", len("testType("), env)
	if tip != "testType(val: any) -> string" {
		t.Fatalf("unexpected call tip: %q", tip)
	}
	if strings.Contains(tip, "{") || strings.Contains(tip, "\n") || strings.Contains(tip, "return") {
		t.Fatalf("call tip leaked function body formatting: %q", tip)
	}
}

func TestReplHintLinesWrapAndCompact(t *testing.T) {
	lines := repl.ReplHintLines("function(val) {\n  return match (val) { case n: integer => { n } case _ => { 0 } };\n}", 60)
	if len(lines) == 0 || len(lines) > 2 {
		t.Fatalf("expected one or two helper lines, got %#v", lines)
	}
	for _, line := range lines {
		if strings.Contains(line, "\n") {
			t.Fatalf("hint line contains newline: %q", line)
		}
		if len(line) > 100 {
			t.Fatalf("hint line too long: %q", line)
		}
	}
}

func TestReplConfigListAndSetRuntime(t *testing.T) {
	env := NewGlobalEnvironment(nil)
	out := captureReplStdout(t, func() {
		if !repl.HandleReplMetaCommand(":config", nil, env) {
			t.Fatalf(":config was not handled")
		}
	})
	if !strings.Contains(out, "Key") || !strings.Contains(out, "execution.profile") || !strings.Contains(out, "runtime.max_steps") {
		t.Fatalf("expected tabular config output, got %q", out)
	}

	out = captureReplStdout(t, func() {
		if !repl.HandleReplMetaCommand(":config set runtime.max_steps 1234", nil, env) {
			t.Fatalf(":config set was not handled")
		}
	})
	if !strings.Contains(out, "runtime.max_steps = 1234") {
		t.Fatalf("unexpected set output: %q", out)
	}
	if env.RuntimeLimits == nil || env.RuntimeLimits.MaxSteps != 1234 {
		t.Fatalf("expected runtime max steps to be applied, got %#v", env.RuntimeLimits)
	}
}

func TestReplConfigUntrustedProfileDeniesExec(t *testing.T) {
	env := NewGlobalEnvironment(nil)
	out := captureReplStdout(t, func() {
		if !repl.HandleReplMetaCommand(":config profile untrusted", nil, env) {
			t.Fatalf(":config profile was not handled")
		}
	})
	if !strings.Contains(out, "execution.profile = untrusted") {
		t.Fatalf("unexpected profile output: %q", out)
	}
	if env.SecurityPolicy == nil || !env.SecurityPolicy.StrictMode || !env.SecurityPolicy.ProtectHost {
		t.Fatalf("expected strict host-protected policy, got %#v", env.SecurityPolicy)
	}

	out = captureReplStdout(t, func() {
		repl.ReplEvalSource(`exec("echo", "hi")`, env, "<repl>", true)
	})
	if !strings.Contains(strings.ToLower(out), "exec denied") {
		t.Fatalf("expected exec denial, got %q", out)
	}
}
