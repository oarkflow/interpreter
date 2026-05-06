// Package interpreter provides a facade that re-exports every public symbol
// from the pkg/ sub-packages, so that consumers and test files in the root
// package can continue to use `interpreter.Foo` without reaching into sub-packages.
//
// After this file is in place the original root-level source files (core.go,
// functions.go, builtins_*.go, security_policy.go, sandbox_vm.go, etc.) are
// deleted. Only test files remain alongside this facade.
package interpreter

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/lexer"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/parser"
	"github.com/oarkflow/interpreter/pkg/pkgmgr"
	"github.com/oarkflow/interpreter/pkg/playground"
	"github.com/oarkflow/interpreter/pkg/sandbox"
	"github.com/oarkflow/interpreter/pkg/security"
	"github.com/oarkflow/interpreter/pkg/template"
	"github.com/oarkflow/interpreter/pkg/token"

	// Blank imports for side-effect registration.
	builtinspkg "github.com/oarkflow/interpreter/pkg/builtins"
	_ "github.com/oarkflow/interpreter/pkg/builtins/integrations"
	_ "github.com/oarkflow/interpreter/pkg/builtins/reactive"
	"github.com/oarkflow/interpreter/pkg/bytecode"
	"github.com/oarkflow/interpreter/pkg/config"
)

// =========================================================================
// Token type aliases and constants
// =========================================================================

type TokenType = token.TokenType
type Token = token.Token

// TOKEN_* constants re-exported with the legacy prefix used by tests.
const (
	TOKEN_INT    = token.INT
	TOKEN_FLOAT  = token.FLOAT
	TOKEN_STRING = token.STRING
	TOKEN_IDENT  = token.IDENT
	TOKEN_TRUE   = token.TRUE
	TOKEN_FALSE  = token.FALSE
	TOKEN_NULL   = token.NULL

	TOKEN_LET      = token.LET
	TOKEN_IF       = token.IF
	TOKEN_ELSE     = token.ELSE
	TOKEN_WHILE    = token.WHILE
	TOKEN_FOR      = token.FOR
	TOKEN_BREAK    = token.BREAK
	TOKEN_CONTINUE = token.CONTINUE
	TOKEN_FUNCTION = token.FUNCTION
	TOKEN_RETURN   = token.RETURN
	TOKEN_PRINT    = token.PRINT
	TOKEN_CONST    = token.CONST
	TOKEN_IMPORT   = token.IMPORT
	TOKEN_EXPORT   = token.EXPORT
	TOKEN_TRY      = token.TRY
	TOKEN_CATCH    = token.CATCH
	TOKEN_THROW    = token.THROW
	TOKEN_SWITCH   = token.SWITCH
	TOKEN_CASE     = token.CASE
	TOKEN_DEFAULT  = token.DEFAULT
	TOKEN_IN       = token.IN
	TOKEN_DO       = token.DO
	TOKEN_TYPEOF   = token.TYPEOF
	TOKEN_MATCH    = token.MATCH
	TOKEN_ASYNC    = token.ASYNC
	TOKEN_AWAIT    = token.AWAIT
	TOKEN_INIT     = token.INIT
	TOKEN_NEW      = token.NEW

	TOKEN_ASSIGN          = token.ASSIGN
	TOKEN_PLUS            = token.PLUS
	TOKEN_MINUS           = token.MINUS
	TOKEN_MULTIPLY        = token.MULTIPLY
	TOKEN_DIVIDE          = token.DIVIDE
	TOKEN_MODULO          = token.MODULO
	TOKEN_EQ              = token.EQ
	TOKEN_NEQ             = token.NEQ
	TOKEN_LT              = token.LT
	TOKEN_GT              = token.GT
	TOKEN_LTE             = token.LTE
	TOKEN_GTE             = token.GTE
	TOKEN_AND             = token.AND
	TOKEN_OR              = token.OR
	TOKEN_NOT             = token.NOT
	TOKEN_INCREMENT       = token.INCREMENT
	TOKEN_DECREMENT       = token.DECREMENT
	TOKEN_PLUS_ASSIGN     = token.PLUS_ASSIGN
	TOKEN_MINUS_ASSIGN    = token.MINUS_ASSIGN
	TOKEN_MULTIPLY_ASSIGN = token.MULTIPLY_ASSIGN
	TOKEN_DIVIDE_ASSIGN   = token.DIVIDE_ASSIGN
	TOKEN_MODULO_ASSIGN   = token.MODULO_ASSIGN
	TOKEN_NULLISH         = token.NULLISH
	TOKEN_NULLISH_ASSIGN  = token.NULLISH_ASSIGN
	TOKEN_BITAND_ASSIGN   = token.BITAND_ASSIGN
	TOKEN_BITOR_ASSIGN    = token.BITOR_ASSIGN
	TOKEN_BITXOR_ASSIGN   = token.BITXOR_ASSIGN
	TOKEN_LSHIFT_ASSIGN   = token.LSHIFT_ASSIGN
	TOKEN_RSHIFT_ASSIGN   = token.RSHIFT_ASSIGN
	TOKEN_POWER_ASSIGN    = token.POWER_ASSIGN
	TOKEN_AND_ASSIGN      = token.AND_ASSIGN
	TOKEN_OR_ASSIGN       = token.OR_ASSIGN
	TOKEN_PIPELINE        = token.PIPELINE

	TOKEN_LPAREN       = token.LPAREN
	TOKEN_RPAREN       = token.RPAREN
	TOKEN_LBRACE       = token.LBRACE
	TOKEN_RBRACE       = token.RBRACE
	TOKEN_LBRACKET     = token.LBRACKET
	TOKEN_RBRACKET     = token.RBRACKET
	TOKEN_COMMA        = token.COMMA
	TOKEN_SEMICOLON    = token.SEMICOLON
	TOKEN_COLON        = token.COLON
	TOKEN_DOT          = token.DOT
	TOKEN_OPTIONAL_DOT = token.OPTIONAL_DOT
	TOKEN_SPREAD       = token.SPREAD
	TOKEN_RANGE        = token.RANGE
	TOKEN_ARROW        = token.ARROW
	TOKEN_QUESTION     = token.QUESTION

	TOKEN_BITAND = token.BITAND
	TOKEN_BITOR  = token.BITOR
	TOKEN_BITXOR = token.BITXOR
	TOKEN_BITNOT = token.BITNOT
	TOKEN_LSHIFT = token.LSHIFT
	TOKEN_RSHIFT = token.RSHIFT
	TOKEN_POWER  = token.POWER

	TOKEN_EOF     = token.EOF
	TOKEN_ILLEGAL = token.ILLEGAL
)

// =========================================================================
// AST type aliases
// =========================================================================

type (
	Node       = ast.Node
	Expression = ast.Expression
	Statement  = ast.Statement
	Pattern    = ast.Pattern
	Program    = ast.Program

	IntegerLiteral  = ast.IntegerLiteral
	FloatLiteral    = ast.FloatLiteral
	StringLiteral   = ast.StringLiteral
	BooleanLiteral  = ast.BooleanLiteral
	NullLiteral     = ast.NullLiteral
	Identifier      = ast.Identifier
	ArrayLiteral    = ast.ArrayLiteral
	HashEntry       = ast.HashEntry
	HashLiteral     = ast.HashLiteral
	IndexExpression = ast.IndexExpression
	DotExpression   = ast.DotExpression

	OptionalDotExpression    = ast.OptionalDotExpression
	PrefixExpression         = ast.PrefixExpression
	PostfixExpression        = ast.PostfixExpression
	InfixExpression          = ast.InfixExpression
	IfExpression             = ast.IfExpression
	FunctionLiteral          = ast.FunctionLiteral
	SpreadExpression         = ast.SpreadExpression
	CallExpression           = ast.CallExpression
	AssignExpression         = ast.AssignExpression
	CompoundAssignExpression = ast.CompoundAssignExpression

	LetStatement             = ast.LetStatement
	ClassStatement           = ast.ClassStatement
	ClassMethod              = ast.ClassMethod
	InterfaceStatement       = ast.InterfaceStatement
	InterfaceMethod          = ast.InterfaceMethod
	InitStatement            = ast.InitStatement
	TestStatement            = ast.TestStatement
	TypeDeclarationStatement = ast.TypeDeclarationStatement
	ADTVariantDecl           = ast.ADTVariantDecl
	DestructurePattern       = ast.DestructurePattern
	DestructureLetStatement  = ast.DestructureLetStatement
	ReturnStatement          = ast.ReturnStatement
	BreakStatement           = ast.BreakStatement
	ContinueStatement        = ast.ContinueStatement
	ExpressionStatement      = ast.ExpressionStatement
	BlockStatement           = ast.BlockStatement
	WhileStatement           = ast.WhileStatement
	DoWhileStatement         = ast.DoWhileStatement
	ForStatement             = ast.ForStatement
	ForInStatement           = ast.ForInStatement
	PrintStatement           = ast.PrintStatement
	ImportStatement          = ast.ImportStatement
	ExportStatement          = ast.ExportStatement
	ThrowStatement           = ast.ThrowStatement
	TryCatchExpression       = ast.TryCatchExpression
	TernaryExpression        = ast.TernaryExpression
	SwitchStatement          = ast.SwitchStatement
	SwitchCase               = ast.SwitchCase

	LiteralPattern     = ast.LiteralPattern
	WildcardPattern    = ast.WildcardPattern
	BindingPattern     = ast.BindingPattern
	ArrayPattern       = ast.ArrayPattern
	ObjectPattern      = ast.ObjectPattern
	OrPattern          = ast.OrPattern
	ExtractorPattern   = ast.ExtractorPattern
	ConstructorPattern = ast.ConstructorPattern
	RangePattern       = ast.RangePattern
	ComparisonPattern  = ast.ComparisonPattern

	RangeExpression = ast.RangeExpression
	MatchExpression = ast.MatchExpression
	MatchCase       = ast.MatchCase
	AwaitExpression = ast.AwaitExpression
	TemplateLiteral = ast.TemplateLiteral
	LazyExpression  = ast.LazyExpression
)

// =========================================================================
// Object type aliases
// =========================================================================

type (
	Object                 = object.Object
	ObjectType             = object.ObjectType
	Integer                = object.Integer
	Float                  = object.Float
	Boolean                = object.Boolean
	String                 = object.String
	Secret                 = object.Secret
	Null                   = object.Null
	Error                  = object.Error
	CallFrame              = object.CallFrame
	ReturnValue            = object.ReturnValue
	Break                  = object.Break
	Continue               = object.Continue
	Function               = object.Function
	BuiltinFunction        = object.BuiltinFunction
	BuiltinFunctionWithEnv = object.BuiltinFunctionWithEnv
	Builtin                = object.Builtin
	Array                  = object.Array
	HashKey                = object.HashKey
	Hashable               = object.Hashable
	HashPair               = object.HashPair
	Hash                   = object.Hash
	DB                     = object.DB
	DBTx                   = object.DBTx
	Future                 = object.Future
	ADTTypeDef             = object.ADTTypeDef
	ADTValue               = object.ADTValue
	InterfaceLiteral       = object.InterfaceLiteral
	LazyValue              = object.LazyValue
	OwnedValue             = object.OwnedValue
	Channel                = object.Channel
	ImmutableValue         = object.ImmutableValue
	GeneratorValue         = object.GeneratorValue
	FileValue              = object.FileValue
	ImageValue             = object.ImageValue
	TableValue             = object.TableValue
	RuntimeLimits          = object.RuntimeLimits
	TestStats              = object.TestStats
	ModuleContext          = object.ModuleContext
	ModuleCacheEntry       = object.ModuleCacheEntry
	Environment            = object.Environment
)

// ObjectType constants.
const (
	INTEGER_OBJ         = object.INTEGER_OBJ
	FLOAT_OBJ           = object.FLOAT_OBJ
	BOOLEAN_OBJ         = object.BOOLEAN_OBJ
	STRING_OBJ          = object.STRING_OBJ
	NULL_OBJ            = object.NULL_OBJ
	ERROR_OBJ           = object.ERROR_OBJ
	RETURN_VALUE_OBJ    = object.RETURN_VALUE_OBJ
	BREAK_OBJ           = object.BREAK_OBJ
	CONTINUE_OBJ        = object.CONTINUE_OBJ
	FUNCTION_OBJ        = object.FUNCTION_OBJ
	BUILTIN_OBJ         = object.BUILTIN_OBJ
	ARRAY_OBJ           = object.ARRAY_OBJ
	HASH_OBJ            = object.HASH_OBJ
	DB_OBJ              = object.DB_OBJ
	DB_TX_OBJ           = object.DB_TX_OBJ
	FUTURE_OBJ          = object.FUTURE_OBJ
	INTERFACE_OBJ       = object.INTERFACE_OBJ
	ADT_TYPE_OBJ        = object.ADT_TYPE_OBJ
	ADT_VALUE_OBJ       = object.ADT_VALUE_OBJ
	LAZY_OBJ            = object.LAZY_OBJ
	OWNED_OBJ           = object.OWNED_OBJ
	SECRET_OBJ          = object.SECRET_OBJ
	RENDER_ARTIFACT_OBJ = object.RENDER_ARTIFACT_OBJ
	FILE_VALUE_OBJ      = object.FILE_VALUE_OBJ
	IMAGE_VALUE_OBJ     = object.IMAGE_VALUE_OBJ
	TABLE_VALUE_OBJ     = object.TABLE_VALUE_OBJ
	SERVER_OBJ          = object.SERVER_OBJ
	REQUEST_OBJ         = object.REQUEST_OBJ
	RESPONSE_OBJ        = object.RESPONSE_OBJ
	SSE_WRITER_OBJ      = object.SSE_WRITER_OBJ
	QUERY_BUILDER_OBJ   = object.QUERY_BUILDER_OBJ
	LAZY_DB_QUERY_OBJ   = object.LAZY_DB_QUERY_OBJ
	SIGNAL_OBJ          = object.SIGNAL_OBJ
	COMPUTED_OBJ        = object.COMPUTED_OBJ
	EFFECT_OBJ          = object.EFFECT_OBJ
)

// Singleton values.
var (
	TRUE  = object.TRUE
	FALSE = object.FALSE
	NULL  = object.NULL
	BREAK = object.BREAK
	CONT  = object.CONT
)

// =========================================================================
// Lexer / Parser type aliases
// =========================================================================

type (
	Lexer       = lexer.Lexer
	LexerState  = lexer.LexerState
	Parser      = parser.Parser
	ParserState = parser.ParserState
)

// =========================================================================
// Security / Sandbox / Playground type aliases
// =========================================================================

type SecurityPolicy = object.SecurityPolicy
type SandboxConfig = sandbox.SandboxConfig
type SandboxVM = sandbox.SandboxVM

type PlaygroundOptions = playground.PlaygroundOptions
type PlaygroundResult = playground.PlaygroundResult
type RenderConfig = object.RenderConfig
type RenderArtifact = object.RenderArtifact

// =========================================================================
// Exported function re-exports (var = func)
// =========================================================================

var (
	NewLexer                       = lexer.NewLexer
	NewParser                      = parser.NewParser
	Eval                           = eval.Eval
	MatchPattern                   = eval.MatchPattern
	IsTruthy                       = object.IsTruthy
	NewEnvironment                 = object.NewEnvironment
	NewGlobalEnvironment           = object.NewGlobalEnvironment
	NewEnclosedEnvironment         = object.NewEnclosedEnvironment
	ToObject                       = eval.ToObject
	InjectData                     = eval.InjectData
	StartCLI                       = eval.StartCLI
	EvalForPlayground              = playground.EvalForPlayground
	DefaultExecSandboxConfig       = sandbox.DefaultExecSandboxConfig
	DefaultReplSandboxConfig       = sandbox.DefaultReplSandboxConfig
	NewSandboxVM                   = sandbox.NewSandboxVM
	InitModuleManifest             = pkgmgr.InitModuleManifest
	SyncModuleLock                 = pkgmgr.SyncModuleLock
	RegisterTemplateRuntimeFactory = template.RegisterTemplateRuntimeFactory
	RegisterHotReloadHook          = template.RegisterHotReloadHook
)

// =========================================================================
// Package manager constants
// =========================================================================

const (
	SPLManifestFileName = pkgmgr.SPLManifestFileName
	SPLLockFileName     = pkgmgr.SPLLockFileName
)

// =========================================================================
// Unexported wrappers for test compatibility
// =========================================================================

func isError(obj Object) bool                       { return object.IsError(obj) }
func nativeBoolToBooleanObject(input bool) *Boolean { return object.NativeBoolToBooleanObject(input) }
func formatCallStack(stack []CallFrame) string      { return object.FormatCallStack(stack) }

// objectErrorString extracts error message text from an object.
func objectErrorString(obj Object) string {
	if obj == nil {
		return ""
	}
	if h, ok := obj.(*Hash); ok {
		if pair, ok := h.Pairs[(&String{Value: "message"}).HashKey()]; ok {
			if s, ok := pair.Value.(*String); ok {
				return s.Value
			}
			return pair.Value.Inspect()
		}
	}
	if errObj, ok := obj.(*Error); ok {
		return errObj.Message
	}
	if owned, ok := obj.(*OwnedValue); ok {
		return objectErrorString(owned.Inner)
	}
	if strObj, ok := obj.(*String); ok && strings.HasPrefix(strObj.Value, "ERROR:") {
		return strings.TrimPrefix(strObj.Value, "ERROR: ")
	}
	return obj.Inspect()
}

// =========================================================================
// Security policy helpers (kept as thin wrappers over security package)
// =========================================================================

var securityPolicyOverride struct {
	mu     sync.RWMutex
	policy *SecurityPolicy
}

func withSecurityPolicyOverride(policy *SecurityPolicy, fn func() (Object, error)) (Object, error) {
	if policy == nil {
		return fn()
	}
	securityPolicyOverride.mu.Lock()
	prev := securityPolicyOverride.policy
	securityPolicyOverride.policy = policy
	securityPolicyOverride.mu.Unlock()

	defer func() {
		securityPolicyOverride.mu.Lock()
		securityPolicyOverride.policy = prev
		securityPolicyOverride.mu.Unlock()
	}()

	return fn()
}

func activeSandboxBaseDir() string {
	return sandbox.ActiveSandboxBaseDir()
}

func runProgramSandboxed(program *Program, env *Environment, policy *SecurityPolicy) Object {
	return sandbox.RunProgramSandboxed(program, env, policy)
}

// =========================================================================
// Exec API (previously in defs.go)
// =========================================================================

// ExecErrorKind classifies the kind of exec error.
type ExecErrorKind string

const (
	ExecErrorIO         ExecErrorKind = "io"
	ExecErrorParser     ExecErrorKind = "parser"
	ExecErrorRuntime    ExecErrorKind = "runtime"
	ExecErrorValidation ExecErrorKind = "validation"
)

// ExecError is returned by Exec*/ExecFile* functions on failure.
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

// ExecOptions controls the behaviour of ExecWithOptions / ExecFileWithOptions.
type ExecOptions struct {
	Args               []string
	ModuleDir          string
	MaxDepth           int
	MaxSteps           int64
	MaxHeapMB          int64
	MaxOutputBytes     int64
	MaxHTTPBodyBytes   int64
	MaxExecOutputBytes int64
	Timeout            time.Duration
	Context            context.Context
	Output             io.Writer
	Security           *SecurityPolicy
	Sandbox            *SandboxConfig
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
		defer env.RunCleanup()
		if len(opts.Args) > 0 {
			env.Set("ARGS", toObject(opts.Args))
		}
		if opts.Output != nil {
			env.Output = opts.Output
		}
		env.SourcePath = "<memory>"
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
		defer env.RunCleanup()
		if len(opts.Args) > 0 {
			env.Set("ARGS", toObject(opts.Args))
		}
		if opts.Output != nil {
			env.Output = opts.Output
		}
		env.SourcePath = filename
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
	if opts.MaxOutputBytes < 0 {
		return &ExecError{Kind: ExecErrorValidation, Message: "MaxOutputBytes must be >= 0"}
	}
	if opts.MaxHTTPBodyBytes < 0 {
		return &ExecError{Kind: ExecErrorValidation, Message: "MaxHTTPBodyBytes must be >= 0"}
	}
	if opts.MaxExecOutputBytes < 0 {
		return &ExecError{Kind: ExecErrorValidation, Message: "MaxExecOutputBytes must be >= 0"}
	}
	if opts.Timeout < 0 {
		return &ExecError{Kind: ExecErrorValidation, Message: "Timeout must be >= 0"}
	}
	return nil
}

func applyExecRuntimeOptions(env *Environment, opts ExecOptions) {
	rl := env.RuntimeLimits
	if rl == nil {
		rl = &RuntimeLimits{HeapCheckEvery: 128}
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
	if opts.MaxOutputBytes > 0 {
		rl.MaxOutputBytes = opts.MaxOutputBytes
	}
	if opts.MaxHTTPBodyBytes > 0 {
		rl.MaxHTTPBodyBytes = opts.MaxHTTPBodyBytes
	}
	if opts.MaxExecOutputBytes > 0 {
		rl.MaxExecOutputBytes = opts.MaxExecOutputBytes
	}
	if opts.Timeout > 0 {
		rl.Deadline = time.Now().Add(opts.Timeout)
	}
	if opts.Context != nil {
		rl.Ctx = opts.Context
	}

	if rl.MaxDepth == 0 && rl.MaxSteps == 0 && rl.MaxHeapBytes == 0 && rl.MaxOutputBytes == 0 && rl.MaxHTTPBodyBytes == 0 && rl.MaxExecOutputBytes == 0 && rl.Deadline.IsZero() && rl.Ctx == nil {
		env.RuntimeLimits = nil
		return
	}
	env.RuntimeLimits = rl
}

func injectData(env *Environment, data map[string]interface{}) {
	for k, v := range data {
		obj := toObject(v)
		env.Set(k, obj)
	}
}

// =========================================================================
// toObject – converts Go values to SPL objects (previously in defs.go)
// =========================================================================

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
		pairs := make(map[HashKey]HashPair)
		iter := v.MapRange()
		for iter.Next() {
			key := toObject(iter.Key().Interface())
			hashKey, ok := key.(Hashable)
			if !ok {
				continue
			}
			val := toObject(iter.Value().Interface())
			pairs[hashKey.HashKey()] = HashPair{Key: key, Value: val}
		}
		return &Hash{Pairs: pairs}
	case reflect.Struct:
		pairs := make(map[HashKey]HashPair)
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
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

// hashGet looks up a string key in a Hash object.
func hashGet(h *Hash, key string) (Object, bool) {
	if h == nil {
		return nil, false
	}
	k := &String{Value: key}
	pair, ok := h.Pairs[k.HashKey()]
	if !ok {
		return nil, false
	}
	return pair.Value, true
}

// HashGet is the exported version of hashGet for use in external test packages.
func HashGet(h *Hash, key string) (Object, bool) { return hashGet(h, key) }

// ObjectErrorString extracts error message text from an object (exported for tests).
func ObjectErrorString(obj Object) string { return objectErrorString(obj) }

// WithSecurityPolicyOverride temporarily overrides the active security policy (exported for tests).
func WithSecurityPolicyOverride(policy *SecurityPolicy, fn func() (Object, error)) (Object, error) {
	return withSecurityPolicyOverride(policy, fn)
}

// =========================================================================
// Ensure all explicitly-imported packages are used (silence compiler).
// =========================================================================

var (
	_ = security.CheckExecAllowed
	_ = template.RegisterHotReloadHook
	_ = pkgmgr.SPLManifestFileName
	_ = playground.EvalForPlayground
	_ = sandbox.DefaultExecSandboxConfig
	_ = ast.Node(nil)
)

func init() {
	// Wire sandbox function pointers
	sandbox.EvalProgramFn = func(program any, env *object.Environment) object.Object {
		if p, ok := program.(*ast.Program); ok {
			return eval.Eval(p, env)
		}
		return object.NewError("sandbox: expected *ast.Program, got %T", program)
	}

	// Wire object-level function pointers
	object.ApplyFunctionFn = func(fn object.Object, args []object.Object, env *object.Environment) object.Object {
		return eval.ApplyFunction(fn, args, env, nil)
	}
	object.ExtendFunctionEnvFn = func(fn *object.Function, args []object.Object, callerEnv *object.Environment) *object.Environment {
		return eval.ExtendFunctionEnv(fn, args, callerEnv, nil)
	}
	object.UnwrapReturnValueFn = eval.UnwrapReturnValue

	// Wire bytecode function pointers
	bytecode.EvalProgramFn = sandbox.EvalProgramFn
	bytecode.EvalPrefixExpressionFn = eval.EvalPrefixExpression
	bytecode.EvalInfixExpressionFn = eval.EvalInfixExpression

	// Wire eval bytecode fast-path hooks
	eval.BytecodeCompileFn = func(program *ast.Program) (any, error) {
		return bytecode.CompileToBytecode(program)
	}
	eval.BytecodeRunFn = func(compiled any, env *object.Environment) object.Object {
		return bytecode.RunOnVM(compiled.(*bytecode.BytecodeProgram), env)
	}
	eval.BytecodeIsUnsupportedErr = func(err error) bool {
		_, ok := err.(*bytecode.ErrUnsupportedNode)
		return ok
	}

	// Wire playground function pointers
	playground.NewLexerFn = func(input string) any { return lexer.NewLexer(input) }
	playground.NewParserFn = func(l any) any { return parser.NewParser(l.(*lexer.Lexer)) }
	playground.ParseProgramFn = func(p any) (any, []string) {
		pp := p.(*parser.Parser)
		prog := pp.ParseProgram()
		return prog, pp.Errors()
	}
	playground.EvalFn = func(program any, env *object.Environment) object.Object {
		return eval.Eval(program.(*ast.Program), env)
	}
	playground.IsErrorFn = object.IsError
	playground.ObjectErrorStringFn = func(obj object.Object) string {
		if e, ok := obj.(*object.Error); ok {
			return e.Message
		}
		return obj.Inspect()
	}
	playground.FormatCallStackFn = object.FormatCallStack

	// Wire security policy override for sandbox
	sandbox.WithSecurityPolicyOverrideFn = func(policy *object.SecurityPolicy, fn func() (object.Object, error)) (object.Object, error) {
		wrappedFn := func() (any, error) { return fn() }
		result, err := security.WithSecurityPolicyOverride(policy, wrappedFn)
		if result == nil {
			return nil, err
		}
		return result.(object.Object), err
	}

	// Wire playground security policy override
	playground.WithSecurityPolicyOverrideFn = func(policy *object.SecurityPolicy, fn func() (object.Object, error)) (object.Object, error) {
		wrappedFn := func() (any, error) { return fn() }
		result, err := security.WithSecurityPolicyOverride(policy, wrappedFn)
		if result == nil {
			return nil, err
		}
		return result.(object.Object), err
	}

	// Wire config function pointers
	config.ParseJSONToObjectFn = builtinspkg.ParseJSONToObject
	config.ToObjectFn = eval.ToObject
	config.SanitizePathFn = func(p string) (string, error) {
		return builtinspkg.SanitizePathLocal(p)
	}
	config.CheckFileReadAllowedFn = security.CheckFileReadAllowed

	// Wire template runtime
	RegisterTemplateRuntimeFactory(func(baseDir string) TemplateRuntime {
		return template.NewSimpleTemplateRuntime(baseDir)
	})

	initReplBridge()

	// Wire import path resolution
	eval.ResolveImportPathFn = resolveImportPath
}

// resolveImportPath resolves a module import path relative to the caller's
// module directory, the SPL_MODULE_PATH env var, and the manifest lock.
func resolveImportPath(importPath string, env *Environment) (string, error) {
	if filepath.IsAbs(importPath) {
		return sanitizeImportPath(importPath)
	}

	// Try manifest-based resolution first
	moduleDir := ""
	sourcePath := ""
	if env != nil {
		moduleDir = env.ModuleDir
		sourcePath = env.SourcePath
	}
	if resolved, ok, err := pkgmgr.ResolveManifestImport(importPath, moduleDir, sourcePath); ok || err != nil {
		return resolved, err
	}

	// Build candidate paths
	candidates := make([]string, 0, 4)
	if moduleDir != "" {
		candidates = append(candidates, filepath.Join(moduleDir, importPath))
	}
	candidates = append(candidates, importPath)
	for _, base := range importSearchPaths() {
		candidates = append(candidates, filepath.Join(base, importPath))
	}

	var lastErr error
	for _, c := range candidates {
		resolved, err := sanitizeImportPath(c)
		if err != nil {
			lastErr = err
			continue
		}
		if _, err := os.Stat(resolved); err == nil {
			return resolved, nil
		}
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("module not found: %s", importPath)
}

// sanitizeImportPath validates and resolves a path, ensuring it is within the
// sandbox base directory (or CWD if no sandbox is active).
func sanitizeImportPath(userPath string) (string, error) {
	base := activeSandboxBaseDir()
	if strings.TrimSpace(base) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		base = cwd
	}

	absPath, err := filepath.Abs(userPath)
	if err != nil {
		return "", err
	}

	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			dir := filepath.Dir(absPath)
			realDir, dirErr := filepath.EvalSymlinks(dir)
			if dirErr != nil {
				return "", dirErr
			}
			realPath = filepath.Join(realDir, filepath.Base(absPath))
		} else {
			return "", err
		}
	}

	cleanPath := filepath.Clean(realPath)
	baseClean := filepath.Clean(base)
	if !strings.HasSuffix(baseClean, string(os.PathSeparator)) {
		baseClean += string(os.PathSeparator)
	}
	checkPath := cleanPath
	if !strings.HasSuffix(checkPath, string(os.PathSeparator)) {
		checkPath += string(os.PathSeparator)
	}
	if !strings.HasPrefix(checkPath, baseClean) {
		return "", fmt.Errorf("access denied: path '%s' is outside project root", userPath)
	}
	return cleanPath, nil
}

func importSearchPaths() []string {
	raw := strings.TrimSpace(os.Getenv("SPL_MODULE_PATH"))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, string(os.PathListSeparator))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// pkgmgr type aliases and wrappers
type SPLModuleManifest = pkgmgr.SPLModuleManifest
type SPLModuleLock = pkgmgr.SPLModuleLock
type SPLLockedDependency = pkgmgr.SPLLockedDependency

type TemplateRuntime = template.TemplateRuntime
