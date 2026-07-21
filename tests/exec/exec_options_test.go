package interpreter_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/security"
)

func TestExecWithOptionsUsesArgs(t *testing.T) {
	res, err := ExecWithOptions(`ARGS[0];`, nil, ExecOptions{Args: []string{"alpha"}})
	if err != nil {
		t.Fatalf("ExecWithOptions failed: %v", err)
	}
	str, ok := res.(*String)
	if !ok {
		t.Fatalf("expected String result, got %T", res)
	}
	if str.Value != "alpha" {
		t.Fatalf("unexpected ARGS[0]: got %q", str.Value)
	}
}

func TestExecWithOptionsRuntimeLimitError(t *testing.T) {
	_, err := ExecWithOptions(`while (true) { }`, nil, ExecOptions{MaxSteps: 1000})
	if err == nil {
		t.Fatalf("expected runtime error, got nil")
	}
	var execErr *ExecError
	if !errors.As(err, &execErr) {
		t.Fatalf("expected ExecError, got %T", err)
	}
	if execErr.Kind != ExecErrorResourceLimit {
		t.Fatalf("expected resource limit error kind, got %q", execErr.Kind)
	}
	if !strings.Contains(execErr.Message, "execution step limit exceeded") {
		t.Fatalf("unexpected runtime error message: %q", execErr.Message)
	}
}

func TestExecWithOptionsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	time.Sleep(2 * time.Millisecond)

	_, err := ExecWithOptions(`while (true) { }`, nil, ExecOptions{Context: ctx})
	if err == nil {
		t.Fatalf("expected cancellation error, got nil")
	}
	var execErr *ExecError
	if !errors.As(err, &execErr) {
		t.Fatalf("expected ExecError, got %T", err)
	}
	if execErr.Kind != ExecErrorTimeout {
		t.Fatalf("expected timeout error kind, got %q", execErr.Kind)
	}
	if !strings.Contains(execErr.Message, "execution cancelled") {
		t.Fatalf("unexpected cancellation message: %q", execErr.Message)
	}
}

func TestExecWithOptionsParserErrorKind(t *testing.T) {
	_, err := ExecWithOptions(`let x = ;`, nil, ExecOptions{})
	if err == nil {
		t.Fatalf("expected parser error, got nil")
	}
	var execErr *ExecError
	if !errors.As(err, &execErr) {
		t.Fatalf("expected ExecError, got %T", err)
	}
	if execErr.Kind != ExecErrorParser {
		t.Fatalf("expected parser error kind, got %q", execErr.Kind)
	}
	if len(execErr.Diagnostics) == 0 {
		t.Fatalf("expected parser diagnostics")
	}
}

func TestExecWithOptionsValidationError(t *testing.T) {
	_, err := ExecWithOptions(`1+1`, nil, ExecOptions{MaxSteps: -1})
	if err == nil {
		t.Fatalf("expected validation error, got nil")
	}
	var execErr *ExecError
	if !errors.As(err, &execErr) {
		t.Fatalf("expected ExecError, got %T", err)
	}
	if execErr.Kind != ExecErrorValidation {
		t.Fatalf("expected validation kind, got %q", execErr.Kind)
	}
}

func TestExecWithOptionsSecurityDenyExec(t *testing.T) {
	_, err := ExecWithOptions(`exec("echo", "hi")`, nil, ExecOptions{
		Security: &SecurityPolicy{StrictMode: true},
	})
	if err == nil {
		t.Fatalf("expected runtime policy error")
	}
	var execErr *ExecError
	if !errors.As(err, &execErr) {
		t.Fatalf("expected ExecError, got %T", err)
	}
	if execErr.Kind != ExecErrorPolicyDenied {
		t.Fatalf("expected policy denied kind, got %q", execErr.Kind)
	}
	if !strings.Contains(strings.ToLower(execErr.Message), "denied") {
		t.Fatalf("unexpected message: %q", execErr.Message)
	}
}

func TestRuntimeOnPolicyDeniedHookFires(t *testing.T) {
	// security.DenialHook is a single process-wide variable (see Feature 2's
	// documented limitation in runtime.go's NewRuntime). Reset it after this
	// test so it can't leak into unrelated tests that run later in the same
	// process.
	t.Cleanup(func() { security.DenialHook = nil })

	var mu sync.Mutex
	var categories []string
	rt, err := NewRuntime(RuntimeOptions{
		Profile: "trusted",
		Security: &SecurityPolicy{
			StrictMode: true,
		},
		Observability: &ObservabilityHooks{
			OnPolicyDenied: func(category, detail string) {
				mu.Lock()
				categories = append(categories, category)
				mu.Unlock()
			},
		},
	})
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	if _, err := rt.Exec(`exec("echo", "hi")`, nil); err == nil {
		t.Fatalf("expected exec to be denied")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(categories) == 0 {
		t.Fatalf("expected OnPolicyDenied to be invoked at least once")
	}
	found := false
	for _, c := range categories {
		if c == "exec" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an %q denial category, got %v", "exec", categories)
	}
}

func TestExecWithOptionsMaxDepthIsResourceLimit(t *testing.T) {
	script := `function rec(n) { return rec(n + 1); } rec(0);`
	_, err := ExecWithOptions(script, nil, ExecOptions{MaxDepth: 5})
	if err == nil {
		t.Fatalf("expected recursion depth error, got nil")
	}
	var execErr *ExecError
	if !errors.As(err, &execErr) {
		t.Fatalf("expected ExecError, got %T", err)
	}
	if execErr.Kind != ExecErrorResourceLimit {
		t.Fatalf("expected resource limit error kind, got %q", execErr.Kind)
	}
	if !strings.Contains(execErr.Message, "recursion depth exceeded") {
		t.Fatalf("unexpected message: %q", execErr.Message)
	}
}
