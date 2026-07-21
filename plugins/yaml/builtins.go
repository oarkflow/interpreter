package yaml

import (
	"bytes"
	"strings"

	"github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	yamlv3 "gopkg.in/yaml.v3"
)

// yamlObjectToNative converts an object.Object tree to a plain Go value tree
// (map[string]interface{}, []interface{}, string, int64, float64, bool, nil)
// suitable for yaml.Marshal. This mirrors the shape produced by eval.ToObject
// (the converter config_load already uses to turn parsed YAML into
// object.Object values), just running in the opposite direction.
func yamlObjectToNative(obj object.Object) interface{} {
	switch v := obj.(type) {
	case nil:
		return nil
	case *object.Null:
		return nil
	case *object.Boolean:
		return v.Value
	case *object.Integer:
		return v.Value
	case *object.Float:
		return v.Value
	case *object.String:
		return v.Value
	case *object.Secret:
		return v.Value
	case *object.Array:
		out := make([]interface{}, len(v.Elements))
		for i, el := range v.Elements {
			out[i] = yamlObjectToNative(el)
		}
		return out
	case *object.Hash:
		out := make(map[string]interface{}, len(v.Pairs))
		for _, pair := range v.Pairs {
			out[pair.Key.Inspect()] = yamlObjectToNative(pair.Value)
		}
		return out
	case *object.OwnedValue:
		return yamlObjectToNative(v.Inner)
	case *object.ImmutableValue:
		return yamlObjectToNative(v.Inner)
	default:
		return v.Inspect()
	}
}

// builtinYAMLEncode implements yaml_encode(value[, opts]). opts currently
// supports {"indent": <int>}.
func builtinYAMLEncode(args ...object.Object) object.Object {
	if len(args) < 1 || len(args) > 2 {
		return object.NewError("yaml_encode(value[, opts]) expects 1 or 2 arguments, got=%d", len(args))
	}

	indent := 0
	if len(args) == 2 {
		optsHash, ok := args[1].(*object.Hash)
		if !ok {
			return object.NewError("yaml_encode: opts must be a HASH")
		}
		for _, pair := range optsHash.Pairs {
			keyStr, ok := pair.Key.(*object.String)
			if !ok || keyStr.Value != "indent" {
				continue
			}
			switch iv := pair.Value.(type) {
			case *object.Integer:
				indent = int(iv.Value)
			case *object.Float:
				indent = int(iv.Value)
			default:
				return object.NewError("yaml_encode: opts.indent must be a number")
			}
		}
	}

	native := yamlObjectToNative(args[0])

	var buf bytes.Buffer
	enc := yamlv3.NewEncoder(&buf)
	if indent > 0 {
		enc.SetIndent(indent)
	}
	if err := enc.Encode(native); err != nil {
		_ = enc.Close()
		return object.NewError("yaml_encode: %s", err)
	}
	if err := enc.Close(); err != nil {
		return object.NewError("yaml_encode: %s", err)
	}

	return &object.String{Value: buf.String()}
}

// builtinYAMLDecode implements yaml_decode(yamlString), reusing the same
// interface{}->object.Object conversion (eval.ToObject) that config_load
// uses for parsed YAML content.
func builtinYAMLDecode(args ...object.Object) object.Object {
	if len(args) != 1 {
		return object.NewError("yaml_decode(yamlString) expects 1 argument, got=%d", len(args))
	}
	s, ok := args[0].(*object.String)
	if !ok {
		return object.NewError("yaml_decode: argument must be a STRING")
	}
	if strings.TrimSpace(s.Value) == "" {
		return object.NULL
	}

	var v interface{}
	if err := yamlv3.Unmarshal([]byte(s.Value), &v); err != nil {
		return object.NewError("yaml_decode: %s", err)
	}

	return eval.ToObject(v)
}

func init() {
	builtins := map[string]*object.Builtin{
		"yaml_encode": {Fn: builtinYAMLEncode},
		"yaml_decode": {Fn: builtinYAMLDecode},
	}
	eval.RegisterPluginBuiltins(builtins)

	_ = interpreter.RegisterStdBuiltinModuleWithPrefix("yaml", "yaml_", "config_load", "config_parse", "yaml_encode", "yaml_decode")
	_ = interpreter.RegisterStdBuiltinModuleWithPrefix("config/yaml", "yaml_", "config_load", "config_parse", "yaml_encode", "yaml_decode")
}
