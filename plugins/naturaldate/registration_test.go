package naturaldate

import (
	"testing"

	"github.com/oarkflow/interpreter/pkg/eval"
)

func TestNaturaldateBuiltinsRegisterOnImport(t *testing.T) {
	for _, name := range []string{"naturaldate_parse", "naturaldate_parse_all"} {
		if !eval.HasBuiltin(name) {
			t.Fatalf("expected naturaldate builtin %q to be registered", name)
		}
	}
}
