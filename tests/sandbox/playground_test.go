package interpreter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/oarkflow/interpreter"
)

func TestEvalForPlaygroundCapturesOutputAndResult(t *testing.T) {
	res := EvalForPlayground(`print "hello"; 40 + 2;`, PlaygroundOptions{TimeoutMS: 2000})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if !strings.Contains(res.Output, "hello") {
		t.Fatalf("expected print output, got %q", res.Output)
	}
	if res.Result != "42" {
		t.Fatalf("expected result 42, got %q", res.Result)
	}
	if res.ResultTy != "INTEGER" {
		t.Fatalf("expected INTEGER result type, got %q", res.ResultTy)
	}
}

func TestEvalForPlaygroundReturnsParserErrors(t *testing.T) {
	res := EvalForPlayground(`let x = ;`, PlaygroundOptions{TimeoutMS: 2000})
	if res.Error == "" {
		t.Fatalf("expected parser error")
	}
	if !strings.Contains(res.Error, "Parser error(s):") {
		t.Fatalf("expected friendly parser heading, got %q", res.Error)
	}
	if !strings.Contains(res.Error, "Line") {
		t.Fatalf("expected line details in error, got %q", res.Error)
	}
	if !strings.Contains(res.Error, "Source:") {
		t.Fatalf("expected source context in error, got %q", res.Error)
	}
	if res.ErrorKind != "parser" {
		t.Fatalf("expected parser error kind, got %q", res.ErrorKind)
	}
	if len(res.Diagnostics) == 0 {
		t.Fatalf("expected diagnostics to be populated")
	}
}

func TestEvalForPlaygroundReturnsRenderArtifacts(t *testing.T) {
	res := EvalForPlayground(`print render("<html><body><h1>Hello</h1></body></html>");`, PlaygroundOptions{
		TimeoutMS:    2000,
		RenderConfig: &RenderConfig{Mode: "auto", MaxBytes: 4096},
	})
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if res.Output != "" {
		t.Fatalf("expected render print to be collected, got output %q", res.Output)
	}
	if len(res.Artifacts) != 1 {
		t.Fatalf("expected one artifact, got %#v", res.Artifacts)
	}
	if res.Artifacts[0].Kind != "html" || res.Artifacts[0].MIME != "text/html" || !strings.Contains(res.Artifacts[0].Content, "<h1>Hello</h1>") {
		t.Fatalf("unexpected artifact: %#v", res.Artifacts[0])
	}
}

func TestEvalForPlaygroundURLArtifactsDisabledByDefault(t *testing.T) {
	res := EvalForPlayground(`image("https://example.com/pic.png");`, PlaygroundOptions{
		TimeoutMS:    2000,
		RenderConfig: &RenderConfig{Mode: "auto", MaxBytes: 4096},
	})
	if res.Error != "" {
		t.Fatalf("unexpected runtime error: %s", res.Error)
	}
	if len(res.Artifacts) != 1 || !strings.Contains(res.Artifacts[0].Error, "URL rendering is disabled") {
		t.Fatalf("expected disabled URL artifact error, got %#v", res.Artifacts)
	}
}

func TestEvalForPlaygroundAllowsWorkspaceFileReadsWithProtectHost(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(cwd, "..", ".."))
	path := filepath.Join(repoRoot, "testdata", "test_io.txt")
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd for restore: %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prevWD)
	})
	res := EvalForPlayground(`print file_text(file_load("`+path+`"));`, PlaygroundOptions{
		TimeoutMS: 2000,
		ModuleDir: repoRoot,
		Security: &SecurityPolicy{
			ProtectHost:         true,
			AllowedCapabilities: []string{"filesystem_read"},
			AllowedFileReadPaths: []string{
				repoRoot,
			},
		},
	})
	if res.Error != "" {
		t.Fatalf("unexpected error reading %s: %s", path, res.Error)
	}
	if !strings.Contains(res.Output, "Hello File System!") {
		t.Fatalf("expected file contents in output, got %q", res.Output)
	}
}
