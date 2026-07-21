package ide

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryCreateListDeleteRoundTrip(t *testing.T) {
	root := t.TempDir()
	reg, err := NewRegistry(root)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	p, err := reg.Create("My App", ScaffoldMinimal, func(dir string) error {
		return os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("hi"), 0o644)
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.Slug != "my-app" {
		t.Fatalf("expected slug my-app, got %q", p.Slug)
	}
	if _, err := os.Stat(filepath.Join(root, p.Dir, "marker.txt")); err != nil {
		t.Fatalf("expected scaffold side effect on disk: %v", err)
	}

	// Reload from disk to verify persistence.
	reg2, err := NewRegistry(root)
	if err != nil {
		t.Fatalf("reload NewRegistry: %v", err)
	}
	list := reg2.List()
	if len(list) != 1 || list[0].ID != p.ID {
		t.Fatalf("expected reloaded registry to contain the created project, got %+v", list)
	}

	got, ok := reg2.Get(p.ID)
	if !ok || got.Name != "My App" {
		t.Fatalf("Get after reload: ok=%v got=%+v", ok, got)
	}

	if err := reg2.Delete(p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, p.Dir)); !os.IsNotExist(err) {
		t.Fatalf("expected project dir removed, stat err=%v", err)
	}
	if len(reg2.List()) != 0 {
		t.Fatalf("expected empty registry after delete")
	}
}

func TestRegistrySlugCollisionIsDeduplicated(t *testing.T) {
	root := t.TempDir()
	reg, err := NewRegistry(root)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	noop := func(dir string) error { return nil }

	p1, err := reg.Create("Todo App", ScaffoldMinimal, noop)
	if err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	p2, err := reg.Create("Todo App", ScaffoldMinimal, noop)
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	if p1.Dir == p2.Dir {
		t.Fatalf("expected distinct dirs for colliding slugs, both got %q", p1.Dir)
	}
}

func TestRegistryCreateRollsBackOnScaffoldError(t *testing.T) {
	root := t.TempDir()
	reg, err := NewRegistry(root)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	_, err = reg.Create("Broken", ScaffoldMinimal, func(dir string) error {
		return os.ErrInvalid
	})
	if err == nil {
		t.Fatalf("expected error from failing scaffold func")
	}
	if len(reg.List()) != 0 {
		t.Fatalf("expected no project registered after scaffold failure")
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.Name() == ".spl-ide" {
			continue
		}
		t.Fatalf("expected project dir to be cleaned up, found %q", e.Name())
	}
}
