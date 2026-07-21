// Package shamir wraps github.com/oarkflow/shamir, exposing Shamir secret
// sharing (split a secret into N shares such that any T of them
// reconstruct it, but fewer reveal nothing) as SPL builtins - useful for
// distributing a master key/password across multiple holders so no single
// person can recover it alone (e.g. splitting a database encryption key
// among on-call engineers, or a root credential among founders).
package shamir

import (
	"crypto/rand"
	"encoding/base64"

	"github.com/oarkflow/shamir"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

func init() {
	eval.RegisterPluginBuiltins(map[string]*object.Builtin{
		// shamir_split(secret, threshold, shares[, auth_key]) -> (result, err)
		// Splits `secret` into `shares` base64-encoded pieces, any
		// `threshold` of which reconstruct it via shamir_combine; fewer
		// than `threshold` reveal nothing about the secret at all. E.g.
		// shamir_split("db-master-key", 3, 5) makes 5 shares where any 3
		// are enough to recover the key.
		//
		// Every share is HMAC-tagged with an `auth_key` so tampered or
		// mismatched shares are rejected at combine time rather than
		// silently producing garbage. Pass a base64 `auth_key` (as
		// returned by an earlier shamir_split) to reuse one; when omitted,
		// a random one is generated and returned in the result - keep it
		// alongside the shares (but distributed separately, since its own
		// secrecy is what makes the shares tamper-evident) and pass it to
		// shamir_combine.
		// Returns {shares: ARRAY of STRING, auth_key: STRING}.
		"shamir_split": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 3 || len(args) > 4 {
					return object.NewError("shamir_split() takes 3 or 4 arguments (secret, threshold, shares[, auth_key]), got %d", len(args))
				}
				secret, errObj := asString(args[0], "secret")
				if errObj != nil {
					return errObj
				}
				threshold, errObj := asInt(args[1], "threshold")
				if errObj != nil {
					return errObj
				}
				total, errObj := asInt(args[2], "shares")
				if errObj != nil {
					return errObj
				}
				auth, authKeyBytes, errObj := resolveAuthKey(args, 3)
				if errObj != nil {
					return errObj
				}
				shares, err := shamir.Split(rand.Reader, []byte(secret), int(threshold), int(total), auth)
				if err != nil {
					return tuple(object.NULL, &object.String{Value: err.Error()})
				}
				elements := make([]object.Object, len(shares))
				for i, s := range shares {
					elements[i] = &object.String{Value: base64.StdEncoding.EncodeToString(s)}
				}
				return tuple(hashFromPairs(map[string]object.Object{
					"shares":   &object.Array{Elements: elements},
					"auth_key": &object.String{Value: base64.StdEncoding.EncodeToString(authKeyBytes)},
				}), object.NULL)
			},
		},

		// shamir_combine(shares, auth_key) -> (secret, err)
		// Reconstructs the original secret from at least `threshold` of the
		// base64-encoded shares returned by shamir_split, authenticated
		// with the matching base64 `auth_key` shamir_split returned
		// alongside them. Fewer than `threshold` shares, a wrong auth_key,
		// or tampered/mismatched shares all fail with an error rather than
		// silently returning garbage.
		"shamir_combine": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("shamir_combine() takes 2 arguments (shares, auth_key), got %d", len(args))
				}
				arr, ok := args[0].(*object.Array)
				if !ok {
					return object.NewError("argument `shares` must be ARRAY, got %s", args[0].Type())
				}
				auth, _, errObj := resolveAuthKey(args, 1)
				if errObj != nil {
					return errObj
				}
				shares := make([][]byte, len(arr.Elements))
				for i, el := range arr.Elements {
					s, errObj := asString(el, "shares[]")
					if errObj != nil {
						return errObj
					}
					decoded, err := base64.StdEncoding.DecodeString(s)
					if err != nil {
						return tuple(object.NULL, &object.String{Value: "shamir_combine: invalid share encoding: " + err.Error()})
					}
					shares[i] = decoded
				}
				secret, err := shamir.Combine(shares, auth)
				if err != nil {
					return tuple(object.NULL, &object.String{Value: err.Error()})
				}
				return tuple(&object.String{Value: string(secret)}, object.NULL)
			},
		},
	})
}

// resolveAuthKey reads an optional base64 auth-key STRING argument at
// args[idx]. When absent, a fresh random 32-byte key is generated. Returns
// both the parsed *shamir.AuthKey (for Split/Combine) and its raw bytes
// (for echoing back to the caller as base64 in shamir_split's result).
func resolveAuthKey(args []object.Object, idx int) (*shamir.AuthKey, []byte, object.Object) {
	var keyBytes []byte
	if len(args) > idx {
		s, errObj := asString(args[idx], "auth_key")
		if errObj != nil {
			return nil, nil, errObj
		}
		decoded, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, nil, object.NewError("argument `auth_key` is not valid base64: %s", err)
		}
		keyBytes = decoded
	} else {
		random, err := shamir.GenerateRandomBytes(32)
		if err != nil {
			return nil, nil, object.NewError("shamir: %s", err)
		}
		keyBytes = random
	}
	auth, err := shamir.NewAuthKey(keyBytes)
	if err != nil {
		return nil, nil, object.NewError("shamir: %s", err)
	}
	return auth, keyBytes, nil
}

func hashFromPairs(pairs map[string]object.Object) *object.Hash {
	out := make(map[object.HashKey]object.HashPair, len(pairs))
	for k, v := range pairs {
		key := &object.String{Value: k}
		out[key.HashKey()] = object.HashPair{Key: key, Value: v}
	}
	return &object.Hash{Pairs: out}
}

func asString(arg object.Object, name string) (string, object.Object) {
	if s, ok := arg.(*object.Secret); ok {
		return s.Value, nil
	}
	if arg == nil {
		return "", object.NewError("argument `%s` must be STRING, got <nil>", name)
	}
	if arg.Type() != object.STRING_OBJ {
		return "", object.NewError("argument `%s` must be STRING, got %s", name, arg.Type())
	}
	return arg.(*object.String).Value, nil
}

func asInt(arg object.Object, name string) (int64, object.Object) {
	switch v := arg.(type) {
	case *object.Integer:
		return v.Value, nil
	case *object.Float:
		return int64(v.Value), nil
	case nil:
		return 0, object.NewError("argument `%s` must be INTEGER, got <nil>", name)
	default:
		return 0, object.NewError("argument `%s` must be INTEGER, got %s", name, arg.Type())
	}
}

func tuple(values ...object.Object) *object.Array {
	return &object.Array{Elements: values}
}
