package interpreter_test

import (
	"testing"

	. "github.com/oarkflow/interpreter"

	_ "github.com/oarkflow/interpreter/pkg/builtins"
	_ "github.com/oarkflow/interpreter/pkg/builtins/database"
	_ "github.com/oarkflow/interpreter/pkg/builtins/integrations"
	_ "github.com/oarkflow/interpreter/pkg/builtins/reactive"
	_ "github.com/oarkflow/interpreter/pkg/builtins/scheduler"
	_ "github.com/oarkflow/interpreter/pkg/builtins/server"
	_ "github.com/oarkflow/interpreter/pkg/builtins/watcher"
)

func expectTrueResult(t *testing.T, filename string) {
	t.Helper()
	res, err := ExecFile(filename, nil)
	if err != nil {
		t.Fatalf("ExecFile(%s) failed: %v", filename, err)
	}
	boolRes, ok := res.(*Boolean)
	if !ok {
		t.Fatalf("ExecFile(%s) expected Boolean result, got %T (%v)", filename, res, res)
	}
	if !boolRes.Value {
		t.Fatalf("ExecFile(%s) returned false", filename)
	}
}

func TestAdditionalTestdataScripts(t *testing.T) {
	files := []string{
		"testdata/test_strings_additional.spl",
		"testdata/test_time_additional.spl",
		"testdata/test_collections_additional.spl",
		"testdata/test_crypto_additional.spl",
		"testdata/test_methods_parse_additional.spl",
	}
	for _, file := range files {
		expectTrueResult(t, file)
	}
}
