package eval

import (
	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/object"
)

type fastIntLoop struct {
	name      string
	start     ast.Expression
	condition *ast.InfixExpression
	postOp    string
	postStep  ast.Expression
	body      []*ast.AssignExpression
}

func evalFastForStatement(fs *ast.ForStatement, env *object.Environment) (object.Object, bool) {
	loop, ok := analyzeFastIntLoop(fs)
	if !ok {
		return nil, false
	}
	if result, ok := evalFastAccumulatorLoop(loop, env); ok {
		return result, true
	}

	locals := make(map[string]int64, len(loop.body)+1)
	dirty := make(map[string]struct{}, len(loop.body)+1)

	start, ok, errObj := evalFastIntExpression(loop.start, locals, env)
	if errObj != nil {
		return errObj, true
	}
	if !ok {
		return nil, false
	}
	locals[loop.name] = start
	dirty[loop.name] = struct{}{}

	for _, assignment := range loop.body {
		_, ok, errObj := evalFastIntExpression(assignment.Value, locals, env)
		if errObj != nil {
			return errObj, true
		}
		if !ok {
			return nil, false
		}
	}
	_, ok, errObj = evalFastIntExpression(loop.condition.Right, locals, env)
	if errObj != nil {
		return errObj, true
	}
	if !ok {
		return nil, false
	}
	_, ok, errObj = evalFastIntExpression(loop.postStep, locals, env)
	if errObj != nil {
		return errObj, true
	}
	if !ok {
		return nil, false
	}

	var result object.Object = object.NULL
	for {
		pass, ok, errObj := evalFastIntCondition(loop.condition, locals, env)
		if errObj != nil {
			return errObj, true
		}
		if !ok {
			return nil, false
		}
		if !pass {
			break
		}

		for _, assignment := range loop.body {
			value, ok, errObj := evalFastIntExpression(assignment.Value, locals, env)
			if errObj != nil {
				return errObj, true
			}
			if !ok {
				return nil, false
			}
			target := assignment.Target.(*ast.Identifier).Name
			locals[target] = value
			dirty[target] = struct{}{}
			result = object.IntegerObj(value)
		}

		step, ok, errObj := evalFastIntExpression(loop.postStep, locals, env)
		if errObj != nil {
			return errObj, true
		}
		if !ok {
			return nil, false
		}
		current := locals[loop.name]
		if loop.postOp == "+" {
			locals[loop.name] = current + step
		} else {
			locals[loop.name] = current - step
		}
		dirty[loop.name] = struct{}{}
	}

	for name := range dirty {
		val := object.IntegerObj(locals[name])
		if name == loop.name {
			env.Set(name, val)
			continue
		}
		if _, ok := env.Assign(name, val); !ok {
			return object.NewError("variable %s not declared", name), true
		}
	}
	return result, true
}

func evalFastAccumulatorLoop(loop *fastIntLoop, env *object.Environment) (object.Object, bool) {
	if len(loop.body) != 1 {
		return nil, false
	}
	assignment := loop.body[0]
	target := assignment.Target.(*ast.Identifier).Name
	if target == loop.name {
		return nil, false
	}
	if fastIntExprReferencesIdentifier(loop.condition.Right, target) || fastIntExprReferencesIdentifier(loop.postStep, target) {
		return nil, false
	}
	value, ok := assignment.Value.(*ast.InfixExpression)
	if !ok {
		return nil, false
	}

	accumulateTargetOnLeft := false
	switch left := value.Left.(type) {
	case *ast.Identifier:
		accumulateTargetOnLeft = left.Name == target
	default:
		return nil, false
	}
	rightIdent, ok := value.Right.(*ast.Identifier)
	if !ok || rightIdent.Name != loop.name || !accumulateTargetOnLeft {
		return nil, false
	}
	if value.Operator != "+" && value.Operator != "-" {
		return nil, false
	}

	locals := make(map[string]int64, 2)
	start, ok, errObj := evalFastIntExpression(loop.start, locals, env)
	if errObj != nil {
		return errObj, true
	}
	if !ok {
		return nil, false
	}

	targetObj, exists := env.Get(target)
	if !exists {
		return object.NewError("identifier not found: %s", target), true
	}
	targetInt, ok := targetObj.(*object.Integer)
	if !ok {
		return nil, false
	}

	locals[loop.name] = start
	locals[target] = targetInt.Value
	limit, ok, errObj := evalFastIntExpression(loop.condition.Right, locals, env)
	if errObj != nil {
		return errObj, true
	}
	if !ok {
		return nil, false
	}
	step, ok, errObj := evalFastIntExpression(loop.postStep, locals, env)
	if errObj != nil {
		return errObj, true
	}
	if !ok || step <= 0 {
		return nil, false
	}

	i := start
	acc := targetInt.Value
	for compareFastInts(i, limit, loop.condition.Operator) {
		if value.Operator == "+" {
			acc += i
		} else {
			acc -= i
		}
		if loop.postOp == "+" {
			i += step
		} else {
			i -= step
		}
	}

	accObj := object.IntegerObj(acc)
	if _, ok := env.Assign(target, accObj); !ok {
		return object.NewError("variable %s not declared", target), true
	}
	env.Set(loop.name, object.IntegerObj(i))
	return accObj, true
}

func fastIntExprReferencesIdentifier(expr ast.Expression, name string) bool {
	switch expr := expr.(type) {
	case *ast.Identifier:
		return expr.Name == name
	case *ast.InfixExpression:
		return fastIntExprReferencesIdentifier(expr.Left, name) || fastIntExprReferencesIdentifier(expr.Right, name)
	default:
		return false
	}
}

func compareFastInts(left, right int64, operator string) bool {
	switch operator {
	case "<":
		return left < right
	case "<=":
		return left <= right
	case ">":
		return left > right
	case ">=":
		return left >= right
	default:
		return false
	}
}

func analyzeFastIntLoop(fs *ast.ForStatement) (*fastIntLoop, bool) {
	init, ok := fs.Init.(*ast.LetStatement)
	if !ok || init.Name == nil || len(init.Names) > 1 || init.TypeName != "" || init.Value == nil {
		return nil, false
	}

	condition, ok := fs.Condition.(*ast.InfixExpression)
	if !ok || !isFastIntLoopComparison(condition.Operator) {
		return nil, false
	}
	conditionLeft, ok := condition.Left.(*ast.Identifier)
	if !ok || conditionLeft.Name != init.Name.Name {
		return nil, false
	}

	postStmt, ok := fs.Post.(*ast.ExpressionStatement)
	if !ok {
		return nil, false
	}
	post, ok := postStmt.Expression.(*ast.AssignExpression)
	if !ok {
		return nil, false
	}
	postTarget, ok := post.Target.(*ast.Identifier)
	if !ok || postTarget.Name != init.Name.Name {
		return nil, false
	}
	postValue, ok := post.Value.(*ast.InfixExpression)
	if !ok || (postValue.Operator != "+" && postValue.Operator != "-") {
		return nil, false
	}
	postLeft, ok := postValue.Left.(*ast.Identifier)
	if !ok || postLeft.Name != init.Name.Name {
		return nil, false
	}

	if fs.Body == nil || len(fs.Body.Statements) == 0 {
		return nil, false
	}
	body := make([]*ast.AssignExpression, 0, len(fs.Body.Statements))
	for _, stmt := range fs.Body.Statements {
		exprStmt, ok := stmt.(*ast.ExpressionStatement)
		if !ok {
			return nil, false
		}
		assignment, ok := exprStmt.Expression.(*ast.AssignExpression)
		if !ok {
			return nil, false
		}
		if _, ok := assignment.Target.(*ast.Identifier); !ok {
			return nil, false
		}
		body = append(body, assignment)
	}

	return &fastIntLoop{
		name:      init.Name.Name,
		start:     init.Value,
		condition: condition,
		postOp:    postValue.Operator,
		postStep:  postValue.Right,
		body:      body,
	}, true
}

func isFastIntLoopComparison(operator string) bool {
	switch operator {
	case "<", "<=", ">", ">=":
		return true
	default:
		return false
	}
}

func evalFastIntCondition(condition *ast.InfixExpression, locals map[string]int64, env *object.Environment) (bool, bool, object.Object) {
	left, ok, errObj := evalFastIntExpression(condition.Left, locals, env)
	if errObj != nil || !ok {
		return false, ok, errObj
	}
	right, ok, errObj := evalFastIntExpression(condition.Right, locals, env)
	if errObj != nil || !ok {
		return false, ok, errObj
	}
	switch condition.Operator {
	case "<":
		return left < right, true, nil
	case "<=":
		return left <= right, true, nil
	case ">":
		return left > right, true, nil
	case ">=":
		return left >= right, true, nil
	default:
		return false, false, nil
	}
}

func evalFastIntExpression(expr ast.Expression, locals map[string]int64, env *object.Environment) (int64, bool, object.Object) {
	switch expr := expr.(type) {
	case *ast.IntegerLiteral:
		return expr.Value, true, nil
	case *ast.Identifier:
		if value, ok := locals[expr.Name]; ok {
			return value, true, nil
		}
		obj, ok := env.Get(expr.Name)
		if !ok {
			return 0, true, object.NewError("identifier not found: %s", expr.Name)
		}
		integer, ok := obj.(*object.Integer)
		if !ok {
			return 0, false, nil
		}
		return integer.Value, true, nil
	case *ast.InfixExpression:
		left, ok, errObj := evalFastIntExpression(expr.Left, locals, env)
		if errObj != nil || !ok {
			return 0, ok, errObj
		}
		right, ok, errObj := evalFastIntExpression(expr.Right, locals, env)
		if errObj != nil || !ok {
			return 0, ok, errObj
		}
		switch expr.Operator {
		case "+":
			return left + right, true, nil
		case "-":
			return left - right, true, nil
		case "*":
			return left * right, true, nil
		case "/":
			if right == 0 {
				return 0, true, object.NewError("division by zero")
			}
			return left / right, true, nil
		case "%":
			if right == 0 {
				return 0, true, object.NewError("division by zero")
			}
			return left % right, true, nil
		default:
			return 0, false, nil
		}
	default:
		return 0, false, nil
	}
}
