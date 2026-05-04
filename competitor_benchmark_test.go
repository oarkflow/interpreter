package interpreter_test

import (
	"testing"

	"github.com/dop251/goja"
	exprlang "github.com/expr-lang/expr"
	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/lexer"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/parser"
	"go.starlark.net/starlark"
)

const (
	competitorLoopLimit = 999

	competitorSPLTightLoop = `
let sum = 0;
for (let i = 0; i <= 999; i = i + 1) {
  sum = sum + i;
}
sum;
`

	competitorGojaTightLoop = `
(function(n) {
  let sum = 0;
  for (let i = 0; i <= n; i++) {
    sum += i;
  }
  return sum;
})(n);
`

	competitorExprTightLoop = `sum(0..n)`

	competitorStarlarkTightLoop = `
def tight_loop(n):
    total = 0
    for i in range(n + 1):
        total = total + i
    return total

result = tight_loop(999)
`
)

var competitorResultSink any

func requireCompetitorResult(b *testing.B, got any) {
	b.Helper()
	const want = 499500

	switch value := got.(type) {
	case *object.Integer:
		if value.Value != want {
			b.Fatalf("unexpected SPL result: got %d, want %d", value.Value, want)
		}
	case goja.Value:
		requireCompetitorResult(b, value.Export())
	case starlark.Int:
		got, ok := value.Int64()
		if !ok || got != want {
			b.Fatalf("unexpected Starlark result: got %s, want %d", value.String(), want)
		}
	case int:
		if value != want {
			b.Fatalf("unexpected int result: got %d, want %d", value, want)
		}
	case int64:
		if value != want {
			b.Fatalf("unexpected int64 result: got %d, want %d", value, want)
		}
	case float64:
		if value != want {
			b.Fatalf("unexpected float64 result: got %f, want %d", value, want)
		}
	default:
		b.Fatalf("unexpected result type %T: %[1]v", got)
	}
}

func parseCompetitorSPL(input string, b *testing.B) *ast.Program {
	b.Helper()
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		b.Fatalf("parser errors: %v", p.Errors())
	}
	return program
}

func compileCompetitorStarlark(b *testing.B) *starlark.Program {
	b.Helper()
	_, program, err := starlark.SourceProgram("competitor_tight_loop.star", competitorStarlarkTightLoop, func(string) bool {
		return false
	})
	if err != nil {
		b.Fatalf("compile starlark: %v", err)
	}
	return program
}

func runCompetitorStarlark(program *starlark.Program, b *testing.B) starlark.Value {
	b.Helper()
	globals, err := program.Init(&starlark.Thread{Name: "competitor-benchmark"}, nil)
	if err != nil {
		b.Fatalf("run starlark: %v", err)
	}
	result, ok := globals["result"]
	if !ok {
		b.Fatalf("starlark result global not found")
	}
	return result
}

func BenchmarkCompetitorSPLTightLoopParseAndRun(b *testing.B) {
	program := parseCompetitorSPL(competitorSPLTightLoop, b)
	requireCompetitorResult(b, eval.Eval(program, object.NewEnvironment()))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		program := parseCompetitorSPL(competitorSPLTightLoop, b)
		competitorResultSink = eval.Eval(program, object.NewEnvironment())
	}
}

func BenchmarkCompetitorSPLTightLoopPreparsed(b *testing.B) {
	program := parseCompetitorSPL(competitorSPLTightLoop, b)
	requireCompetitorResult(b, eval.Eval(program, object.NewEnvironment()))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		competitorResultSink = eval.Eval(program, object.NewEnvironment())
	}
}

func BenchmarkCompetitorGojaTightLoopCompileAndRun(b *testing.B) {
	program, err := goja.Compile("competitor_tight_loop.js", competitorGojaTightLoop, false)
	if err != nil {
		b.Fatalf("compile goja: %v", err)
	}
	vm := goja.New()
	if err := vm.Set("n", competitorLoopLimit); err != nil {
		b.Fatalf("set goja env: %v", err)
	}
	result, err := vm.RunProgram(program)
	if err != nil {
		b.Fatalf("run goja: %v", err)
	}
	requireCompetitorResult(b, result)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		program, err := goja.Compile("competitor_tight_loop.js", competitorGojaTightLoop, false)
		if err != nil {
			b.Fatalf("compile goja: %v", err)
		}
		vm := goja.New()
		if err := vm.Set("n", competitorLoopLimit); err != nil {
			b.Fatalf("set goja env: %v", err)
		}
		result, err := vm.RunProgram(program)
		if err != nil {
			b.Fatalf("run goja: %v", err)
		}
		competitorResultSink = result.Export()
	}
}

func BenchmarkCompetitorGojaTightLoopPrecompiled(b *testing.B) {
	program, err := goja.Compile("competitor_tight_loop.js", competitorGojaTightLoop, false)
	if err != nil {
		b.Fatalf("compile goja: %v", err)
	}
	vm := goja.New()
	if err := vm.Set("n", competitorLoopLimit); err != nil {
		b.Fatalf("set goja env: %v", err)
	}
	result, err := vm.RunProgram(program)
	if err != nil {
		b.Fatalf("run goja: %v", err)
	}
	requireCompetitorResult(b, result)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		vm := goja.New()
		if err := vm.Set("n", competitorLoopLimit); err != nil {
			b.Fatalf("set goja env: %v", err)
		}
		result, err := vm.RunProgram(program)
		if err != nil {
			b.Fatalf("run goja: %v", err)
		}
		competitorResultSink = result.Export()
	}
}

func BenchmarkCompetitorExprTightLoopCompileAndRun(b *testing.B) {
	env := map[string]any{"n": competitorLoopLimit}
	program, err := exprlang.Compile(competitorExprTightLoop, exprlang.Env(env))
	if err != nil {
		b.Fatalf("compile expr: %v", err)
	}
	result, err := exprlang.Run(program, env)
	if err != nil {
		b.Fatalf("run expr: %v", err)
	}
	requireCompetitorResult(b, result)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		program, err := exprlang.Compile(competitorExprTightLoop, exprlang.Env(env))
		if err != nil {
			b.Fatalf("compile expr: %v", err)
		}
		result, err := exprlang.Run(program, env)
		if err != nil {
			b.Fatalf("run expr: %v", err)
		}
		competitorResultSink = result
	}
}

func BenchmarkCompetitorExprTightLoopPrecompiled(b *testing.B) {
	env := map[string]any{"n": competitorLoopLimit}
	program, err := exprlang.Compile(competitorExprTightLoop, exprlang.Env(env))
	if err != nil {
		b.Fatalf("compile expr: %v", err)
	}
	result, err := exprlang.Run(program, env)
	if err != nil {
		b.Fatalf("run expr: %v", err)
	}
	requireCompetitorResult(b, result)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result, err := exprlang.Run(program, env)
		if err != nil {
			b.Fatalf("run expr: %v", err)
		}
		competitorResultSink = result
	}
}

func BenchmarkCompetitorStarlarkTightLoopCompileAndRun(b *testing.B) {
	program := compileCompetitorStarlark(b)
	requireCompetitorResult(b, runCompetitorStarlark(program, b))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		program := compileCompetitorStarlark(b)
		competitorResultSink = runCompetitorStarlark(program, b)
	}
}

func BenchmarkCompetitorStarlarkTightLoopPrecompiled(b *testing.B) {
	program := compileCompetitorStarlark(b)
	requireCompetitorResult(b, runCompetitorStarlark(program, b))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		competitorResultSink = runCompetitorStarlark(program, b)
	}
}

func BenchmarkCompetitorNativeGoTightLoop(b *testing.B) {
	sum := 0
	for j := 0; j <= competitorLoopLimit; j++ {
		sum += j
	}
	requireCompetitorResult(b, sum)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sum := 0
		for j := 0; j <= competitorLoopLimit; j++ {
			sum += j
		}
		competitorResultSink = sum
	}
}
