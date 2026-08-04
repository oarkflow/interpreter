package lua

import "testing"

func TestLexerLua51Tokens(t *testing.T) {
	source := `-- comment
	local hex = 0xff
	local text = [=[hello
world]=]
	if hex >= 10 and text ~= "" then return hex .. text, ... end`
	l := newLexer(source, "lexer_test")
	want := []tokenKind{
		tLocal, tName, tAssign, tNumber, tLocal, tName, tAssign, tString,
		tIf, tName, tGTE, tNumber, tAnd, tName, tNotEq, tString, tThen,
		tReturn, tName, tConcat, tName, tComma, tDots, tEnd, tEOF,
	}
	for i, expected := range want {
		tok, err := l.next()
		if err != nil {
			t.Fatalf("token %d: %v", i, err)
		}
		if tok.kind != expected {
			t.Fatalf("token %d: got %d (%q), want %d", i, tok.kind, tok.lit, expected)
		}
	}
}

func TestLexerAcceptsBOMAndInterpreterDirective(t *testing.T) {
	results := runLua(t, "\xef\xbb\xbf#!/usr/bin/env lua\nreturn 40 + 2")
	if len(results) != 1 || results[0].Number() != 42 {
		t.Fatalf("results = %#v", results)
	}
}

func TestHybridTableDenseAndStringPaths(t *testing.T) {
	table := NewTable(4, 2)
	for i := 1; i <= 4; i++ {
		if err := table.Set(Number(float64(i)), Number(float64(i*i))); err != nil {
			t.Fatal(err)
		}
	}
	table.SetString("name", String("lua"))
	if table.Len() != 4 {
		t.Fatalf("len = %d", table.Len())
	}
	if got := table.Get(Number(3)).Number(); got != 9 {
		t.Fatalf("table[3] = %v", got)
	}
	if got := table.GetString("name").StringValue(); got != "lua" {
		t.Fatalf("name = %q", got)
	}
}

func BenchmarkTableDenseGet(b *testing.B) {
	table := NewTable(1024, 0)
	for i := 1; i <= 1024; i++ {
		_ = table.Set(Number(float64(i)), Number(float64(i)))
	}
	key := Number(513)
	b.ReportAllocs()
	b.ResetTimer()
	var result Value
	for i := 0; i < b.N; i++ {
		result = table.Get(key)
	}
	_ = result
}

func BenchmarkTableStringGet(b *testing.B) {
	table := NewTable(0, 1)
	table.SetString("answer", Number(42))
	b.ReportAllocs()
	b.ResetTimer()
	var result Value
	for i := 0; i < b.N; i++ {
		result = table.GetString("answer")
	}
	_ = result
}
