package lua

import "testing"

func TestCompileReportsUnclosedTableAtEOF(t *testing.T) {
	_, err := Compile("\n  local a = {4\n\n", "chunk")
	if err == nil {
		t.Fatal("expected syntax error")
	}
}
