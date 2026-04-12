package interpreter

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPGetBuiltin(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	fn, ok := builtins["http_get"]
	if !ok {
		t.Fatalf("http_get builtin not found")
	}

	res := fn.Fn(&String{Value: ts.URL})
	tuple, ok := res.(*Array)
	if !ok || len(tuple.Elements) != 2 {
		t.Fatalf("expected [result, err], got %T", res)
	}
	if tuple.Elements[1] != NULL {
		t.Fatalf("expected null error, got %s", tuple.Elements[1].Inspect())
	}
	body := tuple.Elements[0].Inspect()
	if !strings.Contains(body, "ok") {
		t.Fatalf("expected response body in result, got %s", body)
	}
}

func TestSprintfTypeVerb(t *testing.T) {
	fn, ok := builtins["sprintf"]
	if !ok {
		t.Fatalf("sprintf builtin not found")
	}
	res := fn.Fn(&String{Value: "%T"}, &Integer{Value: 1})
	s, ok := res.(*String)
	if !ok {
		t.Fatalf("expected string result, got %T", res)
	}
	if s.Value != "INTEGER" {
		t.Fatalf("expected INTEGER, got %q", s.Value)
	}
}
