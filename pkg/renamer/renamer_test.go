package renamer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenamePreviewAndApply(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "IMG-12.jpg")
	if err := os.WriteFile(src, []byte("photo"), 0o644); err != nil {
		t.Fatal(err)
	}
	rule := Rule{Pattern: "IMG-{number}.jpg", Replacement: "Photo-{number}.jpg", Type: "simple"}
	ops, err := Rename(rule, dir, true, nil)
	if err != nil || len(ops) != 1 || ops[0].Status != "planned" {
		t.Fatalf("preview: ops=%#v err=%v", ops, err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("preview changed source: %v", err)
	}
	ops, err = Rename(rule, dir, false, nil)
	if err != nil || len(ops) != 1 || ops[0].Status != "applied" {
		t.Fatalf("apply: ops=%#v err=%v", ops, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Photo-12.jpg")); err != nil {
		t.Fatalf("renamed file: %v", err)
	}
}

func TestRenameDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "taken.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ops, err := Rename(Rule{Pattern: "a.txt", Replacement: "taken.txt", Type: "simple"}, dir, false, nil)
	if err != nil || len(ops) != 1 || ops[0].Status != "failed" {
		t.Fatalf("ops=%#v err=%v", ops, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "taken.txt"))
	if err != nil || string(data) != "taken.txt" {
		t.Fatalf("destination overwritten: %q %v", data, err)
	}
}

func TestMoveCreatesDestinationDirectory(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	dst := filepath.Join(dir, "nested", "b.txt")
	if err := os.WriteFile(src, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if op := Move(src, dst, false); op.Status != "applied" {
		t.Fatalf("move: %#v", op)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatal(err)
	}
}
