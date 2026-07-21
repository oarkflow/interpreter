package eval

import (
	"math"

	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/object"
)

type fastNumber struct {
	integer bool
	i       int64
	f       float64
}

// evalFastSealedRule evaluates a pure boolean rule against sealed bindings
// without intermediate Object values or per-identifier lock/mode checks.
func evalFastSealedRule(expr ast.Expression, env *object.Environment) (bool, bool) {
	if env == nil || !env.BindingsSealed() || env.RuntimeLimits != nil {
		return false, false
	}
	return fastSealedBooleanValue(expr, env)
}

func fastSealedBooleanValue(expr ast.Expression, env *object.Environment) (bool, bool) {
	node, ok := expr.(*ast.InfixExpression)
	if !ok {
		return false, false
	}
	switch node.Operator {
	case "&&":
		left, ok := fastSealedBooleanValue(node.Left, env)
		if !ok {
			return false, false
		}
		if !left {
			return false, true
		}
		return fastSealedBooleanValue(node.Right, env)
	case "||":
		left, ok := fastSealedBooleanValue(node.Left, env)
		if !ok {
			return false, false
		}
		if left {
			return true, true
		}
		return fastSealedBooleanValue(node.Right, env)
	case "in", "not in":
		return fastSealedLiteralMembership(node, env)
	case "<", ">", "<=", ">=", "==", "!=":
		left, ok := fastSealedNumericValue(node.Left, env)
		if !ok {
			return false, false
		}
		right, ok := fastSealedNumericValue(node.Right, env)
		if !ok {
			return false, false
		}
		if left.integer && right.integer {
			switch node.Operator {
			case "<":
				return left.i < right.i, true
			case ">":
				return left.i > right.i, true
			case "<=":
				return left.i <= right.i, true
			case ">=":
				return left.i >= right.i, true
			case "==":
				return left.i == right.i, true
			case "!=":
				return left.i != right.i, true
			}
		}
		l, r := left.float64(), right.float64()
		switch node.Operator {
		case "<":
			return l < r, true
		case ">":
			return l > r, true
		case "<=":
			return l <= r, true
		case ">=":
			return l >= r, true
		case "==":
			return l == r, true
		case "!=":
			return l != r, true
		}
	}
	return false, false
}

func fastSealedNumericValue(expr ast.Expression, env *object.Environment) (fastNumber, bool) {
	switch node := expr.(type) {
	case *ast.IntegerLiteral:
		return fastNumber{integer: true, i: node.Value}, true
	case *ast.FloatLiteral:
		return fastNumber{f: node.Value}, true
	case *ast.Identifier:
		value, ok := env.Store[node.Name]
		if !ok {
			if env.Outer == nil {
				return fastNumber{}, false
			}
			value, ok = env.Outer.Get(node.Name)
			if !ok {
				return fastNumber{}, false
			}
		}
		switch number := value.(type) {
		case *object.Integer:
			return fastNumber{integer: true, i: number.Value}, true
		case *object.Float:
			return fastNumber{f: number.Value}, true
		default:
			return fastNumber{}, false
		}
	case *ast.PrefixExpression:
		if node.Operator != "-" {
			return fastNumber{}, false
		}
		value, ok := fastSealedNumericValue(node.Right, env)
		if !ok {
			return fastNumber{}, false
		}
		if value.integer {
			value.i = -value.i
		} else {
			value.f = -value.f
		}
		return value, true
	case *ast.InfixExpression:
		left, ok := fastSealedNumericValue(node.Left, env)
		if !ok {
			return fastNumber{}, false
		}
		right, ok := fastSealedNumericValue(node.Right, env)
		if !ok {
			return fastNumber{}, false
		}
		result, handled, errObj := applyFastNumericOperator(node.Operator, left, right)
		return result, handled && errObj == nil
	default:
		return fastNumber{}, false
	}
}

func fastSealedLiteralMembership(node *ast.InfixExpression, env *object.Environment) (bool, bool) {
	array, ok := node.Right.(*ast.ArrayLiteral)
	if !ok {
		return false, false
	}
	identifier, ok := node.Left.(*ast.Identifier)
	if !ok {
		return false, false
	}
	value, ok := env.Store[identifier.Name]
	if !ok {
		if env.Outer == nil {
			return false, false
		}
		value, ok = env.Outer.Get(identifier.Name)
		if !ok {
			return false, false
		}
	}
	value = unwrapComparable(value)
	found := false
	for _, element := range array.Elements {
		switch element.(type) {
		case *ast.IntegerLiteral, *ast.FloatLiteral, *ast.StringLiteral, *ast.BooleanLiteral, *ast.NullLiteral:
		default:
			return false, false
		}
		if literalMatchesObject(element, value) {
			found = true
			break
		}
	}
	if node.Operator == "not in" {
		found = !found
	}
	return found, true
}

func (n fastNumber) float64() float64 {
	if n.integer {
		return float64(n.i)
	}
	return n.f
}

// evalFastNumericInfix fuses a side-effect-free numeric expression tree and
// boxes only its final result. Runtime-limited executions retain the ordinary
// per-node evaluator path so instruction/depth accounting is unchanged.
func evalFastNumericInfix(node *ast.InfixExpression, env *object.Environment) (object.Object, bool) {
	if node == nil || (env != nil && env.RuntimeLimits != nil) {
		return nil, false
	}
	left, ok := fastNumericValue(node.Left, env)
	if !ok {
		return nil, false
	}
	right, ok := fastNumericValue(node.Right, env)
	if !ok {
		return nil, false
	}

	if left.integer && right.integer {
		switch node.Operator {
		case "<":
			return object.NativeBoolToBooleanObject(left.i < right.i), true
		case ">":
			return object.NativeBoolToBooleanObject(left.i > right.i), true
		case "<=":
			return object.NativeBoolToBooleanObject(left.i <= right.i), true
		case ">=":
			return object.NativeBoolToBooleanObject(left.i >= right.i), true
		case "==":
			return object.NativeBoolToBooleanObject(left.i == right.i), true
		case "!=":
			return object.NativeBoolToBooleanObject(left.i != right.i), true
		}
	} else {
		switch node.Operator {
		case "<":
			return object.NativeBoolToBooleanObject(left.float64() < right.float64()), true
		case ">":
			return object.NativeBoolToBooleanObject(left.float64() > right.float64()), true
		case "<=":
			return object.NativeBoolToBooleanObject(left.float64() <= right.float64()), true
		case ">=":
			return object.NativeBoolToBooleanObject(left.float64() >= right.float64()), true
		case "==":
			return object.NativeBoolToBooleanObject(left.float64() == right.float64()), true
		case "!=":
			return object.NativeBoolToBooleanObject(left.float64() != right.float64()), true
		}
	}

	result, handled, errObj := applyFastNumericOperator(node.Operator, left, right)
	if errObj != nil {
		return errObj, true
	}
	if !handled {
		return nil, false
	}
	if result.integer {
		return object.IntegerObj(result.i), true
	}
	return &object.Float{Value: result.f}, true
}

func fastNumericValue(expr ast.Expression, env *object.Environment) (fastNumber, bool) {
	switch node := expr.(type) {
	case *ast.IntegerLiteral:
		return fastNumber{integer: true, i: node.Value}, true
	case *ast.FloatLiteral:
		return fastNumber{f: node.Value}, true
	case *ast.Identifier:
		if env == nil {
			return fastNumber{}, false
		}
		value, ok := env.Get(node.Name)
		if !ok {
			return fastNumber{}, false
		}
		for {
			switch wrapped := value.(type) {
			case *object.OwnedValue:
				value = wrapped.Inner
			case *object.ImmutableValue:
				value = wrapped.Inner
			default:
				goto unwrapped
			}
		}
	unwrapped:
		switch number := value.(type) {
		case *object.Integer:
			return fastNumber{integer: true, i: number.Value}, true
		case *object.Float:
			return fastNumber{f: number.Value}, true
		default:
			return fastNumber{}, false
		}
	case *ast.PrefixExpression:
		if node.Operator != "-" {
			return fastNumber{}, false
		}
		value, ok := fastNumericValue(node.Right, env)
		if !ok {
			return fastNumber{}, false
		}
		if value.integer {
			value.i = -value.i
		} else {
			value.f = -value.f
		}
		return value, true
	case *ast.InfixExpression:
		left, ok := fastNumericValue(node.Left, env)
		if !ok {
			return fastNumber{}, false
		}
		right, ok := fastNumericValue(node.Right, env)
		if !ok {
			return fastNumber{}, false
		}
		result, handled, errObj := applyFastNumericOperator(node.Operator, left, right)
		return result, handled && errObj == nil
	default:
		return fastNumber{}, false
	}
}

func applyFastNumericOperator(operator string, left, right fastNumber) (fastNumber, bool, object.Object) {
	if left.integer && right.integer {
		switch operator {
		case "+":
			return fastNumber{integer: true, i: left.i + right.i}, true, nil
		case "-":
			return fastNumber{integer: true, i: left.i - right.i}, true, nil
		case "*":
			return fastNumber{integer: true, i: left.i * right.i}, true, nil
		case "/":
			if right.i == 0 {
				return fastNumber{}, true, object.NewError("division by zero")
			}
			return fastNumber{integer: true, i: left.i / right.i}, true, nil
		case "%":
			if right.i == 0 {
				return fastNumber{}, true, object.NewError("division by zero")
			}
			return fastNumber{integer: true, i: left.i % right.i}, true, nil
		case "**":
			return fastNumber{integer: true, i: int64(math.Pow(float64(left.i), float64(right.i)))}, true, nil
		}
		return fastNumber{}, false, nil
	}

	l, r := left.float64(), right.float64()
	switch operator {
	case "+":
		return fastNumber{f: l + r}, true, nil
	case "-":
		return fastNumber{f: l - r}, true, nil
	case "*":
		return fastNumber{f: l * r}, true, nil
	case "/":
		if r == 0 {
			return fastNumber{}, true, object.NewError("division by zero")
		}
		return fastNumber{f: l / r}, true, nil
	case "**":
		return fastNumber{f: math.Pow(l, r)}, true, nil
	default:
		return fastNumber{}, false, nil
	}
}

// evalFastLiteralMembership avoids constructing a temporary array and boxed
// literals for `value in [constant, ...]` expressions.
func evalFastLiteralMembership(node *ast.InfixExpression, env *object.Environment) (object.Object, bool) {
	if node == nil || (node.Operator != "in" && node.Operator != "not in") || (env != nil && env.RuntimeLimits != nil) {
		return nil, false
	}
	array, ok := node.Right.(*ast.ArrayLiteral)
	if !ok {
		return nil, false
	}
	for _, element := range array.Elements {
		switch element.(type) {
		case *ast.IntegerLiteral, *ast.FloatLiteral, *ast.StringLiteral, *ast.BooleanLiteral, *ast.NullLiteral:
		default:
			return nil, false
		}
	}
	left := Eval(node.Left, env)
	if object.IsError(left) {
		return left, true
	}
	left = unwrapComparable(left)
	found := false
	for _, element := range array.Elements {
		if literalMatchesObject(element, left) {
			found = true
			break
		}
	}
	if node.Operator == "not in" {
		found = !found
	}
	return object.NativeBoolToBooleanObject(found), true
}

func literalMatchesObject(literal ast.Expression, value object.Object) bool {
	switch node := literal.(type) {
	case *ast.IntegerLiteral:
		switch actual := value.(type) {
		case *object.Integer:
			return node.Value == actual.Value
		case *object.Float:
			return float64(node.Value) == actual.Value
		}
	case *ast.FloatLiteral:
		switch actual := value.(type) {
		case *object.Integer:
			return node.Value == float64(actual.Value)
		case *object.Float:
			return node.Value == actual.Value
		}
	case *ast.StringLiteral:
		actual, ok := value.(*object.String)
		return ok && node.Value == actual.Value
	case *ast.BooleanLiteral:
		actual, ok := value.(*object.Boolean)
		return ok && node.Value == actual.Value
	case *ast.NullLiteral:
		return value == nil || value == object.NULL
	}
	return false
}
