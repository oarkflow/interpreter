module github.com/oarkflow/interpreter/benchmarks/exprcompare

go 1.25.7

require (
	github.com/expr-lang/expr v1.17.8
	github.com/oarkflow/interpreter v0.0.0
)

require github.com/oarkflow/convert v0.0.6 // indirect

replace github.com/oarkflow/interpreter => ../..

replace github.com/oarkflow/interpreter/plugins/tools => ../../builtins/tools
