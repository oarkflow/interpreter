package interpreter_test

import (
	"strings"
	"testing"

	"github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/eval"
)

func TestRootImportDoesNotRegisterOptionalBuiltins(t *testing.T) {
	for _, name := range []string{"db_connect", "http_get", "ftp_list", "bcrypt_hash", "image_resize", "bulk_rename", "media_convert"} {
		if eval.HasBuiltin(name) {
			t.Fatalf("optional builtin %q registered by root import", name)
		}
	}
}

func TestOptionalBuiltinModuleImportExplainsMissingLink(t *testing.T) {
	_, err := interpreter.ExecWithOptions(`import "database"; db_connect("sqlite", ":memory:");`, nil, interpreter.ExecOptions{AllowInProcessFallback: true})
	if err == nil {
		t.Fatal("expected unlinked optional database builtin to fail")
	}
	if !strings.Contains(err.Error(), "optional builtin") || !strings.Contains(err.Error(), "cmd/interpreter-full") {
		t.Fatalf("expected actionable optional builtin error, got %v", err)
	}
}
