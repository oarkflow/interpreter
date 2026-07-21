package crypto

import (
	"testing"

	"github.com/oarkflow/interpreter/pkg/eval"
)

func TestCryptoExtraBuiltinsRegisterOnImport(t *testing.T) {
	for _, name := range []string{"bcrypt_hash", "bcrypt_verify"} {
		if !eval.HasBuiltin(name) {
			t.Fatalf("expected cryptoextra builtin %q to be registered", name)
		}
	}
}
