package eval_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/lexer"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/parser"
	"github.com/oarkflow/interpreter/pkg/token"
)

func benchmarkParse(input string, b *testing.B) *ast.Program {
	b.Helper()
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		b.Fatalf("parser errors: %v", p.Errors())
	}
	return program
}

func BenchmarkLexerNextToken(b *testing.B) {
	script := `
let x = 10;
let y = 20;
let add = function(a, b) { return a + b; };
if (x < y) { print add(x, y); }
let arr = [1,2,3,4,5];
for (let i = 0; i < len(arr); i = i + 1) { x = x + arr[i]; }
`

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		l := lexer.NewLexer(script)
		for tok := l.NextToken(); tok.Type != token.EOF; tok = l.NextToken() {
		}
	}
}

func BenchmarkParserParseProgram(b *testing.B) {
	script := `
let x = 10;
let y = 20;
let add = function(a, b) { return a + b; };
let arr = [1,2,3,4,5];
let u = {"name": "bench", "role": "test"};
for (let i = 0; i < len(arr); i = i + 1) {
  x = x + arr[i];
}
if (x > 0) { y = add(x, y); } else { y = y - 1; }
`

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		l := lexer.NewLexer(script)
		p := parser.NewParser(l)
		_ = p.ParseProgram()
		if len(p.Errors()) != 0 {
			b.Fatalf("parser errors: %v", p.Errors())
		}
	}
}

func BenchmarkEvalParseAndRun(b *testing.B) {
	script := `
let sum = 0;
for (let i = 0; i < 1000; i = i + 1) {
  sum = sum + i;
}
sum;
`

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		l := lexer.NewLexer(script)
		p := parser.NewParser(l)
		program := p.ParseProgram()
		if len(p.Errors()) != 0 {
			b.Fatalf("parser errors: %v", p.Errors())
		}
		env := object.NewEnvironment()
		_ = eval.Eval(program, env)
	}
}

func BenchmarkEvalRunOnlyPreparsed(b *testing.B) {
	script := `
let sum = 0;
for (let i = 0; i < 1000; i = i + 1) {
  sum = sum + i;
}
sum;
`
	program := benchmarkParse(script, b)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		env := object.NewEnvironment()
		_ = eval.Eval(program, env)
	}
}

func BenchmarkBuiltinsStringAndJSON(b *testing.B) {
	script := `
let s = "hello,world,from,spl";
let a = split(s, ",");
let j = json_encode({"a": 1, "b": true, "c": a});
let d = json_decode(j);
upper(join(d.c, "-"));
`
	program := benchmarkParse(script, b)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		env := object.NewEnvironment()
		_ = eval.Eval(program, env)
	}
}

func BenchmarkImportCached(b *testing.B) {
	tmpDir, err := os.MkdirTemp(".", "bench-import-")
	if err != nil {
		b.Fatalf("mktemp: %v", err)
	}
	b.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	modulePath := filepath.Join(tmpDir, "math.spl")
	if err := os.WriteFile(modulePath, []byte("export let x = 40; export let y = 2;"), 0o600); err != nil {
		b.Fatalf("write module: %v", err)
	}

	relPath, err := filepath.Rel(".", modulePath)
	if err != nil {
		b.Fatalf("rel path: %v", err)
	}

	script := fmt.Sprintf("import %q; x + y;", filepath.ToSlash(relPath))
	program := benchmarkParse(script, b)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		env := object.NewEnvironment()
		env.ModuleDir = "."
		_ = eval.Eval(program, env)
	}
}

func BenchmarkParseCompleteFeatureShowcase(b *testing.B) {
	content, err := os.ReadFile("../../testdata/complete_feature_showcase.spl")
	if err != nil {
		b.Fatalf("read showcase: %v", err)
	}
	script := string(content)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		l := lexer.NewLexer(script)
		p := parser.NewParser(l)
		_ = p.ParseProgram()
		if len(p.Errors()) != 0 {
			b.Fatalf("parser errors: %v", p.Errors())
		}
	}
}

func BenchmarkEvalCompleteFeatureShowcasePreparsed(b *testing.B) {
	content, err := os.ReadFile("../../testdata/complete_feature_showcase.spl")
	if err != nil {
		b.Fatalf("read showcase: %v", err)
	}
	program := benchmarkParse(string(content), b)

	stdout := os.Stdout
	nullFile, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Fatalf("open devnull: %v", err)
	}
	os.Stdout = nullFile
	b.Cleanup(func() {
		os.Stdout = stdout
		_ = nullFile.Close()
	})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		env := object.NewEnvironment()
		env.ModuleDir = "."
		_ = eval.Eval(program, env)
	}
}

func BenchmarkEvalTightArithmeticPreparsed(b *testing.B) {
	script := `
let sum = 0;
for (let i = 0; i < 1000; i = i + 1) {
  sum = sum + i;
}
sum;
`
	program := benchmarkParse(script, b)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		env := object.NewEnvironment()
		_ = eval.Eval(program, env)
	}
}
