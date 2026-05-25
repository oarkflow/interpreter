package tooling

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/lexer"
	"github.com/oarkflow/interpreter/pkg/parser"
	"github.com/oarkflow/interpreter/pkg/pkgmgr"
	"github.com/oarkflow/interpreter/pkg/token"

	// Keep editor tooling aligned with the first-party runtime packages shipped
	// in this repository, not just the minimal CLI builtin set.
	_ "github.com/oarkflow/interpreter/pkg/builtins/reactive"
	_ "github.com/oarkflow/interpreter/pkg/builtins/scheduler"
	_ "github.com/oarkflow/interpreter/pkg/builtins/server"
	_ "github.com/oarkflow/interpreter/pkg/builtins/watcher"
)

type DiagnosticSeverity string

const (
	SeverityError   DiagnosticSeverity = "error"
	SeverityWarning DiagnosticSeverity = "warning"
	SeverityInfo    DiagnosticSeverity = "info"
)

type Diagnostic struct {
	Severity DiagnosticSeverity `json:"severity"`
	Path     string             `json:"path,omitempty"`
	Line     int                `json:"line,omitempty"`
	Column   int                `json:"column,omitempty"`
	Code     string             `json:"code,omitempty"`
	Message  string             `json:"message"`
	Hint     string             `json:"hint,omitempty"`
	Snippet  string             `json:"snippet,omitempty"`
}

type Report struct {
	Path        string       `json:"path,omitempty"`
	OK          bool         `json:"ok"`
	Changed     bool         `json:"changed,omitempty"`
	Formatted   string       `json:"formatted,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	Symbols     []Symbol     `json:"symbols,omitempty"`
}

type ProjectConfig struct {
	Runtime struct {
		MaxDepth           int   `json:"max_depth,omitempty"`
		MaxSteps           int64 `json:"max_steps,omitempty"`
		MaxHeapMB          int64 `json:"max_heap_mb,omitempty"`
		TimeoutMS          int64 `json:"timeout_ms,omitempty"`
		MaxOutputBytes     int64 `json:"max_output_bytes,omitempty"`
		MaxHTTPBodyBytes   int64 `json:"max_http_body_bytes,omitempty"`
		MaxExecOutputBytes int64 `json:"max_exec_output_bytes,omitempty"`
	} `json:"runtime,omitempty"`
	Security struct {
		Profile               string   `json:"profile,omitempty"`
		AllowedCapabilities   []string `json:"allowed_capabilities,omitempty"`
		DeniedCapabilities    []string `json:"denied_capabilities,omitempty"`
		AllowedExecCommands   []string `json:"allowed_exec_commands,omitempty"`
		AllowedNetworkHosts   []string `json:"allowed_network_hosts,omitempty"`
		AllowedDBDrivers      []string `json:"allowed_db_drivers,omitempty"`
		AllowedFileReadPaths  []string `json:"allowed_file_read_paths,omitempty"`
		AllowedFileWritePaths []string `json:"allowed_file_write_paths,omitempty"`
	} `json:"security,omitempty"`
	Tooling struct {
		UndefinedVariables bool     `json:"undefined_variables,omitempty"`
		Shadowing          bool     `json:"shadowing,omitempty"`
		Unreachable        bool     `json:"unreachable,omitempty"`
		DeprecatedBuiltins []string `json:"deprecated_builtins,omitempty"`
	} `json:"tooling,omitempty"`
	Test struct {
		Patterns  []string `json:"patterns,omitempty"`
		TimeoutMS int64    `json:"timeout_ms,omitempty"`
	} `json:"test,omitempty"`
	REPL struct {
		Profile string `json:"profile,omitempty"`
	} `json:"repl,omitempty"`
}

type Symbol struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Path   string `json:"path,omitempty"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type CompletionItem struct {
	Label  string `json:"label"`
	Kind   string `json:"kind,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type HoverInfo struct {
	Name   string `json:"name"`
	Kind   string `json:"kind,omitempty"`
	Detail string `json:"detail,omitempty"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
}

type PartialParseResult struct {
	Path        string       `json:"path,omitempty"`
	Complete    bool         `json:"complete"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	Program     *ast.Program `json:"-"`
}

var parserLineColRe = regexp.MustCompile(`^Line (\d+)(?::(\d+))?`)

var knownStdModuleExports = map[string][]string{
	"std/core":     {"help", "sprintf", "printf", "interpolate", "len", "type"},
	"core":         {"help", "sprintf", "printf", "interpolate", "len", "type"},
	"std/fs":       {"read_file", "write_file", "file_exists", "remove_file", "readdir", "glob", "mkdir", "rmdir", "stat"},
	"fs":           {"read_file", "write_file", "file_exists", "remove_file", "readdir", "glob", "mkdir", "rmdir", "stat"},
	"std/render":   {"file", "image", "render"},
	"render":       {"file", "image", "render"},
	"std/test":     {"assert_true", "assert_eq", "assert_neq", "assert_contains", "assert_throws", "test_summary", "run_tests"},
	"test":         {"assert_true", "assert_eq", "assert_neq", "assert_contains", "assert_throws", "test_summary", "run_tests"},
	"std/config":   {"config_load", "config_parse", "secret", "secret_reveal", "secret_mask"},
	"config":       {"config_load", "config_parse", "secret", "secret_reveal", "secret_mask"},
	"database":     {"db_connect", "db_query", "db_exec", "db_begin", "db_commit", "db_rollback", "db_tables", "db_close", "query", "lazy_query"},
	"images":       {"image_load", "image_resize", "image_crop", "image_rotate", "image_convert", "image_save", "image_info", "image_render", "image_resize_file", "image_convert_file"},
	"integrations": {"http_request", "http_get", "http_post", "webhook", "smtp_send", "ftp_list", "ftp_get", "ftp_put", "sftp_list", "sftp_get", "sftp_put"},
	"cryptoextra":  {"bcrypt_hash", "bcrypt_verify"},
	"yaml":         {"config_load", "config_parse"},
	"config/yaml":  {"config_load", "config_parse"},
	"builtins":     {},
}

var knownStdModules = func() map[string]struct{} {
	out := make(map[string]struct{}, len(knownStdModuleExports))
	for name := range knownStdModuleExports {
		out[name] = struct{}{}
	}
	return out
}()

func CheckSource(path, src string) Report {
	return analyzeSource(path, src, false)
}

func FormatSource(path, src string) Report {
	return analyzeSource(path, src, true)
}

func Analyze(path, src string, wantFormat bool) Report {
	return analyzeSource(path, src, wantFormat)
}

func ParsePartial(path, src string) PartialParseResult {
	l := lexer.NewLexer(src)
	p := parser.NewParser(l)
	program := p.ParseProgram()
	errs := p.Errors()
	return PartialParseResult{
		Path:        path,
		Complete:    len(errs) == 0,
		Diagnostics: diagnosticsFromParserErrors(path, src, errs),
		Program:     program,
	}
}

func DiagnosticsJSON(diags []Diagnostic) ([]byte, error) {
	return json.MarshalIndent(diags, "", "  ")
}

func analyzeSource(path, src string, format bool) Report {
	report := Report{Path: path, OK: true}

	l := lexer.NewLexer(src)
	p := parser.NewParser(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		report.OK = false
		report.Diagnostics = diagnosticsFromParserErrors(path, src, p.Errors())
		return report
	}

	report.Symbols = SymbolsForSource(path, src)
	report.Diagnostics = append(report.Diagnostics, StaticDiagnostics(path, src, program)...)
	for _, d := range report.Diagnostics {
		if d.Severity == SeverityError {
			report.OK = false
			break
		}
	}

	if !format {
		return report
	}

	formatted := formatProgram(src)
	report.Formatted = formatted
	report.Changed = normalizeWhitespace(src) != normalizeWhitespace(formatted)
	return report
}

func LoadProjectConfig(start string) (ProjectConfig, string, error) {
	var cfg ProjectConfig
	dir := start
	if strings.TrimSpace(dir) == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return cfg, "", err
		}
	}
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return cfg, "", err
	}
	for {
		path := filepath.Join(abs, "spl.config.json")
		data, err := os.ReadFile(path)
		if err == nil {
			if err := json.Unmarshal(data, &cfg); err != nil {
				return cfg, path, err
			}
			return cfg, path, nil
		}
		if !os.IsNotExist(err) {
			return cfg, path, err
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return cfg, "", nil
		}
		abs = parent
	}
}

func DefaultProjectConfig() ProjectConfig {
	var cfg ProjectConfig
	cfg.Runtime.MaxDepth = 256
	cfg.Runtime.MaxSteps = 2_000_000
	cfg.Runtime.MaxHeapMB = 256
	cfg.Runtime.MaxOutputBytes = 1 << 20
	cfg.Runtime.MaxHTTPBodyBytes = 1 << 20
	cfg.Runtime.MaxExecOutputBytes = 1 << 20
	cfg.Security.Profile = "trusted"
	cfg.Tooling.UndefinedVariables = true
	cfg.Tooling.Shadowing = true
	cfg.Tooling.Unreachable = true
	cfg.Test.Patterns = []string{"tests/**/*.spl", "**/*_test.spl"}
	cfg.REPL.Profile = "trusted"
	return cfg
}

func StaticDiagnostics(path, src string, program *ast.Program) []Diagnostic {
	if program == nil {
		return nil
	}
	checker := &staticChecker{
		path:           path,
		src:            src,
		scopes:         []map[string]Symbol{{}},
		diags:          []Diagnostic{},
		deprecated:     map[string]string{"puts": "prefer print or printf for new code"},
		builtinNames:   builtinNameSet(),
		declaredLines:  map[string]int{},
		declPositions:  lexicalDeclarationPositions(src),
		declCursors:    map[string]int{},
		namePositions:  lexicalNamePositions(src),
		nameCursors:    map[string]int{},
		typeVariants:   map[string][]string{},
		constructorFor: map[string]string{},
		variableTypes:  map[string]string{},
	}
	checker.seedGlobals()
	checker.walkProgram(program)
	return checker.diags
}

type staticChecker struct {
	path           string
	src            string
	scopes         []map[string]Symbol
	diags          []Diagnostic
	deprecated     map[string]string
	builtinNames   map[string]struct{}
	declaredLines  map[string]int
	declPositions  map[string][]sourcePosition
	declCursors    map[string]int
	namePositions  map[string][]sourcePosition
	nameCursors    map[string]int
	typeVariants   map[string][]string
	constructorFor map[string]string
	variableTypes  map[string]string
}

func (c *staticChecker) seedGlobals() {
	for _, name := range []string{"ARGS", "CONFIG", "true", "false", "null"} {
		c.scopes[0][name] = Symbol{Name: name, Kind: "global"}
	}
	for name := range c.builtinNames {
		c.scopes[0][name] = Symbol{Name: name, Kind: "builtin"}
	}
}

func (c *staticChecker) pushScope() { c.scopes = append(c.scopes, map[string]Symbol{}) }
func (c *staticChecker) popScope()  { c.scopes = c.scopes[:len(c.scopes)-1] }

func (c *staticChecker) declare(name, kind string) {
	c.declareWithOptions(name, kind, true, true)
}

func (c *staticChecker) declarePatternBinding(name string) {
	c.declareWithOptions(name, "variable", false, false)
}

func (c *staticChecker) declareWithOptions(name, kind string, warnDuplicate, warnOuter bool) {
	if strings.TrimSpace(name) == "" {
		return
	}
	line, col := c.nextDeclarationPosition(name)
	scope := c.scopes[len(c.scopes)-1]
	if existing, ok := scope[name]; ok && warnDuplicate && !isSeededGlobal(existing) {
		c.warn("shadow", name, line, col, fmt.Sprintf("%q is already declared in this scope", name), "rename one binding or remove the duplicate declaration")
	}
	if existing, ok := c.lookupOuter(name); ok && warnOuter && kind != "parameter" && !isSeededGlobal(existing) {
		c.warn("shadow", name, line, col, fmt.Sprintf("%q shadows an outer binding", name), "shadowing can make scripts harder to debug")
	}
	scope[name] = Symbol{Name: name, Kind: kind, Path: c.path, Line: line, Column: col}
	c.declaredLines[name] = line
}

func (c *staticChecker) lookup(name string) (Symbol, bool) {
	for i := len(c.scopes) - 1; i >= 0; i-- {
		if sym, ok := c.scopes[i][name]; ok {
			return sym, true
		}
	}
	return Symbol{}, false
}

func (c *staticChecker) lookupOuter(name string) (Symbol, bool) {
	for i := len(c.scopes) - 2; i >= 0; i-- {
		if sym, ok := c.scopes[i][name]; ok {
			return sym, true
		}
	}
	return Symbol{}, false
}

func (c *staticChecker) use(name string) {
	if strings.TrimSpace(name) == "" || isLanguageWord(name) {
		return
	}
	if _, ok := c.lookup(name); ok {
		return
	}
	line, col := c.nextNamePosition(name)
	c.warn("undefined", name, line, col, fmt.Sprintf("undefined identifier %q", name), "declare it with let/const, import it, or check the spelling")
}

func (c *staticChecker) nextNamePosition(name string) (int, int) {
	positions := c.namePositions[name]
	if len(positions) == 0 {
		return findNamePosition(c.src, name)
	}
	cursor := c.nameCursors[name]
	if cursor >= len(positions) {
		cursor = len(positions) - 1
	}
	c.nameCursors[name] = cursor + 1
	pos := positions[cursor]
	return pos.line, pos.column
}

func (c *staticChecker) nextDeclarationPosition(name string) (int, int) {
	positions := c.declPositions[name]
	if len(positions) == 0 {
		return c.nextNamePosition(name)
	}
	cursor := c.declCursors[name]
	if cursor >= len(positions) {
		cursor = len(positions) - 1
	}
	c.declCursors[name] = cursor + 1
	pos := positions[cursor]
	return pos.line, pos.column
}

func (c *staticChecker) warn(code, name string, line, col int, msg, hint string) {
	c.diags = append(c.diags, Diagnostic{
		Severity: SeverityWarning,
		Path:     c.path,
		Line:     line,
		Column:   col,
		Code:     code,
		Message:  msg,
		Hint:     hint,
		Snippet:  sourceSnippet(c.src, line, col),
	})
}

func (c *staticChecker) walkProgram(program *ast.Program) {
	for _, stmt := range program.Statements {
		c.walkStatement(stmt)
	}
}

func (c *staticChecker) walkBlock(block *ast.BlockStatement) {
	if block == nil {
		return
	}
	terminated := false
	for _, stmt := range block.Statements {
		if terminated {
			line, col := findNamePosition(c.src, firstStatementToken(stmt))
			c.warn("unreachable", "", line, col, "unreachable statement after terminating control flow", "remove the statement or move it before return/throw/break/continue")
			continue
		}
		c.walkStatement(stmt)
		switch stmt.(type) {
		case *ast.ReturnStatement, *ast.ThrowStatement, *ast.BreakStatement, *ast.ContinueStatement:
			terminated = true
		}
	}
}

func (c *staticChecker) walkStatement(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.LetStatement:
		names := s.Names
		if len(names) == 0 && s.Name != nil {
			names = []*ast.Identifier{s.Name}
		}
		for _, n := range names {
			if n != nil {
				c.declare(n.Name, "variable")
			}
		}
		c.walkExpression(s.Value)
		if typ := c.inferExpressionType(s.Value); typ != "" {
			for _, n := range names {
				if n != nil {
					c.variableTypes[n.Name] = typ
				}
			}
		}
	case *ast.DestructureLetStatement:
		c.walkExpression(s.Value)
		c.declarePatternBindings(s.Pattern)
	case *ast.ReturnStatement:
		c.walkExpression(s.ReturnValue)
	case *ast.ThrowStatement:
		c.walkExpression(s.Value)
	case *ast.ExpressionStatement:
		c.walkExpression(s.Expression)
	case *ast.PrintStatement:
		c.walkExpression(s.Expression)
	case *ast.BlockStatement:
		c.pushScope()
		c.walkBlock(s)
		c.popScope()
	case *ast.WhileStatement:
		c.walkExpression(s.Condition)
		c.pushScope()
		c.walkBlock(s.Body)
		c.popScope()
	case *ast.DoWhileStatement:
		c.pushScope()
		c.walkBlock(s.Body)
		c.popScope()
		c.walkExpression(s.Condition)
	case *ast.ForStatement:
		c.pushScope()
		if s.Init != nil {
			c.walkStatement(s.Init)
		}
		c.walkExpression(s.Condition)
		if s.Post != nil {
			c.walkStatement(s.Post)
		}
		c.walkBlock(s.Body)
		c.popScope()
	case *ast.ForInStatement:
		c.walkExpression(s.Iterable)
		c.pushScope()
		if s.KeyName != nil {
			c.declareWithOptions(s.KeyName.Name, "variable", true, false)
		}
		if s.ValueName != nil {
			c.declareWithOptions(s.ValueName.Name, "variable", true, false)
		}
		c.walkBlock(s.Body)
		c.popScope()
	case *ast.ImportStatement:
		c.checkImport(s)
	case *ast.ExportStatement:
		if s.Declaration != nil {
			c.walkStatement(s.Declaration)
		}
	case *ast.InitStatement:
		c.walkBlock(s.Body)
	case *ast.TestStatement:
		c.walkBlock(s.Body)
	case *ast.TypeDeclarationStatement:
		if s.Name != nil {
			c.declare(s.Name.Name, "type")
		}
		for _, v := range s.Variants {
			if v != nil && v.Name != nil {
				c.declare(v.Name.Name, "constructor")
				if s.Name != nil {
					c.typeVariants[s.Name.Name] = append(c.typeVariants[s.Name.Name], v.Name.Name)
					c.constructorFor[v.Name.Name] = s.Name.Name
				}
			}
		}
	case *ast.ClassStatement:
		if s.Name != nil {
			c.declare(s.Name.Name, "class")
		}
	case *ast.InterfaceStatement:
		if s.Name != nil {
			c.declare(s.Name.Name, "interface")
		}
	case *ast.SwitchStatement:
		c.walkExpression(s.Value)
		for _, sc := range s.Cases {
			for _, val := range sc.Values {
				c.walkExpression(val)
			}
			c.walkBlock(sc.Body)
		}
		c.walkBlock(s.Default)
	}
}

func (c *staticChecker) walkExpression(expr ast.Expression) {
	switch e := expr.(type) {
	case nil:
	case *ast.Identifier:
		c.use(e.Name)
	case *ast.ArrayLiteral:
		for _, el := range e.Elements {
			c.walkExpression(el)
		}
	case *ast.HashLiteral:
		for _, entry := range e.Entries {
			if entry.IsComputed || entry.IsSpread {
				c.walkExpression(entry.Key)
			}
			c.walkExpression(entry.Value)
		}
	case *ast.IndexExpression:
		c.walkExpression(e.Left)
		c.walkExpression(e.Index)
	case *ast.DotExpression:
		c.walkExpression(e.Left)
	case *ast.OptionalDotExpression:
		c.walkExpression(e.Left)
	case *ast.PrefixExpression:
		c.walkExpression(e.Right)
	case *ast.PostfixExpression:
		c.walkExpression(e.Target)
	case *ast.InfixExpression:
		c.walkExpression(e.Left)
		c.walkExpression(e.Right)
	case *ast.IfExpression:
		c.walkExpression(e.Condition)
		c.walkBlock(e.Consequence)
		c.walkBlock(e.Alternative)
	case *ast.FunctionLiteral:
		if e.Name != nil {
			if _, exists := c.scopes[len(c.scopes)-1][e.Name.Name]; !exists {
				c.declare(e.Name.Name, "function")
			}
		}
		c.pushScope()
		for _, p := range e.Parameters {
			if p != nil {
				c.declare(p.Name, "parameter")
			}
		}
		for _, def := range e.Defaults {
			c.walkExpression(def)
		}
		c.walkBlock(e.Body)
		c.popScope()
	case *ast.SpreadExpression:
		c.walkExpression(e.Value)
	case *ast.CallExpression:
		if id, ok := e.Function.(*ast.Identifier); ok {
			if msg, deprecated := c.deprecated[id.Name]; deprecated {
				line, col := findNamePosition(c.src, id.Name)
				c.warn("deprecated", id.Name, line, col, fmt.Sprintf("deprecated builtin %q", id.Name), msg)
			}
		}
		c.walkExpression(e.Function)
		for _, arg := range e.Arguments {
			c.walkExpression(arg)
		}
	case *ast.AssignExpression:
		c.walkExpression(e.Target)
		c.walkExpression(e.Value)
	case *ast.CompoundAssignExpression:
		c.walkExpression(e.Target)
		c.walkExpression(e.Value)
	case *ast.TryCatchExpression:
		c.walkBlock(e.TryBlock)
		c.pushScope()
		if e.CatchIdent != nil {
			c.declare(e.CatchIdent.Name, "variable")
		}
		c.walkBlock(e.CatchBlock)
		c.popScope()
		c.walkBlock(e.FinallyBlock)
	case *ast.TernaryExpression:
		c.walkExpression(e.Condition)
		c.walkExpression(e.Consequence)
		c.walkExpression(e.Alternative)
	case *ast.RangeExpression:
		c.walkExpression(e.Left)
		c.walkExpression(e.Right)
	case *ast.MatchExpression:
		c.walkExpression(e.Value)
		hasFallback := false
		constructorCases := map[string]struct{}{}
		for _, mc := range e.Cases {
			if mc.Guard == nil && c.isExhaustiveFallbackPattern(mc.Pattern) {
				hasFallback = true
			}
			if mc.Guard == nil {
				collectConstructorPatterns(mc.Pattern, constructorCases)
			}
			c.pushScope()
			c.declareMatchPatternBindings(mc.Pattern)
			c.walkExpression(mc.Guard)
			c.walkBlock(mc.Body)
			c.popScope()
		}
		if len(e.Cases) > 0 && !hasFallback && c.knownConstructorTypeMissingCases(e.Value, constructorCases) {
			line := e.Cases[len(e.Cases)-1].Line
			c.warn("match-exhaustiveness", "", line, 1, "match expression has no wildcard fallback", "add `case _ => { ... }` unless all variants are intentionally covered")
		}
	case *ast.AwaitExpression:
		c.walkExpression(e.Value)
	case *ast.TemplateLiteral:
		for _, part := range e.Parts {
			c.walkExpression(part)
		}
	case *ast.LazyExpression:
		c.walkExpression(e.Value)
	}
}

func (c *staticChecker) declarePatternBindings(p *ast.DestructurePattern) {
	if p == nil {
		return
	}
	for _, n := range p.Names {
		if n != nil {
			c.declare(n.Name, "variable")
		}
	}
	if p.RestName != nil {
		c.declare(p.RestName.Name, "variable")
	}
}

func (c *staticChecker) declareMatchPatternBindings(p ast.Pattern) {
	switch p := p.(type) {
	case *ast.BindingPattern:
		if p.Name != nil && p.Name.Name != "_" {
			c.declarePatternBinding(p.Name.Name)
		}
	case *ast.ArrayPattern:
		for _, child := range p.Elements {
			c.declareMatchPatternBindings(child)
		}
		if p.Rest != nil {
			c.declarePatternBinding(p.Rest.Name)
		}
	case *ast.ObjectPattern:
		for _, child := range p.Patterns {
			c.declareMatchPatternBindings(child)
		}
		if p.Rest != nil {
			c.declarePatternBinding(p.Rest.Name)
		}
	case *ast.OrPattern:
		for _, child := range p.Patterns {
			c.declareMatchPatternBindings(child)
		}
	case *ast.ExtractorPattern:
		for _, child := range p.Args {
			c.declareMatchPatternBindings(child)
		}
	case *ast.ConstructorPattern:
		for _, child := range p.Args {
			c.declareMatchPatternBindings(child)
		}
	}
}

func (c *staticChecker) inferExpressionType(expr ast.Expression) string {
	switch e := expr.(type) {
	case *ast.CallExpression:
		if id, ok := e.Function.(*ast.Identifier); ok {
			return c.constructorFor[id.Name]
		}
	case *ast.Identifier:
		return c.variableTypes[e.Name]
	}
	return ""
}

func (c *staticChecker) isExhaustiveFallbackPattern(p ast.Pattern) bool {
	switch p := p.(type) {
	case *ast.WildcardPattern:
		return true
	case *ast.BindingPattern:
		return p.TypeName == ""
	case *ast.OrPattern:
		for _, child := range p.Patterns {
			if c.isExhaustiveFallbackPattern(child) {
				return true
			}
		}
	}
	return false
}

func (c *staticChecker) knownConstructorTypeMissingCases(value ast.Expression, seen map[string]struct{}) bool {
	typ := c.inferExpressionType(value)
	if typ == "" {
		return false
	}
	variants := c.typeVariants[typ]
	if len(variants) == 0 {
		return false
	}
	for _, name := range variants {
		if _, ok := seen[name]; !ok {
			return true
		}
	}
	return false
}

func collectConstructorPatterns(p ast.Pattern, seen map[string]struct{}) {
	switch p := p.(type) {
	case *ast.ConstructorPattern:
		seen[p.Name] = struct{}{}
	case *ast.OrPattern:
		for _, child := range p.Patterns {
			collectConstructorPatterns(child, seen)
		}
	}
}

func (c *staticChecker) checkImport(s *ast.ImportStatement) {
	sl, ok := s.Path.(*ast.StringLiteral)
	if !ok {
		if s.Alias != nil {
			c.declare(s.Alias.Name, "module")
		}
		for _, n := range s.Names {
			if n != nil {
				c.declare(n.Name, "import")
			}
		}
		return
	}
	if strings.Contains(sl.Value, "://") {
		if s.Alias != nil {
			c.declare(s.Alias.Name, "module")
		}
		for _, n := range s.Names {
			if n != nil {
				c.declare(n.Name, "import")
			}
		}
		return
	}
	resolved, isStd, ok := resolveImportPathForTooling(sl.Value, c.path)
	if !ok {
		line, col := findNamePosition(c.src, sl.Value)
		c.warn("missing-import", sl.Value, line, col, fmt.Sprintf("import path %q was not found", sl.Value), "check the path, spl.mod alias, or SPL_MODULE_PATH")
		return
	}
	if s.Alias != nil {
		c.declare(s.Alias.Name, "module")
	}
	exports := map[string]Symbol{}
	if isStd {
		if stdExports, ok := knownStdModuleExports[sl.Value]; ok {
			for _, name := range stdExports {
				exports[name] = Symbol{Name: name, Kind: "function"}
			}
		}
	} else if resolved != "" {
		exports = exportedSymbolsFromFile(resolved, map[string]struct{}{c.path: {}})
	}
	if len(s.Names) > 0 {
		for _, n := range s.Names {
			if n == nil {
				continue
			}
			c.declare(n.Name, "import")
			if len(exports) > 0 {
				if _, ok := exports[n.Name]; !ok {
					line, col := c.nextNamePosition(n.Name)
					c.warn("missing-import", n.Name, line, col, fmt.Sprintf("module %q does not export %q", sl.Value, n.Name), "export it from the module or fix the imported name")
				}
			}
		}
		return
	}
	if s.Alias == nil {
		for name := range exports {
			c.declareWithOptions(name, "import", true, true)
		}
	}
}

func importPathResolvable(importPath, sourcePath string) bool {
	_, _, ok := resolveImportPathForTooling(importPath, sourcePath)
	return ok
}

func resolveImportPathForTooling(importPath, sourcePath string) (string, bool, bool) {
	importPath = strings.TrimSpace(importPath)
	if importPath == "" {
		return "", false, false
	}
	if _, ok := knownStdModules[importPath]; ok {
		return "", true, true
	}

	moduleDir := ""
	if sourcePath != "" && sourcePath != DefaultStdinPath() {
		moduleDir = filepath.Dir(sourcePath)
	}
	if resolved, matched, err := pkgmgr.ResolveManifestImport(importPath, moduleDir, sourcePath); err == nil && matched {
		if _, ok := knownStdModules[importPath]; ok {
			return "", true, true
		}
		if pathExists(resolved) {
			return resolved, false, true
		}
		return "", false, false
	}

	candidates := []string{importPath}
	if moduleDir != "" {
		candidates = append([]string{filepath.Join(moduleDir, importPath)}, candidates...)
	}
	for _, base := range importSearchPaths() {
		candidates = append(candidates, filepath.Join(base, importPath))
	}
	for _, candidate := range candidates {
		if pathExists(candidate) {
			return candidate, false, true
		}
	}
	return "", false, false
}

func exportedSymbolsFromFile(path string, stack map[string]struct{}) map[string]Symbol {
	exports := map[string]Symbol{}
	if strings.TrimSpace(path) == "" {
		return exports
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	if _, loading := stack[path]; loading {
		return exports
	}
	stack[path] = struct{}{}
	defer delete(stack, path)

	content, err := os.ReadFile(path)
	if err != nil {
		return exports
	}
	l := lexer.NewLexer(string(content))
	p := parser.NewParser(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 || program == nil {
		return exports
	}
	for _, stmt := range program.Statements {
		exportStmt, ok := stmt.(*ast.ExportStatement)
		if !ok || exportStmt.Declaration == nil {
			continue
		}
		for _, sym := range exportedSymbolsFromDeclaration(path, string(content), exportStmt.Declaration) {
			exports[sym.Name] = sym
		}
	}
	return exports
}

func exportedSymbolsFromDeclaration(path, src string, stmt ast.Statement) []Symbol {
	switch s := stmt.(type) {
	case *ast.LetStatement:
		names := s.Names
		if len(names) == 0 && s.Name != nil {
			names = []*ast.Identifier{s.Name}
		}
		symbols := make([]Symbol, 0, len(names))
		for _, n := range names {
			if n == nil {
				continue
			}
			kind := "variable"
			detail := n.Name
			if fn, ok := s.Value.(*ast.FunctionLiteral); ok {
				kind = "function"
				detail = signatureForFunctionLiteral(n.Name, fn)
			}
			line, col := findNamePosition(src, n.Name)
			symbols = append(symbols, Symbol{Name: n.Name, Kind: kind, Path: path, Line: line, Column: col, Detail: detail})
		}
		return symbols
	case *ast.DestructureLetStatement:
		names := map[string]struct{}{}
		collectDestructurePatternNames(s.Pattern, names)
		symbols := make([]Symbol, 0, len(names))
		for name := range names {
			line, col := findNamePosition(src, name)
			symbols = append(symbols, Symbol{Name: name, Kind: "variable", Path: path, Line: line, Column: col, Detail: name})
		}
		return symbols
	default:
		return nil
	}
}

func collectDestructurePatternNames(p *ast.DestructurePattern, names map[string]struct{}) {
	if p == nil {
		return
	}
	for _, n := range p.Names {
		if n != nil {
			names[n.Name] = struct{}{}
		}
	}
	if p.RestName != nil {
		names[p.RestName.Name] = struct{}{}
	}
}

func pathExists(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	}
	return false
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

func builtinNameSet() map[string]struct{} {
	out := map[string]struct{}{}
	for _, name := range eval.BuiltinNames() {
		out[name] = struct{}{}
	}
	for name := range eval.BuiltinHelpDescriptions {
		out[name] = struct{}{}
	}
	for name := range splRuntimeDocs {
		out[name] = struct{}{}
	}
	return out
}

func isSeededGlobal(sym Symbol) bool {
	return sym.Kind == "builtin" || sym.Kind == "global"
}

func isLanguageWord(name string) bool {
	switch name {
	case "let", "const", "function", "return", "if", "else", "for", "while", "do", "in", "and", "or", "not", "break", "continue", "true", "false", "null", "import", "export", "try", "catch", "throw", "switch", "case", "default", "match", "type", "class", "interface", "test", "init", "async", "await", "lazy":
		return true
	default:
		return false
	}
}

func firstStatementToken(stmt ast.Statement) string {
	switch stmt.(type) {
	case *ast.ReturnStatement:
		return "return"
	case *ast.ThrowStatement:
		return "throw"
	case *ast.BreakStatement:
		return "break"
	case *ast.ContinueStatement:
		return "continue"
	case *ast.LetStatement:
		return "let"
	case *ast.PrintStatement:
		return "print"
	default:
		if stmt == nil {
			return ""
		}
		return strings.Fields(stmt.String())[0]
	}
}

type sourcePosition struct {
	line   int
	column int
}

type lexicalToken struct {
	typ     token.TokenType
	literal string
	pos     sourcePosition
}

func lexicalTokens(src string) []lexicalToken {
	tokens := []lexicalToken{}
	l := lexer.NewLexer(src)
	for {
		tok := l.NextToken()
		if tok.Type == token.EOF {
			break
		}
		if tok.Type == token.ILLEGAL || tok.Literal == "" {
			continue
		}
		tokens = append(tokens, lexicalToken{
			typ:     tok.Type,
			literal: tok.Literal,
			pos:     sourcePosition{line: tok.Line, column: tok.Column},
		})
	}
	return tokens
}

func lexicalNamePositions(src string) map[string][]sourcePosition {
	positions := map[string][]sourcePosition{}
	for _, tok := range lexicalTokens(src) {
		positions[tok.literal] = append(positions[tok.literal], tok.pos)
	}
	return positions
}

func lexicalDeclarationPositions(src string) map[string][]sourcePosition {
	positions := map[string][]sourcePosition{}
	tokens := lexicalTokens(src)
	add := func(tok lexicalToken) {
		if tok.typ == token.IDENT {
			positions[tok.literal] = append(positions[tok.literal], tok.pos)
		}
	}
	for i := 0; i < len(tokens); i++ {
		switch tokens[i].typ {
		case token.LET, token.CONST:
			for j := i + 1; j < len(tokens); j++ {
				switch tokens[j].typ {
				case token.ASSIGN, token.SEMICOLON, token.LBRACE:
					j = len(tokens)
				default:
					add(tokens[j])
				}
			}
		case token.FUNCTION:
			paramStart := i + 1
			if paramStart < len(tokens) && tokens[paramStart].typ == token.IDENT {
				add(tokens[paramStart])
				paramStart++
			}
			for paramStart < len(tokens) && tokens[paramStart].typ != token.LPAREN {
				paramStart++
			}
			if paramStart < len(tokens) && tokens[paramStart].typ == token.LPAREN {
				depth := 0
				for j := paramStart; j < len(tokens); j++ {
					switch tokens[j].typ {
					case token.LPAREN:
						depth++
					case token.RPAREN:
						depth--
						if depth == 0 {
							j = len(tokens)
						}
					case token.IDENT:
						if depth == 1 {
							add(tokens[j])
						}
					}
				}
			}
		case token.FOR:
			for j := i + 1; j < len(tokens); j++ {
				if tokens[j].typ == token.IN {
					break
				}
				if tokens[j].typ == token.LBRACE || tokens[j].typ == token.SEMICOLON {
					break
				}
				add(tokens[j])
			}
		case token.IDENT:
			if tokens[i].literal != "type" {
				continue
			}
			for j := i + 1; j < len(tokens); j++ {
				switch tokens[j].typ {
				case token.SEMICOLON, token.LBRACE:
					j = len(tokens)
				default:
					add(tokens[j])
				}
			}
		case token.IMPORT:
			for j := i + 1; j < len(tokens); j++ {
				if tokens[j].typ == token.SEMICOLON {
					break
				}
				add(tokens[j])
			}
		}
	}
	return positions
}

func findNamePosition(src, name string) (int, int) {
	if name == "" {
		return 0, 0
	}
	positions := lexicalNamePositions(src)
	if matches := positions[name]; len(matches) > 0 {
		return matches[0].line, matches[0].column
	}
	return 0, 0
}

func SymbolsForSource(path, src string) []Symbol {
	l := lexer.NewLexer(src)
	p := parser.NewParser(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 || program == nil {
		return nil
	}
	syms := []Symbol{}
	add := func(name, kind, detail string) {
		line, col := findNamePosition(src, name)
		syms = append(syms, Symbol{Name: name, Kind: kind, Path: path, Line: line, Column: col, Detail: detail})
	}
	var walkStmt func(ast.Statement)
	var walkExpr func(ast.Expression)
	walkBlock := func(block *ast.BlockStatement) {
		if block == nil {
			return
		}
		for _, stmt := range block.Statements {
			walkStmt(stmt)
		}
	}
	walkExpr = func(expr ast.Expression) {
		switch e := expr.(type) {
		case *ast.FunctionLiteral:
			if e.Name != nil {
				add(e.Name.Name, "function", signatureForFunctionLiteral(e.Name.Name, e))
			}
			walkBlock(e.Body)
		case *ast.IfExpression:
			walkBlock(e.Consequence)
			walkBlock(e.Alternative)
		case *ast.CallExpression:
			for _, arg := range e.Arguments {
				walkExpr(arg)
			}
		case *ast.MatchExpression:
			for _, mc := range e.Cases {
				walkBlock(mc.Body)
			}
		}
	}
	walkStmt = func(stmt ast.Statement) {
		switch s := stmt.(type) {
		case *ast.LetStatement:
			names := s.Names
			if len(names) == 0 && s.Name != nil {
				names = []*ast.Identifier{s.Name}
			}
			for _, n := range names {
				if n != nil {
					kind := "variable"
					detail := ""
					if fn, ok := s.Value.(*ast.FunctionLiteral); ok {
						kind = "function"
						detail = signatureForFunctionLiteral(n.Name, fn)
					}
					add(n.Name, kind, detail)
				}
			}
			walkExpr(s.Value)
		case *ast.ImportStatement:
			if s.Alias != nil {
				add(s.Alias.Name, "module", s.Path.String())
			}
			for _, n := range s.Names {
				if n != nil {
					add(n.Name, "import", s.Path.String())
				}
			}
		case *ast.TypeDeclarationStatement:
			if s.Name != nil {
				add(s.Name.Name, "type", s.String())
			}
			for _, v := range s.Variants {
				if v != nil && v.Name != nil {
					add(v.Name.Name, "constructor", s.Name.String())
				}
			}
		case *ast.ClassStatement:
			if s.Name != nil {
				add(s.Name.Name, "class", s.String())
			}
		case *ast.InterfaceStatement:
			if s.Name != nil {
				add(s.Name.Name, "interface", s.String())
			}
		case *ast.TestStatement:
			add(s.Name, "test", s.Name)
		case *ast.ExpressionStatement:
			walkExpr(s.Expression)
		case *ast.BlockStatement:
			walkBlock(s)
		case *ast.ExportStatement:
			if s.Declaration != nil {
				walkStmt(s.Declaration)
			}
		}
	}
	for _, stmt := range program.Statements {
		walkStmt(stmt)
	}
	sort.Slice(syms, func(i, j int) bool {
		if syms[i].Line == syms[j].Line {
			return syms[i].Column < syms[j].Column
		}
		return syms[i].Line < syms[j].Line
	})
	return syms
}

func VisibleSymbolsForSource(path, src string) []Symbol {
	syms := SymbolsForSource(path, src)
	syms = append(syms, importedSymbolsForSource(path, src)...)
	sort.Slice(syms, func(i, j int) bool {
		if syms[i].Name == syms[j].Name {
			return syms[i].Path < syms[j].Path
		}
		return syms[i].Name < syms[j].Name
	})
	return syms
}

func importedSymbolsForSource(path, src string) []Symbol {
	l := lexer.NewLexer(src)
	p := parser.NewParser(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 || program == nil {
		return nil
	}
	out := []Symbol{}
	for _, stmt := range program.Statements {
		importStmt, ok := stmt.(*ast.ImportStatement)
		if !ok || importStmt.Alias != nil {
			continue
		}
		sl, ok := importStmt.Path.(*ast.StringLiteral)
		if !ok || strings.Contains(sl.Value, "://") {
			continue
		}
		resolved, isStd, ok := resolveImportPathForTooling(sl.Value, path)
		if !ok || isStd || resolved == "" {
			continue
		}
		exports := exportedSymbolsFromFile(resolved, map[string]struct{}{})
		if len(importStmt.Names) == 0 {
			for _, sym := range exports {
				out = append(out, sym)
			}
			continue
		}
		for _, n := range importStmt.Names {
			if n == nil {
				continue
			}
			if sym, ok := exports[n.Name]; ok {
				out = append(out, sym)
			}
		}
	}
	return out
}

func signatureForFunctionLiteral(name string, fn *ast.FunctionLiteral) string {
	parts := make([]string, 0, len(fn.Parameters))
	for i, p := range fn.Parameters {
		part := "arg"
		if p != nil {
			part = p.Name
		}
		if fn.HasRest && i == len(fn.Parameters)-1 {
			part = "..." + part
		}
		if i < len(fn.ParamTypes) && fn.ParamTypes[i] != "" {
			part += ": " + fn.ParamTypes[i]
		}
		parts = append(parts, part)
	}
	out := fmt.Sprintf("%s(%s)", name, strings.Join(parts, ", "))
	if fn.ReturnType != "" {
		out += " -> " + fn.ReturnType
	}
	return out
}

func CompletionItems(path, src, prefix string) []CompletionItem {
	items := []CompletionItem{}
	for _, name := range eval.BuiltinNames() {
		if strings.HasPrefix(name, prefix) {
			items = append(items, CompletionItem{Label: name, Kind: "builtin", Detail: eval.BuiltinHelpText(name)})
		}
	}
	for _, sym := range VisibleSymbolsForSource(path, src) {
		if strings.HasPrefix(sym.Name, prefix) {
			items = append(items, CompletionItem{Label: sym.Name, Kind: sym.Kind, Detail: sym.Detail})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
	return items
}

func HoverAt(path, src string, line, col int) HoverInfo {
	word := wordAt(src, line, col)
	if word == "" {
		return HoverInfo{}
	}
	if detail := KeywordMarkdown(word); detail != "" {
		return HoverInfo{Name: word, Kind: "keyword", Detail: detail, Line: line, Column: col}
	}
	if detail := RuntimeDocMarkdown(word); detail != "" {
		return HoverInfo{Name: word, Kind: "builtin", Detail: detail, Line: line, Column: col}
	}
	if eval.HasBuiltin(word) {
		return HoverInfo{Name: word, Kind: "builtin", Detail: eval.BuiltinHelpText(word), Line: line, Column: col}
	}
	for _, sym := range VisibleSymbolsForSource(path, src) {
		if sym.Name == word {
			return HoverInfo{Name: sym.Name, Kind: sym.Kind, Detail: sym.Detail, Line: sym.Line, Column: sym.Column}
		}
	}
	return HoverInfo{Name: word, Kind: "identifier", Line: line, Column: col}
}

func wordAt(src string, line, col int) string {
	lines := strings.Split(src, "\n")
	if line <= 0 || line > len(lines) {
		return ""
	}
	runes := []rune(lines[line-1])
	idx := col - 1
	if idx < 0 || idx >= len(runes) {
		return ""
	}
	start := idx
	for start > 0 && (unicode.IsLetter(runes[start-1]) || unicode.IsDigit(runes[start-1]) || runes[start-1] == '_') {
		start--
	}
	end := idx
	for end < len(runes) && (unicode.IsLetter(runes[end]) || unicode.IsDigit(runes[end]) || runes[end] == '_') {
		end++
	}
	return string(runes[start:end])
}

func WordAt(src string, line, col int) string {
	return wordAt(src, line, col)
}

func DocsMarkdown(path, src string) string {
	symbols := SymbolsForSource(path, src)
	var b strings.Builder
	title := filepath.Base(path)
	if path == DefaultStdinPath() || path == "" {
		title = "SPL Module"
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	if len(symbols) == 0 {
		b.WriteString("No exported or top-level symbols found.\n")
		return b.String()
	}
	groups := map[string][]Symbol{}
	order := []string{"type", "constructor", "class", "interface", "function", "variable", "module", "import", "test"}
	for _, sym := range symbols {
		groups[sym.Kind] = append(groups[sym.Kind], sym)
	}
	for _, kind := range order {
		items := groups[kind]
		if len(items) == 0 {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n", strings.Title(kind)+"s")
		for _, sym := range items {
			detail := sym.Detail
			if detail == "" {
				detail = sym.Name
			}
			fmt.Fprintf(&b, "- `%s`", sym.Name)
			if detail != sym.Name {
				fmt.Fprintf(&b, " - `%s`", detail)
			}
			if sym.Line > 0 {
				fmt.Fprintf(&b, " _(line %d)_", sym.Line)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func diagnosticsFromParserErrors(path, src string, errs []string) []Diagnostic {
	diags := make([]Diagnostic, 0, len(errs))
	for _, raw := range errs {
		line, col := parseLineCol(raw)
		diag := Diagnostic{
			Severity: SeverityError,
			Path:     path,
			Line:     line,
			Column:   col,
			Message:  raw,
		}
		if line > 0 {
			diag.Snippet = sourceSnippet(src, line, col)
		}
		diags = append(diags, diag)
	}
	return diags
}

func parseLineCol(msg string) (int, int) {
	m := parserLineColRe.FindStringSubmatch(msg)
	if m == nil {
		return 0, 0
	}
	var line, col int
	fmt.Sscanf(m[1], "%d", &line)
	if m[2] != "" {
		fmt.Sscanf(m[2], "%d", &col)
	}
	return line, col
}

func sourceSnippet(src string, line, col int) string {
	if line <= 0 {
		return ""
	}
	lines := strings.Split(src, "\n")
	if line > len(lines) {
		return ""
	}
	text := lines[line-1]
	if strings.TrimSpace(text) == "" {
		return ""
	}
	if col < 1 {
		col = 1
	}
	return fmt.Sprintf("%s\n%s^", text, strings.Repeat(" ", col-1))
}

func formatProgram(src string) string {
	var out strings.Builder
	indent := 0
	inString := false
	escaped := false
	needSpace := false

	writeIndent := func() {
		if out.Len() == 0 {
			return
		}
		last := out.String()
		if len(last) == 0 || last[len(last)-1] == '\n' {
			out.WriteString(strings.Repeat("  ", indent))
		}
	}

	flushSpace := func() {
		if needSpace {
			out.WriteByte(' ')
			needSpace = false
		}
	}

	for _, r := range src {
		if inString {
			out.WriteRune(r)
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				inString = false
			}
			continue
		}

		switch r {
		case '"':
			writeIndent()
			flushSpace()
			inString = true
			out.WriteRune(r)
		case '{':
			flushSpace()
			out.WriteString(" {")
			out.WriteByte('\n')
			indent++
			out.WriteString(strings.Repeat("  ", indent))
		case '}':
			trimTrailingSpace(&out)
			out.WriteByte('\n')
			if indent > 0 {
				indent--
			}
			out.WriteString(strings.Repeat("  ", indent))
			out.WriteByte('}')
			needSpace = true
		case ';':
			out.WriteByte(';')
			out.WriteByte('\n')
			out.WriteString(strings.Repeat("  ", indent))
			needSpace = false
		case '\n', '\r', '\t':
			needSpace = true
		default:
			if unicode.IsSpace(r) {
				needSpace = true
				continue
			}
			writeIndent()
			flushSpace()
			out.WriteRune(r)
		}
	}

	trimTrailingSpace(&out)
	formatted := strings.TrimSpace(out.String())
	if formatted == "" {
		return ""
	}
	return formatted + "\n"
}

func trimTrailingSpace(out *strings.Builder) {
	s := out.String()
	s = strings.TrimRight(s, " \t")
	out.Reset()
	out.WriteString(s)
}

func normalizeWhitespace(src string) string {
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func DefaultStdinPath() string {
	return filepath.Clean("<stdin>")
}
