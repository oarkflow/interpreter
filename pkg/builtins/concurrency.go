package builtins

import (
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/security"
)

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{

		// -----------------------------------------------------------------
		// go
		// -----------------------------------------------------------------
		"go": {
			FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
				if len(args) < 1 {
					return &object.String{Value: "go expects function and optional args"}
				}
				fn := args[0]
				callArgs := []object.Object{}
				if len(args) > 1 {
					callArgs = args[1:]
				}
				ch := make(chan object.Object, 1)
				go func() {
					if object.ApplyFunctionFn != nil {
						ch <- object.ApplyFunctionFn(fn, callArgs, env)
					} else {
						ch <- object.NULL
					}
				}()
				return &object.Future{Ch: ch}
			},
		},

		// -----------------------------------------------------------------
		// generator
		// -----------------------------------------------------------------
		"generator": {
			FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: "generator expects 1 function"}
				}
				var res object.Object
				if object.ApplyFunctionFn != nil {
					res = object.ApplyFunctionFn(args[0], []object.Object{}, env)
				} else {
					res = eval.ExecuteCallback(args[0], []object.Object{})
				}
				if object.IsError(res) {
					return res
				}
				if arr, ok := res.(*object.Array); ok {
					return &object.GeneratorValue{Elements: arr.Elements}
				}
				return &object.GeneratorValue{Elements: []object.Object{res}}
			},
		},

		// -----------------------------------------------------------------
		// permissions
		// -----------------------------------------------------------------
		"permissions": {
			FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: "permissions expects 1 hash argument"}
				}
				h, ok := args[0].(*object.Hash)
				if !ok {
					return &object.String{Value: "permissions argument must be HASH"}
				}
				policy := &object.SecurityPolicy{
					StrictMode:          hashBoolValue(h, "strict", false),
					ProtectHost:         hashBoolValue(h, "protect_host", false),
					AllowEnvWrite:       hashBoolValue(h, "allow_env_write", true),
					AllowedExecCommands: hashStringArray(h, "allow_exec"),
					DeniedExecCommands:  hashStringArray(h, "deny_exec"),
					AllowedNetworkHosts: hashStringArray(h, "allow_http"),
					DeniedNetworkHosts:  hashStringArray(h, "deny_http"),
				}
				if env != nil {
					env.SecurityPolicy = policy
				}
				// Also set the global override
				security.WithSecurityPolicyOverride(policy, func() (any, error) {
					return nil, nil
				})
				return object.TRUE
			},
		},

		// -----------------------------------------------------------------
		// move
		// -----------------------------------------------------------------
		"move": {
			FnWithEnv: func(env *object.Environment, args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.String{Value: "move expects exactly 1 argument"}
				}
				if env == nil {
					return args[0]
				}
				return maybeWrapOwned(args[0], env)
			},
		},
	})
}

func maybeWrapOwned(val object.Object, env *object.Environment) object.Object {
	if env == nil || val == nil {
		return val
	}
	switch v := val.(type) {
	case *object.OwnedValue:
		v.OwnerID = env.EnsureOwnerID()
		return v
	case *object.Array, *object.Hash, *object.ADTValue:
		return &object.OwnedValue{OwnerID: env.EnsureOwnerID(), Inner: val}
	default:
		return val
	}
}
