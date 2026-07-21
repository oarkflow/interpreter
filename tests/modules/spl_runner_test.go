package interpreter_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	. "github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/lexer"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/parser"

	_ "github.com/oarkflow/interpreter/pkg/builtins"
	_ "github.com/oarkflow/interpreter/pkg/builtins/reactive"
	_ "github.com/oarkflow/interpreter/pkg/builtins/scheduler"
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

func writeSPLFixture(t *testing.T, name, source string) string {
	t.Helper()
	dir, err := os.MkdirTemp(".", "spl-test-")
	if err != nil {
		t.Fatalf("create SPL fixture directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write SPL fixture: %v", err)
	}
	return filepath.ToSlash(path)
}

func mathModuleFixture(t *testing.T) string {
	t.Helper()
	return writeSPLFixture(t, "math.spl", `export let base = 40; export const increment = 2;`)
}

func TestRunTestsBuiltinWithPassScript(t *testing.T) {
	path := writeSPLFixture(t, "pass_assertions.spl", `
assert_true(1 < 2, "basic comparison");
assert_eq(2 + 2, 4, "arithmetic");
assert_eq("ab" + "c", "abc");
test_summary();`)
	result := testEvalModule(fmt.Sprintf(`run_tests(%q);`, path))

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
	path := writeSPLFixture(t, "fail_assertions.spl", `assert_true(false, "intentional failure");`)
	result := testEvalModule(fmt.Sprintf(`run_tests(%q);`, path))

	if !object.IsError(result) {
		t.Fatalf("expected error result, got %T", result)
	}
}

func TestImportFromTestdataFiles(t *testing.T) {
	path := mathModuleFixture(t)
	result := testEvalModule(fmt.Sprintf(`import %q; base + increment;`, path))
	iv, ok := result.(*Integer)
	if !ok {
		t.Fatalf("expected Integer, got %T (%v)", result, result.Inspect())
	}
	if iv.Value != 42 {
		t.Fatalf("expected 42, got %d", iv.Value)
	}
}

func TestImportSelectiveFromSyntax(t *testing.T) {
	path := mathModuleFixture(t)
	result := testEvalModule(fmt.Sprintf(`import {base, increment} from %q; base + increment;`, path))
	iv, ok := result.(*Integer)
	if !ok {
		t.Fatalf("expected Integer, got %T", result)
	}
	if iv.Value != 42 {
		t.Fatalf("expected 42, got %d", iv.Value)
	}
}

func TestImportWildcardAliasSyntax(t *testing.T) {
	path := mathModuleFixture(t)
	result := testEvalModule(fmt.Sprintf(`import * as math from %q; math.base + math.increment;`, path))
	iv, ok := result.(*Integer)
	if !ok {
		t.Fatalf("expected Integer, got %T", result)
	}
	if iv.Value != 42 {
		t.Fatalf("expected 42, got %d", iv.Value)
	}
}

func TestExecFileRelativeImportsFromTestdata(t *testing.T) {
	dir, err := os.MkdirTemp(".", "spl-test-")
	if err != nil {
		t.Fatalf("create SPL fixture directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.WriteFile(filepath.Join(dir, "math.spl"), []byte(`export let answer = 42;`), 0o600); err != nil {
		t.Fatalf("write module fixture: %v", err)
	}
	entry := filepath.Join(dir, "entry.spl")
	if err := os.WriteFile(entry, []byte(`import "math.spl" as math; math.answer;`), 0o600); err != nil {
		t.Fatalf("write entry fixture: %v", err)
	}
	result, err := ExecFile(entry, nil)
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
