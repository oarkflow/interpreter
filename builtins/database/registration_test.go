package database

import (
	"testing"

	"github.com/oarkflow/interpreter/pkg/eval"
)

func TestDatabaseBuiltinsRegisterOnImport(t *testing.T) {
	for _, name := range []string{"db_connect", "db_query", "db_exec", "db_begin", "db_commit", "db_rollback", "query"} {
		if !eval.HasBuiltin(name) {
			t.Fatalf("expected database builtin %q to be registered", name)
		}
	}
}
