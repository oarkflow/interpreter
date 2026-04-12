package interpreter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
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
	if execErr.Kind != ExecErrorRuntime {
		t.Fatalf("expected runtime error kind, got %q", execErr.Kind)
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
	if execErr.Kind != ExecErrorRuntime {
		t.Fatalf("expected runtime error kind, got %q", execErr.Kind)
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
	if execErr.Kind != ExecErrorRuntime {
		t.Fatalf("expected runtime kind, got %q", execErr.Kind)
	}
	if !strings.Contains(strings.ToLower(execErr.Message), "denied") {
		t.Fatalf("unexpected message: %q", execErr.Message)
	}
}
