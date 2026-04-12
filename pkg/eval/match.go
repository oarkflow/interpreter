package eval

import (
	"regexp"
	"strings"

	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/token"
)

// ---------------------------------------------------------------------------
// Match expression evaluation
// ---------------------------------------------------------------------------

func evalMatchExpression(node *ast.MatchExpression, env *object.Environment) object.Object {
	val := Eval(node.Value, env)
	if object.IsError(val) {
		return val
	}
	if adt, ok := val.(*object.ADTValue); ok {
		if err := ensureExhaustiveMatchADT(node, adt); err != nil {
			return err
		}
	}

	for _, mc := range node.Cases {
		matchEnv := object.NewEnclosedEnvironment(env)
		if MatchPattern(mc.Pattern, val, matchEnv) {
			if mc.Guard != nil {
				guardResult := Eval(mc.Guard, matchEnv)
				if object.IsError(guardResult) {
					return guardResult
				}
				if !object.IsTruthy(guardResult) {
					continue
				}
			}
			result := Eval(mc.Body, matchEnv)
			if result == nil {
				return object.NULL
			}
			if result.Type() == object.BREAK_OBJ {
				return object.NULL
			}
			return result
		}
	}
	return object.NULL
}

func ensureExhaustiveMatchADT(node *ast.MatchExpression, val *object.ADTValue) object.Object {
	if node == nil || val == nil || len(val.AllVariants) == 0 {
		return nil
	}
	covered := make(map[string]bool, len(val.AllVariants))
	hasWildcard := false
	for _, c := range node.Cases {
		switch p := c.Pattern.(type) {
		case *ast.WildcardPattern:
			hasWildcard = true
		case *ast.BindingPattern:
			hasWildcard = true
		case *ast.OrPattern:
			for _, sub := range p.Patterns {
				if cp, ok := sub.(*ast.ConstructorPattern); ok {
					covered[cp.Name] = true
				}
			}
		case *ast.ConstructorPattern:
			covered[p.Name] = true
		}
	}
	if hasWildcard {
		return nil
	}
	missing := make([]string, 0)
	for _, v := range val.AllVariants {
		if !covered[v] {
			missing = append(missing, v)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return object.NewError("non-exhaustive ADT match for %s: missing %s", val.TypeName, strings.Join(missing, ", "))
}

// ---------------------------------------------------------------------------
// MatchPattern – exported for external use
// ---------------------------------------------------------------------------

// MatchPattern checks whether a value matches a pattern, binding variables
// into env on success.
func MatchPattern(pattern ast.Pattern, value object.Object, env *object.Environment) bool {
	switch p := pattern.(type) {
	case *ast.LiteralPattern:
		litVal := Eval(p.Value, env)
		if object.IsError(litVal) {
			return false
		}
		return objectsEqual(value, litVal)

	case *ast.WildcardPattern:
		return true

	case *ast.BindingPattern:
		if p.TypeName != "" {
			if !typeMatches(value, p.TypeName) {
				return false
			}
		}
		if p.Name != nil && p.Name.Name != "_" {
			env.Set(p.Name.Name, value)
		}
		return true

	case *ast.ArrayPattern:
		if owned, ok := value.(*object.OwnedValue); ok {
			value = owned.Inner
		}
		arr, ok := value.(*object.Array)
		if !ok {
			return false
		}
		if p.Rest == nil {
			if len(arr.Elements) != len(p.Elements) {
				return false
			}
		} else {
			if len(arr.Elements) < len(p.Elements) {
				return false
			}
		}
		for i, elemPat := range p.Elements {
			if !MatchPattern(elemPat, arr.Elements[i], env) {
				return false
			}
		}
		if p.Rest != nil {
			restElems := arr.Elements[len(p.Elements):]
			env.Set(p.Rest.Name, &object.Array{Elements: restElems})
		}
		return true

	case *ast.ObjectPattern:
		if owned, ok := value.(*object.OwnedValue); ok {
			value = owned.Inner
		}
		hash, ok := value.(*object.Hash)
		if !ok {
			return false
		}
		for i, key := range p.Keys {
			strKey := &object.String{Value: key}
			hk := strKey.HashKey()
			pair, exists := hash.Pairs[hk]
			if !exists {
				return false
			}
			if !MatchPattern(p.Patterns[i], pair.Value, env) {
				return false
			}
		}
		if p.Rest != nil {
			rest := make(map[object.HashKey]object.HashPair)
			bound := make(map[string]bool, len(p.Keys))
			for _, key := range p.Keys {
				bound[key] = true
			}
			for hk, pair := range hash.Pairs {
				if strKey, ok := pair.Key.(*object.String); ok {
					if !bound[strKey.Value] {
						rest[hk] = pair
					}
				}
			}
			env.Set(p.Rest.Name, &object.Hash{Pairs: rest})
		}
		return true

	case *ast.OrPattern:
		for _, subPat := range p.Patterns {
			tempEnv := object.NewEnclosedEnvironment(env)
			if MatchPattern(subPat, value, tempEnv) {
				for k, v := range tempEnv.Store {
					env.Set(k, v)
				}
				return true
			}
		}
		return false

	case *ast.ExtractorPattern:
		return matchExtractor(p, value, env)

	case *ast.ConstructorPattern:
		return matchConstructorPattern(p, value, env)

	case *ast.RangePattern:
		return matchRange(p, value, env)

	case *ast.ComparisonPattern:
		return matchComparison(p, value, env)
	}
	return false
}

func typeMatches(obj object.Object, typeName string) bool {
	switch typeName {
	case "integer":
		return obj.Type() == object.INTEGER_OBJ
	case "float":
		return obj.Type() == object.FLOAT_OBJ
	case "string":
		return obj.Type() == object.STRING_OBJ
	case "boolean":
		return obj.Type() == object.BOOLEAN_OBJ
	case "array":
		return obj.Type() == object.ARRAY_OBJ
	case "hash":
		return obj.Type() == object.HASH_OBJ
	case "function":
		_, ok1 := obj.(*object.Function)
		_, ok2 := obj.(*object.Builtin)
		return ok1 || ok2
	case "null":
		return obj == object.NULL
	}
	return false
}

func matchExtractor(p *ast.ExtractorPattern, value object.Object, env *object.Environment) bool {
	switch p.Name {
	case "Some":
		if value == nil || value == object.NULL {
			return false
		}
		if len(p.Args) > 0 {
			return MatchPattern(p.Args[0], value, env)
		}
		return true

	case "None", "Nil":
		return value == nil || value == object.NULL

	case "All":
		for _, arg := range p.Args {
			tempEnv := object.NewEnclosedEnvironment(env)
			if !MatchPattern(arg, value, tempEnv) {
				return false
			}
			for k, v := range tempEnv.Store {
				env.Set(k, v)
			}
		}
		return true

	case "Any":
		for _, arg := range p.Args {
			tempEnv := object.NewEnclosedEnvironment(env)
			if MatchPattern(arg, value, tempEnv) {
				for k, v := range tempEnv.Store {
					env.Set(k, v)
				}
				return true
			}
		}
		return false

	case "Tuple":
		arr, ok := value.(*object.Array)
		if !ok {
			return false
		}
		if len(arr.Elements) != len(p.Args) {
			return false
		}
		for i, arg := range p.Args {
			if !MatchPattern(arg, arr.Elements[i], env) {
				return false
			}
		}
		return true

	case "Regex":
		strVal, ok := value.(*object.String)
		if !ok {
			return false
		}
		if len(p.Args) == 0 {
			return false
		}
		litPat, ok := p.Args[0].(*ast.LiteralPattern)
		if !ok {
			return false
		}
		regexStr, ok := litPat.Value.(*ast.StringLiteral)
		if !ok {
			return false
		}
		re, err := regexp.Compile(regexStr.Value)
		if err != nil {
			return false
		}
		return re.MatchString(strVal.Value)
	}
	return false
}

func matchConstructorPattern(p *ast.ConstructorPattern, value object.Object, env *object.Environment) bool {
	if owned, ok := value.(*object.OwnedValue); ok {
		value = owned.Inner
	}
	adt, ok := value.(*object.ADTValue)
	if !ok {
		return false
	}
	if p.Name != adt.VariantName {
		return false
	}
	if len(p.Args) != len(adt.Values) {
		return false
	}
	for i, arg := range p.Args {
		if !MatchPattern(arg, adt.Values[i], env) {
			return false
		}
	}
	return true
}

func matchRange(p *ast.RangePattern, value object.Object, env *object.Environment) bool {
	low := Eval(p.Low, env)
	if object.IsError(low) {
		return false
	}
	high := Eval(p.High, env)
	if object.IsError(high) {
		return false
	}
	cmpLow, okLow := objectCompare(value, low)
	cmpHigh, okHigh := objectCompare(value, high)
	if !okLow || !okHigh {
		return false
	}
	return cmpLow >= 0 && cmpHigh <= 0
}

func matchComparison(p *ast.ComparisonPattern, value object.Object, env *object.Environment) bool {
	patVal := Eval(p.Value, env)
	if object.IsError(patVal) {
		return false
	}
	cmp, ok := objectCompare(value, patVal)
	if !ok {
		return false
	}
	switch p.Operator {
	case token.GT:
		return cmp > 0
	case token.GTE:
		return cmp >= 0
	case token.LT:
		return cmp < 0
	case token.LTE:
		return cmp <= 0
	case token.NEQ:
		return cmp != 0
	}
	return false
}
