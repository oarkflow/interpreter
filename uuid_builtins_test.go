package interpreter

import (
	"regexp"
	"strings"
	"testing"
)

func TestUUIDBuiltinDefaultsToV7(t *testing.T) {
	obj := testEval(`uuid();`)
	s, ok := obj.(*String)
	if !ok {
		t.Fatalf("expected STRING result, got %T", obj)
	}

	uuidV7Re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidV7Re.MatchString(s.Value) {
		t.Fatalf("invalid default uuid(v7) format: %q", s.Value)
	}

	if !strings.Contains(builtinHelpText("uuid"), "default version is 7") {
		t.Fatalf("expected help details for uuid")
	}
}

func TestUUIDBuiltinVersionSelection(t *testing.T) {
	v4 := testEval(`uuid(4);`)
	s4, ok := v4.(*String)
	if !ok {
		t.Fatalf("expected STRING result, got %T", v4)
	}
	uuidV4Re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidV4Re.MatchString(s4.Value) {
		t.Fatalf("invalid uuid(4) format: %q", s4.Value)
	}

	v7 := testEval(`uuid("7");`)
	s7, ok := v7.(*String)
	if !ok {
		t.Fatalf("expected STRING result, got %T", v7)
	}
	uuidV7Re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidV7Re.MatchString(s7.Value) {
		t.Fatalf("invalid uuid(7) format: %q", s7.Value)
	}
}

func TestUUIDBuiltinArgValidation(t *testing.T) {
	obj := testEval(`uuid(1);`)
	errStr, ok := obj.(*String)
	if !ok {
		t.Fatalf("expected STRING error, got %T", obj)
	}
	if !strings.Contains(errStr.Value, "unsupported uuid version") {
		t.Fatalf("unexpected error: %q", errStr.Value)
	}
}
