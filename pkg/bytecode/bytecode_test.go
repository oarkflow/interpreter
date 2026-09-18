package bytecode

import (
	"testing"

	"github.com/oarkflow/interpreter/pkg/lexer"
	"github.com/oarkflow/interpreter/pkg/parser"
)

func compileSource(t *testing.T, src string) *BytecodeProgram {
	t.Helper()
	p := parser.NewParser(lexer.NewLexer(src))
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}
	compiled, err := CompileToBytecode(program)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	return compiled
}

// TestCallExpressionCompilesToOpCall is a regression test for the
// previously-unreachable OpCall path: *ast.CallExpression used to be
// rejected outright at compile time (ErrUnsupportedNode{"call expression"})
// even though the VM's OpCall execution was fully implemented.
func TestCallExpressionCompilesToOpCall(t *testing.T) {
	prog := compileSource(t, `foo(1, 2, 3);`)
	var found bool
	for _, ins := range prog.Instructions {
		if ins.Op == OpCall {
			found = true
			if ins.Arg != 3 {
				t.Fatalf("expected OpCall arg (argc) == 3, got %d", ins.Arg)
			}
			if ins.Call == nil {
				t.Fatalf("expected OpCall instruction to carry the originating *ast.CallExpression")
			}
		}
	}
	if !found {
		t.Fatalf("expected an OpCall instruction, got none: %+v", prog.Instructions)
	}
}

// TestIfExpressionJumpsAreWellFormed is a regression test for the jump
// opcodes added to support `if` expressions: every jump instruction's Arg
// must be a valid, forward-pointing instruction index within bounds (a
// dangling arg of -1 means patchJump was never called for it).
func TestIfExpressionJumpsAreWellFormed(t *testing.T) {
	prog := compileSource(t, `if (1 > 0) { "a"; } else { "b"; }`)
	sawJumpIfFalse := false
	sawJump := false
	for i, ins := range prog.Instructions {
		switch ins.Op {
		case OpJumpIfFalse:
			sawJumpIfFalse = true
			fallthrough
		case OpJump, OpJumpIfTrue:
			if ins.Op == OpJump {
				sawJump = true
			}
			if ins.Arg < 0 || ins.Arg > len(prog.Instructions) {
				t.Fatalf("instruction %d (%v) has out-of-range jump target %d (program has %d instructions)", i, ins.Op, ins.Arg, len(prog.Instructions))
			}
			if ins.Arg <= i {
				t.Fatalf("instruction %d (%v) jump target %d is not forward-pointing", i, ins.Op, ins.Arg)
			}
		}
	}
	if !sawJumpIfFalse || !sawJump {
		t.Fatalf("expected both OpJumpIfFalse and OpJump in compiled if-expression, got: %+v", prog.Instructions)
	}
}

// TestShortCircuitInfixCompiles is a regression test for `&&`/`||`, which
// used to be rejected outright (ErrUnsupportedNode{"short-circuit infix"})
// because the opcode set had no jump instructions at all to short-circuit
// with.
func TestShortCircuitInfixCompiles(t *testing.T) {
	for _, src := range []string{`true && false;`, `true || false;`} {
		prog := compileSource(t, src)
		var sawToBool bool
		for _, ins := range prog.Instructions {
			if ins.Op == OpToBool {
				sawToBool = true
			}
		}
		if !sawToBool {
			t.Fatalf("%q: expected OpToBool in compiled short-circuit infix, got: %+v", src, prog.Instructions)
		}
	}
}

// TestMultiLetAndLoopsStillFallBackToTreeWalk documents the deliberately
// out-of-scope gaps for this VM pass (see docs/features and the
// pkg/eval/eval.go bytecode fast-path comment): destructuring `let`, `for`
// loops, and function literals still compile-fail with ErrUnsupportedNode,
// which the caller must treat as "fall back to the tree walker", never as a
// hard error.
func TestMultiLetAndLoopsStillFallBackToTreeWalk(t *testing.T) {
	cases := []string{
		`let a, b = [1, 2];`,
		`for (let i = 0; i < 3; i += 1) { i; }`,
	}
	for _, src := range cases {
		p := parser.NewParser(lexer.NewLexer(src))
		program := p.ParseProgram()
		if len(p.Errors()) != 0 {
			t.Fatalf("%q: parse errors: %v", src, p.Errors())
		}
		_, err := CompileToBytecode(program)
		if err == nil {
			t.Fatalf("%q: expected compile to be rejected as unsupported (tree-walk fallback), but it succeeded", src)
		}
		if _, ok := err.(*ErrUnsupportedNode); !ok {
			t.Fatalf("%q: expected *ErrUnsupportedNode so the caller falls back to tree-walk, got %T: %v", src, err, err)
		}
	}
}
