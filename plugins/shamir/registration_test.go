package shamir

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

func hashFieldValue(t *testing.T, h *object.Hash, key string) object.Object {
	t.Helper()
	k := (&object.String{Value: key}).HashKey()
	pair, ok := h.Pairs[k]
	if !ok {
		t.Fatalf("hash missing field %q", key)
	}
	return pair.Value
}

func TestShamirBuiltinsRegisterOnImport(t *testing.T) {
	for _, name := range []string{"shamir_split", "shamir_combine"} {
		if !eval.HasBuiltin(name) {
			t.Fatalf("expected shamir builtin %q to be registered", name)
		}
	}
}

func TestShamirSplitCombineRoundTrip(t *testing.T) {
	splitFn := lookupBuiltin(t, "shamir_split")
	combineFn := lookupBuiltin(t, "shamir_combine")

	result := splitFn.Fn(&object.String{Value: "db-master-key"}, &object.Integer{Value: 3}, &object.Integer{Value: 5})
	arr, ok := result.(*object.Array)
	if !ok || len(arr.Elements) != 2 {
		t.Fatalf("shamir_split returned unexpected shape: %#v", result)
	}
	if arr.Elements[1] != object.NULL {
		t.Fatalf("shamir_split returned err: %#v", arr.Elements[1])
	}
	h, ok := arr.Elements[0].(*object.Hash)
	if !ok {
		t.Fatalf("shamir_split result is not a hash: %#v", arr.Elements[0])
	}
	shares, ok := hashFieldValue(t, h, "shares").(*object.Array)
	if !ok || len(shares.Elements) != 5 {
		t.Fatalf("expected 5 shares, got %#v", hashFieldValue(t, h, "shares"))
	}
	authKey, ok := hashFieldValue(t, h, "auth_key").(*object.String)
	if !ok || authKey.Value == "" {
		t.Fatalf("expected non-empty auth_key, got %#v", hashFieldValue(t, h, "auth_key"))
	}

	// Any 3 of the 5 shares, plus the matching auth_key, reconstruct the secret.
	subset := &object.Array{Elements: shares.Elements[:3]}
	combined := combineFn.Fn(subset, authKey)
	combinedArr, ok := combined.(*object.Array)
	if !ok || len(combinedArr.Elements) != 2 {
		t.Fatalf("shamir_combine returned unexpected shape: %#v", combined)
	}
	if combinedArr.Elements[1] != object.NULL {
		t.Fatalf("shamir_combine returned err: %#v", combinedArr.Elements[1])
	}
	secret, ok := combinedArr.Elements[0].(*object.String)
	if !ok || secret.Value != "db-master-key" {
		t.Fatalf("expected reconstructed secret db-master-key, got %#v", combinedArr.Elements[0])
	}
}

func TestShamirCombineWrongAuthKeyFails(t *testing.T) {
	splitFn := lookupBuiltin(t, "shamir_split")
	combineFn := lookupBuiltin(t, "shamir_combine")

	result := splitFn.Fn(&object.String{Value: "top-secret"}, &object.Integer{Value: 3}, &object.Integer{Value: 5})
	h := result.(*object.Array).Elements[0].(*object.Hash)
	shares := hashFieldValue(t, h, "shares").(*object.Array)

	otherResult := splitFn.Fn(&object.String{Value: "unrelated"}, &object.Integer{Value: 2}, &object.Integer{Value: 3})
	otherAuthKey := hashFieldValue(t, otherResult.(*object.Array).Elements[0].(*object.Hash), "auth_key")

	subset := &object.Array{Elements: shares.Elements[:3]}
	combined := combineFn.Fn(subset, otherAuthKey)
	combinedArr, ok := combined.(*object.Array)
	if !ok || len(combinedArr.Elements) != 2 {
		t.Fatalf("shamir_combine returned unexpected shape: %#v", combined)
	}
	if combinedArr.Elements[1] == object.NULL {
		t.Fatalf("expected error combining shares with the wrong auth_key")
	}
}

func TestShamirSplitWithExplicitAuthKey(t *testing.T) {
	splitFn := lookupBuiltin(t, "shamir_split")
	combineFn := lookupBuiltin(t, "shamir_combine")

	first := splitFn.Fn(&object.String{Value: "reused-key-secret"}, &object.Integer{Value: 2}, &object.Integer{Value: 3})
	firstHash := first.(*object.Array).Elements[0].(*object.Hash)
	authKey := hashFieldValue(t, firstHash, "auth_key")

	// Split a second secret reusing the same auth_key.
	second := splitFn.Fn(&object.String{Value: "another-secret"}, &object.Integer{Value: 2}, &object.Integer{Value: 3}, authKey)
	secondArr, ok := second.(*object.Array)
	if !ok || len(secondArr.Elements) != 2 || secondArr.Elements[1] != object.NULL {
		t.Fatalf("shamir_split with explicit auth_key failed: %#v", second)
	}
	secondHash := secondArr.Elements[0].(*object.Hash)
	if hashFieldValue(t, secondHash, "auth_key").(*object.String).Value != authKey.(*object.String).Value {
		t.Fatalf("expected echoed auth_key to match the supplied one")
	}
	secondShares := hashFieldValue(t, secondHash, "shares").(*object.Array)

	combined := combineFn.Fn(&object.Array{Elements: secondShares.Elements[:2]}, authKey)
	combinedArr := combined.(*object.Array)
	if combinedArr.Elements[1] != object.NULL {
		t.Fatalf("shamir_combine returned err: %#v", combinedArr.Elements[1])
	}
	if combinedArr.Elements[0].(*object.String).Value != "another-secret" {
		t.Fatalf("expected another-secret, got %#v", combinedArr.Elements[0])
	}
}
