package interpreter

import "testing"

func TestSprintfBuiltin(t *testing.T) {
	result := testEval(`sprintf("name=%s n=%d ok=%t type=%T val=%v", "spl", 7, true, 3.14, {"a": 1});`)
	s, ok := result.(*String)
	if !ok {
		t.Fatalf("expected STRING result, got %T", result)
	}
	if s.Value == "" {
		t.Fatalf("expected non-empty formatted string")
	}
}

func TestSprintfErrorsOnArgMismatch(t *testing.T) {
	result := testEval(`sprintf("%s %d", "only-one");`)
	if !isError(result) {
		t.Fatalf("expected error result, got %T", result)
	}
}

func TestInterpolateBuiltinWithHashAndPositional(t *testing.T) {
	result := testEval(`interpolate("Hello {name} #{count}", {"name":"SPL", "count": 3});`)
	s, ok := result.(*String)
	if !ok {
		t.Fatalf("expected STRING result, got %T", result)
	}
	if s.Value != "Hello SPL #3" {
		t.Fatalf("unexpected interpolation result: %q", s.Value)
	}

	result = testEval(`interpolate("{0} + {1} = {2}", null, 20, 22, 42);`)
	s, ok = result.(*String)
	if !ok {
		t.Fatalf("expected STRING result, got %T", result)
	}
	if s.Value != "20 + 22 = 42" {
		t.Fatalf("unexpected positional interpolation result: %q", s.Value)
	}
}
