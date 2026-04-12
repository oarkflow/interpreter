package interpreter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplInstallDependencyCreatesManifestAndLock(t *testing.T) {
	projectDir, err := os.MkdirTemp(".", "repl-install-test-")
	if err != nil {
		t.Fatalf("mkdir temp project: %v", err)
	}
	defer os.RemoveAll(projectDir)
	depDir := filepath.Join(projectDir, "deps", "m")
	if err := os.MkdirAll(depDir, 0o755); err != nil {
		t.Fatalf("mkdir dep dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(depDir, "math.spl"), []byte("export let answer = 42;"), 0o600); err != nil {
		t.Fatalf("write dep module: %v", err)
	}

	env := NewGlobalEnvironment(nil)
	env.moduleDir = projectDir
	if err := replInstallDependency("mathlib", "./deps/m", env); err != nil {
		t.Fatalf("install dependency failed: %v", err)
	}

	manifest, err := readModuleManifestFromFile(filepath.Join(projectDir, SPLManifestFileName))
	if err != nil {
		t.Fatalf("read manifest failed: %v", err)
	}
	if manifest.Dependencies["mathlib"] != "./deps/m" {
		t.Fatalf("dependency not recorded in manifest: %#v", manifest.Dependencies)
	}

	lock, err := readModuleLockFromFile(filepath.Join(projectDir, SPLLockFileName))
	if err != nil {
		t.Fatalf("read lock failed: %v", err)
	}
	if _, ok := lock.Dependencies["mathlib"]; !ok {
		t.Fatalf("dependency not recorded in lock: %#v", lock.Dependencies)
	}
}
