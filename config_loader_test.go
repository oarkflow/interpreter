package interpreter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDotEnv(t *testing.T) {
	m, err := parseDotEnv([]byte("A=1\n#x\nexport TOKEN=abc\n"))
	if err != nil {
		t.Fatalf("parse dotenv failed: %v", err)
	}
	if m["A"] != "1" || m["TOKEN"] != "abc" {
		t.Fatalf("unexpected dotenv map: %#v", m)
	}
}

func TestConfigLoadWrapsSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.json")
	if err := os.WriteFile(path, []byte(`{"username":"u","password":"p@ss"}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	env := NewGlobalEnvironment(nil)
	env.moduleDir = dir
	res := withSandboxRootOverride(dir, func() Object {
		obj, err := loadConfigObjectFromPath(path, "json")
		if err != nil {
			return &String{Value: "ERROR: " + err.Error()}
		}
		env.Set("CFG", obj)
		return obj
	})

	h, ok := res.(*Hash)
	if !ok {
		t.Fatalf("expected hash config, got %T", res)
	}
	pw, ok := hashGet(h, "password")
	if !ok {
		t.Fatalf("missing password")
	}
	if pw.Type() != SECRET_OBJ {
		t.Fatalf("expected SECRET_OBJ, got %s", pw.Type())
	}
}

func TestSecretBuiltins(t *testing.T) {
	obj := testEval(`let s = secret("abcd1234"); [s, secret_mask(s, 2), secret_reveal(s)];`)
	arr, ok := obj.(*Array)
	if !ok || len(arr.Elements) != 3 {
		t.Fatalf("unexpected value: %#v", obj)
	}
	if arr.Elements[0].Type() != SECRET_OBJ {
		t.Fatalf("expected secret type, got %s", arr.Elements[0].Type())
	}
	if arr.Elements[1].(*String).Value != "******34" {
		t.Fatalf("unexpected masked value: %q", arr.Elements[1].Inspect())
	}
	if arr.Elements[2].(*String).Value != "abcd1234" {
		t.Fatalf("unexpected reveal value: %q", arr.Elements[2].Inspect())
	}
}
