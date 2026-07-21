package wuid

import (
	"testing"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

func lookupBuiltin(t *testing.T, name string) *object.Builtin {
	t.Helper()
	b, ok := eval.BuiltinByName(name)
	if !ok {
		t.Fatalf("expected builtin %q to be registered", name)
	}
	return b
}

func TestWuidBuiltinsRegisterOnImport(t *testing.T) {
	for _, name := range []string{"wuid_new", "wuid_new_uuid", "wuid_parse"} {
		if !eval.HasBuiltin(name) {
			t.Fatalf("expected wuid builtin %q to be registered", name)
		}
	}
}

func TestWuidNewParseRoundTrip(t *testing.T) {
	newFn := lookupBuiltin(t, "wuid_new")
	parseFn := lookupBuiltin(t, "wuid_parse")

	idObj := newFn.Fn()
	id, ok := idObj.(*object.String)
	if !ok || id.Value == "" {
		t.Fatalf("wuid_new returned non-string or empty: %#v", idObj)
	}

	result := parseFn.Fn(id)
	arr, ok := result.(*object.Array)
	if !ok || len(arr.Elements) != 2 {
		t.Fatalf("wuid_parse returned unexpected shape: %#v", result)
	}
	if arr.Elements[1] != object.NULL {
		t.Fatalf("expected nil err parsing a freshly generated id, got %#v", arr.Elements[1])
	}
	h, ok := arr.Elements[0].(*object.Hash)
	if !ok {
		t.Fatalf("expected result HASH, got %#v", arr.Elements[0])
	}
	idKey := (&object.String{Value: "id"}).HashKey()
	pair, ok := h.Pairs[idKey]
	if !ok {
		t.Fatalf("result missing `id` field")
	}
	roundTripped, ok := pair.Value.(*object.String)
	if !ok || roundTripped.Value != id.Value {
		t.Fatalf("expected round-tripped id %q, got %#v", id.Value, pair.Value)
	}
}

func TestWuidParseRejectsGarbage(t *testing.T) {
	parseFn := lookupBuiltin(t, "wuid_parse")
	result := parseFn.Fn(&object.String{Value: "not-a-valid-id"})
	arr, ok := result.(*object.Array)
	if !ok || len(arr.Elements) != 2 {
		t.Fatalf("wuid_parse returned unexpected shape: %#v", result)
	}
	if arr.Elements[0] != object.NULL {
		t.Fatalf("expected nil result for garbage input, got %#v", arr.Elements[0])
	}
	if arr.Elements[1] == object.NULL {
		t.Fatalf("expected non-nil err for garbage input")
	}
}
