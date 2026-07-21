package crypto

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

// jwtSigningMethod maps the string `alg` option to a golang-jwt HMAC signing
// method. Only HMAC-based algorithms are supported: JWT encode/decode here is
// meant to pair with this package's existing HMAC/secret-based helpers, not
// to provide general-purpose asymmetric (RS256/ES256/etc.) support.
func jwtSigningMethod(alg string) (*jwt.SigningMethodHMAC, error) {
	switch alg {
	case "", "HS256":
		return jwt.SigningMethodHS256, nil
	case "HS384":
		return jwt.SigningMethodHS384, nil
	case "HS512":
		return jwt.SigningMethodHS512, nil
	default:
		return nil, fmt.Errorf("unsupported alg %q (supported: HS256, HS384, HS512)", alg)
	}
}

func init() {
	eval.RegisterPluginBuiltins(map[string]*object.Builtin{
		// jwt_encode(claims, secret[, opts])
		// Encodes a HASH of claims into a signed JWT string using an
		// HMAC algorithm (default HS256). Optional `opts` HASH supports:
		//   alg:         "HS256" | "HS384" | "HS512" (default "HS256")
		//   expires_in:  seconds from now, sets/overrides the `exp` claim
		"jwt_encode": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 2 || len(args) > 3 {
					return object.NewError("jwt_encode() takes 2 or 3 arguments (claims, secret[, opts]), got %d", len(args))
				}
				claimsHash, ok := args[0].(*object.Hash)
				if !ok {
					return object.NewError("argument `claims` must be HASH, got %s", args[0].Type())
				}
				secret, errObj := asString(args[1], "secret")
				if errObj != nil {
					return errObj
				}

				alg := "HS256"
				var expiresIn int64
				hasExpiresIn := false
				if len(args) == 3 {
					optsHash, ok := args[2].(*object.Hash)
					if !ok {
						return object.NewError("argument `opts` must be HASH, got %s", args[2].Type())
					}
					if v, ok := hashGet(optsHash, "alg"); ok {
						s, errObj := asString(v, "opts.alg")
						if errObj != nil {
							return errObj
						}
						alg = s
					}
					if v, ok := hashGet(optsHash, "expires_in"); ok {
						n, errObj := asInt(v, "opts.expires_in")
						if errObj != nil {
							return errObj
						}
						expiresIn = n
						hasExpiresIn = true
					}
				}

				method, err := jwtSigningMethod(alg)
				if err != nil {
					return object.NewError("jwt_encode: %s", err)
				}

				claims := jwt.MapClaims{}
				for _, pair := range claimsHash.Pairs {
					key := pair.Key.Inspect()
					if s, ok := pair.Key.(*object.String); ok {
						key = s.Value
					}
					claims[key] = objectToNative(pair.Value)
				}
				if hasExpiresIn {
					claims["exp"] = time.Now().Add(time.Duration(expiresIn) * time.Second).Unix()
				}

				token := jwt.NewWithClaims(method, claims)
				signed, err := token.SignedString([]byte(secret))
				if err != nil {
					return object.NewError("jwt_encode: %s", err)
				}
				return &object.String{Value: signed}
			},
		},

		// jwt_decode(token, secret[, opts])
		// Verifies and decodes a JWT string into a HASH of claims. Rejects
		// tokens with an invalid signature, an `alg` that does not match the
		// configured/expected algorithm (protecting against "alg: none" and
		// algorithm-confusion attacks — the token's own header is never
		// trusted to select the verification algorithm), and, if present,
		// expired (`exp`) or not-yet-valid (`nbf`) claims.
		// Optional `opts` HASH supports:
		//   alg: "HS256" | "HS384" | "HS512" (default "HS256") — the only
		//        algorithm that will be accepted for verification.
		"jwt_decode": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 2 || len(args) > 3 {
					return object.NewError("jwt_decode() takes 2 or 3 arguments (token, secret[, opts]), got %d", len(args))
				}
				tokenStr, errObj := asString(args[0], "token")
				if errObj != nil {
					return errObj
				}
				secret, errObj := asString(args[1], "secret")
				if errObj != nil {
					return errObj
				}

				alg := "HS256"
				if len(args) == 3 {
					optsHash, ok := args[2].(*object.Hash)
					if !ok {
						return object.NewError("argument `opts` must be HASH, got %s", args[2].Type())
					}
					if v, ok := hashGet(optsHash, "alg"); ok {
						s, errObj := asString(v, "opts.alg")
						if errObj != nil {
							return errObj
						}
						alg = s
					}
				}
				if _, err := jwtSigningMethod(alg); err != nil {
					return object.NewError("jwt_decode: %s", err)
				}

				claims := jwt.MapClaims{}
				_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
					return []byte(secret), nil
				}, jwt.WithValidMethods([]string{alg}))
				if err != nil {
					return object.NewError("jwt_decode: %s", err)
				}

				result := toObject(map[string]interface{}(claims))
				return result
			},
		},
	})
}

// hashGet looks up a string-keyed value in a HASH, returning (nil, false)
// when the key is absent.
func hashGet(h *object.Hash, key string) (object.Object, bool) {
	k := &object.String{Value: key}
	pair, ok := h.Pairs[k.HashKey()]
	if !ok {
		return nil, false
	}
	return pair.Value, true
}

// objectToNative converts an SPL object into a plain Go value suitable for
// building JWT claims (mirrors the equivalent helper in builtins/integrations).
func objectToNative(obj object.Object) interface{} {
	switch v := obj.(type) {
	case *object.Integer:
		return v.Value
	case *object.Float:
		return v.Value
	case *object.String:
		return v.Value
	case *object.Secret:
		return v.Value
	case *object.Boolean:
		return v.Value
	case *object.Null:
		return nil
	case nil:
		return nil
	case *object.Array:
		result := make([]interface{}, len(v.Elements))
		for i, el := range v.Elements {
			result[i] = objectToNative(el)
		}
		return result
	case *object.Hash:
		result := make(map[string]interface{})
		for _, pair := range v.Pairs {
			key := pair.Key.Inspect()
			if s, ok := pair.Key.(*object.String); ok {
				key = s.Value
			}
			result[key] = objectToNative(pair.Value)
		}
		return result
	default:
		return obj.Inspect()
	}
}

// toObject converts a plain Go value (as produced by decoding JWT claims)
// back into an SPL object (mirrors the equivalent helper in
// builtins/integrations).
func toObject(val interface{}) object.Object {
	if val == nil {
		return object.NULL
	}
	switch v := val.(type) {
	case object.Object:
		return v
	case bool:
		return object.NativeBoolToBooleanObject(v)
	case int:
		return &object.Integer{Value: int64(v)}
	case int64:
		return &object.Integer{Value: v}
	case uint64:
		return &object.Integer{Value: int64(v)}
	case float64:
		return &object.Float{Value: v}
	case string:
		return &object.String{Value: v}
	case []byte:
		return &object.String{Value: string(v)}
	case map[string]interface{}:
		pairs := make(map[object.HashKey]object.HashPair, len(v))
		for k, vv := range v {
			key := &object.String{Value: k}
			pairs[key.HashKey()] = object.HashPair{Key: key, Value: toObject(vv)}
		}
		return &object.Hash{Pairs: pairs}
	case []interface{}:
		elems := make([]object.Object, len(v))
		for i, el := range v {
			elems[i] = toObject(el)
		}
		return &object.Array{Elements: elems}
	default:
		return &object.String{Value: fmt.Sprintf("%v", v)}
	}
}
