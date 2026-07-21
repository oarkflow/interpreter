package eval

import (
	"fmt"
	"sync/atomic"

	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/object"
)

var macroExpansionID atomic.Uint64

func evalMacroCall(call *ast.MacroCallExpression, env *object.Environment) object.Object {
	value, ok := env.Get(call.Name.Name)
	if !ok {
		return object.NewError("macro not found: %s", call.Name.Name)
	}
	macro, ok := value.(*object.Macro)
	if !ok {
		return object.NewError("%s is not a macro", call.Name.Name)
	}
	provided := len(call.Arguments)
	if call.Block != nil {
		provided++
	}
	if provided != len(macro.Parameters) {
		return object.NewError("macro %s expects %d arguments, got %d", macro.Name, len(macro.Parameters), provided)
	}

	expressions := make(map[string]ast.Expression, len(call.Arguments))
	blocks := make(map[string]*ast.BlockStatement, 1)
	for i, arg := range call.Arguments {
		expressions[macro.Parameters[i].Name] = arg
	}
	if call.Block != nil {
		blocks[macro.Parameters[len(macro.Parameters)-1].Name] = call.Block
	}

	params := make(map[string]struct{}, len(macro.Parameters))
	for _, param := range macro.Parameters {
		params[param.Name] = struct{}{}
	}
	locals := make(map[string]string)
	collectMacroLocals(macro.Body, params, locals)
	id := macroExpansionID.Add(1)
	for name := range locals {
		locals[name] = fmt.Sprintf("__macro_%d_%s", id, name)
	}
	expanded := expandMacroBlock(macro.Body, expressions, blocks, locals)
	return Eval(expanded, env)
}

func collectMacroLocals(block *ast.BlockStatement, params map[string]struct{}, locals map[string]string) {
	if block == nil {
		return
	}
	for _, statement := range block.Statements {
		switch node := statement.(type) {
		case *ast.LetStatement:
			if node.Name != nil {
				if _, parameter := params[node.Name.Name]; !parameter {
					locals[node.Name.Name] = ""
				}
			}
		case *ast.WhileStatement:
			collectMacroLocals(node.Body, params, locals)
		case *ast.ExpressionStatement:
			if conditional, ok := node.Expression.(*ast.IfExpression); ok {
				collectMacroLocals(conditional.Consequence, params, locals)
				collectMacroLocals(conditional.Alternative, params, locals)
			}
		}
	}
}

func expandMacroBlock(block *ast.BlockStatement, bindings map[string]ast.Expression, blocks map[string]*ast.BlockStatement, locals map[string]string) *ast.BlockStatement {
	out := &ast.BlockStatement{}
	if block == nil {
		return out
	}
	for _, statement := range block.Statements {
		if expressionStatement, ok := statement.(*ast.ExpressionStatement); ok {
			if identifier, ok := expressionStatement.Expression.(*ast.Identifier); ok {
				if inserted, exists := blocks[identifier.Name]; exists {
					out.Statements = append(out.Statements, inserted.Statements...)
					continue
				}
			}
		}
		out.Statements = append(out.Statements, expandMacroStatement(statement, bindings, blocks, locals))
	}
	return out
}

func expandMacroStatement(statement ast.Statement, bindings map[string]ast.Expression, blocks map[string]*ast.BlockStatement, locals map[string]string) ast.Statement {
	switch node := statement.(type) {
	case *ast.LetStatement:
		name := node.Name
		if replacement, exists := bindings[name.Name]; exists {
			if identifier, ok := replacement.(*ast.Identifier); ok {
				name = &ast.Identifier{Name: identifier.Name}
			}
		} else if renamed, exists := locals[name.Name]; exists {
			name = &ast.Identifier{Name: renamed}
		}
		return &ast.LetStatement{Name: name, Names: []*ast.Identifier{name}, TypeName: node.TypeName, Type: node.Type, Value: expandMacroExpression(node.Value, bindings, blocks, locals), IsConst: node.IsConst}
	case *ast.ExpressionStatement:
		return &ast.ExpressionStatement{Expression: expandMacroExpression(node.Expression, bindings, blocks, locals)}
	case *ast.PrintStatement:
		return &ast.PrintStatement{Expression: expandMacroExpression(node.Expression, bindings, blocks, locals)}
	case *ast.WhileStatement:
		return &ast.WhileStatement{Condition: expandMacroExpression(node.Condition, bindings, blocks, locals), Body: expandMacroBlock(node.Body, bindings, blocks, locals)}
	case *ast.ReturnStatement:
		return &ast.ReturnStatement{ReturnValue: expandMacroExpression(node.ReturnValue, bindings, blocks, locals)}
	case *ast.YieldStatement:
		return &ast.YieldStatement{Value: expandMacroExpression(node.Value, bindings, blocks, locals)}
	default:
		return statement
	}
}

func expandMacroExpression(expression ast.Expression, bindings map[string]ast.Expression, blocks map[string]*ast.BlockStatement, locals map[string]string) ast.Expression {
	switch node := expression.(type) {
	case nil:
		return nil
	case *ast.Identifier:
		if replacement, exists := bindings[node.Name]; exists {
			return replacement
		}
		if renamed, exists := locals[node.Name]; exists {
			return &ast.Identifier{Name: renamed}
		}
		return node
	case *ast.PrefixExpression:
		return &ast.PrefixExpression{Operator: node.Operator, Right: expandMacroExpression(node.Right, bindings, blocks, locals)}
	case *ast.PostfixExpression:
		return &ast.PostfixExpression{Operator: node.Operator, Target: expandMacroExpression(node.Target, bindings, blocks, locals)}
	case *ast.InfixExpression:
		return &ast.InfixExpression{Left: expandMacroExpression(node.Left, bindings, blocks, locals), Operator: node.Operator, Right: expandMacroExpression(node.Right, bindings, blocks, locals)}
	case *ast.AssignExpression:
		return &ast.AssignExpression{Target: expandMacroExpression(node.Target, bindings, blocks, locals), Value: expandMacroExpression(node.Value, bindings, blocks, locals)}
	case *ast.CompoundAssignExpression:
		return &ast.CompoundAssignExpression{Target: expandMacroExpression(node.Target, bindings, blocks, locals), Operator: node.Operator, Value: expandMacroExpression(node.Value, bindings, blocks, locals)}
	case *ast.IfExpression:
		return &ast.IfExpression{Condition: expandMacroExpression(node.Condition, bindings, blocks, locals), Consequence: expandMacroBlock(node.Consequence, bindings, blocks, locals), Alternative: expandMacroBlock(node.Alternative, bindings, blocks, locals)}
	case *ast.CallExpression:
		args := make([]ast.Expression, len(node.Arguments))
		for i, arg := range node.Arguments {
			args[i] = expandMacroExpression(arg, bindings, blocks, locals)
		}
		return &ast.CallExpression{Function: expandMacroExpression(node.Function, bindings, blocks, locals), Arguments: args, Line: node.Line, Column: node.Column}
	case *ast.IndexExpression:
		return &ast.IndexExpression{Left: expandMacroExpression(node.Left, bindings, blocks, locals), Index: expandMacroExpression(node.Index, bindings, blocks, locals)}
	case *ast.DotExpression:
		return &ast.DotExpression{Left: expandMacroExpression(node.Left, bindings, blocks, locals), Right: node.Right}
	case *ast.OptionalDotExpression:
		return &ast.OptionalDotExpression{Left: expandMacroExpression(node.Left, bindings, blocks, locals), Right: node.Right}
	case *ast.ArrayLiteral:
		elements := make([]ast.Expression, len(node.Elements))
		for i, element := range node.Elements {
			elements[i] = expandMacroExpression(element, bindings, blocks, locals)
		}
		return &ast.ArrayLiteral{Elements: elements}
	case *ast.HashLiteral:
		entries := make([]ast.HashEntry, len(node.Entries))
		for i, entry := range node.Entries {
			entries[i] = entry
			entries[i].Key = expandMacroExpression(entry.Key, bindings, blocks, locals)
			entries[i].Value = expandMacroExpression(entry.Value, bindings, blocks, locals)
		}
		return &ast.HashLiteral{Entries: entries}
	default:
		return expression
	}
}
