package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oarkflow/interpreter"
)

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type lspServer struct {
	in       io.Reader
	out      io.Writer
	err      io.Writer
	sendMu   sync.Mutex
	root     string
	index    *WorkspaceIndex
	docs     map[string]string
	sessions map[string]*interpreter.Session
	closing  bool
}

func runLSPServer(stdin io.Reader, stdout, stderr io.Writer) int {
	s := newLSPServer(stdin, stdout, stderr)
	if err := s.serve(); err != nil && !s.closing {
		fmt.Fprintf(stderr, "lsp error: %v\n", err)
		return 1
	}
	return 0
}

func newLSPServer(stdin io.Reader, stdout, stderr io.Writer) *lspServer {
	root, _ := os.Getwd()
	return &lspServer{
		in:       stdin,
		out:      stdout,
		err:      stderr,
		root:     root,
		index:    NewWorkspaceIndex(root),
		docs:     map[string]string{},
		sessions: map[string]*interpreter.Session{},
	}
}

func (s *lspServer) serve() error {
	br := bufio.NewReader(s.in)
	for {
		body, err := readRPCBody(br)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		var msg rpcMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			continue
		}
		s.handleMessage(msg)
		if s.closing {
			return nil
		}
	}
}

func readRPCBody(br *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(parts[0]), "Content-Length") {
			if _, err := fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &contentLength); err != nil {
				return nil, err
			}
		}
	}
	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length")
	}
	body := make([]byte, contentLength)
	_, err := io.ReadFull(br, body)
	return body, err
}

func (s *lspServer) handleMessage(msg rpcMessage) {
	if msg.Method == "" {
		return
	}
	result, err := s.dispatch(msg.Method, msg.Params)
	if len(msg.ID) == 0 {
		return
	}
	if err != nil {
		s.sendResponse(msg.ID, nil, &rpcError{Code: -32603, Message: err.Error()})
		return
	}
	s.sendResponse(msg.ID, result, nil)
}

func (s *lspServer) dispatch(method string, params json.RawMessage) (any, error) {
	switch method {
	case "initialize":
		return s.initialize(params)
	case "initialized":
		return nil, nil
	case "shutdown":
		return nil, nil
	case "exit":
		s.closing = true
		return nil, nil
	case "textDocument/didOpen":
		s.didOpen(params)
		return nil, nil
	case "textDocument/didChange":
		s.didChange(params)
		return nil, nil
	case "textDocument/didSave":
		s.didSave(params)
		return nil, nil
	case "textDocument/didClose":
		s.didClose(params)
		return nil, nil
	case "workspace/didChangeWatchedFiles":
		s.didChangeWatchedFiles(params)
		return nil, nil
	case "textDocument/completion":
		return s.completion(params), nil
	case "textDocument/hover":
		return s.hover(params), nil
	case "textDocument/definition":
		return s.definition(params), nil
	case "textDocument/references":
		return s.references(params), nil
	case "textDocument/documentSymbol":
		return s.documentSymbols(params), nil
	case "workspace/symbol":
		return s.workspaceSymbols(params), nil
	case "textDocument/formatting":
		return s.formatting(params), nil
	case "spl/evaluate":
		return s.evaluate(params), nil
	case "spl/sessionCheckpoint":
		return s.sessionCheckpoint(params), nil
	case "spl/sessionRestore":
		return s.sessionRestore(params), nil
	case "spl/sessionInspect":
		return s.sessionInspect(params), nil
	case "spl/refreshIndex":
		s.index.Refresh()
		return map[string]any{"ok": true, "documents": len(s.index.Documents)}, nil
	default:
		return nil, nil
	}
}

func (s *lspServer) initialize(params json.RawMessage) (any, error) {
	var p struct {
		RootURI          string `json:"rootUri"`
		RootPath         string `json:"rootPath"`
		WorkspaceFolders []struct {
			URI string `json:"uri"`
		} `json:"workspaceFolders"`
	}
	_ = json.Unmarshal(params, &p)
	root := ""
	if len(p.WorkspaceFolders) > 0 {
		root = uriToPath(p.WorkspaceFolders[0].URI)
	} else if p.RootURI != "" {
		root = uriToPath(p.RootURI)
	} else if p.RootPath != "" {
		root = p.RootPath
	}
	if root != "" {
		s.root = cleanAbsPath(root)
		s.index = NewWorkspaceIndex(s.root)
	}
	return map[string]any{
		"capabilities": map[string]any{
			"textDocumentSync":           1,
			"completionProvider":         map[string]any{"resolveProvider": false, "triggerCharacters": []string{".", "_"}},
			"hoverProvider":              true,
			"definitionProvider":         true,
			"referencesProvider":         true,
			"documentSymbolProvider":     true,
			"workspaceSymbolProvider":    true,
			"documentFormattingProvider": true,
		},
		"serverInfo": map[string]any{"name": "spltool-lsp", "version": "0.1.0"},
	}, nil
}

func (s *lspServer) didOpen(params json.RawMessage) {
	var p struct {
		TextDocument struct {
			URI  string `json:"uri"`
			Text string `json:"text"`
		} `json:"textDocument"`
	}
	_ = json.Unmarshal(params, &p)
	s.setDocument(p.TextDocument.URI, p.TextDocument.Text)
}

func (s *lspServer) didChange(params json.RawMessage) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		ContentChanges []struct {
			Text string `json:"text"`
		} `json:"contentChanges"`
	}
	_ = json.Unmarshal(params, &p)
	if len(p.ContentChanges) == 0 {
		return
	}
	s.setDocument(p.TextDocument.URI, p.ContentChanges[len(p.ContentChanges)-1].Text)
}

func (s *lspServer) didSave(params json.RawMessage) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Text *string `json:"text,omitempty"`
	}
	_ = json.Unmarshal(params, &p)
	if p.Text != nil {
		s.setDocument(p.TextDocument.URI, *p.Text)
		return
	}
	path := uriToPath(p.TextDocument.URI)
	if data, err := os.ReadFile(path); err == nil {
		s.setDocument(p.TextDocument.URI, string(data))
	}
}

func (s *lspServer) didClose(params json.RawMessage) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	_ = json.Unmarshal(params, &p)
	delete(s.docs, p.TextDocument.URI)
}

func (s *lspServer) didChangeWatchedFiles(params json.RawMessage) {
	var p struct {
		Changes []struct {
			URI  string `json:"uri"`
			Type int    `json:"type"`
		} `json:"changes"`
	}
	_ = json.Unmarshal(params, &p)
	for _, change := range p.Changes {
		path := uriToPath(change.URI)
		if change.Type == 3 {
			s.index.Remove(path)
			continue
		}
		if data, err := os.ReadFile(path); err == nil {
			s.index.Update(path, string(data))
		}
	}
}

func (s *lspServer) setDocument(uri, text string) {
	path := uriToPath(uri)
	s.docs[uri] = text
	s.index.Update(path, text)
	s.publishDiagnostics(uri, path, text)
}

func (s *lspServer) document(uri string) (string, string) {
	if text, ok := s.docs[uri]; ok {
		return uriToPath(uri), text
	}
	path := uriToPath(uri)
	if doc, ok := s.index.Documents[path]; ok {
		return path, doc.Source
	}
	if data, err := os.ReadFile(path); err == nil {
		return path, string(data)
	}
	return path, ""
}

func (s *lspServer) publishDiagnostics(uri, path, src string) {
	report := CheckSource(path, src)
	items := make([]any, 0, len(report.Diagnostics))
	for _, d := range report.Diagnostics {
		items = append(items, map[string]any{
			"range":    lspRange(d.Line, d.Column, max(1, len(wordAt(src, d.Line, d.Column)))),
			"severity": lspDiagnosticSeverity(d.Severity),
			"code":     d.Code,
			"source":   "spltool",
			"message":  diagnosticMessage(d),
		})
	}
	s.notify("textDocument/publishDiagnostics", map[string]any{"uri": uri, "diagnostics": items})
}

func (s *lspServer) completion(params json.RawMessage) any {
	path, src, line, col := s.positionParams(params)
	prefix := prefixAt(src, line, col)
	raw := WorkspaceCompletionItems(s.index, path, src, prefix)
	items := make([]any, 0, len(raw))
	for _, item := range raw {
		items = append(items, map[string]any{
			"label":         item.Label,
			"kind":          lspCompletionKind(item.Kind),
			"detail":        completionDetail(item),
			"documentation": item.Detail,
		})
	}
	return map[string]any{"isIncomplete": false, "items": items}
}

func (s *lspServer) hover(params json.RawMessage) any {
	path, src, line, col := s.positionParams(params)
	value := HoverMarkdown(s.index, path, src, line, col)
	if value == "" {
		return nil
	}
	return map[string]any{"contents": map[string]any{"kind": "markdown", "value": value}}
}

func (s *lspServer) definition(params json.RawMessage) any {
	path, src, line, col := s.positionParams(params)
	loc, ok := s.index.Definition(path, src, line, col)
	if !ok {
		return nil
	}
	return lspLocation(loc.Path, loc.Line, loc.Column, max(1, len(loc.Name)))
}

func (s *lspServer) references(params json.RawMessage) any {
	path, src, line, col := s.positionParams(params)
	refs := s.index.References(path, src, line, col)
	out := make([]any, 0, len(refs))
	for _, ref := range refs {
		out = append(out, lspLocation(ref.Path, ref.Line, ref.Column, len(ref.Name)))
	}
	return out
}

func (s *lspServer) documentSymbols(params json.RawMessage) any {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	_ = json.Unmarshal(params, &p)
	path, src := s.document(p.TextDocument.URI)
	syms := SymbolsForSource(path, src)
	out := make([]any, 0, len(syms))
	for _, sym := range syms {
		out = append(out, map[string]any{
			"name":           sym.Name,
			"kind":           lspSymbolKind(sym.Kind),
			"detail":         sym.Detail,
			"range":          lspRange(sym.Line, sym.Column, max(1, len(sym.Name))),
			"selectionRange": lspRange(sym.Line, sym.Column, max(1, len(sym.Name))),
		})
	}
	return out
}

func (s *lspServer) workspaceSymbols(params json.RawMessage) any {
	var p struct {
		Query string `json:"query"`
	}
	_ = json.Unmarshal(params, &p)
	query := strings.ToLower(p.Query)
	out := []any{}
	for _, sym := range s.index.AllSymbols() {
		if query != "" && !strings.Contains(strings.ToLower(sym.Name), query) {
			continue
		}
		out = append(out, map[string]any{
			"name":          sym.Name,
			"kind":          lspSymbolKind(sym.Kind),
			"containerName": filepath.Base(sym.Path),
			"location":      lspLocation(sym.Path, sym.Line, sym.Column, max(1, len(sym.Name))),
		})
	}
	return out
}

func (s *lspServer) formatting(params json.RawMessage) any {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	_ = json.Unmarshal(params, &p)
	path, src := s.document(p.TextDocument.URI)
	rep := FormatSource(path, src)
	if !rep.OK || rep.Formatted == "" || rep.Formatted == src {
		return []any{}
	}
	return []any{map[string]any{"range": fullDocumentRange(src), "newText": rep.Formatted}}
}

func (s *lspServer) evaluate(params json.RawMessage) any {
	var p struct {
		URI     string            `json:"uri"`
		Text    string            `json:"text"`
		Options EvaluationOptions `json:"options"`
	}
	_ = json.Unmarshal(params, &p)
	path, src := s.document(p.URI)
	if p.Text != "" {
		src = p.Text
	}
	return s.evaluateWithSession(path, src, p.Options)
}

func (s *lspServer) evaluateWithSession(path, src string, opts EvaluationOptions) EvaluationResult {
	start := time.Now()
	var output bytes.Buffer
	sess, err := s.sessionForPath(path, opts, &output)
	if err != nil {
		return EvaluationResult{OK: false, Path: path, Error: err.Error(), Duration: time.Since(start).Milliseconds()}
	}
	sess.SetOutput(&output)
	res := sess.Execute(interpreter.ExecutionRequest{Source: src, Path: path})
	out := EvaluationResult{
		OK:          res.OK,
		Path:        path,
		Result:      res.ResultText,
		Output:      output.String(),
		Error:       res.Error,
		Diagnostics: res.Diagnostics,
		Duration:    res.Metrics.Duration.Milliseconds(),
		Metrics: map[string]any{
			"steps":       res.Metrics.Steps,
			"outputBytes": res.Metrics.OutputBytes,
			"resultType":  res.Metrics.ResultType,
			"durationMs":  res.Metrics.Duration.Milliseconds(),
			"executionId": string(res.ID),
			"sessionId":   string(res.SessionID),
		},
	}
	for _, ev := range sess.Events() {
		out.Events = append(out.Events, map[string]any{
			"kind":    ev.Kind,
			"time":    ev.Time.Format(time.RFC3339Nano),
			"message": ev.Message,
		})
	}
	for _, art := range res.Artifacts {
		out.Artifacts = append(out.Artifacts, map[string]any{
			"kind":       art.Kind,
			"name":       art.Name,
			"mime":       art.MIME,
			"source":     art.Source,
			"sourceType": art.SourceTyp,
			"width":      art.Width,
			"height":     art.Height,
		})
	}
	return out
}

func (s *lspServer) sessionForPath(path string, opts EvaluationOptions, output io.Writer) (*interpreter.Session, error) {
	key := path
	if strings.TrimSpace(key) == "" {
		key = "<memory>"
	}
	if sess := s.sessions[key]; sess != nil {
		return sess, nil
	}
	timeout := time.Duration(opts.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 1500 * time.Millisecond
	}
	maxOutput := opts.MaxOutputBytes
	if maxOutput <= 0 {
		maxOutput = 64 * 1024
	}
	maxSteps := opts.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 200_000
	}
	maxDepth := opts.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 128
	}
	profile := strings.ToLower(strings.TrimSpace(opts.Profile))
	if profile == "" {
		profile = "untrusted"
	}
	rt, err := interpreter.NewRuntime(interpreter.RuntimeOptions{
		Profile:                profile,
		ModuleDir:              ModuleDirForPath(path),
		MaxSteps:               maxSteps,
		MaxDepth:               maxDepth,
		MaxOutputBytes:         maxOutput,
		Timeout:                timeout,
		Output:                 output,
		AllowInProcessFallback: true,
	})
	if err != nil {
		return nil, err
	}
	sess, err := rt.NewSession(interpreter.SessionOptions{
		ID:             interpreter.SessionID("lsp-" + strings.ReplaceAll(filepath.Clean(key), string(filepath.Separator), "-")),
		Profile:        profile,
		ModuleDir:      ModuleDirForPath(path),
		SourcePath:     path,
		Output:         output,
		MaxSteps:       maxSteps,
		MaxDepth:       maxDepth,
		MaxOutputBytes: maxOutput,
		Timeout:        timeout,
		EventLimit:     64,
	})
	if err != nil {
		return nil, err
	}
	s.sessions[key] = sess
	return sess, nil
}

func (s *lspServer) sessionCheckpoint(params json.RawMessage) any {
	var p struct {
		URI  string `json:"uri"`
		Name string `json:"name"`
	}
	_ = json.Unmarshal(params, &p)
	path, _ := s.document(p.URI)
	sess, err := s.sessionForPath(path, EvaluationOptions{}, io.Discard)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	snap, err := sess.Checkpoint(p.Name)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{"ok": true, "checkpoint": snap.ID, "variables": snap.Variables}
}

func (s *lspServer) sessionRestore(params json.RawMessage) any {
	var p struct {
		URI  string `json:"uri"`
		Name string `json:"name"`
	}
	_ = json.Unmarshal(params, &p)
	path, _ := s.document(p.URI)
	sess, err := s.sessionForPath(path, EvaluationOptions{}, io.Discard)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	if err := sess.Restore(p.Name); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{"ok": true}
}

func (s *lspServer) sessionInspect(params json.RawMessage) any {
	var p struct {
		URI string `json:"uri"`
	}
	_ = json.Unmarshal(params, &p)
	path, _ := s.document(p.URI)
	sess, err := s.sessionForPath(path, EvaluationOptions{}, io.Discard)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	return map[string]any{"ok": true, "session": sess.Inspect()}
}

func (s *lspServer) positionParams(params json.RawMessage) (string, string, int, int) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"position"`
	}
	_ = json.Unmarshal(params, &p)
	path, src := s.document(p.TextDocument.URI)
	return path, src, p.Position.Line + 1, p.Position.Character + 1
}

func (s *lspServer) notify(method string, params any) {
	s.send(rpcMessage{JSONRPC: "2.0", Method: method, Params: mustRaw(params)})
}

func (s *lspServer) sendResponse(id json.RawMessage, result any, respErr *rpcError) {
	msg := map[string]any{
		"jsonrpc": "2.0",
	}
	var decodedID any
	if err := json.Unmarshal(id, &decodedID); err != nil {
		decodedID = string(id)
	}
	msg["id"] = decodedID
	if respErr != nil {
		msg["error"] = respErr
	} else {
		msg["result"] = result
	}
	s.sendRaw(msg)
}

func (s *lspServer) send(msg rpcMessage) {
	s.sendRaw(msg)
}

func (s *lspServer) sendRaw(msg any) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	_, _ = fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n", len(data))
	_, _ = s.out.Write(data)
}

func mustRaw(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

func lspDiagnosticSeverity(s DiagnosticSeverity) int {
	switch s {
	case SeverityError:
		return 1
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 3
	default:
		return 3
	}
}

func diagnosticMessage(d Diagnostic) string {
	if d.Hint != "" {
		return d.Message + "\n" + d.Hint
	}
	return d.Message
}

func lspCompletionKind(kind string) int {
	switch kind {
	case "function", "builtin":
		return 3
	case "constructor":
		return 4
	case "class":
		return 7
	case "module", "import":
		return 9
	case "interface":
		return 8
	case "keyword":
		return 14
	case "type":
		return 13
	default:
		return 6
	}
}

func lspSymbolKind(kind string) int {
	switch kind {
	case "function", "builtin":
		return 12
	case "constructor":
		return 9
	case "class":
		return 5
	case "interface":
		return 11
	case "module", "import":
		return 2
	case "type":
		return 23
	case "test":
		return 12
	default:
		return 13
	}
}

func lspLocation(path string, line, col, width int) map[string]any {
	return map[string]any{"uri": pathToURI(path), "range": lspRange(line, col, width)}
}

func lspRange(line, col, width int) map[string]any {
	if line <= 0 {
		line = 1
	}
	if col <= 0 {
		col = 1
	}
	if width <= 0 {
		width = 1
	}
	startLine := line - 1
	startChar := col - 1
	return map[string]any{
		"start": map[string]any{"line": startLine, "character": startChar},
		"end":   map[string]any{"line": startLine, "character": startChar + width},
	}
}

func fullDocumentRange(src string) map[string]any {
	lines := strings.Split(src, "\n")
	lastLine := len(lines) - 1
	lastChar := len([]rune(lines[len(lines)-1]))
	return map[string]any{
		"start": map[string]any{"line": 0, "character": 0},
		"end":   map[string]any{"line": lastLine, "character": lastChar},
	}
}

func encodeRPC(method string, id any, params any) []byte {
	msg := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
	if id != nil {
		msg["id"] = id
	}
	data, _ := json.Marshal(msg)
	var out bytes.Buffer
	fmt.Fprintf(&out, "Content-Length: %d\r\n\r\n", len(data))
	out.Write(data)
	return out.Bytes()
}
