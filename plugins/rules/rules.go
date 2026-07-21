// Package rules exposes github.com/oarkflow/rules ("Condition") as SPL
// builtins: publish a BCL decision definition against an in-process policy
// service, then evaluate it with facts to get back an effect/allow/score
// decision. This wraps only the core publish/evaluate/activate loop -
// workflows, chains, canary, blast-radius, and approvals are intentionally
// not exposed yet.
package rules

import (
	"context"
	"os"
	"strings"

	condrules "github.com/oarkflow/rules"
	"github.com/oarkflow/rules/pkg/storage"

	builtinspkg "github.com/oarkflow/interpreter/pkg/builtins"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/security"
)

// RulesService wraps an in-process *rules.Service (memory-backed storage)
// as an SPL handle passed to rules_publish/rules_evaluate/rules_activate.
type RulesService struct {
	svc *condrules.Service
}

func (s *RulesService) Type() object.ObjectType { return object.RULES_SERVICE_OBJ }
func (s *RulesService) Inspect() string         { return "<rules_service>" }

func init() {
	eval.RegisterPluginBuiltins(map[string]*object.Builtin{
		"rules_service":  {Fn: builtinRulesService},
		"rules_publish":  {Fn: builtinRulesPublish},
		"rules_evaluate": {Fn: builtinRulesEvaluate},
		"rules_activate": {Fn: builtinRulesActivate},
		"rules_rollback": {Fn: builtinRulesRollback},
	})
}

// rules_service([opts_hash]) -> service
// opts_hash: environment (string), default_tenant (string),
// strict_validation (bool), strict_evaluation (bool),
// require_activation_approval (bool), require_tests (bool).
func builtinRulesService(args ...object.Object) object.Object {
	if err := security.CheckCapabilityAllowed(security.CapabilityPolicy); err != nil {
		return object.NewError("%s", err)
	}
	opts := optHash(args, 0)
	cfg := condrules.Config{
		Environment:               optString(opts, "environment", ""),
		DefaultTenant:             optString(opts, "default_tenant", ""),
		StrictValidation:          optBool(opts, "strict_validation", false),
		StrictEvaluation:          optBool(opts, "strict_evaluation", false),
		RequireActivationApproval: optBool(opts, "require_activation_approval", false),
		RequireTests:              optBool(opts, "require_tests", false),
	}
	return &RulesService{svc: condrules.NewService(storage.NewMemoryStore(), cfg)}
}

// rules_publish(service, name, bcl_source_or_path[, opts_hash]) -> (result_hash, err)
// The third argument is auto-detected: if it resolves to a real file on
// disk it's loaded from there, otherwise it's treated as an inline BCL
// block. opts_hash: version (string), environment (string), run_tests (bool).
func builtinRulesPublish(args ...object.Object) object.Object {
	if len(args) < 3 {
		return object.NewError("rules_publish() requires at least (service, name, bcl_source_or_path)")
	}
	svc, ok := args[0].(*RulesService)
	if !ok {
		return object.NewError("rules_publish() first argument must be a rules service, got %s", args[0].Type())
	}
	name, errObj := asString(args[1], "name")
	if errObj != nil {
		return errObj
	}
	sourceOrPath, errObj := asString(args[2], "bcl_source_or_path")
	if errObj != nil {
		return errObj
	}
	opts := optHash(args, 3)
	req := condrules.PublishRequest{
		Name:        name,
		Version:     optString(opts, "version", ""),
		Environment: optString(opts, "environment", ""),
		RunTests:    optBool(opts, "run_tests", false),
	}
	if safe, isPath := resolveSourceOrPath(sourceOrPath); isPath {
		if err := security.CheckFileReadAllowed(safe); err != nil {
			return object.NewError("rules_publish: %s", err)
		}
		req.Path = safe
	} else {
		req.Source = sourceOrPath
	}
	resp, err := svc.svc.Publish(context.Background(), req)
	if err != nil {
		return tuple(nullOr(resp), errString(err))
	}
	return tuple(eval.ToObject(resp), object.NULL)
}

// rules_evaluate(service, name, decision, facts_hash[, opts_hash]) -> (decision_hash, err)
func builtinRulesEvaluate(args ...object.Object) object.Object {
	if len(args) < 4 {
		return object.NewError("rules_evaluate() requires at least (service, name, decision, facts)")
	}
	svc, ok := args[0].(*RulesService)
	if !ok {
		return object.NewError("rules_evaluate() first argument must be a rules service, got %s", args[0].Type())
	}
	name, errObj := asString(args[1], "name")
	if errObj != nil {
		return errObj
	}
	decision, errObj := asString(args[2], "decision")
	if errObj != nil {
		return errObj
	}
	facts, ok := eval.ToNative(args[3]).(map[string]any)
	if !ok {
		return object.NewError("rules_evaluate() facts argument must be a hash")
	}
	opts := optHash(args, 4)
	resp, err := svc.svc.Evaluate(context.Background(), name, condrules.EvaluateRequest{
		Decision: decision,
		Input:    facts,
		Strict:   optBool(opts, "strict", false),
	})
	if err != nil {
		return tuple(object.NULL, errString(err))
	}
	// eval.ToObject converts EvaluateResponse's nested structs/maps/slices
	// generically via reflection; pointer-typed fields (Outcome, Rank, ...)
	// fall back to a Go-syntax string rather than a nested hash.
	return tuple(eval.ToObject(resp), object.NULL)
}

// rules_activate(service, name, version[, environment]) -> (result_hash, err)
func builtinRulesActivate(args ...object.Object) object.Object {
	if len(args) < 3 {
		return object.NewError("rules_activate() requires at least (service, name, version)")
	}
	svc, ok := args[0].(*RulesService)
	if !ok {
		return object.NewError("rules_activate() first argument must be a rules service, got %s", args[0].Type())
	}
	name, errObj := asString(args[1], "name")
	if errObj != nil {
		return errObj
	}
	version, errObj := asString(args[2], "version")
	if errObj != nil {
		return errObj
	}
	environment := ""
	if len(args) >= 4 {
		environment, errObj = asString(args[3], "environment")
		if errObj != nil {
			return errObj
		}
	}
	resp, err := svc.svc.Activate(context.Background(), name, version, environment)
	if err != nil {
		return tuple(nullOr(resp), errString(err))
	}
	return tuple(eval.ToObject(resp), object.NULL)
}

// rules_rollback(service, name, version[, environment]) -> (result_hash, err)
func builtinRulesRollback(args ...object.Object) object.Object {
	if len(args) < 3 {
		return object.NewError("rules_rollback() requires at least (service, name, version)")
	}
	svc, ok := args[0].(*RulesService)
	if !ok {
		return object.NewError("rules_rollback() first argument must be a rules service, got %s", args[0].Type())
	}
	name, errObj := asString(args[1], "name")
	if errObj != nil {
		return errObj
	}
	version, errObj := asString(args[2], "version")
	if errObj != nil {
		return errObj
	}
	environment := ""
	if len(args) >= 4 {
		environment, errObj = asString(args[3], "environment")
		if errObj != nil {
			return errObj
		}
	}
	resp, err := svc.svc.Rollback(context.Background(), name, version, environment)
	if err != nil {
		return tuple(nullOr(resp), errString(err))
	}
	return tuple(eval.ToObject(resp), object.NULL)
}

// ── helpers ──────────────────────────────────────────────────────────

// resolveSourceOrPath tells apart an inline BCL block from a file path. A
// real file path never contains a newline, so anything with one is treated
// as inline source without even attempting a filesystem stat; anything
// else is sanitized (path traversal, sandbox root) and stat-checked, and
// only reported as a path if it actually resolves to a regular file.
func resolveSourceOrPath(sourceOrPath string) (safePath string, isPath bool) {
	if strings.ContainsAny(sourceOrPath, "\n\r") {
		return "", false
	}
	safe, err := builtinspkg.SanitizePathLocal(sourceOrPath)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(safe)
	if err != nil || info.IsDir() {
		return "", false
	}
	return safe, true
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

func nullOr(v any) object.Object {
	if v == nil {
		return object.NULL
	}
	return eval.ToObject(v)
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
