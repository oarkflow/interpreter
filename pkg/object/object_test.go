package object

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRuntimeLimitsCloneForConcurrentExecution(t *testing.T) {
	ctx := context.Background()
	deadline := time.Now().Add(time.Minute)
	original := &RuntimeLimits{
		MaxDepth: 12, MaxSteps: 100, Steps: 42, CurrentDepth: 3,
		Deadline: deadline, Ctx: ctx, MaxOutputBytes: 2048, OutputBytes: 512,
		MaxImportDepth: 4, CurrentImportDepth: 2, MaxImportCount: 8, ImportCount: 5,
	}

	clone := original.CloneForConcurrentExecution()
	if clone == original {
		t.Fatal("clone must have independent counters")
	}
	if clone.MaxDepth != original.MaxDepth || clone.MaxSteps != original.MaxSteps ||
		clone.Deadline != deadline || clone.Ctx != ctx || clone.MaxOutputBytes != original.MaxOutputBytes ||
		clone.MaxImportDepth != original.MaxImportDepth || clone.MaxImportCount != original.MaxImportCount {
		t.Fatalf("configured limits were not preserved: %#v", clone)
	}
	if clone.Steps != 0 || clone.CurrentDepth != 0 || clone.OutputBytes != 0 ||
		clone.CurrentImportDepth != 0 || clone.ImportCount != 0 {
		t.Fatalf("execution counters were not reset: %#v", clone)
	}
}

func TestFormatPlainFormatsNestedArraysAndHashes(t *testing.T) {
	statusKey := &String{Value: "status"}
	statusVal := &String{Value: "planned"}
	opKey := &String{Value: "op"}
	opVal := &String{Value: "rename"}
	h := &Hash{Pairs: map[HashKey]HashPair{
		statusKey.HashKey(): {Key: statusKey, Value: statusVal},
		opKey.HashKey():     {Key: opKey, Value: opVal},
	}}
	out := FormatPlain(&Array{Elements: []Object{h}})
	for _, want := range []string{"[\n", "  {\n", `op: "rename"`, `status: "planned"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in formatted output:\n%s", want, out)
		}
	}
}

func TestFormatPlainEscapesMultilineStringsInsideStructuredValues(t *testing.T) {
	bodyKey := &String{Value: "body"}
	obj := &Array{Elements: []Object{
		&Hash{Pairs: map[HashKey]HashPair{
			bodyKey.HashKey(): {
				Key:   bodyKey,
				Value: &String{Value: "first\nsecond"},
			},
		}},
	}}

	got := FormatPlain(obj)
	if strings.Contains(got, "first\nsecond") {
		t.Fatalf("expected embedded newline to be escaped, got %q", got)
	}
	if !strings.Contains(got, `body: "first\nsecond"`) {
		t.Fatalf("expected quoted escaped string field, got %q", got)
	}
}

func TestEnvironmentSealedBindingsRemainReadableAndRejectMutation(t *testing.T) {
	env := NewEnvironment()
	env.Set("answer", IntegerObj(42))
	env.SealBindings()

	value, ok := env.Get("answer")
	if !ok || value.(*Integer).Value != 42 || !env.BindingsSealed() {
		t.Fatalf("sealed binding was not readable: %v, %v", value, ok)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("expected mutation of sealed bindings to panic")
		}
	}()
	env.Set("answer", IntegerObj(7))
}

func TestEnvironmentResetUnsealsBindings(t *testing.T) {
	env := NewEnvironment()
	env.Set("answer", IntegerObj(42))
	env.SealBindings()
	env.Reset()
	env.Set("answer", IntegerObj(7))
	if env.BindingsSealed() {
		t.Fatal("reset environment remained sealed")
	}
	value, _ := env.Get("answer")
	if value.(*Integer).Value != 7 {
		t.Fatalf("unexpected value after reset: %v", value)
	}
}
