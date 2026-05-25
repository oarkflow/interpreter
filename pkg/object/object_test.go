package object

import (
	"strings"
	"testing"
)

func TestFormatPlainFormatsNestedArraysAndHashes(t *testing.T) {
	statusKey := &String{Value: "status"}
	statusVal := &String{Value: "planned"}
	opKey := &String{Value: "op"}
	opVal := &String{Value: "rename"}
	h := &Hash{Pairs: map[HashKey]HashPair{
		statusKey.HashKey(): {Key: statusKey, Value: statusVal},
		opKey.HashKey():     {Key: opKey, Value: opVal},
	}}
	out := FormatPlain(&Array{Elements: []Object{h}})
	for _, want := range []string{"[\n", "  {\n", "op: rename", "status: planned"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in formatted output:\n%s", want, out)
		}
	}
}
