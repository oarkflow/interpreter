package interpreter

import (
	"fmt"
	"io"
	"os"
)

type OpCode byte

const (
	opConstant OpCode = iota
	opGetVar
	opSetVar
	opPop
	opUnary
	opBinary
	opCall
	opReturn
	opNull
	opPrint
)

type instruction struct {
	op  OpCode
	arg int
	s   string
}

type bytecodeProgram struct {
	instructions []instruction
	constants    []Object
}

type bytecodeCompiler struct {
	program *bytecodeProgram
}

type errUnsupportedNode struct {
	node string
}

func (e *errUnsupportedNode) Error() string {
	return fmt.Sprintf("bytecode unsupported node: %s", e.node)
}

func compileToBytecode(program *Program) (*bytecodeProgram, error) {
	c := &bytecodeCompiler{program: &bytecodeProgram{instructions: make([]instruction, 0, 128), constants: make([]Object, 0, 64)}}
	for i, stmt := range program.Statements {
		allowPop := i != len(program.Statements)-1
		if err := c.compileStatement(stmt, allowPop); err != nil {
			return nil, err
		}
	}
	return c.program, nil
}

func (c *bytecodeCompiler) emit(op OpCode, arg int, s string) {
	c.program.instructions = append(c.program.instructions, instruction{op: op, arg: arg, s: s})
}

func (c *bytecodeCompiler) addConstant(obj Object) int {
	idx := len(c.program.constants)
	c.program.constants = append(c.program.constants, obj)
	return idx
}

func (c *bytecodeCompiler) compileStatement(stmt Statement, allowPop bool) error {
	switch node := stmt.(type) {
	case *ExpressionStatement:
		if err := c.compileExpression(node.Expression); err != nil {
			return err
		}
		if allowPop {
			c.emit(opPop, 0, "")
		}
		return nil
	case *LetStatement:
		if len(node.Names) != 1 || node.Name == nil {
			return &errUnsupportedNode{node: "multi-let"}
		}
		if err := c.compileExpression(node.Value); err != nil {
			return err
		}
		c.emit(opSetVar, 0, node.Name.Name)
		if allowPop {
			c.emit(opPop, 0, "")
		}
		return nil
	case *ReturnStatement:
		if node.ReturnValue == nil {
			c.emit(opNull, 0, "")
		} else if err := c.compileExpression(node.ReturnValue); err != nil {
			return err
		}
		c.emit(opReturn, 0, "")
		return nil
	case *PrintStatement:
		if err := c.compileExpression(node.Expression); err != nil {
			return err
		}
		c.emit(opPrint, 0, "")
		if allowPop {
			c.emit(opPop, 0, "")
		}
		return nil
	default:
		return &errUnsupportedNode{node: fmt.Sprintf("%T", stmt)}
	}
}

func (c *bytecodeCompiler) compileExpression(exp Expression) error {
	switch node := exp.(type) {
	case *IntegerLiteral:
		idx := c.addConstant(integerObj(node.Value))
		c.emit(opConstant, idx, "")
		return nil
	case *FloatLiteral:
		idx := c.addConstant(&Float{Value: node.Value})
		c.emit(opConstant, idx, "")
		return nil
	case *StringLiteral:
		idx := c.addConstant(&String{Value: node.Value})
		c.emit(opConstant, idx, "")
		return nil
	case *BooleanLiteral:
		if node.Value {
			idx := c.addConstant(TRUE)
			c.emit(opConstant, idx, "")
		} else {
			idx := c.addConstant(FALSE)
			c.emit(opConstant, idx, "")
		}
		return nil
	case *NullLiteral:
		c.emit(opNull, 0, "")
		return nil
	case *Identifier:
		c.emit(opGetVar, 0, node.Name)
		return nil
	case *PrefixExpression:
		if err := c.compileExpression(node.Right); err != nil {
			return err
		}
		c.emit(opUnary, 0, node.Operator)
		return nil
	case *InfixExpression:
		if node.Operator == "&&" || node.Operator == "||" {
			return &errUnsupportedNode{node: "short-circuit infix"}
		}
		if err := c.compileExpression(node.Left); err != nil {
			return err
		}
		if err := c.compileExpression(node.Right); err != nil {
			return err
		}
		c.emit(opBinary, 0, node.Operator)
		return nil
	case *CallExpression:
		return &errUnsupportedNode{node: "call expression"}
	default:
		return &errUnsupportedNode{node: fmt.Sprintf("%T", exp)}
	}
}

type vm struct {
	program *bytecodeProgram
	env     *Environment
	stack   []Object
	ip      int
}

func runOnVM(program *bytecodeProgram, env *Environment) Object {
	v := &vm{
		program: program,
		env:     env,
		stack:   make([]Object, 0, 256),
		ip:      0,
	}
	return v.run()
}

func (v *vm) push(obj Object) {
	v.stack = append(v.stack, obj)
}

func (v *vm) pop() (Object, bool) {
	if len(v.stack) == 0 {
		return nil, false
	}
	last := v.stack[len(v.stack)-1]
	v.stack = v.stack[:len(v.stack)-1]
	return last, true
}

func (v *vm) run() Object {
	for v.ip < len(v.program.instructions) {
		ins := v.program.instructions[v.ip]
		switch ins.op {
		case opConstant:
			if ins.arg < 0 || ins.arg >= len(v.program.constants) {
				return newError("vm constant index out of bounds: %d", ins.arg)
			}
			v.push(v.program.constants[ins.arg])
		case opGetVar:
			if obj, ok := v.vmLookup(ins.s); ok {
				v.push(obj)
			} else {
				return newError("identifier not found: %s", ins.s)
			}
		case opSetVar:
			val, ok := v.pop()
			if !ok {
				return newError("vm stack underflow on set")
			}
			v.env.Set(ins.s, val)
			v.push(NULL)
		case opPop:
			if _, ok := v.pop(); !ok {
				return newError("vm stack underflow on pop")
			}
		case opUnary:
			right, ok := v.pop()
			if !ok {
				return newError("vm stack underflow on unary op")
			}
			res := evalPrefixExpression(ins.s, right)
			if isError(res) {
				return res
			}
			v.push(res)
		case opBinary:
			right, okR := v.pop()
			left, okL := v.pop()
			if !okR || !okL {
				return newError("vm stack underflow on binary op")
			}
			res := evalInfixExpression(ins.s, left, right)
			if isError(res) {
				return res
			}
			v.push(res)
		case opCall:
			argc := ins.arg
			if argc < 0 || len(v.stack) < argc+1 {
				return newError("vm invalid call arity: %d", argc)
			}
			args := make([]Object, argc)
			for i := argc - 1; i >= 0; i-- {
				arg, _ := v.pop()
				args[i] = arg
			}
			fn, _ := v.pop()
			res := applyFunction(fn, args, v.env, nil)
			if isError(res) {
				return res
			}
			v.push(res)
		case opReturn:
			ret, ok := v.pop()
			if !ok {
				return NULL
			}
			return ret
		case opNull:
			v.push(NULL)
		case opPrint:
			val, ok := v.pop()
			if !ok {
				return newError("vm stack underflow on print")
			}
			var out = io.Writer(os.Stdout)
			if v.env != nil && v.env.output != nil {
				out = v.env.output
			}
			fmt.Fprintln(out, val.Inspect())
			v.push(NULL)
		default:
			return newError("unknown vm opcode: %d", ins.op)
		}
		v.ip++
	}
	if len(v.stack) == 0 {
		return NULL
	}
	return v.stack[len(v.stack)-1]
}

func (v *vm) vmLookup(name string) (Object, bool) {
	if v.env != nil {
		if obj, ok := v.env.Get(name); ok {
			return obj, true
		}
	}
	if builtin, ok := builtins[name]; ok {
		if v.env != nil {
			return builtin.bindEnv(v.env), true
		}
		return builtin, true
	}
	return nil, false
}

func runProgram(program *Program, env *Environment) Object {
	return evalProgram(program, env)
}
