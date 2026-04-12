package interpreter

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
)

type replEditor struct {
	in          *os.File
	out         *os.File
	fd          int
	oldState    *term.State
	env         *Environment
	history     []string
	historyPos  int
	historyFile string
	historyBase int
	candidates  []string
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

func runReplInteractive(env *Environment) error {
	if !isTerminal(os.Stdin) {
		return fmt.Errorf("stdin is not a terminal")
	}

	editor, err := newReplEditor(os.Stdin, os.Stdout, replCandidatesForEnv(env), env)
	if err != nil {
		return err
	}
	defer editor.close()

	for {
		editor.candidates = replCandidatesForEnv(env)
		line, err := editor.readLine(">> ")
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		if handleReplMetaCommand(line, editor, env) {
			continue
		}
		if strings.TrimSpace(line) == "exit" {
			return nil
		}
		if strings.TrimSpace(line) == "" {
			continue
		}

		input := line
		for replNeedsContinuation(input) {
			editor.candidates = replCandidatesForEnv(env)
			nextLine, err := editor.readLine(".. ")
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			input += "\n" + nextLine
		}

		evalReplInput(input, env)
	}
}

func runReplBasic(env *Environment) {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print(stylePrompt(">> "))
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
		if handleReplMetaCommand(line, nil, env) {
			continue
		}

		input := line
		for replNeedsContinuation(input) {
			fmt.Print(styleContinuationPrompt(".. "))
			if !scanner.Scan() {
				return
			}
			nextLine := scanner.Text()
			input += "\n" + nextLine
		}
		evalReplInput(input, env)
	}
}

func replNeedsContinuation(input string) bool {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return false
	}

	balanceParen := 0
	balanceBrace := 0
	balanceBracket := 0
	lastTok := TOKEN_EOF

	l := NewLexer(input)
	for {
		tok := l.NextToken()
		if tok.Type == TOKEN_EOF {
			break
		}
		lastTok = tok.Type
		switch tok.Type {
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

	switch lastTok {
	case TOKEN_ASSIGN, TOKEN_PLUS, TOKEN_MINUS, TOKEN_MULTIPLY, TOKEN_DIVIDE, TOKEN_MODULO,
		TOKEN_EQ, TOKEN_NEQ, TOKEN_LT, TOKEN_GT, TOKEN_LTE, TOKEN_GTE,
		TOKEN_AND, TOKEN_OR, TOKEN_BITAND, TOKEN_BITOR, TOKEN_BITXOR,
		TOKEN_COMMA, TOKEN_COLON, TOKEN_DOT,
		TOKEN_LET, TOKEN_CONST, TOKEN_RETURN, TOKEN_IF, TOKEN_ELSE,
		TOKEN_FOR, TOKEN_WHILE, TOKEN_FUNCTION, TOKEN_TRY, TOKEN_CATCH,
		TOKEN_SWITCH, TOKEN_CASE, TOKEN_THROW, TOKEN_IMPORT, TOKEN_EXPORT:
		return true
	}

	_, errs := replParseProgram(input)
	for _, err := range errs {
		lower := strings.ToLower(err)
		if strings.Contains(lower, "got eof") || strings.Contains(lower, "got <eof>") || strings.Contains(lower, "unexpected token eof") {
			return true
		}
	}

	return false
}

func evalReplInput(input string, env *Environment) {
	replEvalSource(input, env, "<repl>", true)
}

func replEvalSource(input string, env *Environment, sourcePath string, printResult bool) {
	l := NewLexer(input)
	p := NewParser(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		for _, msg := range p.Errors() {
			fmt.Println(paint(msg, colorRed))
		}
		return
	}

	prevModuleDir := ""
	prevSourcePath := ""
	if env != nil {
		prevModuleDir = env.moduleDir
		prevSourcePath = env.sourcePath
		if sourcePath != "" {
			env.sourcePath = sourcePath
			if sourcePath != "<repl>" && sourcePath != "<memory>" {
				env.moduleDir = filepath.Dir(sourcePath)
			}
		}
		defer func() {
			env.moduleDir = prevModuleDir
			env.sourcePath = prevSourcePath
		}()
	}

	evaluated := runProgramSandboxed(program, env, env.securityPolicy)
	if evaluated != nil {
		if isError(evaluated) {
			fmt.Println(formatRuntimeErrorForDisplay(evaluated, input))
			return
		}
		if printResult && evaluated.Type() != NULL_OBJ {
			fmt.Println(formatObjectForDisplay(evaluated))
		}
	}
}

func replPrintLine(s string) {
	// Interactive mode uses raw terminal input; force carriage return so each
	// printed line starts at column 0 instead of current cursor column.
	fmt.Print("\r")
	fmt.Println(s)
}

func replPrintBlock(s string) {
	for _, line := range strings.Split(s, "\n") {
		replPrintLine(line)
	}
}

func replCandidatesForEnv(env *Environment) []string {
	kw := []string{
		"let", "if", "else", "while", "for", "in", "break", "continue", "function", "return",
		"print", "const", "import", "export", "true", "false", "null", "do", "typeof",
		"try", "catch", "throw", "switch", "case", "default",
		"exit", ":help", ":builtins", ":search", ":history", ":clear",
		":vars", ":type", ":doc", ":methods", ":fields", ":ast", ":time", ":load", ":reload", ":reset",
		":debug", ":mem", ":install",
		":config",
	}
	all := make(map[string]struct{}, len(builtins)+len(kw)+16)
	for name := range builtins {
		all[name] = struct{}{}
	}
	for _, k := range kw {
		all[k] = struct{}{}
	}
	if env != nil {
		for name := range env.store {
			all[name] = struct{}{}
		}
	}
	out := make([]string, 0, len(all))
	for k := range all {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func replParseProgram(input string) (*Program, []string) {
	l := NewLexer(input)
	p := NewParser(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		return nil, append([]string(nil), p.Errors()...)
	}
	return program, nil
}

func replEvalExpression(input string, env *Environment) (Object, []string) {
	program, errs := replParseProgram(input)
	if len(errs) != 0 {
		return nil, errs
	}
	prevModuleDir := ""
	prevSourcePath := ""
	if env != nil {
		prevModuleDir = env.moduleDir
		prevSourcePath = env.sourcePath
		env.sourcePath = "<repl>"
		defer func() {
			env.moduleDir = prevModuleDir
			env.sourcePath = prevSourcePath
		}()
	}
	return runProgramSandboxed(program, env, env.securityPolicy), nil
}

func replPrintParserErrors(errs []string) {
	for _, msg := range errs {
		replPrintBlock(paint(msg, colorRed))
	}
}

func replResolvedPath(path string, env *Environment) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("path is required")
	}
	return resolveImportPath(trimmed, env)
}

func replDocText(name string, env *Environment) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "usage: :doc <builtin|identifier|expression>"
	}
	if hasBuiltin(name) {
		return builtinHelpText(name)
	}
	if env != nil {
		if val, ok := env.Get(name); ok {
			return fmt.Sprintf("%s: %s\n%s", name, val.Type(), formatObjectPlain(val))
		}
	}
	if result, errs := replEvalExpression(name, env); len(errs) == 0 && result != nil {
		return fmt.Sprintf("%s\nType: %s\nValue:\n%s", name, result.Type(), formatObjectPlain(result))
	}
	return fmt.Sprintf("no documentation available for %q", name)
}

func replObjectMethods(obj Object) []string {
	if obj == nil {
		return nil
	}
	switch v := obj.(type) {
	case *OwnedValue:
		return replObjectMethods(v.inner)
	case *ImmutableValue:
		return replObjectMethods(v.inner)
	case *GeneratorValue:
		return replObjectMethods(&Array{Elements: v.elements})
	case *Hash:
		return []string{"entries", "keys", "length", "values"}
	case *String:
		return []string{"at", "camel_case", "charAt", "count_substr", "ends_with", "endsWith", "includes", "index_of", "indexOf", "kebab_case", "length", "lower", "pad_left", "pad_right", "padEnd", "padStart", "pascal_case", "regex_match", "regex_replace", "repeat", "replace", "slug", "snake_case", "split", "split_lines", "starts_with", "startsWith", "substring", "swap_case", "title", "toLowerCase", "toUpperCase", "trim", "trim_prefix", "trim_suffix", "truncate", "upper"}
	case *Integer:
		return []string{"abs", "is_even", "isEven", "is_odd", "isOdd", "pow", "sqrt", "to_float", "to_string", "toFloat", "toString"}
	case *Float:
		return []string{"abs", "ceil", "floor", "round", "to_int", "to_string", "toInt", "toString"}
	case *Array:
		return []string{"at", "every", "filter", "find", "flat", "flatMap", "forEach", "includes", "indexOf", "join", "length", "map", "pop", "push", "reduce", "reverse", "shift", "slice", "some", "sort", "unshift"}
	case *SPLServer:
		return []string{"addr", "routes", "running"}
	case *SPLRequest:
		return []string{"body", "get_header", "headers", "json", "method", "param", "params", "path", "query"}
	case *SPLResponse:
		return []string{"file", "header", "html", "json", "redirect", "render", "render_ssr", "send", "sse", "status", "stream", "text"}
	case *SSEWriter:
		return []string{"close", "send"}
	case *Signal:
		return []string{"get", "name", "set", "subscribe", "value"}
	case *Computed:
		return []string{"get", "value"}
	case *Effect:
		return []string{"dispose"}
	default:
		return nil
	}
}

func replObjectFields(obj Object) []string {
	if obj == nil {
		return nil
	}
	switch v := obj.(type) {
	case *OwnedValue:
		return replObjectFields(v.inner)
	case *ImmutableValue:
		return replObjectFields(v.inner)
	case *GeneratorValue:
		return replObjectFields(&Array{Elements: v.elements})
	case *Hash:
		fields := make([]string, 0, len(v.Pairs))
		for _, pair := range v.Pairs {
			fields = append(fields, pair.Key.Inspect())
		}
		sort.Strings(fields)
		return fields
	case *String:
		return []string{"length"}
	case *Array:
		return []string{"length"}
	case *SPLServer:
		return []string{"addr", "routes", "running"}
	case *SPLRequest:
		return []string{"body", "headers", "method", "params", "path", "query"}
	case *Signal:
		return []string{"name", "value"}
	case *Computed:
		return []string{"value"}
	default:
		return nil
	}
}

func replDescribeObjectList(title string, items []string) string {
	if len(items) == 0 {
		return title + ": none"
	}
	return title + ":\n- " + strings.Join(items, "\n- ")
}

func handleReplMetaCommand(line string, editor *replEditor, env *Environment) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if trimmed == ":help" {
		replPrintLine(paint("Interactive features:", colorBold+colorCyan))
		replPrintLine("- Arrow keys: history and cursor movement")
		replPrintLine("- Tab: semantic completion for names/methods/fields")
		replPrintLine("- Inline suggestion: gray suffix preview")
		replPrintLine("- Call tips: signatures/docs shown while typing calls")
		replPrintLine("- Ctrl+R: reverse history search")
		replPrintLine("- Ctrl+C: clear current line")
		replPrintLine("- Ctrl+D: exit when line is empty")
		replPrintLine(paint("Commands:", colorBold+colorCyan))
		replPrintLine("- :builtins   list all available builtins")
		replPrintLine("- :search X   search builtins/keywords by text")
		replPrintLine("- :history    print command history")
		replPrintLine("- :clear      clear screen")
		replPrintLine("- :vars       list all variables in current environment")
		replPrintLine("- :type <expr> show the type of an expression")
		replPrintLine("- :doc <name> show builtin/object documentation")
		replPrintLine("- :methods <expr> list methods available on a value")
		replPrintLine("- :fields <expr> list fields available on a value")
		replPrintLine("- :ast <expr> print parsed AST representation")
		replPrintLine("- :time <expr> evaluate and show execution time")
		replPrintLine("- :debug <expr> step through statements")
		replPrintLine("- :mem        show runtime memory usage")
		replPrintLine("- :load <file> load and execute a script file")
		replPrintLine("- :reload [file] clear module cache or one module")
		replPrintLine("- :install <alias> <path> add dependency to spl.mod and refresh lock")
		replPrintLine("- :config <file> [format] load config with secret masking")
		replPrintLine("- !<cmd>      execute shell command")
		replPrintLine("- :reset      reset the environment")
		return true
	}
	if strings.HasPrefix(trimmed, ":config ") {
		args := strings.Fields(strings.TrimSpace(strings.TrimPrefix(trimmed, ":config ")))
		if len(args) < 1 || len(args) > 2 {
			replPrintLine("usage: :config <file> [json|yaml|env]")
			return true
		}
		format := ""
		if len(args) == 2 {
			format = args[1]
		}
		obj, err := loadConfigObjectFromPath(args[0], format)
		if err != nil {
			replPrintLine("config error: " + err.Error())
			return true
		}
		if env != nil {
			env.Set("CONFIG", obj)
		}
		replPrintLine("CONFIG loaded")
		return true
	}
	if strings.HasPrefix(trimmed, "!") {
		cmdText := strings.TrimSpace(strings.TrimPrefix(trimmed, "!"))
		if cmdText == "" {
			replPrintLine("usage: !<shell command>")
			return true
		}
		out, err := replRunShellCommand(cmdText)
		if strings.TrimSpace(out) != "" {
			replPrintBlock(strings.TrimRight(out, "\n"))
		}
		if err != nil {
			replPrintLine("shell error: " + err.Error())
		}
		return true
	}
	if trimmed == ":builtins" {
		names := make([]string, 0, len(builtins))
		for name := range builtins {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			replPrintLine(name)
		}
		return true
	}
	if trimmed == ":history" {
		if editor == nil {
			replPrintLine("history is only available in interactive mode")
			return true
		}
		for i, h := range editor.history {
			replPrintLine(fmt.Sprintf("%4d  %s", i+1, h))
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
			replPrintLine("usage: :search <text>")
			return true
		}
		candidates := replCandidatesForEnv(env)
		found := 0
		for _, c := range candidates {
			if strings.Contains(strings.ToLower(c), strings.ToLower(query)) {
				replPrintLine(c)
				found++
			}
		}
		if found == 0 {
			replPrintLine("no matches found")
		}
		return true
	}
	if trimmed == ":vars" {
		if env == nil {
			replPrintLine("environment not available")
			return true
		}
		names := make([]string, 0, len(env.store))
		for name := range env.store {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			val := env.store[name]
			replPrintLine(fmt.Sprintf("  %s = %s", name, formatObjectPlain(val)))
		}
		if len(names) == 0 {
			replPrintLine("no variables defined")
		}
		return true
	}
	if trimmed == ":reset" {
		if env != nil {
			env.store = make(map[string]Object)
		}
		replPrintLine("environment reset")
		return true
	}
	if strings.HasPrefix(trimmed, ":type ") {
		expr := strings.TrimSpace(strings.TrimPrefix(trimmed, ":type "))
		if expr == "" {
			replPrintLine("usage: :type <expr>")
			return true
		}
		if env != nil {
			result, errs := replEvalExpression(expr, env)
			if len(errs) != 0 {
				replPrintParserErrors(errs)
			} else if result != nil {
				replPrintLine(result.Type().String())
			}
		}
		return true
	}
	if strings.HasPrefix(trimmed, ":doc ") {
		target := strings.TrimSpace(strings.TrimPrefix(trimmed, ":doc "))
		replPrintBlock(replDocText(target, env))
		return true
	}
	if strings.HasPrefix(trimmed, ":methods ") {
		expr := strings.TrimSpace(strings.TrimPrefix(trimmed, ":methods "))
		if expr == "" {
			replPrintLine("usage: :methods <expr>")
			return true
		}
		result, errs := replEvalExpression(expr, env)
		if len(errs) != 0 {
			replPrintParserErrors(errs)
			return true
		}
		replPrintBlock(replDescribeObjectList("methods", replObjectMethods(result)))
		return true
	}
	if strings.HasPrefix(trimmed, ":fields ") {
		expr := strings.TrimSpace(strings.TrimPrefix(trimmed, ":fields "))
		if expr == "" {
			replPrintLine("usage: :fields <expr>")
			return true
		}
		result, errs := replEvalExpression(expr, env)
		if len(errs) != 0 {
			replPrintParserErrors(errs)
			return true
		}
		replPrintBlock(replDescribeObjectList("fields", replObjectFields(result)))
		return true
	}
	if strings.HasPrefix(trimmed, ":ast ") {
		expr := strings.TrimSpace(strings.TrimPrefix(trimmed, ":ast "))
		if expr == "" {
			replPrintLine("usage: :ast <expr>")
			return true
		}
		program, errs := replParseProgram(expr)
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
			replPrintLine("usage: :time <expr>")
			return true
		}
		if env != nil {
			start := time.Now()
			result, errs := replEvalExpression(expr, env)
			if len(errs) != 0 {
				replPrintParserErrors(errs)
			} else {
				elapsed := time.Since(start)
				if result != nil && !isError(result) && result.Type() != NULL_OBJ {
					replPrintLine(formatObjectForDisplay(result))
				}
				replPrintLine(paint(fmt.Sprintf("elapsed: %s", elapsed), colorGray))
			}
		}
		return true
	}
	if strings.HasPrefix(trimmed, ":debug ") {
		expr := strings.TrimSpace(strings.TrimPrefix(trimmed, ":debug "))
		if expr == "" {
			replPrintLine("usage: :debug <expr>")
			return true
		}
		replDebugExpression(expr, env)
		return true
	}
	if trimmed == ":mem" {
		replPrintLine(replMemoryUsage())
		return true
	}
	if strings.HasPrefix(trimmed, ":load ") {
		path := strings.TrimSpace(strings.TrimPrefix(trimmed, ":load "))
		resolved, err := replResolvedPath(path, env)
		if err != nil {
			replPrintLine("load error: " + err.Error())
			return true
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			replPrintLine("load error: " + err.Error())
			return true
		}
		replEvalSource(string(data), env, resolved, true)
		return true
	}
	if strings.HasPrefix(trimmed, ":reload") {
		arg := strings.TrimSpace(strings.TrimPrefix(trimmed, ":reload"))
		if env == nil {
			replPrintLine("environment not available")
			return true
		}
		if arg == "" {
			env.moduleCache = make(map[string]ModuleCacheEntry)
			replPrintLine("module cache cleared")
			return true
		}
		resolved, err := replResolvedPath(arg, env)
		if err != nil {
			replPrintLine("reload error: " + err.Error())
			return true
		}
		delete(env.moduleCacheMap(), resolved)
		dispatchHotReloadHooks(resolved)
		replPrintLine("reloaded: " + resolved)
		return true
	}
	if strings.HasPrefix(trimmed, ":install ") {
		args := strings.Fields(strings.TrimSpace(strings.TrimPrefix(trimmed, ":install ")))
		if len(args) != 2 {
			replPrintLine("usage: :install <alias> <path>")
			return true
		}
		if err := replInstallDependency(args[0], args[1], env); err != nil {
			replPrintLine("install error: " + err.Error())
		} else {
			replPrintLine(fmt.Sprintf("installed %s => %s", args[0], args[1]))
		}
		return true
	}
	return false
}

func replCandidates() []string {
	return replCandidatesForEnv(nil)
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func newReplEditor(in, out *os.File, candidates []string, env *Environment) (*replEditor, error) {
	fd := int(in.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}

	editor := &replEditor{
		in:         in,
		out:        out,
		fd:         fd,
		oldState:   state,
		env:        env,
		history:    make([]string, 0, 256),
		historyPos: 0,
		candidates: candidates,
	}

	if historyFile, err := replHistoryPath(); err == nil {
		editor.historyFile = historyFile
		if loaded, err := loadHistoryEntries(historyFile); err == nil {
			editor.history = append(editor.history, loaded...)
		}
		editor.historyBase = len(editor.history)
	}

	return editor, nil
}

func (e *replEditor) close() {
	if e.historyFile != "" {
		_ = appendHistoryEntries(e.historyFile, historyEntriesToPersist(e.history, e.historyBase))
	}
	if e.oldState != nil {
		_ = term.Restore(e.fd, e.oldState)
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

func loadHistoryEntries(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return parseHistoryData(data), nil
}

func parseHistoryData(data []byte) []string {
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

func historyEntriesToPersist(history []string, from int) []string {
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

func appendHistoryEntries(path string, entries []string) error {
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

func (e *replEditor) readLine(prompt string) (string, error) {
	buf := make([]rune, 0, 128)
	cursor := 0
	e.historyPos = len(e.history)
	render := func() {
		_, _ = fmt.Fprint(e.out, "\r\033[2K")
		line := string(buf)
		styledPrompt := stylePrompt(prompt)
		if strings.HasPrefix(prompt, "..") {
			styledPrompt = styleContinuationPrompt(prompt)
		}
		_, _ = fmt.Fprint(e.out, styledPrompt, colorizeInputLine(line))
		if cursor == len(buf) {
			ctx := completionContext(buf, cursor)
			if ctx.ok && ctx.prefix != "" {
				if suggestion, ok := firstCompletion(ctx.prefix, e.completionsForContext(ctx)); ok && suggestion != ctx.prefix {
					suffix := suggestion[len(ctx.prefix):]
					_, _ = fmt.Fprint(e.out, paint(suffix, colorGray))
				}
			}
			if tip := replCallTip(line, cursor, e.env); tip != "" {
				_, _ = fmt.Fprint(e.out, paint("  "+tip, colorGray))
			}
		}
		_, _ = fmt.Fprint(e.out, "\r")
		_, _ = fmt.Fprintf(e.out, "\033[%dC", len([]rune(prompt))+cursor)
	}

	render()
	var b [1]byte
	for {
		_, err := e.in.Read(b[:])
		if err != nil {
			return "", err
		}
		ch := b[0]
		switch ch {
		case '\r', '\n':
			_, _ = fmt.Fprint(e.out, "\r\n")
			line := string(buf)
			if strings.TrimSpace(line) != "" {
				if len(e.history) == 0 || e.history[len(e.history)-1] != line {
					e.history = append(e.history, line)
				}
			}
			return line, nil
		case 3: // Ctrl+C
			buf = buf[:0]
			cursor = 0
			_, _ = fmt.Fprint(e.out, "^C\r\n")
			return "", nil
		case 4: // Ctrl+D
			if len(buf) == 0 {
				_, _ = fmt.Fprint(e.out, "\r\n")
				return "", io.EOF
			}
		case 1: // Ctrl+A
			cursor = 0
		case 5: // Ctrl+E
			cursor = len(buf)
		case 18: // Ctrl+R
			buf, cursor = e.reverseHistorySearch(buf)
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
				if len(e.history) > 0 && e.historyPos > 0 {
					e.historyPos--
					buf = []rune(e.history[e.historyPos])
					cursor = len(buf)
				}
			case keyDown:
				if len(e.history) > 0 && e.historyPos < len(e.history)-1 {
					e.historyPos++
					buf = []rune(e.history[e.historyPos])
					cursor = len(buf)
				} else if e.historyPos >= len(e.history)-1 {
					e.historyPos = len(e.history)
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

func (e *replEditor) reverseHistorySearch(current []rune) ([]rune, int) {
	query := strings.TrimSpace(string(current))
	if len(e.history) == 0 {
		return current, len(current)
	}
	start := e.historyPos
	if start > len(e.history)-1 {
		start = len(e.history) - 1
	}
	for i := start; i >= 0; i-- {
		entry := e.history[i]
		if query == "" || strings.Contains(entry, query) {
			e.historyPos = i
			r := []rune(entry)
			return r, len(r)
		}
	}
	_, _ = fmt.Fprint(e.out, "\a")
	return current, len(current)
}

func (e *replEditor) readEscapeAction() keyAction {
	var b [1]byte
	if _, err := e.in.Read(b[:]); err != nil {
		return keyUnknown
	}

	switch b[0] {
	case '[':
		return e.readCSIAction()
	case 'O':
		// SS3 sequences: arrows/home/end in many terminals.
		if _, err := e.in.Read(b[:]); err != nil {
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

func (e *replEditor) readCSIAction() keyAction {
	var b [1]byte
	if _, err := e.in.Read(b[:]); err != nil {
		return keyUnknown
	}

	// Simple one-byte CSI forms.
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

	// Extended forms: e.g. [1~, [4~, [7~, [8~, [3~, [1;5C
	seq := []byte{b[0]}
	for {
		if _, err := e.in.Read(b[:]); err != nil {
			break
		}
		seq = append(seq, b[0])
		// Final CSI byte is usually letter or '~'.
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

func (e *replEditor) applyCompletion(buf []rune, cursor int, prompt string) ([]rune, int) {
	ctx := completionContext(buf, cursor)
	if !ctx.ok || ctx.prefix == "" {
		return buf, cursor
	}
	matches := findCompletions(ctx.prefix, e.completionsForContext(ctx))
	if len(matches) == 0 {
		return buf, cursor
	}
	if len(matches) == 1 {
		completion := []rune(matches[0])
		newBuf := append([]rune{}, buf[:ctx.start]...)
		newBuf = append(newBuf, completion...)
		newBuf = append(newBuf, buf[ctx.end:]...)
		return newBuf, ctx.start + len(completion)
	}

	common := longestCommonPrefix(matches)
	if len(common) > len(ctx.prefix) {
		completion := []rune(common)
		newBuf := append([]rune{}, buf[:ctx.start]...)
		newBuf = append(newBuf, completion...)
		newBuf = append(newBuf, buf[ctx.end:]...)
		return newBuf, ctx.start + len(completion)
	}

	_, _ = fmt.Fprint(e.out, "\r\n")
	for _, m := range matches {
		_, _ = fmt.Fprintln(e.out, m)
	}
	if strings.HasPrefix(prompt, "..") {
		_, _ = fmt.Fprint(e.out, styleContinuationPrompt(prompt))
	} else {
		_, _ = fmt.Fprint(e.out, stylePrompt(prompt))
	}
	return buf, cursor
}

type replCompletionContext struct {
	prefix   string
	baseExpr string
	start    int
	end      int
	ok       bool
}

func completionContext(buf []rune, cursor int) replCompletionContext {
	prefix, start, end, ok := currentToken(buf, cursor)
	if !ok {
		return replCompletionContext{}
	}
	ctx := replCompletionContext{prefix: prefix, start: start, end: end, ok: true}
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
		ctx.baseExpr = base
	}
	return ctx
}

func (e *replEditor) completionsForContext(ctx replCompletionContext) []string {
	if ctx.baseExpr == "" || e.env == nil {
		return e.candidates
	}
	obj, errs := replEvalExpression(ctx.baseExpr, e.env)
	if len(errs) != 0 || obj == nil || isError(obj) {
		return e.candidates
	}
	fields := replObjectFields(obj)
	methods := replObjectMethods(obj)
	out := make([]string, 0, len(fields)+len(methods))
	out = append(out, fields...)
	out = append(out, methods...)
	sort.Strings(out)
	if len(out) == 0 {
		return e.candidates
	}
	return out
}

func replCallTip(line string, cursor int, env *Environment) string {
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
	if hasBuiltin(name) {
		return builtinHelpText(name)
	}
	if env != nil {
		if val, ok := env.Get(name); ok {
			if fn, ok := val.(*Function); ok {
				return fn.Inspect()
			}
			return fmt.Sprintf("%s: %s", name, val.Type())
		}
	}
	return ""
}

func currentToken(buf []rune, cursor int) (prefix string, start int, end int, ok bool) {
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

func findCompletions(prefix string, candidates []string) []string {
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

func longestCommonPrefix(items []string) string {
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

func replMemoryUsage() string {
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

func replInstallDependency(alias, source string, env *Environment) error {
	alias = strings.TrimSpace(alias)
	source = strings.TrimSpace(source)
	if alias == "" || source == "" {
		return fmt.Errorf("alias and source path are required")
	}
	startDir := ""
	if env != nil {
		startDir = env.moduleDir
		if startDir == "" {
			startDir = env.sourcePath
		}
	}
	projectDir := discoverProjectRoot(startDir)
	manifestPath := filepath.Join(projectDir, SPLManifestFileName)
	manifest := &SPLModuleManifest{Module: defaultModuleName(projectDir), Dependencies: map[string]string{}}
	if _, err := os.Stat(manifestPath); err == nil {
		loaded, readErr := readModuleManifestFromFile(manifestPath)
		if readErr != nil {
			return readErr
		}
		manifest = loaded
	}
	if manifest.Dependencies == nil {
		manifest.Dependencies = map[string]string{}
	}
	manifest.Dependencies[alias] = source
	if err := writeModuleManifest(projectDir, manifest); err != nil {
		return err
	}
	_, err := SyncModuleLock(projectDir)
	return err
}

func replDebugExpression(input string, env *Environment) {
	program, errs := replParseProgram(input)
	if len(errs) != 0 {
		replPrintParserErrors(errs)
		return
	}
	stmts := program.Statements
	if len(stmts) == 0 {
		replPrintLine("debug: no statements")
		return
	}
	replPrintLine("debug mode: commands = step|next, continue|c, locals|vars, break <n>, quit")
	idx := 0
	breakpoints := map[int]struct{}{}
	for idx < len(stmts) {
		replPrintLine(fmt.Sprintf("[%d/%d] %s", idx+1, len(stmts), strings.TrimSpace(stmts[idx].String())))
		cmd, err := replReadDebugCommand()
		if err != nil {
			replPrintLine("debug error: " + err.Error())
			return
		}
		switch {
		case cmd == "", cmd == "step", cmd == "next", cmd == "n", cmd == "s":
			obj := Eval(stmts[idx], env)
			if obj != nil && obj.Type() != NULL_OBJ {
				if isError(obj) {
					replPrintBlock(formatRuntimeErrorForDisplay(obj, input))
					return
				}
				replPrintLine(formatObjectForDisplay(obj))
			}
			idx++
		case cmd == "locals", cmd == "vars":
			names := make([]string, 0, len(env.store))
			for n := range env.store {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				replPrintLine(fmt.Sprintf("  %s = %s", n, formatObjectPlain(env.store[n])))
			}
		case strings.HasPrefix(cmd, "break "):
			arg := strings.TrimSpace(strings.TrimPrefix(cmd, "break "))
			lineNo := 0
			_, scanErr := fmt.Sscanf(arg, "%d", &lineNo)
			if scanErr != nil || lineNo < 1 || lineNo > len(stmts) {
				replPrintLine("usage: break <statement-index>")
				continue
			}
			breakpoints[lineNo-1] = struct{}{}
			replPrintLine(fmt.Sprintf("breakpoint set at %d", lineNo))
		case cmd == "continue", cmd == "c":
			for idx < len(stmts) {
				if _, ok := breakpoints[idx]; ok {
					replPrintLine(fmt.Sprintf("hit breakpoint at %d", idx+1))
					break
				}
				obj := Eval(stmts[idx], env)
				if obj != nil && isError(obj) {
					replPrintBlock(formatRuntimeErrorForDisplay(obj, input))
					return
				}
				idx++
			}
		case cmd == "quit", cmd == "q", cmd == "exit":
			replPrintLine("debug aborted")
			return
		default:
			replPrintLine("unknown debug command")
		}
	}
	replPrintLine("debug finished")
}

func formatRuntimeErrorForDisplay(obj Object, source string) string {
	errObj, ok := obj.(*Error)
	if !ok || errObj == nil {
		return paint("ERROR: "+objectErrorString(obj), colorBold+colorRed)
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
		if ctx := lineContext(source, errObj.Line, errObj.Column); ctx != "" {
			out.WriteString("\n")
			out.WriteString(ctx)
		}
	}
	if trace := formatCallStack(errObj.Stack); trace != "" {
		out.WriteString("\n")
		out.WriteString(trace)
	}
	return paint(out.String(), colorBold+colorRed)
}

func replReadDebugCommand() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print(styleContinuationPrompt("dbg> "))
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
