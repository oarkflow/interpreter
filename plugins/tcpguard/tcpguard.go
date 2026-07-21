// Package tcpguard exposes github.com/oarkflow/tcpguard as SPL builtins: load
// a BCL "guard pack" (inline block or file/directory), build a *tcpguard.
// Guard from it, and attach it as request middleware to an SPL server()
// from plugins/server. Deeper features (abuse detection tuning, approval
// workflows, Redis-backed stores, the management server) are intentionally
// not exposed yet - see docs/features for the supported surface.
package tcpguard

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/oarkflow/tcpguard"

	builtinspkg "github.com/oarkflow/interpreter/pkg/builtins"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/security"
	"github.com/oarkflow/interpreter/plugins/server"
)

// TCPGuardBundle wraps a parsed tcpguard.Bundle (a BCL guard pack) as an SPL
// handle passed to tcpguard_new.
type TCPGuardBundle struct {
	bundle tcpguard.Bundle
}

func (b *TCPGuardBundle) Type() object.ObjectType { return object.TCPGUARD_BUNDLE_OBJ }
func (b *TCPGuardBundle) Inspect() string {
	return "<tcpguard_bundle " + b.bundle.Name + " " + string(b.bundle.Mode) + ">"
}

// SPLGuard wraps a *tcpguard.Guard as an SPL handle passed to
// tcpguard_evaluate/guard_middleware.
type SPLGuard struct {
	g *tcpguard.Guard
}

func (g *SPLGuard) Type() object.ObjectType { return object.TCPGUARD_GUARD_OBJ }
func (g *SPLGuard) Inspect() string         { return "<tcpguard_guard>" }

func init() {
	eval.RegisterPluginBuiltins(map[string]*object.Builtin{
		"tcpguard_load":     {Fn: builtinTCPGuardLoad},
		"tcpguard_new":      {Fn: builtinTCPGuardNew},
		"tcpguard_evaluate": {Fn: builtinTCPGuardEvaluate},
		"guard_middleware":  {Fn: builtinGuardMiddleware},
	})
}

// tcpguard_load(source_or_path) -> (bundle, err)
// The argument is auto-detected: a real file is loaded with
// LoadTCPGuardBundleFile, a real directory with LoadTCPGuardBundleDir
// (following its "include" globs), and anything else is parsed as an
// inline BCL block via ParseTCPGuardBundle.
func builtinTCPGuardLoad(args ...object.Object) object.Object {
	if len(args) < 1 {
		return object.NewError("tcpguard_load() requires (source_or_path)")
	}
	sourceOrPath, errObj := asString(args[0], "source_or_path")
	if errObj != nil {
		return errObj
	}
	ctx := context.Background()
	if safe, isDir, isPath := resolveSourceOrPath(sourceOrPath); isPath {
		if err := security.CheckFileReadAllowed(safe); err != nil {
			return object.NewError("tcpguard_load: %s", err)
		}
		var bundle tcpguard.Bundle
		var err error
		if isDir {
			bundle, err = tcpguard.LoadTCPGuardBundleDir(ctx, safe)
		} else {
			bundle, err = tcpguard.LoadTCPGuardBundleFile(ctx, safe)
		}
		if err != nil {
			return tuple(object.NULL, errString(err))
		}
		return tuple(&TCPGuardBundle{bundle: bundle}, object.NULL)
	}
	bundle, err := tcpguard.ParseTCPGuardBundle([]byte(sourceOrPath))
	if err != nil {
		return tuple(object.NULL, errString(err))
	}
	return tuple(&TCPGuardBundle{bundle: bundle}, object.NULL)
}

// tcpguard_new(bundle[, opts_hash]) -> (guard, err)
// opts_hash: mode ("enforce" (default) or "monitor").
func builtinTCPGuardNew(args ...object.Object) object.Object {
	if err := security.CheckCapabilityAllowed(security.CapabilityPolicy); err != nil {
		return object.NewError("%s", err)
	}
	if len(args) < 1 {
		return object.NewError("tcpguard_new() requires (bundle)")
	}
	b, ok := args[0].(*TCPGuardBundle)
	if !ok {
		return object.NewError("tcpguard_new() first argument must be a tcpguard bundle, got %s", args[0].Type())
	}
	opts := optHash(args, 1)
	mode := tcpguard.Enforce
	switch strings.ToLower(optString(opts, "mode", "enforce")) {
	case "enforce", "":
		mode = tcpguard.Enforce
	case "monitor":
		mode = tcpguard.Monitor
	default:
		return object.NewError("tcpguard_new() opts.mode must be \"enforce\" or \"monitor\"")
	}
	// GeoIP enrichment is opt-in: tcpguard's default HTTPContextBuilder
	// loads a large in-memory IP-geolocation trie on first use, which is a
	// surprising and expensive default inside an embedded script runtime
	// unless a policy actually depends on network.country/network.city
	// facts. Pass {"geoip": true} to enable it.
	builder := tcpguard.HTTPContextBuilder{DisableGeoIP: !optBool(opts, "geoip", false)}
	guard, err := tcpguard.New(tcpguard.WithBundle(b.bundle), tcpguard.WithMode(mode), tcpguard.WithContextBuilder(builder))
	if err != nil {
		return tuple(object.NULL, errString(err))
	}
	return tuple(&SPLGuard{g: guard}, object.NULL)
}

// tcpguard_evaluate(guard, facts_hash) -> (decision_hash, err)
// facts_hash describes a synthetic HTTP request to evaluate:
// method, path (may include a "?query"), body, ip, and a nested headers
// hash. This is for ad-hoc scripted evaluation outside a real server();
// guard_middleware is the integration for live requests.
func builtinTCPGuardEvaluate(args ...object.Object) object.Object {
	if len(args) < 2 {
		return object.NewError("tcpguard_evaluate() requires (guard, facts)")
	}
	g, ok := args[0].(*SPLGuard)
	if !ok {
		return object.NewError("tcpguard_evaluate() first argument must be a tcpguard guard, got %s", args[0].Type())
	}
	facts, ok := args[1].(*object.Hash)
	if !ok {
		return object.NewError("tcpguard_evaluate() facts argument must be a hash")
	}
	method := optString(facts, "method", "GET")
	path := optString(facts, "path", "/")
	body := optString(facts, "body", "")
	ip := optString(facts, "ip", "")
	host := optString(facts, "host", "")
	headers := headersFromHash(facts)

	httpReq, err := newHTTPRequest(context.Background(), method, path, ip, host, body, headers)
	if err != nil {
		return tuple(object.NULL, errString(err))
	}
	result, err := g.g.EvaluateHTTPRequest(httpReq)
	if err != nil {
		return tuple(object.NULL, errString(err))
	}
	return tuple(eval.ToObject(result.Decision), object.NULL)
}

// guard_middleware(server, guard) -> server
// Attaches guard as global middleware on an SPL server() from
// plugins/server: every request is bridged into a synthetic *http.Request,
// evaluated through the guard, and either blocked with the guard's decision
// response or passed through to the rest of the middleware/route chain.
func builtinGuardMiddleware(args ...object.Object) object.Object {
	if len(args) < 2 {
		return object.NewError("guard_middleware() requires (server, guard)")
	}
	srv, ok := args[0].(*server.SPLServer)
	if !ok {
		return object.NewError("guard_middleware() first argument must be a server, got %s", args[0].Type())
	}
	g, ok := args[1].(*SPLGuard)
	if !ok {
		return object.NewError("guard_middleware() second argument must be a tcpguard guard, got %s", args[1].Type())
	}

	mw := &object.Builtin{Fn: func(mwArgs ...object.Object) object.Object {
		req, ok := mwArgs[0].(*server.SPLRequest)
		if !ok {
			return object.NewError("guard_middleware: expected a request object")
		}
		res, ok := mwArgs[1].(*server.SPLResponse)
		if !ok {
			return object.NewError("guard_middleware: expected a response object")
		}
		next, _ := mwArgs[2].(*object.Builtin)

		httpReq, err := newHTTPRequest(req.Ctx.Context(), req.Method, req.Ctx.OriginalURL(), req.Ctx.IP(), req.Ctx.Hostname(), req.Body, req.Ctx.GetReqHeaders())
		if err != nil {
			res.Ctx.Status(http.StatusInternalServerError)
			_ = res.Ctx.SendString(err.Error())
			return object.NULL
		}
		result, err := g.g.EvaluateHTTPRequest(httpReq)
		if err != nil {
			res.Ctx.Status(http.StatusInternalServerError)
			_ = res.Ctx.SendString(err.Error())
			return object.NULL
		}
		if result.Enforced {
			resp := result.Response
			res.Ctx.Status(resp.Status)
			for k, v := range resp.Headers {
				res.Ctx.Set(k, v)
			}
			_ = res.Ctx.JSON(resp.Body)
			return object.NULL
		}
		if next != nil {
			next.Fn()
		}
		return object.NULL
	}}

	mwBuiltin, ok := eval.BuiltinByName("middleware")
	if !ok {
		return object.NewError("guard_middleware() requires the server plugin's middleware() builtin to be registered")
	}
	return mwBuiltin.Fn(srv, mw)
}

// ── HTTP request bridging ────────────────────────────────────────────

func newHTTPRequest(ctx context.Context, method, rawURL, remoteAddr, host, body string, headers map[string][]string) (*http.Request, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if method == "" {
		method = "GET"
	}
	if rawURL == "" {
		rawURL = "/"
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, vals := range headers {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	if host != "" {
		req.Host = host
	}
	return req, nil
}

func headersFromHash(facts *object.Hash) map[string][]string {
	v, ok := hashGet(facts, "headers")
	if !ok {
		return nil
	}
	h, ok := v.(*object.Hash)
	if !ok {
		return nil
	}
	out := make(map[string][]string, len(h.Pairs))
	for _, pair := range h.Pairs {
		key, ok := pair.Key.(*object.String)
		if !ok {
			continue
		}
		switch val := pair.Value.(type) {
		case *object.String:
			out[key.Value] = []string{val.Value}
		case *object.Array:
			for _, el := range val.Elements {
				if s, ok := el.(*object.String); ok {
					out[key.Value] = append(out[key.Value], s.Value)
				}
			}
		}
	}
	return out
}

// ── helpers ──────────────────────────────────────────────────────────

// resolveSourceOrPath tells apart an inline BCL block from a file/directory
// path, same heuristic as plugins/rules: a real path never contains a
// newline, so anything with one is treated as inline source without even
// attempting a filesystem stat.
func resolveSourceOrPath(sourceOrPath string) (safePath string, isDir bool, isPath bool) {
	if strings.ContainsAny(sourceOrPath, "\n\r") {
		return "", false, false
	}
	safe, err := builtinspkg.SanitizePathLocal(sourceOrPath)
	if err != nil {
		return "", false, false
	}
	info, err := os.Stat(safe)
	if err != nil {
		return "", false, false
	}
	return safe, info.IsDir(), true
}

func tuple(values ...object.Object) *object.Array {
	return &object.Array{Elements: values}
}

func errString(err error) object.Object {
	if err == nil {
		return object.NULL
	}
	return &object.String{Value: err.Error()}
}

func asString(arg object.Object, name string) (string, object.Object) {
	if s, ok := arg.(*object.Secret); ok {
		return s.Value, nil
	}
	if arg == nil {
		return "", object.NewError("argument `%s` must be STRING, got <nil>", name)
	}
	if arg.Type() != object.STRING_OBJ {
		return "", object.NewError("argument `%s` must be STRING, got %s", name, arg.Type())
	}
	return arg.(*object.String).Value, nil
}

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

func optBool(h *object.Hash, key string, def bool) bool {
	if v, ok := hashGet(h, key); ok {
		if b, ok := v.(*object.Boolean); ok {
			return b.Value
		}
	}
	return def
}
