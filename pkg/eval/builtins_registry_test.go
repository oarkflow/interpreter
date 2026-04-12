package eval_test

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

func TestRegisterBuiltins_DuplicateRegistrationWarnsAndSkips(t *testing.T) {
	builtinName := "__test_duplicate_builtin__"
	original, hadOriginal := eval.Builtins[builtinName]

	first := &object.Builtin{Fn: func(args ...object.Object) object.Object { return &object.Null{} }}
	second := &object.Builtin{Fn: func(args ...object.Object) object.Object { return &object.Integer{Value: 1} }}

	eval.Builtins[builtinName] = first
	defer func() {
		if hadOriginal {
			eval.Builtins[builtinName] = original
			return
		}
		delete(eval.Builtins, builtinName)
	}()

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	os.Stderr = w
	defer func() {
		os.Stderr = oldStderr
	}()

	eval.RegisterBuiltins(map[string]*object.Builtin{builtinName: second})

	_ = w.Close()
	warningBytes, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read warning output: %v", err)
	}

	if eval.Builtins[builtinName] != first {
		t.Fatalf("expected duplicate registration to keep original builtin")
	}

	warning := string(warningBytes)
	if !strings.Contains(warning, "warning: builtin \""+builtinName+"\" already exists") {
		t.Fatalf("expected warning for duplicate builtin, got %q", warning)
	}
	if !strings.Contains(warning, "skipping duplicate registration") {
		t.Fatalf("expected duplicate skip message, got %q", warning)
	}
}
