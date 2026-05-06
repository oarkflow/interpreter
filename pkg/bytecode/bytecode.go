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

// ApplyFunctionFn calls a function object with arguments. Must be set before
// running the bytecode VM.
var ApplyFunctionFn func(fn object.Object, args []object.Object, env *object.Environment) object.Object

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
)

// ---------------------------------------------------------------------------
// Instruction / BytecodeProgram
// ---------------------------------------------------------------------------

type Instruction struct {
	Op  OpCode
	Arg int
	S   string
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
			return &ErrUnsupportedNode{Node: "short-circuit infix"}
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
		return &ErrUnsupportedNode{Node: "call expression"}
	default:
		return &ErrUnsupportedNode{Node: fmt.Sprintf("%T", exp)}
	}
}

// ---------------------------------------------------------------------------
// VM
// ---------------------------------------------------------------------------

type VM struct {
	program *BytecodeProgram
	env     *object.Environment
	stack   []object.Object
	ip      int
}

// RunOnVM executes a compiled BytecodeProgram in the given environment.
func RunOnVM(program *BytecodeProgram, env *object.Environment) object.Object {
	stackCap := len(program.Instructions)
	if stackCap < 8 {
		stackCap = 8
	}
	v := &VM{
		program: program,
		env:     env,
		stack:   make([]object.Object, 0, stackCap),
		ip:      0,
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
			if obj, ok := v.vmLookup(ins.S); ok {
				v.push(obj)
			} else {
				return object.NewError("identifier not found: %s", ins.S)
			}
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
			res := ApplyFunctionFn(fn, args, v.env)
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
