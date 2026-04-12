package repl

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/term"

	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/pkgmgr"
)

// ---------------------------------------------------------------------------
// Function variables – pluggable from the host package
// ---------------------------------------------------------------------------

// NewLexerFn creates a new lexer from source input.
var NewLexerFn func(input string) any

// NewParserFn creates a new parser from a lexer.
var NewParserFn func(lexer any) any

// ParseProgramFn parses and returns (program, errors).
var ParseProgramFn func(parser any) (program any, errors []string)

// LexerNextTokenFn returns the next token from a lexer.
// Token is returned as (tokenType int, literal string).
var LexerNextTokenFn func(lexer any) (tokenType int, literal string)

// EvalFn evaluates an AST node in the given environment.
var EvalFn func(node any, env *object.Environment) object.Object

// RunProgramSandboxedFn evaluates a program in the sandbox.
var RunProgramSandboxedFn func(program any, env *object.Environment, policy *object.SecurityPolicy) object.Object

// ResolveImportPathFn resolves an import path to a file system path.
var ResolveImportPathFn func(path string, env *object.Environment) (string, error)

// HasBuiltinFn returns true if a builtin with the given name exists.
var HasBuiltinFn func(name string) bool

// BuiltinHelpTextFn returns help text for a builtin.
var BuiltinHelpTextFn func(name string) string

// DispatchHotReloadHooksFn fires hot-reload hooks for a path.
var DispatchHotReloadHooksFn func(path string)

// IsErrorFn returns true if the object is an Error.
var IsErrorFn func(obj object.Object) bool

// ObjectErrorStringFn formats an error object as a string.
var ObjectErrorStringFn func(obj object.Object) string

// FormatCallStackFn formats call-stack frames.
var FormatCallStackFn func(stack []object.CallFrame) string

// LineContextFn extracts a source-line context snippet.
var LineContextFn func(source string, line, column int) string

// LoadConfigObjectFromPathFn loads a config file.
var LoadConfigObjectFromPathFn func(path, format string) (object.Object, error)

// ---------------------------------------------------------------------------
// Token type constants – must be set by the host package for continuation
// detection.
// ---------------------------------------------------------------------------

var (
	TOKEN_EOF      int
	TOKEN_LPAREN   int
	TOKEN_RPAREN   int
	TOKEN_LBRACE   int
	TOKEN_RBRACE   int
	TOKEN_LBRACKET int
	TOKEN_RBRACKET int
	TOKEN_ASSIGN   int
	TOKEN_PLUS     int
	TOKEN_MINUS    int
	TOKEN_MULTIPLY int
	TOKEN_DIVIDE   int
	TOKEN_MODULO   int
	TOKEN_EQ       int
	TOKEN_NEQ      int
	TOKEN_LT       int
	TOKEN_GT       int
	TOKEN_LTE      int
	TOKEN_GTE      int
	TOKEN_AND      int
	TOKEN_OR       int
	TOKEN_BITAND   int
	TOKEN_BITOR    int
	TOKEN_BITXOR   int
	TOKEN_COMMA    int
	TOKEN_COLON    int
	TOKEN_DOT      int
	TOKEN_LET      int
	TOKEN_CONST    int
	TOKEN_RETURN   int
	TOKEN_IF       int
	TOKEN_ELSE     int
	TOKEN_FOR      int
	TOKEN_WHILE    int
	TOKEN_FUNCTION int
	TOKEN_TRY      int
	TOKEN_CATCH    int
	TOKEN_SWITCH   int
	TOKEN_CASE     int
	TOKEN_THROW    int
	TOKEN_IMPORT   int
	TOKEN_EXPORT   int
)

// ---------------------------------------------------------------------------
// replEditor
// ---------------------------------------------------------------------------

type ReplEditor struct {
	In          *os.File
	Out         *os.File
	Fd          int
	OldState    *term.State
	Env         *object.Environment
	History     []string
	HistoryPos  int
	HistoryFile string
	HistoryBase int
	Candidates  []string
}

const replHistoryFileName = ".interpreter_repl_history"

type keyAction int

const (
	keyUnknown keyAction = iota
	keyUp
	keyDown
	keyLeft
	keyRight
	keyHome
	keyEnd
	keyDelete
)

// ---------------------------------------------------------------------------
// Public entry points
// ---------------------------------------------------------------------------

// RunReplInteractive starts the interactive REPL with line editing.
func RunReplInteractive(env *object.Environment) error {
	if !IsTerminal(os.Stdin) {
		return fmt.Errorf("stdin is not a terminal")
	}

	editor, err := newReplEditor(os.Stdin, os.Stdout, ReplCandidatesForEnv(env), env)
	if err != nil {
		return err
	}
	defer editor.close()

	for {
		editor.Candidates = ReplCandidatesForEnv(env)
		line, err := editor.readLine(">> ")
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		if HandleReplMetaCommand(line, editor, env) {
			continue
		}
		if strings.TrimSpace(line) == "exit" {
			return nil
		}
		if strings.TrimSpace(line) == "" {
			continue
		}

		input := line
		for ReplNeedsContinuation(input) {
			editor.Candidates = ReplCandidatesForEnv(env)
			nextLine, err := editor.readLine(".. ")
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			input += "\n" + nextLine
		}

		EvalReplInput(input, env)
	}
}

// RunReplBasic starts a simple line-based REPL without raw terminal input.
func RunReplBasic(env *object.Environment) {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print(StylePrompt(">> "))
		if !scanner.Scan() {
			return
		}
		line := scanner.Text()
		if strings.TrimSpace(line) == "exit" {
			return
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if HandleReplMetaCommand(line, nil, env) {
			continue
		}

		input := line
		for ReplNeedsContinuation(input) {
			fmt.Print(StyleContinuationPrompt(".. "))
			if !scanner.Scan() {
				return
			}
			nextLine := scanner.Text()
			input += "\n" + nextLine
		}
		EvalReplInput(input, env)
	}
}

// ---------------------------------------------------------------------------
// Continuation detection
// ---------------------------------------------------------------------------

// ReplNeedsContinuation returns true when the input looks incomplete and
// requires additional lines.
func ReplNeedsContinuation(input string) bool {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return false
	}

	if NewLexerFn == nil || LexerNextTokenFn == nil {
		return false
	}

	balanceParen := 0
	balanceBrace := 0
	balanceBracket := 0
	lastTok := TOKEN_EOF

	l := NewLexerFn(input)
	for {
		tokType, _ := LexerNextTokenFn(l)
		if tokType == TOKEN_EOF {
			break
		}
		lastTok = tokType
		switch tokType {
		case TOKEN_LPAREN:
			balanceParen++
		case TOKEN_RPAREN:
			balanceParen--
		case TOKEN_LBRACE:
			balanceBrace++
		case TOKEN_RBRACE:
			balanceBrace--
		case TOKEN_LBRACKET:
			balanceBracket++
		case TOKEN_RBRACKET:
			balanceBracket--
		}
	}

	if balanceParen > 0 || balanceBrace > 0 || balanceBracket > 0 {
		return true
	}
	if strings.HasSuffix(trimmed, "\\") {
		return true
	}

	continuationTokens := map[int]struct{}{
		TOKEN_ASSIGN: {}, TOKEN_PLUS: {}, TOKEN_MINUS: {}, TOKEN_MULTIPLY: {},
		TOKEN_DIVIDE: {}, TOKEN_MODULO: {}, TOKEN_EQ: {}, TOKEN_NEQ: {},
		TOKEN_LT: {}, TOKEN_GT: {}, TOKEN_LTE: {}, TOKEN_GTE: {},
		TOKEN_AND: {}, TOKEN_OR: {}, TOKEN_BITAND: {}, TOKEN_BITOR: {},
		TOKEN_BITXOR: {}, TOKEN_COMMA: {}, TOKEN_COLON: {}, TOKEN_DOT: {},
		TOKEN_LET: {}, TOKEN_CONST: {}, TOKEN_RETURN: {}, TOKEN_IF: {},
		TOKEN_ELSE: {}, TOKEN_FOR: {}, TOKEN_WHILE: {}, TOKEN_FUNCTION: {},
		TOKEN_TRY: {}, TOKEN_CATCH: {}, TOKEN_SWITCH: {}, TOKEN_CASE: {},
		TOKEN_THROW: {}, TOKEN_IMPORT: {}, TOKEN_EXPORT: {},
	}
	if _, ok := continuationTokens[lastTok]; ok {
		return true
	}

	_, errs := ReplParseProgram(input)
	for _, err := range errs {
		lower := strings.ToLower(err)
		if strings.Contains(lower, "got eof") || strings.Contains(lower, "got <eof>") || strings.Contains(lower, "unexpected token eof") {
			return true
		}
	}

	return false
}

// ---------------------------------------------------------------------------
// Evaluation helpers
// ---------------------------------------------------------------------------

// EvalReplInput evaluates a complete REPL input string.
func EvalReplInput(input string, env *object.Environment) {
	ReplEvalSource(input, env, "<repl>", true)
}

// ReplEvalSource evaluates source code and optionally prints the result.
func ReplEvalSource(input string, env *object.Environment, sourcePath string, printResult bool) {
	if NewLexerFn == nil || NewParserFn == nil || ParseProgramFn == nil {
		fmt.Println(Paint("error: lexer/parser not configured", ColorRed))
		return
	}
	l := NewLexerFn(input)
	p := NewParserFn(l)
	program, pErrors := ParseProgramFn(p)
	if len(pErrors) != 0 {
		for _, msg := range pErrors {
			fmt.Println(Paint(msg, ColorRed))
		}
		return
	}

	prevModuleDir := ""
	prevSourcePath := ""
	if env != nil {
		prevModuleDir = env.ModuleDir
		prevSourcePath = env.SourcePath
		if sourcePath != "" {
			env.SourcePath = sourcePath
			if sourcePath != "<repl>" && sourcePath != "<memory>" {
				env.ModuleDir = filepath.Dir(sourcePath)
			}
		}
		defer func() {
			env.ModuleDir = prevModuleDir
			env.SourcePath = prevSourcePath
		}()
	}

	var evaluated object.Object
	if RunProgramSandboxedFn != nil {
		evaluated = RunProgramSandboxedFn(program, env, env.SecurityPolicy)
	} else if EvalFn != nil {
		evaluated = EvalFn(program, env)
	} else {
		fmt.Println(Paint("error: eval function not configured", ColorRed))
		return
	}

	if evaluated != nil {
		isErr := false
		if IsErrorFn != nil {
			isErr = IsErrorFn(evaluated)
		} else {
			isErr = evaluated.Type() == object.ERROR_OBJ
		}
		if isErr {
			fmt.Println(FormatRuntimeErrorForDisplay(evaluated, input))
			return
		}
		if printResult && evaluated.Type() != object.NULL_OBJ {
			fmt.Println(FormatObjectForDisplay(evaluated))
		}
	}
}

func ReplPrintLine(s string) {
	fmt.Print("\r")
	fmt.Println(s)
}

func replPrintBlock(s string) {
	for _, line := range strings.Split(s, "\n") {
		ReplPrintLine(line)
	}
}

// ReplCandidatesForEnv returns completion candidates from builtins, keywords,
// and environment variables.
func ReplCandidatesForEnv(env *object.Environment) []string {
	kw := []string{
		"let", "if", "else", "while", "for", "in", "break", "continue", "function", "return",
		"print", "const", "import", "export", "true", "false", "null", "do", "typeof",
		"try", "catch", "throw", "switch", "case", "default",
		"exit", ":help", ":builtins", ":search", ":history", ":clear",
		":vars", ":type", ":doc", ":methods", ":fields", ":ast", ":time", ":load", ":reload", ":reset",
		":debug", ":mem", ":install",
		":config",
	}
	all := make(map[string]struct{}, len(kw)+16)
	if BuiltinNames != nil {
		for name := range BuiltinNames() {
			all[name] = struct{}{}
		}
	}
	for _, k := range kw {
		all[k] = struct{}{}
	}
	if env != nil {
		env.Mu.RLock()
		for name := range env.Store {
			all[name] = struct{}{}
		}
		env.Mu.RUnlock()
	}
	out := make([]string, 0, len(all))
	for k := range all {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ReplParseProgram parses input and returns (program, errors).
func ReplParseProgram(input string) (any, []string) {
	if NewLexerFn == nil || NewParserFn == nil || ParseProgramFn == nil {
		return nil, []string{"parser not configured"}
	}
	l := NewLexerFn(input)
	p := NewParserFn(l)
	program, errs := ParseProgramFn(p)
	if len(errs) != 0 {
		return nil, append([]string(nil), errs...)
	}
	return program, nil
}

func replEvalExpression(input string, env *object.Environment) (object.Object, []string) {
	program, errs := ReplParseProgram(input)
	if len(errs) != 0 {
		return nil, errs
	}
	prevModuleDir := ""
	prevSourcePath := ""
	if env != nil {
		prevModuleDir = env.ModuleDir
		prevSourcePath = env.SourcePath
		env.SourcePath = "<repl>"
		defer func() {
			env.ModuleDir = prevModuleDir
			env.SourcePath = prevSourcePath
		}()
	}
	if RunProgramSandboxedFn != nil {
		return RunProgramSandboxedFn(program, env, env.SecurityPolicy), nil
	}
	if EvalFn != nil {
		return EvalFn(program, env), nil
	}
	return nil, []string{"eval not configured"}
}

func replPrintParserErrors(errs []string) {
	for _, msg := range errs {
		replPrintBlock(Paint(msg, ColorRed))
	}
}

func ReplResolvedPath(path string, env *object.Environment) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("path is required")
	}
	if ResolveImportPathFn != nil {
		return ResolveImportPathFn(trimmed, env)
	}
	return filepath.Abs(trimmed)
}

func ReplDocText(name string, env *object.Environment) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "usage: :doc <builtin|identifier|expression>"
	}
	if HasBuiltinFn != nil && HasBuiltinFn(name) {
		if BuiltinHelpTextFn != nil {
			return BuiltinHelpTextFn(name)
		}
	}
	if env != nil {
		if val, ok := env.Get(name); ok {
			return fmt.Sprintf("%s: %s\n%s", name, val.Type(), FormatObjectPlain(val))
		}
	}
	if result, errs := replEvalExpression(name, env); len(errs) == 0 && result != nil {
		return fmt.Sprintf("%s\nType: %s\nValue:\n%s", name, result.Type(), FormatObjectPlain(result))
	}
	return fmt.Sprintf("no documentation available for %q", name)
}

// ReplObjectMethods returns known methods for common object types.
func ReplObjectMethods(obj object.Object) []string {
	if obj == nil {
		return nil
	}
	switch v := obj.(type) {
	case *object.OwnedValue:
		return ReplObjectMethods(v.Inner)
	case *object.ImmutableValue:
		return ReplObjectMethods(v.Inner)
	case *object.GeneratorValue:
		return ReplObjectMethods(&object.Array{Elements: v.Elements})
	case *object.Hash:
		return []string{"entries", "keys", "length", "values"}
	case *object.String:
		return []string{"at", "camel_case", "charAt", "count_substr", "ends_with", "endsWith", "includes", "index_of", "indexOf", "kebab_case", "length", "lower", "pad_left", "pad_right", "padEnd", "padStart", "pascal_case", "regex_match", "regex_replace", "repeat", "replace", "slug", "snake_case", "split", "split_lines", "starts_with", "startsWith", "substring", "swap_case", "title", "toLowerCase", "toUpperCase", "trim", "trim_prefix", "trim_suffix", "truncate", "upper"}
	case *object.Integer:
		return []string{"abs", "is_even", "isEven", "is_odd", "isOdd", "pow", "sqrt", "to_float", "to_string", "toFloat", "toString"}
	case *object.Float:
		return []string{"abs", "ceil", "floor", "round", "to_int", "to_string", "toInt", "toString"}
	case *object.Array:
		return []string{"at", "every", "filter", "find", "flat", "flatMap", "forEach", "includes", "indexOf", "join", "length", "map", "pop", "push", "reduce", "reverse", "shift", "slice", "some", "sort", "unshift"}
	default:
		_ = v
		return nil
	}
}

// ReplObjectFields returns known fields for common object types.
func ReplObjectFields(obj object.Object) []string {
	if obj == nil {
		return nil
	}
	switch v := obj.(type) {
	case *object.OwnedValue:
		return ReplObjectFields(v.Inner)
	case *object.ImmutableValue:
		return ReplObjectFields(v.Inner)
	case *object.GeneratorValue:
		return ReplObjectFields(&object.Array{Elements: v.Elements})
	case *object.Hash:
		fields := make([]string, 0, len(v.Pairs))
		for _, pair := range v.Pairs {
			fields = append(fields, pair.Key.Inspect())
		}
		sort.Strings(fields)
		return fields
	case *object.String:
		return []string{"length"}
	case *object.Array:
		return []string{"length"}
	default:
		_ = v
		return nil
	}
}

func replDescribeObjectList(title string, items []string) string {
	if len(items) == 0 {
		return title + ": none"
	}
	return title + ":\n- " + strings.Join(items, "\n- ")
}

// ---------------------------------------------------------------------------
// Meta-command handler
// ---------------------------------------------------------------------------

// HandleReplMetaCommand processes REPL meta-commands (lines starting with
// ':' or '!'). Returns true if the line was handled.
func HandleReplMetaCommand(line string, editor *ReplEditor, env *object.Environment) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if trimmed == ":help" {
		ReplPrintLine(Paint("Interactive features:", ColorBold+ColorCyan))
		ReplPrintLine("- Arrow keys: history and cursor movement")
		ReplPrintLine("- Tab: semantic completion for names/methods/fields")
		ReplPrintLine("- Inline suggestion: gray suffix preview")
		ReplPrintLine("- Call tips: signatures/docs shown while typing calls")
		ReplPrintLine("- Ctrl+R: reverse history search")
		ReplPrintLine("- Ctrl+C: clear current line")
		ReplPrintLine("- Ctrl+D: exit when line is empty")
		ReplPrintLine(Paint("Commands:", ColorBold+ColorCyan))
		ReplPrintLine("- :builtins   list all available builtins")
		ReplPrintLine("- :search X   search builtins/keywords by text")
		ReplPrintLine("- :history    print command history")
		ReplPrintLine("- :clear      clear screen")
		ReplPrintLine("- :vars       list all variables in current environment")
		ReplPrintLine("- :type <expr> show the type of an expression")
		ReplPrintLine("- :doc <name> show builtin/object documentation")
		ReplPrintLine("- :methods <expr> list methods available on a value")
		ReplPrintLine("- :fields <expr> list fields available on a value")
		ReplPrintLine("- :ast <expr> print parsed AST representation")
		ReplPrintLine("- :time <expr> evaluate and show execution time")
		ReplPrintLine("- :debug <expr> step through statements")
		ReplPrintLine("- :mem        show runtime memory usage")
		ReplPrintLine("- :load <file> load and execute a script file")
		ReplPrintLine("- :reload [file] clear module cache or one module")
		ReplPrintLine("- :install <alias> <path> add dependency to spl.mod and refresh lock")
		ReplPrintLine("- :config <file> [format] load config with secret masking")
		ReplPrintLine("- !<cmd>      execute shell command")
		ReplPrintLine("- :reset      reset the environment")
		return true
	}
	if strings.HasPrefix(trimmed, ":config ") {
		args := strings.Fields(strings.TrimSpace(strings.TrimPrefix(trimmed, ":config ")))
		if len(args) < 1 || len(args) > 2 {
			ReplPrintLine("usage: :config <file> [json|yaml|env]")
			return true
		}
		format := ""
		if len(args) == 2 {
			format = args[1]
		}
		if LoadConfigObjectFromPathFn == nil {
			ReplPrintLine("config loader not available")
			return true
		}
		obj, err := LoadConfigObjectFromPathFn(args[0], format)
		if err != nil {
			ReplPrintLine("config error: " + err.Error())
			return true
		}
		if env != nil {
			env.Set("CONFIG", obj)
		}
		ReplPrintLine("CONFIG loaded")
		return true
	}
	if strings.HasPrefix(trimmed, "!") {
		cmdText := strings.TrimSpace(strings.TrimPrefix(trimmed, "!"))
		if cmdText == "" {
			ReplPrintLine("usage: !<shell command>")
			return true
		}
		out, err := replRunShellCommand(cmdText)
		if strings.TrimSpace(out) != "" {
			replPrintBlock(strings.TrimRight(out, "\n"))
		}
		if err != nil {
			ReplPrintLine("shell error: " + err.Error())
		}
		return true
	}
	if trimmed == ":builtins" {
		if BuiltinNames != nil {
			names := make([]string, 0)
			for name := range BuiltinNames() {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				ReplPrintLine(name)
			}
		} else {
			ReplPrintLine("builtins not available")
		}
		return true
	}
	if trimmed == ":history" {
		if editor == nil {
			ReplPrintLine("history is only available in interactive mode")
			return true
		}
		for i, h := range editor.History {
			ReplPrintLine(fmt.Sprintf("%4d  %s", i+1, h))
		}
		return true
	}
	if trimmed == ":clear" {
		fmt.Print("\033[2J\033[H")
		return true
	}
	if strings.HasPrefix(trimmed, ":search ") {
		query := strings.TrimSpace(strings.TrimPrefix(trimmed, ":search "))
		if query == "" {
			ReplPrintLine("usage: :search <text>")
			return true
		}
		candidates := ReplCandidatesForEnv(env)
		found := 0
		for _, c := range candidates {
			if strings.Contains(strings.ToLower(c), strings.ToLower(query)) {
				ReplPrintLine(c)
				found++
			}
		}
		if found == 0 {
			ReplPrintLine("no matches found")
		}
		return true
	}
	if trimmed == ":vars" {
		if env == nil {
			ReplPrintLine("environment not available")
			return true
		}
		env.Mu.RLock()
		names := make([]string, 0, len(env.Store))
		for name := range env.Store {
			names = append(names, name)
		}
		env.Mu.RUnlock()
		sort.Strings(names)
		for _, name := range names {
			val, _ := env.Get(name)
			ReplPrintLine(fmt.Sprintf("  %s = %s", name, FormatObjectPlain(val)))
		}
		if len(names) == 0 {
			ReplPrintLine("no variables defined")
		}
		return true
	}
	if trimmed == ":reset" {
		if env != nil {
			env.Mu.Lock()
			env.Store = make(map[string]object.Object)
			env.Mu.Unlock()
		}
		ReplPrintLine("environment reset")
		return true
	}
	if strings.HasPrefix(trimmed, ":type ") {
		expr := strings.TrimSpace(strings.TrimPrefix(trimmed, ":type "))
		if expr == "" {
			ReplPrintLine("usage: :type <expr>")
			return true
		}
		if env != nil {
			result, errs := replEvalExpression(expr, env)
			if len(errs) != 0 {
				replPrintParserErrors(errs)
			} else if result != nil {
				ReplPrintLine(result.Type().String())
			}
		}
		return true
	}
	if strings.HasPrefix(trimmed, ":doc ") {
		target := strings.TrimSpace(strings.TrimPrefix(trimmed, ":doc "))
		replPrintBlock(ReplDocText(target, env))
		return true
	}
	if strings.HasPrefix(trimmed, ":methods ") {
		expr := strings.TrimSpace(strings.TrimPrefix(trimmed, ":methods "))
		if expr == "" {
			ReplPrintLine("usage: :methods <expr>")
			return true
		}
		result, errs := replEvalExpression(expr, env)
		if len(errs) != 0 {
			replPrintParserErrors(errs)
			return true
		}
		replPrintBlock(replDescribeObjectList("methods", ReplObjectMethods(result)))
		return true
	}
	if strings.HasPrefix(trimmed, ":fields ") {
		expr := strings.TrimSpace(strings.TrimPrefix(trimmed, ":fields "))
		if expr == "" {
			ReplPrintLine("usage: :fields <expr>")
			return true
		}
		result, errs := replEvalExpression(expr, env)
		if len(errs) != 0 {
			replPrintParserErrors(errs)
			return true
		}
		replPrintBlock(replDescribeObjectList("fields", ReplObjectFields(result)))
		return true
	}
	if strings.HasPrefix(trimmed, ":ast ") {
		expr := strings.TrimSpace(strings.TrimPrefix(trimmed, ":ast "))
		if expr == "" {
			ReplPrintLine("usage: :ast <expr>")
			return true
		}
		program, errs := ReplParseProgram(expr)
		if len(errs) != 0 {
			replPrintParserErrors(errs)
			return true
		}
		replPrintBlock(fmt.Sprintf("Program\n%#v", program))
		return true
	}
	if strings.HasPrefix(trimmed, ":time ") {
		expr := strings.TrimSpace(strings.TrimPrefix(trimmed, ":time "))
		if expr == "" {
			ReplPrintLine("usage: :time <expr>")
			return true
		}
		if env != nil {
			start := time.Now()
			result, errs := replEvalExpression(expr, env)
			if len(errs) != 0 {
				replPrintParserErrors(errs)
			} else {
				elapsed := time.Since(start)
				isErr := false
				if result != nil {
					if IsErrorFn != nil {
						isErr = IsErrorFn(result)
					} else {
						isErr = result.Type() == object.ERROR_OBJ
					}
				}
				if result != nil && !isErr && result.Type() != object.NULL_OBJ {
					ReplPrintLine(FormatObjectForDisplay(result))
				}
				ReplPrintLine(Paint(fmt.Sprintf("elapsed: %s", elapsed), ColorGray))
			}
		}
		return true
	}
	if strings.HasPrefix(trimmed, ":debug ") {
		expr := strings.TrimSpace(strings.TrimPrefix(trimmed, ":debug "))
		if expr == "" {
			ReplPrintLine("usage: :debug <expr>")
			return true
		}
		replDebugExpression(expr, env)
		return true
	}
	if trimmed == ":mem" {
		ReplPrintLine(ReplMemoryUsage())
		return true
	}
	if strings.HasPrefix(trimmed, ":load ") {
		path := strings.TrimSpace(strings.TrimPrefix(trimmed, ":load "))
		resolved, err := ReplResolvedPath(path, env)
		if err != nil {
			ReplPrintLine("load error: " + err.Error())
			return true
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			ReplPrintLine("load error: " + err.Error())
			return true
		}
		ReplEvalSource(string(data), env, resolved, true)
		return true
	}
	if strings.HasPrefix(trimmed, ":reload") {
		arg := strings.TrimSpace(strings.TrimPrefix(trimmed, ":reload"))
		if env == nil {
			ReplPrintLine("environment not available")
			return true
		}
		if arg == "" {
			env.ModuleCache = make(map[string]object.ModuleCacheEntry)
			ReplPrintLine("module cache cleared")
			return true
		}
		resolved, err := ReplResolvedPath(arg, env)
		if err != nil {
			ReplPrintLine("reload error: " + err.Error())
			return true
		}
		delete(env.ModuleCacheMap(), resolved)
		if DispatchHotReloadHooksFn != nil {
			DispatchHotReloadHooksFn(resolved)
		}
		ReplPrintLine("reloaded: " + resolved)
		return true
	}
	if strings.HasPrefix(trimmed, ":install ") {
		args := strings.Fields(strings.TrimSpace(strings.TrimPrefix(trimmed, ":install ")))
		if len(args) != 2 {
			ReplPrintLine("usage: :install <alias> <path>")
			return true
		}
		if err := ReplInstallDependency(args[0], args[1], env); err != nil {
			ReplPrintLine("install error: " + err.Error())
		} else {
			ReplPrintLine(fmt.Sprintf("installed %s => %s", args[0], args[1]))
		}
		return true
	}
	return false
}

// ReplCandidates returns completion candidates without an environment.
func ReplCandidates() []string {
	return ReplCandidatesForEnv(nil)
}

func newReplEditor(in, out *os.File, candidates []string, env *object.Environment) (*ReplEditor, error) {
	fd := int(in.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}

	editor := &ReplEditor{
		In:         in,
		Out:        out,
		Fd:         fd,
		OldState:   state,
		Env:        env,
		History:    make([]string, 0, 256),
		HistoryPos: 0,
		Candidates: candidates,
	}

	if historyFile, err := replHistoryPath(); err == nil {
		editor.HistoryFile = historyFile
		if loaded, err := LoadHistoryEntries(historyFile); err == nil {
			editor.History = append(editor.History, loaded...)
		}
		editor.HistoryBase = len(editor.History)
	}

	return editor, nil
}

func (e *ReplEditor) close() {
	if e.HistoryFile != "" {
		_ = AppendHistoryEntries(e.HistoryFile, HistoryEntriesToPersist(e.History, e.HistoryBase))
	}
	if e.OldState != nil {
		_ = term.Restore(e.Fd, e.OldState)
	}
}

func replHistoryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("empty home directory")
	}
	return filepath.Join(home, replHistoryFileName), nil
}

func LoadHistoryEntries(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return ParseHistoryData(data), nil
}

func ParseHistoryData(data []byte) []string {
	raw := strings.Split(string(data), "\n")
	out := make([]string, 0, len(raw))
	for _, entry := range raw {
		entry = strings.TrimRight(entry, "\r")
		if strings.TrimSpace(entry) == "" {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func HistoryEntriesToPersist(history []string, from int) []string {
	if from < 0 {
		from = 0
	}
	if from >= len(history) {
		return nil
	}
	out := make([]string, 0, len(history)-from)
	for _, entry := range history[from:] {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func AppendHistoryEntries(path string, entries []string) error {
	if len(entries) == 0 {
		return nil
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, entry := range entries {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		if _, err := w.WriteString(entry); err != nil {
			return err
		}
		if _, err := w.WriteString("\n"); err != nil {
			return err
		}
	}

	return w.Flush()
}

func (e *ReplEditor) readLine(prompt string) (string, error) {
	buf := make([]rune, 0, 128)
	cursor := 0
	e.HistoryPos = len(e.History)
	render := func() {
		_, _ = fmt.Fprint(e.Out, "\r\033[2K")
		line := string(buf)
		styledPrompt := StylePrompt(prompt)
		if strings.HasPrefix(prompt, "..") {
			styledPrompt = StyleContinuationPrompt(prompt)
		}
		_, _ = fmt.Fprint(e.Out, styledPrompt, ColorizeInputLine(line))
		if cursor == len(buf) {
			ctx := CompletionContext(buf, cursor)
			if ctx.Ok && ctx.Prefix != "" {
				if suggestion, ok := firstCompletion(ctx.Prefix, e.CompletionsForContext(ctx)); ok && suggestion != ctx.Prefix {
					suffix := suggestion[len(ctx.Prefix):]
					_, _ = fmt.Fprint(e.Out, Paint(suffix, ColorGray))
				}
			}
			if tip := ReplCallTip(line, cursor, e.Env); tip != "" {
				_, _ = fmt.Fprint(e.Out, Paint("  "+tip, ColorGray))
			}
		}
		_, _ = fmt.Fprint(e.Out, "\r")
		_, _ = fmt.Fprintf(e.Out, "\033[%dC", len([]rune(prompt))+cursor)
	}

	render()
	var b [1]byte
	for {
		_, err := e.In.Read(b[:])
		if err != nil {
			return "", err
		}
		ch := b[0]
		switch ch {
		case '\r', '\n':
			_, _ = fmt.Fprint(e.Out, "\r\n")
			line := string(buf)
			if strings.TrimSpace(line) != "" {
				if len(e.History) == 0 || e.History[len(e.History)-1] != line {
					e.History = append(e.History, line)
				}
			}
			return line, nil
		case 3: // Ctrl+C
			buf = buf[:0]
			cursor = 0
			_, _ = fmt.Fprint(e.Out, "^C\r\n")
			return "", nil
		case 4: // Ctrl+D
			if len(buf) == 0 {
				_, _ = fmt.Fprint(e.Out, "\r\n")
				return "", io.EOF
			}
		case 1: // Ctrl+A
			cursor = 0
		case 5: // Ctrl+E
			cursor = len(buf)
		case 18: // Ctrl+R
			buf, cursor = e.ReverseHistorySearch(buf)
		case 9: // Tab
			buf, cursor = e.applyCompletion(buf, cursor, prompt)
		case 127, 8: // backspace
			if cursor > 0 {
				buf = append(buf[:cursor-1], buf[cursor:]...)
				cursor--
			}
		case 27: // escape sequence
			switch e.readEscapeAction() {
			case keyUp:
				if len(e.History) > 0 && e.HistoryPos > 0 {
					e.HistoryPos--
					buf = []rune(e.History[e.HistoryPos])
					cursor = len(buf)
				}
			case keyDown:
				if len(e.History) > 0 && e.HistoryPos < len(e.History)-1 {
					e.HistoryPos++
					buf = []rune(e.History[e.HistoryPos])
					cursor = len(buf)
				} else if e.HistoryPos >= len(e.History)-1 {
					e.HistoryPos = len(e.History)
					buf = buf[:0]
					cursor = 0
				}
			case keyRight:
				if cursor < len(buf) {
					cursor++
				}
			case keyLeft:
				if cursor > 0 {
					cursor--
				}
			case keyHome:
				cursor = 0
			case keyEnd:
				cursor = len(buf)
			case keyDelete:
				if cursor < len(buf) {
					buf = append(buf[:cursor], buf[cursor+1:]...)
				}
			}
		default:
			if ch >= 32 {
				r := rune(ch)
				if cursor == len(buf) {
					buf = append(buf, r)
				} else {
					buf = append(buf, 0)
					copy(buf[cursor+1:], buf[cursor:])
					buf[cursor] = r
				}
				cursor++
			}
		}
		render()
	}
}

func (e *ReplEditor) ReverseHistorySearch(current []rune) ([]rune, int) {
	query := strings.TrimSpace(string(current))
	if len(e.History) == 0 {
		return current, len(current)
	}
	start := e.HistoryPos
	if start > len(e.History)-1 {
		start = len(e.History) - 1
	}
	for i := start; i >= 0; i-- {
		entry := e.History[i]
		if query == "" || strings.Contains(entry, query) {
			e.HistoryPos = i
			r := []rune(entry)
			return r, len(r)
		}
	}
	_, _ = fmt.Fprint(e.Out, "\a")
	return current, len(current)
}

func (e *ReplEditor) readEscapeAction() keyAction {
	var b [1]byte
	if _, err := e.In.Read(b[:]); err != nil {
		return keyUnknown
	}

	switch b[0] {
	case '[':
		return e.readCSIAction()
	case 'O':
		if _, err := e.In.Read(b[:]); err != nil {
			return keyUnknown
		}
		switch b[0] {
		case 'A':
			return keyUp
		case 'B':
			return keyDown
		case 'C':
			return keyRight
		case 'D':
			return keyLeft
		case 'H':
			return keyHome
		case 'F':
			return keyEnd
		default:
			return keyUnknown
		}
	default:
		return keyUnknown
	}
}

func (e *ReplEditor) readCSIAction() keyAction {
	var b [1]byte
	if _, err := e.In.Read(b[:]); err != nil {
		return keyUnknown
	}

	switch b[0] {
	case 'A':
		return keyUp
	case 'B':
		return keyDown
	case 'C':
		return keyRight
	case 'D':
		return keyLeft
	case 'H':
		return keyHome
	case 'F':
		return keyEnd
	}

	seq := []byte{b[0]}
	for {
		if _, err := e.In.Read(b[:]); err != nil {
			break
		}
		seq = append(seq, b[0])
		if (b[0] >= 'A' && b[0] <= 'Z') || (b[0] >= 'a' && b[0] <= 'z') || b[0] == '~' {
			break
		}
		if len(seq) >= 8 {
			break
		}
	}
	s := string(seq)

	switch {
	case strings.HasSuffix(s, "A"):
		return keyUp
	case strings.HasSuffix(s, "B"):
		return keyDown
	case strings.HasSuffix(s, "C"):
		return keyRight
	case strings.HasSuffix(s, "D"):
		return keyLeft
	case strings.HasSuffix(s, "H"):
		return keyHome
	case strings.HasSuffix(s, "F"):
		return keyEnd
	case strings.HasPrefix(s, "1~"), strings.HasPrefix(s, "7~"):
		return keyHome
	case strings.HasPrefix(s, "4~"), strings.HasPrefix(s, "8~"):
		return keyEnd
	case strings.HasPrefix(s, "3~"):
		return keyDelete
	default:
		return keyUnknown
	}
}

func (e *ReplEditor) applyCompletion(buf []rune, cursor int, prompt string) ([]rune, int) {
	ctx := CompletionContext(buf, cursor)
	if !ctx.Ok || ctx.Prefix == "" {
		return buf, cursor
	}
	matches := FindCompletions(ctx.Prefix, e.CompletionsForContext(ctx))
	if len(matches) == 0 {
		return buf, cursor
	}
	if len(matches) == 1 {
		completion := []rune(matches[0])
		newBuf := append([]rune{}, buf[:ctx.Start]...)
		newBuf = append(newBuf, completion...)
		newBuf = append(newBuf, buf[ctx.End:]...)
		return newBuf, ctx.Start + len(completion)
	}

	common := LongestCommonPrefix(matches)
	if len(common) > len(ctx.Prefix) {
		completion := []rune(common)
		newBuf := append([]rune{}, buf[:ctx.Start]...)
		newBuf = append(newBuf, completion...)
		newBuf = append(newBuf, buf[ctx.End:]...)
		return newBuf, ctx.Start + len(completion)
	}

	_, _ = fmt.Fprint(e.Out, "\r\n")
	for _, m := range matches {
		_, _ = fmt.Fprintln(e.Out, m)
	}
	if strings.HasPrefix(prompt, "..") {
		_, _ = fmt.Fprint(e.Out, StyleContinuationPrompt(prompt))
	} else {
		_, _ = fmt.Fprint(e.Out, StylePrompt(prompt))
	}
	return buf, cursor
}

type ReplCompletionContext struct {
	Prefix   string
	BaseExpr string
	Start    int
	End      int
	Ok       bool
}

func CompletionContext(buf []rune, cursor int) ReplCompletionContext {
	prefix, start, end, ok := CurrentToken(buf, cursor)
	if !ok {
		return ReplCompletionContext{}
	}
	ctx := ReplCompletionContext{Prefix: prefix, Start: start, End: end, Ok: true}
	if start <= 0 || buf[start-1] != '.' {
		return ctx
	}
	baseEnd := start - 1
	baseStart := baseEnd
	for baseStart > 0 {
		r := buf[baseStart-1]
		if isTokenRune(r) || r == '.' {
			baseStart--
			continue
		}
		break
	}
	base := strings.TrimSpace(string(buf[baseStart:baseEnd]))
	if base != "" {
		ctx.BaseExpr = base
	}
	return ctx
}

func (e *ReplEditor) CompletionsForContext(ctx ReplCompletionContext) []string {
	if ctx.BaseExpr == "" || e.Env == nil {
		return e.Candidates
	}
	obj, errs := replEvalExpression(ctx.BaseExpr, e.Env)
	if len(errs) != 0 || obj == nil {
		return e.Candidates
	}
	if IsErrorFn != nil && IsErrorFn(obj) {
		return e.Candidates
	}
	fields := ReplObjectFields(obj)
	methods := ReplObjectMethods(obj)
	out := make([]string, 0, len(fields)+len(methods))
	out = append(out, fields...)
	out = append(out, methods...)
	sort.Strings(out)
	if len(out) == 0 {
		return e.Candidates
	}
	return out
}

func ReplCallTip(line string, cursor int, env *object.Environment) string {
	runes := []rune(line)
	if cursor < 0 || cursor > len(runes) {
		return ""
	}
	if cursor == 0 {
		return ""
	}
	i := cursor - 1
	for i >= 0 && unicode.IsSpace(runes[i]) {
		i--
	}
	if i < 0 || runes[i] != '(' {
		return ""
	}
	j := i - 1
	for j >= 0 && isTokenRune(runes[j]) {
		j--
	}
	name := string(runes[j+1 : i])
	if strings.TrimSpace(name) == "" {
		return ""
	}
	if HasBuiltinFn != nil && HasBuiltinFn(name) {
		if BuiltinHelpTextFn != nil {
			return BuiltinHelpTextFn(name)
		}
	}
	if env != nil {
		if val, ok := env.Get(name); ok {
			if fn, ok := val.(*object.Function); ok {
				return fn.Inspect()
			}
			return fmt.Sprintf("%s: %s", name, val.Type())
		}
	}
	return ""
}

func CurrentToken(buf []rune, cursor int) (prefix string, start int, end int, ok bool) {
	if cursor < 0 || cursor > len(buf) {
		return "", 0, 0, false
	}
	start = cursor
	for start > 0 && isTokenRune(buf[start-1]) {
		start--
	}
	end = cursor
	for end < len(buf) && isTokenRune(buf[end]) {
		end++
	}
	if start == end {
		return "", 0, 0, false
	}
	return string(buf[start:cursor]), start, end, true
}

func isTokenRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == ':'
}

func FindCompletions(prefix string, candidates []string) []string {
	out := make([]string, 0, 8)
	for _, c := range candidates {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

func firstCompletion(prefix string, candidates []string) (string, bool) {
	for _, c := range candidates {
		if strings.HasPrefix(c, prefix) {
			return c, true
		}
	}
	return "", false
}

func LongestCommonPrefix(items []string) string {
	if len(items) == 0 {
		return ""
	}
	prefix := items[0]
	for _, s := range items[1:] {
		for !strings.HasPrefix(s, prefix) {
			if prefix == "" {
				return ""
			}
			prefix = prefix[:len(prefix)-1]
		}
	}
	return prefix
}

func ReplMemoryUsage() string {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	toMB := func(v uint64) uint64 { return v / (1024 * 1024) }
	return fmt.Sprintf(
		"mem: alloc=%dMB total_alloc=%dMB sys=%dMB num_gc=%d",
		toMB(ms.Alloc),
		toMB(ms.TotalAlloc),
		toMB(ms.Sys),
		ms.NumGC,
	)
}

func replRunShellCommand(cmdText string) (string, error) {
	var cmd *exec.Cmd
	if isWindowsRuntime() {
		cmd = exec.Command("cmd", "/C", cmdText)
	} else {
		cmd = exec.Command("sh", "-c", cmdText)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func isWindowsRuntime() bool {
	return runtime.GOOS == "windows"
}

func ReplInstallDependency(alias, source string, env *object.Environment) error {
	alias = strings.TrimSpace(alias)
	source = strings.TrimSpace(source)
	if alias == "" || source == "" {
		return fmt.Errorf("alias and source path are required")
	}
	startDir := ""
	if env != nil {
		startDir = env.ModuleDir
		if startDir == "" {
			startDir = env.SourcePath
		}
	}
	projectDir := pkgmgr.DiscoverProjectRoot(startDir)
	manifestPath := filepath.Join(projectDir, pkgmgr.SPLManifestFileName)
	manifest := &pkgmgr.SPLModuleManifest{Module: pkgmgr.DefaultModuleName(projectDir), Dependencies: map[string]string{}}
	if _, err := os.Stat(manifestPath); err == nil {
		loaded, readErr := pkgmgr.ReadModuleManifestFromFile(manifestPath)
		if readErr != nil {
			return readErr
		}
		manifest = loaded
	}
	if manifest.Dependencies == nil {
		manifest.Dependencies = map[string]string{}
	}
	manifest.Dependencies[alias] = source
	if err := pkgmgr.WriteModuleManifest(projectDir, manifest); err != nil {
		return err
	}
	_, err := pkgmgr.SyncModuleLock(projectDir)
	return err
}

func replDebugExpression(input string, env *object.Environment) {
	program, errs := ReplParseProgram(input)
	if len(errs) != 0 {
		replPrintParserErrors(errs)
		return
	}
	// We need to extract statements from the program. Since program is `any`,
	// we use a type assertion to an interface with a Statements field.
	type hasStatements interface {
		GetStatements() []any
	}
	type hasString interface {
		String() string
	}
	pgm, ok := program.(hasStatements)
	if !ok {
		ReplPrintLine("debug: cannot extract statements from program")
		return
	}
	stmts := pgm.GetStatements()
	if len(stmts) == 0 {
		ReplPrintLine("debug: no statements")
		return
	}
	ReplPrintLine("debug mode: commands = step|next, continue|c, locals|vars, break <n>, quit")
	idx := 0
	breakpoints := map[int]struct{}{}
	for idx < len(stmts) {
		stmtStr := ""
		if s, ok := stmts[idx].(hasString); ok {
			stmtStr = strings.TrimSpace(s.String())
		} else {
			stmtStr = fmt.Sprintf("%v", stmts[idx])
		}
		ReplPrintLine(fmt.Sprintf("[%d/%d] %s", idx+1, len(stmts), stmtStr))
		cmd, err := replReadDebugCommand()
		if err != nil {
			ReplPrintLine("debug error: " + err.Error())
			return
		}
		switch {
		case cmd == "", cmd == "step", cmd == "next", cmd == "n", cmd == "s":
			if EvalFn == nil {
				ReplPrintLine("debug: eval not configured")
				return
			}
			obj := EvalFn(stmts[idx], env)
			if obj != nil && obj.Type() != object.NULL_OBJ {
				isErr := false
				if IsErrorFn != nil {
					isErr = IsErrorFn(obj)
				} else {
					isErr = obj.Type() == object.ERROR_OBJ
				}
				if isErr {
					replPrintBlock(FormatRuntimeErrorForDisplay(obj, input))
					return
				}
				ReplPrintLine(FormatObjectForDisplay(obj))
			}
			idx++
		case cmd == "locals", cmd == "vars":
			env.Mu.RLock()
			names := make([]string, 0, len(env.Store))
			for n := range env.Store {
				names = append(names, n)
			}
			env.Mu.RUnlock()
			sort.Strings(names)
			for _, n := range names {
				val, _ := env.Get(n)
				ReplPrintLine(fmt.Sprintf("  %s = %s", n, FormatObjectPlain(val)))
			}
		case strings.HasPrefix(cmd, "break "):
			arg := strings.TrimSpace(strings.TrimPrefix(cmd, "break "))
			lineNo := 0
			_, scanErr := fmt.Sscanf(arg, "%d", &lineNo)
			if scanErr != nil || lineNo < 1 || lineNo > len(stmts) {
				ReplPrintLine("usage: break <statement-index>")
				continue
			}
			breakpoints[lineNo-1] = struct{}{}
			ReplPrintLine(fmt.Sprintf("breakpoint set at %d", lineNo))
		case cmd == "continue", cmd == "c":
			for idx < len(stmts) {
				if _, ok := breakpoints[idx]; ok {
					ReplPrintLine(fmt.Sprintf("hit breakpoint at %d", idx+1))
					break
				}
				if EvalFn == nil {
					ReplPrintLine("debug: eval not configured")
					return
				}
				obj := EvalFn(stmts[idx], env)
				if obj != nil {
					isErr := false
					if IsErrorFn != nil {
						isErr = IsErrorFn(obj)
					} else {
						isErr = obj.Type() == object.ERROR_OBJ
					}
					if isErr {
						replPrintBlock(FormatRuntimeErrorForDisplay(obj, input))
						return
					}
				}
				idx++
			}
		case cmd == "quit", cmd == "q", cmd == "exit":
			ReplPrintLine("debug aborted")
			return
		default:
			ReplPrintLine("unknown debug command")
		}
	}
	ReplPrintLine("debug finished")
}

// FormatRuntimeErrorForDisplay formats an error object with context for
// REPL display.
func FormatRuntimeErrorForDisplay(obj object.Object, source string) string {
	errObj, ok := obj.(*object.Error)
	if !ok || errObj == nil {
		errStr := ""
		if ObjectErrorStringFn != nil {
			errStr = ObjectErrorStringFn(obj)
		} else {
			errStr = obj.Inspect()
		}
		return Paint("ERROR: "+errStr, ColorBold+ColorRed)
	}
	var out strings.Builder
	out.WriteString("ERROR")
	if strings.TrimSpace(errObj.Code) != "" {
		out.WriteString(" [")
		out.WriteString(errObj.Code)
		out.WriteString("]")
	}
	out.WriteString(": ")
	out.WriteString(errObj.Message)

	if errObj.Path != "" {
		out.WriteString("\nPath: ")
		out.WriteString(errObj.Path)
	}
	if errObj.Line > 0 {
		out.WriteString(fmt.Sprintf("\nLocation: line %d", errObj.Line))
		if errObj.Column > 0 {
			out.WriteString(fmt.Sprintf(", column %d", errObj.Column))
		}
		if LineContextFn != nil {
			if ctx := LineContextFn(source, errObj.Line, errObj.Column); ctx != "" {
				out.WriteString("\n")
				out.WriteString(ctx)
			}
		}
	}
	if FormatCallStackFn != nil {
		if trace := FormatCallStackFn(errObj.Stack); trace != "" {
			out.WriteString("\n")
			out.WriteString(trace)
		}
	}
	return Paint(out.String(), ColorBold+ColorRed)
}

func replReadDebugCommand() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print(StyleContinuationPrompt("dbg> "))
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
