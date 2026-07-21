package exprcompare_test

import (
	"math"
	"testing"

	exprlang "github.com/expr-lang/expr"
	"github.com/oarkflow/interpreter/pkg/ast"
	_ "github.com/oarkflow/interpreter/pkg/builtins"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/lexer"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/parser"
)

type exprComparisonWorkload struct {
	splSource  string
	exprSource string
	env        map[string]any
	want       any
}

var exprComparisonSink any

func BenchmarkExprComparisonRule(b *testing.B) {
	benchmarkExprComparison(b, exprComparisonWorkload{
		splSource:  `amount > 100000 && department in ["finance", "procurement"] && risk_score >= 70`,
		exprSource: `amount > 100000 && department in ["finance", "procurement"] && risk_score >= 70`,
		env: map[string]any{
			"amount":     125000,
			"department": "finance",
			"risk_score": 72,
		},
		want: true,
	})
}

func BenchmarkExprComparisonArithmetic(b *testing.B) {
	benchmarkExprComparison(b, exprComparisonWorkload{
		splSource:  `((price * quantity) - discount) * (1 + tax_rate)`,
		exprSource: `((price * quantity) - discount) * (1 + tax_rate)`,
		env: map[string]any{
			"price":    25.0,
			"quantity": 24.0,
			"discount": 50.0,
			"tax_rate": 0.2,
		},
		want: 660.0,
	})
}

func BenchmarkExprComparisonCollection(b *testing.B) {
	values := make([]int, 100)
	for i := range values {
		values[i] = i + 1
	}
	benchmarkExprComparison(b, exprComparisonWorkload{
		splSource:  `sum(values)`,
		exprSource: `sum(values)`,
		env:        map[string]any{"values": values},
		want:       int64(5050),
	})
}

func benchmarkExprComparison(b *testing.B, workload exprComparisonWorkload) {
	b.Helper()

	b.Run("CompileAndRun", func(b *testing.B) {
		b.Run("SPL", func(b *testing.B) {
			validateSPLComparisonResult(b, parseAndRunSPLComparison(b, workload.splSource, workload.env), workload.want)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				exprComparisonSink = parseAndRunSPLComparison(b, workload.splSource, workload.env)
			}
		})

		b.Run("Expr", func(b *testing.B) {
			validateExprComparisonResult(b, compileAndRunExprComparison(b, workload.exprSource, workload.env), workload.want)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				exprComparisonSink = compileAndRunExprComparison(b, workload.exprSource, workload.env)
			}
		})
	})

	b.Run("Prepared", func(b *testing.B) {
		splProgram := parseSPLComparison(b, workload.splSource)
		splEnv := object.NewEnvironment()
		eval.InjectData(splEnv, workload.env)
		splEnv.SealBindings()
		validateSPLComparisonResult(b, eval.Eval(splProgram, splEnv), workload.want)

		exprProgram, err := exprlang.Compile(workload.exprSource, exprlang.Env(workload.env))
		if err != nil {
			b.Fatalf("compile Expr workload: %v", err)
		}
		exprResult, err := exprlang.Run(exprProgram, workload.env)
		if err != nil {
			b.Fatalf("run Expr workload: %v", err)
		}
		validateExprComparisonResult(b, exprResult, workload.want)

		b.Run("SPL", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				exprComparisonSink = eval.Eval(splProgram, splEnv)
			}
		})

		b.Run("Expr", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result, err := exprlang.Run(exprProgram, workload.env)
				if err != nil {
					b.Fatalf("run Expr workload: %v", err)
				}
				exprComparisonSink = result
			}
		})
	})
}

func parseAndRunSPLComparison(b *testing.B, source string, data map[string]any) object.Object {
	b.Helper()
	program := parseSPLComparison(b, source)
	env := object.NewEnvironment()
	eval.InjectData(env, data)
	return eval.Eval(program, env)
}

func parseSPLComparison(b *testing.B, source string) *ast.Program {
	b.Helper()
	p := parser.NewParser(lexer.NewLexer(source))
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		b.Fatalf("parse SPL workload: %v", p.Errors())
	}
	return program
}

func compileAndRunExprComparison(b *testing.B, source string, env map[string]any) any {
	b.Helper()
	program, err := exprlang.Compile(source, exprlang.Env(env))
	if err != nil {
		b.Fatalf("compile Expr workload: %v", err)
	}
	result, err := exprlang.Run(program, env)
	if err != nil {
		b.Fatalf("run Expr workload: %v", err)
	}
	return result
}

func validateSPLComparisonResult(b *testing.B, result object.Object, want any) {
	b.Helper()
	if object.IsError(result) {
		b.Fatalf("SPL workload failed: %s", result.Inspect())
	}
	switch value := result.(type) {
	case *object.Boolean:
		validateExprComparisonResult(b, value.Value, want)
	case *object.Integer:
		validateExprComparisonResult(b, value.Value, want)
	case *object.Float:
		validateExprComparisonResult(b, value.Value, want)
	default:
		b.Fatalf("unexpected SPL result %T (%v)", result, result)
	}
}

func validateExprComparisonResult(b *testing.B, result, want any) {
	b.Helper()
	switch expected := want.(type) {
	case bool:
		actual, ok := result.(bool)
		if !ok || actual != expected {
			b.Fatalf("unexpected boolean result: got %T(%v), want %v", result, result, expected)
		}
	case int64:
		var actual int64
		switch value := result.(type) {
		case int:
			actual = int64(value)
		case int64:
			actual = value
		default:
			b.Fatalf("unexpected integer result: got %T(%v), want %d", result, result, expected)
		}
		if actual != expected {
			b.Fatalf("unexpected integer result: got %d, want %d", actual, expected)
		}
	case float64:
		actual, ok := result.(float64)
		if !ok || math.Abs(actual-expected) > 1e-9 {
			b.Fatalf("unexpected float result: got %T(%v), want %v", result, result, expected)
		}
	default:
		b.Fatalf("unsupported expected result type %T", want)
	}
}
