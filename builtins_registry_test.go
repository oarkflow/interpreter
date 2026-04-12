package interpreter

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestRegisterBuiltins_DuplicateRegistrationWarnsAndSkips(t *testing.T) {
	builtinName := "__test_duplicate_builtin__"
	original, hadOriginal := builtins[builtinName]

	first := &Builtin{Fn: func(args ...Object) Object { return &Null{} }}
	second := &Builtin{Fn: func(args ...Object) Object { return &Integer{Value: 1} }}

	builtins[builtinName] = first
	defer func() {
		if hadOriginal {
			builtins[builtinName] = original
			return
		}
		delete(builtins, builtinName)
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

	registerBuiltins(map[string]*Builtin{builtinName: second})

	_ = w.Close()
	warningBytes, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read warning output: %v", err)
	}

	if builtins[builtinName] != first {
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
