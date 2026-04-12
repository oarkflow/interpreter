package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oarkflow/interpreter"
)

func TestCLIImportsTemplateRuntime(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "render_check.spl")
	tmplDir := filepath.Join(dir, "views")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmplDir, "page.html"), []byte(`<h1>${title}</h1>`), 0644); err != nil {
		t.Fatal(err)
	}
	script := `
let app = web_app("` + tmplDir + `");
let runtime = app;
let has_templates = runtime != null;
has_templates;
`
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	res, err := interpreter.ExecFile(scriptPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res != interpreter.TRUE {
		t.Fatalf("expected true result, got %v", res)
	}
}
