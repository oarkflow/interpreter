package ast

import (
	"fmt"
	"strings"

	"github.com/oarkflow/interpreter/pkg/token"
)

// Node is the interface implemented by all AST nodes.
type Node interface {
	String() string
}

// Expression is the interface for all expression nodes.
type Expression interface {
	Node
	expressionNode()
}

// Statement is the interface for all statement nodes.
type Statement interface {
	Node
	statementNode()
}

// Program is the root node of the AST.
type Program struct {
	Statements []Statement
}

func (p *Program) String() string {
	var out strings.Builder
	for _, s := range p.Statements {
		out.WriteString(s.String())
	}
	return out.String()
}

type IntegerLiteral struct {
	Value int64
}

func (il *IntegerLiteral) expressionNode() {}
func (il *IntegerLiteral) String() string  { return fmt.Sprintf("%d", il.Value) }

type FloatLiteral struct {
	Value float64
}

func (fl *FloatLiteral) expressionNode() {}
func (fl *FloatLiteral) String() string  { return fmt.Sprintf("%g", fl.Value) }

type StringLiteral struct {
	Value string
}

func (sl *StringLiteral) expressionNode() {}
func (sl *StringLiteral) String() string  { return fmt.Sprintf("\"%s\"", sl.Value) }

type BooleanLiteral struct {
	Value bool
}

func (bl *BooleanLiteral) expressionNode() {}
func (bl *BooleanLiteral) String() string  { return fmt.Sprintf("%t", bl.Value) }

type NullLiteral struct{}

func (nl *NullLiteral) expressionNode() {}
func (nl *NullLiteral) String() string  { return "null" }

type Identifier struct {
	Name string
}

func (i *Identifier) expressionNode() {}
func (i *Identifier) String() string  { return i.Name }

type ArrayLiteral struct {
	Elements []Expression
}

func (al *ArrayLiteral) expressionNode() {}
func (al *ArrayLiteral) String() string {
	var out strings.Builder
	out.WriteString("[")
	for i, el := range al.Elements {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(el.String())
	}
	out.WriteString("]")
	return out.String()
}

type HashEntry struct {
	Key        Expression
	Value      Expression
	IsSpread   bool
	IsComputed bool
}

type HashLiteral struct {
	Entries []HashEntry
}

func (hl *HashLiteral) expressionNode() {}
func (hl *HashLiteral) String() string {
	var out strings.Builder
	pairs := []string{}
	for _, entry := range hl.Entries {
		if entry.IsSpread {
			pairs = append(pairs, "..."+entry.Value.String())
		} else {
			pairs = append(pairs, entry.Key.String()+":"+entry.Value.String())
		}
	}
	out.WriteString("{")
	out.WriteString(strings.Join(pairs, ", "))
	out.WriteString("}")
	return out.String()
}

type IndexExpression struct {
	Left  Expression
	Index Expression
}

func (ie *IndexExpression) expressionNode() {}
func (ie *IndexExpression) String() string {
	return fmt.Sprintf("(%s[%s])", ie.Left.String(), ie.Index.String())
}

type DotExpression struct {
	Left  Expression
	Right *Identifier
}

func (de *DotExpression) expressionNode() {}
func (de *DotExpression) String() string {
	return fmt.Sprintf("(%s.%s)", de.Left.String(), de.Right.String())
}

type OptionalDotExpression struct {
	Left  Expression
	Right *Identifier
}

func (od *OptionalDotExpression) expressionNode() {}
func (od *OptionalDotExpression) String() string {
	return "(" + od.Left.String() + "?." + od.Right.String() + ")"
}

type PrefixExpression struct {
	Operator string
	Right    Expression
}

func (pe *PrefixExpression) expressionNode() {}
func (pe *PrefixExpression) String() string {
	return fmt.Sprintf("(%s%s)", pe.Operator, pe.Right.String())
}

type PostfixExpression struct {
	Operator string
	Target   Expression
}

func (pe *PostfixExpression) expressionNode() {}
func (pe *PostfixExpression) String() string {
	return "(" + pe.Target.String() + pe.Operator + ")"
}

type InfixExpression struct {
	Left     Expression
	Operator string
	Right    Expression
}

func (ie *InfixExpression) expressionNode() {}
func (ie *InfixExpression) String() string {
	return fmt.Sprintf("(%s %s %s)", ie.Left.String(), ie.Operator, ie.Right.String())
}

type IfExpression struct {
	Condition   Expression
	Consequence *BlockStatement
	Alternative *BlockStatement
}

func (ie *IfExpression) expressionNode() {}
func (ie *IfExpression) String() string {
	return fmt.Sprintf("if %s %s else %s", ie.Condition.String(), ie.Consequence.String(), ie.Alternative.String())
}

type FunctionLiteral struct {
	Name       *Identifier
	Parameters []*Identifier
	ParamTypes []string
	Defaults   []Expression
	ReturnType string
	HasRest    bool
	Body       *BlockStatement
	IsArrow    bool
	IsAsync    bool
}

func (fl *FunctionLiteral) expressionNode() {}
func (fl *FunctionLiteral) String() string {
	var out strings.Builder
	if fl.Name != nil {
		out.WriteString("function ")
		out.WriteString(fl.Name.String())
		out.WriteString("(")
	} else {
		out.WriteString("function(")
	}
	for i, p := range fl.Parameters {
		if i > 0 {
			out.WriteString(", ")
		}
		if fl.HasRest && i == len(fl.Parameters)-1 {
			out.WriteString("...")
		}
		out.WriteString(p.String())
		if i < len(fl.ParamTypes) && fl.ParamTypes[i] != "" {
			out.WriteString(": ")
			out.WriteString(fl.ParamTypes[i])
		}
		if i < len(fl.Defaults) && fl.Defaults[i] != nil {
			out.WriteString(" = ")
			out.WriteString(fl.Defaults[i].String())
		}
	}
	out.WriteString(") ")
	if fl.ReturnType != "" {
		out.WriteString(": ")
		out.WriteString(fl.ReturnType)
		out.WriteString(" ")
	}
	out.WriteString(fl.Body.String())
	return out.String()
}

type SpreadExpression struct {
	Value Expression
}

func (se *SpreadExpression) expressionNode() {}
func (se *SpreadExpression) String() string  { return "..." + se.Value.String() }

type CallExpression struct {
	Function  Expression
	Arguments []Expression
	Line      int
	Column    int
}

func (ce *CallExpression) expressionNode() {}
func (ce *CallExpression) String() string {
	var out strings.Builder
	out.WriteString(ce.Function.String())
	out.WriteString("(")
	for i, a := range ce.Arguments {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(a.String())
	}
	out.WriteString(")")
	return out.String()
}

type AssignExpression struct {
	Target Expression
	Value  Expression
}

func (ae *AssignExpression) expressionNode() {}
func (ae *AssignExpression) String() string {
	return fmt.Sprintf("%s = %s", ae.Target.String(), ae.Value.String())
}

type CompoundAssignExpression struct {
	Target   Expression
	Operator string
	Value    Expression
}

func (ca *CompoundAssignExpression) expressionNode() {}
func (ca *CompoundAssignExpression) String() string {
	return "(" + ca.Target.String() + " " + ca.Operator + "= " + ca.Value.String() + ")"
}

type LetStatement struct {
	Name     *Identifier
	Names    []*Identifier
	TypeName string
	Value    Expression
}

func (ls *LetStatement) statementNode() {}
func (ls *LetStatement) String() string {
	if ls.TypeName != "" {
		return fmt.Sprintf("let %s: %s = %s;", ls.Name.String(), ls.TypeName, ls.Value.String())
	}
	return fmt.Sprintf("let %s = %s;", ls.Name.String(), ls.Value.String())
}

type ClassStatement struct {
	Name    *Identifier
	Methods []*ClassMethod
}

type ClassMethod struct {
	Name       *Identifier
	Parameters []*Identifier
	Body       *BlockStatement
}

func (cs *ClassStatement) statementNode() {}
func (cs *ClassStatement) String() string {
	var out strings.Builder
	out.WriteString("class ")
	out.WriteString(cs.Name.String())
	out.WriteString(" {")
	for _, m := range cs.Methods {
		out.WriteByte(' ')
		out.WriteString(m.String())
	}
	out.WriteString(" }")
	return out.String()
}

func (cm *ClassMethod) String() string {
	var out strings.Builder
	out.WriteString(cm.Name.String())
	out.WriteString("(")
	for i, p := range cm.Parameters {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(p.String())
	}
	out.WriteString(") ")
	out.WriteString(cm.Body.String())
	return out.String()
}

type InterfaceStatement struct {
	Name    *Identifier
	Methods []*InterfaceMethod
}

type InterfaceMethod struct {
	Name       *Identifier
	Parameters []*Identifier
	ParamTypes []string
	ReturnType string
}

func (is *InterfaceStatement) statementNode() {}
func (is *InterfaceStatement) String() string {
	var out strings.Builder
	out.WriteString("interface ")
	out.WriteString(is.Name.String())
	out.WriteString(" {")
	for _, m := range is.Methods {
		out.WriteByte(' ')
		out.WriteString(m.String())
	}
	out.WriteString(" }")
	return out.String()
}

func (im *InterfaceMethod) String() string {
	var out strings.Builder
	out.WriteString(im.Name.String())
	out.WriteString("(")
	for i, p := range im.Parameters {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(p.String())
		if i < len(im.ParamTypes) && im.ParamTypes[i] != "" {
			out.WriteString(": ")
			out.WriteString(im.ParamTypes[i])
		}
	}
	out.WriteString(")")
	if im.ReturnType != "" {
		out.WriteString(": ")
		out.WriteString(im.ReturnType)
	}
	out.WriteString(";")
	return out.String()
}

type InitStatement struct {
	Body *BlockStatement
}

func (is *InitStatement) statementNode() {}
func (is *InitStatement) String() string {
	if is.Body == nil {
		return "init {}"
	}
	return "init " + is.Body.String()
}

type TestStatement struct {
	Name string
	Body *BlockStatement
}

func (ts *TestStatement) statementNode() {}
func (ts *TestStatement) String() string {
	if ts.Body == nil {
		return fmt.Sprintf("test %q {}", ts.Name)
	}
	return fmt.Sprintf("test %q %s", ts.Name, ts.Body.String())
}

type TypeDeclarationStatement struct {
	Name     *Identifier
	Variants []*ADTVariantDecl
}

type ADTVariantDecl struct {
	Name   *Identifier
	Fields []*Identifier
}

func (td *TypeDeclarationStatement) statementNode() {}
func (td *TypeDeclarationStatement) String() string {
	var out strings.Builder
	out.WriteString("type ")
	out.WriteString(td.Name.String())
	out.WriteString(" = ")
	for i, v := range td.Variants {
		if i > 0 {
			out.WriteString(" | ")
		}
		out.WriteString(v.Name.String())
		out.WriteString("(")
		for j, f := range v.Fields {
			if j > 0 {
				out.WriteString(", ")
			}
			out.WriteString(f.String())
		}
		out.WriteString(")")
	}
	out.WriteString(";")
	return out.String()
}

type DestructurePattern struct {
	Kind     string
	Keys     []Expression
	Names    []*Identifier
	Defaults []Expression
	RestName *Identifier
}

type DestructureLetStatement struct {
	Pattern *DestructurePattern
	Value   Expression
	IsConst bool
}

func (d *DestructureLetStatement) statementNode() {}
func (d *DestructureLetStatement) String() string {
	return fmt.Sprintf("let <destructure> = %s;", d.Value.String())
}

type ReturnStatement struct {
	ReturnValue Expression
}

func (rs *ReturnStatement) statementNode() {}
func (rs *ReturnStatement) String() string {
	if rs.ReturnValue != nil {
		return fmt.Sprintf("return %s;", rs.ReturnValue.String())
	}
	return "return;"
}

type BreakStatement struct{}

func (bs *BreakStatement) statementNode() {}
func (bs *BreakStatement) String() string { return "break;" }

type ContinueStatement struct{}

func (cs *ContinueStatement) statementNode() {}
func (cs *ContinueStatement) String() string { return "continue;" }

type ExpressionStatement struct {
	Expression Expression
}

func (es *ExpressionStatement) statementNode() {}
func (es *ExpressionStatement) String() string {
	if es.Expression != nil {
		return es.Expression.String()
	}
	return ""
}

type BlockStatement struct {
	Statements []Statement
}

func (bs *BlockStatement) statementNode() {}
func (bs *BlockStatement) String() string {
	var out strings.Builder
	out.WriteString("{ ")
	for _, s := range bs.Statements {
		out.WriteString(s.String())
		out.WriteString(" ")
	}
	out.WriteString("}")
	return out.String()
}

type WhileStatement struct {
	Condition Expression
	Body      *BlockStatement
}

func (ws *WhileStatement) statementNode() {}
func (ws *WhileStatement) String() string {
	return fmt.Sprintf("while (%s) %s", ws.Condition.String(), ws.Body.String())
}

type DoWhileStatement struct {
	Body      *BlockStatement
	Condition Expression
}

func (dw *DoWhileStatement) statementNode() {}
func (dw *DoWhileStatement) String() string {
	return "do " + dw.Body.String() + " while (" + dw.Condition.String() + ")"
}

type ForStatement struct {
	Init      Statement
	Condition Expression
	Post      Statement
	Body      *BlockStatement
}

func (fs *ForStatement) statementNode() {}
func (fs *ForStatement) String() string {
	var out strings.Builder
	out.WriteString("for (")
	if fs.Init != nil {
		out.WriteString(fs.Init.String())
	} else {
		out.WriteString(";")
	}
	if fs.Condition != nil {
		out.WriteString(" " + fs.Condition.String() + ";")
	}
	if fs.Post != nil {
		out.WriteString(" " + fs.Post.String())
	}
	out.WriteString(") ")
	out.WriteString(fs.Body.String())
	return out.String()
}

type ForInStatement struct {
	KeyName   *Identifier
	ValueName *Identifier
	Iterable  Expression
	Body      *BlockStatement
}

func (fi *ForInStatement) statementNode() {}
func (fi *ForInStatement) String() string {
	var out strings.Builder
	out.WriteString("for (")
	if fi.KeyName != nil {
		out.WriteString(fi.KeyName.String())
		out.WriteString(", ")
	}
	out.WriteString(fi.ValueName.String())
	out.WriteString(" in ")
	out.WriteString(fi.Iterable.String())
	out.WriteString(") ")
	out.WriteString(fi.Body.String())
	return out.String()
}

type PrintStatement struct {
	Expression Expression
}

func (ps *PrintStatement) statementNode() {}
func (ps *PrintStatement) String() string {
	return fmt.Sprintf("print %s;", ps.Expression.String())
}

type ImportStatement struct {
	Path  Expression
	Alias *Identifier
	Names []*Identifier
}

func (is *ImportStatement) statementNode() {}
func (is *ImportStatement) String() string {
	if is.Path == nil {
		return "import;"
	}
	if len(is.Names) > 0 {
		parts := make([]string, 0, len(is.Names))
		for _, n := range is.Names {
			parts = append(parts, n.String())
		}
		return fmt.Sprintf("import {%s} from %s;", strings.Join(parts, ", "), is.Path.String())
	}
	if is.Alias != nil {
		return fmt.Sprintf("import %s as %s;", is.Path.String(), is.Alias.String())
	}
	return fmt.Sprintf("import %s;", is.Path.String())
}

type ExportStatement struct {
	Declaration Statement
	IsConst     bool
}

func (es *ExportStatement) statementNode() {}
func (es *ExportStatement) String() string {
	kind := "let"
	if es.IsConst {
		kind = "const"
	}
	if es.Declaration == nil {
		return fmt.Sprintf("export %s;", kind)
	}
	if ls, ok := es.Declaration.(*LetStatement); ok && len(ls.Names) > 0 {
		return fmt.Sprintf("export %s %s = %s;", kind, ls.Names[0].String(), ls.Value.String())
	}
	return fmt.Sprintf("export %s %s", kind, es.Declaration.String())
}

type ThrowStatement struct {
	Value Expression
}

func (ts *ThrowStatement) statementNode() {}
func (ts *ThrowStatement) String() string {
	if ts.Value == nil {
		return "throw;"
	}
	return fmt.Sprintf("throw %s;", ts.Value.String())
}

type TryCatchExpression struct {
	TryBlock     *BlockStatement
	CatchIdent   *Identifier
	CatchType    string
	CatchBlock   *BlockStatement
	FinallyBlock *BlockStatement
}

func (tce *TryCatchExpression) expressionNode() {}
func (tce *TryCatchExpression) String() string {
	var out strings.Builder
	out.WriteString("try ")
	out.WriteString(tce.TryBlock.String())
	if tce.CatchIdent != nil {
		if tce.CatchType != "" {
			fmt.Fprintf(&out, " catch (%s: %s) %s", tce.CatchIdent.String(), tce.CatchType, tce.CatchBlock.String())
		} else {
			fmt.Fprintf(&out, " catch (%s) %s", tce.CatchIdent.String(), tce.CatchBlock.String())
		}
	} else if tce.CatchBlock != nil {
		out.WriteString(" catch ")
		out.WriteString(tce.CatchBlock.String())
	}
	if tce.FinallyBlock != nil {
		out.WriteString(" finally ")
		out.WriteString(tce.FinallyBlock.String())
	}
	return out.String()
}

type TernaryExpression struct {
	Condition   Expression
	Consequence Expression
	Alternative Expression
}

func (te *TernaryExpression) expressionNode() {}
func (te *TernaryExpression) String() string {
	return "(" + te.Condition.String() + " ? " + te.Consequence.String() + " : " + te.Alternative.String() + ")"
}

type SwitchStatement struct {
	Value   Expression
	Cases   []*SwitchCase
	Default *BlockStatement
}

type SwitchCase struct {
	Values []Expression
	Body   *BlockStatement
}

func (ss *SwitchStatement) statementNode() {}
func (ss *SwitchStatement) String() string {
	var out strings.Builder
	out.WriteString("switch (")
	out.WriteString(ss.Value.String())
	out.WriteString(") { ")
	for _, c := range ss.Cases {
		out.WriteString("case ")
		for i, v := range c.Values {
			if i > 0 {
				out.WriteString(", ")
			}
			out.WriteString(v.String())
		}
		out.WriteString(": ")
		out.WriteString(c.Body.String())
	}
	if ss.Default != nil {
		out.WriteString("default: ")
		out.WriteString(ss.Default.String())
	}
	out.WriteString("}")
	return out.String()
}

// --- Pattern matching AST ---

// Pattern is the interface for all pattern types in match expressions.
type Pattern interface {
	patternNode()
	String() string
}

type LiteralPattern struct{ Value Expression }

func (p *LiteralPattern) patternNode()   {}
func (p *LiteralPattern) String() string { return p.Value.String() }

type WildcardPattern struct{}

func (p *WildcardPattern) patternNode()   {}
func (p *WildcardPattern) String() string { return "_" }

type BindingPattern struct {
	Name     *Identifier
	TypeName string
}

func (p *BindingPattern) patternNode() {}
func (p *BindingPattern) String() string {
	if p.TypeName != "" {
		return p.Name.String() + ": " + p.TypeName
	}
	return p.Name.String()
}

type ArrayPattern struct {
	Elements []Pattern
	Rest     *Identifier
}

func (p *ArrayPattern) patternNode() {}
func (p *ArrayPattern) String() string {
	var out strings.Builder
	out.WriteString("[")
	for i, el := range p.Elements {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(el.String())
	}
	if p.Rest != nil {
		if len(p.Elements) > 0 {
			out.WriteString(", ")
		}
		out.WriteString("..." + p.Rest.String())
	}
	out.WriteString("]")
	return out.String()
}

type ObjectPattern struct {
	Keys     []string
	Patterns []Pattern
	Rest     *Identifier
}

func (p *ObjectPattern) patternNode() {}
func (p *ObjectPattern) String() string {
	var out strings.Builder
	out.WriteString("{")
	for i, key := range p.Keys {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(key + ": " + p.Patterns[i].String())
	}
	if p.Rest != nil {
		if len(p.Keys) > 0 {
			out.WriteString(", ")
		}
		out.WriteString("..." + p.Rest.String())
	}
	out.WriteString("}")
	return out.String()
}

type OrPattern struct{ Patterns []Pattern }

func (p *OrPattern) patternNode() {}
func (p *OrPattern) String() string {
	parts := make([]string, len(p.Patterns))
	for i, pat := range p.Patterns {
		parts[i] = pat.String()
	}
	return strings.Join(parts, " | ")
}

type ExtractorPattern struct {
	Name string
	Args []Pattern
}

func (p *ExtractorPattern) patternNode() {}
func (p *ExtractorPattern) String() string {
	if p.Args == nil {
		return p.Name
	}
	parts := make([]string, len(p.Args))
	for i, a := range p.Args {
		parts[i] = a.String()
	}
	return p.Name + "(" + strings.Join(parts, ", ") + ")"
}

type ConstructorPattern struct {
	Name string
	Args []Pattern
}

func (p *ConstructorPattern) patternNode() {}
func (p *ConstructorPattern) String() string {
	parts := make([]string, len(p.Args))
	for i, a := range p.Args {
		parts[i] = a.String()
	}
	return p.Name + "(" + strings.Join(parts, ", ") + ")"
}

type RangePattern struct {
	Low  Expression
	High Expression
}

func (p *RangePattern) patternNode()   {}
func (p *RangePattern) String() string { return p.Low.String() + ".." + p.High.String() }

type ComparisonPattern struct {
	Operator token.TokenType
	Value    Expression
}

func (p *ComparisonPattern) patternNode() {}
func (p *ComparisonPattern) String() string {
	return token.TypeName(p.Operator) + " " + p.Value.String()
}

type RangeExpression struct {
	Left  Expression
	Right Expression
}

func (re *RangeExpression) expressionNode() {}
func (re *RangeExpression) String() string  { return re.Left.String() + ".." + re.Right.String() }

type MatchExpression struct {
	Value Expression
	Cases []*MatchCase
}

type MatchCase struct {
	Pattern Pattern
	Guard   Expression
	Body    *BlockStatement
	Line    int
}

func (me *MatchExpression) expressionNode() {}
func (me *MatchExpression) statementNode()  {}
func (me *MatchExpression) String() string {
	var out strings.Builder
	out.WriteString("match (")
	out.WriteString(me.Value.String())
	out.WriteString(") { ")
	for _, c := range me.Cases {
		out.WriteString("case ")
		out.WriteString(c.Pattern.String())
		if c.Guard != nil {
			out.WriteString(" if ")
			out.WriteString(c.Guard.String())
		}
		out.WriteString(" => ")
		out.WriteString(c.Body.String())
		out.WriteString(" ")
	}
	out.WriteString("}")
	return out.String()
}

type AwaitExpression struct {
	Value Expression
}

func (ae *AwaitExpression) expressionNode() {}
func (ae *AwaitExpression) String() string  { return "await " + ae.Value.String() }

type TemplateLiteral struct {
	Parts []Expression
}

func (tl *TemplateLiteral) expressionNode() {}
func (tl *TemplateLiteral) String() string {
	var out strings.Builder
	out.WriteString("`")
	for _, p := range tl.Parts {
		if sl, ok := p.(*StringLiteral); ok {
			out.WriteString(sl.Value)
		} else {
			out.WriteString("${")
			out.WriteString(p.String())
			out.WriteString("}")
		}
	}
	out.WriteString("`")
	return out.String()
}

type LazyExpression struct {
	Value Expression
}

func (le *LazyExpression) expressionNode() {}
func (le *LazyExpression) String() string {
	return "lazy " + le.Value.String()
}
