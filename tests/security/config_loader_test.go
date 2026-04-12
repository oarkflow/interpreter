package interpreter_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/config"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/lexer"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/parser"
	"github.com/oarkflow/interpreter/pkg/sandbox"

	_ "github.com/oarkflow/interpreter/pkg/builtins"
	_ "github.com/oarkflow/interpreter/pkg/builtins/database"
	_ "github.com/oarkflow/interpreter/pkg/builtins/integrations"
	_ "github.com/oarkflow/interpreter/pkg/builtins/reactive"
	_ "github.com/oarkflow/interpreter/pkg/builtins/scheduler"
	_ "github.com/oarkflow/interpreter/pkg/builtins/server"
	_ "github.com/oarkflow/interpreter/pkg/builtins/watcher"
)

func testEvalConfig(input string) Object {
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	env := object.NewEnvironment()
	return eval.Eval(prog, env)
}

func TestParseDotEnv(t *testing.T) {
	m, err := config.ParseDotEnv([]byte("A=1\n#x\nexport TOKEN=abc\n"))
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
	env.ModuleDir = dir
	res := sandbox.WithSandboxRootOverride(dir, func() Object {
		obj, err := config.LoadConfigObjectFromPath(path, "json")
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
	pw, ok := HashGet(h, "password")
	if !ok {
		t.Fatalf("missing password")
	}
	if pw.Type() != SECRET_OBJ {
		t.Fatalf("expected SECRET_OBJ, got %s", pw.Type())
	}
}

func TestSecretBuiltins(t *testing.T) {
	obj := testEvalConfig(`let s = secret("abcd1234"); [s, secret_mask(s, 2), secret_reveal(s)];`)
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
