// Package wuid wraps github.com/oarkflow/wuid, exposing short, sortable,
// time-ordered unique ID generation as SPL builtins - a sortable-ID
// counterpart to the existing uuid()/token_generate() daily-ops builtins.
package wuid

import (
	"time"

	"github.com/oarkflow/wuid"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

func init() {
	eval.RegisterPluginBuiltins(map[string]*object.Builtin{
		// wuid_new() -> STRING
		// Generates a new 128-bit, time-ordered, lexicographically sortable
		// ID (UUIDv7-compatible), encoded as fixed-width base62 text - about
		// a third shorter than a dashed UUID while preserving sort order.
		"wuid_new": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 0 {
					return object.NewError("wuid_new() takes no arguments, got %d", len(args))
				}
				return &object.String{Value: wuid.NewString()}
			},
		},

		// wuid_new_uuid() -> STRING
		// Generates the same sortable ID as wuid_new, but formatted as a
		// standard dashed UUID string for systems that expect that shape.
		"wuid_new_uuid": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 0 {
					return object.NewError("wuid_new_uuid() takes no arguments, got %d", len(args))
				}
				return &object.String{Value: wuid.New().UUIDString()}
			},
		},

		// wuid_parse(id) -> (result, err)
		// Decodes a base62, Crockford base32, hex, or dashed-UUID id string
		// and returns its embedded creation time.
		"wuid_parse": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return object.NewError("wuid_parse() takes 1 argument (id), got %d", len(args))
				}
				s, errObj := asString(args[0], "id")
				if errObj != nil {
					return errObj
				}
				id, err := wuid.Parse(s)
				if err != nil {
					return tuple(object.NULL, &object.String{Value: err.Error()})
				}
				pairs := map[string]object.Object{
					"id":      &object.String{Value: id.String()},
					"uuid":    &object.String{Value: id.UUIDString()},
					"hex":     &object.String{Value: id.Hex()},
					"unix_ms": &object.Integer{Value: id.UnixMilli()},
					"time":    &object.String{Value: id.Time().UTC().Format(time.RFC3339Nano)},
				}
				return tuple(hashFromPairs(pairs), object.NULL)
			},
		},
	})
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

func tuple(values ...object.Object) *object.Array {
	return &object.Array{Elements: values}
}
