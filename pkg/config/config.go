package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oarkflow/interpreter/pkg/object"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// SanitizePathFn / CheckFileReadAllowedFn – pluggable from the host package
// ---------------------------------------------------------------------------

// SanitizePathFn validates/resolves user-supplied paths. The host package
// must set this before calling LoadConfigObjectFromPath.
var SanitizePathFn func(userPath string) (string, error) = func(p string) (string, error) {
	return filepath.Abs(p)
}

// CheckFileReadAllowedFn checks whether reading a file at the given path is
// allowed by the active security policy. Defaults to no-op (allow all).
var CheckFileReadAllowedFn func(path string) error = func(string) error { return nil }

// ParseJSONToObjectFn converts a JSON string to an object.Object. Must be set
// by the host package.
var ParseJSONToObjectFn func(raw string) (object.Object, error)

// ToObjectFn converts an arbitrary Go value to an object.Object. Must be set
// by the host package.
var ToObjectFn func(v interface{}) object.Object

// ObjectToNativeFn converts an object.Object back to a Go native value. Must
// be set by the host package.
var ObjectToNativeFn func(obj object.Object) interface{}

// ---------------------------------------------------------------------------
// Format detection
// ---------------------------------------------------------------------------

func DetectConfigFormat(path, explicit string) string {
	f := strings.ToLower(strings.TrimSpace(explicit))
	if f != "" {
		switch f {
		case "json", "yaml", "yml", "env", "dotenv":
			return f
		}
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".env":
		return "env"
	default:
		return "json"
	}
}

// ---------------------------------------------------------------------------
// .env parser
// ---------------------------------------------------------------------------

func ParseDotEnv(data []byte) (map[string]string, error) {
	lines := strings.Split(string(data), "\n")
	out := make(map[string]string)
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		idx := strings.Index(line, "=")
		if idx <= 0 {
			return nil, fmt.Errorf("invalid .env entry at line %d", i+1)
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		out[key] = val
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Sensitive-key detection & secret wrapping
// ---------------------------------------------------------------------------

func IsSensitiveKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" {
		return false
	}
	sensitive := []string{
		"password", "passwd", "secret", "token", "api_key", "apikey",
		"private_key", "secret_key", "access_key", "refresh_token",
		"client_secret", "credentials", "credential", "auth",
	}
	for _, s := range sensitive {
		if k == s || strings.Contains(k, s) {
			return true
		}
	}
	return false
}

func ApplySecretWrapping(obj object.Object) object.Object {
	return applySecretWrappingRec(obj, false)
}

func applySecretWrappingRec(obj object.Object, sensitiveCtx bool) object.Object {
	if obj == nil {
		return object.NULL
	}
	switch v := obj.(type) {
	case *object.OwnedValue:
		return &object.OwnedValue{OwnerID: v.OwnerID, Inner: applySecretWrappingRec(v.Inner, sensitiveCtx)}
	case *object.ImmutableValue:
		return &object.ImmutableValue{Inner: applySecretWrappingRec(v.Inner, sensitiveCtx)}
	case *object.String:
		if sensitiveCtx {
			return &object.Secret{Value: v.Value}
		}
		return v
	case *object.Array:
		elements := make([]object.Object, 0, len(v.Elements))
		for _, el := range v.Elements {
			elements = append(elements, applySecretWrappingRec(el, sensitiveCtx))
		}
		return &object.Array{Elements: elements}
	case *object.Hash:
		pairs := make(map[object.HashKey]object.HashPair, len(v.Pairs))
		for hk, pair := range v.Pairs {
			keyName := pair.Key.Inspect()
			nextSensitive := sensitiveCtx || IsSensitiveKey(keyName)
			val := applySecretWrappingRec(pair.Value, nextSensitive)
			pairs[hk] = object.HashPair{Key: pair.Key, Value: val}
		}
		return &object.Hash{Pairs: pairs}
	default:
		return obj
	}
}

// ---------------------------------------------------------------------------
// Config loading
// ---------------------------------------------------------------------------

// LoadConfigObjectFromPath reads a config file and returns an object.Object
// with sensitive values wrapped as Secrets.
func LoadConfigObjectFromPath(path, format string) (object.Object, error) {
	safePath, err := SanitizePathFn(path)
	if err != nil {
		return nil, err
	}
	if err := CheckFileReadAllowedFn(safePath); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(safePath)
	if err != nil {
		return nil, err
	}

	f := DetectConfigFormat(safePath, format)
	var obj object.Object
	switch f {
	case "json":
		if ParseJSONToObjectFn == nil {
			return nil, fmt.Errorf("ParseJSONToObjectFn not set")
		}
		obj, err = ParseJSONToObjectFn(string(data))
		if err != nil {
			return nil, err
		}
	case "yaml", "yml":
		var v interface{}
		if err := yaml.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		if ToObjectFn == nil {
			return nil, fmt.Errorf("ToObjectFn not set")
		}
		obj = ToObjectFn(v)
	case "env", "dotenv":
		m, err := ParseDotEnv(data)
		if err != nil {
			return nil, err
		}
		if ToObjectFn == nil {
			return nil, fmt.Errorf("ToObjectFn not set")
		}
		obj = ToObjectFn(m)
	default:
		return nil, fmt.Errorf("unsupported config format %q", format)
	}
	return ApplySecretWrapping(obj), nil
}

// LoadConfigObjectFromRaw parses raw config content (not from a file).
func LoadConfigObjectFromRaw(raw, format string) (object.Object, error) {
	f := DetectConfigFormat("config."+format, format)
	var obj object.Object
	var err error
	switch f {
	case "json":
		if ParseJSONToObjectFn == nil {
			return nil, fmt.Errorf("ParseJSONToObjectFn not set")
		}
		obj, err = ParseJSONToObjectFn(raw)
		if err != nil {
			return nil, err
		}
	case "yaml", "yml":
		var v interface{}
		if err := yaml.Unmarshal([]byte(raw), &v); err != nil {
			return nil, err
		}
		if ToObjectFn == nil {
			return nil, fmt.Errorf("ToObjectFn not set")
		}
		obj = ToObjectFn(v)
	case "env", "dotenv":
		m, err := ParseDotEnv([]byte(raw))
		if err != nil {
			return nil, err
		}
		if ToObjectFn == nil {
			return nil, fmt.Errorf("ToObjectFn not set")
		}
		obj = ToObjectFn(m)
	default:
		return nil, fmt.Errorf("unsupported config format %q", format)
	}
	return ApplySecretWrapping(obj), nil
}

// SecretValue extracts the raw string value from a Secret (or wrapped Secret).
func SecretValue(obj object.Object) (string, bool) {
	switch v := obj.(type) {
	case *object.Secret:
		return v.Value, true
	case *object.OwnedValue:
		return SecretValue(v.Inner)
	case *object.ImmutableValue:
		return SecretValue(v.Inner)
	default:
		return "", false
	}
}

// ObjectToJSONWithSecrets marshals an object to JSON, exposing secret values.
func ObjectToJSONWithSecrets(obj object.Object) (string, error) {
	if ObjectToNativeFn == nil {
		return "", fmt.Errorf("ObjectToNativeFn not set")
	}
	raw := ObjectToNativeFn(obj)
	out, err := json.Marshal(raw)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
