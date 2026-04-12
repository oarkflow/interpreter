package interpreter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplLineContextHasAlignedSourceBlock(t *testing.T) {
	ctx := lineContext("if x > 5 {\nprint \"Hello\"", 1, 4)
	if strings.HasPrefix(ctx, " Source:") {
		t.Fatalf("unexpected leading indentation in line context: %q", ctx)
	}
	if !strings.Contains(ctx, "Source: if x > 5 {") {
		t.Fatalf("missing source line in context: %q", ctx)
	}
	if !strings.Contains(ctx, "\n           ^") {
		t.Fatalf("missing aligned caret line: %q", ctx)
	}
}

func TestFormatObjectPlainPrettyPrintsNestedValues(t *testing.T) {
	obj := &Hash{Pairs: map[HashKey]HashPair{}}
	keyB := &String{Value: "b"}
	keyA := &String{Value: "a"}
	obj.Pairs[keyB.HashKey()] = HashPair{Key: keyB, Value: &Array{Elements: []Object{&Integer{Value: 1}, &Integer{Value: 2}}}}
	obj.Pairs[keyA.HashKey()] = HashPair{Key: keyA, Value: &Hash{Pairs: map[HashKey]HashPair{(&String{Value: "x"}).HashKey(): {Key: &String{Value: "x"}, Value: &Boolean{Value: true}}}}}

	got := formatObjectPlain(obj)
	if !strings.Contains(got, "a: {") || !strings.Contains(got, "b: [") {
		t.Fatalf("expected nested pretty output, got %q", got)
	}
	if strings.Index(got, "a: {") > strings.Index(got, "b: [") {
		t.Fatalf("expected stable sorted keys, got %q", got)
	}
}

func TestReplCandidatesIncludeVariablesAndCommands(t *testing.T) {
	env := NewGlobalEnvironment(nil)
	env.Set("userName", &String{Value: "sujit"})
	got := replCandidatesForEnv(env)
	joined := strings.Join(got, " ")
	for _, want := range []string{"userName", ":doc", ":methods", ":fields", ":ast", ":load", ":reload"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing candidate %q in %v", want, got)
		}
	}
}

func TestReplDocTextForBuiltinAndVariable(t *testing.T) {
	env := NewGlobalEnvironment(nil)
	env.Set("cfg", &Hash{Pairs: map[HashKey]HashPair{(&String{Value: "port"}).HashKey(): {Key: &String{Value: "port"}, Value: &Integer{Value: 8080}}}})

	if got := replDocText("help", env); !strings.Contains(got, "help() lists builtin names") {
		t.Fatalf("unexpected builtin doc: %q", got)
	}
	if got := replDocText("cfg", env); !strings.Contains(got, "cfg: HASH") || !strings.Contains(got, "port: 8080") {
		t.Fatalf("unexpected variable doc: %q", got)
	}
}

func TestReplObjectMethodsAndFields(t *testing.T) {
	methods := replObjectMethods(&Array{})
	fields := replObjectFields(&Hash{Pairs: map[HashKey]HashPair{(&String{Value: "name"}).HashKey(): {Key: &String{Value: "name"}, Value: &String{Value: "spl"}}}})
	if !strings.Contains(strings.Join(methods, " "), "map") {
		t.Fatalf("expected array methods to include map, got %v", methods)
	}
	if len(fields) != 1 || fields[0] != "name" {
		t.Fatalf("expected hash field list, got %v", fields)
	}
}

func TestReplResolvedPathUsesModuleDir(t *testing.T) {
	dir, err := os.MkdirTemp(".", "repl-module-test-")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "math.spl")
	if err := os.WriteFile(path, []byte("export let answer = 42;"), 0o644); err != nil {
		t.Fatalf("write module: %v", err)
	}
	env := NewGlobalEnvironment(nil)
	env.moduleDir = dir
	resolved, err := replResolvedPath("math.spl", env)
	if err != nil {
		t.Fatalf("resolve path: %v", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	if resolved != absPath {
		t.Fatalf("unexpected resolved path: got=%q want=%q", resolved, absPath)
	}
}
