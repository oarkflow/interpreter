// Package server provides the SPL http server/router builtins, backed by
// github.com/oarkflow/fh instead of net/http. It supports global
// middleware, path-scoped middleware, per-route middleware chains, and
// nested route groups with group-level middleware.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oarkflow/fh"
	"github.com/oarkflow/fh/mw/basicauth"
	"github.com/oarkflow/fh/mw/bodylimit"
	"github.com/oarkflow/fh/mw/compress"
	"github.com/oarkflow/fh/mw/cors"
	"github.com/oarkflow/fh/mw/csrf"
	"github.com/oarkflow/fh/mw/etag"
	"github.com/oarkflow/fh/mw/logger"
	"github.com/oarkflow/fh/mw/ratelimiter"
	"github.com/oarkflow/fh/mw/realip"
	"github.com/oarkflow/fh/mw/recover"
	"github.com/oarkflow/fh/mw/requestid"
	"github.com/oarkflow/fh/mw/timeout"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/security"
	"github.com/oarkflow/interpreter/pkg/template"
)

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

// ── SPLServer ──────────────────────────────────────────────────────

// routeHandler is a single registered route: a method, a path pattern, and
// an ordered chain of handlers where every entry but the last is a
// middleware (req, res, next) and the last is the terminal handler
// (req, res).
type routeHandler struct {
	Method   string
	Pattern  string
	Handlers []object.Object
}

// middlewareEntry is a server-level middleware, optionally scoped to a path
// prefix (Path == "" or "/" means global). Exactly one of Handler (an SPL
// function) or Native (a ready-made fh.HandlerFunc, e.g. from fh/mw/*) is
// set.
type middlewareEntry struct {
	Path    string
	Handler object.Object
	Native  fh.HandlerFunc
}

type SPLServer struct {
	mu          sync.RWMutex
	routes      []routeHandler
	middlewares []middlewareEntry
	staticDirs  map[string]string // url prefix -> filesystem path
	templateDir string
	template    TemplateRuntime
	env         *object.Environment
	app         *fh.App
	addr        string
	running     bool
	shutdownCh  chan struct{}
	hookOnce    sync.Once
}

func registerServerShutdownHook(srv *SPLServer) {
	srv.hookOnce.Do(func() {
		object.RegisterShutdownHook(func(ctx context.Context) {
			srv.mu.RLock()
			app := srv.app
			running := srv.running
			srv.mu.RUnlock()
			if !running || app == nil {
				return
			}
			done := make(chan struct{})
			go func() {
				_ = app.Shutdown()
				close(done)
			}()
			select {
			case <-done:
			case <-ctx.Done():
			}
		})
	})
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

func (s *SPLServer) addRoute(method, pattern string, handlers []object.Object) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes = append(s.routes, routeHandler{
		Method:   strings.ToUpper(method),
		Pattern:  pattern,
		Handlers: handlers,
	})
}

func (s *SPLServer) addMiddleware(path string, handler object.Object) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.middlewares = append(s.middlewares, middlewareEntry{Path: path, Handler: handler})
}

func (s *SPLServer) addNativeMiddleware(handler fh.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.middlewares = append(s.middlewares, middlewareEntry{Native: handler})
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
		s.template = template.NewTemplateRuntime(baseDir)
	}
	return s.template
}

// ── SPLGroup ───────────────────────────────────────────────────────

// SPLGroup is a route group: a shared path prefix plus an ordered list of
// group-level middlewares prepended to every route registered through it.
// Nested groups accumulate their parent's middlewares.
type SPLGroup struct {
	srv         *SPLServer
	prefix      string
	middlewares []object.Object
}

func (g *SPLGroup) Type() object.ObjectType { return object.ROUTE_GROUP_OBJ }
func (g *SPLGroup) Inspect() string         { return fmt.Sprintf("<route_group %s>", g.prefix) }

func joinPath(base, part string) string {
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

func (g *SPLGroup) route(method, pattern string, handlers []object.Object) *SPLGroup {
	all := make([]object.Object, 0, len(g.middlewares)+len(handlers))
	all = append(all, g.middlewares...)
	all = append(all, handlers...)
	g.srv.addRoute(method, joinPath(g.prefix, pattern), all)
	return g
}

func (g *SPLGroup) use(mw object.Object) *SPLGroup {
	g.middlewares = append(g.middlewares, mw)
	return g
}

func (g *SPLGroup) subGroup(prefix string, mw []object.Object) *SPLGroup {
	merged := make([]object.Object, 0, len(g.middlewares)+len(mw))
	merged = append(merged, g.middlewares...)
	merged = append(merged, mw...)
	return &SPLGroup{srv: g.srv, prefix: joinPath(g.prefix, prefix), middlewares: merged}
}

// ── request/response param helpers ──────────────────────────────────

func paramNames(pattern string) []string {
	var names []string
	for _, seg := range strings.Split(strings.Trim(pattern, "/"), "/") {
		if strings.HasPrefix(seg, ":") {
			names = append(names, seg[1:])
		} else if strings.HasPrefix(seg, "*") && len(seg) > 1 {
			names = append(names, seg[1:])
		}
	}
	return names
}

// ── request/response bridging to fh.Ctx ─────────────────────────────

type reqResPair struct {
	req *SPLRequest
	res *SPLResponse
}

const reqResLocalsKey = "__spl_reqres"

func (s *SPLServer) reqRes(ctx fh.Ctx, names []string) (*SPLRequest, *SPLResponse) {
	if v := ctx.Locals(reqResLocalsKey); v != nil {
		if pair, ok := v.(*reqResPair); ok {
			return pair.req, pair.res
		}
	}
	req := newSPLRequest(ctx, names)
	res := newSPLResponse(ctx, s)
	ctx.Locals(reqResLocalsKey, &reqResPair{req: req, res: res})
	return req, res
}

// buildChain turns an ordered list of SPL handler objects (middlewares
// followed by a terminal handler) into fh.HandlerFuncs that thread a single
// req/res pair through fh's own Next()-based chaining.
func (s *SPLServer) buildChain(names []string, handlers []object.Object) []fh.HandlerFunc {
	out := make([]fh.HandlerFunc, len(handlers))
	last := len(handlers) - 1
	for i, h := range handlers {
		isLast := i == last
		out[i] = func(ctx fh.Ctx) error {
			req, res := s.reqRes(ctx, names)
			if isLast {
				callSPLHandler(h, req, res, s.env)
				return nil
			}
			nextFn := &object.Builtin{Fn: func(args ...object.Object) object.Object {
				_ = ctx.Next()
				return object.NULL
			}}
			callSPLMiddleware(h, req, res, nextFn, s.env)
			return nil
		}
	}
	return out
}

// buildPathScopedMiddleware wraps a path-scoped SPL middleware as a global
// fh middleware that only runs for requests under the given prefix; other
// requests fall through to the rest of the chain untouched. allParamNames is
// the union of every registered route's param names in the app: since
// global/path-scoped middleware runs before route-specific handlers (and,
// via ctx.Locals caching in reqRes, whichever handler runs first decides
// the request wrapper's known param names for the whole chain), it doesn't
// know the eventual route's own pattern - passing the full superset instead
// of nil keeps req.params populated correctly regardless of registration
// order (unmatched names are simply skipped, see newSPLRequest).
func (s *SPLServer) buildPathScopedMiddleware(path string, handler object.Object, allParamNames []string) fh.HandlerFunc {
	global := path == "" || path == "/"
	return func(ctx fh.Ctx) error {
		if !global && !strings.HasPrefix(ctx.Path(), path) {
			return ctx.Next()
		}
		req, res := s.reqRes(ctx, allParamNames)
		nextFn := &object.Builtin{Fn: func(args ...object.Object) object.Object {
			_ = ctx.Next()
			return object.NULL
		}}
		callSPLMiddleware(handler, req, res, nextFn, s.env)
		return nil
	}
}

// build materializes all recorded routes/middlewares/static dirs into a
// real *fh.App. Called once, right before Listen/ListenAsync.
func (s *SPLServer) build() *fh.App {
	app := fh.New()

	s.mu.RLock()
	staticDirs := s.staticDirs
	middlewares := append([]middlewareEntry(nil), s.middlewares...)
	routes := append([]routeHandler(nil), s.routes...)
	s.mu.RUnlock()

	// Static files are served natively by fh (ETag, Last-Modified, byte
	// ranges, directory index) and registered before user middleware so
	// they always take priority over anything else for their prefix.
	for prefix, dir := range staticDirs {
		// fh.Static computes its own trailing-slash redirect route; passing
		// a prefix that already ends in "/" (the SPL convention, e.g.
		// static(server, "/public/", dir)) produces an invalid empty path
		// segment, so normalize it away here.
		app.Static(strings.TrimSuffix(prefix, "/"), dir)
	}
	var allParamNames []string
	seenParamNames := map[string]struct{}{}
	for _, r := range routes {
		for _, name := range paramNames(r.Pattern) {
			if _, ok := seenParamNames[name]; !ok {
				seenParamNames[name] = struct{}{}
				allParamNames = append(allParamNames, name)
			}
		}
	}
	for _, mw := range middlewares {
		if mw.Native != nil {
			app.Use(mw.Native)
			continue
		}
		app.Use(s.buildPathScopedMiddleware(mw.Path, mw.Handler, allParamNames))
	}
	for _, r := range routes {
		names := paramNames(r.Pattern)
		chain := s.buildChain(names, r.Handlers)
		method := r.Method
		if method == "ANY" || method == "" {
			app.All(r.Pattern, chain...)
		} else {
			app.Add(method, r.Pattern, chain...)
		}
	}
	return app
}

// ── SPLRequest ─────────────────────────────────────────────────────

type SPLRequest struct {
	Method  string
	Path    string
	Params  map[string]string
	Query   map[string][]string
	Headers map[string][]string
	Body    string
	Ctx     fh.Ctx
}

func (r *SPLRequest) Type() object.ObjectType { return object.REQUEST_OBJ }
func (r *SPLRequest) Inspect() string {
	return fmt.Sprintf("<request %s %s>", r.Method, r.Path)
}

func newSPLRequest(ctx fh.Ctx, names []string) *SPLRequest {
	params := make(map[string]string, len(names))
	for _, name := range names {
		if v := ctx.Param(name); v != "" {
			params[name] = v
		}
	}
	query := make(map[string][]string)
	if u, err := url.Parse(ctx.OriginalURL()); err == nil && u.RawQuery != "" {
		if vals, err := url.ParseQuery(u.RawQuery); err == nil {
			query = vals
		}
	}
	return &SPLRequest{
		Method:  ctx.Method(),
		Path:    ctx.Path(),
		Params:  params,
		Query:   query,
		Headers: ctx.GetReqHeaders(),
		Body:    string(ctx.Body()),
		Ctx:     ctx,
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
			// Query fh's Ctx live rather than the req.Params snapshot: the
			// snapshot is built once (and cached across the whole
			// middleware+handler chain via ctx.Locals) from whichever
			// handler runs first, which for global/path-scoped middleware
			// is built with no route pattern in hand at all. fh's Ctx
			// already has the real matched params for this request
			// regardless of which handler asks, so this is always correct.
			if v := req.Ctx.Param(key.Value); v != "" {
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
			v := req.Ctx.Get(key.Value)
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
	Ctx        fh.Ctx
	Server     *SPLServer
	StatusCode int
	written    bool
}

func (r *SPLResponse) Type() object.ObjectType { return object.RESPONSE_OBJ }
func (r *SPLResponse) Inspect() string {
	return fmt.Sprintf("<response status=%d>", r.StatusCode)
}

func newSPLResponse(ctx fh.Ctx, srv *SPLServer) *SPLResponse {
	return &SPLResponse{Ctx: ctx, Server: srv, StatusCode: 200}
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
			res.Ctx.Status(res.StatusCode)
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
			res.Ctx.Set(key.Value, val.Value)
			return res
		}}
	case "json":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("json() requires data")
			}
			res.written = true
			goVal := SPLObjectToGo(args[0])
			if err := res.Ctx.JSON(goVal); err != nil {
				return object.NewError("json marshal error: %s", err)
			}
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
			res.Ctx.Type("text/plain; charset=utf-8")
			res.written = true
			if err := res.Ctx.SendString(str.Value); err != nil {
				return object.NewError("text error: %s", err)
			}
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
			res.Ctx.Type("text/html; charset=utf-8")
			res.written = true
			if err := res.Ctx.SendString(str.Value); err != nil {
				return object.NewError("html error: %s", err)
			}
			return object.NULL
		}}
	case "send":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("send() requires data")
			}
			res.written = true
			if err := res.Ctx.SendString(args[0].Inspect()); err != nil {
				return object.NewError("send error: %s", err)
			}
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
			res.written = true
			if err := res.Ctx.Redirect(url.Value, code); err != nil {
				return object.NewError("redirect error: %s", err)
			}
			return object.NULL
		}}
	case "sse":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			writer, err := newSSEWriter(res.Ctx)
			if err != nil {
				return object.NewError("%s", err)
			}
			res.written = true
			return writer
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
			res.Ctx.Type(ct)
			res.written = true
			if err := res.Ctx.Send(data); err != nil {
				return object.NewError("file send error: %s", err)
			}
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
			res.Ctx.Type("text/html; charset=utf-8")
			res.written = true
			if err := res.Ctx.SendString(htmlStr); err != nil {
				return object.NewError("render error: %s", err)
			}
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
			res.Ctx.Type("text/html; charset=utf-8")
			res.written = true
			if err := res.Ctx.SendString(htmlStr); err != nil {
				return object.NewError("render_ssr error: %s", err)
			}
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
			res.Ctx.Type("text/html; charset=utf-8")
			res.written = true
			err := res.Ctx.Stream(func(w *fh.StreamWriter) error {
				return engine.RenderStreamFile(w, path.Value, data)
			})
			if err != nil {
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

// SSEWriter hijacks the underlying connection so events can be pushed to
// the client at arbitrary times from SPL code, independent of fh's normal
// per-request response lifecycle.
type SSEWriter struct {
	conn   *fh.ResponseConn
	doneCh chan struct{}
	once   sync.Once
}

func newSSEWriter(ctx fh.Ctx) (*SSEWriter, error) {
	readyCh := make(chan *fh.ResponseConn, 1)
	errCh := make(chan error, 1)
	doneCh := make(chan struct{})
	go func() {
		err := ctx.Hijack(func(rc *fh.ResponseConn) error {
			rc.SetHeader([]byte("Content-Type"), []byte("text/event-stream"))
			rc.SetHeader([]byte("Cache-Control"), []byte("no-cache"))
			rc.SetHeader([]byte("Connection"), []byte("keep-alive"))
			rc.WriteHeader(200)
			readyCh <- rc
			<-doneCh
			return nil
		})
		if err != nil {
			select {
			case errCh <- err:
			default:
			}
		}
	}()
	select {
	case rc := <-readyCh:
		return &SSEWriter{conn: rc, doneCh: doneCh}, nil
	case err := <-errCh:
		return nil, fmt.Errorf("SSE not supported: %w", err)
	}
}

func (s *SSEWriter) Type() object.ObjectType { return object.SSE_WRITER_OBJ }
func (s *SSEWriter) Inspect() string         { return "<sse writer>" }

func (s *SSEWriter) close() {
	s.once.Do(func() { close(s.doneCh) })
}

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
			var b strings.Builder
			if event != "" {
				fmt.Fprintf(&b, "event: %s\n", event)
			}
			fmt.Fprintf(&b, "data: %s\n\n", data)
			if _, err := sse.conn.Write([]byte(b.String())); err != nil {
				return object.NewError("sse.send() write error: %s", err)
			}
			return object.NULL
		}}
	case "close":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			sse.close()
			return object.NULL
		}}
	default:
		return nil
	}
}

// ── SPLGroup property dispatch ──────────────────────────────────────

func splitHandlers(args []object.Object) (pattern string, handlers []object.Object, errObj object.Object) {
	if len(args) < 2 {
		return "", nil, object.NewError("requires at least (pattern, handler)")
	}
	p, ok := args[0].(*object.String)
	if !ok {
		return "", nil, object.NewError("pattern must be a string")
	}
	return p.Value, args[1:], nil
}

// GetGroupProperty returns a property or method of the route group object.
func GetGroupProperty(g *SPLGroup, name string) object.Object {
	methodFn := func(method string) *object.Builtin {
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			pattern, handlers, errObj := splitHandlers(args)
			if errObj != nil {
				return errObj
			}
			g.route(method, pattern, handlers)
			return g
		}}
	}
	switch name {
	case "get":
		return methodFn("GET")
	case "post":
		return methodFn("POST")
	case "put":
		return methodFn("PUT")
	case "delete":
		return methodFn("DELETE")
	case "patch":
		return methodFn("PATCH")
	case "head":
		return methodFn("HEAD")
	case "options":
		return methodFn("OPTIONS")
	case "any":
		return methodFn("ANY")
	case "route":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 3 {
				return object.NewError("route() requires (method, pattern, handler)")
			}
			m, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("route() method must be a string")
			}
			p, ok := args[1].(*object.String)
			if !ok {
				return object.NewError("route() pattern must be a string")
			}
			g.route(m.Value, p.Value, args[2:])
			return g
		}}
	case "use":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("use() requires a middleware function")
			}
			for _, mw := range args {
				g.use(mw)
			}
			return g
		}}
	case "group":
		return &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return object.NewError("group() requires a prefix")
			}
			prefix, ok := args[0].(*object.String)
			if !ok {
				return object.NewError("group() prefix must be a string")
			}
			return g.subGroup(prefix.Value, args[1:])
		}}
	case "prefix":
		return &object.String{Value: g.prefix}
	default:
		return nil
	}
}

// ── handler invocation ───────────────────────────────────────────────

func callSPLHandler(handler object.Object, req *SPLRequest, res *SPLResponse, env *object.Environment) {
	args := []object.Object{req, res}
	switch fn := handler.(type) {
	case *object.Function:
		if object.ExtendFunctionEnvFn != nil && object.EvalFn != nil {
			extEnv := object.ExtendFunctionEnvFn(fn, args, env)
			extEnv.RuntimeLimits = perRequestRuntimeLimits(env)
			reportHandlerError(req, object.EvalFn(fn.Body, extEnv))
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
			extEnv.RuntimeLimits = perRequestRuntimeLimits(env)
			reportHandlerError(req, object.EvalFn(fn.Body, extEnv))
		}
	case *object.Builtin:
		if fn.FnWithEnv != nil {
			fn.FnWithEnv(env, args...)
		} else {
			fn.Fn(args...)
		}
	}
}

func perRequestRuntimeLimits(env *object.Environment) *object.RuntimeLimits {
	if env == nil || env.RuntimeLimits == nil {
		return nil
	}
	return env.RuntimeLimits.CloneForConcurrentExecution()
}

func reportHandlerError(req *SPLRequest, result object.Object) {
	if !object.IsError(result) {
		return
	}
	method, path := "", ""
	if req != nil {
		method, path = req.Method, req.Path
	}
	fmt.Fprintf(os.Stderr, "spl server: %s %s: %s\n", method, path, result.Inspect())
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

// ── Native middleware (fh / fh/mw/*) ────────────────────────────────

func optHash(args []object.Object, idx int) *object.Hash {
	if len(args) <= idx {
		return nil
	}
	h, _ := args[idx].(*object.Hash)
	return h
}

func hashGet(h *object.Hash, key string) (object.Object, bool) {
	if h == nil {
		return nil, false
	}
	k := &object.String{Value: key}
	pair, ok := h.Pairs[k.HashKey()]
	if !ok {
		return nil, false
	}
	return pair.Value, true
}

func optString(h *object.Hash, key, def string) string {
	if v, ok := hashGet(h, key); ok {
		if s, ok := v.(*object.String); ok {
			return s.Value
		}
	}
	return def
}

func optInt(h *object.Hash, key string, def int) int {
	if v, ok := hashGet(h, key); ok {
		if i, ok := v.(*object.Integer); ok {
			return int(i.Value)
		}
	}
	return def
}

func optBool(h *object.Hash, key string, def bool) bool {
	if v, ok := hashGet(h, key); ok {
		if b, ok := v.(*object.Boolean); ok {
			return b.Value
		}
	}
	return def
}

func optSeconds(h *object.Hash, key string, def time.Duration) time.Duration {
	if v, ok := hashGet(h, key); ok {
		switch n := v.(type) {
		case *object.Integer:
			return time.Duration(n.Value) * time.Second
		case *object.Float:
			return time.Duration(n.Value * float64(time.Second))
		}
	}
	return def
}

func optStringSlice(h *object.Hash, key string) []string {
	v, ok := hashGet(h, key)
	if !ok {
		return nil
	}
	arr, ok := v.(*object.Array)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr.Elements))
	for _, el := range arr.Elements {
		if s, ok := el.(*object.String); ok {
			out = append(out, s.Value)
		}
	}
	return out
}

// nativeMiddlewareRegistry maps a name to a constructor that builds a ready
// fh.HandlerFunc from an SPL options hash (nil means "use defaults").
// This is the curated set backing native_middleware(); anything not listed
// here can still be reached by building a custom SPL middleware.
var nativeMiddlewareRegistry = map[string]func(opts *object.Hash) fh.HandlerFunc{
	"cors": func(opts *object.Hash) fh.HandlerFunc {
		cfg := cors.DefaultConfig
		if origins := optStringSlice(opts, "allow_origins"); origins != nil {
			cfg.AllowOrigins = origins
		}
		if methods := optStringSlice(opts, "allow_methods"); methods != nil {
			cfg.AllowMethods = methods
		}
		if headers := optStringSlice(opts, "allow_headers"); headers != nil {
			cfg.AllowHeaders = headers
		}
		if expose := optStringSlice(opts, "expose_headers"); expose != nil {
			cfg.ExposeHeaders = expose
		}
		cfg.AllowCredentials = optBool(opts, "allow_credentials", cfg.AllowCredentials)
		cfg.AllowPrivateNetwork = optBool(opts, "allow_private_network", cfg.AllowPrivateNetwork)
		cfg.MaxAge = optInt(opts, "max_age", cfg.MaxAge)
		return cors.New(cfg)
	},
	"logger": func(opts *object.Hash) fh.HandlerFunc {
		cfg := logger.Config{
			Format:     optString(opts, "format", ""),
			FormatName: optString(opts, "format_name", ""),
			TimeFormat: optString(opts, "time_format", ""),
		}
		return logger.New(cfg)
	},
	"recover": func(opts *object.Hash) fh.HandlerFunc {
		cfg := recover.Config{
			EnableStackTrace: optBool(opts, "enable_stack_trace", false),
			StackTraceLimit:  optInt(opts, "stack_trace_limit", 0),
		}
		return recover.New(cfg)
	},
	"requestid": func(opts *object.Hash) fh.HandlerFunc {
		cfg := requestid.Config{
			Header:            optString(opts, "header", requestid.DefaultHeader),
			LocalKey:          optString(opts, "local_key", requestid.DefaultLocalKey),
			TrustIncoming:     optBool(opts, "trust_incoming", false),
			MaxIncomingLength: optInt(opts, "max_incoming_length", 0),
		}
		return requestid.New(cfg)
	},
	"ratelimiter": func(opts *object.Hash) fh.HandlerFunc {
		cfg := ratelimiter.Config{
			Max:         optInt(opts, "max", 100),
			Window:      optSeconds(opts, "window_seconds", time.Minute),
			SendHeaders: optBool(opts, "send_headers", true),
		}
		return ratelimiter.New(cfg)
	},
	"basicauth": func(opts *object.Hash) fh.HandlerFunc {
		return basicauth.New(optString(opts, "username", ""), optString(opts, "password", ""))
	},
	"csrf": func(opts *object.Hash) fh.HandlerFunc {
		cfg := csrf.DefaultConfig
		cfg.CookieName = optString(opts, "cookie_name", cfg.CookieName)
		cfg.HeaderName = optString(opts, "header_name", cfg.HeaderName)
		cfg.CookiePath = optString(opts, "cookie_path", cfg.CookiePath)
		cfg.CookieDomain = optString(opts, "cookie_domain", cfg.CookieDomain)
		cfg.CookieSecure = optBool(opts, "cookie_secure", cfg.CookieSecure)
		cfg.AllowInsecureCookie = optBool(opts, "allow_insecure_cookie", cfg.AllowInsecureCookie)
		cfg.CookieMaxAge = optSeconds(opts, "cookie_max_age_seconds", cfg.CookieMaxAge)
		cfg.RequireOriginHeader = optBool(opts, "require_origin_header", cfg.RequireOriginHeader)
		cfg.AllowMissingOrigin = optBool(opts, "allow_missing_origin", cfg.AllowMissingOrigin)
		if origins := optStringSlice(opts, "trusted_origins"); origins != nil {
			cfg.TrustedOrigins = origins
		}
		return csrf.New(cfg)
	},
	"compress": func(opts *object.Hash) fh.HandlerFunc {
		cfg := compress.DefaultConfig
		cfg.Level = optInt(opts, "level", cfg.Level)
		cfg.MinSize = optInt(opts, "min_size", cfg.MinSize)
		cfg.BreachProtection = optBool(opts, "breach_protection", cfg.BreachProtection)
		if types := optStringSlice(opts, "compressible_types"); types != nil {
			cfg.CompressibleTypes = types
		}
		return compress.New(cfg)
	},
	"realip": func(opts *object.Hash) fh.HandlerFunc {
		cfg := realip.Config{
			LocalKey: optString(opts, "local_key", ""),
			TrustAll: optBool(opts, "trust_all", false),
		}
		if headers := optStringSlice(opts, "headers"); headers != nil {
			cfg.Headers = headers
		}
		return realip.New(cfg)
	},
	"bodylimit": func(opts *object.Hash) fh.HandlerFunc {
		return bodylimit.New(optInt(opts, "limit", 4<<20))
	},
	"timeout": func(opts *object.Hash) fh.HandlerFunc {
		return timeout.New(optSeconds(opts, "seconds", 30*time.Second))
	},
	"etag": func(opts *object.Hash) fh.HandlerFunc {
		cfg := etag.Config{
			Weak:    optBool(opts, "weak", false),
			MinSize: optInt(opts, "min_size", 0),
		}
		if methods := optStringSlice(opts, "methods"); methods != nil {
			cfg.Methods = methods
		}
		return etag.New(cfg)
	},
}

func builtinNativeMiddleware(args ...object.Object) object.Object {
	if len(args) < 2 {
		return object.NewError("native_middleware() requires at least (server, name)")
	}
	srv, ok := args[0].(*SPLServer)
	if !ok {
		return object.NewError("native_middleware() first argument must be a server, got %s", args[0].Type())
	}
	name, ok := args[1].(*object.String)
	if !ok {
		return object.NewError("native_middleware() name must be a string")
	}
	factory, ok := nativeMiddlewareRegistry[name.Value]
	if !ok {
		names := make([]string, 0, len(nativeMiddlewareRegistry))
		for k := range nativeMiddlewareRegistry {
			names = append(names, k)
		}
		return object.NewError("native_middleware() unknown middleware %q, available: %s", name.Value, strings.Join(names, ", "))
	}
	srv.addNativeMiddleware(factory(optHash(args, 2)))
	return srv
}

// ── Builtins ───────────────────────────────────────────────────────

func init() {
	eval.RegisterPluginBuiltins(map[string]*object.Builtin{
		"server":            {Fn: builtinServer},
		"route":             {Fn: builtinRoute},
		"middleware":        {Fn: builtinMiddleware},
		"native_middleware": {Fn: builtinNativeMiddleware},
		"static":            {Fn: builtinStatic},
		"template_dir":      {Fn: builtinTemplateDir},
		"web_app":           {Fn: builtinWebApp},
		"route_group":       {Fn: builtinRouteGroup},
		"group":             {Fn: builtinGroup},
		"listen":            {FnWithEnv: builtinListen},
		"listen_async":      {FnWithEnv: builtinListenAsync},
		"shutdown":          {Fn: builtinShutdown},
	})

	prev := eval.DotExpressionHook
	eval.DotExpressionHook = func(left object.Object, name string) object.Object {
		switch obj := left.(type) {
		case *SPLServer:
			return GetServerProperty(obj, name)
		case *SPLGroup:
			return GetGroupProperty(obj, name)
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
		routes:      make([]routeHandler, 0),
		middlewares: make([]middlewareEntry, 0),
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

func isHandlerObject(obj object.Object) bool {
	switch obj.(type) {
	case *object.Function, *object.Builtin:
		return true
	default:
		return false
	}
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
	var handlers []object.Object

	if strArg, ok := args[1].(*object.String); ok {
		if _, methodIsPattern := args[2].(*object.String); methodIsPattern {
			// (server, method, pattern, handler, ...middlewares)
			method = strArg.Value
			p, ok := args[2].(*object.String)
			if !ok {
				return object.NewError("route() pattern must be a string")
			}
			pattern = p.Value
			handlers = args[3:]
		} else {
			// (server, pattern, handler, ...middlewares)
			pattern = strArg.Value
			method = "ANY"
			handlers = args[2:]
		}
	} else {
		return object.NewError("route() pattern/method must be a string")
	}

	if len(handlers) == 0 {
		return object.NewError("route() requires a handler")
	}
	for _, h := range handlers {
		if !isHandlerObject(h) {
			return object.NewError("route() handlers must be functions")
		}
	}

	srv.addRoute(method, pattern, handlers)
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

// builtinGroup is the primary way to create a route group:
// group(server, prefix, ...middlewares) -> group object with
// .get/.post/.put/.delete/.patch/.head/.options/.any/.route/.use/.group.
func builtinGroup(args ...object.Object) object.Object {
	if len(args) < 2 {
		return object.NewError("group() requires at least (server, prefix)")
	}
	srv, ok := args[0].(*SPLServer)
	if !ok {
		return object.NewError("group() first argument must be a server")
	}
	prefix, ok := args[1].(*object.String)
	if !ok {
		return object.NewError("group() prefix must be a string")
	}
	return &SPLGroup{srv: srv, prefix: prefix.Value, middlewares: append([]object.Object(nil), args[2:]...)}
}

// builtinRouteGroup supports both:
//   - legacy: route_group(server, prefix, method, pattern, handler, ...mw)
//   - new:    route_group(server, prefix, ...middlewares) -> group object
func builtinRouteGroup(args ...object.Object) object.Object {
	if len(args) < 2 {
		return object.NewError("route_group() requires at least (server, prefix)")
	}
	srv, ok := args[0].(*SPLServer)
	if !ok {
		return object.NewError("route_group() first argument must be a server")
	}
	prefix, ok := args[1].(*object.String)
	if !ok {
		return object.NewError("route_group() prefix must be a string")
	}

	if len(args) >= 5 {
		if method, ok := args[2].(*object.String); ok {
			if pattern, ok := args[3].(*object.String); ok {
				handlers := args[4:]
				for _, h := range handlers {
					if !isHandlerObject(h) {
						return object.NewError("route_group() handlers must be functions")
					}
				}
				srv.addRoute(method.Value, joinPath(prefix.Value, pattern.Value), handlers)
				return srv
			}
		}
	}

	return &SPLGroup{srv: srv, prefix: prefix.Value, middlewares: append([]object.Object(nil), args[2:]...)}
}

func builtinListen(env *object.Environment, args ...object.Object) object.Object {
	if err := security.CheckCapabilityAllowed(security.CapabilityServer); err != nil {
		return object.NewError("%s", err)
	}
	if err := security.CheckCapabilityAllowed(security.CapabilityNetwork); err != nil {
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
	app := srv.build()
	srv.mu.Lock()
	srv.app = app
	srv.running = true
	srv.mu.Unlock()
	registerServerShutdownHook(srv)

	fmt.Printf("SPL server listening on %s\n", srv.addr)
	err := app.Listen(srv.addr)
	srv.mu.Lock()
	srv.running = false
	srv.mu.Unlock()

	if err != nil {
		return object.NewError("server error: %s", err)
	}
	return object.NULL
}

func builtinListenAsync(env *object.Environment, args ...object.Object) object.Object {
	if err := security.CheckCapabilityAllowed(security.CapabilityServer); err != nil {
		return object.NewError("%s", err)
	}
	if err := security.CheckCapabilityAllowed(security.CapabilityNetwork); err != nil {
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
	app := srv.build()
	srv.mu.Lock()
	srv.app = app
	srv.running = true
	srv.mu.Unlock()
	registerServerShutdownHook(srv)

	go func() {
		fmt.Printf("SPL server listening on %s\n", srv.addr)
		err := app.Listen(srv.addr)
		srv.mu.Lock()
		srv.running = false
		srv.mu.Unlock()
		if err != nil {
			fmt.Printf("server error: %s\n", err)
		}
	}()

	if env != nil {
		env.RegisterCleanup(func() {
			done := make(chan struct{})
			go func() {
				_ = app.Shutdown()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
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
	srv.mu.RLock()
	app := srv.app
	srv.mu.RUnlock()
	if app == nil {
		return object.NULL
	}
	if err := app.Shutdown(); err != nil {
		return object.NewError("shutdown error: %s", err)
	}
	return object.NULL
}
