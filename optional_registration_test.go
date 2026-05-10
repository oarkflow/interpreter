package interpreter_test

import (
	"testing"

	_ "github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/eval"
)

func TestRootImportDoesNotRegisterOptionalBuiltins(t *testing.T) {
	for _, name := range []string{"db_connect", "http_get", "ftp_list", "bcrypt_hash", "image_resize"} {
		if eval.HasBuiltin(name) {
			t.Fatalf("optional builtin %q registered by root import", name)
		}
	}
}
