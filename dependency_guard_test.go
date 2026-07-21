//go:build ignore

package interpreter_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestRootImportHasNoThirdPartyDeps(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", "github.com/oarkflow/interpreter")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list dependency guard failed: %v\n%s", err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "github.com/oarkflow/interpreter" || strings.HasPrefix(line, "github.com/oarkflow/interpreter/") {
			continue
		}
		// convert is the interpreter's deliberately selected conversion engine.
		if line == "github.com/oarkflow/convert" || strings.HasPrefix(line, "github.com/oarkflow/convert/") {
			continue
		}
		t.Fatalf("root import includes third-party dependency %q", line)
	}
}
