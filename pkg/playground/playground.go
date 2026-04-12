package playground

import (
	"bytes"
	"strings"
	"time"

	"github.com/oarkflow/interpreter/pkg/object"
)

// ---------------------------------------------------------------------------
// Function variables – must be set by the host package at init time.
// ---------------------------------------------------------------------------

// NewLexerFn creates a new lexer for the given input string.
var NewLexerFn func(input string) any

// NewParserFn creates a new parser from a lexer.
var NewParserFn func(lexer any) any

// ParseProgramFn calls ParseProgram on the parser and returns
// (program, errors).
var ParseProgramFn func(parser any) (program any, errors []string)

// EvalFn evaluates an AST program in the given environment.
var EvalFn func(program any, env *object.Environment) object.Object

// WithSecurityPolicyOverrideFn temporarily overrides the active security
// policy while running fn.
var WithSecurityPolicyOverrideFn func(policy *object.SecurityPolicy, fn func() (object.Object, error)) (object.Object, error)

// IsErrorFn returns true if the object is an Error.
var IsErrorFn func(obj object.Object) bool

// ObjectErrorStringFn extracts the error message from an Error object.
var ObjectErrorStringFn func(obj object.Object) string

// FormatCallStackFn formats a call stack trace for display.
var FormatCallStackFn func(stack []object.CallFrame) string

// ---------------------------------------------------------------------------
// PlaygroundResult / PlaygroundOptions
// ---------------------------------------------------------------------------

type PlaygroundResult struct {
	Output      string   `json:"output"`
	Result      string   `json:"result"`
	ResultTy    string   `json:"result_type"`
	Error       string   `json:"error"`
	ErrorKind   string   `json:"error_kind"`
	Diagnostics []string `json:"diagnostics,omitempty"`
	Duration    int64    `json:"duration_ms"`
}

type PlaygroundOptions struct {
	Args      []string
	MaxDepth  int
	MaxSteps  int64
	MaxHeapMB int64
	TimeoutMS int64
	ModuleDir string
	Security  *object.SecurityPolicy
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func friendlyParserError(raw string) string {
	msg := strings.TrimSpace(raw)
	if msg == "" {
		return ""
	}
	if strings.Contains(msg, "expected") && strings.Contains(msg, "got") {
		return msg + " Check syntax near the reported line/column."
	}
	if strings.Contains(msg, "unexpected token") {
		return msg + " Verify parentheses, braces, commas, and statement separators ';'."
	}
	if strings.Contains(msg, "could not parse") {
		return msg + " Verify numeric/string literal format."
	}
	return msg
}

func FormatParserErrors(errs []string) string {
	if len(errs) == 0 {
		return ""
	}
	friendly := make([]string, 0, len(errs))
	for _, e := range errs {
		friendly = append(friendly, friendlyParserError(e))
	}
	return "Parser error(s):\n- " + strings.Join(friendly, "\n- ")
}

// ---------------------------------------------------------------------------
// EvalForPlayground
// ---------------------------------------------------------------------------

// EvalForPlayground evaluates a script in a sandboxed playground context.
func EvalForPlayground(script string, opts PlaygroundOptions) PlaygroundResult {
	start := time.Now()
	res := PlaygroundResult{}
	policy := opts.Security
	if policy == nil {
		policy = &object.SecurityPolicy{ProtectHost: true}
	}

	overrideFn := WithSecurityPolicyOverrideFn
	if overrideFn == nil {
		// Fallback: just run without policy override
		overrideFn = func(p *object.SecurityPolicy, fn func() (object.Object, error)) (object.Object, error) {
			return fn()
		}
	}

	_, err := overrideFn(policy, func() (object.Object, error) {
		env := object.NewGlobalEnvironment(opts.Args)
		if opts.ModuleDir != "" {
			env.ModuleDir = opts.ModuleDir
		} else {
			env.ModuleDir = "."
		}
		env.SourcePath = "<playground>"

		buf := &bytes.Buffer{}
		env.Output = buf

		rl := &object.RuntimeLimits{HeapCheckEvery: 128}
		if opts.MaxDepth > 0 {
			rl.MaxDepth = opts.MaxDepth
		}
		if opts.MaxSteps > 0 {
			rl.MaxSteps = opts.MaxSteps
		}
		if opts.MaxHeapMB > 0 {
			rl.MaxHeapBytes = uint64(opts.MaxHeapMB) * 1024 * 1024
		}
		if opts.TimeoutMS > 0 {
			rl.Deadline = time.Now().Add(time.Duration(opts.TimeoutMS) * time.Millisecond)
		}
		if rl.MaxDepth > 0 || rl.MaxSteps > 0 || rl.MaxHeapBytes > 0 || !rl.Deadline.IsZero() {
			env.RuntimeLimits = rl
		} else {
			env.RuntimeLimits = nil
		}

		if NewLexerFn == nil || NewParserFn == nil || ParseProgramFn == nil || EvalFn == nil {
			res.Error = "playground: lexer/parser/eval functions not configured"
			res.ErrorKind = "internal"
			return nil, nil
		}

		l := NewLexerFn(script)
		p := NewParserFn(l)
		program, pErrors := ParseProgramFn(p)
		if len(pErrors) != 0 {
			res.Error = FormatParserErrors(pErrors)
			res.ErrorKind = "parser"
			res.Diagnostics = append(res.Diagnostics, pErrors...)
			res.Output = buf.String()
			return nil, nil
		}

		evaluated := EvalFn(program, env)
		res.Output = buf.String()
		if evaluated != nil {
			isErr := false
			if IsErrorFn != nil {
				isErr = IsErrorFn(evaluated)
			} else {
				isErr = evaluated.Type() == object.ERROR_OBJ
			}
			if isErr {
				if ObjectErrorStringFn != nil {
					res.Error = ObjectErrorStringFn(evaluated)
				} else {
					res.Error = evaluated.Inspect()
				}
				res.ErrorKind = "runtime"
				if errObj, ok := evaluated.(*object.Error); ok && len(errObj.Stack) > 0 {
					if FormatCallStackFn != nil {
						res.Diagnostics = append(res.Diagnostics, FormatCallStackFn(errObj.Stack))
					}
				}
			} else {
				res.Result = evaluated.Inspect()
				res.ResultTy = evaluated.Type().String()
			}
		}
		return evaluated, nil
	})
	if err != nil {
		res.Error = err.Error()
		res.ErrorKind = "runtime"
	}
	res.Duration = time.Since(start).Milliseconds()
	return res
}
