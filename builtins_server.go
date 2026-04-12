package interpreter

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func init() {
	registerBuiltins(map[string]*Builtin{
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
}

// server() -> creates a new SPL server
// server(port) -> creates a new SPL server bound to port
func builtinServer(args ...Object) Object {
	srv := &SPLServer{
		routes:      make([]RouteHandler, 0),
		middlewares: make([]MiddlewareEntry, 0),
		staticDirs:  make(map[string]string),
		shutdownCh:  make(chan struct{}),
	}
	if len(args) >= 1 {
		switch v := args[0].(type) {
		case *Integer:
			srv.addr = fmt.Sprintf(":%d", v.Value)
		case *String:
			srv.addr = v.Value
		}
	}
	return srv
}

// route(server, method, pattern, handler)
// route(server, pattern, handler) — defaults to ANY method
func builtinRoute(args ...Object) Object {
	if len(args) < 3 {
		return newError("route() requires at least (server, pattern, handler)")
	}
	srv, ok := args[0].(*SPLServer)
	if !ok {
		return newError("route() first argument must be a server, got %s", args[0].Type())
	}
	var method, pattern string
	var handler Object

	if len(args) == 3 {
		// route(server, pattern, handler)
		p, ok := args[1].(*String)
		if !ok {
			return newError("route() pattern must be a string")
		}
		pattern = p.Value
		method = "ANY"
		handler = args[2]
	} else {
		// route(server, method, pattern, handler)
		m, ok := args[1].(*String)
		if !ok {
			return newError("route() method must be a string")
		}
		method = m.Value
		p, ok := args[2].(*String)
		if !ok {
			return newError("route() pattern must be a string")
		}
		pattern = p.Value
		handler = args[3]
	}

	if _, ok := handler.(*Function); !ok {
		if _, ok := handler.(*Builtin); !ok {
			return newError("route() handler must be a function")
		}
	}

	srv.addRoute(method, pattern, handler)
	return srv
}

// middleware(server, handler)
// middleware(server, path, handler)
func builtinMiddleware(args ...Object) Object {
	if len(args) < 2 {
		return newError("middleware() requires at least (server, handler)")
	}
	srv, ok := args[0].(*SPLServer)
	if !ok {
		return newError("middleware() first argument must be a server, got %s", args[0].Type())
	}

	var path string
	var handler Object

	if len(args) == 2 {
		path = "/"
		handler = args[1]
	} else {
		p, ok := args[1].(*String)
		if !ok {
			return newError("middleware() path must be a string")
		}
		path = p.Value
		handler = args[2]
	}

	srv.addMiddleware(path, handler)
	return srv
}

// static(server, prefix, directory)
func builtinStatic(args ...Object) Object {
	if len(args) < 3 {
		return newError("static() requires (server, prefix, directory)")
	}
	srv, ok := args[0].(*SPLServer)
	if !ok {
		return newError("static() first argument must be a server")
	}
	prefix, ok := args[1].(*String)
	if !ok {
		return newError("static() prefix must be a string")
	}
	dir, ok := args[2].(*String)
	if !ok {
		return newError("static() directory must be a string")
	}
	srv.addStaticDir(prefix.Value, dir.Value)
	return srv
}

// template_dir(server, directory)
func builtinTemplateDir(args ...Object) Object {
	if len(args) < 2 {
		return newError("template_dir() requires (server, directory)")
	}
	srv, ok := args[0].(*SPLServer)
	if !ok {
		return newError("template_dir() first argument must be a server")
	}
	dir, ok := args[1].(*String)
	if !ok {
		return newError("template_dir() directory must be a string")
	}
	srv.mu.Lock()
	srv.templateDir = dir.Value
	srv.mu.Unlock()
	return srv
}

// web_app(base_path?) -> creates a web server with optional template base dir
func builtinWebApp(args ...Object) Object {
	srvObj := builtinServer()
	srv, ok := srvObj.(*SPLServer)
	if !ok {
		return srvObj
	}
	if len(args) >= 1 {
		if dir, ok := args[0].(*String); ok {
			srv.templateDir = dir.Value
		}
	}
	return srv
}

// route_group(server, prefix, method?, pattern, handler)
func builtinRouteGroup(args ...Object) Object {
	if len(args) < 4 {
		return newError("route_group() requires at least (server, prefix, pattern, handler)")
	}
	srv, ok := args[0].(*SPLServer)
	if !ok {
		return newError("route_group() first argument must be a server")
	}
	prefix, ok := args[1].(*String)
	if !ok {
		return newError("route_group() prefix must be a string")
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
		pattern, ok := args[2].(*String)
		if !ok {
			return newError("route_group() pattern must be a string")
		}
		return builtinRoute(srv, &String{Value: joinPath(prefix.Value, pattern.Value)}, args[3])
	}
	method, ok := args[2].(*String)
	if !ok {
		return newError("route_group() method must be a string")
	}
	pattern, ok := args[3].(*String)
	if !ok {
		return newError("route_group() pattern must be a string")
	}
	return builtinRoute(srv, method, &String{Value: joinPath(prefix.Value, pattern.Value)}, args[4])
}

// listen(server, port?) — blocks until shutdown
func builtinListen(env *Environment, args ...Object) Object {
	if len(args) < 1 {
		return newError("listen() requires a server argument")
	}
	srv, ok := args[0].(*SPLServer)
	if !ok {
		return newError("listen() first argument must be a server")
	}

	if len(args) >= 2 {
		switch v := args[1].(type) {
		case *Integer:
			srv.addr = fmt.Sprintf(":%d", v.Value)
		case *String:
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
		return newError("server error: %s", err)
	}
	return NULL
}

// listen_async(server, port?) — starts server in background, returns server
func builtinListenAsync(env *Environment, args ...Object) Object {
	if len(args) < 1 {
		return newError("listen_async() requires a server argument")
	}
	srv, ok := args[0].(*SPLServer)
	if !ok {
		return newError("listen_async() first argument must be a server")
	}

	if len(args) >= 2 {
		switch v := args[1].(type) {
		case *Integer:
			srv.addr = fmt.Sprintf(":%d", v.Value)
		case *String:
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

	// Give the server a moment to start
	time.Sleep(50 * time.Millisecond)
	return srv
}

// shutdown(server)
func builtinShutdown(args ...Object) Object {
	if len(args) < 1 {
		return newError("shutdown() requires a server argument")
	}
	srv, ok := args[0].(*SPLServer)
	if !ok {
		return newError("shutdown() first argument must be a server")
	}
	if srv.server == nil {
		return NULL
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.server.Shutdown(ctx); err != nil {
		return newError("shutdown error: %s", err)
	}
	return NULL
}
