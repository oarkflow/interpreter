package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/lexer"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/parser"
	"github.com/oarkflow/interpreter/pkg/sandbox"
	"github.com/oarkflow/interpreter/pkg/security"
)

type SessionID string
type ExecutionID string
type CheckpointID string

type EventKind string

const (
	EventExecutionStart  EventKind = "execution.start"
	EventExecutionFinish EventKind = "execution.finish"
	EventOutput          EventKind = "output"
	EventDiagnostic      EventKind = "diagnostic"
	EventMetric          EventKind = "metric"
	EventTrace           EventKind = "trace"
	EventRenderArtifact  EventKind = "render_artifact"
	EventError           EventKind = "error"
	EventDebug           EventKind = "debug"
	EventCheckpoint      EventKind = "checkpoint"
	EventRestore         EventKind = "restore"
	EventReplay          EventKind = "replay"
)

type AssistantRequest struct {
	Kind    string
	Prompt  string
	Context SessionInspect
}

type AssistantResponse struct {
	Text string
}

type AssistantProvider interface {
	Assist(AssistantRequest) (AssistantResponse, error)
}

type SessionOptions struct {
	ID             SessionID
	Profile        string
	Args           []string
	ModuleDir      string
	SourcePath     string
	Output         io.Writer
	Security       *object.SecurityPolicy
	Sandbox        *sandbox.SandboxConfig
	MaxDepth       int
	MaxSteps       int64
	MaxHeapMB      int64
	MaxOutputBytes int64
	Timeout        time.Duration
	Persist        bool
	PersistenceDir string
	Assistant      AssistantProvider
	EventLimit     int
}

type ExecutionRequest struct {
	Source     string
	Path       string
	Data       map[string]interface{}
	Print      bool
	Replay     bool
	Record     bool
	Checkpoint CheckpointID
}

type ExecutionMetrics struct {
	Duration    time.Duration `json:"duration"`
	Steps       int64         `json:"steps"`
	OutputBytes int64         `json:"output_bytes"`
	ResultType  string        `json:"result_type,omitempty"`
	Error       string        `json:"error,omitempty"`
}

type ArtifactSummary struct {
	Kind      string `json:"kind,omitempty"`
	Name      string `json:"name,omitempty"`
	MIME      string `json:"mime,omitempty"`
	Source    string `json:"source,omitempty"`
	SourceTyp string `json:"source_type,omitempty"`
	Width     int64  `json:"width,omitempty"`
	Height    int64  `json:"height,omitempty"`
}

type ExecutionResult struct {
	ID          ExecutionID       `json:"id"`
	SessionID   SessionID         `json:"session_id"`
	Source      string            `json:"source,omitempty"`
	Path        string            `json:"path,omitempty"`
	OK          bool              `json:"ok"`
	Result      object.Object     `json:"-"`
	ResultText  string            `json:"result,omitempty"`
	Output      string            `json:"output,omitempty"`
	Error       string            `json:"error,omitempty"`
	Diagnostics []string          `json:"diagnostics,omitempty"`
	Artifacts   []ArtifactSummary `json:"artifacts,omitempty"`
	Metrics     ExecutionMetrics  `json:"metrics"`
	StartedAt   time.Time         `json:"started_at"`
	FinishedAt  time.Time         `json:"finished_at"`
}

type DebugStep struct {
	Index      int               `json:"index"`
	Statement  string            `json:"statement"`
	OK         bool              `json:"ok"`
	Result     string            `json:"result,omitempty"`
	Error      string            `json:"error,omitempty"`
	Variables  map[string]string `json:"variables,omitempty"`
	DurationMS int64             `json:"duration_ms"`
}

type DebugTrace struct {
	SessionID   SessionID   `json:"session_id"`
	ExecutionID ExecutionID `json:"execution_id"`
	OK          bool        `json:"ok"`
	Steps       []DebugStep `json:"steps"`
	Error       string      `json:"error,omitempty"`
}

type ExecutionEvent struct {
	ID          string         `json:"id"`
	SessionID   SessionID      `json:"session_id"`
	ExecutionID ExecutionID    `json:"execution_id,omitempty"`
	Kind        EventKind      `json:"kind"`
	Time        time.Time      `json:"time"`
	Message     string         `json:"message,omitempty"`
	Data        map[string]any `json:"data,omitempty"`
}

type SessionSnapshot struct {
	ID          CheckpointID      `json:"id"`
	SessionID   SessionID         `json:"session_id"`
	CreatedAt   time.Time         `json:"created_at"`
	Description string            `json:"description,omitempty"`
	Variables   map[string]string `json:"variables"`
	store       map[string]object.Object
	historyLen  int
}

type SessionInspect struct {
	ID             SessionID         `json:"id"`
	Profile        string            `json:"profile"`
	ModuleDir      string            `json:"module_dir,omitempty"`
	SourcePath     string            `json:"source_path,omitempty"`
	Variables      map[string]string `json:"variables"`
	History        []ExecutionResult `json:"history"`
	Checkpoints    []SessionSnapshot `json:"checkpoints"`
	Events         []ExecutionEvent  `json:"events"`
	RuntimeLimits  map[string]any    `json:"runtime_limits,omitempty"`
	RenderArtifact int               `json:"render_artifacts,omitempty"`
}

type RuntimeInspector interface {
	Inspect() SessionInspect
}

type Session struct {
	mu           sync.Mutex
	id           SessionID
	profile      string
	env          *object.Environment
	opts         SessionOptions
	history      []ExecutionResult
	checkpoints  map[CheckpointID]SessionSnapshot
	events       []ExecutionEvent
	eventLimit   int
	execCounter  atomic.Uint64
	eventCounter atomic.Uint64
}

func New(opts SessionOptions) (*Session, error) {
	env := object.NewGlobalEnvironment(opts.Args)
	if strings.TrimSpace(opts.SourcePath) == "" {
		opts.SourcePath = "<session>"
	}
	env.SourcePath = opts.SourcePath
	moduleDir := opts.ModuleDir
	if strings.TrimSpace(moduleDir) == "" {
		moduleDir = "."
	}
	env.ModuleDir = moduleDir
	if opts.Output != nil {
		env.Output = opts.Output
	}
	if opts.Security != nil {
		env.SecurityPolicy = opts.Security
	}
	if opts.Sandbox != nil {
		env.RuntimeLimits = runtimeLimitsFromSandbox(*opts.Sandbox)
		if env.SecurityPolicy == nil {
			env.SecurityPolicy = securityPolicyFromSandbox(*opts.Sandbox)
		}
	}
	applyRuntimeOptions(env, opts)
	return NewWithEnvironment(env, opts)
}

func NewWithEnvironment(env *object.Environment, opts SessionOptions) (*Session, error) {
	if env == nil {
		env = object.NewGlobalEnvironment(opts.Args)
	}
	id := opts.ID
	if strings.TrimSpace(string(id)) == "" {
		id = SessionID(fmt.Sprintf("session-%d", time.Now().UnixNano()))
	}
	profile := strings.TrimSpace(opts.Profile)
	if profile == "" {
		profile = "trusted"
	}
	if strings.TrimSpace(env.ModuleDir) == "" {
		env.ModuleDir = opts.ModuleDir
	}
	if strings.TrimSpace(env.ModuleDir) == "" {
		env.ModuleDir = "."
	}
	if strings.TrimSpace(env.SourcePath) == "" {
		env.SourcePath = opts.SourcePath
	}
	if strings.TrimSpace(env.SourcePath) == "" {
		env.SourcePath = "<session>"
	}
	if opts.Output != nil {
		env.Output = opts.Output
	}
	eventLimit := opts.EventLimit
	if eventLimit <= 0 {
		eventLimit = 256
	}
	s := &Session{
		id:          id,
		profile:     profile,
		env:         env,
		opts:        opts,
		checkpoints: map[CheckpointID]SessionSnapshot{},
		eventLimit:  eventLimit,
	}
	if opts.Persist {
		if err := s.persistMetadata(); err != nil {
			return nil, err
		}
	}
	s.checkpoints["initial"] = s.snapshotLocked("initial", "initial session state")
	return s, nil
}

func (s *Session) ID() SessionID {
	if s == nil {
		return ""
	}
	return s.id
}

func (s *Session) Environment() *object.Environment {
	if s == nil {
		return nil
	}
	return s.env
}

func (s *Session) SetOutput(output io.Writer) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.opts.Output = output
	if s.env != nil {
		s.env.Output = output
	}
	s.mu.Unlock()
}

func (s *Session) Execute(req ExecutionRequest) ExecutionResult {
	if s == nil {
		return ExecutionResult{OK: false, Error: "session is nil"}
	}
	if !req.Replay {
		req.Record = true
	}
	start := time.Now()
	execID := ExecutionID(fmt.Sprintf("exec-%d", s.execCounter.Add(1)))
	result := ExecutionResult{
		ID:        execID,
		SessionID: s.id,
		Source:    req.Source,
		Path:      sourcePath(req.Path, s.env),
		StartedAt: start,
	}
	s.emit(EventExecutionStart, execID, "execution started", map[string]any{"path": result.Path})

	var output bytes.Buffer
	envOutput := s.env.Output
	if envOutput == nil {
		envOutput = s.opts.Output
	}
	s.env.Output = io.MultiWriter(&output, discardNil(envOutput))
	prevCollect := s.env.CollectRenderArtifacts
	prevSink := s.env.RenderArtifactSink
	artifacts := []*object.RenderArtifact{}
	s.env.CollectRenderArtifacts = true
	s.env.RenderArtifactSink = &artifacts
	defer func() {
		s.env.Output = envOutput
		s.env.CollectRenderArtifacts = prevCollect
		s.env.RenderArtifactSink = prevSink
	}()

	eval.InjectData(s.env, req.Data)
	program, parserErrors := parse(req.Source)
	if len(parserErrors) != 0 {
		result.OK = false
		result.Diagnostics = append([]string(nil), parserErrors...)
		result.Error = strings.Join(parserErrors, "\n")
		result.Output = output.String()
		result.Artifacts = artifactSummaries(artifacts)
		result.FinishedAt = time.Now()
		result.Metrics = metricsFor(s.env, start, result)
		s.emit(EventDiagnostic, execID, result.Error, map[string]any{"diagnostics": result.Diagnostics})
		s.finish(result, req)
		return result
	}

	prevSourcePath, prevModuleDir := s.env.SourcePath, s.env.ModuleDir
	s.env.SourcePath = result.Path
	if result.Path != "" && result.Path != "<session>" && result.Path != "<repl>" && result.Path != "<memory>" {
		s.env.ModuleDir = filepath.Dir(result.Path)
	}
	defer func() {
		s.env.SourcePath = prevSourcePath
		s.env.ModuleDir = prevModuleDir
	}()

	obj := runProgram(program, s.env)
	result.Result = obj
	result.Output = output.String()
	result.Artifacts = artifactSummaries(artifacts)
	if art, ok := obj.(*object.RenderArtifact); ok {
		result.Artifacts = append(result.Artifacts, artifactSummary(art))
	}
	result.FinishedAt = time.Now()
	if object.IsError(obj) {
		result.OK = false
		result.Error = objectErrorString(obj)
		s.emit(EventError, execID, result.Error, nil)
	} else {
		result.OK = true
		if obj != nil {
			result.ResultText = object.FormatPlain(obj)
		}
	}
	result.Metrics = metricsFor(s.env, start, result)
	s.finish(result, req)
	return result
}

func (s *Session) Checkpoint(name string) (SessionSnapshot, error) {
	if s == nil {
		return SessionSnapshot{}, fmt.Errorf("session is nil")
	}
	id := CheckpointID(strings.TrimSpace(name))
	if id == "" {
		id = CheckpointID(fmt.Sprintf("checkpoint-%d", time.Now().Unix()))
	}
	s.mu.Lock()
	snap := s.snapshotLocked(id, "")
	s.checkpoints[id] = snap
	s.mu.Unlock()
	s.emit(EventCheckpoint, "", "checkpoint created", map[string]any{"checkpoint": string(id)})
	if s.opts.Persist {
		return snap, s.persistCheckpoint(snap)
	}
	return snap, nil
}

func (s *Session) Restore(name string) error {
	if s == nil {
		return fmt.Errorf("session is nil")
	}
	id := CheckpointID(strings.TrimSpace(name))
	if id == "" {
		return fmt.Errorf("checkpoint name is required")
	}
	s.mu.Lock()
	snap, ok := s.checkpoints[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("checkpoint %q not found", id)
	}
	s.env.Mu.Lock()
	s.env.Store = cloneStore(snap.store)
	s.env.Mu.Unlock()
	if snap.historyLen >= 0 && snap.historyLen <= len(s.history) {
		s.history = append([]ExecutionResult(nil), s.history[:snap.historyLen]...)
	}
	s.mu.Unlock()
	s.emit(EventRestore, "", "checkpoint restored", map[string]any{"checkpoint": string(id)})
	return nil
}

func (s *Session) Replay() ([]ExecutionResult, error) {
	if s == nil {
		return nil, fmt.Errorf("session is nil")
	}
	s.mu.Lock()
	history := append([]ExecutionResult(nil), s.history...)
	initial := s.checkpoints["initial"]
	s.env.Mu.Lock()
	s.env.Store = cloneStore(initial.store)
	s.env.Mu.Unlock()
	s.history = nil
	s.mu.Unlock()

	results := make([]ExecutionResult, 0, len(history))
	for _, item := range history {
		if strings.TrimSpace(item.Source) == "" {
			continue
		}
		res := s.Execute(ExecutionRequest{Source: item.Source, Path: item.Path, Replay: true, Record: true})
		results = append(results, res)
	}
	s.emit(EventReplay, "", "session replayed", map[string]any{"executions": len(results)})
	return results, nil
}

func (s *Session) Inspect() SessionInspect {
	if s == nil {
		return SessionInspect{}
	}
	s.mu.Lock()
	history := append([]ExecutionResult(nil), s.history...)
	events := append([]ExecutionEvent(nil), s.events...)
	checkpoints := make([]SessionSnapshot, 0, len(s.checkpoints))
	for _, snap := range s.checkpoints {
		snap.store = nil
		checkpoints = append(checkpoints, snap)
	}
	s.mu.Unlock()
	sort.Slice(checkpoints, func(i, j int) bool { return checkpoints[i].CreatedAt.Before(checkpoints[j].CreatedAt) })
	inspect := SessionInspect{
		ID:             s.id,
		Profile:        s.profile,
		ModuleDir:      s.env.ModuleDir,
		SourcePath:     s.env.SourcePath,
		Variables:      objectSummaryMap(s.env.Snapshot()),
		History:        history,
		Checkpoints:    checkpoints,
		Events:         events,
		RenderArtifact: len(s.env.RenderArtifacts),
	}
	if rl := s.env.RuntimeLimits; rl != nil {
		inspect.RuntimeLimits = map[string]any{
			"steps":        rl.Steps,
			"max_steps":    rl.MaxSteps,
			"output_bytes": rl.OutputBytes,
			"max_output":   rl.MaxOutputBytes,
			"max_depth":    rl.MaxDepth,
		}
	}
	return inspect
}

func (s *Session) Events() []ExecutionEvent {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ExecutionEvent(nil), s.events...)
}

func (s *Session) Debug(src string, path string) DebugTrace {
	trace := DebugTrace{OK: true}
	if s == nil {
		trace.OK = false
		trace.Error = "session is nil"
		return trace
	}
	trace.SessionID = s.id
	trace.ExecutionID = ExecutionID(fmt.Sprintf("debug-%d", s.execCounter.Add(1)))
	program, parserErrors := parse(src)
	if len(parserErrors) != 0 {
		trace.OK = false
		trace.Error = strings.Join(parserErrors, "\n")
		s.emit(EventDebug, trace.ExecutionID, trace.Error, map[string]any{"ok": false})
		return trace
	}
	prevSourcePath, prevModuleDir := s.env.SourcePath, s.env.ModuleDir
	source := sourcePath(path, s.env)
	s.env.SourcePath = source
	if source != "" && source != "<session>" && source != "<repl>" && source != "<memory>" {
		s.env.ModuleDir = filepath.Dir(source)
	}
	defer func() {
		s.env.SourcePath = prevSourcePath
		s.env.ModuleDir = prevModuleDir
	}()
	for i, stmt := range program.Statements {
		start := time.Now()
		obj := runProgram(&ast.Program{Statements: []ast.Statement{stmt}}, s.env)
		step := DebugStep{
			Index:      i + 1,
			Statement:  strings.TrimSpace(stmt.String()),
			OK:         !object.IsError(obj),
			Variables:  objectSummaryMap(s.env.Snapshot()),
			DurationMS: time.Since(start).Milliseconds(),
		}
		if obj != nil && obj.Type() != object.NULL_OBJ {
			step.Result = object.FormatPlain(obj)
		}
		if object.IsError(obj) {
			step.Error = objectErrorString(obj)
			trace.OK = false
			trace.Error = step.Error
			trace.Steps = append(trace.Steps, step)
			s.emit(EventDebug, trace.ExecutionID, "debug step failed", map[string]any{"step": step.Index, "error": step.Error})
			return trace
		}
		trace.Steps = append(trace.Steps, step)
		s.emit(EventDebug, trace.ExecutionID, "debug step", map[string]any{"step": step.Index, "statement": step.Statement})
	}
	return trace
}

func (s *Session) Ask(kind, prompt string) (AssistantResponse, error) {
	if s == nil {
		return AssistantResponse{}, fmt.Errorf("session is nil")
	}
	if s.opts.Assistant == nil {
		return AssistantResponse{}, fmt.Errorf("assistant provider is not configured")
	}
	return s.opts.Assistant.Assist(AssistantRequest{Kind: kind, Prompt: prompt, Context: s.Inspect()})
}

func (s *Session) Close() {
	if s != nil && s.env != nil {
		s.env.RunCleanup()
	}
}

func (s *Session) finish(result ExecutionResult, req ExecutionRequest) {
	if result.Output != "" {
		s.emit(EventOutput, result.ID, "output captured", map[string]any{"bytes": len(result.Output)})
	}
	if result.Result != nil && result.Result.Type() == object.RENDER_ARTIFACT_OBJ {
		s.emit(EventRenderArtifact, result.ID, "render artifact", map[string]any{"result": result.ResultText})
	}
	s.emit(EventMetric, result.ID, "execution metrics", map[string]any{
		"duration_ms":  result.Metrics.Duration.Milliseconds(),
		"steps":        result.Metrics.Steps,
		"output_bytes": result.Metrics.OutputBytes,
	})
	if result.OK {
		s.emit(EventExecutionFinish, result.ID, "execution finished", map[string]any{"ok": true, "result_type": result.Metrics.ResultType})
	} else {
		s.emit(EventExecutionFinish, result.ID, "execution failed", map[string]any{"ok": false, "error": result.Error})
	}
	if req.Record {
		s.mu.Lock()
		s.history = append(s.history, result)
		s.mu.Unlock()
		if s.opts.Persist {
			_ = s.persistExecution(result)
		}
	}
}

func (s *Session) emit(kind EventKind, execID ExecutionID, message string, data map[string]any) {
	if s == nil {
		return
	}
	ev := ExecutionEvent{
		ID:          fmt.Sprintf("event-%d", s.eventCounter.Add(1)),
		SessionID:   s.id,
		ExecutionID: execID,
		Kind:        kind,
		Time:        time.Now(),
		Message:     message,
		Data:        data,
	}
	s.mu.Lock()
	s.events = append(s.events, ev)
	if len(s.events) > s.eventLimit {
		s.events = append([]ExecutionEvent(nil), s.events[len(s.events)-s.eventLimit:]...)
	}
	s.mu.Unlock()
}

func (s *Session) snapshotLocked(id CheckpointID, desc string) SessionSnapshot {
	store := cloneStore(s.env.Snapshot())
	return SessionSnapshot{
		ID:          id,
		SessionID:   s.id,
		CreatedAt:   time.Now(),
		Description: desc,
		Variables:   objectSummaryMap(store),
		store:       store,
		historyLen:  len(s.history),
	}
}

func parse(src string) (*ast.Program, []string) {
	l := lexer.NewLexer(src)
	p := parser.NewParser(l)
	program := p.ParseProgram()
	return program, p.Errors()
}

func runProgram(program *ast.Program, env *object.Environment) object.Object {
	sandbox.ResetRuntimeLimitCounters(env)
	policy := env.SecurityPolicy
	run := func() object.Object {
		return sandbox.WithSandboxRootOverride(env.ModuleDir, func() object.Object {
			return eval.Eval(program, env)
		})
	}
	if policy == nil {
		return run()
	}
	result, err := security.WithSecurityPolicyOverride(policy, func() (any, error) {
		return run(), nil
	})
	if err != nil {
		return object.NewError("%s", err.Error())
	}
	if result == nil {
		return object.NULL
	}
	if obj, ok := result.(object.Object); ok {
		return obj
	}
	return object.NewError("session: expected object result, got %T", result)
}

func metricsFor(env *object.Environment, start time.Time, result ExecutionResult) ExecutionMetrics {
	m := ExecutionMetrics{Duration: time.Since(start)}
	if result.Result != nil {
		m.ResultType = result.Result.Type().String()
	}
	if result.Error != "" {
		m.Error = result.Error
	}
	if env != nil && env.RuntimeLimits != nil {
		m.Steps = env.RuntimeLimits.Steps
		m.OutputBytes = env.RuntimeLimits.OutputBytes
	}
	if result.Output != "" && m.OutputBytes == 0 {
		m.OutputBytes = int64(len(result.Output))
	}
	return m
}

func sourcePath(path string, env *object.Environment) string {
	if strings.TrimSpace(path) != "" {
		return path
	}
	if env != nil && strings.TrimSpace(env.SourcePath) != "" {
		return env.SourcePath
	}
	return "<session>"
}

func objectErrorString(obj object.Object) string {
	if obj == nil {
		return ""
	}
	if errObj, ok := obj.(*object.Error); ok {
		return errObj.Message
	}
	return obj.Inspect()
}

func objectSummaryMap(store map[string]object.Object) map[string]string {
	out := make(map[string]string, len(store))
	for name, obj := range store {
		out[name] = safeInspect(obj)
	}
	return out
}

func safeInspect(obj object.Object) string {
	if obj == nil {
		return "null"
	}
	if obj.Type() == object.SECRET_OBJ {
		return "***"
	}
	return obj.Inspect()
}

func artifactSummaries(items []*object.RenderArtifact) []ArtifactSummary {
	if len(items) == 0 {
		return nil
	}
	out := make([]ArtifactSummary, 0, len(items))
	for _, art := range items {
		if art == nil {
			continue
		}
		out = append(out, artifactSummary(art))
	}
	return out
}

func artifactSummary(art *object.RenderArtifact) ArtifactSummary {
	if art == nil {
		return ArtifactSummary{}
	}
	return ArtifactSummary{
		Kind:      art.Kind,
		Name:      art.Name,
		MIME:      art.MIME,
		Source:    art.Source,
		SourceTyp: art.SourceTyp,
		Width:     art.Width,
		Height:    art.Height,
	}
}

func cloneStore(in map[string]object.Object) map[string]object.Object {
	out := make(map[string]object.Object, len(in))
	for k, v := range in {
		out[k] = cloneObject(v)
	}
	return out
}

func cloneObject(obj object.Object) object.Object {
	switch v := obj.(type) {
	case *object.Integer:
		return &object.Integer{Value: v.Value}
	case *object.Float:
		return &object.Float{Value: v.Value}
	case *object.Boolean:
		return object.NativeBoolToBooleanObject(v.Value)
	case *object.String:
		return &object.String{Value: v.Value}
	case *object.Secret:
		return &object.Secret{Value: v.Value}
	case *object.Array:
		items := make([]object.Object, len(v.Elements))
		for i, item := range v.Elements {
			items[i] = cloneObject(item)
		}
		return &object.Array{Elements: items}
	case *object.Hash:
		pairs := make(map[object.HashKey]object.HashPair, len(v.Pairs))
		for key, pair := range v.Pairs {
			pairs[key] = object.HashPair{Key: cloneObject(pair.Key), Value: cloneObject(pair.Value)}
		}
		return &object.Hash{Pairs: pairs}
	case *object.Null:
		return object.NULL
	default:
		return obj
	}
}

func applyRuntimeOptions(env *object.Environment, opts SessionOptions) {
	if env == nil {
		return
	}
	rl := env.RuntimeLimits
	if rl == nil {
		rl = &object.RuntimeLimits{HeapCheckEvery: 128}
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
	if opts.Timeout > 0 {
		rl.Deadline = time.Now().Add(opts.Timeout)
	}
	env.RuntimeLimits = rl
}

func runtimeLimitsFromSandbox(cfg sandbox.SandboxConfig) *object.RuntimeLimits {
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
	if cfg.MaxOutputBytes > 0 {
		rl.MaxOutputBytes = cfg.MaxOutputBytes
	}
	if cfg.Timeout > 0 {
		rl.Deadline = time.Now().Add(cfg.Timeout)
	}
	return rl
}

func securityPolicyFromSandbox(cfg sandbox.SandboxConfig) *object.SecurityPolicy {
	return &object.SecurityPolicy{
		StrictMode:            cfg.StrictMode,
		ProtectHost:           cfg.ProtectHost,
		AllowEnvWrite:         cfg.AllowEnvWrite,
		AllowedCapabilities:   append([]string(nil), cfg.AllowedCapabilities...),
		DeniedCapabilities:    append([]string(nil), cfg.DeniedCapabilities...),
		AllowedExecCommands:   append([]string(nil), cfg.AllowedExecCommands...),
		AllowedNetworkHosts:   append([]string(nil), cfg.AllowedNetworkHosts...),
		AllowedFileReadPaths:  append([]string(nil), cfg.AllowedFileReadPaths...),
		AllowedFileWritePaths: append([]string(nil), cfg.AllowedFileWritePaths...),
	}
}

func discardNil(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}

func (s *Session) persistDir() string {
	if strings.TrimSpace(s.opts.PersistenceDir) != "" {
		return s.opts.PersistenceDir
	}
	return filepath.Join(".spl", "sessions", string(s.id))
}

func (s *Session) persistMetadata() error {
	dir := s.persistDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(map[string]any{
		"id":         s.id,
		"profile":    s.profile,
		"module_dir": s.env.ModuleDir,
		"created_at": time.Now(),
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "metadata.json"), append(data, '\n'), 0o600)
}

func (s *Session) persistExecution(result ExecutionResult) error {
	dir := s.persistDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	logPath := filepath.Join(dir, "inputs.spl")
	line := result.Source
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	return appendFile(logPath, line)
}

func (s *Session) persistCheckpoint(snap SessionSnapshot) error {
	dir := filepath.Join(s.persistDir(), "checkpoints")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	snap.store = nil
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, string(snap.ID)+".json"), append(data, '\n'), 0o600)
}

func appendFile(path, text string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(text)
	return err
}
