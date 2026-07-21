package rules

import (
	"os"
	"testing"

	"github.com/oarkflow/interpreter/pkg/object"
)

// demoSource mirrors the minimal fixture from github.com/oarkflow/rules's
// own service_test.go: a single decision table that allows when
// request.ok is true and denies otherwise.
const demoSource = `module "demo" {
  decision_schema "access" { effects [allow, deny] default deny strategy first_match }
  decision_table "access" {
    default deny
    hit_policy first
    row "allow-ok" {
      when { request.ok == true }
      then { decision allow reason "ok" }
      reason "ok"
      reason_code "OK"
    }
  }
}`

func newService(t *testing.T) *RulesService {
	t.Helper()
	svcObj := builtinRulesService()
	svc, ok := svcObj.(*RulesService)
	if !ok {
		t.Fatalf("rules_service() did not return *RulesService: %s", svcObj.Inspect())
	}
	return svc
}

func mustOK(t *testing.T, result object.Object) *object.Hash {
	t.Helper()
	arr, ok := result.(*object.Array)
	if !ok || len(arr.Elements) != 2 {
		t.Fatalf("expected a 2-element tuple, got %s", result.Inspect())
	}
	if arr.Elements[1] != object.NULL {
		t.Fatalf("unexpected error: %s", arr.Elements[1].Inspect())
	}
	h, ok := arr.Elements[0].(*object.Hash)
	if !ok {
		t.Fatalf("expected a hash result, got %s", arr.Elements[0].Inspect())
	}
	return h
}

func nestedGet(t *testing.T, h *object.Hash, keys ...string) object.Object {
	t.Helper()
	var cur object.Object = h
	for _, k := range keys {
		hh, ok := cur.(*object.Hash)
		if !ok {
			t.Fatalf("expected a hash while looking up %v, got %s", keys, cur.Inspect())
		}
		v, ok := hashGet(hh, k)
		if !ok {
			t.Fatalf("missing key %q (path %v) in %s", k, keys, hh.Inspect())
		}
		cur = v
	}
	return cur
}

func factsHash(ok bool) *object.Hash {
	inner := &object.Hash{Pairs: map[object.HashKey]object.HashPair{}}
	okKey := &object.String{Value: "ok"}
	inner.Pairs[okKey.HashKey()] = object.HashPair{Key: okKey, Value: object.NativeBoolToBooleanObject(ok)}
	outer := &object.Hash{Pairs: map[object.HashKey]object.HashPair{}}
	reqKey := &object.String{Value: "request"}
	outer.Pairs[reqKey.HashKey()] = object.HashPair{Key: reqKey, Value: inner}
	return outer
}

func TestRulesPublishAndEvaluate(t *testing.T) {
	svc := newService(t)

	pubResult := builtinRulesPublish(svc, &object.String{Value: "demo"}, &object.String{Value: demoSource})
	mustOK(t, pubResult)

	allowResult := builtinRulesEvaluate(svc, &object.String{Value: "demo"}, &object.String{Value: "access"}, factsHash(true))
	allowHash := mustOK(t, allowResult)
	effect := nestedGet(t, allowHash, "Report", "Decision", "Effect")
	if s, ok := effect.(*object.String); !ok || s.Value != "allow" {
		t.Fatalf("expected effect=allow, got %s", effect.Inspect())
	}
	allowed := nestedGet(t, allowHash, "Report", "Decision", "Allowed")
	if allowed != object.TRUE {
		t.Fatalf("expected allowed=true, got %s", allowed.Inspect())
	}

	denyResult := builtinRulesEvaluate(svc, &object.String{Value: "demo"}, &object.String{Value: "access"}, factsHash(false))
	denyHash := mustOK(t, denyResult)
	denyEffect := nestedGet(t, denyHash, "Report", "Decision", "Effect")
	if s, ok := denyEffect.(*object.String); !ok || s.Value != "deny" {
		t.Fatalf("expected effect=deny, got %s", denyEffect.Inspect())
	}
}

func TestRulesPublishFromFile(t *testing.T) {
	svc := newService(t)
	// SanitizePathLocal confines file reads to the process working
	// directory (the sandbox root), so the fixture must live under the
	// package dir rather than in t.TempDir(), which resolves outside it.
	path := "testdata_demo_decision.bcl"
	if err := os.WriteFile(path, []byte(demoSource), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	pubResult := builtinRulesPublish(svc, &object.String{Value: "demo-file"}, &object.String{Value: path})
	mustOK(t, pubResult)

	evalResult := builtinRulesEvaluate(svc, &object.String{Value: "demo-file"}, &object.String{Value: "access"}, factsHash(true))
	h := mustOK(t, evalResult)
	effect := nestedGet(t, h, "Report", "Decision", "Effect")
	if s, ok := effect.(*object.String); !ok || s.Value != "allow" {
		t.Fatalf("expected effect=allow, got %s", effect.Inspect())
	}
}

func TestRulesPublishInvalidService(t *testing.T) {
	result := builtinRulesPublish(&object.String{Value: "not-a-service"}, &object.String{Value: "demo"}, &object.String{Value: demoSource})
	if !object.IsError(result) {
		t.Fatalf("expected an error object, got %s", result.Inspect())
	}
}
