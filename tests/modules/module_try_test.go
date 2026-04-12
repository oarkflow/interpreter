package interpreter_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func evalWithParserCheck(t *testing.T, input string, env *object.Environment) Object {
	t.Helper()
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	return eval.Eval(program, env)
}

func TestImportExportHappyPath(t *testing.T) {
	moduleDir, err := os.MkdirTemp(".", "module-test-")
	if err != nil {
		t.Fatalf("failed to create module dir: %v", err)
	}
	defer os.RemoveAll(moduleDir)

	modulePath := filepath.Join(moduleDir, "math.spl")
	moduleContent := "export let base = 40; export const increment = 2;"
	if err := os.WriteFile(modulePath, []byte(moduleContent), 0o600); err != nil {
		t.Fatalf("failed to write module: %v", err)
	}

	relPath, err := filepath.Rel(".", modulePath)
	if err != nil {
		t.Fatalf("failed to create relative path: %v", err)
	}

	script := fmt.Sprintf("import %q; base + increment;", filepath.ToSlash(relPath))
	result := evalWithParserCheck(t, script, object.NewEnvironment())
	iv, ok := result.(*Integer)
	if !ok {
		t.Fatalf("expected Integer, got %T", result)
	}
	if iv.Value != 42 {
		t.Fatalf("expected 42, got %d", iv.Value)
	}
}

func TestImportWithAlias(t *testing.T) {
	moduleDir, err := os.MkdirTemp(".", "module-alias-test-")
	if err != nil {
		t.Fatalf("failed to create module dir: %v", err)
	}
	defer os.RemoveAll(moduleDir)

	modulePath := filepath.Join(moduleDir, "math.spl")
	moduleContent := "export let base = 40; export const increment = 2;"
	if err := os.WriteFile(modulePath, []byte(moduleContent), 0o600); err != nil {
		t.Fatalf("failed to write module: %v", err)
	}

	relPath, err := filepath.Rel(".", modulePath)
	if err != nil {
		t.Fatalf("failed to create relative path: %v", err)
	}

	script := fmt.Sprintf("import %q as math; math.base + math.increment;", filepath.ToSlash(relPath))
	result := evalWithParserCheck(t, script, object.NewEnvironment())
	iv, ok := result.(*Integer)
	if !ok {
		t.Fatalf("expected Integer, got %T", result)
	}
	if iv.Value != 42 {
		t.Fatalf("expected 42, got %d", iv.Value)
	}
}

func TestImportUsesModuleCache(t *testing.T) {
	moduleDir, err := os.MkdirTemp(".", "module-cache-test-")
	if err != nil {
		t.Fatalf("failed to create module dir: %v", err)
	}
	defer os.RemoveAll(moduleDir)

	modulePath := filepath.Join(moduleDir, "token.spl")
	moduleContent := "export let uid = uuid();"
	if err := os.WriteFile(modulePath, []byte(moduleContent), 0o600); err != nil {
		t.Fatalf("failed to write module: %v", err)
	}

	relPath, err := filepath.Rel(".", modulePath)
	if err != nil {
		t.Fatalf("failed to create relative path: %v", err)
	}

	script := fmt.Sprintf("import %q; let first = uid; import %q; first == uid;", filepath.ToSlash(relPath), filepath.ToSlash(relPath))
	result := evalWithParserCheck(t, script, object.NewEnvironment())
	bv, ok := result.(*Boolean)
	if !ok {
		t.Fatalf("expected Boolean, got %T", result)
	}
	if !bv.Value {
		t.Fatalf("expected true, got false")
	}
}

func TestImportCycleDetection(t *testing.T) {
	moduleDir, err := os.MkdirTemp(".", "module-cycle-test-")
	if err != nil {
		t.Fatalf("failed to create module dir: %v", err)
	}
	defer os.RemoveAll(moduleDir)

	aPath := filepath.Join(moduleDir, "a.spl")
	bPath := filepath.Join(moduleDir, "b.spl")

	relA, err := filepath.Rel(".", aPath)
	if err != nil {
		t.Fatalf("failed to create a relative path: %v", err)
	}
	relB, err := filepath.Rel(".", bPath)
	if err != nil {
		t.Fatalf("failed to create b relative path: %v", err)
	}

	aScript := fmt.Sprintf("import %q; export let a = 1;", filepath.ToSlash(relB))
	bScript := fmt.Sprintf("import %q; export let b = 2;", filepath.ToSlash(relA))

	if err := os.WriteFile(aPath, []byte(aScript), 0o600); err != nil {
		t.Fatalf("failed to write a module: %v", err)
	}
	if err := os.WriteFile(bPath, []byte(bScript), 0o600); err != nil {
		t.Fatalf("failed to write b module: %v", err)
	}

	entry := fmt.Sprintf("import %q;", filepath.ToSlash(relA))
	result := evalWithParserCheck(t, entry, object.NewEnvironment())
	if !object.IsError(result) {
		t.Fatalf("expected runtime error, got %T", result)
	}
	if !strings.Contains(ObjectErrorString(result), "module cycle detected") {
		t.Fatalf("unexpected error: %s", ObjectErrorString(result))
	}
}

func TestImportSelectiveFrom(t *testing.T) {
	moduleDir, err := os.MkdirTemp(".", "module-selective-test-")
	if err != nil {
		t.Fatalf("failed to create module dir: %v", err)
	}
	defer os.RemoveAll(moduleDir)

	modulePath := filepath.Join(moduleDir, "math.spl")
	moduleContent := "export let base = 40; export const increment = 2;"
	if err := os.WriteFile(modulePath, []byte(moduleContent), 0o600); err != nil {
		t.Fatalf("failed to write module: %v", err)
	}

	relPath, err := filepath.Rel(".", modulePath)
	if err != nil {
		t.Fatalf("failed to create relative path: %v", err)
	}

	script := fmt.Sprintf("import {base, increment} from %q; base + increment;", filepath.ToSlash(relPath))
	result := evalWithParserCheck(t, script, object.NewEnvironment())
	iv, ok := result.(*Integer)
	if !ok {
		t.Fatalf("expected Integer, got %T", result)
	}
	if iv.Value != 42 {
		t.Fatalf("expected 42, got %d", iv.Value)
	}
}

func TestImportWildcardAliasFrom(t *testing.T) {
	moduleDir, err := os.MkdirTemp(".", "module-wildcard-test-")
	if err != nil {
		t.Fatalf("failed to create module dir: %v", err)
	}
	defer os.RemoveAll(moduleDir)

	modulePath := filepath.Join(moduleDir, "math.spl")
	moduleContent := "export let base = 40; export const increment = 2;"
	if err := os.WriteFile(modulePath, []byte(moduleContent), 0o600); err != nil {
		t.Fatalf("failed to write module: %v", err)
	}

	relPath, err := filepath.Rel(".", modulePath)
	if err != nil {
		t.Fatalf("failed to create relative path: %v", err)
	}

	script := fmt.Sprintf("import * as math from %q; math.base + math.increment;", filepath.ToSlash(relPath))
	result := evalWithParserCheck(t, script, object.NewEnvironment())
	iv, ok := result.(*Integer)
	if !ok {
		t.Fatalf("expected Integer, got %T", result)
	}
	if iv.Value != 42 {
		t.Fatalf("expected 42, got %d", iv.Value)
	}
}

func TestNestedRelativeImport(t *testing.T) {
	moduleDir, err := os.MkdirTemp(".", "module-relative-test-")
	if err != nil {
		t.Fatalf("failed to create module dir: %v", err)
	}
	defer os.RemoveAll(moduleDir)

	libDir := filepath.Join(moduleDir, "lib")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatalf("failed to create lib dir: %v", err)
	}

	commonPath := filepath.Join(libDir, "common.spl")
	if err := os.WriteFile(commonPath, []byte("export const offset = 2;"), 0o600); err != nil {
		t.Fatalf("failed to write common module: %v", err)
	}

	mathPath := filepath.Join(libDir, "math.spl")
	if err := os.WriteFile(mathPath, []byte("import \"common.spl\"; export let answer = 40 + offset;"), 0o600); err != nil {
		t.Fatalf("failed to write math module: %v", err)
	}

	entryPath := filepath.Join(moduleDir, "entry.spl")
	entryScript := "import \"lib/math.spl\" as math; math.answer;"
	if err := os.WriteFile(entryPath, []byte(entryScript), 0o600); err != nil {
		t.Fatalf("failed to write entry script: %v", err)
	}

	result, err := ExecFile(entryPath, nil)
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

func TestImportTypeError(t *testing.T) {
	result := evalWithParserCheck(t, "import 123;", object.NewEnvironment())
	if !object.IsError(result) {
		t.Fatalf("expected runtime error, got %T", result)
	}
	if !strings.Contains(ObjectErrorString(result), "import path must be STRING") {
		t.Fatalf("unexpected error: %s", ObjectErrorString(result))
	}
}

func TestImportModuleParseError(t *testing.T) {
	moduleDir, err := os.MkdirTemp(".", "module-parse-test-")
	if err != nil {
		t.Fatalf("failed to create module dir: %v", err)
	}
	defer os.RemoveAll(moduleDir)

	modulePath := filepath.Join(moduleDir, "broken.spl")
	if err := os.WriteFile(modulePath, []byte("export let x = ;"), 0o600); err != nil {
		t.Fatalf("failed to write module: %v", err)
	}

	relPath, err := filepath.Rel(".", modulePath)
	if err != nil {
		t.Fatalf("failed to create relative path: %v", err)
	}

	script := fmt.Sprintf("import %q;", filepath.ToSlash(relPath))
	result := evalWithParserCheck(t, script, object.NewEnvironment())
	if !object.IsError(result) {
		t.Fatalf("expected runtime error, got %T", result)
	}
	if !strings.Contains(ObjectErrorString(result), "module parse error") {
		t.Fatalf("unexpected error: %s", ObjectErrorString(result))
	}
}

func TestImportMissingExports(t *testing.T) {
	moduleDir, err := os.MkdirTemp(".", "module-empty-test-")
	if err != nil {
		t.Fatalf("failed to create module dir: %v", err)
	}
	defer os.RemoveAll(moduleDir)

	modulePath := filepath.Join(moduleDir, "plain.spl")
	if err := os.WriteFile(modulePath, []byte("let hidden = 1;"), 0o600); err != nil {
		t.Fatalf("failed to write module: %v", err)
	}

	relPath, err := filepath.Rel(".", modulePath)
	if err != nil {
		t.Fatalf("failed to create relative path: %v", err)
	}

	script := fmt.Sprintf("import %q;", filepath.ToSlash(relPath))
	result := evalWithParserCheck(t, script, object.NewEnvironment())
	if !object.IsError(result) {
		t.Fatalf("expected runtime error, got %T", result)
	}
	if !strings.Contains(ObjectErrorString(result), "has no exports") {
		t.Fatalf("unexpected error: %s", ObjectErrorString(result))
	}
}

func TestImportInvalidPath(t *testing.T) {
	outsidePath := filepath.Join(os.TempDir(), "outside-module.spl")
	if err := os.WriteFile(outsidePath, []byte("export let x = 1;"), 0o600); err != nil {
		t.Fatalf("failed to write external module: %v", err)
	}
	defer os.Remove(outsidePath)

	script := fmt.Sprintf("import %q;", filepath.ToSlash(outsidePath))
	result := evalWithParserCheck(t, script, object.NewEnvironment())
	if !object.IsError(result) {
		t.Fatalf("expected runtime error, got %T", result)
	}
	if !strings.Contains(ObjectErrorString(result), "invalid import path") {
		t.Fatalf("unexpected error: %s", ObjectErrorString(result))
	}
}

func TestTryCatchThrowBehavior(t *testing.T) {
	tests := []struct {
		input        string
		expectInt    *int64
		expectString string
	}{
		{input: "try { 1 + 2; } catch (e) { 0; }", expectInt: int64Ptr(3)},
		{input: "try { throw \"boom\"; } catch (e) { e; }", expectString: "boom"},
		{input: "try { throw 5; } catch (e) { e; }", expectString: "5"},
		{input: "try { unknown_symbol; } catch (e) { e; }", expectString: "identifier not found: unknown_symbol"},
	}

	for _, tt := range tests {
		result := evalWithParserCheck(t, tt.input, object.NewEnvironment())
		if tt.expectInt != nil {
			iv, ok := result.(*Integer)
			if !ok {
				t.Fatalf("expected Integer, got %T", result)
			}
			if iv.Value != *tt.expectInt {
				t.Fatalf("expected %d, got %d", *tt.expectInt, iv.Value)
			}
			continue
		}
		str, ok := result.(*String)
		if !ok {
			t.Fatalf("expected String, got %T", result)
		}
		if str.Value != tt.expectString {
			t.Fatalf("unexpected catch value: got=%q want=%q", str.Value, tt.expectString)
		}
	}
}

func int64Ptr(v int64) *int64 {
	return &v
}
