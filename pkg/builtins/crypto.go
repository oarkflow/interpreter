package builtins

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{
		// md5(input)
		// Returns the MD5 hex digest of the input string.
		"md5": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("md5() takes 1 argument, got %d", len(args))
				}
				s, errObj := asString(args[0], "input")
				if errObj != nil {
					return errObj
				}
				h := md5.Sum([]byte(s))
				return &object.String{Value: hex.EncodeToString(h[:])}
			},
		},

		// sha256(input)
		// Returns the SHA-256 hex digest of the input string.
		"sha256": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("sha256() takes 1 argument, got %d", len(args))
				}
				s, errObj := asString(args[0], "input")
				if errObj != nil {
					return errObj
				}
				h := sha256.Sum256([]byte(s))
				return &object.String{Value: hex.EncodeToString(h[:])}
			},
		},

		// sha512(input)
		// Returns the SHA-512 hex digest of the input string.
		"sha512": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("sha512() takes 1 argument, got %d", len(args))
				}
				s, errObj := asString(args[0], "input")
				if errObj != nil {
					return errObj
				}
				h := sha512.Sum512([]byte(s))
				return &object.String{Value: hex.EncodeToString(h[:])}
			},
		},

		// hmac_sha256(message, key)
		// Returns the HMAC-SHA256 hex digest.
		"hmac_sha256": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("hmac_sha256() takes 2 arguments (message, key), got %d", len(args))
				}
				msg, errObj := asString(args[0], "message")
				if errObj != nil {
					return errObj
				}
				key, errObj := asString(args[1], "key")
				if errObj != nil {
					return errObj
				}
				mac := hmac.New(sha256.New, []byte(key))
				mac.Write([]byte(msg))
				return &object.String{Value: hex.EncodeToString(mac.Sum(nil))}
			},
		},

		// hmac_sha512(message, key)
		// Returns the HMAC-SHA512 hex digest.
		"hmac_sha512": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("hmac_sha512() takes 2 arguments (message, key), got %d", len(args))
				}
				msg, errObj := asString(args[0], "message")
				if errObj != nil {
					return errObj
				}
				key, errObj := asString(args[1], "key")
				if errObj != nil {
					return errObj
				}
				mac := hmac.New(sha512.New, []byte(key))
				mac.Write([]byte(msg))
				return &object.String{Value: hex.EncodeToString(mac.Sum(nil))}
			},
		},
	})
}
