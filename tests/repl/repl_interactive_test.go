package interpreter_test

import (
	"strings"
	"testing"

	. "github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/repl"

	_ "github.com/oarkflow/interpreter/pkg/builtins"
	_ "github.com/oarkflow/interpreter/pkg/builtins/database"
	_ "github.com/oarkflow/interpreter/pkg/builtins/integrations"
	_ "github.com/oarkflow/interpreter/pkg/builtins/reactive"
	_ "github.com/oarkflow/interpreter/pkg/builtins/scheduler"
	_ "github.com/oarkflow/interpreter/pkg/builtins/server"
	_ "github.com/oarkflow/interpreter/pkg/builtins/watcher"
)

func TestLongestCommonPrefix(t *testing.T) {
	if got := repl.LongestCommonPrefix([]string{"parse_int", "parse_float", "parse_type"}); got != "parse_" {
		t.Fatalf("unexpected prefix: %q", got)
	}
	if got := repl.LongestCommonPrefix([]string{"upper"}); got != "upper" {
		t.Fatalf("unexpected single prefix: %q", got)
	}
	if got := repl.LongestCommonPrefix(nil); got != "" {
		t.Fatalf("unexpected empty prefix: %q", got)
	}
}

func TestCurrentToken(t *testing.T) {
	buf := []rune("user.name.upper")
	prefix, start, end, ok := repl.CurrentToken(buf, len(buf))
	if !ok {
		t.Fatalf("expected token")
	}
	if prefix != "upper" {
		t.Fatalf("unexpected prefix: %q", prefix)
	}
	if start != 10 || end != 15 {
		t.Fatalf("unexpected span: %d..%d", start, end)
	}
}

func TestFindCompletions(t *testing.T) {
	out := repl.FindCompletions("par", []string{"parse_int", "parse_float", "upper"})
	if len(out) != 2 {
		t.Fatalf("unexpected match count: %d", len(out))
	}
	if out[0] != "parse_int" || out[1] != "parse_float" {
		t.Fatalf("unexpected matches: %#v", out)
	}
}

func TestReplNeedsContinuation(t *testing.T) {
	if !repl.ReplNeedsContinuation("let x =") {
		t.Fatalf("expected continuation for trailing operator")
	}
	if !repl.ReplNeedsContinuation("if x > 1 {") {
		t.Fatalf("expected continuation for open brace")
	}
	if repl.ReplNeedsContinuation("let x = 1;") {
		t.Fatalf("did not expect continuation for complete statement")
	}
}

func TestCompletionContextDotAccess(t *testing.T) {
	buf := []rune("user.na")
	ctx := repl.CompletionContext(buf, len(buf))
	if !ctx.Ok {
		t.Fatalf("expected context to be ok")
	}
	if ctx.Prefix != "na" {
		t.Fatalf("unexpected prefix: %q", ctx.Prefix)
	}
	if ctx.BaseExpr != "user" {
		t.Fatalf("unexpected base expression: %q", ctx.BaseExpr)
	}
}

func TestCompletionsForContextUsesObjectFieldsAndMethods(t *testing.T) {
	env := NewGlobalEnvironment(nil)
	env.Set("user", &Hash{Pairs: map[HashKey]HashPair{(&String{Value: "name"}).HashKey(): {Key: &String{Value: "name"}, Value: &String{Value: "sujit"}}}})
	e := &repl.ReplEditor{Env: env, Candidates: []string{"print", "let"}}
	ctx := repl.ReplCompletionContext{Prefix: "na", BaseExpr: "user", Ok: true}
	out := e.CompletionsForContext(ctx)
	joined := strings.Join(out, " ")
	if !strings.Contains(joined, "name") || !strings.Contains(joined, "keys") {
		t.Fatalf("unexpected semantic completions: %v", out)
	}
}

func TestReverseHistorySearch(t *testing.T) {
	e := &repl.ReplEditor{History: []string{"print(1)", "let user = {}", "print(user)"}, HistoryPos: 3}
	out, cursor := e.ReverseHistorySearch([]rune("user"))
	if string(out) != "print(user)" {
		t.Fatalf("unexpected history match: %q", string(out))
	}
	if cursor != len([]rune("print(user)")) {
		t.Fatalf("unexpected cursor: %d", cursor)
	}
}

func TestReplCallTipBuiltin(t *testing.T) {
	tip := repl.ReplCallTip("help(", len([]rune("help(")), nil)
	if !strings.Contains(tip, "builtin") && !strings.Contains(tip, "prints") {
		t.Fatalf("unexpected call tip: %q", tip)
	}
}

func TestFormatRuntimeErrorForDisplay(t *testing.T) {
	errObj := &Error{Message: "boom", Code: "E_RUNTIME", Line: 1, Column: 6}
	text := repl.FormatRuntimeErrorForDisplay(errObj, "print(\n")
	if !strings.Contains(text, "E_RUNTIME") || !strings.Contains(text, "Location: line 1") {
		t.Fatalf("unexpected runtime error display: %q", text)
	}
}

func TestReplMemoryUsage(t *testing.T) {
	out := repl.ReplMemoryUsage()
	if !strings.Contains(out, "alloc=") || !strings.Contains(out, "num_gc=") {
		t.Fatalf("unexpected memory usage output: %q", out)
	}
}
