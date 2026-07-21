package server

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oarkflow/interpreter/pkg/object"
)

// startTestServer builds the SPLServer's fh.App and serves it on an
// ephemeral loopback port, returning the base URL and a shutdown func.
func startTestServer(t *testing.T, srv *SPLServer) (string, func()) {
	t.Helper()
	srv.env = object.NewEnvironment()
	app := srv.build()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = app.Serve(ln) }()
	// give the accept loop a moment to start.
	time.Sleep(20 * time.Millisecond)
	base := "http://" + ln.Addr().String()
	return base, func() { _ = app.Shutdown() }
}

func builtinFn(fn func(args ...object.Object) object.Object) *object.Builtin {
	return &object.Builtin{Fn: fn}
}

func get(t *testing.T, url string) (int, string, http.Header) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), resp.Header
}

func TestRouteBasic(t *testing.T) {
	srv := &SPLServer{staticDirs: map[string]string{}}
	srv.addRoute("GET", "/hello", []object.Object{builtinFn(func(args ...object.Object) object.Object {
		res := args[1].(*SPLResponse)
		res.Ctx.Type("text/plain")
		_ = res.Ctx.SendString("hi")
		return object.NULL
	})})

	base, shutdown := startTestServer(t, srv)
	defer shutdown()

	code, body, _ := get(t, base+"/hello")
	if code != 200 || body != "hi" {
		t.Fatalf("got %d %q", code, body)
	}
}

func TestRouteParams(t *testing.T) {
	srv := &SPLServer{staticDirs: map[string]string{}}
	srv.addRoute("GET", "/users/:id", []object.Object{builtinFn(func(args ...object.Object) object.Object {
		req := args[0].(*SPLRequest)
		res := args[1].(*SPLResponse)
		_ = res.Ctx.SendString("id=" + req.Params["id"])
		return object.NULL
	})})

	base, shutdown := startTestServer(t, srv)
	defer shutdown()

	code, body, _ := get(t, base+"/users/42")
	if code != 200 || body != "id=42" {
		t.Fatalf("got %d %q", code, body)
	}
}

func TestPerRouteMiddlewareChain(t *testing.T) {
	var order []string
	mw := func(name string) object.Object {
		return builtinFn(func(args ...object.Object) object.Object {
			order = append(order, name)
			next := args[2].(*object.Builtin)
			next.Fn()
			return object.NULL
		})
	}
	srv := &SPLServer{staticDirs: map[string]string{}}
	srv.addRoute("GET", "/chain", []object.Object{
		mw("mw1"),
		mw("mw2"),
		builtinFn(func(args ...object.Object) object.Object {
			order = append(order, "handler")
			res := args[1].(*SPLResponse)
			_ = res.Ctx.SendString("done")
			return object.NULL
		}),
	})

	base, shutdown := startTestServer(t, srv)
	defer shutdown()

	code, body, _ := get(t, base+"/chain")
	if code != 200 || body != "done" {
		t.Fatalf("got %d %q", code, body)
	}
	want := "mw1,mw2,handler"
	if got := strings.Join(order, ","); got != want {
		t.Fatalf("order = %q, want %q", got, want)
	}
}

func TestMiddlewareShortCircuit(t *testing.T) {
	handlerCalled := false
	srv := &SPLServer{staticDirs: map[string]string{}}
	srv.addMiddleware("/", builtinFn(func(args ...object.Object) object.Object {
		res := args[1].(*SPLResponse)
		res.StatusCode = 401
		res.Ctx.Status(401)
		_ = res.Ctx.SendString("unauthorized")
		// deliberately not calling next()
		return object.NULL
	}))
	srv.addRoute("GET", "/secret", []object.Object{builtinFn(func(args ...object.Object) object.Object {
		handlerCalled = true
		return object.NULL
	})})

	base, shutdown := startTestServer(t, srv)
	defer shutdown()

	code, body, _ := get(t, base+"/secret")
	if code != 401 || body != "unauthorized" {
		t.Fatalf("got %d %q", code, body)
	}
	if handlerCalled {
		t.Fatalf("handler should not have run")
	}
}

// TestGlobalMiddlewareThenRouteParam is a regression test for a bug where a
// global middleware registered before a param route (the common case: a
// logging/auth middleware added via middleware(app, fn), then routes)
// caused req.param() to always return null in the route handler. The
// middleware's request wrapper was built and cached (via ctx.Locals) with
// no known param names, and every later handler in the same chain reused
// that cached wrapper instead of rebuilding with the route's own names.
func TestGlobalMiddlewareThenRouteParam(t *testing.T) {
	srv := &SPLServer{staticDirs: map[string]string{}}
	srv.addMiddleware("/", builtinFn(func(args ...object.Object) object.Object {
		args[2].(*object.Builtin).Fn()
		return object.NULL
	}))
	srv.addRoute("POST", "/todos/:id/toggle", []object.Object{builtinFn(func(args ...object.Object) object.Object {
		req := args[0].(*SPLRequest)
		res := args[1].(*SPLResponse)
		_ = res.Ctx.SendString("id=[" + req.Params["id"] + "]")
		return object.NULL
	})})

	base, shutdown := startTestServer(t, srv)
	defer shutdown()

	resp, err := http.Post(base+"/todos/1/toggle", "", nil)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 256)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])
	if body != "id=[1]" {
		t.Fatalf("got %d %q, want id=[1] (global middleware must not blank out route params)", resp.StatusCode, body)
	}
}

func TestPathScopedMiddleware(t *testing.T) {
	var hit bool
	srv := &SPLServer{staticDirs: map[string]string{}}
	srv.addMiddleware("/api", builtinFn(func(args ...object.Object) object.Object {
		hit = true
		next := args[2].(*object.Builtin)
		next.Fn()
		return object.NULL
	}))
	srv.addRoute("GET", "/api/ping", []object.Object{builtinFn(func(args ...object.Object) object.Object {
		res := args[1].(*SPLResponse)
		_ = res.Ctx.SendString("pong")
		return object.NULL
	})})
	srv.addRoute("GET", "/other", []object.Object{builtinFn(func(args ...object.Object) object.Object {
		res := args[1].(*SPLResponse)
		_ = res.Ctx.SendString("other")
		return object.NULL
	})})

	base, shutdown := startTestServer(t, srv)
	defer shutdown()

	_, _, _ = get(t, base+"/other")
	if hit {
		t.Fatalf("path-scoped middleware ran for unrelated path")
	}
	_, body, _ := get(t, base+"/api/ping")
	if !hit || body != "pong" {
		t.Fatalf("path-scoped middleware did not run for matching path")
	}
}

func TestRouteGroup(t *testing.T) {
	srv := &SPLServer{staticDirs: map[string]string{}}
	var groupMwRan, nestedMwRan bool

	api := &SPLGroup{srv: srv, prefix: "/api"}
	api.use(builtinFn(func(args ...object.Object) object.Object {
		groupMwRan = true
		args[2].(*object.Builtin).Fn()
		return object.NULL
	}))
	api.route("GET", "/todos", []object.Object{builtinFn(func(args ...object.Object) object.Object {
		res := args[1].(*SPLResponse)
		_ = res.Ctx.SendString("todos")
		return object.NULL
	})})

	admin := api.subGroup("/admin", nil)
	admin.use(builtinFn(func(args ...object.Object) object.Object {
		nestedMwRan = true
		args[2].(*object.Builtin).Fn()
		return object.NULL
	}))
	admin.route("GET", "/stats", []object.Object{builtinFn(func(args ...object.Object) object.Object {
		res := args[1].(*SPLResponse)
		_ = res.Ctx.SendString("stats")
		return object.NULL
	})})

	base, shutdown := startTestServer(t, srv)
	defer shutdown()

	_, body, _ := get(t, base+"/api/todos")
	if body != "todos" || !groupMwRan {
		t.Fatalf("group route failed: body=%q groupMwRan=%v", body, groupMwRan)
	}

	groupMwRan, nestedMwRan = false, false
	_, body, _ = get(t, base+"/api/admin/stats")
	if body != "stats" || !groupMwRan || !nestedMwRan {
		t.Fatalf("nested group route failed: body=%q groupMwRan=%v nestedMwRan=%v", body, groupMwRan, nestedMwRan)
	}
}

func TestStaticFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("static-content"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	srv := &SPLServer{staticDirs: map[string]string{"/public/": dir}}

	base, shutdown := startTestServer(t, srv)
	defer shutdown()

	code, body, _ := get(t, base+"/public/hello.txt")
	if code != 200 || body != "static-content" {
		t.Fatalf("got %d %q", code, body)
	}
}

func TestNativeMiddlewareRequestID(t *testing.T) {
	srv := &SPLServer{staticDirs: map[string]string{}}
	srv.addNativeMiddleware(nativeMiddlewareRegistry["requestid"](nil))
	srv.addRoute("GET", "/ping", []object.Object{builtinFn(func(args ...object.Object) object.Object {
		res := args[1].(*SPLResponse)
		_ = res.Ctx.SendString("pong")
		return object.NULL
	})})

	base, shutdown := startTestServer(t, srv)
	defer shutdown()

	_, _, headers := get(t, base+"/ping")
	if headers.Get("X-Request-ID") == "" {
		t.Fatalf("expected X-Request-ID header from native requestid middleware")
	}
}

func TestNativeMiddlewareUnknown(t *testing.T) {
	result := builtinNativeMiddleware(&SPLServer{}, &object.String{Value: "does-not-exist"})
	if !object.IsError(result) {
		t.Fatalf("expected error for unknown native middleware, got %v", result)
	}
}

func TestGoToSPLObjectRoundtrip(t *testing.T) {
	m := map[string]any{"a": float64(1), "b": "x", "c": []any{float64(1), float64(2)}}
	obj := GoToSPLObject(m)
	back := SPLObjectToGo(obj)
	backMap, ok := back.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", back)
	}
	if fmt.Sprint(backMap["a"]) != "1" || backMap["b"] != "x" {
		t.Fatalf("roundtrip mismatch: %#v", backMap)
	}
}
