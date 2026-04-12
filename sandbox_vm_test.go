package interpreter

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultSandboxConfigValues(t *testing.T) {
	execCfg := DefaultExecSandboxConfig()
	if !execCfg.Enabled {
		t.Fatalf("expected exec sandbox enabled by default")
	}
	if execCfg.MaxDepth <= 0 || execCfg.MaxSteps <= 0 || execCfg.MaxHeapMB <= 0 || execCfg.Timeout <= 0 {
		t.Fatalf("expected bounded defaults, got %#v", execCfg)
	}

	replCfg := DefaultReplSandboxConfig()
	if !replCfg.Enabled {
		t.Fatalf("expected repl sandbox enabled by default")
	}
	if !replCfg.StrictMode || !replCfg.ProtectHost {
		t.Fatalf("expected strict/protect host defaults for repl, got %#v", replCfg)
	}
}

func TestNewSandboxVMBindsModuleDirAsPolicyRoot(t *testing.T) {
	dir := t.TempDir()
	vm, err := NewSandboxVM(nil, "<memory>", dir, SandboxConfig{
		Enabled:       true,
		StrictMode:    true,
		ProtectHost:   true,
		AllowEnvWrite: false,
		MaxDepth:      10,
		MaxSteps:      1000,
		MaxHeapMB:     32,
		Timeout:       time.Second,
	})
	if err != nil {
		t.Fatalf("new sandbox vm failed: %v", err)
	}
	if vm.Environment().moduleDir != dir {
		t.Fatalf("unexpected module dir: got=%q want=%q", vm.Environment().moduleDir, dir)
	}
	if vm.Policy() == nil {
		t.Fatalf("expected security policy")
	}
	if len(vm.Policy().AllowedFileReadPaths) == 0 || filepath.Clean(vm.Policy().AllowedFileReadPaths[0]) != filepath.Clean(dir) {
		t.Fatalf("expected read path rooted at module dir, got %#v", vm.Policy().AllowedFileReadPaths)
	}
}

func TestExecWithSandboxPolicyDeniesExec(t *testing.T) {
	_, err := ExecWithOptions(`exec("echo", "hi")`, nil, ExecOptions{Sandbox: &SandboxConfig{
		Enabled:       true,
		StrictMode:    true,
		ProtectHost:   true,
		AllowEnvWrite: false,
		MaxDepth:      64,
		MaxSteps:      10000,
		MaxHeapMB:     64,
		Timeout:       2 * time.Second,
		BaseDir:       ".",
	}})
	if err == nil {
		t.Fatalf("expected runtime error")
	}
	if !strings.Contains(err.Error(), "exec denied") {
		t.Fatalf("expected exec denied error, got %v", err)
	}
}
