package eval

import (
	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/object"
)

// ApplyFn is an indirect reference to ApplyFunction, used by builtins to avoid
// init cycles.
var ApplyFn func(object.Object, []object.Object, *object.Environment, *ast.CallExpression) object.Object

func init() {
	ApplyFn = ApplyFunction
	object.ApplyFunctionFn = func(fn object.Object, args []object.Object, env *object.Environment) object.Object {
		return ApplyFunction(fn, args, env, nil)
	}
	object.ExtendFunctionEnvFn = func(fn *object.Function, args []object.Object, callerEnv *object.Environment) *object.Environment {
		return extendFunctionEnv(fn, args, callerEnv, nil)
	}
	object.UnwrapReturnValueFn = unwrapReturnValue
}

// ApplyFunction calls a function object with the given arguments.
func ApplyFunction(fn object.Object, args []object.Object, callerEnv *object.Environment, call *ast.CallExpression) object.Object {
	if fn == nil {
		return object.NewError("attempting to call nil function")
	}

	if errObj := validateFunctionCall(fn, len(args)); errObj != nil {
		if call != nil {
			if runtimeErr, ok := errObj.(*object.Error); ok {
				return runtimeErr.WithFrame(callFrameFromExpression(call.Function, callerEnv, call.Line, call.Column))
			}
		}
		return errObj
	}

	switch fn := fn.(type) {
	case *object.ClassObject:
		return evalClassCall(fn, args, callerEnv)
	case *object.Function:
		extendedEnv := extendFunctionEnv(fn, args, callerEnv, call)
		if errObj := validateBoundFunctionTypes(fn, extendedEnv); errObj != nil {
			return errObj
		}
		if fn.IsGenerator {
			collected := make([]object.Object, 0)
			extendedEnv.YieldSink = &collected
			result := Eval(fn.Body, extendedEnv)
			if err, ok := unwrapReturnValue(result).(*object.Error); ok {
				return err
			}
			return &object.GeneratorValue{Elements: collected, Env: extendedEnv, Body: fn.Body, Func: fn}
		}
		if fn.IsAsync {
			extendedEnv.RuntimeLimits = extendedEnv.RuntimeLimits.CloneForConcurrentExecution()
			ch := make(chan object.Object, 1)
			go func() {
				result := unwrapReturnValue(Eval(fn.Body, extendedEnv))
				ch <- validateFunctionReturnType(fn, result)
			}()
			return &object.Future{Ch: ch}
		}
		evaluated := unwrapReturnValue(Eval(fn.Body, extendedEnv))
		return validateFunctionReturnType(fn, evaluated)

	case *object.Builtin:
		if fn.FnWithEnv != nil {
			return fn.FnWithEnv(fn.Env, args...)
		}
		if len(args) == 1 && fn.Fn1 != nil {
			return fn.Fn1(args[0])
		}
		return fn.Fn(args...)

	default:
		return object.NewError("not a function: %s", fn.Type())
	}
}

func validateBoundFunctionTypes(fn *object.Function, env *object.Environment) object.Object {
	for i, parameter := range fn.Parameters {
		if i >= len(fn.ParamTypes) || fn.ParamTypes[i] == "" {
			continue
		}
		value, _ := env.Get(parameter.Name)
		if !valueMatchesTypeName(value, fn.ParamTypes[i]) {
			return object.NewError("type mismatch for parameter %s: expected %s, got %s", parameter.Name, fn.ParamTypes[i], value.Type())
		}
	}
	return nil
}

func validateFunctionReturnType(fn *object.Function, result object.Object) object.Object {
	if object.IsError(result) || fn.ReturnType == "" || valueMatchesTypeName(result, fn.ReturnType) {
		return result
	}
	return object.NewError("return type mismatch for %s: expected %s, got %s", fn.Name, fn.ReturnType, result.Type())
}

func validateFunctionCall(fn object.Object, argc int) object.Object {
	switch fn := fn.(type) {
	case *object.ClassObject:
		return nil
	case *object.Function:
		if fn.HasRest {
			actualMin := 0
			for i := 0; i < len(fn.Parameters)-1; i++ {
				if i >= len(fn.Defaults) || fn.Defaults[i] == nil {
					actualMin = i + 1
				}
			}
			if argc < actualMin {
				return object.NewError("wrong number of arguments. got=%d, want at least %d", argc, actualMin)
			}
			return nil
		}
		minArgs := 0
		for i, d := range fn.Defaults {
			if d == nil {
				minArgs = i + 1
			}
		}
		if argc < minArgs {
			return object.NewError("wrong number of arguments. got=%d, want at least %d", argc, minArgs)
		}
		if argc > len(fn.Parameters) {
			return object.NewError("wrong number of arguments. got=%d, want at most %d", argc, len(fn.Parameters))
		}
		return nil
	case *object.Builtin:
		return nil
	default:
		return object.NewError("not a function: %s", fn.Type())
	}
}

func extendFunctionEnv(fn *object.Function, args []object.Object, callerEnv *object.Environment, call *ast.CallExpression) *object.Environment {
	callStack := cloneCallStack(nil)
	if callerEnv != nil {
		callStack = cloneCallStack(callerEnv.CallStack)
	}
	if call != nil {
		callStack = append(callStack, callFrameFromExpression(call.Function, callerEnv, call.Line, call.Column))
	}
	env := &object.Environment{
		Store:                  make(map[string]object.Object, len(fn.Parameters)),
		Outer:                  fn.Env,
		ModuleContext:          fn.Env.ModuleContext,
		ModuleDir:              fn.Env.ModuleDir,
		SourcePath:             fn.Env.SourcePath,
		ModuleCache:            fn.Env.ModuleCache,
		ModuleLoading:          fn.Env.ModuleLoading,
		RuntimeLimits:          fn.Env.RuntimeLimits,
		SecurityPolicy:         fn.Env.SecurityPolicy,
		Output:                 fn.Env.Output,
		RenderConfig:           fn.Env.RenderConfig,
		RenderArtifacts:        fn.Env.RenderArtifacts,
		RenderArtifactSink:     fn.Env.RenderArtifactSink,
		CollectRenderArtifacts: fn.Env.CollectRenderArtifacts,
		CallStack:              callStack,
	}
	// Limits belong to the active execution, not to the lexical environment
	// where a function happened to be declared. This also lets background and
	// async callers provide isolated counters to recursive function calls.
	if callerEnv != nil {
		env.RuntimeLimits = callerEnv.RuntimeLimits
	}
	if len(fn.ParamTypes) > 0 {
		env.Set("__param_types", ToObject(fn.ParamTypes))
	}
	if fn.ReturnType != "" {
		env.Set("__return_type", &object.String{Value: fn.ReturnType})
	}

	if fn.HasRest {
		regularCount := len(fn.Parameters) - 1
		for i := 0; i < regularCount; i++ {
			if i < len(args) {
				env.Set(fn.Parameters[i].Name, args[i])
			} else if i < len(fn.Defaults) && fn.Defaults[i] != nil {
				defaultVal := Eval(fn.Defaults[i], fn.Env)
				env.Set(fn.Parameters[i].Name, defaultVal)
			} else {
				env.Set(fn.Parameters[i].Name, object.NULL)
			}
		}
		restElements := []object.Object{}
		if len(args) > regularCount {
			restElements = args[regularCount:]
		}
		env.Set(fn.Parameters[regularCount].Name, &object.Array{Elements: restElements})
	} else {
		for paramIdx, param := range fn.Parameters {
			if paramIdx < len(args) {
				env.Set(param.Name, args[paramIdx])
			} else if paramIdx < len(fn.Defaults) && fn.Defaults[paramIdx] != nil {
				defaultVal := Eval(fn.Defaults[paramIdx], fn.Env)
				env.Set(param.Name, defaultVal)
			} else {
				env.Set(param.Name, object.NULL)
			}
		}
	}

	if fn.Name != "" {
		env.Set(fn.Name, fn)
	}

	return env
}

func unwrapReturnValue(obj object.Object) object.Object {
	if returnValue, ok := obj.(*object.ReturnValue); ok {
		return returnValue.Value
	}
	return obj
}

// UnwrapReturnValue is the exported version of unwrapReturnValue.
func UnwrapReturnValue(obj object.Object) object.Object {
	return unwrapReturnValue(obj)
}

// ExtendFunctionEnv is the exported version of extendFunctionEnv.
func ExtendFunctionEnv(fn *object.Function, args []object.Object, outer *object.Environment, callExpr interface{}) *object.Environment {
	var call *ast.CallExpression
	if callExpr != nil {
		if ce, ok := callExpr.(*ast.CallExpression); ok {
			call = ce
		}
	}
	return extendFunctionEnv(fn, args, outer, call)
}
