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
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/pkgmgr"
	sessionpkg "github.com/oarkflow/interpreter/pkg/session"
	"github.com/oarkflow/interpreter/pkg/tooling"
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

type NativeCommandResult struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	OK        bool
	TimedOut  bool
	Cancelled bool
	Truncated bool
	Error     string
}

var RunNativeCommandLineFn func(env *object.Environment, command string, args []string) NativeCommandResult

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
	Env         *object.Environment
	History     []string
	HistoryPos  int
	HistoryFile string
	HistoryBase int
	Candidates  []string
	Index       *tooling.WorkspaceIndex
	SourcePath  string
	Source      string
}

const replHistoryFileName = ".interpreter_repl_history"

type replCommandInfo struct {
	Name     string
	Usage    string
	Summary  string
	Category string
}

var replCommandCatalog = []replCommandInfo{
	{":help", ":help", "show guided help and keyboard shortcuts", "discover"},
	{":tips", ":tips", "show practical REPL tips", "discover"},
	{":commands", ":commands [query]", "list commands with descriptions", "discover"},
	{":palette", ":palette <query>", "search commands, builtins, variables, and workspace symbols", "discover"},
	{":examples", ":examples", "show runnable SPL example scripts", "discover"},
	{":hint", ":hint <expr|name>", "show hover-style documentation for code", "discover"},
	{":builtins", ":builtins", "list available builtin functions", "language"},
	{":search", ":search <text>", "search builtins and keywords", "language"},
	{":doc", ":doc <name>", "show builtin, symbol, or object documentation", "language"},
	{":type", ":type <expr>", "evaluate an expression and show its SPL type", "language"},
	{":methods", ":methods <expr>", "list methods available on a value", "language"},
	{":fields", ":fields <expr>", "list fields available on a value", "language"},
	{":ast", ":ast <expr>", "print parsed AST representation", "language"},
	{":diagnostics", ":diagnostics [source]", "show parser and static diagnostics", "tooling"},
	{":symbols", ":symbols [query]", "list REPL and workspace symbols", "tooling"},
	{":def", ":def <name>", "jump-style definition lookup", "tooling"},
	{":refs", ":refs <name>", "find references for a symbol", "tooling"},
	{":format", ":format [source]", "format SPL source", "tooling"},
	{":history", ":history", "print command history", "session"},
	{":vars", ":vars", "list variables in the current environment", "session"},
	{":checkpoint", ":checkpoint [name]", "save a session checkpoint", "session"},
	{":restore", ":restore <name>", "restore a session checkpoint", "session"},
	{":replay", ":replay", "replay recorded session inputs", "session"},
	{":inspect", ":inspect [name]", "inspect session state or a variable", "session"},
	{":metrics", ":metrics", "show recent execution metrics", "observability"},
	{":events", ":events", "show recent session events", "observability"},
	{":trace", ":trace on|off", "toggle trace display", "observability"},
	{":time", ":time <expr>", "evaluate and show elapsed time", "debug"},
	{":debug", ":debug <expr>", "step through statements", "debug"},
	{":mem", ":mem", "show Go runtime memory usage", "debug"},
	{":tasks", ":tasks", "list background tasks when available", "runtime"},
	{":cancel", ":cancel <id>", "cancel a task when available", "runtime"},
	{":load", ":load <file>", "load and execute an SPL script", "workspace"},
	{":reload", ":reload [file]", "clear module cache or reload a module", "workspace"},
	{":install", ":install <alias> <path>", "add dependency to spl.mod and refresh lock", "workspace"},
	{":tools", ":tools", "show daily tech chore modules and preview-first tool examples", "discover"},
	{":config", ":config ...", "view or update REPL runtime/security config", "runtime"},
	{":ask", ":ask <prompt>", "ask configured assistant provider", "assistant"},
	{":explain", ":explain <code|error>", "explain code or an error with configured assistant", "assistant"},
	{":fix", ":fix <code|error>", "ask configured assistant for a fix", "assistant"},
	{":clear", ":clear", "clear the terminal", "terminal"},
	{":reset", ":reset", "reset environment and REPL source context", "session"},
}

type ReplConfig struct {
	ExecutionProfile       string
	ModuleDir              string
	StrictMode             bool
	ProtectHost            bool
	AllowEnvWrite          bool
	AllowedCapabilities    []string
	DeniedCapabilities     []string
	AllowedExecCommands    []string
	AllowedNativeModules   []string
	DeniedNativeModules    []string
	AllowedNetworkHosts    []string
	AllowedDBDrivers       []string
	AllowedDBDSNPatterns   []string
	AllowedFileReadPaths   []string
	AllowedFileWritePaths  []string
	MaxDepth               int
	MaxSteps               int64
	MaxHeapMB              int64
	TimeoutMS              int64
	MaxOutputBytes         int64
	MaxHTTPBodyBytes       int64
	MaxExecOutputBytes     int64
	RenderMode             string
	RenderTerminalProtocol string
	RenderMaxBytes         int64
	RenderAllowURLs        bool
	RenderAllowURLHosts    []string
}

var replConfigs = struct {
	mu    sync.Mutex
	items map[string]*ReplConfig
}{items: make(map[string]*ReplConfig)}

var replSessions = struct {
	mu    sync.Mutex
	items map[*object.Environment]*sessionpkg.Session
}{items: make(map[*object.Environment]*sessionpkg.Session)}

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
	keyWordLeft
	keyWordRight
)

// ---------------------------------------------------------------------------
// Public entry points
// ---------------------------------------------------------------------------

// RunReplInteractive starts the interactive REPL with line editing.
func RunReplInteractive(env *object.Environment) error {
	if !IsTerminal(os.Stdin) {
		return fmt.Errorf("stdin is not a terminal")
	}
	restoreTerminal, err := enableRawTerminal(os.Stdin)
	if err == nil && restoreTerminal != nil {
		defer restoreTerminal()
	}

	editor, err := newReplEditor(os.Stdin, os.Stdout, ReplCandidatesForEnv(env), env)
	if err != nil {
		return err
	}
	defer editor.close()
	PrintWelcome(env)

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
		editor.RecordInput(input)
	}
}

func enableRawTerminal(in *os.File) (func(), error) {
	if in == nil || isWindowsRuntime() {
		return nil, nil
	}
	stateCmd := exec.Command("stty", "-g")
	stateCmd.Stdin = in
	stateBytes, err := stateCmd.Output()
	if err != nil {
		return nil, err
	}
	state := strings.TrimSpace(string(stateBytes))
	rawCmd := exec.Command("stty", "raw", "-echo")
	rawCmd.Stdin = in
	if err := rawCmd.Run(); err != nil {
		return nil, err
	}
	return func() {
		if state == "" {
			return
		}
		restoreCmd := exec.Command("stty", state)
		restoreCmd.Stdin = in
		_ = restoreCmd.Run()
	}, nil
}

// RunReplBasic starts a simple line-based REPL without raw terminal input.
func RunReplBasic(env *object.Environment) {
	PrintWelcome(env)
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
	sess := replSessionForEnv(env)
	if sess == nil {
		fmt.Println(Paint("error: session not available", ColorRed))
		return
	}
	res := sess.Execute(sessionpkg.ExecutionRequest{Source: input, Path: sourcePath, Print: printResult})
	if len(res.Diagnostics) != 0 {
		for _, msg := range res.Diagnostics {
			fmt.Println(Paint(msg, ColorRed))
		}
		return
	}
	if !res.OK {
		if handled := maybeRunNativeCommandLine(input, env, res.Result); handled {
			return
		}
		if res.Result != nil {
			fmt.Println(FormatRuntimeErrorForDisplay(res.Result, input))
		} else if res.Error != "" {
			fmt.Println(Paint(res.Error, ColorRed))
		}
		return
	}
	if printResult && res.Result != nil && res.Result.Type() != object.NULL_OBJ {
		fmt.Println(FormatObjectForDisplayWithEnv(res.Result, env))
	}
}

func maybeRunNativeCommandLine(input string, env *object.Environment, result object.Object) bool {
	if RunNativeCommandLineFn == nil || !nativeOSImportedForCommandLine(env) {
		return false
	}
	missing := missingIdentifierName(result)
	if missing == "" {
		return false
	}
	command, args, ok := parseReplNativeCommandLine(input)
	if !ok || command != missing {
		return false
	}
	res := RunNativeCommandLineFn(env, command, args)
	if res.Stdout != "" {
		fmt.Print(res.Stdout)
		if !strings.HasSuffix(res.Stdout, "\n") {
			fmt.Println()
		}
	}
	if res.Stderr != "" {
		fmt.Print(res.Stderr)
		if !strings.HasSuffix(res.Stderr, "\n") {
			fmt.Println()
		}
	}
	if !res.OK {
		msg := strings.TrimSpace(res.Error)
		if msg == "" {
			msg = fmt.Sprintf("command exited with status %d", res.ExitCode)
		}
		if res.TimedOut {
			msg = "command timed out"
		} else if res.Cancelled {
			msg = "command cancelled"
		}
		fmt.Println(Paint("ERROR: "+msg, ColorBold+ColorRed))
	}
	return true
}

func nativeOSImportedForCommandLine(env *object.Environment) bool {
	if env == nil {
		return false
	}
	if nativeOSExportsInEnv(env) {
		return true
	}
	obj, ok := env.Get("os")
	if !ok {
		return false
	}
	h, ok := obj.(*object.Hash)
	if !ok {
		return false
	}
	for _, name := range []string{"run", "which", "list", "platform", "capabilities"} {
		key := (&object.String{Value: name}).HashKey()
		pair, ok := h.Pairs[key]
		if !ok || pair.Value == nil || pair.Value.Type() != object.BUILTIN_OBJ {
			return false
		}
	}
	return true
}

func nativeOSExportsInEnv(env *object.Environment) bool {
	for _, name := range []string{"run", "which", "list", "platform", "capabilities"} {
		obj, ok := env.Get(name)
		if !ok || obj == nil || obj.Type() != object.BUILTIN_OBJ {
			return false
		}
	}
	return true
}

func missingIdentifierName(result object.Object) string {
	errObj, ok := result.(*object.Error)
	if !ok || errObj == nil {
		return ""
	}
	msg := strings.TrimSpace(errObj.Message)
	const prefix = "identifier not found:"
	if !strings.HasPrefix(strings.ToLower(msg), prefix) {
		return ""
	}
	return strings.TrimSpace(msg[len(prefix):])
}

func parseReplNativeCommandLine(input string) (string, []string, bool) {
	line := strings.TrimSpace(input)
	if line == "" || strings.Contains(line, "\n") || strings.HasPrefix(line, ":") {
		return "", nil, false
	}
	if strings.ContainsAny(line, ";{}[]()=<>|&") {
		return "", nil, false
	}
	parts, ok := splitReplCommandWords(line)
	if !ok || len(parts) == 0 || !isReplCommandName(parts[0]) {
		return "", nil, false
	}
	return parts[0], parts[1:], true
}

func splitReplCommandWords(line string) ([]string, bool) {
	words := []string{}
	var current strings.Builder
	var quote rune
	escaped := false
	for _, r := range line {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if unicode.IsSpace(r) {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteRune(r)
	}
	if escaped || quote != 0 {
		return nil, false
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words, true
}

func isReplCommandName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' || r == '/') {
			return false
		}
	}
	return true
}

func PrintWelcome(env *object.Environment) {
	profile := "trusted"
	if cfg := ReplConfigForEnv(env); cfg != nil && strings.TrimSpace(cfg.ExecutionProfile) != "" {
		profile = cfg.ExecutionProfile
	}
	ReplPrintLine(Paint("SPL Runtime Workspace", ColorBold+ColorCyan))
	ReplPrintLine(Paint("Type :tips for a quick tour, :commands to search commands, Tab for completion, Ctrl-R for history.", ColorGray))
	ReplPrintLine(Paint("Session profile: "+profile, ColorGray))
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

func replSessionForEnv(env *object.Environment) *sessionpkg.Session {
	if env == nil {
		return nil
	}
	replSessions.mu.Lock()
	defer replSessions.mu.Unlock()
	if sess := replSessions.items[env]; sess != nil {
		return sess
	}
	opts := sessionpkg.SessionOptions{
		ID:         sessionpkg.SessionID(fmt.Sprintf("repl-%p", env)),
		Profile:    "trusted",
		ModuleDir:  env.ModuleDir,
		SourcePath: env.SourcePath,
		Output:     os.Stdout,
		EventLimit: 256,
	}
	sess, err := sessionpkg.NewWithEnvironment(env, opts)
	if err != nil {
		ReplPrintLine("session error: " + err.Error())
		return nil
	}
	replSessions.items[env] = sess
	return sess
}

// ReplCandidatesForEnv returns completion candidates from builtins, keywords,
// and environment variables.
func ReplCandidatesForEnv(env *object.Environment) []string {
	kw := []string{
		"let", "if", "else", "while", "for", "in", "and", "or", "not", "break", "continue", "function", "return",
		"print", "const", "import", "export", "true", "false", "null", "do", "typeof",
		"try", "catch", "throw", "switch", "case", "default",
		"exit",
	}
	kw = append(kw, ReplCommandNames()...)
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

func ReplCommandNames() []string {
	names := make([]string, 0, len(replCommandCatalog)+4)
	for _, cmd := range replCommandCatalog {
		names = append(names, cmd.Name)
	}
	names = append(names, ":config list", ":config get", ":config set", ":config profile")
	sort.Strings(names)
	return names
}

func ReplCommandDetail(name string) string {
	for _, cmd := range replCommandCatalog {
		if cmd.Name == name || strings.HasPrefix(name, cmd.Name+" ") {
			return cmd.Summary
		}
	}
	return ""
}

func ReplCommandsTable(query string) string {
	query = strings.ToLower(strings.TrimSpace(query))
	var b strings.Builder
	b.WriteString("Command                  Category       Description\n")
	b.WriteString("-------                  --------       -----------\n")
	for _, cmd := range replCommandCatalog {
		if query != "" && !strings.Contains(strings.ToLower(cmd.Name+" "+cmd.Usage+" "+cmd.Summary+" "+cmd.Category), query) {
			continue
		}
		fmt.Fprintf(&b, "%-24s %-14s %s\n", cmd.Usage, cmd.Category, cmd.Summary)
	}
	return strings.TrimRight(b.String(), "\n")
}

func ReplTipsText() string {
	return strings.Join([]string{
		"Start with :examples, then :load <file> to run one.",
		"Press Tab after a partial name for completion; press Tab again to list matches.",
		"Type object. then Tab to see fields and methods for a runtime value.",
		"Type a builtin call like read_json( to see a signature hint.",
		"Daily chores live in tools/* modules; start with import \"tools/files\" and keep apply false until the preview plan looks right.",
		"Native host commands live in import \"native/os\"; for command-style lines, or import \"native/os\" as os; for os.run(...).",
		"Use :checkpoint name, :restore name, and :replay for session workflows.",
		"Use :diagnostics, :symbols, :def, :refs, and :format for editor-like tooling.",
		"Use :metrics and :events after running code to inspect runtime behavior.",
		"Use :palette text to search commands, builtins, variables, and workspace symbols together.",
	}, "\n")
}

func ReplExamplesText() string {
	examples := []string{
		"testdata/hello.spl",
		"testdata/examples_runtime_workspace.spl",
		"testdata/examples_session_replay.spl",
		"testdata/examples_json_csv_values.spl",
		"testdata/examples_native_os.spl",
		"testdata/examples_tools_files.spl",
		"testdata/examples_tools_media.spl",
		"testdata/examples_resource_limits.spl",
		"testdata/examples_streaming.spl",
		"testdata/pattern_matching.spl",
	}
	var b strings.Builder
	b.WriteString("Runnable examples:\n")
	for _, path := range examples {
		b.WriteString("  :load ")
		b.WriteString(path)
		b.WriteByte('\n')
	}
	b.WriteString("\nFrom the shell:\n")
	b.WriteString("  go run ./cmd/interpreter testdata/examples_json_csv_values.spl\n")
	b.WriteString("  go run ./cmd/interpreter testdata/examples_native_os.spl\n")
	b.WriteString("  go run ./cmd/interpreter-full testdata/examples_tools_media.spl\n")
	b.WriteString("  go run ./cmd/spltool session run --json --checkpoint baseline testdata/examples_runtime_workspace.spl")
	return b.String()
}

func ReplToolsText() string {
	return strings.Join([]string{
		"Daily tech chore modules:",
		"  tools/files    bulk_rename, file_search, file_organize, file_checksum, file_remove_plan",
		"  tools/archive  archive_compress, archive_extract, archive_list",
		"  tools/images   image_convert_batch, image_info_file, image_resize_file, image_thumbnail, image_crop_file",
		"  tools/office   office_text, office_read",
		"  tools/secrets  secret_generate, token_generate, file_encrypt, file_decrypt",
		"  tools/media    media_info, media_convert, ffmpeg_status, ffmpeg_install",
		"  native/os      run, which, list, platform, capabilities",
		"",
		"Preview-first examples:",
		"  import \"tools/files\";",
		"  bulk_rename(\"photos\", {\"match\": \"*.jpg\", \"template\": \"{date}_{seq}.{ext}\", \"apply\": false});",
		"  import \"tools/media\";",
		"  media_convert(\"input.mov\", \"output.mp4\", {\"install\": true, \"apply\": false});",
		"",
		"Native OS examples:",
		"  :config set security.allow_capabilities exec",
		"  :config set security.allow_exec go",
		"  import \"native/os\";",
		"  echo \"hello from the host\"",
		"  import \"native/os\" as os;",
		"  os.run(\"go\", [\"version\"]);",
	}, "\n")
}

func ReplPalette(query string, editor *ReplEditor, env *object.Environment) string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return "usage: :palette <query>"
	}
	type row struct {
		Kind   string
		Label  string
		Detail string
	}
	rows := []row{}
	add := func(kind, label, detail string) {
		haystack := strings.ToLower(kind + " " + label + " " + detail)
		if strings.Contains(haystack, query) || fuzzyContains(haystack, query) {
			rows = append(rows, row{Kind: kind, Label: label, Detail: detail})
		}
	}
	for _, cmd := range replCommandCatalog {
		add("command", cmd.Usage, cmd.Summary)
	}
	if BuiltinNames != nil {
		for name := range BuiltinNames() {
			detail := ""
			if BuiltinHelpTextFn != nil {
				detail = BuiltinHelpTextFn(name)
			}
			add("builtin", name, detail)
		}
	}
	if env != nil {
		for _, name := range env.Names() {
			if val, ok := env.Get(name); ok {
				add("variable", name, val.Type().String())
			}
		}
	}
	for _, sym := range replSymbols(editor) {
		add(sym.Kind, sym.Name, sym.Detail)
	}
	if len(rows) == 0 {
		return "no matches"
	}
	if len(rows) > 30 {
		rows = rows[:30]
	}
	var b strings.Builder
	for _, item := range rows {
		fmt.Fprintf(&b, "%-10s %-28s %s\n", item.Kind, item.Label, ReplCompactHint(item.Detail, 80))
	}
	return strings.TrimRight(b.String(), "\n")
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
		var policy *object.SecurityPolicy
		if env != nil {
			policy = env.SecurityPolicy
		}
		return RunProgramSandboxedFn(program, env, policy), nil
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

func ReplConfigForEnv(env *object.Environment) *ReplConfig {
	key := "global"
	if env != nil {
		key = env.EnsureOwnerID()
	}
	replConfigs.mu.Lock()
	defer replConfigs.mu.Unlock()
	if cfg := replConfigs.items[key]; cfg != nil {
		return cfg
	}
	cfg := defaultReplConfig(env)
	replConfigs.items[key] = cfg
	return cfg
}

func defaultReplConfig(env *object.Environment) *ReplConfig {
	moduleDir := "."
	if env != nil && strings.TrimSpace(env.ModuleDir) != "" {
		moduleDir = env.ModuleDir
	}
	cfg := &ReplConfig{
		ExecutionProfile:       "trusted",
		ModuleDir:              moduleDir,
		AllowEnvWrite:          true,
		MaxDepth:               256,
		MaxSteps:               2_000_000,
		MaxHeapMB:              256,
		TimeoutMS:              0,
		MaxOutputBytes:         1 << 20,
		MaxHTTPBodyBytes:       1 << 20,
		MaxExecOutputBytes:     1 << 20,
		RenderMode:             "auto",
		RenderTerminalProtocol: "auto",
		RenderMaxBytes:         1 << 20,
		AllowedCapabilities:    nil,
		DeniedCapabilities:     nil,
		AllowedFileReadPaths:   nil,
	}
	if env != nil {
		if p := env.SecurityPolicy; p != nil {
			cfg.StrictMode = p.StrictMode
			cfg.ProtectHost = p.ProtectHost
			cfg.AllowEnvWrite = p.AllowEnvWrite
			cfg.AllowedCapabilities = append([]string(nil), p.AllowedCapabilities...)
			cfg.DeniedCapabilities = append([]string(nil), p.DeniedCapabilities...)
			cfg.AllowedExecCommands = append([]string(nil), p.AllowedExecCommands...)
			cfg.AllowedNativeModules = append([]string(nil), p.AllowedNativeModules...)
			cfg.DeniedNativeModules = append([]string(nil), p.DeniedNativeModules...)
			cfg.AllowedNetworkHosts = append([]string(nil), p.AllowedNetworkHosts...)
			cfg.AllowedDBDrivers = append([]string(nil), p.AllowedDBDrivers...)
			cfg.AllowedDBDSNPatterns = append([]string(nil), p.AllowedDBDSNPatterns...)
			cfg.AllowedFileReadPaths = append([]string(nil), p.AllowedFileReadPaths...)
			cfg.AllowedFileWritePaths = append([]string(nil), p.AllowedFileWritePaths...)
		}
		if rl := env.RuntimeLimits; rl != nil {
			if rl.MaxDepth > 0 {
				cfg.MaxDepth = rl.MaxDepth
			}
			if rl.MaxSteps > 0 {
				cfg.MaxSteps = rl.MaxSteps
			}
			if rl.MaxHeapBytes > 0 {
				cfg.MaxHeapMB = int64(rl.MaxHeapBytes / 1024 / 1024)
			}
			if !rl.Deadline.IsZero() {
				cfg.TimeoutMS = int64(time.Until(rl.Deadline) / time.Millisecond)
				if cfg.TimeoutMS < 0 {
					cfg.TimeoutMS = 0
				}
			}
			if rl.MaxOutputBytes > 0 {
				cfg.MaxOutputBytes = rl.MaxOutputBytes
			}
			if rl.MaxHTTPBodyBytes > 0 {
				cfg.MaxHTTPBodyBytes = rl.MaxHTTPBodyBytes
			}
			if rl.MaxExecOutputBytes > 0 {
				cfg.MaxExecOutputBytes = rl.MaxExecOutputBytes
			}
		}
		if rc := env.RenderConfig; rc != nil {
			if strings.TrimSpace(rc.Mode) != "" {
				cfg.RenderMode = rc.Mode
			}
			if strings.TrimSpace(rc.TerminalProtocol) != "" {
				cfg.RenderTerminalProtocol = rc.TerminalProtocol
			}
			if rc.MaxBytes > 0 {
				cfg.RenderMaxBytes = rc.MaxBytes
			}
			cfg.RenderAllowURLs = rc.AllowURLs
			cfg.RenderAllowURLHosts = append([]string(nil), rc.AllowURLHosts...)
		}
	}
	return cfg
}

func ApplyReplConfig(env *object.Environment, cfg *ReplConfig) {
	if env == nil || cfg == nil {
		return
	}
	if strings.TrimSpace(cfg.ModuleDir) != "" {
		env.ModuleDir = cfg.ModuleDir
	}
	env.SecurityPolicy = &object.SecurityPolicy{
		StrictMode:            cfg.StrictMode,
		ProtectHost:           cfg.ProtectHost,
		AllowEnvWrite:         cfg.AllowEnvWrite,
		AllowedCapabilities:   append([]string(nil), cfg.AllowedCapabilities...),
		DeniedCapabilities:    append([]string(nil), cfg.DeniedCapabilities...),
		AllowedExecCommands:   append([]string(nil), cfg.AllowedExecCommands...),
		AllowedNativeModules:  append([]string(nil), cfg.AllowedNativeModules...),
		DeniedNativeModules:   append([]string(nil), cfg.DeniedNativeModules...),
		AllowedNetworkHosts:   append([]string(nil), cfg.AllowedNetworkHosts...),
		AllowedDBDrivers:      append([]string(nil), cfg.AllowedDBDrivers...),
		AllowedDBDSNPatterns:  append([]string(nil), cfg.AllowedDBDSNPatterns...),
		AllowedFileReadPaths:  append([]string(nil), cfg.AllowedFileReadPaths...),
		AllowedFileWritePaths: append([]string(nil), cfg.AllowedFileWritePaths...),
	}
	rl := &object.RuntimeLimits{HeapCheckEvery: 128}
	if cfg.MaxDepth > 0 {
		rl.MaxDepth = cfg.MaxDepth
	}
	if cfg.MaxSteps > 0 {
		rl.MaxSteps = cfg.MaxSteps
	}
	if cfg.MaxHeapMB > 0 {
		rl.MaxHeapBytes = uint64(cfg.MaxHeapMB) * 1024 * 1024
	}
	if cfg.TimeoutMS > 0 {
		rl.Deadline = time.Now().Add(time.Duration(cfg.TimeoutMS) * time.Millisecond)
	}
	if cfg.MaxOutputBytes > 0 {
		rl.MaxOutputBytes = cfg.MaxOutputBytes
	}
	if cfg.MaxHTTPBodyBytes > 0 {
		rl.MaxHTTPBodyBytes = cfg.MaxHTTPBodyBytes
	}
	if cfg.MaxExecOutputBytes > 0 {
		rl.MaxExecOutputBytes = cfg.MaxExecOutputBytes
	}
	if rl.MaxDepth == 0 && rl.MaxSteps == 0 && rl.MaxHeapBytes == 0 && rl.MaxOutputBytes == 0 && rl.MaxHTTPBodyBytes == 0 && rl.MaxExecOutputBytes == 0 && rl.Deadline.IsZero() {
		env.RuntimeLimits = nil
	} else {
		env.RuntimeLimits = rl
	}
	env.RenderConfig = &object.RenderConfig{
		Mode:             cfg.RenderMode,
		TerminalProtocol: cfg.RenderTerminalProtocol,
		MaxBytes:         cfg.RenderMaxBytes,
		AllowURLs:        cfg.RenderAllowURLs,
		AllowURLHosts:    append([]string(nil), cfg.RenderAllowURLHosts...),
	}
}

func ApplyReplProfile(cfg *ReplConfig, profile string) error {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "trusted", "":
		cfg.ExecutionProfile = "trusted"
		cfg.StrictMode = false
		cfg.ProtectHost = false
		cfg.AllowEnvWrite = true
		cfg.AllowedCapabilities = nil
		cfg.DeniedCapabilities = nil
		cfg.RenderMode = "auto"
		cfg.RenderAllowURLs = false
	case "untrusted":
		cfg.ExecutionProfile = "untrusted"
		cfg.StrictMode = true
		cfg.ProtectHost = true
		cfg.AllowEnvWrite = false
		cfg.AllowedCapabilities = []string{"filesystem_read"}
		cfg.DeniedCapabilities = []string{"async", "db", "env_write", "exec", "filesystem_write", "network", "policy", "process_exit", "scheduler", "server", "watch"}
		if len(cfg.AllowedFileReadPaths) == 0 {
			cfg.AllowedFileReadPaths = []string{cfg.ModuleDir}
		}
		cfg.AllowedFileWritePaths = nil
		if cfg.MaxDepth <= 0 || cfg.MaxDepth > 128 {
			cfg.MaxDepth = 128
		}
		if cfg.MaxSteps <= 0 || cfg.MaxSteps > 500_000 {
			cfg.MaxSteps = 500_000
		}
		if cfg.MaxHeapMB <= 0 || cfg.MaxHeapMB > 64 {
			cfg.MaxHeapMB = 64
		}
		if cfg.TimeoutMS <= 0 || cfg.TimeoutMS > 2_000 {
			cfg.TimeoutMS = 2_000
		}
		if cfg.MaxOutputBytes <= 0 || cfg.MaxOutputBytes > 64*1024 {
			cfg.MaxOutputBytes = 64 * 1024
		}
		if cfg.MaxHTTPBodyBytes <= 0 || cfg.MaxHTTPBodyBytes > 64*1024 {
			cfg.MaxHTTPBodyBytes = 64 * 1024
		}
		if cfg.MaxExecOutputBytes <= 0 || cfg.MaxExecOutputBytes > 64*1024 {
			cfg.MaxExecOutputBytes = 64 * 1024
		}
		cfg.RenderAllowURLs = false
		if cfg.RenderMaxBytes <= 0 || cfg.RenderMaxBytes > 64*1024 {
			cfg.RenderMaxBytes = 64 * 1024
		}
	default:
		return fmt.Errorf("unknown profile %q", profile)
	}
	return nil
}

func ReplConfigTable(env *object.Environment) string {
	cfg := ReplConfigForEnv(env)
	rows := [][]string{
		{"execution.profile", cfg.ExecutionProfile, "trusted|untrusted"},
		{"module.dir", cfg.ModuleDir, "import/file root"},
		{"security.strict", formatBool(cfg.StrictMode), "deny unspecified host access"},
		{"security.protect_host", formatBool(cfg.ProtectHost), "deny host mutation by default"},
		{"security.allow_env_write", formatBool(cfg.AllowEnvWrite), "allow os_env writes"},
		{"security.allow_capabilities", strings.Join(cfg.AllowedCapabilities, ","), "capability allowlist"},
		{"security.deny_capabilities", strings.Join(cfg.DeniedCapabilities, ","), "capability denylist"},
		{"security.allow_exec", strings.Join(cfg.AllowedExecCommands, ","), "allowed commands"},
		{"security.allow_native", strings.Join(cfg.AllowedNativeModules, ","), "allowed native modules"},
		{"security.deny_native", strings.Join(cfg.DeniedNativeModules, ","), "denied native modules"},
		{"security.allow_network", strings.Join(cfg.AllowedNetworkHosts, ","), "allowed hosts"},
		{"security.allow_db_drivers", strings.Join(cfg.AllowedDBDrivers, ","), "allowed DB drivers"},
		{"security.allow_db_dsn", strings.Join(cfg.AllowedDBDSNPatterns, ","), "allowed DSN patterns"},
		{"security.allow_file_read", strings.Join(cfg.AllowedFileReadPaths, ","), "read roots"},
		{"security.allow_file_write", strings.Join(cfg.AllowedFileWritePaths, ","), "write roots"},
		{"runtime.max_depth", strconv.Itoa(cfg.MaxDepth), "call depth"},
		{"runtime.max_steps", strconv.FormatInt(cfg.MaxSteps, 10), "eval steps"},
		{"runtime.max_heap_mb", strconv.FormatInt(cfg.MaxHeapMB, 10), "heap guard"},
		{"runtime.timeout_ms", strconv.FormatInt(cfg.TimeoutMS, 10), "wall-clock guard"},
		{"runtime.max_output_bytes", strconv.FormatInt(cfg.MaxOutputBytes, 10), "print/result output cap"},
		{"runtime.max_http_body_bytes", strconv.FormatInt(cfg.MaxHTTPBodyBytes, 10), "HTTP response cap"},
		{"runtime.max_exec_output_bytes", strconv.FormatInt(cfg.MaxExecOutputBytes, 10), "exec output cap"},
		{"render.mode", cfg.RenderMode, "auto|off|metadata|inline"},
		{"render.terminal_protocol", cfg.RenderTerminalProtocol, "auto|kitty|iterm|sixel|none"},
		{"render.max_bytes", strconv.FormatInt(cfg.RenderMaxBytes, 10), "artifact byte cap"},
		{"render.allow_urls", formatBool(cfg.RenderAllowURLs), "allow URL artifacts"},
		{"render.allow_url_hosts", strings.Join(cfg.RenderAllowURLHosts, ","), "render URL allowlist"},
	}
	return formatTable([]string{"Key", "Value", "Description"}, rows)
}

func formatTable(headers []string, rows [][]string) string {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i := range headers {
			if i < len(row) && len(row[i]) > widths[i] {
				widths[i] = len(row[i])
			}
		}
	}
	var b strings.Builder
	writeRow := func(cols []string) {
		for i := range headers {
			if i > 0 {
				b.WriteString("  ")
			}
			val := ""
			if i < len(cols) {
				val = cols[i]
			}
			b.WriteString(val)
			if pad := widths[i] - len(val); pad > 0 {
				b.WriteString(strings.Repeat(" ", pad))
			}
		}
		b.WriteString("\n")
	}
	writeRow(headers)
	sep := make([]string, len(headers))
	for i := range headers {
		sep[i] = strings.Repeat("-", widths[i])
	}
	writeRow(sep)
	for _, row := range rows {
		writeRow(row)
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatBool(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func handleReplConfigCommand(trimmed string, env *object.Environment) bool {
	if trimmed != ":config" && !strings.HasPrefix(trimmed, ":config ") {
		return false
	}
	raw := strings.TrimSpace(strings.TrimPrefix(trimmed, ":config"))
	if raw == "" || raw == "list" {
		replPrintBlock(ReplConfigTable(env))
		return true
	}
	args := strings.Fields(raw)
	if len(args) == 0 {
		replPrintBlock(ReplConfigTable(env))
		return true
	}
	switch args[0] {
	case "get":
		if len(args) != 2 {
			ReplPrintLine("usage: :config get <key>")
			return true
		}
		cfg := ReplConfigForEnv(env)
		val, err := replConfigValue(cfg, args[1])
		if err != nil {
			ReplPrintLine("config error: " + err.Error())
			return true
		}
		ReplPrintLine(fmt.Sprintf("%s = %s", args[1], val))
		return true
	case "set":
		if len(args) < 3 {
			ReplPrintLine("usage: :config set <key> <value>")
			return true
		}
		cfg := ReplConfigForEnv(env)
		if err := setReplConfigValue(cfg, args[1], strings.Join(args[2:], " ")); err != nil {
			ReplPrintLine("config error: " + err.Error())
			return true
		}
		ApplyReplConfig(env, cfg)
		ReplPrintLine(fmt.Sprintf("%s = %s", args[1], strings.Join(args[2:], " ")))
		return true
	case "profile":
		if len(args) != 2 {
			ReplPrintLine("usage: :config profile <trusted|untrusted>")
			return true
		}
		cfg := ReplConfigForEnv(env)
		if err := ApplyReplProfile(cfg, args[1]); err != nil {
			ReplPrintLine("config error: " + err.Error())
			return true
		}
		ApplyReplConfig(env, cfg)
		ReplPrintLine("execution.profile = " + cfg.ExecutionProfile)
		return true
	case "reset":
		key := "global"
		if env != nil {
			key = env.EnsureOwnerID()
		}
		replConfigs.mu.Lock()
		delete(replConfigs.items, key)
		replConfigs.mu.Unlock()
		cfg := ReplConfigForEnv(env)
		ApplyReplConfig(env, cfg)
		ReplPrintLine("configuration reset")
		return true
	case "load":
		if len(args) < 2 || len(args) > 3 {
			ReplPrintLine("usage: :config load <file> [json|yaml|env]")
			return true
		}
		format := ""
		if len(args) == 3 {
			format = args[2]
		}
		return replLoadConfigFile(args[1], format, env)
	default:
		if len(args) >= 1 && len(args) <= 2 {
			format := ""
			if len(args) == 2 {
				format = args[1]
			}
			return replLoadConfigFile(args[0], format, env)
		}
		ReplPrintLine("usage: :config [list|get|set|profile|reset|load]")
		return true
	}
}

func replLoadConfigFile(path, format string, env *object.Environment) bool {
	if LoadConfigObjectFromPathFn == nil {
		ReplPrintLine("config loader not available")
		return true
	}
	obj, err := LoadConfigObjectFromPathFn(path, format)
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

func replConfigValue(cfg *ReplConfig, key string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "execution.profile":
		return cfg.ExecutionProfile, nil
	case "module.dir":
		return cfg.ModuleDir, nil
	case "security.strict":
		return formatBool(cfg.StrictMode), nil
	case "security.protect_host":
		return formatBool(cfg.ProtectHost), nil
	case "security.allow_env_write":
		return formatBool(cfg.AllowEnvWrite), nil
	case "security.allow_capabilities":
		return strings.Join(cfg.AllowedCapabilities, ","), nil
	case "security.deny_capabilities":
		return strings.Join(cfg.DeniedCapabilities, ","), nil
	case "security.allow_exec":
		return strings.Join(cfg.AllowedExecCommands, ","), nil
	case "security.allow_native":
		return strings.Join(cfg.AllowedNativeModules, ","), nil
	case "security.deny_native":
		return strings.Join(cfg.DeniedNativeModules, ","), nil
	case "security.allow_network":
		return strings.Join(cfg.AllowedNetworkHosts, ","), nil
	case "security.allow_db_drivers":
		return strings.Join(cfg.AllowedDBDrivers, ","), nil
	case "security.allow_db_dsn":
		return strings.Join(cfg.AllowedDBDSNPatterns, ","), nil
	case "security.allow_file_read":
		return strings.Join(cfg.AllowedFileReadPaths, ","), nil
	case "security.allow_file_write":
		return strings.Join(cfg.AllowedFileWritePaths, ","), nil
	case "runtime.max_depth":
		return strconv.Itoa(cfg.MaxDepth), nil
	case "runtime.max_steps":
		return strconv.FormatInt(cfg.MaxSteps, 10), nil
	case "runtime.max_heap_mb":
		return strconv.FormatInt(cfg.MaxHeapMB, 10), nil
	case "runtime.timeout_ms":
		return strconv.FormatInt(cfg.TimeoutMS, 10), nil
	case "runtime.max_output_bytes":
		return strconv.FormatInt(cfg.MaxOutputBytes, 10), nil
	case "runtime.max_http_body_bytes":
		return strconv.FormatInt(cfg.MaxHTTPBodyBytes, 10), nil
	case "runtime.max_exec_output_bytes":
		return strconv.FormatInt(cfg.MaxExecOutputBytes, 10), nil
	case "render.mode":
		return cfg.RenderMode, nil
	case "render.terminal_protocol":
		return cfg.RenderTerminalProtocol, nil
	case "render.max_bytes":
		return strconv.FormatInt(cfg.RenderMaxBytes, 10), nil
	case "render.allow_urls":
		return formatBool(cfg.RenderAllowURLs), nil
	case "render.allow_url_hosts":
		return strings.Join(cfg.RenderAllowURLHosts, ","), nil
	default:
		return "", fmt.Errorf("unknown config key %q", key)
	}
}

func setReplConfigValue(cfg *ReplConfig, key, raw string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	raw = strings.TrimSpace(raw)
	switch key {
	case "execution.profile":
		return ApplyReplProfile(cfg, raw)
	case "module.dir":
		if raw == "" {
			return fmt.Errorf("module.dir cannot be empty")
		}
		cfg.ModuleDir = raw
	case "security.strict":
		v, err := parseReplBool(raw)
		if err != nil {
			return err
		}
		cfg.StrictMode = v
	case "security.protect_host":
		v, err := parseReplBool(raw)
		if err != nil {
			return err
		}
		cfg.ProtectHost = v
	case "security.allow_env_write":
		v, err := parseReplBool(raw)
		if err != nil {
			return err
		}
		cfg.AllowEnvWrite = v
	case "security.allow_capabilities":
		cfg.AllowedCapabilities = parseReplCSV(raw)
	case "security.deny_capabilities":
		cfg.DeniedCapabilities = parseReplCSV(raw)
	case "security.allow_exec":
		cfg.AllowedExecCommands = parseReplCSV(raw)
	case "security.allow_native":
		cfg.AllowedNativeModules = parseReplCSV(raw)
	case "security.deny_native":
		cfg.DeniedNativeModules = parseReplCSV(raw)
	case "security.allow_network":
		cfg.AllowedNetworkHosts = parseReplCSV(raw)
	case "security.allow_db_drivers":
		cfg.AllowedDBDrivers = parseReplCSV(raw)
	case "security.allow_db_dsn":
		cfg.AllowedDBDSNPatterns = parseReplCSV(raw)
	case "security.allow_file_read":
		cfg.AllowedFileReadPaths = parseReplCSV(raw)
	case "security.allow_file_write":
		cfg.AllowedFileWritePaths = parseReplCSV(raw)
	case "runtime.max_depth":
		v, err := parseReplInt(raw)
		if err != nil {
			return err
		}
		cfg.MaxDepth = int(v)
	case "runtime.max_steps":
		v, err := parseReplInt(raw)
		if err != nil {
			return err
		}
		cfg.MaxSteps = v
	case "runtime.max_heap_mb":
		v, err := parseReplInt(raw)
		if err != nil {
			return err
		}
		cfg.MaxHeapMB = v
	case "runtime.timeout_ms":
		v, err := parseReplInt(raw)
		if err != nil {
			return err
		}
		cfg.TimeoutMS = v
	case "runtime.max_output_bytes":
		v, err := parseReplInt(raw)
		if err != nil {
			return err
		}
		cfg.MaxOutputBytes = v
	case "runtime.max_http_body_bytes":
		v, err := parseReplInt(raw)
		if err != nil {
			return err
		}
		cfg.MaxHTTPBodyBytes = v
	case "runtime.max_exec_output_bytes":
		v, err := parseReplInt(raw)
		if err != nil {
			return err
		}
		cfg.MaxExecOutputBytes = v
	case "render.mode":
		v := strings.ToLower(strings.TrimSpace(raw))
		switch v {
		case "auto", "off", "metadata", "inline":
			cfg.RenderMode = v
		default:
			return fmt.Errorf("expected render mode auto|off|metadata|inline, got %q", raw)
		}
	case "render.terminal_protocol":
		v := strings.ToLower(strings.TrimSpace(raw))
		switch v {
		case "auto", "kitty", "iterm", "sixel", "none":
			cfg.RenderTerminalProtocol = v
		default:
			return fmt.Errorf("expected terminal protocol auto|kitty|iterm|sixel|none, got %q", raw)
		}
	case "render.max_bytes":
		v, err := parseReplInt(raw)
		if err != nil {
			return err
		}
		cfg.RenderMaxBytes = v
	case "render.allow_urls":
		v, err := parseReplBool(raw)
		if err != nil {
			return err
		}
		cfg.RenderAllowURLs = v
	case "render.allow_url_hosts":
		cfg.RenderAllowURLHosts = parseReplCSV(raw)
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
}

func parseReplBool(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("expected boolean, got %q", raw)
	}
}

func parseReplInt(raw string) (int64, error) {
	v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("expected non-negative integer, got %q", raw)
	}
	return v, nil
}

func parseReplCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "-" || strings.EqualFold(raw, "none") {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
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
		ReplPrintLine("- Home/End or Ctrl+A/Ctrl+E: jump to start/end")
		ReplPrintLine("- Alt+Left/Alt+Right, Ctrl+Left/Ctrl+Right, Alt+B/Alt+F: move by word")
		ReplPrintLine("- Ctrl+U/Ctrl+K/Ctrl+W: clear before cursor, clear after cursor, delete previous word")
		ReplPrintLine("- Tab: semantic completion for names/methods/fields")
		ReplPrintLine("- Suggestions appear while typing; Tab accepts or expands the best match")
		ReplPrintLine("- Inline suggestion: gray suffix preview")
		ReplPrintLine("- Call tips: signatures/docs shown while typing calls")
		ReplPrintLine("- Ctrl+R: reverse history search")
		ReplPrintLine("- Ctrl+C: clear current line")
		ReplPrintLine("- Ctrl+D: exit when line is empty")
		ReplPrintLine(Paint("Commands:", ColorBold+ColorCyan))
		ReplPrintLine("- :tips       show practical REPL tips")
		ReplPrintLine("- :commands [query] list/search all REPL commands")
		ReplPrintLine("- :palette <query> search commands/builtins/symbols")
		ReplPrintLine("- :examples   show runnable examples")
		ReplPrintLine("- :tools      show daily tech chore modules and preview-first examples")
		ReplPrintLine("- :hint <expr> show inline documentation")
		ReplPrintLine("- :builtins   list all available builtins")
		ReplPrintLine("- :search X   search builtins/keywords by text")
		ReplPrintLine("- :history    print command history")
		ReplPrintLine("- :clear      clear screen")
		ReplPrintLine("- :vars       list all variables in current environment")
		ReplPrintLine("- :type <expr> show the type of an expression")
		ReplPrintLine("- :doc <name> show builtin/object documentation")
		ReplPrintLine("- :diagnostics [source] show parser/static diagnostics")
		ReplPrintLine("- :symbols [query] list REPL/workspace symbols")
		ReplPrintLine("- :def <name> show symbol definition")
		ReplPrintLine("- :refs <name> show symbol references")
		ReplPrintLine("- :format [source] format SPL source")
		ReplPrintLine("- :methods <expr> list methods available on a value")
		ReplPrintLine("- :fields <expr> list fields available on a value")
		ReplPrintLine("- :ast <expr> print parsed AST representation")
		ReplPrintLine("- :time <expr> evaluate and show execution time")
		ReplPrintLine("- :debug <expr> step through statements")
		ReplPrintLine("- :mem        show runtime memory usage")
		ReplPrintLine("- :checkpoint [name] save a session checkpoint")
		ReplPrintLine("- :restore <name> restore a session checkpoint")
		ReplPrintLine("- :replay     replay recorded session inputs from the initial checkpoint")
		ReplPrintLine("- :inspect [name] inspect session state or a variable")
		ReplPrintLine("- :metrics    show recent execution metrics")
		ReplPrintLine("- :events     show recent session events")
		ReplPrintLine("- :trace on|off toggle trace event display")
		ReplPrintLine("- :tasks      list background tasks when task tracking is available")
		ReplPrintLine("- :cancel <id> cancel a background task when cancellation is available")
		ReplPrintLine("- :ask/:explain/:fix use configured assistant provider")
		ReplPrintLine("- :load <file> load and execute a script file")
		ReplPrintLine("- :reload [file] clear module cache or one module")
		ReplPrintLine("- :install <alias> <path> add dependency to spl.mod and refresh lock")
		ReplPrintLine("- :config     list active REPL runtime/security configuration")
		ReplPrintLine("- :config set <key> <value> change REPL configuration")
		ReplPrintLine("- :config profile <trusted|untrusted> apply a preset")
		ReplPrintLine("- :config load <file> [format] load app config into CONFIG")
		ReplPrintLine("- !<cmd>      execute shell command")
		ReplPrintLine("- :reset      reset the environment")
		return true
	}
	if handleReplConfigCommand(trimmed, env) {
		return true
	}
	if trimmed == ":tips" {
		replPrintBlock(ReplTipsText())
		return true
	}
	if trimmed == ":examples" {
		replPrintBlock(ReplExamplesText())
		return true
	}
	if trimmed == ":tools" {
		replPrintBlock(ReplToolsText())
		return true
	}
	if trimmed == ":commands" || strings.HasPrefix(trimmed, ":commands ") {
		query := strings.TrimSpace(strings.TrimPrefix(trimmed, ":commands"))
		replPrintBlock(ReplCommandsTable(query))
		return true
	}
	if strings.HasPrefix(trimmed, ":palette") {
		query := strings.TrimSpace(strings.TrimPrefix(trimmed, ":palette"))
		if query == "" {
			ReplPrintLine("usage: :palette <query>")
			return true
		}
		replPrintBlock(ReplPalette(query, editor, env))
		return true
	}
	if trimmed == ":hint" || strings.HasPrefix(trimmed, ":hint ") {
		target := strings.TrimSpace(strings.TrimPrefix(trimmed, ":hint"))
		if target == "" {
			ReplPrintLine("usage: :hint <expr|name>")
			return true
		}
		if doc := replToolingDoc(target, editor); doc != "" {
			replPrintBlock(doc)
			return true
		}
		replPrintBlock(ReplDocText(target, env))
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
			replSessions.mu.Lock()
			delete(replSessions.items, env)
			replSessions.mu.Unlock()
		}
		if editor != nil {
			editor.Source = ""
			if editor.Index != nil {
				editor.Index.Remove(editor.SourcePath)
			}
		}
		ReplPrintLine("environment reset")
		return true
	}
	if trimmed == ":checkpoint" || strings.HasPrefix(trimmed, ":checkpoint ") {
		name := strings.TrimSpace(strings.TrimPrefix(trimmed, ":checkpoint"))
		sess := replSessionForEnv(env)
		if sess == nil {
			ReplPrintLine("session not available")
			return true
		}
		snap, err := sess.Checkpoint(name)
		if err != nil {
			ReplPrintLine("checkpoint error: " + err.Error())
			return true
		}
		ReplPrintLine("checkpoint saved: " + string(snap.ID))
		return true
	}
	if strings.HasPrefix(trimmed, ":restore") {
		name := strings.TrimSpace(strings.TrimPrefix(trimmed, ":restore"))
		if name == "" {
			ReplPrintLine("usage: :restore <checkpoint>")
			return true
		}
		sess := replSessionForEnv(env)
		if sess == nil {
			ReplPrintLine("session not available")
			return true
		}
		if err := sess.Restore(name); err != nil {
			ReplPrintLine("restore error: " + err.Error())
		} else {
			ReplPrintLine("restored: " + name)
		}
		return true
	}
	if trimmed == ":replay" || strings.HasPrefix(trimmed, ":replay ") {
		sess := replSessionForEnv(env)
		if sess == nil {
			ReplPrintLine("session not available")
			return true
		}
		results, err := sess.Replay()
		if err != nil {
			ReplPrintLine("replay error: " + err.Error())
			return true
		}
		ReplPrintLine(fmt.Sprintf("replayed %d execution(s)", len(results)))
		for _, res := range results {
			status := "ok"
			if !res.OK {
				status = "error"
			}
			detail := res.ResultText
			if detail == "" {
				detail = res.Error
			}
			ReplPrintLine(fmt.Sprintf("  %s %s %s", res.ID, status, detail))
		}
		return true
	}
	if trimmed == ":inspect" || strings.HasPrefix(trimmed, ":inspect ") {
		name := strings.TrimSpace(strings.TrimPrefix(trimmed, ":inspect"))
		sess := replSessionForEnv(env)
		if sess == nil {
			ReplPrintLine("session not available")
			return true
		}
		if name != "" {
			if val, ok := env.Get(name); ok {
				ReplPrintLine(fmt.Sprintf("%s: %s = %s", name, val.Type(), FormatObjectPlain(val)))
			} else {
				ReplPrintLine("not found: " + name)
			}
			return true
		}
		replPrintSessionInspect(sess.Inspect())
		return true
	}
	if trimmed == ":metrics" {
		sess := replSessionForEnv(env)
		if sess == nil {
			ReplPrintLine("session not available")
			return true
		}
		inspect := sess.Inspect()
		if len(inspect.History) == 0 {
			ReplPrintLine("no executions recorded")
			return true
		}
		for _, res := range inspect.History {
			ReplPrintLine(fmt.Sprintf("%s ok=%t duration=%s steps=%d output=%d result=%s", res.ID, res.OK, res.Metrics.Duration, res.Metrics.Steps, res.Metrics.OutputBytes, res.Metrics.ResultType))
		}
		return true
	}
	if trimmed == ":events" {
		sess := replSessionForEnv(env)
		if sess == nil {
			ReplPrintLine("session not available")
			return true
		}
		events := sess.Events()
		if len(events) == 0 {
			ReplPrintLine("no session events")
			return true
		}
		start := 0
		if len(events) > 20 {
			start = len(events) - 20
		}
		for _, ev := range events[start:] {
			ReplPrintLine(fmt.Sprintf("%s %s %s", ev.Time.Format("15:04:05"), ev.Kind, ev.Message))
		}
		return true
	}
	if strings.HasPrefix(trimmed, ":trace") {
		arg := strings.TrimSpace(strings.TrimPrefix(trimmed, ":trace"))
		if arg == "on" || arg == "off" {
			ReplPrintLine("trace display " + arg)
		} else {
			ReplPrintLine("usage: :trace <on|off>")
		}
		return true
	}
	if trimmed == ":tasks" {
		ReplPrintLine("task tracking is not yet available for this session")
		return true
	}
	if strings.HasPrefix(trimmed, ":cancel") {
		ReplPrintLine("task cancellation is not yet available for this session")
		return true
	}
	if strings.HasPrefix(trimmed, ":ask") || strings.HasPrefix(trimmed, ":explain") || strings.HasPrefix(trimmed, ":fix") {
		parts := strings.Fields(trimmed)
		kind := strings.TrimPrefix(parts[0], ":")
		prompt := strings.TrimSpace(strings.TrimPrefix(trimmed, parts[0]))
		sess := replSessionForEnv(env)
		if sess == nil {
			ReplPrintLine("session not available")
			return true
		}
		answer, err := sess.Ask(kind, prompt)
		if err != nil {
			ReplPrintLine("assistant unavailable: " + err.Error())
			return true
		}
		replPrintBlock(answer.Text)
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
		if doc := replToolingDoc(target, editor); doc != "" {
			replPrintBlock(doc)
			return true
		}
		replPrintBlock(ReplDocText(target, env))
		return true
	}
	if trimmed == ":diagnostics" || strings.HasPrefix(trimmed, ":diagnostics ") {
		source := strings.TrimSpace(strings.TrimPrefix(trimmed, ":diagnostics"))
		if source == "" && editor != nil {
			source = editor.Source
		}
		if strings.TrimSpace(source) == "" {
			ReplPrintLine("usage: :diagnostics <source> or run some REPL input first")
			return true
		}
		replPrintDiagnostics(tooling.CheckSource(replSourcePath(editor), source).Diagnostics)
		return true
	}
	if trimmed == ":symbols" || strings.HasPrefix(trimmed, ":symbols ") {
		query := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(trimmed, ":symbols")))
		symbols := replSymbols(editor)
		count := 0
		for _, sym := range symbols {
			if query != "" && !strings.Contains(strings.ToLower(sym.Name), query) {
				continue
			}
			ReplPrintLine(fmt.Sprintf("%s %s  %s:%d:%d", sym.Kind, sym.Name, sym.Path, sym.Line, sym.Column))
			count++
		}
		if count == 0 {
			ReplPrintLine("no symbols found")
		}
		return true
	}
	if strings.HasPrefix(trimmed, ":def ") {
		name := strings.TrimSpace(strings.TrimPrefix(trimmed, ":def "))
		replPrintDefinition(name, editor)
		return true
	}
	if strings.HasPrefix(trimmed, ":refs ") {
		name := strings.TrimSpace(strings.TrimPrefix(trimmed, ":refs "))
		replPrintReferences(name, editor)
		return true
	}
	if trimmed == ":format" || strings.HasPrefix(trimmed, ":format ") {
		source := strings.TrimSpace(strings.TrimPrefix(trimmed, ":format"))
		if source == "" && editor != nil {
			source = editor.Source
		}
		if strings.TrimSpace(source) == "" {
			ReplPrintLine("usage: :format <source> or run some REPL input first")
			return true
		}
		report := tooling.FormatSource(replSourcePath(editor), source)
		if len(report.Diagnostics) > 0 {
			replPrintDiagnostics(report.Diagnostics)
			return true
		}
		replPrintBlock(strings.TrimRight(report.Formatted, "\n"))
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
					ReplPrintLine(FormatObjectForDisplayWithEnv(result, env))
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
		if editor != nil && editor.Index != nil {
			editor.Index.Update(resolved, string(data))
		}
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

func replSourcePath(editor *ReplEditor) string {
	if editor != nil && strings.TrimSpace(editor.SourcePath) != "" {
		return editor.SourcePath
	}
	return "<repl>"
}

func replPrintDiagnostics(diags []tooling.Diagnostic) {
	if len(diags) == 0 {
		ReplPrintLine("no diagnostics")
		return
	}
	for _, d := range diags {
		loc := ""
		if d.Line > 0 {
			loc = fmt.Sprintf("%s:%d:%d: ", d.Path, d.Line, d.Column)
		}
		ReplPrintLine(fmt.Sprintf("%s%s: %s", loc, d.Severity, d.Message))
		if d.Hint != "" {
			ReplPrintLine("  hint: " + d.Hint)
		}
		if d.Snippet != "" {
			replPrintBlock("  " + strings.ReplaceAll(strings.TrimRight(d.Snippet, "\n"), "\n", "\n  "))
		}
	}
}

func replPrintSessionInspect(inspect sessionpkg.SessionInspect) {
	ReplPrintLine(fmt.Sprintf("session: %s profile=%s", inspect.ID, inspect.Profile))
	if inspect.ModuleDir != "" {
		ReplPrintLine("module_dir: " + inspect.ModuleDir)
	}
	names := make([]string, 0, len(inspect.Variables))
	for name := range inspect.Variables {
		names = append(names, name)
	}
	sort.Strings(names)
	ReplPrintLine(fmt.Sprintf("variables: %d", len(names)))
	for _, name := range names {
		ReplPrintLine(fmt.Sprintf("  %s = %s", name, inspect.Variables[name]))
	}
	ReplPrintLine(fmt.Sprintf("executions: %d", len(inspect.History)))
	ReplPrintLine(fmt.Sprintf("checkpoints: %d", len(inspect.Checkpoints)))
	if inspect.RuntimeLimits != nil {
		ReplPrintLine(fmt.Sprintf("runtime: steps=%v output=%v", inspect.RuntimeLimits["steps"], inspect.RuntimeLimits["output_bytes"]))
	}
}

func replSymbols(editor *ReplEditor) []tooling.Symbol {
	if editor == nil {
		return nil
	}
	seen := map[string]struct{}{}
	out := []tooling.Symbol{}
	add := func(sym tooling.Symbol) {
		key := fmt.Sprintf("%s\x00%s\x00%d\x00%d", sym.Path, sym.Name, sym.Line, sym.Column)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, sym)
	}
	if strings.TrimSpace(editor.Source) != "" {
		for _, sym := range tooling.SymbolsForSource(replSourcePath(editor), editor.Source) {
			add(sym)
		}
	}
	if editor.Index != nil {
		for _, sym := range editor.Index.AllSymbols() {
			add(sym)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].Path < out[j].Path
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func replToolingDoc(target string, editor *ReplEditor) string {
	if strings.TrimSpace(target) == "" {
		return ""
	}
	if editor != nil {
		path, src, line, col, ok := replFindWordPosition(target, editor)
		if ok {
			if hover := tooling.HoverMarkdown(editor.Index, path, src, line, col); hover != "" {
				return hover
			}
		}
	}
	src := target
	return tooling.HoverMarkdown(nil, "<repl-doc>", src, 1, 1)
}

func replPrintDefinition(name string, editor *ReplEditor) {
	if strings.TrimSpace(name) == "" {
		ReplPrintLine("usage: :def <name>")
		return
	}
	if editor == nil || editor.Index == nil {
		ReplPrintLine("workspace index is not available")
		return
	}
	path, src, line, col, ok := replFindWordPosition(name, editor)
	if ok {
		if loc, found := editor.Index.Definition(path, src, line, col); found {
			ReplPrintLine(fmt.Sprintf("%s: %s:%d:%d", loc.Name, loc.Path, loc.Line, loc.Column))
			return
		}
	}
	for _, sym := range replSymbols(editor) {
		if sym.Name == name {
			ReplPrintLine(fmt.Sprintf("%s: %s:%d:%d", sym.Name, sym.Path, sym.Line, sym.Column))
			return
		}
	}
	ReplPrintLine("definition not found")
}

func replPrintReferences(name string, editor *ReplEditor) {
	if strings.TrimSpace(name) == "" {
		ReplPrintLine("usage: :refs <name>")
		return
	}
	if editor == nil || editor.Index == nil {
		ReplPrintLine("workspace index is not available")
		return
	}
	path, src, line, col, ok := replFindWordPosition(name, editor)
	if !ok {
		ReplPrintLine("reference source not found")
		return
	}
	refs := editor.Index.References(path, src, line, col)
	if len(refs) == 0 {
		ReplPrintLine("no references found")
		return
	}
	for _, ref := range refs {
		ReplPrintLine(fmt.Sprintf("%s: %s:%d:%d", ref.Name, ref.Path, ref.Line, ref.Column))
	}
}

func replFindWordPosition(name string, editor *ReplEditor) (string, string, int, int, bool) {
	if editor == nil {
		return "", "", 0, 0, false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", 0, 0, false
	}
	if strings.TrimSpace(editor.Source) != "" {
		if line, col, ok := findWordInSource(editor.Source, name); ok {
			return replSourcePath(editor), editor.Source, line, col, true
		}
	}
	if editor.Index != nil {
		for path, doc := range editor.Index.Documents {
			if line, col, ok := findWordInSource(doc.Source, name); ok {
				return path, doc.Source, line, col, true
			}
		}
	}
	return "", "", 0, 0, false
}

func findWordInSource(src, word string) (int, int, bool) {
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		start := 0
		for {
			idx := strings.Index(line[start:], word)
			if idx < 0 {
				break
			}
			col := start + idx
			beforeOK := col == 0 || !isIdentifierRune(rune(line[col-1]))
			after := col + len(word)
			afterOK := after >= len(line) || !isIdentifierRune(rune(line[after]))
			if beforeOK && afterOK {
				return i + 1, len([]rune(line[:col])) + 1, true
			}
			start = after
			if start >= len(line) {
				break
			}
		}
	}
	return 0, 0, false
}

func isIdentifierRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// ReplCandidates returns completion candidates without an environment.
func ReplCandidates() []string {
	return ReplCandidatesForEnv(nil)
}

func newReplEditor(in, out *os.File, candidates []string, env *object.Environment) (*ReplEditor, error) {
	fd := int(in.Fd())
	editor := &ReplEditor{
		In:         in,
		Out:        out,
		Fd:         fd,
		Env:        env,
		History:    make([]string, 0, 256),
		HistoryPos: 0,
		Candidates: candidates,
		SourcePath: "<repl>",
	}
	if wd, err := os.Getwd(); err == nil {
		editor.Index = tooling.NewWorkspaceIndex(wd)
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
}

func (e *ReplEditor) RecordInput(input string) {
	if e == nil || strings.TrimSpace(input) == "" {
		return
	}
	if strings.TrimSpace(e.Source) == "" {
		e.Source = input
	} else {
		e.Source = strings.TrimRight(e.Source, "\n") + "\n" + input
	}
	if e.Index != nil {
		e.Index.Update(e.SourcePath, e.Source)
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
		_, _ = fmt.Fprint(e.Out, "\r\033[2K\033[J")
		line := string(buf)
		styledPrompt := StylePrompt(prompt)
		if strings.HasPrefix(prompt, "..") {
			styledPrompt = StyleContinuationPrompt(prompt)
		}
		_, _ = fmt.Fprint(e.Out, styledPrompt, ColorizeInputLine(line))
		helperLines := []string{}
		if cursor == len(buf) {
			ctx := CompletionContext(buf, cursor)
			if ctx.Ok && ctx.Prefix != "" {
				completions := e.CompletionsForContext(ctx)
				if suggestion, ok := firstCompletion(ctx.Prefix, completions); ok && suggestion != ctx.Prefix && strings.HasPrefix(suggestion, ctx.Prefix) {
					suffix := suggestion[len(ctx.Prefix):]
					_, _ = fmt.Fprint(e.Out, Paint(suffix, ColorGray))
				}
				helperLines = append(helperLines, e.SuggestionLines(ctx, completions, replEditorWidth(e))...)
			}
			if tip := e.InlineHint(line, cursor); tip != "" {
				helperLines = append(helperLines, ReplHintLines(tip, replEditorWidth(e))...)
			}
		}
		if len(helperLines) > 0 {
			for _, helper := range helperLines {
				_, _ = fmt.Fprint(e.Out, "\r\n", Paint("  "+helper, ColorGray))
			}
			_, _ = fmt.Fprintf(e.Out, "\033[%dA", len(helperLines))
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
		case 11: // Ctrl+K
			buf = buf[:cursor]
		case 21: // Ctrl+U
			buf = buf[:0]
			cursor = 0
		case 23: // Ctrl+W
			start := previousWordBoundary(buf, cursor)
			buf = append(buf[:start], buf[cursor:]...)
			cursor = start
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
			case keyWordLeft:
				cursor = previousWordBoundary(buf, cursor)
			case keyWordRight:
				cursor = nextWordBoundary(buf, cursor)
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
	case 'b':
		return keyWordLeft
	case 'f':
		return keyWordRight
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
	case strings.HasSuffix(s, ";5D"), strings.HasSuffix(s, ";3D"):
		return keyWordLeft
	case strings.HasSuffix(s, ";5C"), strings.HasSuffix(s, ";3C"):
		return keyWordRight
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
	case strings.HasPrefix(s, "1~"), strings.HasPrefix(s, "7~"), strings.HasPrefix(s, "1;"):
		return keyHome
	case strings.HasPrefix(s, "4~"), strings.HasPrefix(s, "8~"), strings.HasPrefix(s, "4;"):
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
		return replaceCompletion(buf, ctx, matches[0])
	}
	if suggestion, ok := firstCompletion(ctx.Prefix, matches); ok && strings.HasPrefix(suggestion, ctx.Prefix) {
		return replaceCompletion(buf, ctx, suggestion)
	}

	common := LongestCommonPrefix(matches)
	if len(common) > len(ctx.Prefix) {
		return replaceCompletion(buf, ctx, common)
	}

	_, _ = fmt.Fprint(e.Out, "\r\n")
	details := e.completionDetails(ctx)
	for _, m := range matches {
		if detail := details[m]; detail != "" {
			_, _ = fmt.Fprintf(e.Out, "%-24s %s\n", m, detail)
		} else {
			_, _ = fmt.Fprintln(e.Out, m)
		}
	}
	if strings.HasPrefix(prompt, "..") {
		_, _ = fmt.Fprint(e.Out, StyleContinuationPrompt(prompt))
	} else {
		_, _ = fmt.Fprint(e.Out, StylePrompt(prompt))
	}
	return buf, cursor
}

func replaceCompletion(buf []rune, ctx ReplCompletionContext, completion string) ([]rune, int) {
	replacement := []rune(completion)
	newBuf := append([]rune{}, buf[:ctx.Start]...)
	newBuf = append(newBuf, replacement...)
	newBuf = append(newBuf, buf[ctx.End:]...)
	return newBuf, ctx.Start + len(replacement)
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
		return e.workspaceCompletionLabels(ctx, e.Candidates)
	}
	obj, errs := replEvalExpression(ctx.BaseExpr, e.Env)
	if len(errs) != 0 || obj == nil {
		return e.workspaceCompletionLabels(ctx, e.Candidates)
	}
	if IsErrorFn != nil && IsErrorFn(obj) {
		return e.workspaceCompletionLabels(ctx, e.Candidates)
	}
	fields := ReplObjectFields(obj)
	methods := ReplObjectMethods(obj)
	out := make([]string, 0, len(fields)+len(methods))
	out = append(out, fields...)
	out = append(out, methods...)
	sort.Strings(out)
	if len(out) == 0 {
		return e.workspaceCompletionLabels(ctx, e.Candidates)
	}
	return out
}

func (e *ReplEditor) workspaceCompletionLabels(ctx ReplCompletionContext, base []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(base)+16)
	add := func(label string) {
		label = strings.TrimSpace(label)
		if label == "" {
			return
		}
		if _, ok := seen[label]; ok {
			return
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}
	for _, label := range base {
		add(label)
	}
	if e != nil && e.Index != nil && ctx.BaseExpr == "" {
		src, _, _ := e.toolingSourceForCursor("", 0)
		for _, item := range tooling.WorkspaceCompletionItems(e.Index, e.SourcePath, src, ctx.Prefix) {
			add(item.Label)
		}
	}
	sort.Strings(out)
	return out
}

func (e *ReplEditor) completionDetails(ctx ReplCompletionContext) map[string]string {
	details := map[string]string{}
	for _, cmd := range replCommandCatalog {
		if strings.HasPrefix(cmd.Name, ctx.Prefix) || fuzzyContains(cmd.Name+" "+cmd.Summary, ctx.Prefix) {
			details[cmd.Name] = cmd.Summary
		}
	}
	if ctx.BaseExpr == "" {
		if e != nil && e.Env != nil {
			for _, name := range e.Env.Names() {
				if name == "" {
					continue
				}
				if !strings.HasPrefix(name, ctx.Prefix) && !fuzzyContains(name, ctx.Prefix) {
					continue
				}
				if val, ok := e.Env.Get(name); ok {
					if fn, ok := val.(*object.Function); ok {
						details[name] = ReplFunctionSignature(name, fn)
					} else if val != nil {
						details[name] = fmt.Sprintf("%s value", val.Type())
					}
				}
			}
		}
		if e != nil && BuiltinHelpTextFn != nil {
			for _, name := range e.CompletionsForContext(ctx) {
				if name == "" {
					continue
				}
				if !strings.HasPrefix(name, ctx.Prefix) && !fuzzyContains(name, ctx.Prefix) {
					continue
				}
				if detail := ReplBuiltinDetail(name); detail != "" {
					details[name] = detail
				}
			}
		}
	}
	if e == nil || e.Index == nil || ctx.BaseExpr != "" {
		return details
	}
	src, _, _ := e.toolingSourceForCursor("", 0)
	for _, item := range tooling.WorkspaceCompletionItems(e.Index, e.SourcePath, src, ctx.Prefix) {
		if item.Label == "" {
			continue
		}
		detail := tooling.CompletionDetail(item)
		if detail == "" {
			detail = item.Kind
		}
		details[item.Label] = ReplCompactHint(markdownToPlain(detail), 96)
	}
	return details
}

func (e *ReplEditor) SuggestionLines(ctx ReplCompletionContext, candidates []string, width int) []string {
	if !ctx.Ok || strings.TrimSpace(ctx.Prefix) == "" {
		return nil
	}
	matches := FindCompletions(ctx.Prefix, candidates)
	if len(matches) == 0 {
		return nil
	}
	if len(matches) > 5 {
		matches = matches[:5]
	}
	details := e.completionDetails(ctx)
	lines := make([]string, 0, len(matches)+1)
	lines = append(lines, "suggestions: "+strings.Join(matches, "  "))
	for _, match := range matches {
		if detail := strings.TrimSpace(details[match]); detail != "" {
			lines = append(lines, "  "+match+" - "+ReplCompactHint(detail, suggestionDetailWidth(width)))
			if len(lines) >= 3 {
				break
			}
		}
	}
	return lines
}

func suggestionDetailWidth(width int) int {
	if width <= 0 {
		return 90
	}
	if width < 60 {
		return 50
	}
	if width > 140 {
		return 120
	}
	return width - 20
}

func (e *ReplEditor) InlineHint(line string, cursor int) string {
	if tip := ReplCallTip(line, cursor, e.Env); tip != "" {
		return tip
	}
	if e == nil || e.Index == nil {
		return ""
	}
	ctx := CompletionContext([]rune(line), cursor)
	if ctx.Ok && ctx.Prefix != "" && ctx.BaseExpr == "" {
		src, _, _ := e.toolingSourceForCursor(line, cursor)
		for _, item := range tooling.WorkspaceCompletionItems(e.Index, e.SourcePath, src, ctx.Prefix) {
			if item.Label == ctx.Prefix {
				continue
			}
			detail := tooling.CompletionDetail(item)
			if detail == "" {
				detail = item.Detail
			}
			if detail != "" {
				return ReplCompactHint(item.Label+" - "+markdownToPlain(detail), 140)
			}
			break
		}
	}
	src, lno, col := e.toolingSourceForCursor(line, cursor)
	if hover := tooling.HoverMarkdown(e.Index, e.SourcePath, src, lno, col); hover != "" {
		return ReplCompactHint(markdownToPlain(hover), 140)
	}
	return ""
}

func (e *ReplEditor) toolingSourceForCursor(line string, cursor int) (string, int, int) {
	prefix := ""
	if e != nil {
		prefix = strings.TrimRight(e.Source, "\n")
	}
	src := line
	if prefix != "" {
		if line != "" {
			src = prefix + "\n" + line
		} else {
			src = prefix
		}
	}
	lines := strings.Split(src, "\n")
	lineNo := len(lines)
	col := cursor + 1
	if line == "" && len(lines) > 0 {
		col = len([]rune(lines[len(lines)-1])) + 1
	}
	if lineNo < 1 {
		lineNo = 1
	}
	if col < 1 {
		col = 1
	}
	return src, lineNo, col
}

func ReplCallTip(line string, cursor int, env *object.Environment) string {
	name, argIndex, ok := ReplActiveCall(line, cursor)
	if !ok || strings.TrimSpace(name) == "" {
		return ""
	}
	if HasBuiltinFn != nil && HasBuiltinFn(name) {
		if detail := ReplBuiltinCallTip(name, argIndex); detail != "" {
			return detail
		}
	}
	if env != nil {
		if val, ok := env.Get(name); ok {
			if fn, ok := val.(*object.Function); ok {
				return ReplFunctionCallTip(name, fn, argIndex)
			}
			return ReplCompactHint(fmt.Sprintf("%s: %s", name, val.Type()), 140)
		}
	}
	return ""
}

// ReplActiveCall returns the function call enclosing cursor and the current
// zero-based argument index. It is intentionally lightweight so it can run on
// every keypress in the REPL.
func ReplActiveCall(line string, cursor int) (name string, argIndex int, ok bool) {
	runes := []rune(line)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	type frame struct {
		delim    rune
		name     string
		argIndex int
	}
	stack := make([]frame, 0, 8)
	var quote rune
	escaped := false
	for i := 0; i < cursor; i++ {
		r := runes[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"', '`':
			quote = r
		case '(':
			stack = append(stack, frame{delim: r, name: callNameBefore(runes, i)})
		case '[', '{':
			stack = append(stack, frame{delim: r})
		case ')', ']', '}':
			if len(stack) == 0 {
				continue
			}
			top := stack[len(stack)-1]
			if (r == ')' && top.delim == '(') || (r == ']' && top.delim == '[') || (r == '}' && top.delim == '{') {
				stack = stack[:len(stack)-1]
			}
		case ',':
			if len(stack) == 0 {
				continue
			}
			top := &stack[len(stack)-1]
			if top.delim == '(' && strings.TrimSpace(top.name) != "" {
				top.argIndex++
			}
		}
	}
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].delim == '(' && strings.TrimSpace(stack[i].name) != "" {
			return stack[i].name, stack[i].argIndex, true
		}
	}
	return "", 0, false
}

func callNameBefore(runes []rune, openParen int) string {
	i := openParen - 1
	for i >= 0 && unicode.IsSpace(runes[i]) {
		i--
	}
	end := i + 1
	for i >= 0 && isTokenRune(runes[i]) {
		i--
	}
	return strings.TrimSpace(string(runes[i+1 : end]))
}

func ReplFunctionCallTip(name string, fn *object.Function, argIndex int) string {
	signature := ReplFunctionSignature(name, fn)
	if signature == "" {
		return ""
	}
	param := ReplFunctionParamTip(fn, argIndex)
	if param == "" {
		return signature
	}
	return ReplCompactHint(signature+" | "+param, 180)
}

func ReplFunctionParamTip(fn *object.Function, argIndex int) string {
	if fn == nil || argIndex < 0 || len(fn.Parameters) == 0 {
		return ""
	}
	if argIndex >= len(fn.Parameters) {
		if fn.HasRest && len(fn.Parameters) > 0 {
			argIndex = len(fn.Parameters) - 1
		} else {
			return fmt.Sprintf("arg %d: extra argument", argIndex+1)
		}
	}
	name := "arg"
	if p := fn.Parameters[argIndex]; p != nil && strings.TrimSpace(p.String()) != "" {
		name = p.String()
	}
	if fn.HasRest && argIndex == len(fn.Parameters)-1 {
		name = "..." + name
	}
	if argIndex < len(fn.ParamTypes) && strings.TrimSpace(fn.ParamTypes[argIndex]) != "" {
		name += ": " + strings.TrimSpace(fn.ParamTypes[argIndex])
	}
	if argIndex < len(fn.Defaults) && fn.Defaults[argIndex] != nil {
		name += " = ..."
	}
	return fmt.Sprintf("active: %s", name)
}

func ReplBuiltinCallTip(name string, argIndex int) string {
	detail := ReplBuiltinDetail(name)
	if detail == "" {
		return ""
	}
	argHint := ReplBuiltinArgHint(detail, argIndex)
	if argHint != "" {
		detail += " | " + argHint
	}
	return ReplCompactHint(detail, 180)
}

func ReplBuiltinDetail(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || BuiltinHelpTextFn == nil {
		return ""
	}
	help := strings.TrimSpace(BuiltinHelpTextFn(name))
	doc := strings.TrimSpace(markdownToPlain(tooling.RuntimeDocMarkdown(name)))
	switch {
	case help == "" && doc == "":
		return ""
	case help == "":
		return doc
	case doc == "":
		return help
	case !strings.Contains(help, "(...) builtin function"):
		return help
	default:
		return help + " " + doc
	}
}

func ReplBuiltinArgHint(detail string, argIndex int) string {
	signature := firstSignatureFragment(detail)
	open := strings.IndexRune(signature, '(')
	close := strings.LastIndex(signature, ")")
	if open < 0 || close <= open {
		return ""
	}
	params := splitSignatureParams(signature[open+1 : close])
	if len(params) == 0 {
		return ""
	}
	idx := argIndex
	if idx >= len(params) {
		last := strings.TrimSpace(params[len(params)-1])
		if !strings.Contains(last, "...") {
			return fmt.Sprintf("active: extra argument %d", argIndex+1)
		}
		idx = len(params) - 1
	}
	return fmt.Sprintf("active: %s", strings.TrimSpace(params[idx]))
}

func firstSignatureFragment(detail string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return ""
	}
	end := len(detail)
	for _, sep := range []string{";", " Signature:", ". "} {
		if idx := strings.Index(detail, sep); idx >= 0 && idx < end {
			end = idx
		}
	}
	return strings.TrimSpace(detail[:end])
}

func splitSignatureParams(params string) []string {
	params = strings.TrimSpace(params)
	if params == "" {
		return nil
	}
	var out []string
	start := 0
	depth := 0
	var quote rune
	escaped := false
	runes := []rune(params)
	for i, r := range runes {
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"', '`':
			quote = r
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(string(runes[start:i])))
				start = i + 1
			}
		}
	}
	out = append(out, strings.TrimSpace(string(runes[start:])))
	filtered := out[:0]
	for _, item := range out {
		for _, normalized := range expandOptionalSignatureParam(item) {
			if normalized != "" {
				filtered = append(filtered, normalized)
			}
		}
	}
	return filtered
}

func expandOptionalSignatureParam(param string) []string {
	param = strings.TrimSpace(param)
	if param == "" {
		return nil
	}
	open := strings.Index(param, "[,")
	close := strings.LastIndex(param, "]")
	if open < 0 || close <= open {
		return []string{param}
	}
	required := strings.TrimSpace(param[:open])
	optional := strings.TrimSpace(param[open+2 : close])
	out := []string{}
	if required != "" {
		out = append(out, required)
	}
	for _, part := range splitSignatureParams(optional) {
		part = strings.TrimSpace(part)
		if part != "" && !strings.Contains(strings.ToLower(part), "optional") {
			part += " optional"
		}
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func ReplFunctionSignature(name string, fn *object.Function) string {
	if fn == nil {
		return ""
	}
	fnName := strings.TrimSpace(fn.Name)
	if fnName == "" {
		fnName = strings.TrimSpace(name)
	}
	if fnName == "" {
		fnName = "function"
	}
	parts := make([]string, 0, len(fn.Parameters))
	for i, p := range fn.Parameters {
		part := "arg"
		if p != nil && strings.TrimSpace(p.String()) != "" {
			part = p.String()
		}
		if fn.HasRest && i == len(fn.Parameters)-1 {
			part = "..." + part
		}
		if i < len(fn.ParamTypes) && strings.TrimSpace(fn.ParamTypes[i]) != "" {
			part += ": " + strings.TrimSpace(fn.ParamTypes[i])
		}
		if i < len(fn.Defaults) && fn.Defaults[i] != nil {
			part += " = ..."
		}
		parts = append(parts, part)
	}
	signature := fmt.Sprintf("%s(%s)", fnName, strings.Join(parts, ", "))
	if strings.TrimSpace(fn.ReturnType) != "" {
		signature += " -> " + strings.TrimSpace(fn.ReturnType)
	}
	return ReplCompactHint(signature, 140)
}

func ReplCompactHint(text string, maxRunes int) string {
	text = strings.Join(strings.Fields(text), " ")
	if maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

func markdownToPlain(text string) string {
	replacer := strings.NewReplacer(
		"```spl", " ",
		"```", " ",
		"**", "",
		"`", "",
		"###", "",
		"##", "",
		"#", "",
		"\r", "\n",
	)
	text = replacer.Replace(text)
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, " ")
}

func ReplHintLines(text string, width int) []string {
	text = ReplCompactHint(text, 180)
	if text == "" {
		return nil
	}
	if width <= 0 {
		width = 100
	}
	lineWidth := width - 4
	if lineWidth < 40 {
		lineWidth = 40
	}
	if lineWidth > 100 {
		lineWidth = 100
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	lines := make([]string, 0, 2)
	var current strings.Builder
	for _, word := range words {
		if current.Len() == 0 {
			current.WriteString(word)
			continue
		}
		if current.Len()+1+len(word) > lineWidth {
			lines = append(lines, current.String())
			current.Reset()
			current.WriteString(word)
			if len(lines) == 2 {
				break
			}
			continue
		}
		current.WriteByte(' ')
		current.WriteString(word)
	}
	if current.Len() > 0 && len(lines) < 2 {
		lines = append(lines, current.String())
	}
	return lines
}

func replEditorWidth(e *ReplEditor) int {
	return 100
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

func previousWordBoundary(buf []rune, cursor int) int {
	if cursor > len(buf) {
		cursor = len(buf)
	}
	for cursor > 0 && unicode.IsSpace(buf[cursor-1]) {
		cursor--
	}
	for cursor > 0 && isNavigationWordRune(buf[cursor-1]) {
		cursor--
	}
	return cursor
}

func nextWordBoundary(buf []rune, cursor int) int {
	if cursor < 0 {
		cursor = 0
	}
	for cursor < len(buf) && unicode.IsSpace(buf[cursor]) {
		cursor++
	}
	for cursor < len(buf) && isNavigationWordRune(buf[cursor]) {
		cursor++
	}
	return cursor
}

func isNavigationWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == ':' || r == '.'
}

func FindCompletions(prefix string, candidates []string) []string {
	out := make([]string, 0, 8)
	for _, c := range candidates {
		if strings.HasPrefix(c, prefix) || fuzzyContains(c, prefix) {
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
	for _, c := range candidates {
		if fuzzyContains(c, prefix) {
			return c, true
		}
	}
	return "", false
}

func fuzzyContains(candidate, query string) bool {
	candidate = strings.ToLower(candidate)
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	if strings.Contains(candidate, query) {
		return true
	}
	j := 0
	for _, r := range candidate {
		if j < len(query) && byte(r) == query[j] {
			j++
		}
	}
	return j == len(query)
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
				ReplPrintLine(FormatObjectForDisplayWithEnv(obj, env))
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
