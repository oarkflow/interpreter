package eval_test

import (
	"strings"
	"testing"

	"github.com/oarkflow/interpreter/pkg/object"
)

func TestHelpBuiltinListsBuiltins(t *testing.T) {
	obj := testEval(`help()`)
	arr, ok := obj.(*object.Array)
	if !ok {
		t.Fatalf("expected ARRAY, got %T", obj)
	}
	if len(arr.Elements) == 0 {
		t.Fatal("help() returned empty builtin list")
	}

	if arr.Elements[0].Type() != object.STRING_OBJ {
		t.Fatalf("expected STRING elements, got %s", arr.Elements[0].Type())
	}

	foundHelp := false
	foundExec := false
	for _, el := range arr.Elements {
		name, ok := el.(*object.String)
		if !ok {
			t.Fatalf("expected STRING element, got %T", el)
		}
		if name.Value == "help" {
			foundHelp = true
		}
		if name.Value == "exec" {
			foundExec = true
		}
	}
	if !foundHelp || !foundExec {
		t.Fatalf("help() missing expected builtins: help=%v exec=%v", foundHelp, foundExec)
	}
}

func TestHelpBuiltinDetails(t *testing.T) {
	detail := testEval(`help("exec")`)
	str, ok := detail.(*object.String)
	if !ok {
		t.Fatalf("expected STRING, got %T", detail)
	}
	if !strings.Contains(str.Value, "exec(") {
		t.Fatalf("unexpected help(\"exec\") details: %q", str.Value)
	}
}

func TestHelpBuiltinUnknown(t *testing.T) {
	obj := testEval(`help("__missing_builtin__")`)
	str, ok := obj.(*object.String)
	if !ok {
		t.Fatalf("expected STRING, got %T", obj)
	}
	if !strings.Contains(str.Value, "not found") {
		t.Fatalf("unexpected error: %q", str.Value)
	}
}

func TestExecBuiltinDisabledByEnv(t *testing.T) {
	t.Setenv("SPL_DISABLE_EXEC", "1")
	obj := testEval(`exec("echo", "hello")`)
	str, ok := obj.(*object.String)
	if !ok {
		t.Fatalf("expected STRING, got %T", obj)
	}
	if !strings.Contains(str.Value, "disabled") {
		t.Fatalf("unexpected exec disabled response: %q", str.Value)
	}
}

func TestExecBuiltinDeniedByPolicy(t *testing.T) {
	t.Setenv("SPL_DISABLE_EXEC", "0")
	t.Setenv("SPL_EXEC_DENY_CMDS", "echo")
	obj := testEval(`exec("echo", "hello")`)
	str, ok := obj.(*object.String)
	if !ok {
		t.Fatalf("expected STRING, got %T", obj)
	}
	if !strings.Contains(strings.ToLower(str.Value), "denied") {
		t.Fatalf("unexpected policy deny response: %q", str.Value)
	}
}

func TestExecBuiltinTimeoutArg(t *testing.T) {
	t.Setenv("SPL_DISABLE_EXEC", "0")
	obj := testEval(`exec("tail", "-f", "/dev/null", 100)`)
	str, ok := obj.(*object.String)
	if !ok {
		t.Fatalf("expected STRING, got %T", obj)
	}
	if !strings.Contains(str.Value, "timed out") {
		t.Fatalf("unexpected timeout response: %q", str.Value)
	}
}

func TestExecBuiltinTimeoutFromEnv(t *testing.T) {
	t.Setenv("SPL_DISABLE_EXEC", "0")
	t.Setenv("SPL_EXEC_TIMEOUT_MS", "100")
	obj := testEval(`exec("tail", "-f", "/dev/null")`)
	str, ok := obj.(*object.String)
	if !ok {
		t.Fatalf("expected STRING, got %T", obj)
	}
	if !strings.Contains(str.Value, "timed out") {
		t.Fatalf("unexpected timeout response: %q", str.Value)
	}
}
