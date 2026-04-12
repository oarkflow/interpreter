package eval_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

func TestHTTPGetBuiltin(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	fn, ok := eval.Builtins["http_get"]
	if !ok {
		t.Fatalf("http_get builtin not found")
	}

	res := fn.Fn(&object.String{Value: ts.URL})
	tuple, ok := res.(*object.Array)
	if !ok || len(tuple.Elements) != 2 {
		t.Fatalf("expected [result, err], got %T", res)
	}
	if tuple.Elements[1] != object.NULL {
		t.Fatalf("expected null error, got %s", tuple.Elements[1].Inspect())
	}
	body := tuple.Elements[0].Inspect()
	if !strings.Contains(body, "ok") {
		t.Fatalf("expected response body in result, got %s", body)
	}
}

func TestSprintfTypeVerb(t *testing.T) {
	fn, ok := eval.Builtins["sprintf"]
	if !ok {
		t.Fatalf("sprintf builtin not found")
	}
	res := fn.Fn(&object.String{Value: "%T"}, &object.Integer{Value: 1})
	s, ok := res.(*object.String)
	if !ok {
		t.Fatalf("expected string result, got %T", res)
	}
	if s.Value != "INTEGER" {
		t.Fatalf("expected INTEGER, got %q", s.Value)
	}
}
