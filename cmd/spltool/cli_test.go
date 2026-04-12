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
