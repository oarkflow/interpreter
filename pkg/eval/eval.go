// Package eval implements the tree-walking evaluator for the interpreter.
package eval

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/lexer"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/parser"
	"github.com/oarkflow/interpreter/pkg/security"
)

func init() {
	object.EvalFn = Eval
}

// ---------------------------------------------------------------------------
// Pluggable hooks for functionality that lives outside this package.
// ---------------------------------------------------------------------------

// ResolveImportPathFn resolves a module import path. If nil, imports will fail.
var ResolveImportPathFn func(importPath string, env *object.Environment) (string, error)

// DotExpressionHook is called for dot expressions on types not handled by
// the built-in dispatch. Return non-nil to use the result.
var DotExpressionHook func(left object.Object, name string) object.Object

// BytecodeCompileFn compiles a *ast.Program to bytecode. If nil, the bytecode
// fast-path is disabled and the tree-walk evaluator is used for all statements.
var BytecodeCompileFn func(program *ast.Program) (any, error)

// BytecodeRunFn executes a compiled bytecode program. The first argument is
// the opaque value returned by BytecodeCompileFn.
var BytecodeRunFn func(compiled any, env *object.Environment) object.Object

// BytecodeIsUnsupportedErr returns true when the error from BytecodeCompileFn
// indicates an unsupported AST node (meaning we should fall back to tree-walk).
var BytecodeIsUnsupportedErr func(err error) bool

type bytecodeStatementCacheEntry struct {
	compiled    any
	unsupported bool
}

var bytecodeStatementCache = struct {
	sync.RWMutex
	items map[ast.Statement]bytecodeStatementCacheEntry
}{
	items: make(map[ast.Statement]bytecodeStatementCacheEntry),
}

const maxBytecodeStatementCacheEntries = 512

// ---------------------------------------------------------------------------
// RuntimeLimits enter helper
// ---------------------------------------------------------------------------

func runtimeLimitsEnter(rl *object.RuntimeLimits) object.Object {
	rl.Steps++
	if rl.MaxSteps > 0 && rl.Steps > rl.MaxSteps {
		return object.NewError("execution step limit exceeded (%d)", rl.MaxSteps)
	}
	if !rl.Deadline.IsZero() && time.Now().After(rl.Deadline) {
		return object.NewError("execution timeout exceeded")
	}
	if rl.Ctx != nil {
		select {
		case <-rl.Ctx.Done():
			if err := rl.Ctx.Err(); err != nil {
				return object.NewError("execution cancelled: %s", err.Error())
			}
			return object.NewError("execution cancelled")
		default:
		}
	}
	if rl.MaxHeapBytes > 0 && rl.HeapCheckEvery > 0 && rl.Steps%rl.HeapCheckEvery == 0 {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		if ms.Alloc > rl.MaxHeapBytes {
			return object.NewError("heap usage exceeded (%d MB)", rl.MaxHeapBytes/(1024*1024))
		}
	}
	return nil
}

func writeRuntimeOutput(env *object.Environment, out io.Writer, text string) object.Object {
	if env == nil || env.RuntimeLimits == nil || env.RuntimeLimits.MaxOutputBytes <= 0 {
		fmt.Fprint(out, text)
		return nil
	}
	rl := env.RuntimeLimits
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
	return nil
}

// ---------------------------------------------------------------------------
// Eval – main entry point
// ---------------------------------------------------------------------------

// Eval evaluates an AST node in the given environment.
func Eval(node ast.Node, env *object.Environment) object.Object {
	if env != nil && env.RuntimeLimits != nil {
		if errObj := runtimeLimitsEnter(env.RuntimeLimits); errObj != nil {
			return errObj
		}
	}

	switch node := node.(type) {
	case *ast.Program:
		return evalProgram(node, env)

	case *ast.ExpressionStatement:
		return Eval(node.Expression, env)

	case *ast.IntegerLiteral:
		return object.IntegerObj(node.Value)

	case *ast.FloatLiteral:
		return &object.Float{Value: node.Value}

	case *ast.StringLiteral:
		return &object.String{Value: node.Value}

	case *ast.BooleanLiteral:
		return object.NativeBoolToBooleanObject(node.Value)

	case *ast.NullLiteral:
		return object.NULL

	case *ast.ArrayLiteral:
		elements := evalExpressions(node.Elements, env)
		if len(elements) == 1 && object.IsError(elements[0]) {
			return elements[0]
		}
		return &object.Array{Elements: elements}

	case *ast.HashLiteral:
		return evalHashLiteral(node, env)

	case *ast.IndexExpression:
		left := Eval(node.Left, env)
		if object.IsError(left) {
			return left
		}
		index := Eval(node.Index, env)
		if object.IsError(index) {
			return index
		}
		return evalIndexExpression(left, index)

	case *ast.DotExpression:
		left := Eval(node.Left, env)
		if object.IsError(left) {
			return left
		}
		return evalDotExpression(left, node.Right.Name)

	case *ast.OptionalDotExpression:
		left := Eval(node.Left, env)
		if object.IsError(left) {
			return left
		}
		if left == object.NULL || left == nil {
			return object.NULL
		}
		result := evalDotExpression(left, node.Right.Name)
		if object.IsError(result) {
			return object.NULL
		}
		return result

	case *ast.PrefixExpression:
		right := Eval(node.Right, env)
		if object.IsError(right) {
			return right
		}
		return evalPrefixExpression(node.Operator, right)

	case *ast.AssignExpression:
		val := Eval(node.Value, env)
		if object.IsError(val) {
			return val
		}
		return evalTargetAssign(node.Target, val, env)

	case *ast.CompoundAssignExpression:
		current := evalTargetGet(node.Target, env)
		if object.IsError(current) {
			return current
		}
		if node.Operator == "??" {
			if current != object.NULL && current != nil {
				return current
			}
			right := Eval(node.Value, env)
			if object.IsError(right) {
				return right
			}
			return evalTargetAssign(node.Target, right, env)
		}
		if node.Operator == "&&" {
			if !object.IsTruthy(current) {
				return current
			}
			right := Eval(node.Value, env)
			if object.IsError(right) {
				return right
			}
			return evalTargetAssign(node.Target, right, env)
		}
		if node.Operator == "||" {
			if object.IsTruthy(current) {
				return current
			}
			right := Eval(node.Value, env)
			if object.IsError(right) {
				return right
			}
			return evalTargetAssign(node.Target, right, env)
		}
		right := Eval(node.Value, env)
		if object.IsError(right) {
			return right
		}
		result := evalInfixExpression(node.Operator, current, right)
		if object.IsError(result) {
			return result
		}
		return evalTargetAssign(node.Target, result, env)

	case *ast.PostfixExpression:
		current := evalTargetGet(node.Target, env)
		if object.IsError(current) {
			return current
		}
		var newVal object.Object
		switch node.Operator {
		case "++":
			switch v := current.(type) {
			case *object.Integer:
				newVal = object.IntegerObj(v.Value + 1)
			case *object.Float:
				newVal = &object.Float{Value: v.Value + 1}
			default:
				return object.NewError("postfix ++ not supported on %s", current.Type())
			}
		case "--":
			switch v := current.(type) {
			case *object.Integer:
				newVal = object.IntegerObj(v.Value - 1)
			case *object.Float:
				newVal = &object.Float{Value: v.Value - 1}
			default:
				return object.NewError("postfix -- not supported on %s", current.Type())
			}
		}
		evalTargetAssign(node.Target, newVal, env)
		return current

	case *ast.InfixExpression:
		if node.Operator == "??" {
			left := Eval(node.Left, env)
			if object.IsError(left) {
				return left
			}
			if left == object.NULL || left == nil {
				return Eval(node.Right, env)
			}
			return left
		}
		left := Eval(node.Left, env)
		if object.IsError(left) {
			return left
		}
		if node.Operator == "&&" {
			if !object.IsTruthy(left) {
				return object.FALSE
			}
		}
		if node.Operator == "||" {
			if object.IsTruthy(left) {
				return object.TRUE
			}
		}
		right := Eval(node.Right, env)
		if object.IsError(right) {
			return right
		}
		return evalInfixExpression(node.Operator, left, right)

	case *ast.BlockStatement:
		return evalBlockStatement(node, env)

	case *ast.IfExpression:
		return evalIfExpression(node, env)

	case *ast.WhileStatement:
		return evalWhileStatement(node, env)

	case *ast.DoWhileStatement:
		return evalDoWhileStatement(node, env)

	case *ast.ForStatement:
		return evalForStatement(node, env)

	case *ast.ForInStatement:
		return evalForInStatement(node, env)

	case *ast.ImportStatement:
		return evalImportStatement(node, env)

	case *ast.ExportStatement:
		return evalExportStatement(node, env)

	case *ast.ThrowStatement:
		thrown := Eval(node.Value, env)
		if object.IsError(thrown) {
			return thrown
		}
		if errHash, ok := thrown.(*object.Hash); ok {
			if pair, ok := errHash.Pairs[(&object.String{Value: "message"}).HashKey()]; ok {
				msg := objectToDisplayString(pair.Value)
				errObj := &object.Error{Message: msg}
				if codePair, ok := errHash.Pairs[(&object.String{Value: "code"}).HashKey()]; ok {
					if codeStr, ok := codePair.Value.(*object.String); ok && strings.TrimSpace(codeStr.Value) != "" {
						errObj.Message = fmt.Sprintf("[%s] %s", codeStr.Value, msg)
					}
				}
				return errObj
			}
		}
		return object.NewError("%s", objectErrorString(thrown))

	case *ast.TryCatchExpression:
		return evalTryCatchExpression(node, env)

	case *ast.TernaryExpression:
		condition := Eval(node.Condition, env)
		if object.IsError(condition) {
			return condition
		}
		if object.IsTruthy(condition) {
			return Eval(node.Consequence, env)
		}
		return Eval(node.Alternative, env)

	case *ast.SwitchStatement:
		return evalSwitchStatement(node, env)

	case *ast.MatchExpression:
		return evalMatchExpression(node, env)

	case *ast.RangeExpression:
		return evalRangeExpression(node, env)

	case *ast.AwaitExpression:
		obj := Eval(node.Value, env)
		if object.IsError(obj) {
			return obj
		}
		if lazy, ok := obj.(*object.LazyValue); ok {
			obj = lazy.Force()
		}
		if future, ok := obj.(*object.Future); ok {
			return future.Resolve()
		}
		return obj

	case *ast.LazyExpression:
		return &object.LazyValue{Env: env, Expr: node.Value}

	case *ast.TemplateLiteral:
		return evalTemplateLiteral(node, env)

	case *ast.ReturnStatement:
		val := Eval(node.ReturnValue, env)
		if object.IsError(val) {
			return val
		}
		return &object.ReturnValue{Value: val}

	case *ast.BreakStatement:
		return object.BREAK

	case *ast.ContinueStatement:
		return object.CONT

	case *ast.LetStatement:
		val := Eval(node.Value, env)
		if object.IsError(val) {
			return val
		}
		if len(node.Names) > 1 {
			arr, ok := val.(*object.Array)
			if !ok {
				return object.NewError("assignment mismatch: %d variables but 1 value", len(node.Names))
			}
			if len(node.Names) != len(arr.Elements) {
				return object.NewError("assignment mismatch: %d variables but %d values", len(node.Names), len(arr.Elements))
			}
			for i, name := range node.Names {
				env.Set(name.Name, arr.Elements[i])
			}
		} else {
			targetName := node.Name
			if targetName == nil && len(node.Names) > 0 {
				targetName = node.Names[0]
			}
			if targetName != nil {
				if node.TypeName != "" {
					if !valueMatchesTypeName(val, node.TypeName) {
						return object.NewError("type mismatch for %s: expected %s, got %s", targetName.Name, node.TypeName, val.Type())
					}
				}
				env.Set(targetName.Name, maybeWrapOwned(val, env))
			}
		}

	case *ast.DestructureLetStatement:
		val := Eval(node.Value, env)
		if object.IsError(val) {
			return val
		}
		return evalDestructure(node.Pattern, val, env)

	case *ast.PrintStatement:
		val := Eval(node.Expression, env)
		if object.IsError(val) {
			return val
		}
		var out io.Writer = os.Stdout
		if env != nil && env.Output != nil {
			out = env.Output
		}
		if errObj := writeRuntimeOutput(env, out, val.Inspect()+"\n"); errObj != nil {
			return errObj
		}
		return object.NULL

	case *ast.Identifier:
		return evalIdentifier(node, env)

	case *ast.FunctionLiteral:
		name := ""
		if node.Name != nil {
			name = node.Name.Name
		}
		return &object.Function{
			Name:       name,
			Parameters: node.Parameters,
			ParamTypes: node.ParamTypes,
			Defaults:   node.Defaults,
			HasRest:    node.HasRest,
			ReturnType: node.ReturnType,
			Env:        env,
			Body:       node.Body,
			IsAsync:    node.IsAsync,
		}

	case *ast.ClassStatement:
		return evalClassStatement(node, env)

	case *ast.InterfaceStatement:
		methods := make(map[string]*ast.InterfaceMethod, len(node.Methods))
		for _, m := range node.Methods {
			methods[m.Name.Name] = m
		}
		iface := &object.InterfaceLiteral{Methods: methods}
		env.Set(node.Name.Name, iface)
		return iface

	case *ast.InitStatement:
		if node.Body == nil {
			return object.NULL
		}
		return Eval(node.Body, env)

	case *ast.TestStatement:
		if node.Body == nil {
			return object.NULL
		}
		res := Eval(node.Body, env)
		if object.IsError(res) {
			return res
		}
		return object.NULL

	case *ast.TypeDeclarationStatement:
		return evalTypeDeclaration(node, env)

	case *ast.CallExpression:
		return evalCallExpression(node, env)

	default:
		return object.NewError("unsupported AST node: %T", node)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Complex statement evaluators (extracted for readability)
// ---------------------------------------------------------------------------

func evalClassStatement(node *ast.ClassStatement, env *object.Environment) object.Object {
	instanceMethods := make(map[object.HashKey]object.HashPair, len(node.Methods))
	for _, method := range node.Methods {
		fn := &object.Function{
			Name:       method.Name.Name,
			Parameters: method.Parameters,
			Defaults:   make([]ast.Expression, len(method.Parameters)),
			HasRest:    false,
			Env:        env,
			Body:       method.Body,
		}
		name := &object.String{Value: method.Name.Name}
		instanceMethods[name.HashKey()] = object.HashPair{Key: name, Value: fn}
	}
	classFn := &object.Builtin{FnWithEnv: func(bindEnv *object.Environment, args ...object.Object) object.Object {
		obj := &object.Hash{Pairs: make(map[object.HashKey]object.HashPair, len(instanceMethods)+1)}
		for k, pair := range instanceMethods {
			obj.Pairs[k] = pair
		}
		if ctorPair, ok := obj.Pairs[(&object.String{Value: "constructor"}).HashKey()]; ok {
			if ctorFn, ok := ctorPair.Value.(*object.Function); ok {
				bound := &object.Function{
					Name:       ctorFn.Name,
					Parameters: ctorFn.Parameters,
					Defaults:   ctorFn.Defaults,
					HasRest:    ctorFn.HasRest,
					Env:        object.NewEnclosedEnvironment(bindEnv),
					Body:       ctorFn.Body,
				}
				bound.Env.Set("this", obj)
				result := ApplyFunction(bound, args, bindEnv, nil)
				if object.IsError(result) {
					return result
				}
			}
		}
		for hk, pair := range obj.Pairs {
			if fn, ok := pair.Value.(*object.Function); ok {
				fnCopy := fn
				wrapped := &object.Builtin{FnWithEnv: func(callEnv *object.Environment, callArgs ...object.Object) object.Object {
					methodEnv := object.NewEnclosedEnvironment(fnCopy.Env)
					methodEnv.Set("this", obj)
					boundFn := &object.Function{
						Name:       fnCopy.Name,
						Parameters: fnCopy.Parameters,
						Defaults:   fnCopy.Defaults,
						HasRest:    fnCopy.HasRest,
						Env:        methodEnv,
						Body:       fnCopy.Body,
					}
					return ApplyFunction(boundFn, callArgs, callEnv, nil)
				}}
				obj.Pairs[hk] = object.HashPair{Key: pair.Key, Value: wrapped}
			}
		}
		return obj
	}}
	env.Set(node.Name.Name, classFn.BindEnv(env))
	return classFn.BindEnv(env)
}

func evalTypeDeclaration(node *ast.TypeDeclarationStatement, env *object.Environment) object.Object {
	if node.Name == nil || len(node.Variants) == 0 {
		return object.NewError("invalid type declaration")
	}
	def := &object.ADTTypeDef{
		TypeName: node.Name.Name,
		Variants: make(map[string]int, len(node.Variants)),
		Order:    make([]string, 0, len(node.Variants)),
	}
	for _, variant := range node.Variants {
		if variant == nil || variant.Name == nil {
			return object.NewError("invalid ADT variant declaration")
		}
		if _, exists := def.Variants[variant.Name.Name]; exists {
			return object.NewError("duplicate ADT variant: %s", variant.Name.Name)
		}
		def.Variants[variant.Name.Name] = len(variant.Fields)
		def.Order = append(def.Order, variant.Name.Name)
		fieldNames := make([]string, 0, len(variant.Fields))
		for _, f := range variant.Fields {
			fieldNames = append(fieldNames, f.Name)
		}
		variantName := variant.Name.Name
		ctor := &object.Builtin{FnWithEnv: func(bindEnv *object.Environment, args ...object.Object) object.Object {
			if len(args) != len(fieldNames) {
				return object.NewError("%s expects %d argument(s), got %d", variantName, len(fieldNames), len(args))
			}
			vals := make([]object.Object, len(args))
			copy(vals, args)
			allVariants := make([]string, len(def.Order))
			copy(allVariants, def.Order)
			return &object.ADTValue{
				TypeName:    def.TypeName,
				VariantName: variantName,
				FieldNames:  append([]string(nil), fieldNames...),
				Values:      vals,
				AllVariants: allVariants,
			}
		}}
		env.Set(variantName, ctor.BindEnv(env))
	}
	env.Set(node.Name.Name, def)
	return def
}

func evalCallExpression(node *ast.CallExpression, env *object.Environment) object.Object {
	if env != nil && env.RuntimeLimits != nil && env.RuntimeLimits.MaxDepth > 0 {
		env.RuntimeLimits.CurrentDepth++
		defer func() {
			env.RuntimeLimits.CurrentDepth--
		}()
		if env.RuntimeLimits.CurrentDepth > env.RuntimeLimits.MaxDepth {
			return object.NewError("max recursion depth exceeded (%d)", env.RuntimeLimits.MaxDepth)
		}
	}

	function := Eval(node.Function, env)
	if object.IsError(function) {
		return function
	}

	args := evalExpressions(node.Arguments, env)
	if len(args) == 1 && object.IsError(args[0]) {
		return args[0]
	}

	result := ApplyFunction(function, args, env, node)
	if runtimeErr, ok := result.(*object.Error); ok {
		frame := callFrameFromExpression(node.Function, env, node.Line, node.Column)
		if len(runtimeErr.Stack) == 0 || !object.SameCallFrame(runtimeErr.Stack[len(runtimeErr.Stack)-1], frame) {
			runtimeErr = runtimeErr.WithFrame(frame)
		}
		if len(runtimeErr.Stack) == 0 && env != nil && len(env.CallStack) > 0 {
			cloned := *runtimeErr
			cloned.Stack = cloneCallStack(env.CallStack)
			runtimeErr = &cloned
		}
		return runtimeErr
	}
	return result
}

// ---------------------------------------------------------------------------
// Program / Block evaluation
// ---------------------------------------------------------------------------

func evalProgram(program *ast.Program, env *object.Environment) object.Object {
	var result object.Object
	if env != nil {
		if inited, ok := env.Get("__module_init_done"); !ok || !object.IsTruthy(inited) {
			for _, statement := range program.Statements {
				if initStmt, ok := statement.(*ast.InitStatement); ok {
					initResult := Eval(initStmt.Body, env)
					if object.IsError(initResult) {
						return initResult
					}
				}
			}
			env.Set("__module_init_done", object.TRUE)
		}
	}

	for _, statement := range program.Statements {
		if _, isInit := statement.(*ast.InitStatement); isInit {
			continue
		}
		result = runProgramStatement(statement, env)
		switch result := result.(type) {
		case *object.ReturnValue:
			return result.Value
		case *object.Error:
			return result
		}
	}
	return result
}

// runProgramStatement attempts to run a single top-level statement through
// the bytecode VM. If the VM cannot handle it (unsupported node type), it
// falls back to the tree-walk evaluator.
func runProgramStatement(statement ast.Statement, env *object.Environment) object.Object {
	// Statements that affect control flow must go through tree-walk.
	switch statement.(type) {
	case *ast.ReturnStatement, *ast.BreakStatement, *ast.ContinueStatement,
		*ast.ForStatement, *ast.ForInStatement, *ast.WhileStatement, *ast.DoWhileStatement:
		return Eval(statement, env)
	}

	// If bytecode hooks are not wired up, fall back to tree-walk.
	if BytecodeCompileFn == nil || BytecodeRunFn == nil {
		return Eval(statement, env)
	}

	bytecodeStatementCache.RLock()
	entry, cached := bytecodeStatementCache.items[statement]
	bytecodeStatementCache.RUnlock()
	if cached {
		if entry.unsupported {
			return Eval(statement, env)
		}
		return BytecodeRunFn(entry.compiled, env)
	}

	switch statement.(type) {
	case *ast.LetStatement, *ast.ExpressionStatement, *ast.PrintStatement:
		return Eval(statement, env)
	}

	prog := &ast.Program{Statements: []ast.Statement{statement}}
	compiled, err := BytecodeCompileFn(prog)
	if err == nil {
		storeBytecodeStatementCacheEntry(statement, bytecodeStatementCacheEntry{compiled: compiled})
		return BytecodeRunFn(compiled, env)
	}
	// Unsupported node → fall back to tree-walk.
	if BytecodeIsUnsupportedErr != nil && BytecodeIsUnsupportedErr(err) {
		storeBytecodeStatementCacheEntry(statement, bytecodeStatementCacheEntry{unsupported: true})
		return Eval(statement, env)
	}
	return object.NewError("bytecode compile failed: %s", err)
}

func storeBytecodeStatementCacheEntry(key ast.Statement, entry bytecodeStatementCacheEntry) {
	bytecodeStatementCache.Lock()
	if len(bytecodeStatementCache.items) >= maxBytecodeStatementCacheEntries {
		bytecodeStatementCache.items = make(map[ast.Statement]bytecodeStatementCacheEntry)
	}
	bytecodeStatementCache.items[key] = entry
	bytecodeStatementCache.Unlock()
}

func evalBlockStatement(block *ast.BlockStatement, env *object.Environment) object.Object {
	var result object.Object
	var setup *ast.BlockStatement
	var teardown *ast.BlockStatement
	for _, statement := range block.Statements {
		if exprStmt, ok := statement.(*ast.ExpressionStatement); ok {
			if ident, ok := exprStmt.Expression.(*ast.Identifier); ok {
				if ident.Name == "setup" {
					if blockStmt, ok := nextStatementBlock(block.Statements, statement); ok {
						setup = blockStmt
					}
				}
				if ident.Name == "teardown" {
					if blockStmt, ok := nextStatementBlock(block.Statements, statement); ok {
						teardown = blockStmt
					}
				}
			}
		}
	}
	if setup != nil {
		setupResult := Eval(setup, env)
		if object.IsError(setupResult) {
			return setupResult
		}
	}

	for _, statement := range block.Statements {
		if exprStmt, ok := statement.(*ast.ExpressionStatement); ok {
			if ident, ok := exprStmt.Expression.(*ast.Identifier); ok {
				if ident.Name == "setup" || ident.Name == "teardown" {
					continue
				}
			}
		}
		result = Eval(statement, env)
		if result != nil {
			rt := result.Type()
			if rt == object.RETURN_VALUE_OBJ || rt == object.BREAK_OBJ || rt == object.CONTINUE_OBJ {
				return result
			}
			if object.IsError(result) {
				return result
			}
		}
	}
	if teardown != nil {
		teardownResult := Eval(teardown, env)
		if object.IsError(teardownResult) {
			return teardownResult
		}
	}
	return result
}

func nextStatementBlock(stmts []ast.Statement, current ast.Statement) (*ast.BlockStatement, bool) {
	for i, s := range stmts {
		if s != current {
			continue
		}
		if i+1 < len(stmts) {
			if exprStmt, ok := stmts[i+1].(*ast.ExpressionStatement); ok {
				if block, ok := exprStmt.Expression.(*ast.FunctionLiteral); ok && block.Body != nil {
					return block.Body, true
				}
			}
		}
		break
	}
	return nil, false
}

// ---------------------------------------------------------------------------
// Control flow statements
// ---------------------------------------------------------------------------

func evalIfExpression(ie *ast.IfExpression, env *object.Environment) object.Object {
	condition := Eval(ie.Condition, env)
	if object.IsError(condition) {
		return condition
	}
	if object.IsTruthy(condition) {
		return Eval(ie.Consequence, env)
	} else if ie.Alternative != nil {
		return Eval(ie.Alternative, env)
	}
	return object.NULL
}

func evalWhileStatement(ws *ast.WhileStatement, env *object.Environment) object.Object {
	var result object.Object = object.NULL
	for {
		condition := Eval(ws.Condition, env)
		if object.IsError(condition) {
			return condition
		}
		if !object.IsTruthy(condition) {
			break
		}
		result = Eval(ws.Body, env)
		if object.IsError(result) {
			return result
		}
		if result != nil {
			rt := result.Type()
			if rt == object.RETURN_VALUE_OBJ {
				return result
			}
			if rt == object.BREAK_OBJ {
				result = object.NULL
				break
			}
			if rt == object.CONTINUE_OBJ {
				result = object.NULL
				continue
			}
		}
	}
	return result
}

func evalDoWhileStatement(dw *ast.DoWhileStatement, env *object.Environment) object.Object {
	var result object.Object = object.NULL
	for {
		result = Eval(dw.Body, env)
		if object.IsError(result) {
			return result
		}
		if result != nil {
			rt := result.Type()
			if rt == object.RETURN_VALUE_OBJ {
				return result
			}
			if rt == object.BREAK_OBJ {
				return object.NULL
			}
			if rt == object.CONTINUE_OBJ {
				result = object.NULL
			}
		}
		condition := Eval(dw.Condition, env)
		if object.IsError(condition) {
			return condition
		}
		if !object.IsTruthy(condition) {
			break
		}
	}
	return result
}

func evalForStatement(fs *ast.ForStatement, env *object.Environment) object.Object {
	if result, ok := evalFastForStatement(fs, env); ok {
		return result
	}
	if fs.Init != nil {
		initResult := Eval(fs.Init, env)
		if object.IsError(initResult) {
			return initResult
		}
	}
	var result object.Object = object.NULL
	for {
		if fs.Condition != nil {
			condition := Eval(fs.Condition, env)
			if object.IsError(condition) {
				return condition
			}
			if !object.IsTruthy(condition) {
				break
			}
		}
		result = Eval(fs.Body, env)
		if object.IsError(result) {
			return result
		}
		if result != nil {
			rt := result.Type()
			if rt == object.RETURN_VALUE_OBJ {
				return result
			}
			if rt == object.BREAK_OBJ {
				result = object.NULL
				break
			}
			if rt == object.CONTINUE_OBJ {
				result = object.NULL
			}
		}
		if fs.Post != nil {
			post := Eval(fs.Post, env)
			if object.IsError(post) {
				return post
			}
		}
	}
	return result
}

func evalForInStatement(fi *ast.ForInStatement, env *object.Environment) object.Object {
	iterable := Eval(fi.Iterable, env)
	if object.IsError(iterable) {
		return iterable
	}
	var result object.Object = object.NULL
	switch iter := iterable.(type) {
	case *object.Array:
		for i, el := range iter.Elements {
			loopEnv := object.NewEnclosedEnvironment(env)
			if fi.KeyName != nil {
				loopEnv.Set(fi.KeyName.Name, object.IntegerObj(int64(i)))
			}
			loopEnv.Set(fi.ValueName.Name, el)
			result = Eval(fi.Body, loopEnv)
			if object.IsError(result) {
				return result
			}
			if result != nil {
				rt := result.Type()
				if rt == object.RETURN_VALUE_OBJ {
					return result
				}
				if rt == object.BREAK_OBJ {
					return object.NULL
				}
				if rt == object.CONTINUE_OBJ {
					result = object.NULL
					continue
				}
			}
		}
	case *object.Hash:
		for _, pair := range iter.Pairs {
			loopEnv := object.NewEnclosedEnvironment(env)
			if fi.KeyName != nil {
				loopEnv.Set(fi.KeyName.Name, pair.Key)
			}
			loopEnv.Set(fi.ValueName.Name, pair.Value)
			result = Eval(fi.Body, loopEnv)
			if object.IsError(result) {
				return result
			}
			if result != nil {
				rt := result.Type()
				if rt == object.RETURN_VALUE_OBJ {
					return result
				}
				if rt == object.BREAK_OBJ {
					return object.NULL
				}
				if rt == object.CONTINUE_OBJ {
					result = object.NULL
					continue
				}
			}
		}
	case *object.String:
		for i, ch := range iter.Value {
			loopEnv := object.NewEnclosedEnvironment(env)
			if fi.KeyName != nil {
				loopEnv.Set(fi.KeyName.Name, object.IntegerObj(int64(i)))
			}
			loopEnv.Set(fi.ValueName.Name, &object.String{Value: string(ch)})
			result = Eval(fi.Body, loopEnv)
			if object.IsError(result) {
				return result
			}
			if result != nil {
				rt := result.Type()
				if rt == object.RETURN_VALUE_OBJ {
					return result
				}
				if rt == object.BREAK_OBJ {
					return object.NULL
				}
				if rt == object.CONTINUE_OBJ {
					result = object.NULL
					continue
				}
			}
		}
	default:
		return object.NewError("cannot iterate over %s", iterable.Type())
	}
	return result
}

func evalSwitchStatement(ss *ast.SwitchStatement, env *object.Environment) object.Object {
	switchVal := Eval(ss.Value, env)
	if object.IsError(switchVal) {
		return switchVal
	}
	for _, sc := range ss.Cases {
		for _, caseVal := range sc.Values {
			cv := Eval(caseVal, env)
			if object.IsError(cv) {
				return cv
			}
			if objectsEqual(switchVal, cv) {
				result := Eval(sc.Body, env)
				return unwrapSwitchResult(result)
			}
		}
	}
	if ss.Default != nil {
		result := Eval(ss.Default, env)
		return unwrapSwitchResult(result)
	}
	return object.NULL
}

func unwrapSwitchResult(result object.Object) object.Object {
	if result == nil {
		return object.NULL
	}
	if result.Type() == object.BREAK_OBJ {
		return object.NULL
	}
	return result
}

func evalTemplateLiteral(tl *ast.TemplateLiteral, env *object.Environment) object.Object {
	var out strings.Builder
	for _, part := range tl.Parts {
		val := Eval(part, env)
		if object.IsError(val) {
			return val
		}
		out.WriteString(val.Inspect())
	}
	return &object.String{Value: out.String()}
}

func evalTryCatchExpression(node *ast.TryCatchExpression, env *object.Environment) object.Object {
	tryResult := Eval(node.TryBlock, env)

	var result object.Object
	if object.IsError(tryResult) && node.CatchBlock != nil {
		catchEnv := object.NewEnclosedEnvironment(env)
		if node.CatchIdent != nil {
			if node.CatchType != "" {
				catchEnv.Set(node.CatchIdent.Name, errorObjectToHash(tryResult))
			} else {
				catchEnv.Set(node.CatchIdent.Name, &object.String{Value: objectErrorString(tryResult)})
			}
		}
		result = Eval(node.CatchBlock, catchEnv)
	} else {
		result = tryResult
	}

	if node.FinallyBlock != nil {
		finallyResult := Eval(node.FinallyBlock, env)
		if object.IsError(finallyResult) {
			return finallyResult
		}
	}

	if node.CatchType != "" && node.CatchBlock != nil && object.IsError(result) {
		if _, ok := result.(*object.Error); ok {
			return errorObjectToHash(result)
		}
	}

	return result
}

// ---------------------------------------------------------------------------
// Target get / assign
// ---------------------------------------------------------------------------

func evalTargetGet(target ast.Expression, env *object.Environment) object.Object {
	switch t := target.(type) {
	case *ast.Identifier:
		val, ok := env.Get(t.Name)
		if !ok {
			return object.NewError("variable %s not declared", t.Name)
		}
		return val
	case *ast.DotExpression:
		obj := Eval(t.Left, env)
		if object.IsError(obj) {
			return obj
		}
		if owned, ok := obj.(*object.OwnedValue); ok {
			obj = owned.Inner
		}
		hash, ok := obj.(*object.Hash)
		if !ok {
			return object.NewError("dot access on %s is not supported", obj.Type())
		}
		key := &object.String{Value: t.Right.Name}
		pair, ok := hash.Pairs[key.HashKey()]
		if !ok {
			return object.NULL
		}
		return pair.Value
	case *ast.IndexExpression:
		obj := Eval(t.Left, env)
		if object.IsError(obj) {
			return obj
		}
		if owned, ok := obj.(*object.OwnedValue); ok {
			obj = owned.Inner
		}
		index := Eval(t.Index, env)
		if object.IsError(index) {
			return index
		}
		return evalIndexExpression(obj, index)
	default:
		return object.NewError("invalid assignment target")
	}
}

func evalTargetAssign(target ast.Expression, val object.Object, env *object.Environment) object.Object {
	if owned, ok := val.(*object.OwnedValue); ok {
		val = owned.Inner
	}
	switch t := target.(type) {
	case *ast.Identifier:
		if _, ok := env.Assign(t.Name, val); ok {
			return val
		}
		return object.NewError("variable %s not declared", t.Name)
	case *ast.DotExpression:
		obj := Eval(t.Left, env)
		if object.IsError(obj) {
			return obj
		}
		if _, ok := obj.(*object.ImmutableValue); ok {
			return object.NewError("cannot mutate immutable value")
		}
		hash, ok := obj.(*object.Hash)
		if !ok {
			return object.NewError("cannot set property on %s", obj.Type())
		}
		key := &object.String{Value: t.Right.Name}
		hash.Pairs[key.HashKey()] = object.HashPair{Key: key, Value: val}
		return val
	case *ast.IndexExpression:
		obj := Eval(t.Left, env)
		if object.IsError(obj) {
			return obj
		}
		if _, ok := obj.(*object.ImmutableValue); ok {
			return object.NewError("cannot mutate immutable value")
		}
		index := Eval(t.Index, env)
		if object.IsError(index) {
			return index
		}
		switch o := obj.(type) {
		case *object.Array:
			idx, ok := index.(*object.Integer)
			if !ok {
				return object.NewError("array index must be integer, got %s", index.Type())
			}
			i := int(idx.Value)
			if i < 0 || i >= len(o.Elements) {
				return object.NewError("index %d out of range [0..%d]", i, len(o.Elements)-1)
			}
			o.Elements[i] = val
			return val
		case *object.Hash:
			hashKey, ok := index.(object.Hashable)
			if !ok {
				return object.NewError("unusable as hash key: %s", index.Type())
			}
			o.Pairs[hashKey.HashKey()] = object.HashPair{Key: index, Value: val}
			return val
		default:
			return object.NewError("index assignment not supported on %s", obj.Type())
		}
	default:
		return object.NewError("invalid assignment target")
	}
}

// ---------------------------------------------------------------------------
// Identifier evaluation
// ---------------------------------------------------------------------------

func evalIdentifier(node *ast.Identifier, env *object.Environment) object.Object {
	if val, ok := env.Get(node.Name); ok {
		if lazy, ok := val.(*object.LazyValue); ok {
			return lazy.Force()
		}
		if owned, ok := val.(*object.OwnedValue); ok {
			if env != nil && owned.OwnerID != "" && owned.OwnerID != env.EnsureOwnerID() {
				return object.NewError("ownership violation: value moved to another scope")
			}
			return owned.Inner
		}
		return val
	}

	if builtin, ok := Builtins[node.Name]; ok {
		return builtin.BindEnv(env)
	}

	return object.NewError("identifier not found: %s", node.Name)
}

func evalExpressions(exps []ast.Expression, env *object.Environment) []object.Object {
	result := make([]object.Object, 0, len(exps))
	for _, e := range exps {
		if spread, ok := e.(*ast.SpreadExpression); ok {
			evaluated := Eval(spread.Value, env)
			if object.IsError(evaluated) {
				return []object.Object{evaluated}
			}
			if arr, ok := evaluated.(*object.Array); ok {
				result = append(result, arr.Elements...)
			} else {
				return []object.Object{object.NewError("spread operator requires an array, got %s", evaluated.Type())}
			}
			continue
		}
		evaluated := Eval(e, env)
		if object.IsError(evaluated) {
			return []object.Object{evaluated}
		}
		result = append(result, evaluated)
	}
	return result
}

// ---------------------------------------------------------------------------
// Index / Hash evaluation
// ---------------------------------------------------------------------------

func evalIndexExpression(left, index object.Object) object.Object {
	if imm, ok := left.(*object.ImmutableValue); ok {
		left = imm.Inner
	}
	if gen, ok := left.(*object.GeneratorValue); ok {
		left = &object.Array{Elements: gen.Elements}
	}
	switch {
	case left.Type() == object.ARRAY_OBJ && index.Type() == object.INTEGER_OBJ:
		return evalArrayIndexExpression(left, index)
	case left.Type() == object.HASH_OBJ:
		return evalHashIndexExpression(left, index)
	default:
		return object.NewError("index operator not supported: %s", left.Type())
	}
}

func evalArrayIndexExpression(array, index object.Object) object.Object {
	arrayObject := array.(*object.Array)
	idx := index.(*object.Integer).Value
	max := int64(len(arrayObject.Elements) - 1)
	if idx < 0 || idx > max {
		return object.NULL
	}
	return arrayObject.Elements[idx]
}

func evalHashLiteral(node *ast.HashLiteral, env *object.Environment) object.Object {
	pairs := make(map[object.HashKey]object.HashPair)
	for _, entry := range node.Entries {
		if entry.IsSpread {
			spreadObj := Eval(entry.Value, env)
			if object.IsError(spreadObj) {
				return spreadObj
			}
			spreadHash, ok := spreadObj.(*object.Hash)
			if !ok {
				return object.NewError("spread in object literal requires a hash, got %s", spreadObj.Type())
			}
			for k, v := range spreadHash.Pairs {
				pairs[k] = v
			}
			continue
		}
		key := Eval(entry.Key, env)
		if object.IsError(key) {
			return key
		}
		hashKey, ok := key.(object.Hashable)
		if !ok {
			return object.NewError("unusable as hash key: %s", key.Type())
		}
		value := Eval(entry.Value, env)
		if object.IsError(value) {
			return value
		}
		hashed := hashKey.HashKey()
		pairs[hashed] = object.HashPair{Key: key, Value: value}
	}
	return &object.Hash{Pairs: pairs}
}

func evalHashIndexExpression(hash, index object.Object) object.Object {
	hashObject := hash.(*object.Hash)
	key, ok := index.(object.Hashable)
	if !ok {
		return object.NewError("unusable as hash key: %s", index.Type())
	}
	pair, ok := hashObject.Pairs[key.HashKey()]
	if !ok {
		return object.NULL
	}
	return pair.Value
}

// ---------------------------------------------------------------------------
// Range expression
// ---------------------------------------------------------------------------

func evalRangeExpression(node *ast.RangeExpression, env *object.Environment) object.Object {
	left := Eval(node.Left, env)
	if object.IsError(left) {
		return left
	}
	right := Eval(node.Right, env)
	if object.IsError(right) {
		return right
	}

	switch l := left.(type) {
	case *object.Integer:
		r, ok := right.(*object.Integer)
		if !ok {
			return object.NewError("range: right side must be integer, got %s", right.Type())
		}
		low, high := l.Value, r.Value
		if low > high {
			elements := make([]object.Object, 0, low-high+1)
			for i := low; i >= high; i-- {
				elements = append(elements, &object.Integer{Value: i})
			}
			return &object.Array{Elements: elements}
		}
		elements := make([]object.Object, 0, high-low+1)
		for i := low; i <= high; i++ {
			elements = append(elements, &object.Integer{Value: i})
		}
		return &object.Array{Elements: elements}

	case *object.String:
		r, ok := right.(*object.String)
		if !ok {
			return object.NewError("range: right side must be string, got %s", right.Type())
		}
		lRunes := []rune(l.Value)
		rRunes := []rune(r.Value)
		if len(lRunes) != 1 || len(rRunes) != 1 {
			return object.NewError("range: string range requires single characters, got %q..%q", l.Value, r.Value)
		}
		low, high := lRunes[0], rRunes[0]
		if low > high {
			elements := make([]object.Object, 0, low-high+1)
			for ch := low; ch >= high; ch-- {
				elements = append(elements, &object.String{Value: string(ch)})
			}
			return &object.Array{Elements: elements}
		}
		elements := make([]object.Object, 0, high-low+1)
		for ch := low; ch <= high; ch++ {
			elements = append(elements, &object.String{Value: string(ch)})
		}
		return &object.Array{Elements: elements}

	default:
		return object.NewError("range: unsupported type %s", left.Type())
	}
}

// ---------------------------------------------------------------------------
// Destructure
// ---------------------------------------------------------------------------

func evalDestructure(pat *ast.DestructurePattern, val object.Object, env *object.Environment) object.Object {
	if pat.Kind == "object" {
		hash, ok := val.(*object.Hash)
		if !ok {
			return object.NewError("object destructuring requires a hash, got %s", val.Type())
		}
		for i, keyExpr := range pat.Keys {
			sl, ok := keyExpr.(*ast.StringLiteral)
			if !ok {
				return object.NewError("destructuring key must be a string")
			}
			key := &object.String{Value: sl.Value}
			hashed := key.HashKey()
			if pair, ok := hash.Pairs[hashed]; ok {
				env.Set(pat.Names[i].Name, pair.Value)
			} else if i < len(pat.Defaults) && pat.Defaults[i] != nil {
				def := Eval(pat.Defaults[i], env)
				if object.IsError(def) {
					return def
				}
				env.Set(pat.Names[i].Name, def)
			} else {
				env.Set(pat.Names[i].Name, object.NULL)
			}
		}
		if pat.RestName != nil {
			rest := make(map[object.HashKey]object.HashPair)
			bound := make(map[object.HashKey]bool)
			for _, keyExpr := range pat.Keys {
				sl := keyExpr.(*ast.StringLiteral)
				key := &object.String{Value: sl.Value}
				bound[key.HashKey()] = true
			}
			for k, v := range hash.Pairs {
				if !bound[k] {
					rest[k] = v
				}
			}
			env.Set(pat.RestName.Name, &object.Hash{Pairs: rest})
		}
	} else {
		arr, ok := val.(*object.Array)
		if !ok {
			return object.NewError("array destructuring requires an array, got %s", val.Type())
		}
		for i, name := range pat.Names {
			if i < len(arr.Elements) {
				env.Set(name.Name, arr.Elements[i])
			} else if i < len(pat.Defaults) && pat.Defaults[i] != nil {
				def := Eval(pat.Defaults[i], env)
				if object.IsError(def) {
					return def
				}
				env.Set(name.Name, def)
			} else {
				env.Set(name.Name, object.NULL)
			}
		}
		if pat.RestName != nil {
			startIdx := len(pat.Names)
			if startIdx < len(arr.Elements) {
				env.Set(pat.RestName.Name, &object.Array{Elements: arr.Elements[startIdx:]})
			} else {
				env.Set(pat.RestName.Name, &object.Array{Elements: []object.Object{}})
			}
		}
	}
	return object.NULL
}

// ---------------------------------------------------------------------------
// Import / Export
// ---------------------------------------------------------------------------

func evalExportStatement(node *ast.ExportStatement, env *object.Environment) object.Object {
	if env.ModuleContext == nil {
		return object.NewError("export is only allowed in module context")
	}
	if node.Declaration == nil {
		return object.NewError("invalid export declaration")
	}

	result := Eval(node.Declaration, env)
	if object.IsError(result) {
		return result
	}

	switch decl := node.Declaration.(type) {
	case *ast.LetStatement:
		if len(decl.Names) == 0 && decl.Name != nil {
			value, ok := env.Get(decl.Name.Name)
			if !ok {
				return object.NewError("missing export binding: %s", decl.Name.Name)
			}
			env.ModuleContext.Exports[decl.Name.Name] = value
			break
		}
		for _, name := range decl.Names {
			value, ok := env.Get(name.Name)
			if !ok {
				return object.NewError("missing export binding: %s", name.Name)
			}
			env.ModuleContext.Exports[name.Name] = value
		}
	case *ast.DestructureLetStatement:
		for _, name := range decl.Pattern.Names {
			value, ok := env.Get(name.Name)
			if !ok {
				return object.NewError("missing export binding: %s", name.Name)
			}
			env.ModuleContext.Exports[name.Name] = value
		}
		if decl.Pattern.RestName != nil {
			value, ok := env.Get(decl.Pattern.RestName.Name)
			if !ok {
				return object.NewError("missing export binding: %s", decl.Pattern.RestName.Name)
			}
			env.ModuleContext.Exports[decl.Pattern.RestName.Name] = value
		}
	default:
		return object.NewError("invalid export declaration type")
	}

	return object.NULL
}

func exportsToHashObject(exports map[string]object.Object) *object.Hash {
	pairs := make(map[object.HashKey]object.HashPair, len(exports))
	for name, value := range exports {
		key := &object.String{Value: name}
		pairs[key.HashKey()] = object.HashPair{Key: key, Value: value}
	}
	return &object.Hash{Pairs: pairs}
}

func evalImportStatement(node *ast.ImportStatement, env *object.Environment) object.Object {
	if env == nil {
		return object.NewError("cannot import without environment")
	}

	pathObj := Eval(node.Path, env)
	if object.IsError(pathObj) {
		return pathObj
	}

	pathStr, ok := pathObj.(*object.String)
	if !ok {
		return object.NewError("import path must be STRING, got %s", pathObj.Type())
	}

	if ResolveImportPathFn == nil {
		return object.NewError("import not supported: no path resolver configured")
	}

	resolvedPath, err := ResolveImportPathFn(pathStr.Value, env)
	if err != nil {
		return object.NewError("invalid import path %q: %s", pathStr.Value, err)
	}

	moduleLoading := env.ModuleLoadingMap()
	if moduleLoading[resolvedPath] {
		return object.NewError("module cycle detected while importing %q", pathStr.Value)
	}

	moduleCache := env.ModuleCacheMap()
	if cachedEntry, ok := moduleCache[resolvedPath]; ok {
		cachedExports := cachedEntry.Exports
		if fi, statErr := os.Stat(resolvedPath); statErr == nil {
			if fi.ModTime().UnixNano() != cachedEntry.ModTime {
				delete(moduleCache, resolvedPath)
				cachedExports = nil
			}
		}
		if cachedExports != nil {
			return bindImportedNames(node, cachedExports, pathStr.Value, env)
		}
	}

	// Evaluate module
	moduleLoading[resolvedPath] = true
	defer delete(moduleLoading, resolvedPath)

	if err := security.CheckFileReadAllowed(resolvedPath); err != nil {
		return object.NewError("module read denied for %q: %s", pathStr.Value, err)
	}
	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return object.NewError("failed to read module %q: %s", pathStr.Value, err)
	}

	moduleLexer := lexer.NewLexer(string(content))
	moduleParser := parser.NewParser(moduleLexer)
	moduleProgram := moduleParser.ParseProgram()
	if len(moduleParser.Errors()) > 0 {
		return object.NewError("module parse error in %q: %s", pathStr.Value, strings.Join(moduleParser.Errors(), "; "))
	}

	moduleEnv := object.NewEnvironment()
	moduleEnv.ModuleContext = &object.ModuleContext{Exports: map[string]object.Object{}}
	moduleEnv.ModuleCache = moduleCache
	moduleEnv.ModuleLoading = moduleLoading
	moduleEnv.RuntimeLimits = env.RuntimeLimits
	moduleEnv.SecurityPolicy = env.SecurityPolicy
	moduleEnv.Output = env.Output
	moduleEnv.ModuleDir = filepath.Dir(resolvedPath)
	moduleEnv.SourcePath = resolvedPath
	moduleEnv.CallStack = cloneCallStack(env.CallStack)

	moduleResult := Eval(moduleProgram, moduleEnv)
	if object.IsError(moduleResult) {
		if errObj, ok := moduleResult.(*object.Error); ok {
			cloned := *errObj
			cloned.Message = fmt.Sprintf("module runtime error in %q: %s", pathStr.Value, errObj.Message)
			if cloned.Path == "" {
				cloned.Path = resolvedPath
			}
			return &cloned
		}
		return object.NewError("module runtime error in %q: %s", pathStr.Value, objectErrorString(moduleResult))
	}

	if len(moduleEnv.ModuleContext.Exports) == 0 {
		return object.NewError("module %q has no exports", pathStr.Value)
	}

	exports := make(map[string]object.Object, len(moduleEnv.ModuleContext.Exports))
	for name, value := range moduleEnv.ModuleContext.Exports {
		exports[name] = value
	}
	modTime := int64(0)
	if fi, statErr := os.Stat(resolvedPath); statErr == nil {
		modTime = fi.ModTime().UnixNano()
	}
	moduleCache[resolvedPath] = object.ModuleCacheEntry{Exports: exports, ModTime: modTime}

	return bindImportedNames(node, exports, pathStr.Value, env)
}

func bindImportedNames(node *ast.ImportStatement, exports map[string]object.Object, pathStr string, env *object.Environment) object.Object {
	if len(node.Names) > 0 {
		selected := make(map[string]object.Object, len(node.Names))
		for _, ident := range node.Names {
			val, exists := exports[ident.Name]
			if !exists {
				return object.NewError("module %q does not export %q", pathStr, ident.Name)
			}
			selected[ident.Name] = val
		}
		if node.Alias != nil {
			env.Set(node.Alias.Name, exportsToHashObject(selected))
			return object.NULL
		}
		for name, value := range selected {
			env.Set(name, value)
		}
		return object.NULL
	}

	if node.Alias != nil {
		env.Set(node.Alias.Name, exportsToHashObject(exports))
		return object.NULL
	}

	for name, value := range exports {
		env.Set(name, value)
	}
	return object.NULL
}
