package interpreter_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/lexer"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/parser"

	_ "github.com/oarkflow/interpreter/pkg/builtins"
	_ "github.com/oarkflow/interpreter/pkg/builtins/database"
	_ "github.com/oarkflow/interpreter/pkg/builtins/integrations"
	_ "github.com/oarkflow/interpreter/pkg/builtins/reactive"
	_ "github.com/oarkflow/interpreter/pkg/builtins/scheduler"
	_ "github.com/oarkflow/interpreter/pkg/builtins/server"
	_ "github.com/oarkflow/interpreter/pkg/builtins/watcher"
)

// projectRoot returns the absolute path to the project root (two levels up from tests/modules/).
func projectRoot() string {
	abs, _ := filepath.Abs("../..")
	return abs
}

func TestMain(m *testing.M) {
	// Change to project root so testdata/ paths resolve correctly.
	if err := os.Chdir(projectRoot()); err != nil {
		panic("chdir to project root: " + err.Error())
	}
	os.Exit(m.Run())
}

func testEvalModule(input string) Object {
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	env := object.NewEnvironment()
	return eval.Eval(prog, env)
}

func TestRunTestsBuiltinWithPassScript(t *testing.T) {
	result := testEvalModule(`run_tests("testdata/tests/pass_assertions.spl");`)

	hash, ok := result.(*Hash)
	if !ok {
		t.Fatalf("expected Hash result, got %T", result)
	}

	getInt := func(key string) int64 {
		t.Helper()
		k := (&String{Value: key}).HashKey()
		pair, exists := hash.Pairs[k]
		if !exists {
			t.Fatalf("missing key %q in summary", key)
		}
		iv, ok := pair.Value.(*Integer)
		if !ok {
			t.Fatalf("summary key %q is not Integer: %T", key, pair.Value)
		}
		return iv.Value
	}

	if getInt("total") != 3 {
		t.Fatalf("unexpected total")
	}
	if getInt("passed") != 3 {
		t.Fatalf("unexpected passed")
	}
	if getInt("failed") != 0 {
		t.Fatalf("unexpected failed")
	}
}

func TestRunTestsBuiltinWithFailScript(t *testing.T) {
	result := testEvalModule(`run_tests("testdata/tests/fail_assertions.spl");`)

	if !object.IsError(result) {
		t.Fatalf("expected error result, got %T", result)
	}
}

func TestImportFromTestdataFiles(t *testing.T) {
	result := testEvalModule(`import "testdata/modules/math.spl"; base + increment;`)
	iv, ok := result.(*Integer)
	if !ok {
		t.Fatalf("expected Integer, got %T (%v)", result, result.Inspect())
	}
	if iv.Value != 42 {
		t.Fatalf("expected 42, got %d", iv.Value)
	}
}

func TestImportSelectiveFromSyntax(t *testing.T) {
	result := testEvalModule(`import {base, increment} from "testdata/modules/math.spl"; base + increment;`)
	iv, ok := result.(*Integer)
	if !ok {
		t.Fatalf("expected Integer, got %T", result)
	}
	if iv.Value != 42 {
		t.Fatalf("expected 42, got %d", iv.Value)
	}
}

func TestImportWildcardAliasSyntax(t *testing.T) {
	result := testEvalModule(`import * as math from "testdata/modules/math.spl"; math.base + math.increment;`)
	iv, ok := result.(*Integer)
	if !ok {
		t.Fatalf("expected Integer, got %T", result)
	}
	if iv.Value != 42 {
		t.Fatalf("expected 42, got %d", iv.Value)
	}
}

func TestExecFileRelativeImportsFromTestdata(t *testing.T) {
	result, err := ExecFile("testdata/modules/entry_relative_import.spl", nil)
	if err != nil {
		t.Fatalf("ExecFile failed: %v", err)
	}
	iv, ok := result.(*Integer)
	if !ok {
		t.Fatalf("expected Integer, got %T", result)
	}
	if iv.Value != 42 {
		t.Fatalf("expected 42, got %d", iv.Value)
	}
}
