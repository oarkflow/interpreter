package interpreter_test

import (
	"testing"

	. "github.com/oarkflow/interpreter"
)

func FuzzLexerParserNoPanic(f *testing.F) {
	seeds := []string{
		"let x = 10; x + 2;",
		"if (true) { 1; } else { 2; }",
		"function(x) { x + 1; }(41);",
		"try { throw \"x\"; } catch (e) { e; }",
		"import \"testdata/modules/math.spl\" as m; m.base;",
		"while (false) { 1; }",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		_ = EvalForPlayground(input, PlaygroundOptions{
			MaxDepth:  64,
			MaxSteps:  50000,
			MaxHeapMB: 64,
			TimeoutMS: 500,
			ModuleDir: ".",
		})
	})
}

func FuzzExecWithOptionsNoPanic(f *testing.F) {
	f.Add("1+1;")
	f.Add("let a = [1,2,3]; a[1];")
	f.Add("while (true) { }")

	f.Fuzz(func(t *testing.T, script string) {
		_, _ = ExecWithOptions(script, nil, ExecOptions{
			MaxDepth:  64,
			MaxSteps:  50000,
			MaxHeapMB: 64,
		})
	})
}
