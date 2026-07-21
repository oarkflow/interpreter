# 29 — HTTP Server, Routing & SSE

Source: `plugins/server` (`plugins/server/server.go`). **Verified**: this
package used to live at `pkg/builtins/server` and was linked into every
binary unconditionally; it has since moved to `plugins/server` and is now
an **optional plugin package**, linked only into `cmd/interpreter`
(including its `--playground` mode) via `plugins/builtins.go`'s blank
import — a custom embedding host built against only the root
`github.com/oarkflow/interpreter` module no longer gets `server`/`route`/
`middleware` for free.

## Creating a server and routes

```spl
let app = server(3099);  // or server("localhost:3099")

route(app, "GET", "/hello", function(req, res) {
    res.json({"msg": "hi"});
});

middleware(app, function(req, res, next) {
    print "middleware ran";
    next();
});

route_group(app, "/api", "GET", "/health", function(req, res) {
    res.json({"ok": true});
});

print app.routes;
// [{method:"GET", pattern:"/hello"}, {method:"GET", pattern:"/api/health"}]
```

`route(server, [method,] pattern, handler)` accepts either a 3-arg form
(method defaults to `"ANY"`) or the 4-arg form shown above.
`route_group(server, prefix, [method,] pattern, handler)` prefixes the
pattern and registers it the same way — handy for versioned/grouped APIs
(`/api/v1/...`, `/admin/...`).

`web_app([templateDir])` is shorthand for `server()` with a template
directory preconfigured, for scripts that also render server-side views
(doc 38).

## Middleware chains

```spl
middleware(app, function(req, res, next) {
    print "log: " + req.method + " " + req.path;
    next();
});

middleware(app, "/api", function(req, res, next) {
    if (req.get_header("Authorization") == null) {
        res.status(401).json({"error": "unauthorized"});
        return; // not calling next() short-circuits the chain
    }
    next();
});
```

A path-scoped middleware (`middleware(app, "/api", fn)`) only runs for
requests under that prefix. Skipping `next()` stops the chain — useful for
auth checks that need to reject before reaching the route handler.

## Static files & templates

```spl
static(app, "/public/", "./public");
template_dir(app, "./views");
```

## Request object

```spl
req.method
req.path
req.param("id")          // path parameter
req.get_header("Authorization")
req.json()                // parse request body as JSON
```

## Response object

```spl
res.status(401)
res.header("X-Custom", "value")
res.json({"ok": true})
res.text("plain text")
res.html("<h1>hi</h1>")
res.send(data)
res.redirect("/login"[, 302])
res.file("path/to/file")
res.render("template.html"[, data])       // server-side render
res.render_ssr("template.html"[, data])   // SSR + client hydration payload
res.stream("template.html"[, data])        // flush headers then stream render
res.sse()                                   // returns an SSE writer
```

## Server-Sent Events (SSE)

```spl
route(app, "GET", "/events", function(req, res) {
    let sse = res.sse();
    sse.send("tick", json_encode({"seq": 1}));
    sse.close();
});
```

Verified end-to-end: starting the server, requesting `/events`, and reading
the response body produces the standard SSE wire format:

```text
event: tick
data: {"seq":1}

```

`sse.send([event,] data)` writes an `event:`/`data:` frame and flushes
immediately (requires the underlying response writer to support flushing);
`sse.close()` ends the stream.

## Starting the server

```spl
listen(app, 3099);        // blocks
let handle = listen_async(app, 3099); // non-blocking, returns a handle
shutdown(app);
```

`listen`/`listen_async` require both the `server` and `network` capabilities
under a restrictive security policy (doc 45) — the default playground/
untrusted profile does **not** grant these, so playground example scripts
avoid actually opening sockets.

## Stateful in-memory example pattern

Server-side closures over ordinary SPL variables give you request-scoped
mutable state without a database:

```spl
let users = {};
let nextID = 1;

route(app, "POST", "/api/users", function(req, res) {
    let body = req.json();
    let id = nextID;
    nextID += 1;
    users[to_string(id)] = body;
    res.json({"id": id});
});

route(app, "GET", "/api/users/:id", function(req, res) {
    res.json(users[req.param("id")]);
});
```
