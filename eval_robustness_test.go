package interpreter

import "testing"

type bogusNode struct{}

func (bogusNode) String() string { return "bogus" }

func TestEvalShortCircuitAndOr(t *testing.T) {
	result := testEval(`let x = 0; false && (x = 1); x;`)
	testIntegerObject(t, result, 0)

	result = testEval(`let x = 0; true || (x = 1); x;`)
	testIntegerObject(t, result, 0)
}

func TestEvalUnsupportedNodeReturnsError(t *testing.T) {
	obj := Eval(bogusNode{}, NewEnvironment())
	if !isError(obj) {
		t.Fatalf("expected error object, got %T", obj)
	}
}

func TestModuloByZeroReturnsError(t *testing.T) {
	obj := testEval(`10 % 0;`)
	if !isError(obj) {
		t.Fatalf("expected error object, got %T", obj)
	}
}
