package interpreter_test

import (
	"testing"

	. "github.com/oarkflow/interpreter"

	_ "github.com/oarkflow/interpreter/pkg/builtins"
	_ "github.com/oarkflow/interpreter/pkg/builtins/reactive"
	_ "github.com/oarkflow/interpreter/pkg/builtins/scheduler"
	_ "github.com/oarkflow/interpreter/pkg/builtins/watcher"
)

func TestAllInOneExampleExecutes(t *testing.T) {
	if _, err := ExecFile("examples/all_in_one.spl", nil); err != nil {
		t.Fatalf("all-in-one example failed: %v", err)
	}
}
