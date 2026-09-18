package bytecode

import (
	"fmt"
	"io"
	"os"

	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/object"
)

// ---------------------------------------------------------------------------
// Function variables – must be set by the host (evaluator) package.
// ---------------------------------------------------------------------------

// EvalPrefixExpressionFn evaluates a prefix expression. Must be set before
// running the bytecode VM.
var EvalPrefixExpressionFn func(operator string, right object.Object) object.Object

// EvalInfixExpressionFn evaluates an infix expression. Must be set before
// running the bytecode VM.
var EvalInfixExpressionFn func(operator string, left, right object.Object) object.Object

// ApplyFunctionFn calls a function object with arguments. call is the
// originating *ast.CallExpression (nil if the caller has none), forwarded so
// call-stack frames can be built the same way the tree walker builds them.
// Must be set before running the bytecode VM.
var ApplyFunctionFn func(fn object.Object, args []object.Object, env *object.Environment, call *ast.CallExpression) object.Object

// FinalizeCallResultFn wraps a call's result the same way the tree walker's
// evalCallExpression does via finalizeCallResult: if the result is an error,
// it appends a call-stack frame for `call` (deduplicating a frame
// ApplyFunctionFn may already have added for a validation failure). Without
// this, a bytecode-compiled call loses one level of stack-trace detail
// compared to the same call evaluated by the tree walker. May be nil, in
// which case OpCall skips this step.
var FinalizeCallResultFn func(result object.Object, call *ast.CallExpression, env *object.Environment) object.Object

// BuiltinLookupFn looks up a builtin by name. Returns (builtin, ok).
var BuiltinLookupFn func(name string, env *object.Environment) (object.Object, bool)

// EvalProgramFn evaluates an AST program. Used by RunProgram.
var EvalProgramFn func(program any, env *object.Environment) object.Object

// ---------------------------------------------------------------------------
// OpCode
// ---------------------------------------------------------------------------

type OpCode byte

const (
	OpConstant OpCode = iota
	OpGetVar
	OpSetVar
	OpPop
	OpUnary
	OpBinary
	OpCall
	OpReturn
	OpNull
	OpPrint
	// OpJump unconditionally sets ip to Arg (an instruction index).
	OpJump
	// OpJumpIfFalse pops the top of stack and, if it is falsy (per
	// object.IsTruthy), sets ip to Arg; otherwise falls through. Used to
	// compile `if` expressions and short-circuiting `&&`/`||`.
	OpJumpIfFalse
	// OpJumpIfTrue pops the top of stack and, if it is truthy, sets ip to
	// Arg; otherwise falls through. Used for `||`'s short-circuit path.
	OpJumpIfTrue
	// OpToBool pops the top of stack and pushes object.TRUE/object.FALSE per
	// object.IsTruthy, matching the boolean-coercing (not value-returning)
	// semantics of SPL's `&&`/`||` (see pkg/eval/infix.go).
	OpToBool
)

// ---------------------------------------------------------------------------
// Instruction / BytecodeProgram
// ---------------------------------------------------------------------------

type Instruction struct {
	Op  OpCode
	Arg int
	S   string
	// Call carries the original *ast.CallExpression for OpCall instructions,
	// so ApplyFunctionFn can build the same call-stack frame the tree walker
	// would (see extendFunctionEnv/callFrameFromExpression in
	// pkg/eval/apply.go) - without it, errors from a bytecode-compiled call
	// silently lose one level of stack-trace detail. Unused by every other
	// opcode.
	Call *ast.CallExpression
}

type BytecodeProgram struct {
	Instructions []Instruction
	Constants    []object.Object
}

// ---------------------------------------------------------------------------
// Compiler
// ---------------------------------------------------------------------------

type Compiler struct {
	program *BytecodeProgram
}

type ErrUnsupportedNode struct {
	Node string
}

func (e *ErrUnsupportedNode) Error() string {
	return fmt.Sprintf("bytecode unsupported node: %s", e.Node)
}

// CompileToBytecode compiles an ast.Program into bytecode instructions.
func CompileToBytecode(program *ast.Program) (*BytecodeProgram, error) {
	c := &Compiler{program: &BytecodeProgram{
		Instructions: make([]Instruction, 0, 128),
		Constants:    make([]object.Object, 0, 64),
	}}
	for i, stmt := range program.Statements {
		allowPop := i != len(program.Statements)-1
		if err := c.compileStatement(stmt, allowPop); err != nil {
			return nil, err
		}
	}
	return c.program, nil
}

func (c *Compiler) emit(op OpCode, arg int, s string) {
	c.program.Instructions = append(c.program.Instructions, Instruction{Op: op, Arg: arg, S: s})
}

func (c *Compiler) addConstant(obj object.Object) int {
	idx := len(c.program.Constants)
	c.program.Constants = append(c.program.Constants, obj)
	return idx
}

// emitJump emits a jump instruction (op must be OpJump, OpJumpIfFalse, or
// OpJumpIfTrue) with a placeholder target, returning its index so the
// target can be filled in later via patchJump once the jump destination is
// known.
func (c *Compiler) emitJump(op OpCode) int {
	pos := len(c.program.Instructions)
	c.emit(op, -1, "")
	return pos
}

// patchJump sets the jump instruction at pos to target the next instruction
// that will be emitted (i.e. "jump to here").
func (c *Compiler) patchJump(pos int) {
	c.program.Instructions[pos].Arg = len(c.program.Instructions)
}

// compileBlockAsExpr compiles a block's statements so that exactly one
// value is left on the stack: the value of its last statement (an empty
// block evaluates to null), mirroring the tree-walker's evalBlockStatement.
// Used to compile `if` expression branches.
func (c *Compiler) compileBlockAsExpr(block *ast.BlockStatement) error {
	if block == nil || len(block.Statements) == 0 {
		c.emit(OpNull, 0, "")
		return nil
	}
	for i, stmt := range block.Statements {
		allowPop := i != len(block.Statements)-1
		if err := c.compileStatement(stmt, allowPop); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) compileStatement(stmt ast.Statement, allowPop bool) error {
	switch node := stmt.(type) {
	case *ast.ExpressionStatement:
		if err := c.compileExpression(node.Expression); err != nil {
			return err
		}
		if allowPop {
			c.emit(OpPop, 0, "")
		}
		return nil
	case *ast.LetStatement:
		if len(node.Names) > 1 || node.Name == nil {
			return &ErrUnsupportedNode{Node: "multi-let"}
		}
		if err := c.compileExpression(node.Value); err != nil {
			return err
		}
		c.emit(OpSetVar, 0, node.Name.Name)
		if allowPop {
			c.emit(OpPop, 0, "")
		}
		return nil
	case *ast.ReturnStatement:
		if node.ReturnValue == nil {
			c.emit(OpNull, 0, "")
		} else if err := c.compileExpression(node.ReturnValue); err != nil {
			return err
		}
		c.emit(OpReturn, 0, "")
		return nil
	case *ast.PrintStatement:
		if err := c.compileExpression(node.Expression); err != nil {
			return err
		}
		c.emit(OpPrint, 0, "")
		if allowPop {
			c.emit(OpPop, 0, "")
		}
		return nil
	default:
		return &ErrUnsupportedNode{Node: fmt.Sprintf("%T", stmt)}
	}
}

func (c *Compiler) compileExpression(exp ast.Expression) error {
	switch node := exp.(type) {
	case *ast.IntegerLiteral:
		idx := c.addConstant(object.IntegerObj(node.Value))
		c.emit(OpConstant, idx, "")
		return nil
	case *ast.FloatLiteral:
		idx := c.addConstant(&object.Float{Value: node.Value})
		c.emit(OpConstant, idx, "")
		return nil
	case *ast.StringLiteral:
		idx := c.addConstant(&object.String{Value: node.Value})
		c.emit(OpConstant, idx, "")
		return nil
	case *ast.BooleanLiteral:
		if node.Value {
			idx := c.addConstant(object.TRUE)
			c.emit(OpConstant, idx, "")
		} else {
			idx := c.addConstant(object.FALSE)
			c.emit(OpConstant, idx, "")
		}
		return nil
	case *ast.NullLiteral:
		c.emit(OpNull, 0, "")
		return nil
	case *ast.Identifier:
		c.emit(OpGetVar, 0, node.Name)
		return nil
	case *ast.PrefixExpression:
		if err := c.compileExpression(node.Right); err != nil {
			return err
		}
		c.emit(OpUnary, 0, node.Operator)
		return nil
	case *ast.InfixExpression:
		if node.Operator == "&&" || node.Operator == "||" {
			return c.compileShortCircuitInfix(node)
		}
		if err := c.compileExpression(node.Left); err != nil {
			return err
		}
		if err := c.compileExpression(node.Right); err != nil {
			return err
		}
		c.emit(OpBinary, 0, node.Operator)
		return nil
	case *ast.CallExpression:
		if err := c.compileExpression(node.Function); err != nil {
			return err
		}
		for _, arg := range node.Arguments {
			if err := c.compileExpression(arg); err != nil {
				return err
			}
		}
		c.program.Instructions = append(c.program.Instructions, Instruction{Op: OpCall, Arg: len(node.Arguments), Call: node})
		return nil
	case *ast.IfExpression:
		if err := c.compileExpression(node.Condition); err != nil {
			return err
		}
		jumpToElse := c.emitJump(OpJumpIfFalse)
		if err := c.compileBlockAsExpr(node.Consequence); err != nil {
			return err
		}
		jumpToEnd := c.emitJump(OpJump)
		c.patchJump(jumpToElse)
		if node.Alternative != nil {
			if err := c.compileBlockAsExpr(node.Alternative); err != nil {
				return err
			}
		} else {
			c.emit(OpNull, 0, "")
		}
		c.patchJump(jumpToEnd)
		return nil
	default:
		return &ErrUnsupportedNode{Node: fmt.Sprintf("%T", exp)}
	}
}

// compileShortCircuitInfix compiles `&&`/`||`, matching the boolean-coercing
// short-circuit semantics of evalInfixExpression in pkg/eval/infix.go: `a &&
// b` is FALSE without evaluating b if a is falsy, else IsTruthy(b); `a || b`
// is TRUE without evaluating b if a is truthy, else IsTruthy(b). Neither
// operator ever returns a raw operand value (unlike e.g. `??`).
func (c *Compiler) compileShortCircuitInfix(node *ast.InfixExpression) error {
	if err := c.compileExpression(node.Left); err != nil {
		return err
	}
	var shortCircuitJump int
	if node.Operator == "&&" {
		shortCircuitJump = c.emitJump(OpJumpIfFalse)
	} else {
		shortCircuitJump = c.emitJump(OpJumpIfTrue)
	}
	if err := c.compileExpression(node.Right); err != nil {
		return err
	}
	c.emit(OpToBool, 0, "")
	jumpToEnd := c.emitJump(OpJump)
	c.patchJump(shortCircuitJump)
	if node.Operator == "&&" {
		idx := c.addConstant(object.FALSE)
		c.emit(OpConstant, idx, "")
	} else {
		idx := c.addConstant(object.TRUE)
		c.emit(OpConstant, idx, "")
	}
	c.patchJump(jumpToEnd)
	return nil
}

// ---------------------------------------------------------------------------
// VM
// ---------------------------------------------------------------------------

type VM struct {
	program *BytecodeProgram
	env     *object.Environment
	stack   []object.Object
	inline  [32]object.Object
	ip      int
}

// RunOnVM executes a compiled BytecodeProgram in the given environment.
func RunOnVM(program *BytecodeProgram, env *object.Environment) object.Object {
	stackCap := len(program.Instructions)
	if stackCap < 8 {
		stackCap = 8
	}
	v := VM{
		program: program,
		env:     env,
		ip:      0,
	}
	if stackCap <= len(v.inline) {
		v.stack = v.inline[:0]
	} else {
		v.stack = make([]object.Object, 0, stackCap)
	}
	return v.Run()
}

func (v *VM) push(obj object.Object) {
	v.stack = append(v.stack, obj)
}

func (v *VM) pop() (object.Object, bool) {
	if len(v.stack) == 0 {
		return nil, false
	}
	last := v.stack[len(v.stack)-1]
	v.stack = v.stack[:len(v.stack)-1]
	return last, true
}

// Run executes the bytecode program and returns the result.
func (v *VM) Run() object.Object {
	for v.ip < len(v.program.Instructions) {
		ins := v.program.Instructions[v.ip]
		switch ins.Op {
		case OpConstant:
			if ins.Arg < 0 || ins.Arg >= len(v.program.Constants) {
				return object.NewError("vm constant index out of bounds: %d", ins.Arg)
			}
			v.push(v.program.Constants[ins.Arg])
		case OpGetVar:
			obj, ok := v.vmLookup(ins.S)
			if !ok {
				return object.NewError("identifier not found: %s", ins.S)
			}
			// Mirror evalIdentifier's unwrap behavior exactly (pkg/eval/eval.go)
			// - env.Get alone does not force lazy values or unwrap/validate
			// OwnedValue, so skipping this here silently changed behavior for
			// `let x = move(...)`/`lazy` values once ExpressionStatement
			// started reaching the VM.
			if lazy, ok := obj.(*object.LazyValue); ok {
				obj = lazy.Force()
			} else if owned, ok := obj.(*object.OwnedValue); ok {
				if v.env != nil && owned.OwnerID != "" && !v.env.HasOwner(owned.OwnerID) {
					return object.NewError("ownership violation: value moved to another scope")
				}
				obj = owned.Inner
			}
			v.push(obj)
		case OpSetVar:
			val, ok := v.pop()
			if !ok {
				return object.NewError("vm stack underflow on set")
			}
			v.env.Set(ins.S, val)
			v.push(object.NULL)
		case OpPop:
			if _, ok := v.pop(); !ok {
				return object.NewError("vm stack underflow on pop")
			}
		case OpUnary:
			right, ok := v.pop()
			if !ok {
				return object.NewError("vm stack underflow on unary op")
			}
			if EvalPrefixExpressionFn == nil {
				return object.NewError("vm: EvalPrefixExpressionFn not set")
			}
			res := EvalPrefixExpressionFn(ins.S, right)
			if isError(res) {
				return res
			}
			v.push(res)
		case OpBinary:
			right, okR := v.pop()
			left, okL := v.pop()
			if !okR || !okL {
				return object.NewError("vm stack underflow on binary op")
			}
			if EvalInfixExpressionFn == nil {
				return object.NewError("vm: EvalInfixExpressionFn not set")
			}
			res := EvalInfixExpressionFn(ins.S, left, right)
			if isError(res) {
				return res
			}
			v.push(res)
		case OpCall:
			argc := ins.Arg
			if argc < 0 || len(v.stack) < argc+1 {
				return object.NewError("vm invalid call arity: %d", argc)
			}
			args := make([]object.Object, argc)
			for i := argc - 1; i >= 0; i-- {
				arg, _ := v.pop()
				args[i] = arg
			}
			fn, _ := v.pop()
			if ApplyFunctionFn == nil {
				return object.NewError("vm: ApplyFunctionFn not set")
			}
			res := ApplyFunctionFn(fn, args, v.env, ins.Call)
			if ins.Call != nil && FinalizeCallResultFn != nil {
				res = FinalizeCallResultFn(res, ins.Call, v.env)
			}
			if isError(res) {
				return res
			}
			v.push(res)
		case OpReturn:
			ret, ok := v.pop()
			if !ok {
				return object.NULL
			}
			return ret
		case OpNull:
			v.push(object.NULL)
		case OpJump:
			v.ip = ins.Arg - 1
		case OpJumpIfFalse:
			val, ok := v.pop()
			if !ok {
				return object.NewError("vm stack underflow on conditional jump")
			}
			if !object.IsTruthy(val) {
				v.ip = ins.Arg - 1
			}
		case OpJumpIfTrue:
			val, ok := v.pop()
			if !ok {
				return object.NewError("vm stack underflow on conditional jump")
			}
			if object.IsTruthy(val) {
				v.ip = ins.Arg - 1
			}
		case OpToBool:
			val, ok := v.pop()
			if !ok {
				return object.NewError("vm stack underflow on bool conversion")
			}
			v.push(object.NativeBoolToBooleanObject(object.IsTruthy(val)))
		case OpPrint:
			val, ok := v.pop()
			if !ok {
				return object.NewError("vm stack underflow on print")
			}
			var out = io.Writer(os.Stdout)
			if v.env != nil && v.env.Output != nil {
				out = v.env.Output
			}
			if v.env != nil && v.env.RuntimeLimits != nil && v.env.RuntimeLimits.MaxOutputBytes > 0 {
				text := val.Inspect() + "\n"
				rl := v.env.RuntimeLimits
				remaining := rl.MaxOutputBytes - rl.OutputBytes
				if remaining <= 0 {
					return object.NewError("output limit exceeded (%d bytes)", rl.MaxOutputBytes)
				}
				if int64(len(text)) > remaining {
					fmt.Fprint(out, text[:remaining])
					rl.OutputBytes += remaining
					return object.NewError("output limit exceeded (%d bytes)", rl.MaxOutputBytes)
				}
				fmt.Fprint(out, text)
				rl.OutputBytes += int64(len(text))
				v.push(object.NULL)
				break
			}
			fmt.Fprintln(out, val.Inspect())
			v.push(object.NULL)
		default:
			return object.NewError("unknown vm opcode: %d", ins.Op)
		}
		v.ip++
	}
	if len(v.stack) == 0 {
		return object.NULL
	}
	return v.stack[len(v.stack)-1]
}

func (v *VM) vmLookup(name string) (object.Object, bool) {
	if v.env != nil {
		if obj, ok := v.env.Get(name); ok {
			return obj, true
		}
	}
	if BuiltinLookupFn != nil {
		return BuiltinLookupFn(name, v.env)
	}
	return nil, false
}

func isError(obj object.Object) bool {
	return obj != nil && obj.Type() == object.ERROR_OBJ
}

// RunProgram evaluates a program via the configured EvalProgramFn.
// This is the bytecode package's equivalent of the root-package runProgram.
func RunProgram(program any, env *object.Environment) object.Object {
	if EvalProgramFn == nil {
		return object.NewError("bytecode: EvalProgramFn not set")
	}
	return EvalProgramFn(program, env)
}
