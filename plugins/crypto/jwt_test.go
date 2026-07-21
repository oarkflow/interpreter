package crypto

import (
	"testing"
	"time"

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

func newClaims(pairs map[string]object.Object) *object.Hash {
	h := &object.Hash{Pairs: map[object.HashKey]object.HashPair{}}
	for k, v := range pairs {
		key := &object.String{Value: k}
		h.Pairs[key.HashKey()] = object.HashPair{Key: key, Value: v}
	}
	return h
}

func TestJWTBuiltinsRegisterOnImport(t *testing.T) {
	for _, name := range []string{"jwt_encode", "jwt_decode"} {
		if !eval.HasBuiltin(name) {
			t.Fatalf("expected cryptoextra builtin %q to be registered", name)
		}
	}
}

func TestJWTEncodeDecodeRoundTrip(t *testing.T) {
	encode := lookupBuiltin(t, "jwt_encode")
	decode := lookupBuiltin(t, "jwt_decode")

	claims := newClaims(map[string]object.Object{
		"sub":  &object.String{Value: "user123"},
		"role": &object.String{Value: "admin"},
	})
	secret := &object.String{Value: "top-secret"}

	tokenObj := encode.Fn(claims, secret)
	tok, ok := tokenObj.(*object.String)
	if !ok {
		t.Fatalf("jwt_encode returned non-string: %#v", tokenObj)
	}
	if tok.Value == "" {
		t.Fatalf("jwt_encode returned empty token")
	}

	decodedObj := decode.Fn(tok, secret)
	decoded, ok := decodedObj.(*object.Hash)
	if !ok {
		t.Fatalf("jwt_decode returned non-hash (possibly an error): %#v", decodedObj)
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

func TestJWTDecodeRejectsSignatureMismatch(t *testing.T) {
	encode := lookupBuiltin(t, "jwt_encode")
	decode := lookupBuiltin(t, "jwt_decode")

	claims := newClaims(map[string]object.Object{"sub": &object.String{Value: "user123"}})
	tokenObj := encode.Fn(claims, &object.String{Value: "correct-secret"})
	tok, ok := tokenObj.(*object.String)
	if !ok {
		t.Fatalf("jwt_encode returned non-string: %#v", tokenObj)
	}

	result := decode.Fn(tok, &object.String{Value: "wrong-secret"})
	errObj, ok := result.(*object.Error)
	if !ok {
		t.Fatalf("expected error for signature mismatch, got %#v", result)
	}
	if errObj.Message == "" {
		t.Fatalf("expected non-empty error message")
	}
}

func TestJWTDecodeRejectsExpiredToken(t *testing.T) {
	encode := lookupBuiltin(t, "jwt_encode")
	decode := lookupBuiltin(t, "jwt_decode")

	claims := newClaims(map[string]object.Object{
		"sub": &object.String{Value: "user123"},
		"exp": &object.Integer{Value: time.Now().Add(-1 * time.Hour).Unix()},
	})
	secret := &object.String{Value: "s3cr3t"}

	tokenObj := encode.Fn(claims, secret)
	tok, ok := tokenObj.(*object.String)
	if !ok {
		t.Fatalf("jwt_encode returned non-string: %#v", tokenObj)
	}

	result := decode.Fn(tok, secret)
	if _, ok := result.(*object.Error); !ok {
		t.Fatalf("expected error for expired token, got %#v", result)
	}
}

func TestJWTDecodeRejectsAlgNoneForgery(t *testing.T) {
	decode := lookupBuiltin(t, "jwt_decode")

	// Manually forged token with header {"alg":"none","typ":"JWT"} and an
	// empty signature segment, claiming an admin subject. This must never
	// be accepted regardless of the secret supplied.
	forged := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0." +
		"eyJzdWIiOiJhZG1pbiJ9."

	result := decode.Fn(&object.String{Value: forged}, &object.String{Value: "any-secret"})
	if _, ok := result.(*object.Error); !ok {
		t.Fatalf("expected alg=none forged token to be rejected, got %#v", result)
	}
}

func TestJWTDecodeRejectsAlgorithmConfusion(t *testing.T) {
	encode := lookupBuiltin(t, "jwt_encode")
	decode := lookupBuiltin(t, "jwt_decode")

	claims := newClaims(map[string]object.Object{"sub": &object.String{Value: "user123"}})
	secret := &object.String{Value: "s3cr3t"}

	// Token signed with HS512...
	opts := newClaims(map[string]object.Object{"alg": &object.String{Value: "HS512"}})
	tokenObj := encode.Fn(claims, secret, opts)
	tok, ok := tokenObj.(*object.String)
	if !ok {
		t.Fatalf("jwt_encode returned non-string: %#v", tokenObj)
	}

	// ...but decoded while only HS256 (the default) is accepted must fail,
	// since the caller-configured algorithm doesn't match the token header.
	result := decode.Fn(tok, secret)
	if _, ok := result.(*object.Error); !ok {
		t.Fatalf("expected algorithm-confusion (HS512 token decoded as HS256) to be rejected, got %#v", result)
	}
}

func TestJWTEncodeExpiresIn(t *testing.T) {
	encode := lookupBuiltin(t, "jwt_encode")
	decode := lookupBuiltin(t, "jwt_decode")

	claims := newClaims(map[string]object.Object{"sub": &object.String{Value: "user123"}})
	secret := &object.String{Value: "s3cr3t"}
	opts := newClaims(map[string]object.Object{"expires_in": &object.Integer{Value: 3600}})

	tokenObj := encode.Fn(claims, secret, opts)
	tok, ok := tokenObj.(*object.String)
	if !ok {
		t.Fatalf("jwt_encode returned non-string: %#v", tokenObj)
	}

	decoded := decode.Fn(tok, secret)
	if _, ok := decoded.(*object.Hash); !ok {
		t.Fatalf("expected valid (non-expired) token to decode, got %#v", decoded)
	}
}
