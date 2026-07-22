package eval_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/oarkflow/interpreter/pkg/object"
)

func TestUnquotedHashKeysInsideArray(t *testing.T) {
	input := `
let orders = [
  { id: "A-17", total: 680 },
  { id: "B-42", total: 1400 },
  { id: "C-08", total: 2200 }
];
let large = orders.filter(o => o.total >= 1000);
large.map(o => o.id);
`

	result := evalWithParserCheck(t, input, object.NewEnvironment())
	items, ok := result.(*object.Array)
	if !ok {
		t.Fatalf("expected array result, got %T (%s)", result, result.Inspect())
	}
	if len(items.Elements) != 2 {
		t.Fatalf("expected 2 filtered orders, got %d (%s)", len(items.Elements), result.Inspect())
	}
	want := []string{"B-42", "C-08"}
	for i, expected := range want {
		value, ok := items.Elements[i].(*object.String)
		if !ok || value.Value != expected {
			t.Fatalf("result[%d]: expected %q, got %T (%s)", i, expected, items.Elements[i], items.Elements[i].Inspect())
		}
	}
}

func TestPrintMappedValuesFromUnquotedHashKeys(t *testing.T) {
	input := `
let orders = [
  { id: "A-17", total: 680 },
  { id: "B-42", total: 1400 },
  { id: "C-08", total: 2200 }
];
let large = orders.filter(o => o.total >= 1000);
print large.map(o => o.id);
`

	var output bytes.Buffer
	env := object.NewEnvironment()
	env.Output = &output
	result := evalWithParserCheck(t, input, env)
	if result != object.NULL {
		t.Fatalf("expected print statement to return null, got %T (%s)", result, result.Inspect())
	}
	printed := output.String()
	if !strings.Contains(printed, "B-42") || !strings.Contains(printed, "C-08") || strings.Contains(printed, "A-17") {
		t.Fatalf("unexpected printed output: %q", printed)
	}
}
