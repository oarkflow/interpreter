package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/eval"
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

var parserLineColRe = regexp.MustCompile(`^Line (\d+)(?::(\d+))?`)

func CheckSource(path, src string) Report {
	return analyzeSource(path, src, false)
}

func FormatSource(path, src string) Report {
	return analyzeSource(path, src, true)
}

func Analyze(path, src string, wantFormat bool) Report {
	return analyzeSource(path, src, wantFormat)
}

func DiagnosticsJSON(diags []Diagnostic) ([]byte, error) {
	return json.MarshalIndent(diags, "", "  ")
}

func analyzeSource(path, src string, format bool) Report {
	report := Report{Path: path, OK: true}

	l := interpreter.NewLexer(src)
	p := interpreter.NewParser(l)
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
		path:          path,
		src:           src,
		scopes:        []map[string]Symbol{{}},
		diags:         []Diagnostic{},
		deprecated:    map[string]string{"puts": "prefer print or printf for new code"},
		builtinNames:  builtinNameSet(),
		declaredLines: map[string]int{},
	}
	checker.seedGlobals()
	checker.walkProgram(program)
	return checker.diags
}

type staticChecker struct {
	path          string
	src           string
	scopes        []map[string]Symbol
	diags         []Diagnostic
	deprecated    map[string]string
	builtinNames  map[string]struct{}
	declaredLines map[string]int
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
	if strings.TrimSpace(name) == "" {
		return
	}
	line, col := findNamePosition(c.src, name)
	scope := c.scopes[len(c.scopes)-1]
	if _, ok := scope[name]; ok {
		c.warn("shadow", name, line, col, fmt.Sprintf("%q is already declared in this scope", name), "rename one binding or remove the duplicate declaration")
	}
	if _, ok := c.lookupOuter(name); ok && kind != "parameter" {
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
	line, col := findNamePosition(c.src, name)
	c.warn("undefined", name, line, col, fmt.Sprintf("undefined identifier %q", name), "declare it with let/const, import it, or check the spelling")
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
			c.declare(s.KeyName.Name, "variable")
		}
		if s.ValueName != nil {
			c.declare(s.ValueName.Name, "variable")
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
			c.declare(e.Name.Name, "function")
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
		hasWildcard := false
		for _, mc := range e.Cases {
			if _, ok := mc.Pattern.(*ast.WildcardPattern); ok {
				hasWildcard = true
			}
			c.pushScope()
			c.declareMatchPatternBindings(mc.Pattern)
			c.walkExpression(mc.Guard)
			c.walkBlock(mc.Body)
			c.popScope()
		}
		if len(e.Cases) > 0 && !hasWildcard {
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
			c.declare(p.Name.Name, "variable")
		}
	case *ast.ArrayPattern:
		for _, child := range p.Elements {
			c.declareMatchPatternBindings(child)
		}
		if p.Rest != nil {
			c.declare(p.Rest.Name, "variable")
		}
	case *ast.ObjectPattern:
		for _, child := range p.Patterns {
			c.declareMatchPatternBindings(child)
		}
		if p.Rest != nil {
			c.declare(p.Rest.Name, "variable")
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

func (c *staticChecker) checkImport(s *ast.ImportStatement) {
	if s.Alias != nil {
		c.declare(s.Alias.Name, "module")
	}
	for _, n := range s.Names {
		if n != nil {
			c.declare(n.Name, "import")
		}
	}
	sl, ok := s.Path.(*ast.StringLiteral)
	if !ok {
		return
	}
	if strings.Contains(sl.Value, "://") {
		return
	}
	candidates := []string{sl.Value}
	if c.path != "" && c.path != DefaultStdinPath() {
		candidates = append([]string{filepath.Join(filepath.Dir(c.path), sl.Value)}, candidates...)
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return
		}
	}
	line, col := findNamePosition(c.src, sl.Value)
	c.warn("missing-import", sl.Value, line, col, fmt.Sprintf("import path %q was not found", sl.Value), "check the path, spl.mod alias, or SPL_MODULE_PATH")
}

func builtinNameSet() map[string]struct{} {
	out := map[string]struct{}{}
	for _, name := range eval.BuiltinNames() {
		out[name] = struct{}{}
	}
	return out
}

func isLanguageWord(name string) bool {
	switch name {
	case "let", "const", "function", "return", "if", "else", "for", "while", "do", "in", "break", "continue", "true", "false", "null", "import", "export", "try", "catch", "throw", "switch", "case", "default", "match", "type", "class", "interface", "test", "init", "async", "await", "lazy":
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

func findNamePosition(src, name string) (int, int) {
	if name == "" {
		return 0, 0
	}
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		if idx := strings.Index(line, name); idx >= 0 {
			return i + 1, idx + 1
		}
	}
	return 0, 0
}

func SymbolsForSource(path, src string) []Symbol {
	l := interpreter.NewLexer(src)
	p := interpreter.NewParser(l)
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
	for _, sym := range SymbolsForSource(path, src) {
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
	if eval.HasBuiltin(word) {
		return HoverInfo{Name: word, Kind: "builtin", Detail: eval.BuiltinHelpText(word), Line: line, Column: col}
	}
	for _, sym := range SymbolsForSource(path, src) {
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
