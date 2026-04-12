package interpreter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

type ExecErrorKind string

const (
	ExecErrorIO         ExecErrorKind = "io"
	ExecErrorParser     ExecErrorKind = "parser"
	ExecErrorRuntime    ExecErrorKind = "runtime"
	ExecErrorValidation ExecErrorKind = "validation"
)

type ExecError struct {
	Kind        ExecErrorKind
	Message     string
	Path        string
	Diagnostics []string
	Stack       []CallFrame
}

func (e *ExecError) Error() string {
	if e == nil {
		return ""
	}
	msg := e.Message
	if e.Path != "" {
		msg = fmt.Sprintf("%s: %s", e.Path, e.Message)
	}
	if trace := formatCallStack(e.Stack); trace != "" {
		return msg + "\n" + trace
	}
	return msg
}

type ExecOptions struct {
	Args      []string
	ModuleDir string
	MaxDepth  int
	MaxSteps  int64
	MaxHeapMB int64
	Timeout   time.Duration
	Context   context.Context
	Security  *SecurityPolicy
	Sandbox   *SandboxConfig
}

// Exec executes the given SPL script content with the provided data.
func Exec(script string, data map[string]interface{}) (Object, error) {
	return ExecWithOptions(script, data, ExecOptions{})
}

// ExecWithOptions executes SPL script content with caller-provided runtime controls.
func ExecWithOptions(script string, data map[string]interface{}, opts ExecOptions) (Object, error) {
	if err := validateExecOptions(opts); err != nil {
		return nil, err
	}
	return withSecurityPolicyOverride(opts.Security, func() (retObj Object, retErr error) {
		defer func() {
			if r := recover(); r != nil {
				retObj = nil
				retErr = &ExecError{Kind: ExecErrorRuntime, Message: fmt.Sprintf("panic recovered: %v", r)}
			}
		}()

		moduleDir := opts.ModuleDir
		if moduleDir == "" {
			moduleDir = "."
		}
		sb := DefaultExecSandboxConfig()
		if opts.Sandbox != nil {
			sb = *opts.Sandbox
		}
		vm, vmErr := NewSandboxVM([]string{}, "<memory>", moduleDir, sb)
		if vmErr != nil {
			return nil, &ExecError{Kind: ExecErrorValidation, Message: vmErr.Error()}
		}
		env := vm.Environment()
		if len(opts.Args) > 0 {
			env.Set("ARGS", toObject(opts.Args))
		}
		env.sourcePath = "<memory>"
		applyExecRuntimeOptions(env, opts)

		injectData(env, data)

		l := NewLexer(script)
		p := NewParser(l)
		program := p.ParseProgram()

		if len(p.Errors()) != 0 {
			return nil, &ExecError{
				Kind:        ExecErrorParser,
				Message:     fmt.Sprintf("parser errors: %v", p.Errors()),
				Diagnostics: append([]string(nil), p.Errors()...),
			}
		}

		effectivePolicy := vm.Policy()
		if opts.Security != nil {
			effectivePolicy = opts.Security
		}
		result := runProgramSandboxed(program, env, effectivePolicy)

		if isError(result) {
			return nil, runtimeExecError("<memory>", result)
		}

		return result, nil
	})
}

// ExecFile executes the SPL script from a file with the provided data.
func ExecFile(filename string, data map[string]interface{}) (Object, error) {
	return ExecFileWithOptions(filename, data, ExecOptions{})
}

// ExecFileWithOptions executes an SPL script file with caller-provided runtime controls.
func ExecFileWithOptions(filename string, data map[string]interface{}, opts ExecOptions) (Object, error) {
	if err := validateExecOptions(opts); err != nil {
		return nil, err
	}
	return withSecurityPolicyOverride(opts.Security, func() (retObj Object, retErr error) {
		defer func() {
			if r := recover(); r != nil {
				retObj = nil
				retErr = &ExecError{Kind: ExecErrorRuntime, Message: fmt.Sprintf("panic recovered: %v", r), Path: filename}
			}
		}()

		content, err := os.ReadFile(filename)
		if err != nil {
			return nil, &ExecError{Kind: ExecErrorIO, Message: err.Error(), Path: filename}
		}

		moduleDir := opts.ModuleDir
		if moduleDir == "" {
			moduleDir = filepath.Dir(filename)
		}
		sb := DefaultExecSandboxConfig()
		if opts.Sandbox != nil {
			sb = *opts.Sandbox
		}
		vm, vmErr := NewSandboxVM([]string{}, filename, moduleDir, sb)
		if vmErr != nil {
			return nil, &ExecError{Kind: ExecErrorValidation, Message: vmErr.Error(), Path: filename}
		}
		env := vm.Environment()
		if len(opts.Args) > 0 {
			env.Set("ARGS", toObject(opts.Args))
		}
		env.sourcePath = filename
		applyExecRuntimeOptions(env, opts)
		injectData(env, data)

		l := NewLexer(string(content))
		p := NewParser(l)
		program := p.ParseProgram()

		if len(p.Errors()) != 0 {
			return nil, &ExecError{
				Kind:        ExecErrorParser,
				Message:     fmt.Sprintf("parser errors: %v", p.Errors()),
				Path:        filename,
				Diagnostics: append([]string(nil), p.Errors()...),
			}
		}

		effectivePolicy := vm.Policy()
		if opts.Security != nil {
			effectivePolicy = opts.Security
		}
		result := runProgramSandboxed(program, env, effectivePolicy)
		if isError(result) {
			return nil, runtimeExecError(filename, result)
		}

		return result, nil
	})
}

func runtimeExecError(path string, obj Object) *ExecError {
	execErr := &ExecError{
		Kind:    ExecErrorRuntime,
		Message: objectErrorString(obj),
		Path:    path,
	}
	if errObj, ok := obj.(*Error); ok {
		execErr.Stack = append([]CallFrame(nil), errObj.Stack...)
		if len(errObj.Stack) > 0 {
			execErr.Diagnostics = append(execErr.Diagnostics, formatCallStack(errObj.Stack))
		}
	}
	return execErr
}

func validateExecOptions(opts ExecOptions) error {
	if opts.MaxDepth < 0 {
		return &ExecError{Kind: ExecErrorValidation, Message: "MaxDepth must be >= 0"}
	}
	if opts.MaxSteps < 0 {
		return &ExecError{Kind: ExecErrorValidation, Message: "MaxSteps must be >= 0"}
	}
	if opts.MaxHeapMB < 0 {
		return &ExecError{Kind: ExecErrorValidation, Message: "MaxHeapMB must be >= 0"}
	}
	if opts.Timeout < 0 {
		return &ExecError{Kind: ExecErrorValidation, Message: "Timeout must be >= 0"}
	}
	return nil
}

func applyExecRuntimeOptions(env *Environment, opts ExecOptions) {
	rl := env.runtimeLimits
	if rl == nil {
		rl = &RuntimeLimits{heapCheckEvery: 128}
	}

	if opts.MaxDepth > 0 {
		rl.MaxDepth = opts.MaxDepth
	}
	if opts.MaxSteps > 0 {
		rl.MaxSteps = opts.MaxSteps
	}
	if opts.MaxHeapMB > 0 {
		rl.MaxHeapBytes = uint64(opts.MaxHeapMB) * 1024 * 1024
	}
	if opts.Timeout > 0 {
		rl.Deadline = time.Now().Add(opts.Timeout)
	}
	if opts.Context != nil {
		rl.Ctx = opts.Context
	}

	if rl.MaxDepth == 0 && rl.MaxSteps == 0 && rl.MaxHeapBytes == 0 && rl.Deadline.IsZero() && rl.Ctx == nil {
		env.runtimeLimits = nil
		return
	}
	env.runtimeLimits = rl
}

func injectData(env *Environment, data map[string]interface{}) {
	for k, v := range data {
		obj := toObject(v)
		env.Set(k, obj)
	}
}

// InjectData injects Go values into an SPL environment as variables.
func InjectData(env *Environment, data map[string]interface{}) {
	injectData(env, data)
}

// ToObject converts a Go value to an SPL Object.
func ToObject(val interface{}) Object {
	return toObject(val)
}

// toObject converts a Go value to an SPL Object
func toObject(val interface{}) Object {
	if val == nil {
		return NULL
	}

	switch v := val.(type) {
	case Object:
		return v
	case bool:
		return nativeBoolToBooleanObject(v)
	case int:
		return &Integer{Value: int64(v)}
	case int8:
		return &Integer{Value: int64(v)}
	case int16:
		return &Integer{Value: int64(v)}
	case int32:
		return &Integer{Value: int64(v)}
	case int64:
		return &Integer{Value: v}
	case uint:
		return &Integer{Value: int64(v)}
	case uint8:
		return &Integer{Value: int64(v)}
	case uint16:
		return &Integer{Value: int64(v)}
	case uint32:
		return &Integer{Value: int64(v)}
	case uint64:
		return &Integer{Value: int64(v)}
	case float32:
		return &Float{Value: float64(v)}
	case float64:
		return &Float{Value: v}
	case string:
		return &String{Value: v}
	case []Object:
		return &Array{Elements: append([]Object(nil), v...)}
	case []string:
		elements := make([]Object, len(v))
		for i := range v {
			elements[i] = &String{Value: v[i]}
		}
		return &Array{Elements: elements}
	case []interface{}:
		elements := make([]Object, len(v))
		for i := range v {
			elements[i] = toObject(v[i])
		}
		return &Array{Elements: elements}
	case map[string]interface{}:
		pairs := make(map[HashKey]HashPair, len(v))
		for k, vv := range v {
			key := &String{Value: k}
			pairs[key.HashKey()] = HashPair{Key: key, Value: toObject(vv)}
		}
		return &Hash{Pairs: pairs}
	case map[string]string:
		pairs := make(map[HashKey]HashPair, len(v))
		for k, vv := range v {
			key := &String{Value: k}
			pairs[key.HashKey()] = HashPair{Key: key, Value: &String{Value: vv}}
		}
		return &Hash{Pairs: pairs}
	}

	v := reflect.ValueOf(val)

	switch v.Kind() {
	case reflect.Bool:
		return nativeBoolToBooleanObject(v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &Integer{Value: v.Int()}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &Integer{Value: int64(v.Uint())}
	case reflect.Float32, reflect.Float64:
		return &Float{Value: v.Float()}
	case reflect.String:
		return &String{Value: v.String()}
	case reflect.Slice, reflect.Array:
		elements := make([]Object, v.Len())
		for i := 0; i < v.Len(); i++ {
			elements[i] = toObject(v.Index(i).Interface())
		}
		return &Array{Elements: elements}
	case reflect.Map:
		// Only support map[string]interface{} or similar for now ideally
		// But we can iterate keys
		pairs := make(map[HashKey]HashPair)
		iter := v.MapRange()
		for iter.Next() {
			key := toObject(iter.Key().Interface())
			// Key must be hashable
			hashKey, ok := key.(Hashable)
			if !ok {
				continue // Skip unhashable keys
			}
			val := toObject(iter.Value().Interface()) // Recursion
			pairs[hashKey.HashKey()] = HashPair{Key: key, Value: val}
		}
		return &Hash{Pairs: pairs}
	case reflect.Struct:
		// Convert struct to Hash
		pairs := make(map[HashKey]HashPair)
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			// Use json tag if available? For simplicity use Name.
			fieldName := field.Name
			key := &String{Value: fieldName}
			val := toObject(v.Field(i).Interface())
			pairs[key.HashKey()] = HashPair{Key: key, Value: val}
		}
		return &Hash{Pairs: pairs}
	default:
		return &String{Value: fmt.Sprintf("%v", val)}
	}
}
