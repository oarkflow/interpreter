package tooling

import (
	"sort"

	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/lexer"
	"github.com/oarkflow/interpreter/pkg/parser"
	"github.com/oarkflow/interpreter/pkg/security"
	"github.com/oarkflow/interpreter/pkg/token"
)

// EffectUsage is a single occurrence of a capability-relevant builtin call
// (or, for the "dynamic-import" category, a dynamic import statement) found
// while statically walking a script's AST.
type EffectUsage struct {
	Builtin string `json:"builtin"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
}

// EffectFinding groups every usage found for a single capability (or the
// synthetic "dynamic-import" category) in a script.
type EffectFinding struct {
	Capability string        `json:"capability"`
	Usages     []EffectUsage `json:"usages"`
}

// EffectsReport is the result of the static capability/effects analysis for
// one script: what it MIGHT do at runtime, without running it.
type EffectsReport struct {
	Path         string          `json:"path,omitempty"`
	OK           bool            `json:"ok"`
	Capabilities []EffectFinding `json:"capabilities,omitempty"`
	Diagnostics  []Diagnostic    `json:"diagnostics,omitempty"`
}

// DynamicImportCapability is the synthetic category used to report an
// import statement whose path is not a string literal (see
// security.SecurityPolicy.DenyDynamicImports / pkg/eval/eval.go's
// evalImportStatement for the runtime-enforced equivalent of this check).
const DynamicImportCapability = "dynamic-import"

// AnalyzeEffects parses src and walks its AST to determine which security
// capabilities the script might exercise at runtime (network, filesystem
// read/write, exec, db, secrets, scheduler/async/server/watch/process-exit,
// ...), plus any dynamic (non-literal-path) imports, WITHOUT running the
// script. It is the data source for `spltool check --effects`.
func AnalyzeEffects(path, src string) EffectsReport {
	report := EffectsReport{Path: path, OK: true}

	l := lexer.NewLexer(src)
	p := parser.NewParser(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 || program == nil {
		report.OK = false
		report.Diagnostics = diagnosticsFromParserErrors(path, src, p.Errors())
		return report
	}

	checker := &staticChecker{
		path:            path,
		src:             src,
		scopes:          []map[string]Symbol{{}},
		diags:           []Diagnostic{},
		deprecated:      map[string]string{},
		builtinNames:    builtinNameSet(),
		declaredLines:   map[string]int{},
		declPositions:   lexicalDeclarationPositions(src),
		declCursors:     map[string]int{},
		namePositions:   lexicalNamePositions(src),
		nameCursors:     map[string]int{},
		typeVariants:    map[string][]string{},
		constructorFor:  map[string]string{},
		variableTypes:   map[string]string{},
		macroDefs:       map[string]*ast.MacroDefinition{},
		moduleAliases:   map[string]string{},
		importPositions: importTokenPositions(src),
	}
	checker.seedGlobals()

	byCapability := map[string]map[EffectUsage]struct{}{}
	addUsage := func(capability string, usage EffectUsage) {
		set, ok := byCapability[capability]
		if !ok {
			set = map[EffectUsage]struct{}{}
			byCapability[capability] = set
		}
		set[usage] = struct{}{}
	}

	checker.onCallForEffects = func(builtinName string, line, col int) {
		for _, cap := range security.CapabilitiesForBuiltin(builtinName) {
			addUsage(cap, EffectUsage{Builtin: builtinName, Line: line, Column: col})
		}
	}
	checker.onImportForEffects = func(line, col int, dynamic bool) {
		if dynamic {
			addUsage(DynamicImportCapability, EffectUsage{Builtin: "import", Line: line, Column: col})
		}
	}

	checker.walkProgram(program)

	capabilities := make([]string, 0, len(byCapability))
	for cap := range byCapability {
		capabilities = append(capabilities, cap)
	}
	sort.Strings(capabilities)

	for _, cap := range capabilities {
		usages := make([]EffectUsage, 0, len(byCapability[cap]))
		for usage := range byCapability[cap] {
			usages = append(usages, usage)
		}
		sort.Slice(usages, func(i, j int) bool {
			if usages[i].Line != usages[j].Line {
				return usages[i].Line < usages[j].Line
			}
			if usages[i].Column != usages[j].Column {
				return usages[i].Column < usages[j].Column
			}
			return usages[i].Builtin < usages[j].Builtin
		})
		report.Capabilities = append(report.Capabilities, EffectFinding{Capability: cap, Usages: usages})
	}

	return report
}

// importTokenPositions returns the source position of every "import"
// keyword token in src, in source order. staticChecker.nextImportPosition
// consumes one entry per ast.ImportStatement visited during the walk, which
// lines up with lexer order for ordinary (non-macro-generated) source.
func importTokenPositions(src string) []sourcePosition {
	var positions []sourcePosition
	for _, tok := range lexicalTokens(src) {
		if tok.typ == token.IMPORT {
			positions = append(positions, tok.pos)
		}
	}
	return positions
}
