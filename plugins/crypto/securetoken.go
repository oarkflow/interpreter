package crypto

import (
	"context"
	"crypto/sha256"

	"github.com/oarkflow/securetoken"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

// securetokenKey derives a fixed 32-byte AES-256-GCM key from an arbitrary
// secret string (mirrors how jwt_encode/jwt_decode above take a plain
// `secret` string rather than requiring callers to manage raw key bytes).
func securetokenKey(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

func init() {
	eval.RegisterPluginBuiltins(map[string]*object.Builtin{
		// securetoken_encrypt(claims, secret[, opts])
		// Encrypts a HASH of claims into an AES-256-GCM "s1.local." token
		// (github.com/oarkflow/securetoken) using a key derived (SHA-256)
		// from `secret`. Optional `opts` HASH supports:
		//   footer: STRING, attached in cleartext but authenticated.
		"securetoken_encrypt": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 2 || len(args) > 3 {
					return object.NewError("securetoken_encrypt() takes 2 or 3 arguments (claims, secret[, opts]), got %d", len(args))
				}
				claimsHash, ok := args[0].(*object.Hash)
				if !ok {
					return object.NewError("argument `claims` must be HASH, got %s", args[0].Type())
				}
				secret, errObj := asString(args[1], "secret")
				if errObj != nil {
					return errObj
				}

				var tokenOpts securetoken.TokenOptions
				if len(args) == 3 {
					optsHash, ok := args[2].(*object.Hash)
					if !ok {
						return object.NewError("argument `opts` must be HASH, got %s", args[2].Type())
					}
					if v, ok := hashGet(optsHash, "footer"); ok {
						footer, errObj := asString(v, "opts.footer")
						if errObj != nil {
							return errObj
						}
						tokenOpts.Footer = []byte(footer)
					}
				}

				claims := map[string]interface{}{}
				for _, pair := range claimsHash.Pairs {
					key := pair.Key.Inspect()
					if s, ok := pair.Key.(*object.String); ok {
						key = s.Value
					}
					claims[key] = objectToNative(pair.Value)
				}

				token, err := securetoken.EncryptJSON(securetokenKey(secret), claims, tokenOpts)
				if err != nil {
					return object.NewError("securetoken_encrypt: %s", err)
				}
				return &object.String{Value: token}
			},
		},

		// securetoken_decrypt(token, secret[, opts]) -> HASH of claims.
		// Decrypts and authenticates an "s1.local." token, rejecting any
		// token whose AEAD tag does not match (tampering or wrong secret).
		// Optional `opts` HASH supports:
		//   expected_footer: STRING, must match the token's authenticated
		//     footer exactly or decryption fails.
		"securetoken_decrypt": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 2 || len(args) > 3 {
					return object.NewError("securetoken_decrypt() takes 2 or 3 arguments (token, secret[, opts]), got %d", len(args))
				}
				tokenStr, errObj := asString(args[0], "token")
				if errObj != nil {
					return errObj
				}
				secret, errObj := asString(args[1], "secret")
				if errObj != nil {
					return errObj
				}

				var policy securetoken.ValidationPolicy
				if len(args) == 3 {
					optsHash, ok := args[2].(*object.Hash)
					if !ok {
						return object.NewError("argument `opts` must be HASH, got %s", args[2].Type())
					}
					if v, ok := hashGet(optsHash, "expected_footer"); ok {
						footer, errObj := asString(v, "opts.expected_footer")
						if errObj != nil {
							return errObj
						}
						policy.ExpectedFooter = []byte(footer)
					}
				}

				claims := map[string]interface{}{}
				if err := securetoken.DecryptJSON(context.Background(), tokenStr, securetokenKey(secret), &claims, policy); err != nil {
					return object.NewError("securetoken_decrypt: %s", err)
				}
				return toObject(claims)
			},
		},
	})
}
