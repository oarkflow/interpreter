package cryptoextra

import (
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"golang.org/x/crypto/bcrypt"
)

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{
		"bcrypt_hash": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return object.NewError("bcrypt_hash() takes 1 or 2 arguments (password[, cost]), got %d", len(args))
				}
				password, errObj := asString(args[0], "password")
				if errObj != nil {
					return errObj
				}
				cost := bcrypt.DefaultCost
				if len(args) == 2 {
					c, errObj := asInt(args[1], "cost")
					if errObj != nil {
						return errObj
					}
					cost = int(c)
				}
				hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
				if err != nil {
					return object.NewError("bcrypt_hash: %s", err)
				}
				return &object.String{Value: string(hash)}
			},
		},
		"bcrypt_verify": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 2 {
					return object.NewError("bcrypt_verify() takes 2 arguments (password, hash), got %d", len(args))
				}
				password, errObj := asString(args[0], "password")
				if errObj != nil {
					return errObj
				}
				hash, errObj := asString(args[1], "hash")
				if errObj != nil {
					return errObj
				}
				return object.NativeBoolToBooleanObject(bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil)
			},
		},
	})
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
