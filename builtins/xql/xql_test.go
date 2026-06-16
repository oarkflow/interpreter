package xql

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/object"
)

func TestTaggedXQLBlockReturnsResponseErrTuple(t *testing.T) {
	obj, err := interpreter.ExecWithOptions(`
import "xql";

let http = [
  {"id": 1, "title": "a", "body": "x"},
  {"id": 2, "title": "b", "body": "y"}
];

let response, err = xql`+"```"+`
http
|> keep id, title
`+"```"+`;

[response, err];
`, nil, interpreter.ExecOptions{AllowInProcessFallback: true})
	if err != nil {
		t.Fatalf("exec failed: %v", err)
	}
	arr, ok := obj.(*object.Array)
	if !ok || len(arr.Elements) != 2 {
		t.Fatalf("expected [response, err], got %#v", obj)
	}
	if arr.Elements[1] != object.NULL {
		t.Fatalf("expected nil err, got %s", arr.Elements[1].Inspect())
	}
	rows, ok := arr.Elements[0].(*object.Array)
	if !ok || len(rows.Elements) != 2 {
		t.Fatalf("expected two response rows, got %#v", arr.Elements[0])
	}
	first, ok := rows.Elements[0].(*object.Hash)
	if !ok {
		t.Fatalf("expected first row hash, got %#v", rows.Elements[0])
	}
	if _, ok := first.Pairs[(&object.String{Value: "body"}).HashKey()]; ok {
		t.Fatalf("expected XQL projection to drop body: %s", first.Inspect())
	}
	if pair, ok := first.Pairs[(&object.String{Value: "title"}).HashKey()]; !ok || pair.Value.Inspect() != "a" {
		t.Fatalf("expected title a, got %s", first.Inspect())
	}
}

func TestTaggedXQLBlockSupportsDirectHTTPCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET request, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"userId":1,"id":2,"title":"hello","body":"world","extra":"drop"}]`))
	}))
	defer srv.Close()

	source := fmt.Sprintf(`
import "xql";

let result, err = xql`+"```"+`
call %s {
  method: "GET"
}
|> keep userId, id, title, body
|> take 5
`+"```"+`;

[result, err];
`, srv.URL)

	obj, err := interpreter.ExecWithOptions(source, nil, interpreter.ExecOptions{AllowInProcessFallback: true})
	if err != nil {
		t.Fatalf("exec failed: %v", err)
	}
	arr, ok := obj.(*object.Array)
	if !ok || len(arr.Elements) != 2 {
		t.Fatalf("expected [result, err], got %#v", obj)
	}
	if arr.Elements[1] != object.NULL {
		t.Fatalf("expected nil err, got %s", arr.Elements[1].Inspect())
	}
	rows, ok := arr.Elements[0].(*object.Array)
	if !ok || len(rows.Elements) != 1 {
		t.Fatalf("expected one response row, got %#v", arr.Elements[0])
	}
	first, ok := rows.Elements[0].(*object.Hash)
	if !ok {
		t.Fatalf("expected first row hash, got %#v", rows.Elements[0])
	}
	if _, ok := first.Pairs[(&object.String{Value: "extra"}).HashKey()]; ok {
		t.Fatalf("expected XQL projection to drop extra: %s", first.Inspect())
	}
	if pair, ok := first.Pairs[(&object.String{Value: "title"}).HashKey()]; !ok || pair.Value.Inspect() != "hello" {
		t.Fatalf("expected title hello, got %s", first.Inspect())
	}
}
