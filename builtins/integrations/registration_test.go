package integrations

import (
	"testing"

	"github.com/oarkflow/interpreter/pkg/eval"
)

func TestIntegrationBuiltinsRegisterOnImport(t *testing.T) {
	for _, name := range []string{"http_get", "http_post", "ftp_list", "sftp_list"} {
		if !eval.HasBuiltin(name) {
			t.Fatalf("expected integration builtin %q to be registered", name)
		}
	}
}
