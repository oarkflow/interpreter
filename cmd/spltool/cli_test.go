package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCheckJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"check", "--json"}, strings.NewReader("let x = ;"), &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	if !strings.Contains(stdout.String(), "\"diagnostics\"") {
		t.Fatalf("expected JSON diagnostics, got %q", stdout.String())
	}
}

func TestRunFmtWritesCanonicalOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"fmt"}, strings.NewReader("let x=1;"), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("unexpected exit code: %d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "let x") {
		t.Fatalf("expected formatted output, got %q", stdout.String())
	}
}

func TestRunModInitAndTidy(t *testing.T) {
	projectDir, err := os.MkdirTemp(".", "spltool-mod-test-")
	if err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}
	projectDir, err = filepath.Abs(projectDir)
	if err != nil {
		t.Fatalf("failed to make project dir absolute: %v", err)
	}
	defer os.RemoveAll(projectDir)

	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	defer os.Chdir(prev)
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"mod", "init", "example/app"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("unexpected init exit code: %d stderr=%q", code, stderr.String())
	}
	manifestPath := filepath.Join(projectDir, "spl.mod")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("expected manifest file: %v", err)
	}

	depDir := filepath.Join(projectDir, "deps", "demo")
	if err := os.MkdirAll(depDir, 0o755); err != nil {
		t.Fatalf("failed to create dep dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "lib.spl"), []byte("export let value = 1;"), 0o600); err != nil {
		t.Fatalf("failed to write dep module: %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte("{\n  \"module\": \"example/app\",\n  \"dependencies\": {\n    \"demo\": \"./deps/demo\"\n  }\n}\n"), 0o600); err != nil {
		t.Fatalf("failed to update manifest: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"mod", "tidy"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("unexpected tidy exit code: %d stderr=%q", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(projectDir, "spl.lock")); err != nil {
		t.Fatalf("expected lock file: %v", err)
	}
}

func TestRunConfigInitSymbolsCompleteHoverDocsAndTest(t *testing.T) {
	projectDir, err := os.MkdirTemp(".", "spltool-dx-test-")
	if err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}
	projectDir, err = filepath.Abs(projectDir)
	if err != nil {
		t.Fatalf("abs project dir: %v", err)
	}
	defer os.RemoveAll(projectDir)
	srcPath := filepath.Join(projectDir, "math_test.spl")
	src := `let add = function(a, b) { return a + b; };
test "adds" { assert_eq(add(1, 2), 3); }
`
	if err := os.WriteFile(srcPath, []byte(src), 0o600); err != nil {
		t.Fatalf("write test source: %v", err)
	}

	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(prev)
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"config", "init"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("config init failed: code=%d stderr=%q", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(projectDir, "spl.config.json")); err != nil {
		t.Fatalf("expected config file: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"symbols", srcPath}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("symbols failed: code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"name": "add"`) {
		t.Fatalf("expected add symbol, got %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"complete", "--prefix", "ad", srcPath}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("complete failed: code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"label": "add"`) {
		t.Fatalf("expected add completion, got %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"hover", "--line", "1", "--col", "5", srcPath}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("hover failed: code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"name": "add"`) {
		t.Fatalf("expected add hover, got %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"docs", srcPath}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("docs failed: code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "# math_test.spl") || !strings.Contains(stdout.String(), "`add`") {
		t.Fatalf("unexpected docs: %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"test", "--json", srcPath}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("test failed: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"passed": 1`) {
		t.Fatalf("expected one passing test file, got %q", stdout.String())
	}
}
