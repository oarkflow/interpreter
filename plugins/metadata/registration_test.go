package metadata

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

func TestMetadataBuiltinsRegisterOnImport(t *testing.T) {
	for _, name := range []string{"infer_csv_types", "infer_json_types", "infer_value_type"} {
		if !eval.HasBuiltin(name) {
			t.Fatalf("expected metadata builtin %q to be registered", name)
		}
	}
}

func hashFieldValue(t *testing.T, h *object.Hash, key string) object.Object {
	t.Helper()
	k := (&object.String{Value: key}).HashKey()
	pair, ok := h.Pairs[k]
	if !ok {
		t.Fatalf("hash missing field %q", key)
	}
	return pair.Value
}

func TestInferCSVTypes(t *testing.T) {
	fn := lookupBuiltin(t, "infer_csv_types")
	csv := "id,name,active,joined\n1,Ada,true,2020-01-15\n2,Grace,false,2021-06-30\n"
	result := fn.Fn(&object.String{Value: csv})
	arr, ok := result.(*object.Array)
	if !ok || len(arr.Elements) != 2 {
		t.Fatalf("infer_csv_types returned unexpected shape: %#v", result)
	}
	if arr.Elements[1] != object.NULL {
		t.Fatalf("infer_csv_types returned err: %#v", arr.Elements[1])
	}
	h, ok := arr.Elements[0].(*object.Hash)
	if !ok {
		t.Fatalf("infer_csv_types returned non-hash: %#v", arr.Elements[0])
	}
	cases := map[string]string{"id": "int", "name": "string", "active": "bool", "joined": "time.Time"}
	for field, want := range cases {
		got := hashFieldValue(t, h, field).(*object.String).Value
		if got != want {
			t.Fatalf("field %q: expected type %q, got %q", field, want, got)
		}
	}
}

func TestInferJSONTypes(t *testing.T) {
	fn := lookupBuiltin(t, "infer_json_types")
	rows := &object.Array{Elements: []object.Object{
		newRecord(map[string]object.Object{
			"id":     &object.Integer{Value: 1},
			"name":   &object.String{Value: "Ada"},
			"active": object.TRUE,
			"score":  &object.Float{Value: 9.5},
		}),
		newRecord(map[string]object.Object{
			"id":     &object.Integer{Value: 2},
			"name":   &object.String{Value: "Grace"},
			"active": object.FALSE,
			"score":  &object.Integer{Value: 10},
		}),
	}}
	result := fn.Fn(rows)
	arr, ok := result.(*object.Array)
	if !ok || len(arr.Elements) != 2 {
		t.Fatalf("infer_json_types returned unexpected shape: %#v", result)
	}
	if arr.Elements[1] != object.NULL {
		t.Fatalf("infer_json_types returned err: %#v", arr.Elements[1])
	}
	h := arr.Elements[0].(*object.Hash)
	cases := map[string]string{"id": "int", "name": "string", "active": "bool", "score": "float64"}
	for field, want := range cases {
		got := hashFieldValue(t, h, field).(*object.String).Value
		if got != want {
			t.Fatalf("field %q: expected type %q, got %q (score should merge int+float64 -> float64)", field, want, got)
		}
	}
}

func TestInferValueType(t *testing.T) {
	fn := lookupBuiltin(t, "infer_value_type")
	cases := []struct {
		value object.Object
		want  string
	}{
		{&object.Integer{Value: 42}, "int"},
		{&object.Float{Value: 3.14}, "float64"},
		{&object.String{Value: "hello"}, "string"},
		{object.TRUE, "bool"},
		{object.NULL, "null"},
	}
	for _, tc := range cases {
		result := fn.Fn(tc.value)
		s, ok := result.(*object.String)
		if !ok || s.Value != tc.want {
			t.Fatalf("infer_value_type(%s) = %#v, want %q", tc.value.Inspect(), result, tc.want)
		}
	}
}

func newRecord(pairs map[string]object.Object) *object.Hash {
	h := &object.Hash{Pairs: map[object.HashKey]object.HashPair{}}
	for k, v := range pairs {
		key := &object.String{Value: k}
		h.Pairs[key.HashKey()] = object.HashPair{Key: key, Value: v}
	}
	return h
}
