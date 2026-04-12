package interpreter

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ── Object types ───────────────────────────────────────────────────

const (
	SERVER_OBJ     ObjectType = 100
	REQUEST_OBJ    ObjectType = 101
	RESPONSE_OBJ   ObjectType = 102
	SSE_WRITER_OBJ ObjectType = 103
)

// ── SPLServer ──────────────────────────────────────────────────────

type RouteHandler struct {
	Method  string
	Pattern string
	Handler Object // Function or Builtin
}

type MiddlewareEntry struct {
	Path    string
	Handler Object
}

type SPLServer struct {
	mu          sync.RWMutex
	routes      []RouteHandler
	middlewares []MiddlewareEntry
	staticDirs  map[string]string // url prefix -> filesystem path
	templateDir string
	template    TemplateRuntime
	env         *Environment
	server      *http.Server
	addr        string
	running     bool
	shutdownCh  chan struct{}
}

func (s *SPLServer) Type() ObjectType { return SERVER_OBJ }
func (s *SPLServer) Inspect() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.running {
		return fmt.Sprintf("<server %s running>", s.addr)
	}
	return fmt.Sprintf("<server %d routes>", len(s.routes))
}

func (s *SPLServer) addRoute(method, pattern string, handler Object) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes = append(s.routes, RouteHandler{
		Method:  strings.ToUpper(method),
		Pattern: pattern,
		Handler: handler,
	})
}

func (s *SPLServer) addMiddleware(path string, handler Object) {
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
		s.template = newTemplateRuntime(baseDir)
	}
	return s.template
}

// findRoute finds the matching route for a request.
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

// matchRoute matches a pattern like "/users/:id/posts/:postId" against a path.
func matchRoute(pattern, path string) (map[string]string, bool) {
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	pathParts := strings.Split(strings.Trim(path, "/"), "/")

	if len(patternParts) != len(pathParts) {
		// Check for wildcard
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

// ServeHTTP implements http.Handler.
func (s *SPLServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check static dirs first
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

	// Build middleware chain
	s.mu.RLock()
	var applicableMiddlewares []MiddlewareEntry
	for _, m := range s.middlewares {
		if m.Path == "" || m.Path == "/" || strings.HasPrefix(r.URL.Path, m.Path) {
			applicableMiddlewares = append(applicableMiddlewares, m)
		}
	}
	s.mu.RUnlock()

	// Execute middleware chain, then handler
	s.executeChain(applicableMiddlewares, 0, route.Handler, req, res)
}

func (s *SPLServer) executeChain(middlewares []MiddlewareEntry, idx int, finalHandler Object, req *SPLRequest, res *SPLResponse) {
	if idx >= len(middlewares) {
		// Call final handler
		callSPLHandler(finalHandler, req, res, s.env)
		return
	}

	mw := middlewares[idx]
	nextCalled := false
	nextFn := &Builtin{
		Fn: func(args ...Object) Object {
			if nextCalled {
				return NULL
			}
			nextCalled = true
			s.executeChain(middlewares, idx+1, finalHandler, req, res)
			return NULL
		},
	}

	// Call middleware with (req, res, next)
	callSPLMiddleware(mw.Handler, req, res, nextFn, s.env)
}

func callSPLHandler(handler Object, req *SPLRequest, res *SPLResponse, env *Environment) {
	args := []Object{req, res}
	switch fn := handler.(type) {
	case *Function:
		extEnv := extendFunctionEnv(fn, args, env, nil)
		Eval(fn.Body, extEnv)
	case *Builtin:
		if fn.FnWithEnv != nil {
			fn.FnWithEnv(env, args...)
		} else {
			fn.Fn(args...)
		}
	}
}

func callSPLMiddleware(handler Object, req *SPLRequest, res *SPLResponse, next *Builtin, env *Environment) {
	args := []Object{req, res, next}
	switch fn := handler.(type) {
	case *Function:
		extEnv := extendFunctionEnv(fn, args, env, nil)
		Eval(fn.Body, extEnv)
	case *Builtin:
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

func (r *SPLRequest) Type() ObjectType { return REQUEST_OBJ }
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
func GetRequestProperty(req *SPLRequest, name string) Object {
	switch name {
	case "method":
		return &String{Value: req.Method}
	case "path":
		return &String{Value: req.Path}
	case "body":
		return &String{Value: req.Body}
	case "params":
		h := &Hash{Pairs: make(map[HashKey]HashPair)}
		for k, v := range req.Params {
			key := &String{Value: k}
			h.Pairs[key.HashKey()] = HashPair{Key: key, Value: &String{Value: v}}
		}
		return h
	case "query":
		h := &Hash{Pairs: make(map[HashKey]HashPair)}
		for k, vals := range req.Query {
			key := &String{Value: k}
			if len(vals) == 1 {
				h.Pairs[key.HashKey()] = HashPair{Key: key, Value: &String{Value: vals[0]}}
			} else {
				elems := make([]Object, len(vals))
				for i, v := range vals {
					elems[i] = &String{Value: v}
				}
				h.Pairs[key.HashKey()] = HashPair{Key: key, Value: &Array{Elements: elems}}
			}
		}
		return h
	case "headers":
		h := &Hash{Pairs: make(map[HashKey]HashPair)}
		for k, vals := range req.Headers {
			key := &String{Value: k}
			h.Pairs[key.HashKey()] = HashPair{Key: key, Value: &String{Value: strings.Join(vals, ", ")}}
		}
		return h
	case "json":
		return &Builtin{Fn: func(args ...Object) Object {
			if req.Body == "" {
				return NULL
			}
			var data any
			if err := json.Unmarshal([]byte(req.Body), &data); err != nil {
				return NULL
			}
			return goToSPLObject(data)
		}}
	case "param":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("param() requires a parameter name")
			}
			key, ok := args[0].(*String)
			if !ok {
				return newError("param() argument must be a string")
			}
			if v, found := req.Params[key.Value]; found {
				return &String{Value: v}
			}
			return NULL
		}}
	case "get_header":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("get_header() requires a header name")
			}
			key, ok := args[0].(*String)
			if !ok {
				return newError("get_header() argument must be a string")
			}
			v := req.Headers.Get(key.Value)
			if v == "" {
				return NULL
			}
			return &String{Value: v}
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

func (r *SPLResponse) Type() ObjectType { return RESPONSE_OBJ }
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
func GetResponseProperty(res *SPLResponse, name string) Object {
	switch name {
	case "status":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("status() requires a status code")
			}
			code, ok := args[0].(*Integer)
			if !ok {
				return newError("status() argument must be an integer")
			}
			res.StatusCode = int(code.Value)
			return res
		}}
	case "header":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 2 {
				return newError("header() requires name and value")
			}
			key, ok := args[0].(*String)
			if !ok {
				return newError("header() name must be a string")
			}
			val, ok := args[1].(*String)
			if !ok {
				return newError("header() value must be a string")
			}
			res.Writer.Header().Set(key.Value, val.Value)
			return res
		}}
	case "json":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("json() requires data")
			}
			res.Writer.Header().Set("Content-Type", "application/json")
			res.Writer.WriteHeader(res.StatusCode)
			res.written = true
			goVal := splObjectToGo(args[0])
			data, err := json.Marshal(goVal)
			if err != nil {
				return newError("json marshal error: %s", err)
			}
			res.Writer.Write(data)
			return NULL
		}}
	case "text":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("text() requires data")
			}
			str, ok := args[0].(*String)
			if !ok {
				return newError("text() argument must be a string")
			}
			res.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
			res.Writer.WriteHeader(res.StatusCode)
			res.written = true
			res.Writer.Write([]byte(str.Value))
			return NULL
		}}
	case "html":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("html() requires data")
			}
			str, ok := args[0].(*String)
			if !ok {
				return newError("html() argument must be a string")
			}
			res.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			res.Writer.WriteHeader(res.StatusCode)
			res.written = true
			res.Writer.Write([]byte(str.Value))
			return NULL
		}}
	case "send":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("send() requires data")
			}
			if !res.written {
				res.Writer.WriteHeader(res.StatusCode)
				res.written = true
			}
			data := args[0].Inspect()
			res.Writer.Write([]byte(data))
			return NULL
		}}
	case "redirect":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("redirect() requires a URL")
			}
			url, ok := args[0].(*String)
			if !ok {
				return newError("redirect() argument must be a string")
			}
			code := 302
			if len(args) >= 2 {
				if c, ok := args[1].(*Integer); ok {
					code = int(c.Value)
				}
			}
			http.Redirect(res.Writer, nil, url.Value, code)
			res.written = true
			return NULL
		}}
	case "sse":
		return &Builtin{Fn: func(args ...Object) Object {
			flusher, ok := res.Writer.(http.Flusher)
			if !ok {
				return newError("SSE not supported by this response writer")
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
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("file() requires a file path")
			}
			path, ok := args[0].(*String)
			if !ok {
				return newError("file() argument must be a string")
			}
			data, err := os.ReadFile(path.Value)
			if err != nil {
				return newError("file read error: %s", err)
			}
			ct := mime.TypeByExtension(filepath.Ext(path.Value))
			if ct == "" {
				ct = "application/octet-stream"
			}
			res.Writer.Header().Set("Content-Type", ct)
			res.Writer.WriteHeader(res.StatusCode)
			res.written = true
			res.Writer.Write(data)
			return NULL
		}}
	case "render":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("render() requires a template path")
			}
			if res.Server == nil {
				return newError("render() server context unavailable")
			}
			path, ok := args[0].(*String)
			if !ok {
				return newError("render() template path must be a string")
			}
			data := objectArgToMap(args, 1)
			engine := res.Server.templateRuntime()
			if engine == nil {
				return newError("template runtime unavailable")
			}
			htmlStr, err := engine.RenderFile(path.Value, data)
			if err != nil {
				return newError("render error: %s", err)
			}
			res.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			res.Writer.WriteHeader(res.StatusCode)
			res.written = true
			_, _ = res.Writer.Write([]byte(htmlStr))
			return NULL
		}}
	case "render_ssr":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("render_ssr() requires a template path")
			}
			if res.Server == nil {
				return newError("render_ssr() server context unavailable")
			}
			path, ok := args[0].(*String)
			if !ok {
				return newError("render_ssr() template path must be a string")
			}
			data := objectArgToMap(args, 1)
			engine := res.Server.templateRuntime()
			if engine == nil {
				return newError("template runtime unavailable")
			}
			htmlStr, err := engine.RenderSSRFile(path.Value, data)
			if err != nil {
				return newError("render_ssr error: %s", err)
			}
			res.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			res.Writer.WriteHeader(res.StatusCode)
			res.written = true
			_, _ = res.Writer.Write([]byte(htmlStr))
			return NULL
		}}
	case "stream":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("stream() requires a template path")
			}
			if res.Server == nil {
				return newError("stream() server context unavailable")
			}
			path, ok := args[0].(*String)
			if !ok {
				return newError("stream() template path must be a string")
			}
			data := objectArgToMap(args, 1)
			engine := res.Server.templateRuntime()
			if engine == nil {
				return newError("template runtime unavailable")
			}
			res.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			if flusher, ok := res.Writer.(http.Flusher); ok {
				res.Writer.WriteHeader(res.StatusCode)
				res.written = true
				flusher.Flush()
			}
			if err := engine.RenderStreamFile(res.Writer, path.Value, data); err != nil {
				return newError("stream error: %s", err)
			}
			return NULL
		}}
	default:
		return nil
	}
}

func objectArgToMap(args []Object, idx int) map[string]any {
	if len(args) <= idx {
		return nil
	}
	h, ok := args[idx].(*Hash)
	if !ok {
		return nil
	}
	result := make(map[string]any)
	for _, pair := range h.Pairs {
		result[pair.Key.Inspect()] = splObjectToGo(pair.Value)
	}
	return result
}

// ── SSEWriter ──────────────────────────────────────────────────────

type SSEWriter struct {
	writer  http.ResponseWriter
	flusher http.Flusher
}

func (s *SSEWriter) Type() ObjectType { return SSE_WRITER_OBJ }
func (s *SSEWriter) Inspect() string  { return "<sse writer>" }

func GetSSEWriterProperty(sse *SSEWriter, name string) Object {
	switch name {
	case "send":
		return &Builtin{Fn: func(args ...Object) Object {
			if len(args) < 1 {
				return newError("sse.send() requires data")
			}
			var event, data string
			if len(args) >= 2 {
				if e, ok := args[0].(*String); ok {
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
			return NULL
		}}
	case "close":
		return &Builtin{Fn: func(args ...Object) Object {
			// SSE connection close is handled by client disconnect
			return NULL
		}}
	default:
		return nil
	}
}

// ── Helpers ────────────────────────────────────────────────────────

// splObjectToGo converts an SPL Object to a Go value for JSON marshaling.
func splObjectToGo(obj Object) any {
	switch v := obj.(type) {
	case *Integer:
		return v.Value
	case *Float:
		return v.Value
	case *String:
		return v.Value
	case *Boolean:
		return v.Value
	case *Null:
		return nil
	case *Array:
		result := make([]any, len(v.Elements))
		for i, el := range v.Elements {
			result[i] = splObjectToGo(el)
		}
		return result
	case *Hash:
		result := make(map[string]any)
		for _, pair := range v.Pairs {
			key := pair.Key.Inspect()
			result[key] = splObjectToGo(pair.Value)
		}
		return result
	default:
		return obj.Inspect()
	}
}

// goToSPLObject converts a Go value (from JSON) to an SPL Object.
func goToSPLObject(val any) Object {
	switch v := val.(type) {
	case nil:
		return NULL
	case bool:
		if v {
			return TRUE
		}
		return FALSE
	case float64:
		if v == float64(int64(v)) {
			return &Integer{Value: int64(v)}
		}
		return &Float{Value: v}
	case string:
		return &String{Value: v}
	case []any:
		elems := make([]Object, len(v))
		for i, el := range v {
			elems[i] = goToSPLObject(el)
		}
		return &Array{Elements: elems}
	case map[string]any:
		h := &Hash{Pairs: make(map[HashKey]HashPair)}
		for key, val := range v {
			k := &String{Value: key}
			h.Pairs[k.HashKey()] = HashPair{Key: k, Value: goToSPLObject(val)}
		}
		return h
	default:
		return &String{Value: fmt.Sprintf("%v", v)}
	}
}

// GetServerProperty returns a property or method of the server object.
func GetServerProperty(srv *SPLServer, name string) Object {
	switch name {
	case "addr":
		return &String{Value: srv.addr}
	case "running":
		srv.mu.RLock()
		defer srv.mu.RUnlock()
		if srv.running {
			return TRUE
		}
		return FALSE
	case "routes":
		srv.mu.RLock()
		defer srv.mu.RUnlock()
		elems := make([]Object, len(srv.routes))
		for i, r := range srv.routes {
			h := &Hash{Pairs: make(map[HashKey]HashPair)}
			mk := &String{Value: "method"}
			h.Pairs[mk.HashKey()] = HashPair{Key: mk, Value: &String{Value: r.Method}}
			pk := &String{Value: "pattern"}
			h.Pairs[pk.HashKey()] = HashPair{Key: pk, Value: &String{Value: r.Pattern}}
			elems[i] = h
		}
		return &Array{Elements: elems}
	default:
		return nil
	}
}
