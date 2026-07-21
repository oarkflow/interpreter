package secretr

import (
	"testing"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/security"
)

func TestSecretrBuiltinsRegisterOnImport(t *testing.T) {
	for _, name := range []string{"secretr_get", "secretr_set", "secretr_delete", "secretr_list", "secretr_scan"} {
		if !eval.HasBuiltin(name) {
			t.Fatalf("expected secretr builtin %q to be registered", name)
		}
	}
}

func TestSecretrRegistersSecretScanner(t *testing.T) {
	if !security.HasSecretScanner() {
		t.Fatalf("expected importing builtins/secretr to register a secret scanner")
	}
}
