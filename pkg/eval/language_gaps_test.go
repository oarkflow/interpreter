package eval_test

import (
	"strings"
	"testing"

	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/lexer"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/parser"
)

func TestConstBindingsRejectReassignment(t *testing.T) {
	result := evalWithParserCheck(t, `const answer = 42; answer = 7;`, object.NewEnvironment())
	err, ok := result.(*object.Error)
	if !ok || !strings.Contains(err.Message, "cannot assign to constant answer") {
		t.Fatalf("expected constant assignment error, got %T (%v)", result, result)
	}
}

func TestMacrosExpandBlocksAndBareDSLCalls(t *testing.T) {
	result := evalWithParserCheck(t, `
macro repeat(n, body) {
    let i = 0;
    while (i < n) { body; i += 1; }
}
macro def(name, value) { let name = value; }
let i = 100;
let total = 0;
repeat(3) { total += 2; }
def ANSWER 42
[total, i, ANSWER];
`, object.NewEnvironment())
	array, ok := result.(*object.Array)
	if !ok || len(array.Elements) != 3 {
		t.Fatalf("expected macro result array, got %T (%v)", result, result)
	}
	testIntegerObject(t, array.Elements[0], 6)
	testIntegerObject(t, array.Elements[1], 100) // macro-local i is hygienic
	testIntegerObject(t, array.Elements[2], 42)
}

func TestStructuredTypeAnnotations(t *testing.T) {
	p := parser.NewParser(lexer.NewLexer(`function map(values: Array<int | null>): Map<string, float[]> { return {}; }`))
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	declaration := program.Statements[0].(*ast.LetStatement)
	function := declaration.Value.(*ast.FunctionLiteral)
	if got := function.ParamTypeRefs[0].String(); got != "Array<int | null>" {
		t.Fatalf("unexpected parameter type: %q", got)
	}
	if got := function.ReturnTypeRef.String(); got != "Map<string, float[]>" {
		t.Fatalf("unexpected return type: %q", got)
	}
}

func TestIfExpressionStringWithoutElseIsSafe(t *testing.T) {
	p := parser.NewParser(lexer.NewLexer(`if (true) { 1; }`))
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if got := program.String(); !strings.Contains(got, "if true") {
		t.Fatalf("unexpected AST rendering: %q", got)
	}
}

func TestKeywordCanBeUsedAsContextualMemberName(t *testing.T) {
	p := parser.NewParser(lexer.NewLexer(`builder.select("name").default();`))
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if got := program.String(); !strings.Contains(got, ".select") || !strings.Contains(got, ".default") {
		t.Fatalf("unexpected contextual member AST: %q", got)
	}
}

func TestGenericTypeAnnotationsValidateElements(t *testing.T) {
	result := evalWithParserCheck(t, `let values: Array<int> = [1, "bad"];`, object.NewEnvironment())
	err, ok := result.(*object.Error)
	if !ok || !strings.Contains(err.Message, "type mismatch") {
		t.Fatalf("expected generic element type mismatch, got %T (%v)", result, result)
	}
}

func TestFunctionAndMethodTypesAreEnforced(t *testing.T) {
	parameterResult := evalWithParserCheck(t, `function total(values: Array<int>): int { return 1; } total([1, "bad"]);`, object.NewEnvironment())
	if err, ok := parameterResult.(*object.Error); !ok || !strings.Contains(err.Message, "parameter values") {
		t.Fatalf("expected parameter type mismatch, got %T (%v)", parameterResult, parameterResult)
	}

	returnResult := evalWithParserCheck(t, `function bad(): int { return "bad"; } bad();`, object.NewEnvironment())
	if err, ok := returnResult.(*object.Error); !ok || !strings.Contains(err.Message, "return type mismatch") {
		t.Fatalf("expected return type mismatch, got %T (%v)", returnResult, returnResult)
	}

	methodResult := evalWithParserCheck(t, `
class Counter { value(input: int = 3): int { return input; } }
let counter = new Counter();
counter.value("bad");
`, object.NewEnvironment())
	if err, ok := methodResult.(*object.Error); !ok || !strings.Contains(err.Message, "parameter input") {
		t.Fatalf("expected method parameter mismatch, got %T (%v)", methodResult, methodResult)
	}
}

func TestNumericFastPathPreservesIntegerPrecisionAndErrors(t *testing.T) {
	precision := evalWithParserCheck(t, `9007199254740993 == 9007199254740992;`, object.NewEnvironment())
	testBooleanObject(t, precision, false)

	mixed := evalWithParserCheck(t, `((25.0 * 24.0) - 50.0) * (1 + 0.2);`, object.NewEnvironment())
	value, ok := mixed.(*object.Float)
	if !ok || value.Value != 660 {
		t.Fatalf("expected fused arithmetic result 660, got %T (%v)", mixed, mixed)
	}

	division := evalWithParserCheck(t, `10 / (3 - 3);`, object.NewEnvironment())
	if err, ok := division.(*object.Error); !ok || !strings.Contains(err.Message, "division by zero") {
		t.Fatalf("expected division-by-zero error, got %T (%v)", division, division)
	}
}

func TestLiteralMembershipFastPathPreservesSemantics(t *testing.T) {
	result := evalWithParserCheck(t, `[
    "finance" in ["finance", "procurement"],
    2.0 in [1, 2, 3],
    null not in [true, false]
];`, object.NewEnvironment())
	array, ok := result.(*object.Array)
	if !ok || len(array.Elements) != 3 {
		t.Fatalf("expected membership result array, got %T (%v)", result, result)
	}
	for _, element := range array.Elements {
		testBooleanObject(t, element, true)
	}
}

func TestConstDestructureBindingsRejectReassignment(t *testing.T) {
	result := evalWithParserCheck(t, `const [first] = [1]; first += 1;`, object.NewEnvironment())
	err, ok := result.(*object.Error)
	if !ok || !strings.Contains(err.Message, "cannot assign to constant first") {
		t.Fatalf("expected destructured constant assignment error, got %T (%v)", result, result)
	}
}

func TestChannelSendReceiveSyntax(t *testing.T) {
	result := evalWithParserCheck(t, `
let ch = channel(1);
ch <- 42;
let value = ch <-;
value;
`, object.NewEnvironment())
	testIntegerObject(t, result, 42)
}

func TestSelectReceivesAndBindsValue(t *testing.T) {
	result := evalWithParserCheck(t, `
let ch = channel(1);
ch <- 9;
select {
    case ch <- value: { value + 1; }
    default: { 0; }
}
`, object.NewEnvironment())
	testIntegerObject(t, result, 10)
}

func TestSpawnCallKeepsArgumentsUnevaluatedUntilScheduled(t *testing.T) {
	result := evalWithParserCheck(t, `
let square = function(value) { return value * value; };
let future = spawn square(7);
await future;
`, object.NewEnvironment())
	testIntegerObject(t, result, 49)
}

func TestConvertBackedTypeParsing(t *testing.T) {
	result := evalWithParserCheck(t, `[
    parse_type("42", "int"),
    parse_type("3.5", "float"),
    parse_type("yes", "bool"),
    parse_type(99, "string")
];`, object.NewEnvironment())
	arr, ok := result.(*object.Array)
	if !ok || len(arr.Elements) != 4 {
		t.Fatalf("expected four conversion results, got %T (%v)", result, result)
	}
	testIntegerObject(t, arr.Elements[0], 42)
	if got := arr.Elements[1].(*object.Float).Value; got != 3.5 {
		t.Fatalf("unexpected float conversion: %v", got)
	}
	testBooleanObject(t, arr.Elements[2], true)
	if got := arr.Elements[3].(*object.String).Value; got != "99" {
		t.Fatalf("unexpected string conversion: %q", got)
	}
}

// TestImmutableValuesDoNotPanicAndAreGuarded is a regression test for the
// duplicate builtins.ImmutableValue / object.ImmutableValue type bug: reads
// through immutable()-wrapped arrays/hashes used to panic with an internal
// type-assertion mismatch (since the evaluator's unwrap sites only recognized
// object.ImmutableValue, but immutable() built the unexported builtins one),
// and writes fell through to a misleading "cannot set property on HASH"
// error instead of the intended "cannot mutate immutable value" guard.
func TestImmutableValuesDoNotPanicAndAreGuarded(t *testing.T) {
	arrResult := evalWithParserCheck(t, `let frozen = immutable([1, 2, 3]); frozen[1];`, object.NewEnvironment())
	testIntegerObject(t, arrResult, 2)

	hashReadResult := evalWithParserCheck(t, `let frozen = immutable({"a": 1}); frozen.a;`, object.NewEnvironment())
	testIntegerObject(t, hashReadResult, 1)

	hashIndexResult := evalWithParserCheck(t, `let frozen = immutable({"a": 1}); frozen["a"];`, object.NewEnvironment())
	testIntegerObject(t, hashIndexResult, 1)

	writeResult := evalWithParserCheck(t, `let frozen = immutable({"a": 1}); frozen.a = 2;`, object.NewEnvironment())
	err, ok := writeResult.(*object.Error)
	if !ok || !strings.Contains(err.Message, "cannot mutate immutable value") {
		t.Fatalf("expected immutable mutation guard error, got %T (%v)", writeResult, writeResult)
	}

	outOfRange := evalWithParserCheck(t, `let frozen = immutable([1, 2, 3]); frozen[99];`, object.NewEnvironment())
	testNullObject(t, outOfRange)
}

// TestAsyncFunctionDeclarationBindsName is a regression test: `async
// function name(){}` used to parse as a bare ExpressionStatement (no
// case for token.ASYNC in the statement-level switch), so the FunctionLiteral
// it produced was evaluated and discarded without ever binding `name` into
// the enclosing scope.
func TestAsyncFunctionDeclarationBindsName(t *testing.T) {
	result := evalWithParserCheck(t, `
async function double(n) { return n * 2; }
await double(21);
`, object.NewEnvironment())
	testIntegerObject(t, result, 42)
}

// TestGoAsyncReturnsAwaitableFuture is a regression test: go_async used to
// discard its function's result and always return NULL, even though
// docs/features/20-core-builtins.md already documents `let f1 = go_async(...)`
// being awaited like a Future.
func TestGoAsyncReturnsAwaitableFuture(t *testing.T) {
	result := evalWithParserCheck(t, `
let f = go_async(function() { return 41 + 1; });
await f;
`, object.NewEnvironment())
	testIntegerObject(t, result, 42)
}

// TestInterfaceEnforcementChecksSignatures is a regression test extending
// the existing method-existence check (present since the initial commit) to
// also validate parameter/return types declared on the interface.
func TestInterfaceEnforcementChecksSignatures(t *testing.T) {
	// Matching signature: implements successfully.
	ok := evalWithParserCheck(t, `
interface Greeter {
    greet(name: string): string;
}
class English implements Greeter {
    greet(name: string): string { return "hello " + name; }
}
new English().greet("world");
`, object.NewEnvironment())
	str, isStr := ok.(*object.String)
	if !isStr || str.Value != "hello world" {
		t.Fatalf("expected successful interface implementation, got %T (%v)", ok, ok)
	}

	// Wrong parameter type: rejected at class-declaration time.
	badParam := evalWithParserCheck(t, `
interface Greeter {
    greet(name: string): string;
}
class Bad implements Greeter {
    greet(name: int): string { return "hi"; }
}
`, object.NewEnvironment())
	err, isErr := badParam.(*object.Error)
	if !isErr || !strings.Contains(err.Message, "parameter") {
		t.Fatalf("expected parameter type mismatch error, got %T (%v)", badParam, badParam)
	}

	// Wrong return type: rejected at class-declaration time.
	badReturn := evalWithParserCheck(t, `
interface Greeter {
    greet(name: string): string;
}
class Bad implements Greeter {
    greet(name: string): int { return 1; }
}
`, object.NewEnvironment())
	err2, isErr2 := badReturn.(*object.Error)
	if !isErr2 || !strings.Contains(err2.Message, "return type") {
		t.Fatalf("expected return type mismatch error, got %T (%v)", badReturn, badReturn)
	}

	// Wrong parameter count: rejected at class-declaration time.
	badArity := evalWithParserCheck(t, `
interface Greeter {
    greet(name: string): string;
}
class Bad implements Greeter {
    greet(name: string, extra: string): string { return "hi"; }
}
`, object.NewEnvironment())
	err3, isErr3 := badArity.(*object.Error)
	if !isErr3 || !strings.Contains(err3.Message, "parameter(s)") {
		t.Fatalf("expected parameter count mismatch error, got %T (%v)", badArity, badArity)
	}

	// Untyped implementation of a typed interface method stays permissive
	// (most SPL code has no annotations; requiring them would break
	// previously-valid classes).
	untyped := evalWithParserCheck(t, `
interface Greeter {
    greet(name: string): string;
}
class Loose implements Greeter {
    greet(name) { return "hi " + name; }
}
new Loose().greet("there");
`, object.NewEnvironment())
	str2, isStr2 := untyped.(*object.String)
	if !isStr2 || str2.Value != "hi there" {
		t.Fatalf("expected untyped implementation to still be accepted, got %T (%v)", untyped, untyped)
	}
}
