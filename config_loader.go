package interpreter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func detectConfigFormat(path, explicit string) string {
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

func parseDotEnv(data []byte) (map[string]string, error) {
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

func isSensitiveKey(key string) bool {
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

func applySecretWrapping(obj Object) Object {
	return applySecretWrappingRec(obj, false)
}

func applySecretWrappingRec(obj Object, sensitiveCtx bool) Object {
	if obj == nil {
		return NULL
	}
	switch v := obj.(type) {
	case *OwnedValue:
		return &OwnedValue{ownerID: v.ownerID, inner: applySecretWrappingRec(v.inner, sensitiveCtx)}
	case *ImmutableValue:
		return &ImmutableValue{inner: applySecretWrappingRec(v.inner, sensitiveCtx)}
	case *String:
		if sensitiveCtx {
			return &Secret{Value: v.Value}
		}
		return v
	case *Array:
		elements := make([]Object, 0, len(v.Elements))
		for _, el := range v.Elements {
			elements = append(elements, applySecretWrappingRec(el, sensitiveCtx))
		}
		return &Array{Elements: elements}
	case *Hash:
		pairs := make(map[HashKey]HashPair, len(v.Pairs))
		for hk, pair := range v.Pairs {
			keyName := pair.Key.Inspect()
			nextSensitive := sensitiveCtx || isSensitiveKey(keyName)
			val := applySecretWrappingRec(pair.Value, nextSensitive)
			pairs[hk] = HashPair{Key: pair.Key, Value: val}
		}
		return &Hash{Pairs: pairs}
	default:
		return obj
	}
}

func loadConfigObjectFromPath(path, format string) (Object, error) {
	safePath, err := sanitizePath(path)
	if err != nil {
		return nil, err
	}
	if err := checkFileReadAllowed(safePath); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(safePath)
	if err != nil {
		return nil, err
	}

	f := detectConfigFormat(safePath, format)
	var obj Object
	switch f {
	case "json":
		obj, err = parseJSONToObject(string(data))
		if err != nil {
			return nil, err
		}
	case "yaml", "yml":
		var v interface{}
		if err := yaml.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		obj = toObject(v)
	case "env", "dotenv":
		m, err := parseDotEnv(data)
		if err != nil {
			return nil, err
		}
		obj = toObject(m)
	default:
		return nil, fmt.Errorf("unsupported config format %q", format)
	}
	return applySecretWrapping(obj), nil
}

func loadConfigObjectFromRaw(raw, format string) (Object, error) {
	f := detectConfigFormat("config."+format, format)
	var obj Object
	var err error
	switch f {
	case "json":
		obj, err = parseJSONToObject(raw)
		if err != nil {
			return nil, err
		}
	case "yaml", "yml":
		var v interface{}
		if err := yaml.Unmarshal([]byte(raw), &v); err != nil {
			return nil, err
		}
		obj = toObject(v)
	case "env", "dotenv":
		m, err := parseDotEnv([]byte(raw))
		if err != nil {
			return nil, err
		}
		obj = toObject(m)
	default:
		return nil, fmt.Errorf("unsupported config format %q", format)
	}
	return applySecretWrapping(obj), nil
}

func secretValue(obj Object) (string, bool) {
	switch v := obj.(type) {
	case *Secret:
		return v.Value, true
	case *OwnedValue:
		return secretValue(v.inner)
	case *ImmutableValue:
		return secretValue(v.inner)
	default:
		return "", false
	}
}

func objectToJSONWithSecrets(obj Object) (string, error) {
	raw := objectToNative(obj)
	out, err := json.Marshal(raw)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
