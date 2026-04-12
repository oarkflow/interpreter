package interpreter

import "testing"

func TestRunTestsBuiltinWithPassScript(t *testing.T) {
	result := testEval(`run_tests("testdata/tests/pass_assertions.spl");`)

	hash, ok := result.(*Hash)
	if !ok {
		t.Fatalf("expected Hash result, got %T", result)
	}

	getInt := func(key string) int64 {
		t.Helper()
		k := (&String{Value: key}).HashKey()
		pair, exists := hash.Pairs[k]
		if !exists {
			t.Fatalf("missing key %q in summary", key)
		}
		iv, ok := pair.Value.(*Integer)
		if !ok {
			t.Fatalf("summary key %q is not Integer: %T", key, pair.Value)
		}
		return iv.Value
	}

	if getInt("total") != 3 {
		t.Fatalf("unexpected total")
	}
	if getInt("passed") != 3 {
		t.Fatalf("unexpected passed")
	}
	if getInt("failed") != 0 {
		t.Fatalf("unexpected failed")
	}
}

func TestRunTestsBuiltinWithFailScript(t *testing.T) {
	result := testEval(`run_tests("testdata/tests/fail_assertions.spl");`)

	if !isError(result) {
		t.Fatalf("expected error result, got %T", result)
	}
}

func TestImportFromTestdataFiles(t *testing.T) {
	result := testEval(`import "testdata/modules/math.spl"; base + increment;`)
	testIntegerObject(t, result, 42)
}

func TestImportSelectiveFromSyntax(t *testing.T) {
	result := testEval(`import {base, increment} from "testdata/modules/math.spl"; base + increment;`)
	testIntegerObject(t, result, 42)
}

func TestImportWildcardAliasSyntax(t *testing.T) {
	result := testEval(`import * as math from "testdata/modules/math.spl"; math.base + math.increment;`)
	testIntegerObject(t, result, 42)
}

func TestExecFileRelativeImportsFromTestdata(t *testing.T) {
	result, err := ExecFile("testdata/modules/entry_relative_import.spl", nil)
	if err != nil {
		t.Fatalf("ExecFile failed: %v", err)
	}
	testIntegerObject(t, result, 42)
}
