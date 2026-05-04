package eval

import (
	"math"
	"strings"

	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/object"
)

// ---------------------------------------------------------------------------
// Prefix expression evaluation
// ---------------------------------------------------------------------------

func evalPrefixExpression(operator string, right object.Object) object.Object {
	if lazy, ok := right.(*object.LazyValue); ok {
		right = lazy.Force()
	}
	if owned, ok := right.(*object.OwnedValue); ok {
		right = owned.Inner
	}
	switch operator {
	case "!":
		return evalBangOperatorExpression(right)
	case "-":
		return evalMinusPrefixOperatorExpression(right)
	case "~":
		return evalBitwiseNotExpression(right)
	case "typeof":
		return evalTypeofExpression(right)
	default:
		return object.NewError("unknown operator: %s%s", operator, right.Type())
	}
}

func evalBangOperatorExpression(right object.Object) object.Object {
	switch right {
	case object.TRUE:
		return object.FALSE
	case object.FALSE:
		return object.TRUE
	case object.NULL:
		return object.TRUE
	default:
		return object.FALSE
	}
}

func evalMinusPrefixOperatorExpression(right object.Object) object.Object {
	switch right.Type() {
	case object.INTEGER_OBJ:
		value := right.(*object.Integer).Value
		return &object.Integer{Value: -value}
	case object.FLOAT_OBJ:
		value := right.(*object.Float).Value
		return &object.Float{Value: -value}
	default:
		return object.NewError("unknown operator: -%s", right.Type())
	}
}

func evalBitwiseNotExpression(right object.Object) object.Object {
	switch v := right.(type) {
	case *object.Integer:
		return object.IntegerObj(^v.Value)
	default:
		return object.NewError("bitwise NOT not supported on %s", right.Type())
	}
}

func evalTypeofExpression(right object.Object) object.Object {
	if right == nil || right == object.NULL {
		return &object.String{Value: "null"}
	}
	switch right.(type) {
	case *object.Integer:
		return &object.String{Value: "integer"}
	case *object.Float:
		return &object.String{Value: "float"}
	case *object.String:
		return &object.String{Value: "string"}
	case *object.Boolean:
		return &object.String{Value: "boolean"}
	case *object.Array:
		return &object.String{Value: "array"}
	case *object.Hash:
		return &object.String{Value: "hash"}
	case *object.Function, *object.Builtin:
		return &object.String{Value: "function"}
	default:
		return &object.String{Value: right.Type().String()}
	}
}

// ---------------------------------------------------------------------------
// Infix expression evaluation
// ---------------------------------------------------------------------------

func evalInfixExpression(operator string, left, right object.Object) object.Object {
	if lazy, ok := left.(*object.LazyValue); ok {
		left = lazy.Force()
	}
	if lazy, ok := right.(*object.LazyValue); ok {
		right = lazy.Force()
	}
	if owned, ok := left.(*object.OwnedValue); ok {
		left = owned.Inner
	}
	if owned, ok := right.(*object.OwnedValue); ok {
		right = owned.Inner
	}
	if operator == "&&" {
		if !object.IsTruthy(left) {
			return object.FALSE
		}
		return object.NativeBoolToBooleanObject(object.IsTruthy(right))
	}
	if operator == "||" {
		if object.IsTruthy(left) {
			return object.TRUE
		}
		return object.NativeBoolToBooleanObject(object.IsTruthy(right))
	}
	if operator == "??" {
		if left == object.NULL || left == nil {
			return right
		}
		return left
	}

	switch {
	case left.Type() == object.INTEGER_OBJ && right.Type() == object.INTEGER_OBJ:
		return evalIntegerInfixExpression(operator, left, right)
	case left.Type() == object.FLOAT_OBJ && right.Type() == object.FLOAT_OBJ:
		return evalFloatInfixExpression(operator, left, right)
	case left.Type() == object.INTEGER_OBJ && right.Type() == object.FLOAT_OBJ:
		return evalFloatInfixExpression(operator, &object.Float{Value: float64(left.(*object.Integer).Value)}, right)
	case left.Type() == object.FLOAT_OBJ && right.Type() == object.INTEGER_OBJ:
		return evalFloatInfixExpression(operator, left, &object.Float{Value: float64(right.(*object.Integer).Value)})
	case left.Type() == object.STRING_OBJ && right.Type() == object.STRING_OBJ:
		return evalStringInfixExpression(operator, left, right)
	case operator == "==":
		return object.NativeBoolToBooleanObject(left == right)
	case operator == "!=":
		return object.NativeBoolToBooleanObject(left != right)
	case operator == "+" && (left.Type() == object.STRING_OBJ || right.Type() == object.STRING_OBJ):
		return evalMixedStringConcatenation(left, right)
	case left.Type() != right.Type():
		return object.NewError("type mismatch: %s %s %s", left.Type(), operator, right.Type())
	default:
		return object.NewError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func evalFloatInfixExpression(operator string, left, right object.Object) object.Object {
	leftVal := left.(*object.Float).Value
	rightVal := right.(*object.Float).Value

	switch operator {
	case "+":
		return &object.Float{Value: leftVal + rightVal}
	case "-":
		return &object.Float{Value: leftVal - rightVal}
	case "*":
		return &object.Float{Value: leftVal * rightVal}
	case "/":
		if rightVal == 0 {
			return object.NewError("division by zero")
		}
		return &object.Float{Value: leftVal / rightVal}
	case "<":
		return object.NativeBoolToBooleanObject(leftVal < rightVal)
	case ">":
		return object.NativeBoolToBooleanObject(leftVal > rightVal)
	case "<=":
		return object.NativeBoolToBooleanObject(leftVal <= rightVal)
	case ">=":
		return object.NativeBoolToBooleanObject(leftVal >= rightVal)
	case "==":
		return object.NativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return object.NativeBoolToBooleanObject(leftVal != rightVal)
	case "**":
		return &object.Float{Value: math.Pow(leftVal, rightVal)}
	default:
		return object.NewError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func evalIntegerInfixExpression(operator string, left, right object.Object) object.Object {
	leftVal := left.(*object.Integer).Value
	rightVal := right.(*object.Integer).Value

	switch operator {
	case "+":
		return object.IntegerObj(leftVal + rightVal)
	case "-":
		return object.IntegerObj(leftVal - rightVal)
	case "*":
		return object.IntegerObj(leftVal * rightVal)
	case "/":
		if rightVal == 0 {
			return object.NewError("division by zero")
		}
		return object.IntegerObj(leftVal / rightVal)
	case "%":
		if rightVal == 0 {
			return object.NewError("division by zero")
		}
		return object.IntegerObj(leftVal % rightVal)
	case "<":
		return object.NativeBoolToBooleanObject(leftVal < rightVal)
	case ">":
		return object.NativeBoolToBooleanObject(leftVal > rightVal)
	case "<=":
		return object.NativeBoolToBooleanObject(leftVal <= rightVal)
	case ">=":
		return object.NativeBoolToBooleanObject(leftVal >= rightVal)
	case "==":
		return object.NativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return object.NativeBoolToBooleanObject(leftVal != rightVal)
	case "&":
		return object.IntegerObj(leftVal & rightVal)
	case "|":
		return object.IntegerObj(leftVal | rightVal)
	case "^":
		return object.IntegerObj(leftVal ^ rightVal)
	case "<<":
		return object.IntegerObj(leftVal << uint(rightVal))
	case ">>":
		return object.IntegerObj(leftVal >> uint(rightVal))
	case "**":
		return object.IntegerObj(int64(math.Pow(float64(leftVal), float64(rightVal))))
	default:
		return object.NewError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func evalStringInfixExpression(operator string, left, right object.Object) object.Object {
	leftVal := left.(*object.String).Value
	rightVal := right.(*object.String).Value

	switch operator {
	case "+":
		return &object.String{Value: leftVal + rightVal}
	case "==":
		return object.NativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return object.NativeBoolToBooleanObject(leftVal != rightVal)
	default:
		return object.NewError("unknown operator: %s %s %s", left.Type(), operator, right.Type())
	}
}

func evalMixedStringConcatenation(left, right object.Object) object.Object {
	return &object.String{Value: left.Inspect() + right.Inspect()}
}

// ---------------------------------------------------------------------------
// Shared helpers used across files
// ---------------------------------------------------------------------------

func objectsEqual(a, b object.Object) bool {
	if a.Type() != b.Type() {
		return false
	}
	switch av := a.(type) {
	case *object.Integer:
		return av.Value == b.(*object.Integer).Value
	case *object.Float:
		return av.Value == b.(*object.Float).Value
	case *object.String:
		return av.Value == b.(*object.String).Value
	case *object.Boolean:
		return av.Value == b.(*object.Boolean).Value
	case *object.Null:
		return true
	default:
		return a == b
	}
}

func objectCompare(a, b object.Object) (int, bool) {
	if a.Type() != b.Type() {
		af, aOk := toFloat(a)
		bf, bOk := toFloat(b)
		if aOk && bOk {
			if af < bf {
				return -1, true
			} else if af > bf {
				return 1, true
			}
			return 0, true
		}
		return 0, false
	}
	switch av := a.(type) {
	case *object.Integer:
		bv := b.(*object.Integer).Value
		if av.Value < bv {
			return -1, true
		} else if av.Value > bv {
			return 1, true
		}
		return 0, true
	case *object.Float:
		bv := b.(*object.Float).Value
		if av.Value < bv {
			return -1, true
		} else if av.Value > bv {
			return 1, true
		}
		return 0, true
	case *object.String:
		bv := b.(*object.String).Value
		if av.Value < bv {
			return -1, true
		} else if av.Value > bv {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func toFloat(obj object.Object) (float64, bool) {
	switch v := obj.(type) {
	case *object.Integer:
		return float64(v.Value), true
	case *object.Float:
		return v.Value, true
	}
	return 0, false
}

func valueMatchesTypeName(val object.Object, typeName string) bool {
	t := strings.ToLower(strings.TrimSpace(typeName))
	switch t {
	case "any", "object":
		return true
	case "int", "integer":
		return val != nil && val.Type() == object.INTEGER_OBJ
	case "float", "number":
		return val != nil && (val.Type() == object.FLOAT_OBJ || val.Type() == object.INTEGER_OBJ)
	case "bool", "boolean":
		return val != nil && val.Type() == object.BOOLEAN_OBJ
	case "string":
		return val != nil && val.Type() == object.STRING_OBJ
	case "array", "list":
		return val != nil && val.Type() == object.ARRAY_OBJ
	case "hash", "map":
		return val != nil && val.Type() == object.HASH_OBJ
	case "null", "nil":
		return val == nil || val == object.NULL || val.Type() == object.NULL_OBJ
	case "function", "fn":
		if val == nil {
			return false
		}
		return val.Type() == object.FUNCTION_OBJ || val.Type() == object.BUILTIN_OBJ
	default:
		return true
	}
}

func maybeWrapOwned(val object.Object, env *object.Environment) object.Object {
	if env == nil || val == nil {
		return val
	}
	switch v := val.(type) {
	case *object.OwnedValue:
		v.OwnerID = env.EnsureOwnerID()
		return v
	case *object.Array, *object.Hash, *object.ADTValue:
		return &object.OwnedValue{OwnerID: env.EnsureOwnerID(), Inner: val}
	default:
		return val
	}
}

func objectErrorString(obj object.Object) string {
	if obj == nil {
		return ""
	}
	if h, ok := obj.(*object.Hash); ok {
		if pair, ok := h.Pairs[(&object.String{Value: "message"}).HashKey()]; ok {
			return objectToDisplayString(pair.Value)
		}
	}
	if errObj, ok := obj.(*object.Error); ok {
		return errObj.Message
	}
	if owned, ok := obj.(*object.OwnedValue); ok {
		return objectErrorString(owned.Inner)
	}
	if strObj, ok := obj.(*object.String); ok && strings.HasPrefix(strObj.Value, "ERROR:") {
		return strings.TrimPrefix(strObj.Value, "ERROR: ")
	}
	return obj.Inspect()
}

func objectToDisplayString(obj object.Object) string {
	if obj == nil {
		return "null"
	}
	if s, ok := obj.(*object.String); ok {
		return s.Value
	}
	return obj.Inspect()
}

func errorObjectToHash(obj object.Object) object.Object {
	errObj, ok := obj.(*object.Error)
	if !ok || errObj == nil {
		return &object.Hash{Pairs: map[object.HashKey]object.HashPair{}}
	}
	pairs := make(map[object.HashKey]object.HashPair, 5)
	put := func(key string, value object.Object) {
		k := &object.String{Value: key}
		pairs[k.HashKey()] = object.HashPair{Key: k, Value: value}
	}
	put("name", &object.String{Value: "Error"})
	put("message", &object.String{Value: errObj.Message})
	code := strings.TrimSpace(errObj.Code)
	if code == "" {
		code = "E_RUNTIME"
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(errObj.Message)), "identifier not found") {
			code = "E_NAME"
		}
	}
	put("code", &object.String{Value: code})
	if trace := object.FormatCallStack(errObj.Stack); trace != "" {
		put("stack", &object.String{Value: trace})
	} else {
		put("stack", &object.String{Value: ""})
	}
	if errObj.Path != "" {
		put("path", &object.String{Value: errObj.Path})
	}
	if errObj.Line > 0 {
		put("line", object.IntegerObj(int64(errObj.Line)))
	}
	if errObj.Column > 0 {
		put("column", object.IntegerObj(int64(errObj.Column)))
	}
	return &object.Hash{Pairs: pairs}
}

func cloneCallStack(stack []object.CallFrame) []object.CallFrame {
	if len(stack) == 0 {
		return nil
	}
	return append([]object.CallFrame(nil), stack...)
}

func callFrameFromExpression(function ast.Expression, env *object.Environment, line, column int) object.CallFrame {
	frame := object.CallFrame{
		Function: strings.TrimSpace(function.String()),
		Line:     line,
		Column:   column,
	}
	if env != nil {
		frame.Path = env.SourcePath
	}
	return frame
}

// EvalPrefixExpression is the exported wrapper for evalPrefixExpression,
// used by the bytecode VM.
func EvalPrefixExpression(operator string, right object.Object) object.Object {
	return evalPrefixExpression(operator, right)
}

// EvalInfixExpression is the exported wrapper for evalInfixExpression,
// used by the bytecode VM.
func EvalInfixExpression(operator string, left, right object.Object) object.Object {
	return evalInfixExpression(operator, left, right)
}
