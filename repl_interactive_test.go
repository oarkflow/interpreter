package interpreter

import (
	"strings"
	"testing"
)

func TestLongestCommonPrefix(t *testing.T) {
	if got := longestCommonPrefix([]string{"parse_int", "parse_float", "parse_type"}); got != "parse_" {
		t.Fatalf("unexpected prefix: %q", got)
	}
	if got := longestCommonPrefix([]string{"upper"}); got != "upper" {
		t.Fatalf("unexpected single prefix: %q", got)
	}
	if got := longestCommonPrefix(nil); got != "" {
		t.Fatalf("unexpected empty prefix: %q", got)
	}
}

func TestCurrentToken(t *testing.T) {
	buf := []rune("user.name.upper")
	prefix, start, end, ok := currentToken(buf, len(buf))
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
	out := findCompletions("par", []string{"parse_int", "parse_float", "upper"})
	if len(out) != 2 {
		t.Fatalf("unexpected match count: %d", len(out))
	}
	if out[0] != "parse_int" || out[1] != "parse_float" {
		t.Fatalf("unexpected matches: %#v", out)
	}
}

func TestReplNeedsContinuation(t *testing.T) {
	if !replNeedsContinuation("let x =") {
		t.Fatalf("expected continuation for trailing operator")
	}
	if !replNeedsContinuation("if x > 1 {") {
		t.Fatalf("expected continuation for open brace")
	}
	if replNeedsContinuation("let x = 1;") {
		t.Fatalf("did not expect continuation for complete statement")
	}
}

func TestCompletionContextDotAccess(t *testing.T) {
	buf := []rune("user.na")
	ctx := completionContext(buf, len(buf))
	if !ctx.ok {
		t.Fatalf("expected context to be ok")
	}
	if ctx.prefix != "na" {
		t.Fatalf("unexpected prefix: %q", ctx.prefix)
	}
	if ctx.baseExpr != "user" {
		t.Fatalf("unexpected base expression: %q", ctx.baseExpr)
	}
}

func TestCompletionsForContextUsesObjectFieldsAndMethods(t *testing.T) {
	env := NewGlobalEnvironment(nil)
	env.Set("user", &Hash{Pairs: map[HashKey]HashPair{(&String{Value: "name"}).HashKey(): {Key: &String{Value: "name"}, Value: &String{Value: "sujit"}}}})
	e := &replEditor{env: env, candidates: []string{"print", "let"}}
	ctx := replCompletionContext{prefix: "na", baseExpr: "user", ok: true}
	out := e.completionsForContext(ctx)
	joined := strings.Join(out, " ")
	if !strings.Contains(joined, "name") || !strings.Contains(joined, "keys") {
		t.Fatalf("unexpected semantic completions: %v", out)
	}
}

func TestReverseHistorySearch(t *testing.T) {
	e := &replEditor{history: []string{"print(1)", "let user = {}", "print(user)"}, historyPos: 3}
	out, cursor := e.reverseHistorySearch([]rune("user"))
	if string(out) != "print(user)" {
		t.Fatalf("unexpected history match: %q", string(out))
	}
	if cursor != len([]rune("print(user)")) {
		t.Fatalf("unexpected cursor: %d", cursor)
	}
}

func TestReplCallTipBuiltin(t *testing.T) {
	tip := replCallTip("help(", len([]rune("help(")), nil)
	if !strings.Contains(tip, "builtin") && !strings.Contains(tip, "prints") {
		t.Fatalf("unexpected call tip: %q", tip)
	}
}

func TestFormatRuntimeErrorForDisplay(t *testing.T) {
	errObj := &Error{Message: "boom", Code: "E_RUNTIME", Line: 1, Column: 6}
	text := formatRuntimeErrorForDisplay(errObj, "print(\n")
	if !strings.Contains(text, "E_RUNTIME") || !strings.Contains(text, "Location: line 1") {
		t.Fatalf("unexpected runtime error display: %q", text)
	}
}

func TestReplMemoryUsage(t *testing.T) {
	out := replMemoryUsage()
	if !strings.Contains(out, "alloc=") || !strings.Contains(out, "num_gc=") {
		t.Fatalf("unexpected memory usage output: %q", out)
	}
}
