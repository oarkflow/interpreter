package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/security"
	"github.com/oarkflow/interpreter/pkg/template"
)

// ── Template Runtime Interface ────────────────────────────────────

// TemplateRuntime is the interface for server-side template rendering.
type TemplateRuntime interface {
	Render(tmpl string, data map[string]any) (string, error)
	RenderFile(path string, data map[string]any) (string, error)
	RenderSSR(tmpl string, data map[string]any) (string, error)
	RenderSSRFile(path string, data map[string]any) (string, error)
	RenderStream(w io.Writer, tmpl string, data map[string]any) error
	RenderStreamFile(w io.Writer, path string, data map[string]any) error
	InvalidateCaches()
}

// NewTemplateRuntimeFn creates a TemplateRuntime instance. Set from the main package.
// Deprecated: Use template.NewTemplateRuntime instead
var NewTemplateRuntimeFn func(baseDir string) TemplateRuntime

// ── Object types ───────────────────────────────────────────────────

// ── SPLServer ──────────────────────────────────────────────────────

type RouteHandler struct {
	Method  string
	Pattern string
	Handler object.Object // Function or Builtin
}

type MiddlewareEntry struct {
	Path    string
	Handler object.Object
}

type SPLServer struct {
	mu          sync.RWMutex
	routes      []RouteHandler
	middlewares []MiddlewareEntry
	staticDirs  map[string]string // url prefix -> filesystem path
	templateDir string
	template    TemplateRuntime
	env         *object.Environment
	server      *http.Server
	addr        string
	running     bool
	shutdownCh  chan struct{}
}

func (s *SPLServer) Type() object.ObjectType { return object.SERVER_OBJ }
func (s *SPLServer) Inspect() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.running {
		return fmt.Sprintf("<server %s running>", s.addr)
	}
	return fmt.Sprintf("<server %d routes>", len(s.routes))
}

func (s *SPLServer) addRoute(method, pattern string, handler object.Object) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes = append(s.routes, RouteHandler{
		Method:  strings.ToUpper(method),
		Pattern: pattern,
		Handler: handler,
	})
}

func (s *SPLServer) addMiddleware(path string, handler object.Object) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.middlewares = append(s.middlewares, MiddlewareEntry{
		Path:    path,
		Handler: handler,
	})
}

func (s *SPLServer) addStaticDir(prefix, dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.staticDirs == nil {
		s.staticDirs = make(map[string]string)
	}
	s.staticDirs[prefix] = dir
}

func (s *SPLServer) templateRuntime() TemplateRuntime {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.template == nil {
		baseDir := s.templateDir
		if strings.TrimSpace(baseDir) == "" {
			baseDir = "."
		}
		// First try the template package's NewTemplateRuntime
		s.template = template.NewTemplateRuntime(baseDir)
		if s.template == nil && NewTemplateRuntimeFn != nil {
			// Fallback to legacy NewTemplateRuntimeFn
			s.template = NewTemplateRuntimeFn(baseDir)
		}
	}
	return s.template
}

func (s *SPLServer) findRoute(method, path string) (*RouteHandler, map[string]string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.routes {
		r := &s.routes[i]
		if r.Method != "" && r.Method != method && r.Method != "ANY" {
			continue
		}
		if params, ok := matchRoute(r.Pattern, path); ok {
			return r, params
		}
	}
	return nil, nil
}

func matchRoute(pattern, path string) (map[string]string, bool) {
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	pathParts := strings.Split(strings.Trim(path, "/"), "/")

	if len(patternParts) != len(pathParts) {
		if len(patternParts) > 0 && patternParts[len(patternParts)-1] == "*" {
			if len(pathParts) >= len(patternParts)-1 {
				params := make(map[string]string)
				for i := 0; i < len(patternParts)-1; i++ {
					if strings.HasPrefix(patternParts[i], ":") {
						params[patternParts[i][1:]] = pathParts[i]
					} else if patternParts[i] != pathParts[i] {
						return nil, false
					}
				}
				return params, true
			}
		}
		return nil, false
	}

	params := make(map[string]string)
	for i, pp := range patternParts {
		if strings.HasPrefix(pp, ":") {
			params[pp[1:]] = pathParts[i]
		} else if pp != pathParts[i] {
			return nil, false
		}
	}
	return params, true
}

func (s *SPLServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	for prefix, dir := range s.staticDirs {
		if strings.HasPrefix(r.URL.Path, prefix) {
			s.mu.RUnlock()
			relPath := strings.TrimPrefix(r.URL.Path, prefix)
			fullPath := filepath.Join(dir, relPath)
			if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
				http.ServeFile(w, r, fullPath)
				return
			}
			http.NotFound(w, r)
			return
		}
	}
	s.mu.RUnlock()

	route, params := s.findRoute(r.Method, r.URL.Path)
	if route == nil {
		http.NotFound(w, r)
		return
	}

	req := newSPLRequest(r, params)
	res := newSPLResponse(w, s)

	s.mu.RLock()
	var applicableMiddlewares []MiddlewareEntry
	for _, m := range s.middlewares {
		if m.Path == "" || m.Path == "/" || strings.HasPrefix(r.URL.Path, m.Path) {
			applicableMiddlewares = append(applicableMiddlewares, m)
		}
	}
	s.mu.RUnlock()

	s.executeChain(applicableMiddlewares, 0, route.Handler, req, res)
}

func (s *SPLServer) executeChain(middlewares []MiddlewareEntry, idx int, finalHandler object.Object, req *SPLRequest, res *SPLResponse) {
	if idx >= len(middlewares) {
		callSPLHandler(finalHandler, req, res, s.env)
		return
	}

	mw := middlewares[idx]
	nextCalled := false
	nextFn := &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if nextCalled {
				return object.NULL
			}
			nextCalled = true
			s.executeChain(middlewares, idx+1, finalHandler, req, res)
			return object.NULL
		},
	}

	callSPLMiddleware(mw.Handler, req, res, nextFn, s.env)
}

func callSPLHandler(handler object.Object, req *SPLRequest, res *SPLResponse, env *object.Environment) {
	args := []object.Object{req, res}
	switch fn := handler.(type) {
	case *object.Function:
		if object.ExtendFunctionEnvFn != nil && object.EvalFn != nil {
			extEnv := object.ExtendFunctionEnvFn(fn, args, env)
			object.EvalFn(fn.Body, extEnv)
		}
	case *object.Builtin:
		if fn.FnWithEnv != nil {
			fn.FnWithEnv(env, args...)
		} else {
			fn.Fn(args...)
		}
	}
}

func callSPLMiddleware(handler object.Object, req *SPLRequest, res *SPLResponse, next *object.Builtin, env *object.Environment) {
	args := []object.Object{req, res, next}
	switch fn := handler.(type) {
	case *object.Function:
		if object.ExtendFunctionEnvFn != nil && object.EvalFn != nil {
			extEnv := object.ExtendFunctionEnvFn(fn, args, env)
			object.EvalFn(fn.Body, extEnv)
		}
	case *object.Builtin:
		if fn.FnWithEnv != nil {
			fn.FnWithEnv(env, args...)
		} else {
			fn.Fn(args...)
		}
	}
}

// ── SPLRequest ─────────────────────────────────────────────────────

type SPLRequest struct {
	Method  string
	Path    string
	Params  map[string]string
	Query   map[string][]string
	Headers http.Header
	Body    string
	Raw     *http.Request
}

func (r *SPLRequest) Type() object.ObjectType { return object.REQUEST_OBJ }
func (r *SPLRequest) Inspect() string {
	return fmt.Sprintf("<request %s %s>", r.Method, r.Path)
}

func newSPLRequest(r *http.Request, params map[string]string) *SPLRequest {
	var body string
	if r.Body != nil {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		r.Body.Close()
	}
	return &SPLRequest{
		Method:  r.Method,
		Path:    r.URL.Path,
		Params:  params,
		Query:   r.URL.Query(),
		Headers: r.Header,
		Body:    body,
		Raw:     r,
	}
}

// GetRequestProperty returns a property or method of the request object.
func GetRequestProperty(req *SPLRequest, name string) object.Object {
	switch name {
	case "method":
		return &object.String{Value: req.Method}
	case "path":
		return &object.String{Value: req.Path}
	case "body":
		return &object.String{Value: req.Body}
	case "params":
		h := &object.Hash{Pairs: make(map[object.HashKey]object.HashPair)}
		for k, v := range req.Params {
			key := &object.String{Value: k}
			h.Pairs[key.HashKey()] = object.HashPair{Key: key, Value: &object.String{Value: v}}
		}
		return h
	case "query":
		h := &object.Hash{Pairs: make(map[object.HashKey]object.HashPair)}
		for k, vals := range req.Query {
			key := &object.String{Value: k}
			if len(vals) == 1 {
				h.Pairs[key.HashKey()] = object.HashPair{Key: key, Value: &object.String{Value: vals[0]}}
			} else {
				elems := make([]object.Object, len(vals))
				for i, v := range vals {
					elems[i] = &object.String{Value: v}
				}
				h.Pairs[key.HashKey()] = object.HashPair{Key: key, Value: &object.Array{Elements: elems}}
			}
		}
		return h
	case "headers":
		h := &object.Hash{Pairs: make(map[object.HashKey]object.HashPair)}
		for k, vals := range req.Headers {
			key := &object.String{Value: k}
			h.Pairs[key.HashKey()] = object.HashPair{Key: key, Value: &object.String{Value: strings.Join(vals, ", ")}}
		}
		return h
	case "json":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if req.Body == "" {
				return object.NULL
			}
			var data any
			if err := json.Unmarshal([]byte(req.Body), &data); err != nil {
				return object.NULL
			}
			return GoToSPLObject(data)
		}}
	case "param":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("param() requires a parameter name")
			}
			key, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("param() argument must be a string")
			}
			if v, found := req.Params[key.Value]; found {
				return &object.String{Value: v}
			}
			return object.NULL
		}}
	case "get_header":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("get_header() requires a header name")
			}
			key, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("get_header() argument must be a string")
			}
			v := req.Headers.Get(key.Value)
			if v == "" {
				return object.NULL
			}
			return &object.String{Value: v}
		}}
	default:
		return nil
	}
}

// ── SPLResponse ────────────────────────────────────────────────────

type SPLResponse struct {
	Writer     http.ResponseWriter
	Server     *SPLServer
	StatusCode int
	headers    map[string]string
	written    bool
}

func (r *SPLResponse) Type() object.ObjectType { return object.RESPONSE_OBJ }
func (r *SPLResponse) Inspect() string {
	return fmt.Sprintf("<response status=%d>", r.StatusCode)
}

func newSPLResponse(w http.ResponseWriter, srv *SPLServer) *SPLResponse {
	return &SPLResponse{
		Writer:     w,
		Server:     srv,
		StatusCode: 200,
		headers:    make(map[string]string),
	}
}

// GetResponseProperty returns a property or method of the response object.
func GetResponseProperty(res *SPLResponse, name string) object.Object {
	switch name {
	case "status":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("status() requires a status code")
			}
			code, ok := args[0].(*object.Integer)
			if !ok {
				return object.NewError("status() argument must be an integer")
			}
			res.StatusCode = int(code.Value)
			return res
		}}
	case "header":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 2 {
				return object.NewError("header() requires name and value")
			}
			key, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("header() name must be a string")
			}
			val, ok := args[1].(*object.String)
			if !ok {
				return object.NewError("header() value must be a string")
			}
			res.Writer.Header().Set(key.Value, val.Value)
			return res
		}}
	case "json":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("json() requires data")
			}
			res.Writer.Header().Set("Content-Type", "application/json")
			res.Writer.WriteHeader(res.StatusCode)
			res.written = true
			goVal := SPLObjectToGo(args[0])
			data, err := json.Marshal(goVal)
			if err != nil {
				return object.NewError("json marshal error: %s", err)
			}
			res.Writer.Write(data)
			return object.NULL
		}}
	case "text":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("text() requires data")
			}
			str, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("text() argument must be a string")
			}
			res.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
			res.Writer.WriteHeader(res.StatusCode)
			res.written = true
			res.Writer.Write([]byte(str.Value))
			return object.NULL
		}}
	case "html":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("html() requires data")
			}
			str, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("html() argument must be a string")
			}
			res.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			res.Writer.WriteHeader(res.StatusCode)
			res.written = true
			res.Writer.Write([]byte(str.Value))
			return object.NULL
		}}
	case "send":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("send() requires data")
			}
			if !res.written {
				res.Writer.WriteHeader(res.StatusCode)
				res.written = true
			}
			data := args[0].Inspect()
			res.Writer.Write([]byte(data))
			return object.NULL
		}}
	case "redirect":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("redirect() requires a URL")
			}
			url, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("redirect() argument must be a string")
			}
			code := 302
			if len(args) >= 2 {
				if c, ok := args[1].(*object.Integer); ok {
					code = int(c.Value)
				}
			}
			http.Redirect(res.Writer, nil, url.Value, code)
			res.written = true
			return object.NULL
		}}
	case "sse":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			flusher, ok := res.Writer.(http.Flusher)
			if !ok {
				return object.NewError("SSE not supported by this response writer")
			}
			res.Writer.Header().Set("Content-Type", "text/event-stream")
			res.Writer.Header().Set("Cache-Control", "no-cache")
			res.Writer.Header().Set("Connection", "keep-alive")
			res.Writer.WriteHeader(res.StatusCode)
			res.written = true
			flusher.Flush()
			return &SSEWriter{writer: res.Writer, flusher: flusher}
		}}
	case "file":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("file() requires a file path")
			}
			path, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("file() argument must be a string")
			}
			data, err := os.ReadFile(path.Value)
			if err != nil {
				return object.NewError("file read error: %s", err)
			}
			ct := mime.TypeByExtension(filepath.Ext(path.Value))
			if ct == "" {
				ct = "application/octet-stream"
			}
			res.Writer.Header().Set("Content-Type", ct)
			res.Writer.WriteHeader(res.StatusCode)
			res.written = true
			res.Writer.Write(data)
			return object.NULL
		}}
	case "render":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("render() requires a template path")
			}
			if res.Server == nil {
				return object.NewError("render() server context unavailable")
			}
			path, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("render() template path must be a string")
			}
			data := objectArgToMap(args, 1)
			engine := res.Server.templateRuntime()
			if engine == nil {
				return object.NewError("template runtime unavailable")
			}
			htmlStr, err := engine.RenderFile(path.Value, data)
			if err != nil {
				return object.NewError("render error: %s", err)
			}
			res.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			res.Writer.WriteHeader(res.StatusCode)
			res.written = true
			_, _ = res.Writer.Write([]byte(htmlStr))
			return object.NULL
		}}
	case "render_ssr":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("render_ssr() requires a template path")
			}
			if res.Server == nil {
				return object.NewError("render_ssr() server context unavailable")
			}
			path, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("render_ssr() template path must be a string")
			}
			data := objectArgToMap(args, 1)
			engine := res.Server.templateRuntime()
			if engine == nil {
				return object.NewError("template runtime unavailable")
			}
			htmlStr, err := engine.RenderSSRFile(path.Value, data)
			if err != nil {
				return object.NewError("render_ssr error: %s", err)
			}
			res.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			res.Writer.WriteHeader(res.StatusCode)
			res.written = true
			_, _ = res.Writer.Write([]byte(htmlStr))
			return object.NULL
		}}
	case "stream":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("stream() requires a template path")
			}
			if res.Server == nil {
				return object.NewError("stream() server context unavailable")
			}
			path, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("stream() template path must be a string")
			}
			data := objectArgToMap(args, 1)
			engine := res.Server.templateRuntime()
			if engine == nil {
				return object.NewError("template runtime unavailable")
			}
			res.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			if flusher, ok := res.Writer.(http.Flusher); ok {
				res.Writer.WriteHeader(res.StatusCode)
				res.written = true
				flusher.Flush()
			}
			if err := engine.RenderStreamFile(res.Writer, path.Value, data); err != nil {
				return object.NewError("stream error: %s", err)
			}
			return object.NULL
		}}
	default:
		return nil
	}
}

func objectArgToMap(args []object.Object, idx int) map[string]any {
	if len(args) <= idx {
		return nil
	}
	h, ok := args[idx].(*object.Hash)
	if !ok {
		return nil
	}
	result := make(map[string]any)
	for _, pair := range h.Pairs {
		result[pair.Key.Inspect()] = SPLObjectToGo(pair.Value)
	}
	return result
}

// ── SSEWriter ──────────────────────────────────────────────────────

type SSEWriter struct {
	writer  http.ResponseWriter
	flusher http.Flusher
}

func (s *SSEWriter) Type() object.ObjectType { return object.SSE_WRITER_OBJ }
func (s *SSEWriter) Inspect() string         { return "<sse writer>" }

// GetSSEWriterProperty returns a property or method of the SSE writer.
func GetSSEWriterProperty(sse *SSEWriter, name string) object.Object {
	switch name {
	case "send":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("sse.send() requires data")
			}
			var event, data string
			if len(args) >= 2 {
				if e, ok := args[0].(*object.String); ok {
					event = e.Value
				}
				data = args[1].Inspect()
			} else {
				data = args[0].Inspect()
			}
			if event != "" {
				fmt.Fprintf(sse.writer, "event: %s\n", event)
			}
			fmt.Fprintf(sse.writer, "data: %s\n\n", data)
			sse.flusher.Flush()
			return object.NULL
		}}
	case "close":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			return object.NULL
		}}
	default:
		return nil
	}
}

// ── Helpers ────────────────────────────────────────────────────────

// SPLObjectToGo converts an SPL Object to a Go value for JSON marshaling.
func SPLObjectToGo(obj object.Object) any {
	switch v := obj.(type) {
	case *object.Integer:
		return v.Value
	case *object.Float:
		return v.Value
	case *object.String:
		return v.Value
	case *object.Boolean:
		return v.Value
	case *object.Null:
		return nil
	case *object.Array:
		result := make([]any, len(v.Elements))
		for i, el := range v.Elements {
			result[i] = SPLObjectToGo(el)
		}
		return result
	case *object.Hash:
		result := make(map[string]any)
		for _, pair := range v.Pairs {
			key := pair.Key.Inspect()
			result[key] = SPLObjectToGo(pair.Value)
		}
		return result
	default:
		return obj.Inspect()
	}
}

// GoToSPLObject converts a Go value (from JSON) to an SPL Object.
func GoToSPLObject(val any) object.Object {
	switch v := val.(type) {
	case nil:
		return object.NULL
	case bool:
		if v {
			return object.TRUE
		}
		return object.FALSE
	case float64:
		if v == float64(int64(v)) {
			return &object.Integer{Value: int64(v)}
		}
		return &object.Float{Value: v}
	case string:
		return &object.String{Value: v}
	case []any:
		elems := make([]object.Object, len(v))
		for i, el := range v {
			elems[i] = GoToSPLObject(el)
		}
		return &object.Array{Elements: elems}
	case map[string]any:
		h := &object.Hash{Pairs: make(map[object.HashKey]object.HashPair)}
		for key, val := range v {
			k := &object.String{Value: key}
			h.Pairs[k.HashKey()] = object.HashPair{Key: k, Value: GoToSPLObject(val)}
		}
		return h
	default:
		return &object.String{Value: fmt.Sprintf("%v", v)}
	}
}

// GetServerProperty returns a property or method of the server object.
func GetServerProperty(srv *SPLServer, name string) object.Object {
	switch name {
	case "addr":
		return &object.String{Value: srv.addr}
	case "running":
		srv.mu.RLock()
		defer srv.mu.RUnlock()
		if srv.running {
			return object.TRUE
		}
		return object.FALSE
	case "routes":
		srv.mu.RLock()
		defer srv.mu.RUnlock()
		elems := make([]object.Object, len(srv.routes))
		for i, r := range srv.routes {
			h := &object.Hash{Pairs: make(map[object.HashKey]object.HashPair)}
			mk := &object.String{Value: "method"}
			h.Pairs[mk.HashKey()] = object.HashPair{Key: mk, Value: &object.String{Value: r.Method}}
			pk := &object.String{Value: "pattern"}
			h.Pairs[pk.HashKey()] = object.HashPair{Key: pk, Value: &object.String{Value: r.Pattern}}
			elems[i] = h
		}
		return &object.Array{Elements: elems}
	default:
		return nil
	}
}

// ── Builtins ───────────────────────────────────────────────────────

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{
		"server":       {Fn: builtinServer},
		"route":        {Fn: builtinRoute},
		"middleware":   {Fn: builtinMiddleware},
		"static":       {Fn: builtinStatic},
		"template_dir": {Fn: builtinTemplateDir},
		"web_app":      {Fn: builtinWebApp},
		"route_group":  {Fn: builtinRouteGroup},
		"listen":       {FnWithEnv: builtinListen},
		"listen_async": {FnWithEnv: builtinListenAsync},
		"shutdown":     {Fn: builtinShutdown},
	})

	// Register dot expression hook for server types
	prev := eval.DotExpressionHook
	eval.DotExpressionHook = func(left object.Object, name string) object.Object {
		switch obj := left.(type) {
		case *SPLServer:
			return GetServerProperty(obj, name)
		case *SPLRequest:
			return GetRequestProperty(obj, name)
		case *SPLResponse:
			return GetResponseProperty(obj, name)
		case *SSEWriter:
			return GetSSEWriterProperty(obj, name)
		}
		if prev != nil {
			return prev(left, name)
		}
		return nil
	}
}

func builtinServer(args ...object.Object) object.Object {
	if err := security.CheckCapabilityAllowed(security.CapabilityServer); err != nil {
		return object.NewError("%s", err)
	}
	srv := &SPLServer{
		routes:      make([]RouteHandler, 0),
		middlewares: make([]MiddlewareEntry, 0),
		staticDirs:  make(map[string]string),
		shutdownCh:  make(chan struct{}),
	}
	if len(args) >= 1 {
		switch v := args[0].(type) {
		case *object.Integer:
			srv.addr = fmt.Sprintf(":%d", v.Value)
		case *object.String:
			srv.addr = v.Value
		}
	}
	return srv
}

func builtinRoute(args ...object.Object) object.Object {
	if len(args) < 3 {
		return object.NewError("route() requires at least (server, pattern, handler)")
	}
	srv, ok := args[0].(*SPLServer)
	if !ok {
		return object.NewError("route() first argument must be a server, got %s", args[0].Type())
	}
	var method, pattern string
	var handler object.Object

	if len(args) == 3 {
		p, ok := args[1].(*object.String)
		if !ok {
			return object.NewError("route() pattern must be a string")
		}
		pattern = p.Value
		method = "ANY"
		handler = args[2]
	} else {
		m, ok := args[1].(*object.String)
		if !ok {
			return object.NewError("route() method must be a string")
		}
		method = m.Value
		p, ok := args[2].(*object.String)
		if !ok {
			return object.NewError("route() pattern must be a string")
		}
		pattern = p.Value
		handler = args[3]
	}

	if _, ok := handler.(*object.Function); !ok {
		if _, ok := handler.(*object.Builtin); !ok {
			return object.NewError("route() handler must be a function")
		}
	}

	srv.addRoute(method, pattern, handler)
	return srv
}

func builtinMiddleware(args ...object.Object) object.Object {
	if len(args) < 2 {
		return object.NewError("middleware() requires at least (server, handler)")
	}
	srv, ok := args[0].(*SPLServer)
	if !ok {
		return object.NewError("middleware() first argument must be a server, got %s", args[0].Type())
	}

	var path string
	var handler object.Object

	if len(args) == 2 {
		path = "/"
		handler = args[1]
	} else {
		p, ok := args[1].(*object.String)
		if !ok {
			return object.NewError("middleware() path must be a string")
		}
		path = p.Value
		handler = args[2]
	}

	srv.addMiddleware(path, handler)
	return srv
}

func builtinStatic(args ...object.Object) object.Object {
	if len(args) < 3 {
		return object.NewError("static() requires (server, prefix, directory)")
	}
	srv, ok := args[0].(*SPLServer)
	if !ok {
		return object.NewError("static() first argument must be a server")
	}
	prefix, ok := args[1].(*object.String)
	if !ok {
		return object.NewError("static() prefix must be a string")
	}
	dir, ok := args[2].(*object.String)
	if !ok {
		return object.NewError("static() directory must be a string")
	}
	srv.addStaticDir(prefix.Value, dir.Value)
	return srv
}

func builtinTemplateDir(args ...object.Object) object.Object {
	if len(args) < 2 {
		return object.NewError("template_dir() requires (server, directory)")
	}
	srv, ok := args[0].(*SPLServer)
	if !ok {
		return object.NewError("template_dir() first argument must be a server")
	}
	dir, ok := args[1].(*object.String)
	if !ok {
		return object.NewError("template_dir() directory must be a string")
	}
	srv.mu.Lock()
	srv.templateDir = dir.Value
	srv.mu.Unlock()
	return srv
}

func builtinWebApp(args ...object.Object) object.Object {
	srvObj := builtinServer()
	srv, ok := srvObj.(*SPLServer)
	if !ok {
		return srvObj
	}
	if len(args) >= 1 {
		if dir, ok := args[0].(*object.String); ok {
			srv.templateDir = dir.Value
		}
	}
	return srv
}

func builtinRouteGroup(args ...object.Object) object.Object {
	if len(args) < 4 {
		return object.NewError("route_group() requires at least (server, prefix, pattern, handler)")
	}
	srv, ok := args[0].(*SPLServer)
	if !ok {
		return object.NewError("route_group() first argument must be a server")
	}
	prefix, ok := args[1].(*object.String)
	if !ok {
		return object.NewError("route_group() prefix must be a string")
	}
	joinPath := func(base, part string) string {
		base = strings.TrimSuffix(base, "/")
		part = strings.TrimPrefix(part, "/")
		if base == "" {
			return "/" + part
		}
		if part == "" {
			return base
		}
		return base + "/" + part
	}
	if len(args) == 4 {
		pattern, ok := args[2].(*object.String)
		if !ok {
			return object.NewError("route_group() pattern must be a string")
		}
		return builtinRoute(srv, &object.String{Value: joinPath(prefix.Value, pattern.Value)}, args[3])
	}
	method, ok := args[2].(*object.String)
	if !ok {
		return object.NewError("route_group() method must be a string")
	}
	pattern, ok := args[3].(*object.String)
	if !ok {
		return object.NewError("route_group() pattern must be a string")
	}
	return builtinRoute(srv, method, &object.String{Value: joinPath(prefix.Value, pattern.Value)}, args[4])
}

func builtinListen(env *object.Environment, args ...object.Object) object.Object {
	if err := security.CheckCapabilityAllowed(security.CapabilityServer); err != nil {
		return object.NewError("%s", err)
	}
	if len(args) < 1 {
		return object.NewError("listen() requires a server argument")
	}
	srv, ok := args[0].(*SPLServer)
	if !ok {
		return object.NewError("listen() first argument must be a server")
	}

	if len(args) >= 2 {
		switch v := args[1].(type) {
		case *object.Integer:
			srv.addr = fmt.Sprintf(":%d", v.Value)
		case *object.String:
			srv.addr = v.Value
		}
	}

	if srv.addr == "" {
		srv.addr = ":8080"
	}

	srv.env = env
	srv.server = &http.Server{
		Addr:    srv.addr,
		Handler: srv,
	}

	srv.mu.Lock()
	srv.running = true
	srv.mu.Unlock()

	fmt.Printf("SPL server listening on %s\n", srv.addr)
	err := srv.server.ListenAndServe()
	srv.mu.Lock()
	srv.running = false
	srv.mu.Unlock()

	if err != nil && err != http.ErrServerClosed {
		return object.NewError("server error: %s", err)
	}
	return object.NULL
}

func builtinListenAsync(env *object.Environment, args ...object.Object) object.Object {
	if err := security.CheckCapabilityAllowed(security.CapabilityServer); err != nil {
		return object.NewError("%s", err)
	}
	if len(args) < 1 {
		return object.NewError("listen_async() requires a server argument")
	}
	srv, ok := args[0].(*SPLServer)
	if !ok {
		return object.NewError("listen_async() first argument must be a server")
	}

	if len(args) >= 2 {
		switch v := args[1].(type) {
		case *object.Integer:
			srv.addr = fmt.Sprintf(":%d", v.Value)
		case *object.String:
			srv.addr = v.Value
		}
	}

	if srv.addr == "" {
		srv.addr = ":8080"
	}

	srv.env = env
	srv.server = &http.Server{
		Addr:    srv.addr,
		Handler: srv,
	}

	srv.mu.Lock()
	srv.running = true
	srv.mu.Unlock()

	go func() {
		fmt.Printf("SPL server listening on %s\n", srv.addr)
		err := srv.server.ListenAndServe()
		srv.mu.Lock()
		srv.running = false
		srv.mu.Unlock()
		if err != nil && err != http.ErrServerClosed {
			fmt.Printf("server error: %s\n", err)
		}
	}()

	if env != nil {
		env.RegisterCleanup(func() {
			if srv.server != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_ = srv.server.Shutdown(ctx)
			}
		})
	}
	time.Sleep(50 * time.Millisecond)
	return srv
}

func builtinShutdown(args ...object.Object) object.Object {
	if len(args) < 1 {
		return object.NewError("shutdown() requires a server argument")
	}
	srv, ok := args[0].(*SPLServer)
	if !ok {
		return object.NewError("shutdown() first argument must be a server")
	}
	if srv.server == nil {
		return object.NULL
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.server.Shutdown(ctx); err != nil {
		return object.NewError("shutdown error: %s", err)
	}
	return object.NULL
}
