package interpreter_test

import (
	"testing"

	. "github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/eval"
)

// TestBytecodeVMMatchesTreeWalkerForNewlySupportedNodes is a differential
// test for the bytecode VM's newly-added coverage (call expressions, `if`
// expressions, short-circuit `&&`/`||` - see pkg/bytecode/bytecode.go). It
// runs each snippet twice: once through the normal path (which now lets
// ExpressionStatement reach the bytecode VM - see runProgramStatement in
// pkg/eval/eval.go) and once with the bytecode hooks forced off, so the
// tree walker alone produces the result. The two must agree, since the VM
// is only ever meant to be an equivalent, faster path - never a source of
// behavioral drift.
func TestBytecodeVMMatchesTreeWalkerForNewlySupportedNodes(t *testing.T) {
	cases := []struct {
		name   string
		script string
	}{
		{"call expression", `function double(n) { return n * 2; } double(21);`},
		{"nested call expressions", `function inc(n) { return n + 1; } function twice(n) { return inc(inc(n)); } twice(5);`},
		{"if expression true branch", `let x = 10; if (x > 5) { "big"; } else { "small"; }`},
		{"if expression false branch", `let x = 1; if (x > 5) { "big"; } else { "small"; }`},
		{"if expression no else, condition false", `if (false) { 1; }`},
		{"&& short-circuit false", `false && (1/0 == 0);`},
		{"&& both truthy", `true && 5;`},
		{"&& left truthy right falsy", `1 && 0;`},
		{"|| short-circuit true", `true || (1/0 == 0);`},
		{"|| both falsy", `false || null;`},
		{"|| left falsy right truthy", `0 || 5;`},
		{"if as call argument", `function id(x) { return x; } id(if (true) { 1; } else { 2; });`},
		{"call inside condition", `function isPos(n) { return n > 0; } if (isPos(3)) { "yes"; } else { "no"; }`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withBytecode, errWith := ExecWithOptions(tc.script, nil, ExecOptions{})

			savedCompile, savedRun, savedUnsupported := eval.BytecodeCompileFn, eval.BytecodeRunFn, eval.BytecodeIsUnsupportedErr
			eval.BytecodeCompileFn = nil
			eval.BytecodeRunFn = nil
			eval.BytecodeIsUnsupportedErr = nil
			withoutBytecode, errWithout := ExecWithOptions(tc.script, nil, ExecOptions{})
			eval.BytecodeCompileFn, eval.BytecodeRunFn, eval.BytecodeIsUnsupportedErr = savedCompile, savedRun, savedUnsupported

			if (errWith == nil) != (errWithout == nil) {
				t.Fatalf("error presence mismatch: with-bytecode err=%v, tree-walk-only err=%v", errWith, errWithout)
			}
			if errWith != nil {
				if errWith.Error() != errWithout.Error() {
					t.Fatalf("error message mismatch: with-bytecode=%q, tree-walk-only=%q", errWith.Error(), errWithout.Error())
				}
				return
			}
			if withBytecode.Inspect() != withoutBytecode.Inspect() {
				t.Fatalf("result mismatch: with-bytecode=%q, tree-walk-only=%q", withBytecode.Inspect(), withoutBytecode.Inspect())
			}
		})
	}
}

// TestBytecodeVMPreservesCallStackFrames is a regression test: enabling the
// bytecode VM for ExpressionStatement (so top-level call expressions can be
// compiled) initially dropped one level of call-stack detail from runtime
// errors, since bytecode.ApplyFunctionFn was wired with call=nil and
// bytecode's OpCall never replicated the tree walker's
// finalizeCallResult/WithFrame stack-frame accumulation. Both are now
// threaded through (see the Instruction.Call field and
// bytecode.FinalizeCallResultFn in pkg/bytecode/bytecode.go).
func TestBytecodeVMPreservesCallStackFrames(t *testing.T) {
	_, err := ExecWithOptions(`
let boom = function() {
  unknown_identifier;
};
let wrapper = function() {
  boom();
};
wrapper();
`, nil, ExecOptions{})
	if err == nil {
		t.Fatalf("expected runtime error")
	}
	var execErr *ExecError
	if e, ok := err.(*ExecError); ok {
		execErr = e
	} else {
		t.Fatalf("expected *ExecError, got %T: %v", err, err)
	}
	if len(execErr.Stack) < 2 {
		t.Fatalf("expected at least two stack frames (regression: bytecode VM dropped the top-level call's frame), got %d: %#v", len(execErr.Stack), execErr.Stack)
	}
}

// TestBytecodeVMUnwrapsOwnedAndLazyIdentifiers is a regression test: the
// bytecode VM's OpGetVar originally called env.Get directly without
// replicating evalIdentifier's LazyValue-forcing and OwnedValue-unwrapping
// (pkg/eval/eval.go), so a top-level expression statement referencing a
// move()-wrapped variable saw the raw OwnedValue wrapper instead of its
// inner value once ExpressionStatement started reaching the VM.
func TestBytecodeVMUnwrapsOwnedAndLazyIdentifiers(t *testing.T) {
	result, err := ExecWithOptions(`
let data = move([1, 2, 3]);
len(data);
`, nil, ExecOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	n, ok := result.(*Integer)
	if !ok || n.Value != 3 {
		t.Fatalf("expected len(data) == 3 on an owned value, got %T (%v)", result, result)
	}
}
