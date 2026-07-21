package xql

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/xql/pkg/integration/providers"
	xqlengine "github.com/oarkflow/xql/pkg/xql"
)

type xqlIntegrationCtx struct {
	mu       sync.Mutex
	manager  *xqlengine.IntegrationManager
	bindings map[string]xqlengine.IntegrationBinding
}

var globalCtx struct {
	mu     sync.Mutex
	ctx    *xqlIntegrationCtx
	inited bool
}

func getGlobalCtx() *xqlIntegrationCtx {
	globalCtx.mu.Lock()
	defer globalCtx.mu.Unlock()
	if globalCtx.inited {
		return globalCtx.ctx
	}
	reg := xqlengine.NewIntegrationRegistry()
	registerCommonProviders(reg)
	mgr := xqlengine.NewIntegrationManager(reg)
	_ = connectHTTPURIHandlers(context.Background(), mgr)
	globalCtx.ctx = &xqlIntegrationCtx{
		manager:  mgr,
		bindings: map[string]xqlengine.IntegrationBinding{},
	}
	globalCtx.inited = true
	return globalCtx.ctx
}

func init() {
	_ = interpreter.RegisterEmbeddedLanguage("xql", evalXQLBlock)
	_ = interpreter.RegisterStdModule("xql", map[string]interpreter.Object{
		"run":               &object.Builtin{Fn: builtinRun},
		"connect":           &object.Builtin{Fn: builtinConnect},
		"list_integrations": &object.Builtin{Fn: builtinListIntegrations},
	})
	eval.RegisterPluginBuiltins(map[string]*object.Builtin{
		"xql_run":               {Fn: builtinRun},
		"xql_connect":           {Fn: builtinConnect},
		"xql_list_integrations": {Fn: builtinListIntegrations},
	})
}

func registerCommonProviders(reg *xqlengine.IntegrationRegistry) {
	type providerDef struct {
		key     string
		factory func() xqlengine.Integration
	}
	defs := []providerDef{
		{"http.json", func() xqlengine.Integration { return providers.NewHTTPJSONIntegration("http") }},
		{"rest", func() xqlengine.Integration { return providers.NewHTTPJSONIntegration("rest") }},
		{"graphql", func() xqlengine.Integration { return providers.NewHTTPJSONIntegration("graphql") }},
		{"webhook", func() xqlengine.Integration { return providers.NewHTTPJSONIntegration("webhook") }},
		{"hl7", func() xqlengine.Integration { return providers.NewHTTPJSONIntegration("hl7") }},
		{"google_api", func() xqlengine.Integration { return providers.NewHTTPJSONIntegration("google_api") }},
		{"github_api", func() xqlengine.Integration { return providers.NewHTTPJSONIntegration("github_api") }},
		{"facebook_api", func() xqlengine.Integration { return providers.NewHTTPJSONIntegration("facebook_api") }},
		{"slack_api", func() xqlengine.Integration { return providers.NewHTTPJSONIntegration("slack_api") }},
		{"database", func() xqlengine.Integration { return providers.NewDatabaseIntegration("database") }},
		{"webcrawler", func() xqlengine.Integration { return providers.NewWebCrawlerIntegration("webcrawler") }},
		{"smtp", func() xqlengine.Integration { return providers.NewSMTPEmailIntegration("smtp") }},
		{"smpp", func() xqlengine.Integration { return providers.NewSMPPSMSIntegration("smpp") }},
		{"email_http", func() xqlengine.Integration { return providers.NewHTTPEmailIntegration("email_http") }},
		{"sms_http", func() xqlengine.Integration { return providers.NewHTTPSMSIntegration("sms_http") }},
		{"temp_file", func() xqlengine.Integration { return providers.NewTempFileIntegration("temp_file") }},
	}
	for _, d := range defs {
		reg.Register(d.key, d.factory)
	}
	for _, t := range []string{"ftp", "sftp", "kafka", "mqtt", "grpc", "soap", "customtcp", "push", "voip"} {
		t := t
		reg.Register(t, func() xqlengine.Integration { return providers.NewCatalogOnlyIntegration(t) })
	}
}

func connectHTTPURIHandlers(ctx context.Context, mgr *xqlengine.IntegrationManager) error {
	if mgr == nil {
		return nil
	}
	for _, scheme := range []string{"http", "https"} {
		if err := mgr.Connect(ctx, scheme, "http.json", map[string]any{"url": scheme + "://localhost"}); err != nil {
			return fmt.Errorf("connect %s handler: %w", scheme, err)
		}
	}
	return nil
}

func evalXQLBlock(ctx interpreter.EmbeddedLanguageContext) object.Object {
	result, err := run(ctx.Env, ctx.Code)
	if err != nil {
		return tuple(object.NULL, &object.String{Value: err.Error()})
	}
	return tuple(result, object.NULL)
}

func builtinRun(args ...object.Object) object.Object {
	if len(args) != 1 {
		return tuple(object.NULL, &object.String{Value: fmt.Sprintf("xql_run(query) expects 1 argument, got %d", len(args))})
	}
	query, ok := objectString(args[0])
	if !ok {
		return tuple(object.NULL, &object.String{Value: fmt.Sprintf("xql_run(query) expects STRING, got %s", args[0].Type())})
	}
	result, err := run(nil, query)
	if err != nil {
		return tuple(object.NULL, &object.String{Value: err.Error()})
	}
	return tuple(result, object.NULL)
}

func builtinConnect(args ...object.Object) object.Object {
	if len(args) < 3 || len(args) > 4 {
		return object.NewError("xql_connect(alias, type, config [, source]) expects 3 to 4 arguments")
	}
	alias, ok := objectString(args[0])
	if !ok || strings.TrimSpace(alias) == "" {
		return object.NewError("xql_connect alias must be a non-empty STRING")
	}
	typ, ok := objectString(args[1])
	if !ok || strings.TrimSpace(typ) == "" {
		return object.NewError("xql_connect type must be a non-empty STRING")
	}
	config, ok := hashToAnyMap(args[2])
	if !ok {
		return object.NewError("xql_connect config must be HASH")
	}
	ic := getGlobalCtx()
	if err := ic.manager.Connect(context.Background(), alias, typ, config); err != nil {
		return object.NewError("xql_connect: %s", err)
	}
	if len(args) >= 4 {
		source, ok := objectString(args[3])
		if !ok || strings.TrimSpace(source) == "" {
			return object.NewError("xql_connect source must be a non-empty STRING when provided")
		}
		ic.mu.Lock()
		ic.bindings[source] = xqlengine.IntegrationBinding{
			IntegrationAlias: alias,
			OperationType:    xqlengine.OpQuery,
			Action:           "fetch",
			DatasetKey:       source,
		}
		ic.mu.Unlock()
	}
	return object.TRUE
}

func builtinListIntegrations(args ...object.Object) object.Object {
	ic := getGlobalCtx()
	ic.mu.Lock()
	bound := make([]string, 0, len(ic.bindings))
	for src := range ic.bindings {
		bound = append(bound, src)
	}
	ic.mu.Unlock()
	sort.Strings(bound)
	elements := make([]object.Object, 0, len(bound))
	for _, src := range bound {
		elements = append(elements, &object.String{Value: src})
	}
	return &object.Array{Elements: elements}
}

func run(env *object.Environment, query string) (object.Object, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return object.NULL, nil
	}
	baseCat := xqlengine.NewInMemoryCatalog()
	if env != nil {
		registerSources(baseCat, env)
	}
	ic := getGlobalCtx()
	lazyCat := xqlengine.NewLazyIntegrationCatalog(baseCat, ic.manager)
	ic.mu.Lock()
	for src, binding := range ic.bindings {
		lazyCat.BindSource(src, binding)
	}
	ic.mu.Unlock()
	result, _, err := xqlengine.RunResult(context.Background(), query, lazyCat)
	if err != nil {
		return nil, err
	}
	return resultObject(result), nil
}

func registerSources(cat *xqlengine.InMemoryCatalog, env *object.Environment) {
	if env == nil {
		return
	}
	for _, name := range envNames(env) {
		value, ok := env.Get(name)
		if !ok {
			continue
		}
		rows, ok := objectRows(value)
		if !ok {
			continue
		}
		cat.RegisterWithRows(name, xqlengine.InferSchemaFromRows(name, rowsToMaps(rows)), rows)
	}
}

func resultObject(result xqlengine.QueryResult) object.Object {
	switch result.ResultKind {
	case xqlengine.ResultKindMatrix:
		return eval.ToObject(map[string]any{"kind": string(result.ResultKind), "matrix": result.Matrix})
	case xqlengine.ResultKindReport:
		return eval.ToObject(map[string]any{"kind": string(result.ResultKind), "report": result.Report})
	default:
		return eval.ToObject(rowsToMaps(result.Rows))
	}
}

func objectRows(obj object.Object) ([]xqlengine.Row, bool) {
	obj = unwrap(obj)
	switch v := obj.(type) {
	case *object.Array:
		rows := make([]xqlengine.Row, 0, len(v.Elements))
		for _, el := range v.Elements {
			row, ok := objectRow(el)
			if !ok {
				return nil, false
			}
			rows = append(rows, row)
		}
		return rows, true
	case *object.TableValue:
		rows := make([]xqlengine.Row, 0, len(v.Rows))
		for _, row := range v.Rows {
			out := xqlengine.Row{}
			for key, value := range row {
				out[key] = objectNative(value)
			}
			rows = append(rows, out)
		}
		return rows, true
	default:
		if row, ok := objectRow(obj); ok {
			return []xqlengine.Row{row}, true
		}
		return nil, false
	}
}

func objectRow(obj object.Object) (xqlengine.Row, bool) {
	obj = unwrap(obj)
	h, ok := obj.(*object.Hash)
	if !ok {
		return nil, false
	}
	row := xqlengine.Row{}
	for _, pair := range h.Pairs {
		row[pair.Key.Inspect()] = objectNative(pair.Value)
	}
	return row, true
}

func objectNative(obj object.Object) any {
	obj = unwrap(obj)
	switch v := obj.(type) {
	case *object.Null:
		return nil
	case *object.Boolean:
		return v.Value
	case *object.Integer:
		return v.Value
	case *object.Float:
		return v.Value
	case *object.String:
		return v.Value
	case *object.Secret:
		return v.Value
	case *object.Array:
		out := make([]any, len(v.Elements))
		for i, el := range v.Elements {
			out[i] = objectNative(el)
		}
		return out
	case *object.Hash:
		out := make(map[string]any, len(v.Pairs))
		for _, pair := range v.Pairs {
			out[pair.Key.Inspect()] = objectNative(pair.Value)
		}
		return out
	default:
		return v.Inspect()
	}
}

func unwrap(obj object.Object) object.Object {
	switch v := obj.(type) {
	case *object.OwnedValue:
		return unwrap(v.Inner)
	case *object.ImmutableValue:
		return unwrap(v.Inner)
	case *object.LazyValue:
		return unwrap(v.Force())
	default:
		return obj
	}
}

func rowsToMaps(rows []xqlengine.Row) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		m := make(map[string]any, len(row))
		for k, v := range row {
			m[k] = v
		}
		out = append(out, m)
	}
	return out
}

func envNames(env *object.Environment) []string {
	seen := map[string]struct{}{}
	var names []string
	for cur := env; cur != nil; cur = cur.Outer {
		cur.Mu.RLock()
		for name := range cur.Store {
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			names = append(names, name)
		}
		cur.Mu.RUnlock()
	}
	sort.Strings(names)
	return names
}

func objectString(obj object.Object) (string, bool) {
	switch v := obj.(type) {
	case *object.String:
		return v.Value, true
	case *object.Secret:
		return v.Value, true
	default:
		return "", false
	}
}

func hashToAnyMap(obj object.Object) (map[string]any, bool) {
	h, ok := obj.(*object.Hash)
	if !ok {
		h, ok = unwrap(obj).(*object.Hash)
		if !ok {
			return nil, false
		}
	}
	out := make(map[string]any, len(h.Pairs))
	for _, pair := range h.Pairs {
		out[pair.Key.Inspect()] = objectNative(pair.Value)
	}
	return out, true
}

func tuple(values ...object.Object) *object.Array {
	out := make([]object.Object, len(values))
	copy(out, values)
	return &object.Array{Elements: out}
}
