package crypto

import (
	"testing"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

func TestSecuretokenBuiltinsRegisterOnImport(t *testing.T) {
	for _, name := range []string{"securetoken_encrypt", "securetoken_decrypt"} {
		if !eval.HasBuiltin(name) {
			t.Fatalf("expected securetoken builtin %q to be registered", name)
		}
	}
}

func TestSecuretokenEncryptDecryptRoundTrip(t *testing.T) {
	encrypt := lookupBuiltin(t, "securetoken_encrypt")
	decrypt := lookupBuiltin(t, "securetoken_decrypt")

	claims := newClaims(map[string]object.Object{
		"sub":  &object.String{Value: "user123"},
		"role": &object.String{Value: "admin"},
	})
	secret := &object.String{Value: "top-secret"}

	tokenObj := encrypt.Fn(claims, secret)
	tok, ok := tokenObj.(*object.String)
	if !ok {
		t.Fatalf("securetoken_encrypt returned non-string: %#v", tokenObj)
	}
	if tok.Value == "" {
		t.Fatalf("securetoken_encrypt returned empty token")
	}

	decodedObj := decrypt.Fn(tok, secret)
	decoded, ok := decodedObj.(*object.Hash)
	if !ok {
		t.Fatalf("securetoken_decrypt returned non-hash (possibly an error): %#v", decodedObj)
	}

	subKey := (&object.String{Value: "sub"}).HashKey()
	pair, ok := decoded.Pairs[subKey]
	if !ok {
		t.Fatalf("decoded claims missing `sub`")
	}
	subVal, ok := pair.Value.(*object.String)
	if !ok || subVal.Value != "user123" {
		t.Fatalf("expected sub=user123, got %#v", pair.Value)
	}

	roleKey := (&object.String{Value: "role"}).HashKey()
	rolePair, ok := decoded.Pairs[roleKey]
	if !ok {
		t.Fatalf("decoded claims missing `role`")
	}
	roleVal, ok := rolePair.Value.(*object.String)
	if !ok || roleVal.Value != "admin" {
		t.Fatalf("expected role=admin, got %#v", rolePair.Value)
	}
}

func TestSecuretokenDecryptRejectsWrongSecret(t *testing.T) {
	encrypt := lookupBuiltin(t, "securetoken_encrypt")
	decrypt := lookupBuiltin(t, "securetoken_decrypt")

	claims := newClaims(map[string]object.Object{"sub": &object.String{Value: "user123"}})
	tokenObj := encrypt.Fn(claims, &object.String{Value: "correct-secret"})
	tok, ok := tokenObj.(*object.String)
	if !ok {
		t.Fatalf("securetoken_encrypt returned non-string: %#v", tokenObj)
	}

	result := decrypt.Fn(tok, &object.String{Value: "wrong-secret"})
	if _, ok := result.(*object.Error); !ok {
		t.Fatalf("expected error for wrong secret, got %#v", result)
	}
}

func TestSecuretokenDecryptRejectsFooterMismatch(t *testing.T) {
	encrypt := lookupBuiltin(t, "securetoken_encrypt")
	decrypt := lookupBuiltin(t, "securetoken_decrypt")

	claims := newClaims(map[string]object.Object{"sub": &object.String{Value: "user123"}})
	secret := &object.String{Value: "s3cr3t"}
	encOpts := newClaims(map[string]object.Object{"footer": &object.String{Value: "kid-1"}})

	tokenObj := encrypt.Fn(claims, secret, encOpts)
	tok, ok := tokenObj.(*object.String)
	if !ok {
		t.Fatalf("securetoken_encrypt returned non-string: %#v", tokenObj)
	}

	decOpts := newClaims(map[string]object.Object{"expected_footer": &object.String{Value: "kid-2"}})
	result := decrypt.Fn(tok, secret, decOpts)
	if _, ok := result.(*object.Error); !ok {
		t.Fatalf("expected error for footer mismatch, got %#v", result)
	}
}
