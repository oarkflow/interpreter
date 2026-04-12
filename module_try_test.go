package interpreter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func evalWithParserCheck(t *testing.T, input string, env *Environment) Object {
	t.Helper()
	l := NewLexer(input)
	p := NewParser(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	return Eval(program, env)
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
	result := evalWithParserCheck(t, script, NewEnvironment())
	testIntegerObject(t, result, 42)
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
	result := evalWithParserCheck(t, script, NewEnvironment())
	testIntegerObject(t, result, 42)
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
	result := evalWithParserCheck(t, script, NewEnvironment())
	testBooleanObject(t, result, true)
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
	result := evalWithParserCheck(t, entry, NewEnvironment())
	if !isError(result) {
		t.Fatalf("expected runtime error, got %T", result)
	}
	if !strings.Contains(objectErrorString(result), "module cycle detected") {
		t.Fatalf("unexpected error: %s", objectErrorString(result))
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
	result := evalWithParserCheck(t, script, NewEnvironment())
	testIntegerObject(t, result, 42)
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
	result := evalWithParserCheck(t, script, NewEnvironment())
	testIntegerObject(t, result, 42)
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
	testIntegerObject(t, result, 42)
}

func TestImportTypeError(t *testing.T) {
	result := evalWithParserCheck(t, "import 123;", NewEnvironment())
	if !isError(result) {
		t.Fatalf("expected runtime error, got %T", result)
	}
	if !strings.Contains(objectErrorString(result), "import path must be STRING") {
		t.Fatalf("unexpected error: %s", objectErrorString(result))
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
	result := evalWithParserCheck(t, script, NewEnvironment())
	if !isError(result) {
		t.Fatalf("expected runtime error, got %T", result)
	}
	if !strings.Contains(objectErrorString(result), "module parse error") {
		t.Fatalf("unexpected error: %s", objectErrorString(result))
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
	result := evalWithParserCheck(t, script, NewEnvironment())
	if !isError(result) {
		t.Fatalf("expected runtime error, got %T", result)
	}
	if !strings.Contains(objectErrorString(result), "has no exports") {
		t.Fatalf("unexpected error: %s", objectErrorString(result))
	}
}

func TestImportInvalidPath(t *testing.T) {
	outsidePath := filepath.Join(os.TempDir(), "outside-module.spl")
	if err := os.WriteFile(outsidePath, []byte("export let x = 1;"), 0o600); err != nil {
		t.Fatalf("failed to write external module: %v", err)
	}
	defer os.Remove(outsidePath)

	script := fmt.Sprintf("import %q;", filepath.ToSlash(outsidePath))
	result := evalWithParserCheck(t, script, NewEnvironment())
	if !isError(result) {
		t.Fatalf("expected runtime error, got %T", result)
	}
	if !strings.Contains(objectErrorString(result), "invalid import path") {
		t.Fatalf("unexpected error: %s", objectErrorString(result))
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
		result := evalWithParserCheck(t, tt.input, NewEnvironment())
		if tt.expectInt != nil {
			testIntegerObject(t, result, *tt.expectInt)
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
