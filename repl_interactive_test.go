package interpreter

import "testing"

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
