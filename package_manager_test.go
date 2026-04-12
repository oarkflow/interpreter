package interpreter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPackageManifestLockResolvesBareImports(t *testing.T) {
	projectDir, err := os.MkdirTemp(".", "package-manager-test-")
	if err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}
	defer os.RemoveAll(projectDir)

	depDir := filepath.Join(projectDir, "deps", "mathlib")
	if err := os.MkdirAll(depDir, 0o755); err != nil {
		t.Fatalf("failed to create dependency dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "math.spl"), []byte("export let answer = 42;"), 0o600); err != nil {
		t.Fatalf("failed to write dependency module: %v", err)
	}

	manifest := &SPLModuleManifest{
		Module: "example/app",
		Dependencies: map[string]string{
			"mathlib": "./deps/mathlib",
		},
	}
	if err := writeModuleManifest(projectDir, manifest); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	lock, err := SyncModuleLock(projectDir)
	if err != nil {
		t.Fatalf("failed to sync lock: %v", err)
	}
	dep, ok := lock.Dependencies["mathlib"]
	if !ok {
		t.Fatalf("expected mathlib dependency in lock")
	}
	if dep.Checksum == "" || dep.ResolvedPath == "" {
		t.Fatalf("expected resolved dependency metadata, got %#v", dep)
	}

	appPath := filepath.Join(projectDir, "main.spl")
	if err := os.WriteFile(appPath, []byte(`import "mathlib/math.spl" as math; math.answer;`), 0o600); err != nil {
		t.Fatalf("failed to write app: %v", err)
	}

	result, err := ExecFile(appPath, nil)
	if err != nil {
		t.Fatalf("ExecFile failed: %v", err)
	}
	testIntegerObject(t, result, 42)
}
