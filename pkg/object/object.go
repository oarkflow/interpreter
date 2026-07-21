package object

import (
	"context"
	"fmt"
	"image"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oarkflow/interpreter/pkg/ast"
)

// ---------------------------------------------------------------------------
// Function pointers that must be set by the evaluator package at init time.
// They allow subsystem packages (builtins, reactive, etc.) to call back into
// the evaluator without creating import cycles.
// ---------------------------------------------------------------------------

// EvalFn evaluates an AST node in the given environment.
var EvalFn func(node ast.Node, env *Environment) Object

// ApplyFunctionFn calls a function object (Function or Builtin) with args.
var ApplyFunctionFn func(fn Object, args []Object, env *Environment) Object

// ExtendFunctionEnvFn creates a new enclosed environment for a function call.
var ExtendFunctionEnvFn func(fn *Function, args []Object, callerEnv *Environment) *Environment

// UnwrapReturnValueFn unwraps a ReturnValue object to its inner value.
var UnwrapReturnValueFn func(obj Object) Object

// ---------------------------------------------------------------------------
// Graceful shutdown hooks
// ---------------------------------------------------------------------------
//
// Long-lived constructs started by a script - HTTP servers via listen()/
// listen_async() today, potentially schedulers/workers later - register a
// hook here so a host (the CLI's SIGINT/SIGTERM handler, an embedding host's
// own shutdown path, ...) can ask them to stop cleanly (e.g. drain in-flight
// HTTP requests) instead of the process just being killed mid-request.

var (
	shutdownHooksMu sync.Mutex
	shutdownHooks   []func(ctx context.Context)
)

// RegisterShutdownHook registers a function to be called when the process is
// asked to shut down gracefully. The hook receives a context bounded by the
// overall grace period passed to RunShutdownHooks and should respect it
// rather than blocking indefinitely.
func RegisterShutdownHook(hook func(ctx context.Context)) {
	if hook == nil {
		return
	}
	shutdownHooksMu.Lock()
	shutdownHooks = append(shutdownHooks, hook)
	shutdownHooksMu.Unlock()
}

// RunShutdownHooks invokes every registered shutdown hook concurrently and
// waits for them to finish or for the grace period to elapse, whichever
// comes first.
func RunShutdownHooks(grace time.Duration) {
	shutdownHooksMu.Lock()
	hooks := append([]func(ctx context.Context){}, shutdownHooks...)
	shutdownHooksMu.Unlock()
	if len(hooks) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(len(hooks))
	for _, hook := range hooks {
		go func(h func(ctx context.Context)) {
			defer wg.Done()
			h(ctx)
		}(hook)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
}

// ---------------------------------------------------------------------------
// Object interface
// ---------------------------------------------------------------------------

type Object interface {
	Type() ObjectType
	Inspect() string
}

// ---------------------------------------------------------------------------
// ObjectType
// ---------------------------------------------------------------------------

type ObjectType int

const (
	INTEGER_OBJ ObjectType = iota
	FLOAT_OBJ
	BOOLEAN_OBJ
	STRING_OBJ
	NULL_OBJ
	ERROR_OBJ
	RETURN_VALUE_OBJ
	BREAK_OBJ
	CONTINUE_OBJ
	FUNCTION_OBJ
	BUILTIN_OBJ
	ARRAY_OBJ
	HASH_OBJ
	DB_OBJ
	DB_TX_OBJ
	FUTURE_OBJ
	INTERFACE_OBJ
	ADT_TYPE_OBJ
	ADT_VALUE_OBJ
	LAZY_OBJ
	OWNED_OBJ
	SECRET_OBJ
	RENDER_ARTIFACT_OBJ
	FILE_VALUE_OBJ
	IMAGE_VALUE_OBJ
	TABLE_VALUE_OBJ
	MACRO_OBJ
)

// Extended ObjectType constants defined by other subsystems.
const (
	SERVER_OBJ        ObjectType = 100
	REQUEST_OBJ       ObjectType = 101
	RESPONSE_OBJ      ObjectType = 102
	SSE_WRITER_OBJ    ObjectType = 103
	QUERY_BUILDER_OBJ ObjectType = 104
	LAZY_DB_QUERY_OBJ ObjectType = 105
	SIGNAL_OBJ        ObjectType = 106
	COMPUTED_OBJ      ObjectType = 107
	EFFECT_OBJ        ObjectType = 108
	FILE_FINDER_OBJ   ObjectType = 109
	ROUTE_GROUP_OBJ   ObjectType = 110

	RULES_SERVICE_OBJ   ObjectType = 111
	TCPGUARD_BUNDLE_OBJ ObjectType = 112
	TCPGUARD_GUARD_OBJ  ObjectType = 113
)

func (ot ObjectType) String() string {
	switch ot {
	case INTEGER_OBJ:
		return "INTEGER"
	case FLOAT_OBJ:
		return "FLOAT"
	case BOOLEAN_OBJ:
		return "BOOLEAN"
	case STRING_OBJ:
		return "STRING"
	case NULL_OBJ:
		return "NULL"
	case ERROR_OBJ:
		return "ERROR"
	case RETURN_VALUE_OBJ:
		return "RETURN_VALUE"
	case BREAK_OBJ:
		return "BREAK"
	case CONTINUE_OBJ:
		return "CONTINUE"
	case FUNCTION_OBJ:
		return "FUNCTION"
	case BUILTIN_OBJ:
		return "BUILTIN"
	case ARRAY_OBJ:
		return "ARRAY"
	case HASH_OBJ:
		return "HASH"
	case DB_OBJ:
		return "DB"
	case DB_TX_OBJ:
		return "DB_TX"
	case FUTURE_OBJ:
		return "FUTURE"
	case INTERFACE_OBJ:
		return "INTERFACE"
	case MACRO_OBJ:
		return "MACRO"
	case ADT_TYPE_OBJ:
		return "ADT_TYPE"
	case ADT_VALUE_OBJ:
		return "ADT_VALUE"
	case LAZY_OBJ:
		return "LAZY"
	case OWNED_OBJ:
		return "OWNED"
	case SECRET_OBJ:
		return "SECRET"
	case RENDER_ARTIFACT_OBJ:
		return "RENDER_ARTIFACT"
	case FILE_VALUE_OBJ:
		return "FILE_VALUE"
	case IMAGE_VALUE_OBJ:
		return "IMAGE_VALUE"
	case TABLE_VALUE_OBJ:
		return "TABLE_VALUE"
	case SERVER_OBJ:
		return "SERVER"
	case REQUEST_OBJ:
		return "REQUEST"
	case RESPONSE_OBJ:
		return "RESPONSE"
	case SSE_WRITER_OBJ:
		return "SSE_WRITER"
	case QUERY_BUILDER_OBJ:
		return "QUERY_BUILDER"
	case LAZY_DB_QUERY_OBJ:
		return "LAZY_DB_QUERY"
	case SIGNAL_OBJ:
		return "SIGNAL"
	case COMPUTED_OBJ:
		return "COMPUTED"
	case EFFECT_OBJ:
		return "EFFECT"
	case FILE_FINDER_OBJ:
		return "FILE_FINDER"
	case ROUTE_GROUP_OBJ:
		return "ROUTE_GROUP"
	case RULES_SERVICE_OBJ:
		return "RULES_SERVICE"
	case TCPGUARD_BUNDLE_OBJ:
		return "TCPGUARD_BUNDLE"
	case TCPGUARD_GUARD_OBJ:
		return "TCPGUARD_GUARD"
	default:
		return "UNKNOWN"
	}
}

// ---------------------------------------------------------------------------
// Scalar types
// ---------------------------------------------------------------------------

type Integer struct {
	Value int64
}

func (i *Integer) Type() ObjectType { return INTEGER_OBJ }
func (i *Integer) Inspect() string  { return fmt.Sprintf("%d", i.Value) }

type Float struct {
	Value float64
}

func (f *Float) Type() ObjectType { return FLOAT_OBJ }
func (f *Float) Inspect() string  { return fmt.Sprintf("%g", f.Value) }

type Boolean struct {
	Value bool
}

func (b *Boolean) Type() ObjectType { return BOOLEAN_OBJ }
func (b *Boolean) Inspect() string  { return fmt.Sprintf("%t", b.Value) }

type String struct {
	Value string
}

func (s *String) Type() ObjectType { return STRING_OBJ }
func (s *String) Inspect() string  { return s.Value }

type Secret struct {
	Value string
}

func (s *Secret) Type() ObjectType { return SECRET_OBJ }
func (s *Secret) Inspect() string  { return "***" }

type RenderArtifact struct {
	Kind      string
	Source    string
	SourceTyp string
	MIME      string
	Name      string
	Alt       string
	Width     int64
	Height    int64
	MaxBytes  int64
	Mode      string
}

func (r *RenderArtifact) Type() ObjectType { return RENDER_ARTIFACT_OBJ }
func (r *RenderArtifact) Inspect() string {
	if r == nil {
		return "<render-artifact>"
	}
	kind := strings.TrimSpace(r.Kind)
	if kind == "" {
		kind = "artifact"
	}
	name := strings.TrimSpace(r.Name)
	if name == "" {
		name = strings.TrimSpace(r.Source)
	}
	if name == "" {
		return fmt.Sprintf("<%s>", kind)
	}
	mime := strings.TrimSpace(r.MIME)
	if mime == "" {
		return fmt.Sprintf("<%s %s>", kind, name)
	}
	return fmt.Sprintf("<%s %s %s>", kind, name, mime)
}

type FileValue struct {
	Name       string
	Path       string
	MIME       string
	Encoding   string
	SourceType string
	Size       int64
	Data       []byte
}

func (f *FileValue) Type() ObjectType { return FILE_VALUE_OBJ }
func (f *FileValue) Inspect() string {
	if f == nil {
		return "<file>"
	}
	name := strings.TrimSpace(f.Name)
	if name == "" {
		name = strings.TrimSpace(f.Path)
	}
	if name == "" {
		name = "file"
	}
	mime := strings.TrimSpace(f.MIME)
	if mime == "" {
		mime = "unknown"
	}
	size := f.Size
	if size <= 0 && len(f.Data) > 0 {
		size = int64(len(f.Data))
	}
	return fmt.Sprintf("<file %s %s %d bytes>", name, mime, size)
}

type ImageValue struct {
	Name       string
	Path       string
	MIME       string
	Format     string
	SourceType string
	Width      int64
	Height     int64
	Data       []byte
	Image      image.Image
}

func (i *ImageValue) Type() ObjectType { return IMAGE_VALUE_OBJ }
func (i *ImageValue) Inspect() string {
	if i == nil {
		return "<image>"
	}
	name := strings.TrimSpace(i.Name)
	if name == "" {
		name = strings.TrimSpace(i.Path)
	}
	if name == "" {
		name = "image"
	}
	format := strings.TrimSpace(i.Format)
	if format == "" {
		format = "unknown"
	}
	return fmt.Sprintf("<image %s %s %dx%d>", name, format, i.Width, i.Height)
}

type TableValue struct {
	Name       string
	Path       string
	MIME       string
	SourceType string
	Columns    []string
	Rows       []map[string]Object
}

func (t *TableValue) Type() ObjectType { return TABLE_VALUE_OBJ }
func (t *TableValue) Inspect() string {
	if t == nil {
		return "<table>"
	}
	return fmt.Sprintf("<table columns=%d rows=%d>", len(t.Columns), len(t.Rows))
}

type Null struct{}

func (n *Null) Type() ObjectType { return NULL_OBJ }
func (n *Null) Inspect() string  { return "null" }

// ---------------------------------------------------------------------------
// Error
// ---------------------------------------------------------------------------

type Error struct {
	Message string
	Code    string
	Path    string
	Line    int
	Column  int
	Stack   []CallFrame
	// ModuleChain records the import path of each module a runtime error
	// propagated through, outermost first (e.g. the entry script's own
	// import, then the module it imported, ...), ending with the module
	// where the error actually occurred. Populated alongside Message
	// (which still carries the same information as one concatenated
	// string, for callers - e.g. SPL try/catch - that only look at
	// Message/Inspect()) so CLI-style callers can render each hop on its
	// own line instead of parsing that string back apart.
	ModuleChain []string
	// LeafMessage is the original, unwrapped error message from the
	// innermost module (before any "module runtime error in ..." prefixes
	// were added), captured once at the point the error first occurred.
	LeafMessage string
}

func (e *Error) Type() ObjectType { return ERROR_OBJ }
func (e *Error) Inspect() string {
	if e == nil {
		return "ERROR"
	}
	var out strings.Builder
	out.WriteString("ERROR: ")
	if strings.TrimSpace(e.Code) != "" {
		out.WriteString("[")
		out.WriteString(e.Code)
		out.WriteString("] ")
	}
	out.WriteString(e.Message)
	if trace := FormatCallStack(e.Stack); trace != "" {
		out.WriteByte('\n')
		out.WriteString(trace)
	}
	return out.String()
}

func (e *Error) WithFrame(frame CallFrame) *Error {
	if e == nil {
		return nil
	}
	if frame.Function == "" && frame.Path == "" && frame.Line == 0 && frame.Column == 0 {
		return e
	}
	cloned := *e
	cloned.Stack = append(append([]CallFrame(nil), e.Stack...), frame)
	if cloned.Path == "" {
		cloned.Path = frame.Path
	}
	if cloned.Line == 0 {
		cloned.Line = frame.Line
	}
	if cloned.Column == 0 {
		cloned.Column = frame.Column
	}
	return &cloned
}

// ---------------------------------------------------------------------------
// CallFrame & stack helpers
// ---------------------------------------------------------------------------

type CallFrame struct {
	Function string
	Path     string
	Line     int
	Column   int
}

func SameCallFrame(a, b CallFrame) bool {
	return a.Function == b.Function && a.Path == b.Path && a.Line == b.Line && a.Column == b.Column
}

func FormatCallFrame(frame CallFrame) string {
	location := frame.Path
	if location == "" {
		location = "<memory>"
	}
	if frame.Line > 0 {
		location = fmt.Sprintf("%s:%d", location, frame.Line)
		if frame.Column > 0 {
			location = fmt.Sprintf("%s:%d", location, frame.Column)
		}
	}
	if frame.Function == "" {
		return location
	}
	return fmt.Sprintf("%s (%s)", frame.Function, location)
}

func FormatCallStack(stack []CallFrame) string {
	if len(stack) == 0 {
		return ""
	}
	lines := make([]string, 0, len(stack)+1)
	lines = append(lines, "Stack trace:")
	for i := 0; i < len(stack); i++ {
		lines = append(lines, "  at "+FormatCallFrame(stack[i]))
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// Control-flow wrapper types
// ---------------------------------------------------------------------------

type ReturnValue struct {
	Value Object
}

func (rv *ReturnValue) Type() ObjectType { return RETURN_VALUE_OBJ }
func (rv *ReturnValue) Inspect() string  { return rv.Value.Inspect() }

type Break struct{}

func (b *Break) Type() ObjectType { return BREAK_OBJ }
func (b *Break) Inspect() string  { return "break" }

type Continue struct{}

func (c *Continue) Type() ObjectType { return CONTINUE_OBJ }
func (c *Continue) Inspect() string  { return "continue" }

// ---------------------------------------------------------------------------
// Function
// ---------------------------------------------------------------------------

type Function struct {
	Name        string
	Parameters  []*ast.Identifier
	ParamTypes  []string
	Defaults    []ast.Expression
	ReturnType  string
	HasRest     bool
	Body        *ast.BlockStatement
	Env         *Environment
	IsAsync     bool
	IsGenerator bool
}

type Macro struct {
	Name       string
	Parameters []*ast.Identifier
	Body       *ast.BlockStatement
}

func (m *Macro) Type() ObjectType { return MACRO_OBJ }
func (m *Macro) Inspect() string  { return "macro " + m.Name }

func (f *Function) Type() ObjectType { return FUNCTION_OBJ }
func (f *Function) Inspect() string {
	var out strings.Builder
	if f.Name != "" {
		out.WriteString("function ")
		out.WriteString(f.Name)
		out.WriteString("(")
	} else {
		out.WriteString("function(")
	}
	for i, p := range f.Parameters {
		if i > 0 {
			out.WriteString(", ")
		}
		if f.HasRest && i == len(f.Parameters)-1 {
			out.WriteString("...")
		}
		out.WriteString(p.String())
		if i < len(f.ParamTypes) && f.ParamTypes[i] != "" {
			out.WriteString(": ")
			out.WriteString(f.ParamTypes[i])
		}
		if i < len(f.Defaults) && f.Defaults[i] != nil {
			out.WriteString(" = ")
			out.WriteString(f.Defaults[i].String())
		}
	}
	out.WriteString(") {\n")
	if f.ReturnType != "" {
		out.WriteString("// -> ")
		out.WriteString(f.ReturnType)
		out.WriteString("\n")
	}
	out.WriteString(f.Body.String())
	out.WriteString("\n}")
	return out.String()
}

// ClassFieldDef is a declared instance or static field on a class body
// (e.g. `private count = 0;`). ValueExpr/Env are used to (re-)evaluate the
// default per-instance so mutable defaults (arrays, hashes) aren't shared.
type ClassFieldDef struct {
	Name      string
	ValueExpr ast.Expression
	Env       *Environment
	IsPrivate bool
}

type ClassObject struct {
	Name              string
	Parent            *ClassObject
	Methods           map[string]*Function
	StaticMethods     map[string]*Function
	Fields            map[string]Object // evaluated static field values
	InstanceFieldDefs []*ClassFieldDef  // instance fields declared directly on this class
	PrivateMembers    map[string]bool   // method/static-field names declared private on this class
	IsAbstract        bool
	Builtin           *Builtin
	Implements        []string
}

func (c *ClassObject) GetStaticMethod(name string) (*Function, bool) {
	fn, _, ok := c.GetStaticMethodOwner(name)
	return fn, ok
}

func (c *ClassObject) GetStaticMethodOwner(name string) (*Function, *ClassObject, bool) {
	if c == nil {
		return nil, nil, false
	}
	if m, ok := c.StaticMethods[name]; ok {
		return m, c, true
	}
	if c.Parent != nil {
		return c.Parent.GetStaticMethodOwner(name)
	}
	return nil, nil, false
}

func (c *ClassObject) GetStaticFieldOwner(name string) (Object, *ClassObject, bool) {
	if c == nil {
		return nil, nil, false
	}
	if v, ok := c.Fields[name]; ok {
		return v, c, true
	}
	if c.Parent != nil {
		return c.Parent.GetStaticFieldOwner(name)
	}
	return nil, nil, false
}

func (c *ClassObject) Type() ObjectType { return FUNCTION_OBJ }
func (c *ClassObject) Inspect() string {
	var out strings.Builder
	if c.IsAbstract {
		out.WriteString("abstract ")
	}
	out.WriteString("class ")
	out.WriteString(c.Name)
	if c.Parent != nil {
		out.WriteString(" extends ")
		out.WriteString(c.Parent.Name)
	}
	out.WriteString(" { methods=")
	out.WriteString(fmt.Sprintf("%d", len(c.Methods)))
	out.WriteString(" }")
	return out.String()
}

func (c *ClassObject) GetMethod(name string) (*Function, bool) {
	fn, _, ok := c.GetMethodOwner(name)
	return fn, ok
}

func (c *ClassObject) GetMethodOwner(name string) (*Function, *ClassObject, bool) {
	if c == nil {
		return nil, nil, false
	}
	if m, ok := c.Methods[name]; ok {
		return m, c, true
	}
	if c.Parent != nil {
		return c.Parent.GetMethodOwner(name)
	}
	return nil, nil, false
}

func (c *ClassObject) NewInstance() *ClassInstance {
	return &ClassInstance{
		Class:        c,
		Fields:       make(map[string]Object),
		PrivateOwner: make(map[string]string),
	}
}

type ClassInstance struct {
	Class  *ClassObject
	Fields map[string]Object
	// PrivateOwner maps a private field/method name to the name of the
	// class that declared it private, for access-control checks.
	PrivateOwner map[string]string
}

func (ci *ClassInstance) Type() ObjectType { return HASH_OBJ }
func (ci *ClassInstance) Inspect() string {
	var out strings.Builder
	out.WriteString(ci.Class.Name)
	out.WriteString("{")
	first := true
	for k, v := range ci.Fields {
		if !first {
			out.WriteString(", ")
		}
		first = false
		out.WriteString(k)
		out.WriteString(": ")
		out.WriteString(v.Inspect())
	}
	out.WriteString("}")
	return out.String()
}

func (ci *ClassInstance) Get(name string) (Object, bool) {
	if val, ok := ci.Fields[name]; ok {
		return val, true
	}
	if ci.Class != nil {
		if m, ok := ci.Class.GetMethod(name); ok {
			return m, true
		}
	}
	return nil, false
}

func (ci *ClassInstance) Set(name string, val Object) {
	ci.Fields[name] = val
}

// ---------------------------------------------------------------------------
// Builtin
// ---------------------------------------------------------------------------

type BuiltinFunction func(args ...Object) Object
type BuiltinFunction1 func(arg Object) Object
type BuiltinFunctionWithEnv func(env *Environment, args ...Object) Object

type Builtin struct {
	Fn        BuiltinFunction
	Fn1       BuiltinFunction1
	FnWithEnv BuiltinFunctionWithEnv
	Env       *Environment
}

func (b *Builtin) Type() ObjectType { return BUILTIN_OBJ }
func (b *Builtin) Inspect() string  { return "builtin function" }

func (b *Builtin) BindEnv(env *Environment) *Builtin {
	if b == nil {
		return nil
	}
	if b.FnWithEnv == nil {
		return b
	}
	cloned := *b
	cloned.Env = env
	return &cloned
}

// ---------------------------------------------------------------------------
// Array
// ---------------------------------------------------------------------------

type Array struct {
	Elements []Object
}

func (ao *Array) Type() ObjectType { return ARRAY_OBJ }
func (ao *Array) Inspect() string {
	var out strings.Builder
	out.WriteString("[")
	for i, e := range ao.Elements {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(e.Inspect())
	}
	out.WriteString("]")
	return out.String()
}

// ---------------------------------------------------------------------------
// Hash
// ---------------------------------------------------------------------------

type HashKey struct {
	Type  ObjectType
	Value uint64
}

type Hashable interface {
	HashKey() HashKey
}

func (b *Boolean) HashKey() HashKey {
	var value uint64
	if b.Value {
		value = 1
	} else {
		value = 0
	}
	return HashKey{Type: b.Type(), Value: value}
}

func (i *Integer) HashKey() HashKey {
	return HashKey{Type: i.Type(), Value: uint64(i.Value)}
}

func (s *String) HashKey() HashKey {
	h := uint64(0)
	for _, ch := range s.Value {
		h = 31*h + uint64(ch)
	}
	return HashKey{Type: s.Type(), Value: h}
}

type HashPair struct {
	Key   Object
	Value Object
}

type Hash struct {
	Pairs map[HashKey]HashPair
}

func (h *Hash) Type() ObjectType { return HASH_OBJ }
func (h *Hash) Inspect() string {
	var out strings.Builder
	pairs := make([]string, 0, len(h.Pairs))
	for _, pair := range h.Pairs {
		pairs = append(pairs, fmt.Sprintf("%s: %s", pair.Key.Inspect(), pair.Value.Inspect()))
	}
	out.WriteString("{")
	out.WriteString(strings.Join(pairs, ", "))
	out.WriteString("}")
	return out.String()
}

func FormatPlain(obj Object) string {
	return formatPlainDepth(obj, 0)
}

func formatPlainDepth(obj Object, depth int) string {
	if obj == nil {
		return "null"
	}
	switch v := obj.(type) {
	case *OwnedValue:
		return formatPlainDepth(v.Inner, depth)
	case *ImmutableValue:
		return formatPlainDepth(v.Inner, depth)
	case *GeneratorValue:
		return formatPlainDepth(&Array{Elements: v.Elements}, depth)
	case *String:
		if depth > 0 {
			return strconv.Quote(v.Value)
		}
		return v.Value
	case *Array:
		return formatArrayPlain(v, depth)
	case *Hash:
		return formatHashPlain(v, depth)
	default:
		return obj.Inspect()
	}
}

func formatArrayPlain(arr *Array, depth int) string {
	if arr == nil || len(arr.Elements) == 0 {
		return "[]"
	}
	indent := strings.Repeat("  ", depth)
	childIndent := strings.Repeat("  ", depth+1)
	parts := make([]string, 0, len(arr.Elements))
	for _, el := range arr.Elements {
		parts = append(parts, childIndent+formatPlainDepth(el, depth+1))
	}
	return "[\n" + strings.Join(parts, ",\n") + "\n" + indent + "]"
}

func formatHashPlain(h *Hash, depth int) string {
	if h == nil || len(h.Pairs) == 0 {
		return "{}"
	}
	indent := strings.Repeat("  ", depth)
	childIndent := strings.Repeat("  ", depth+1)
	keys := make([]string, 0, len(h.Pairs))
	keyToPair := make(map[string]HashPair, len(h.Pairs))
	for _, pair := range h.Pairs {
		key := pair.Key.Inspect()
		keys = append(keys, key)
		keyToPair[key] = pair
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		pair := keyToPair[key]
		parts = append(parts, childIndent+key+": "+formatPlainDepth(pair.Value, depth+1))
	}
	return "{\n" + strings.Join(parts, ",\n") + "\n" + indent + "}"
}

// ---------------------------------------------------------------------------
// Future
// ---------------------------------------------------------------------------

type Future struct {
	Ch     chan Object
	Result Object
	Done   bool
	Mu     sync.Mutex
}

func (f *Future) Type() ObjectType { return FUTURE_OBJ }
func (f *Future) Inspect() string {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if f.Done {
		return fmt.Sprintf("<future: resolved=%s>", f.Result.Inspect())
	}
	return "<future: pending>"
}

// Resolve blocks until the future's result is available and returns it.
func (f *Future) Resolve() Object {
	f.Mu.Lock()
	if f.Done {
		f.Mu.Unlock()
		return f.Result
	}
	f.Mu.Unlock()
	result := <-f.Ch
	f.Mu.Lock()
	f.Done = true
	f.Result = result
	f.Mu.Unlock()
	return result
}

// ---------------------------------------------------------------------------
// ADT types
// ---------------------------------------------------------------------------

type ADTTypeDef struct {
	TypeName string
	Variants map[string]int
	Order    []string
}

func (a *ADTTypeDef) Type() ObjectType { return ADT_TYPE_OBJ }
func (a *ADTTypeDef) Inspect() string {
	if a == nil {
		return "<adt-type>"
	}
	return fmt.Sprintf("<adt %s variants=%d>", a.TypeName, len(a.Order))
}

type ADTValue struct {
	TypeName    string
	VariantName string
	FieldNames  []string
	Values      []Object
	AllVariants []string
}

func (a *ADTValue) Type() ObjectType { return ADT_VALUE_OBJ }
func (a *ADTValue) Inspect() string {
	if a == nil {
		return "<adt-value>"
	}
	parts := make([]string, 0, len(a.Values))
	for _, v := range a.Values {
		parts = append(parts, v.Inspect())
	}
	return fmt.Sprintf("%s(%s)", a.VariantName, strings.Join(parts, ", "))
}

// ---------------------------------------------------------------------------
// InterfaceLiteral
// ---------------------------------------------------------------------------

type InterfaceLiteral struct {
	Methods map[string]*ast.InterfaceMethod
}

func (il *InterfaceLiteral) Type() ObjectType { return INTERFACE_OBJ }
func (il *InterfaceLiteral) Inspect() string {
	if il == nil {
		return "<interface>"
	}
	return fmt.Sprintf("<interface methods=%d>", len(il.Methods))
}

// ---------------------------------------------------------------------------
// LazyValue
// ---------------------------------------------------------------------------

type LazyValue struct {
	Env       *Environment
	Expr      ast.Expression
	Evaluated bool
	Result    Object
	Mu        sync.Mutex
}

func (lv *LazyValue) Type() ObjectType { return LAZY_OBJ }
func (lv *LazyValue) Inspect() string {
	res := lv.Force()
	if res == nil {
		return "null"
	}
	return res.Inspect()
}

func (lv *LazyValue) Force() Object {
	if lv == nil {
		return NULL
	}
	lv.Mu.Lock()
	defer lv.Mu.Unlock()
	if lv.Evaluated {
		return lv.Result
	}
	if lv.Expr == nil {
		lv.Result = NULL
		lv.Evaluated = true
		return lv.Result
	}
	if EvalFn == nil {
		lv.Result = NULL
		lv.Evaluated = true
		return lv.Result
	}
	lv.Result = EvalFn(lv.Expr, lv.Env)
	lv.Evaluated = true
	return lv.Result
}

// ---------------------------------------------------------------------------
// OwnedValue
// ---------------------------------------------------------------------------

type OwnedValue struct {
	OwnerID string
	Inner   Object
}

func (ov *OwnedValue) Type() ObjectType { return OWNED_OBJ }
func (ov *OwnedValue) Inspect() string {
	if ov == nil || ov.Inner == nil {
		return "null"
	}
	return ov.Inner.Inspect()
}

// ---------------------------------------------------------------------------
// Channel
// ---------------------------------------------------------------------------

type Channel struct {
	Ch chan Object
}

func (c *Channel) Type() ObjectType { return BUILTIN_OBJ }
func (c *Channel) Inspect() string  { return "<channel>" }

// ---------------------------------------------------------------------------
// ImmutableValue
// ---------------------------------------------------------------------------

type ImmutableValue struct {
	Inner Object
}

func (i *ImmutableValue) Type() ObjectType { return i.Inner.Type() }
func (i *ImmutableValue) Inspect() string  { return i.Inner.Inspect() }

// ---------------------------------------------------------------------------
// GeneratorValue
// ---------------------------------------------------------------------------

type GeneratorValue struct {
	Elements []Object
	Env      *Environment
	Body     *ast.BlockStatement
	Func     *Function
	Index    int
	Done     bool
}

func (g *GeneratorValue) Type() ObjectType { return ARRAY_OBJ }
func (g *GeneratorValue) Inspect() string {
	return (&Array{Elements: g.Elements}).Inspect()
}

func (g *GeneratorValue) Next() (Object, bool) {
	if g.Index < len(g.Elements) {
		val := g.Elements[g.Index]
		g.Index++
		return val, true
	}
	return nil, false
}

type Stream struct {
	Elements []Object
}

func (s *Stream) Type() ObjectType { return ARRAY_OBJ }
func (s *Stream) Inspect() string {
	return (&Array{Elements: s.Elements}).Inspect()
}

// ---------------------------------------------------------------------------
// RuntimeLimits
// ---------------------------------------------------------------------------

type RuntimeLimits struct {
	MaxDepth           int
	CurrentDepth       int
	MaxSteps           int64
	Steps              int64
	Deadline           time.Time
	Ctx                context.Context
	MaxHeapBytes       uint64
	HeapCheckEvery     int64
	MaxOutputBytes     int64
	OutputBytes        int64
	MaxHTTPBodyBytes   int64
	MaxExecOutputBytes int64
	MaxStringBytes     int64
	MaxArrayLength     int
	MaxHashEntries     int
	MaxImportDepth     int
	CurrentImportDepth int
	MaxImportCount     int
	ImportCount        int
}

// CloneForConcurrentExecution returns an independent set of runtime counters
// with the same configured limits, deadline, and cancellation context. Runtime
// counters such as recursion depth and evaluation steps describe one execution
// flow and must not be shared by background or scheduled goroutines.
func (r *RuntimeLimits) CloneForConcurrentExecution() *RuntimeLimits {
	if r == nil {
		return nil
	}
	return &RuntimeLimits{
		MaxDepth:           r.MaxDepth,
		MaxSteps:           r.MaxSteps,
		Deadline:           r.Deadline,
		Ctx:                r.Ctx,
		MaxHeapBytes:       r.MaxHeapBytes,
		HeapCheckEvery:     r.HeapCheckEvery,
		MaxOutputBytes:     r.MaxOutputBytes,
		MaxHTTPBodyBytes:   r.MaxHTTPBodyBytes,
		MaxExecOutputBytes: r.MaxExecOutputBytes,
		MaxStringBytes:     r.MaxStringBytes,
		MaxArrayLength:     r.MaxArrayLength,
		MaxHashEntries:     r.MaxHashEntries,
		MaxImportDepth:     r.MaxImportDepth,
		MaxImportCount:     r.MaxImportCount,
	}
}

type RenderConfig struct {
	Mode             string
	TerminalProtocol string
	MaxBytes         int64
	AllowURLs        bool
	AllowURLHosts    []string
}

func DefaultRenderConfig() *RenderConfig {
	return &RenderConfig{
		Mode:             "auto",
		TerminalProtocol: "auto",
		MaxBytes:         1 << 20,
	}
}

// ---------------------------------------------------------------------------
// TestStats
// ---------------------------------------------------------------------------

type TestStats struct {
	Mu     sync.Mutex
	Total  int64
	Passed int64
	Failed int64
}

func (s *TestStats) Reset() {
	if s == nil {
		return
	}
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.Total = 0
	s.Passed = 0
	s.Failed = 0
}

func (s *TestStats) Pass() {
	if s == nil {
		return
	}
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.Total++
	s.Passed++
}

func (s *TestStats) Fail() {
	if s == nil {
		return
	}
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.Total++
	s.Failed++
}

func (s *TestStats) Snapshot() (total, passed, failed int64) {
	if s == nil {
		return 0, 0, 0
	}
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return s.Total, s.Passed, s.Failed
}

// ---------------------------------------------------------------------------
// ModuleContext / ModuleCacheEntry
// ---------------------------------------------------------------------------

type ModuleContext struct {
	Exports map[string]Object
}

type ModuleCacheEntry struct {
	Exports map[string]Object
	ModTime int64
}

// ---------------------------------------------------------------------------
// SecurityPolicy (mirrors the struct defined in the main package)
// ---------------------------------------------------------------------------

type SecurityPolicy struct {
	StrictMode            bool
	ProtectHost           bool
	AllowEnvWrite         bool
	AllowedCapabilities   []string
	DeniedCapabilities    []string
	AllowedExecCommands   []string
	DeniedExecCommands    []string
	AllowedNetworkHosts   []string
	DeniedNetworkHosts    []string
	AllowedDBDrivers      []string
	DeniedDBDrivers       []string
	AllowedFileReadPaths  []string
	DeniedFileReadPaths   []string
	AllowedFileWritePaths []string
	DeniedFileWritePaths  []string
	AllowedDBDSNPatterns  []string
	DeniedDBDSNPatterns   []string
	AllowedImportPaths    []string
	DeniedImportPaths     []string
	AllowedImportPackages []string
	DeniedImportPackages  []string
	AllowedNativeModules  []string
	DeniedNativeModules   []string
	DenyDynamicImports    bool
	// BlockHardcodedSecrets, when true, rejects scripts whose source text
	// looks like it embeds a hardcoded secret (API key, private key, token,
	// ...) before parsing/eval. Detection itself is supplied by an optional
	// plugin (see security.RegisterSecretScanner, e.g. builtins/secretr) -
	// with no scanner registered this is a no-op even when set.
	BlockHardcodedSecrets bool
}

// ---------------------------------------------------------------------------
// Environment
// ---------------------------------------------------------------------------

type Environment struct {
	Mu                     sync.RWMutex
	bindingsSealed         atomic.Bool
	Store                  map[string]Object
	Constants              map[string]struct{}
	Outer                  *Environment
	ModuleContext          *ModuleContext
	ModuleDir              string
	SourcePath             string
	ModuleCache            map[string]ModuleCacheEntry
	ModuleLoading          map[string]bool
	RuntimeLimits          *RuntimeLimits
	SecurityPolicy         *SecurityPolicy
	Output                 io.Writer
	RenderConfig           *RenderConfig
	RenderArtifacts        []*RenderArtifact
	RenderArtifactSink     *[]*RenderArtifact
	CollectRenderArtifacts bool
	CallStack              []CallFrame
	OwnerID                string
	Cleanup                []func()
	YieldSink              *[]Object
}

var environmentOwnerIDCounter atomic.Uint64

func NewEnvironment() *Environment {
	return &Environment{
		Store:     make(map[string]Object),
		Constants: make(map[string]struct{}),
	}
}

var environmentPool = sync.Pool{New: func() any {
	return NewEnvironment()
}}

// NewPooledEnvironment returns a reusable environment for allocation-sensitive
// embedding paths. Call ReleasePooledEnvironment when the evaluation is done.
func NewPooledEnvironment() *Environment {
	env := environmentPool.Get().(*Environment)
	env.Reset()
	return env
}

func ReleasePooledEnvironment(env *Environment) {
	if env == nil {
		return
	}
	env.RunCleanup()
	env.Reset()
	environmentPool.Put(env)
}

func (e *Environment) Reset() {
	if e == nil {
		return
	}
	e.bindingsSealed.Store(false)
	e.Mu.Lock()
	for k := range e.Store {
		delete(e.Store, k)
	}
	for k := range e.Constants {
		delete(e.Constants, k)
	}
	e.Outer = nil
	e.ModuleContext = nil
	e.ModuleDir = ""
	e.SourcePath = ""
	e.ModuleCache = nil
	e.ModuleLoading = nil
	e.RuntimeLimits = nil
	e.SecurityPolicy = nil
	e.Output = nil
	e.RenderConfig = nil
	e.RenderArtifacts = nil
	e.RenderArtifactSink = nil
	e.CollectRenderArtifacts = false
	e.CallStack = nil
	e.OwnerID = ""
	e.Cleanup = nil
	e.Mu.Unlock()
}

func NewGlobalEnvironment(args []string) *Environment {
	env := NewEnvironment()
	argsArray := &Array{Elements: []Object{}}
	for _, arg := range args {
		argsArray.Elements = append(argsArray.Elements, &String{Value: arg})
	}
	env.Set("ARGS", argsArray)
	env.RuntimeLimits = LoadRuntimeLimitsFromEnv()
	env.RenderConfig = LoadRenderConfigFromEnv()
	env.SourcePath = "<memory>"
	return env
}

func (e *Environment) Snapshot() map[string]Object {
	if e == nil {
		return nil
	}
	if !e.bindingsSealed.Load() {
		e.Mu.RLock()
		defer e.Mu.RUnlock()
	}
	out := make(map[string]Object, len(e.Store))
	for k, v := range e.Store {
		out[k] = v
	}
	return out
}

func (e *Environment) Names() []string {
	snapshot := e.Snapshot()
	names := make([]string, 0, len(snapshot))
	for name := range snapshot {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func NewEnclosedEnvironment(outer *Environment) *Environment {
	return &Environment{
		Store:                  make(map[string]Object),
		Constants:              make(map[string]struct{}),
		Outer:                  outer,
		ModuleContext:          outer.ModuleContext,
		ModuleDir:              outer.ModuleDir,
		SourcePath:             outer.SourcePath,
		ModuleCache:            outer.ModuleCache,
		ModuleLoading:          outer.ModuleLoading,
		RuntimeLimits:          outer.RuntimeLimits,
		SecurityPolicy:         outer.SecurityPolicy,
		Output:                 outer.Output,
		RenderConfig:           outer.RenderConfig,
		RenderArtifacts:        outer.RenderArtifacts,
		RenderArtifactSink:     outer.RenderArtifactSink,
		CollectRenderArtifacts: outer.CollectRenderArtifacts,
		CallStack:              append([]CallFrame(nil), outer.CallStack...),
		Cleanup:                outer.Cleanup,
		YieldSink:              outer.YieldSink,
	}
}

func (e *Environment) AddRenderArtifact(art *RenderArtifact) {
	if e == nil || art == nil {
		return
	}
	e.Mu.Lock()
	defer e.Mu.Unlock()
	if e.RenderArtifactSink != nil {
		*e.RenderArtifactSink = append(*e.RenderArtifactSink, art)
		e.RenderArtifacts = *e.RenderArtifactSink
		return
	}
	e.RenderArtifacts = append(e.RenderArtifacts, art)
}

func (e *Environment) RegisterCleanup(fn func()) {
	if e == nil || fn == nil {
		return
	}
	e.Mu.Lock()
	e.Cleanup = append(e.Cleanup, fn)
	e.Mu.Unlock()
}

func (e *Environment) RunCleanup() {
	if e == nil {
		return
	}
	e.Mu.Lock()
	cleanup := e.Cleanup
	e.Cleanup = nil
	e.Mu.Unlock()
	for i := len(cleanup) - 1; i >= 0; i-- {
		func(fn func()) {
			defer func() { _ = recover() }()
			fn()
		}(cleanup[i])
	}
}

func (e *Environment) EnsureOwnerID() string {
	if e == nil {
		return ""
	}
	if e.OwnerID == "" {
		e.OwnerID = "env-" + strconv.FormatUint(environmentOwnerIDCounter.Add(1), 10)
	}
	return e.OwnerID
}

// HasOwner reports whether ownerID belongs to e or any of its ancestor
// scopes. Reading a variable through a nested scope (closures, loop
// bodies, blocks) walks up this chain via the normal lexical lookup, so
// an owner found anywhere in the chain is a legitimate read rather than
// a cross-scope move.
func (e *Environment) HasOwner(ownerID string) bool {
	for cur := e; cur != nil; cur = cur.Outer {
		if cur.OwnerID == ownerID {
			return true
		}
	}
	return false
}

func (e *Environment) ModuleCacheMap() map[string]ModuleCacheEntry {
	if e.ModuleCache == nil {
		e.ModuleCache = make(map[string]ModuleCacheEntry)
	}
	return e.ModuleCache
}

func (e *Environment) ModuleLoadingMap() map[string]bool {
	if e.ModuleLoading == nil {
		e.ModuleLoading = make(map[string]bool)
	}
	return e.ModuleLoading
}

func (e *Environment) Get(name string) (Object, bool) {
	var obj Object
	var ok bool
	if e.bindingsSealed.Load() {
		obj, ok = e.Store[name]
	} else {
		e.Mu.RLock()
		obj, ok = e.Store[name]
		e.Mu.RUnlock()
	}
	if !ok && e.Outer != nil {
		obj, ok = e.Outer.Get(name)
	}
	return obj, ok
}

// SealBindings marks the current binding map as immutable and enables
// lock-free reads. It is intended for prepared, expression-only workloads.
// Any subsequent Set, SetConst, or Assign call panics; call Reset to reuse a
// pooled environment as writable again. Seal only after all bindings have
// been injected and while no goroutine is mutating the environment.
func (e *Environment) SealBindings() {
	if e == nil || e.bindingsSealed.Load() {
		return
	}
	e.Mu.Lock()
	e.bindingsSealed.Store(true)
	e.Mu.Unlock()
}

// BindingsSealed reports whether binding mutation has been disabled.
func (e *Environment) BindingsSealed() bool {
	return e != nil && e.bindingsSealed.Load()
}

func (e *Environment) assertBindingsWritable() {
	if e != nil && e.bindingsSealed.Load() {
		panic("cannot mutate sealed environment bindings")
	}
}

func (e *Environment) Set(name string, val Object) Object {
	e.assertBindingsWritable()
	e.Mu.Lock()
	if e.bindingsSealed.Load() {
		e.Mu.Unlock()
		panic("cannot mutate sealed environment bindings")
	}
	e.Store[name] = val
	e.Mu.Unlock()
	return val
}

// SetConst creates an immutable binding. The bound object itself is not
// deep-frozen; use immutable() when deep mutation must also be prohibited.
func (e *Environment) SetConst(name string, val Object) Object {
	e.assertBindingsWritable()
	e.Mu.Lock()
	if e.bindingsSealed.Load() {
		e.Mu.Unlock()
		panic("cannot mutate sealed environment bindings")
	}
	e.Store[name] = val
	if e.Constants == nil {
		e.Constants = make(map[string]struct{})
	}
	e.Constants[name] = struct{}{}
	e.Mu.Unlock()
	return val
}

// IsConstant reports whether name resolves to a constant in the lexical chain.
func (e *Environment) IsConstant(name string) bool {
	if e == nil {
		return false
	}
	var present, constant bool
	if e.bindingsSealed.Load() {
		_, present = e.Store[name]
		_, constant = e.Constants[name]
	} else {
		e.Mu.RLock()
		_, present = e.Store[name]
		_, constant = e.Constants[name]
		e.Mu.RUnlock()
	}
	if present {
		return constant
	}
	return e.Outer != nil && e.Outer.IsConstant(name)
}

func (e *Environment) Assign(name string, val Object) (Object, bool) {
	e.assertBindingsWritable()
	e.Mu.Lock()
	if e.bindingsSealed.Load() {
		e.Mu.Unlock()
		panic("cannot mutate sealed environment bindings")
	}
	_, ok := e.Store[name]
	if ok {
		if _, constant := e.Constants[name]; constant {
			e.Mu.Unlock()
			return nil, false
		}
		e.Store[name] = val
		e.Mu.Unlock()
		return val, true
	}
	e.Mu.Unlock()
	if e.Outer != nil {
		return e.Outer.Assign(name, val)
	}
	return nil, false
}

// ---------------------------------------------------------------------------
// Singletons
// ---------------------------------------------------------------------------

var (
	TRUE  = &Boolean{Value: true}
	FALSE = &Boolean{Value: false}
	NULL  = &Null{}
	BREAK = &Break{}
	CONT  = &Continue{}
)

// ---------------------------------------------------------------------------
// Integer cache
// ---------------------------------------------------------------------------

const (
	defaultIntCacheMin = int64(-256)
	defaultIntCacheMax = int64(1_000_000)
	maxIntCacheLimit   = int64(2_000_000)
)

var (
	intCacheMin  = defaultIntCacheMin
	intCacheMax  = defaultIntCacheMax
	integerCache []*Integer
)

func init() {
	if cfgMax := ParsePositiveInt64Env("SPL_INT_CACHE_MAX"); cfgMax > 0 {
		if cfgMax > maxIntCacheLimit {
			cfgMax = maxIntCacheLimit
		}
		if cfgMax >= defaultIntCacheMin {
			intCacheMax = cfgMax
		}
	}

	cacheLen := int(intCacheMax-intCacheMin) + 1
	integerCache = make([]*Integer, cacheLen)
	for i := intCacheMin; i <= intCacheMax; i++ {
		integerCache[int(i-intCacheMin)] = &Integer{Value: i}
	}
}

// IntegerObj returns an *Integer, using the cache for small values.
func IntegerObj(v int64) *Integer {
	if v >= intCacheMin && v <= intCacheMax {
		return integerCache[int(v-intCacheMin)]
	}
	return &Integer{Value: v}
}

// ---------------------------------------------------------------------------
// Environment-variable helpers for runtime configuration
// ---------------------------------------------------------------------------

// ParsePositiveIntEnv reads an environment variable and returns its value as
// a positive int. Returns 0 if unset, empty, or not a positive integer.
func ParsePositiveIntEnv(name string) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// ParsePositiveInt64Env reads an environment variable and returns its value as
// a positive int64. Returns 0 if unset, empty, or not a positive integer.
func ParsePositiveInt64Env(name string) int64 {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func ParseBoolEnvDefault(name string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func ParseCSVEnv(name string) []string {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// LoadRuntimeLimitsFromEnv builds a RuntimeLimits from SPL_MAX_RECURSION,
// SPL_MAX_STEPS, SPL_EVAL_TIMEOUT_MS, and SPL_MAX_HEAP_MB env vars.
// Returns nil when none are set.
func LoadRuntimeLimitsFromEnv() *RuntimeLimits {
	maxDepth := ParsePositiveIntEnv("SPL_MAX_RECURSION")
	maxSteps := ParsePositiveInt64Env("SPL_MAX_STEPS")
	timeoutMs := ParsePositiveInt64Env("SPL_EVAL_TIMEOUT_MS")
	maxHeapMB := ParsePositiveInt64Env("SPL_MAX_HEAP_MB")

	if maxDepth == 0 && maxSteps == 0 && timeoutMs == 0 && maxHeapMB == 0 {
		return nil
	}

	rl := &RuntimeLimits{
		MaxDepth:       maxDepth,
		MaxSteps:       maxSteps,
		MaxHeapBytes:   uint64(maxHeapMB) * 1024 * 1024,
		HeapCheckEvery: 128,
	}
	if timeoutMs > 0 {
		rl.Deadline = time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	}
	return rl
}

func LoadRenderConfigFromEnv() *RenderConfig {
	cfg := DefaultRenderConfig()
	if mode := strings.TrimSpace(os.Getenv("SPL_RENDER_MODE")); mode != "" {
		cfg.Mode = strings.ToLower(mode)
	}
	if protocol := strings.TrimSpace(os.Getenv("SPL_RENDER_TERMINAL_PROTOCOL")); protocol != "" {
		cfg.TerminalProtocol = strings.ToLower(protocol)
	}
	if maxBytes := ParsePositiveInt64Env("SPL_RENDER_MAX_BYTES"); maxBytes > 0 {
		cfg.MaxBytes = maxBytes
	}
	cfg.AllowURLs = ParseBoolEnvDefault("SPL_RENDER_ALLOW_URLS", false)
	cfg.AllowURLHosts = ParseCSVEnv("SPL_RENDER_ALLOW_URL_HOSTS")
	return cfg
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// NativeBoolToBooleanObject converts a Go bool to the canonical Boolean singleton.
func NativeBoolToBooleanObject(input bool) *Boolean {
	if input {
		return TRUE
	}
	return FALSE
}

// NewError creates a new Error object with an auto-detected error code.
func NewError(format string, args ...interface{}) Object {
	msg := fmt.Sprintf(format, args...)
	code := "E_RUNTIME"
	lower := strings.ToLower(strings.TrimSpace(msg))
	switch {
	case strings.Contains(lower, "identifier not found"):
		code = "E_NAME"
	case strings.Contains(lower, "type mismatch"):
		code = "E_TYPE"
	case strings.Contains(lower, "parse"):
		code = "E_PARSE"
	case strings.Contains(lower, "permission") || strings.Contains(lower, "denied"):
		code = "E_PERMISSION"
	}
	return &Error{Message: msg, Code: code}
}

// IsError returns true when obj is an error value.
func IsError(obj Object) bool {
	switch v := obj.(type) {
	case *Error:
		return true
	case *String:
		return strings.HasPrefix(v.Value, "ERROR:")
	default:
		_ = v
		return false
	}
}

// IsTruthy returns whether an SPL object is truthy.
func IsTruthy(obj Object) bool {
	if imm, ok := obj.(*ImmutableValue); ok {
		obj = imm.Inner
	}
	if gen, ok := obj.(*GeneratorValue); ok {
		obj = &Array{Elements: gen.Elements}
	}
	if owned, ok := obj.(*OwnedValue); ok {
		obj = owned.Inner
	}
	switch obj {
	case NULL:
		return false
	case TRUE:
		return true
	case FALSE:
		return false
	default:
		return true
	}
}
