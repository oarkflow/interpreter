package ide

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeJoinRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	// t.TempDir() can itself contain a symlink component (e.g. macOS's
	// /var -> /private/var), and SafeJoin resolves symlinks before
	// comparing, so resolve dir the same way for comparisons below.
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}

	// A request that collapses to a path inside dir is fine.
	if got, err := SafeJoin(dir, "ok.txt"); err != nil || got != filepath.Join(resolvedDir, "ok.txt") {
		t.Fatalf("SafeJoin(ok.txt) = %q, %v", got, err)
	}

	// "../"-heavy paths must collapse to *inside* dir, never escape it -
	// filepath.Clean("/" + p) can never resolve above "/", so the result
	// should land back inside dir rather than erroring or escaping.
	got, err := SafeJoin(dir, "../../../../etc/passwd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != filepath.Join(resolvedDir, "etc/passwd") {
		t.Fatalf("expected traversal to collapse inside project dir, got %q (base %q)", got, resolvedDir)
	}

	// Absolute paths should also resolve inside dir, not to the real root.
	got2, err := SafeJoin(dir, "/etc/passwd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got2 != filepath.Join(resolvedDir, "etc/passwd") {
		t.Fatalf("expected absolute path to be treated as project-relative, got %q", got2)
	}
}

func TestSafeJoinRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "project")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unsupported in this environment: %v", err)
	}

	_, err := SafeJoin(dir, "escape/secret.txt")
	if err == nil {
		t.Fatalf("expected symlink escape to be rejected")
	}
}

func TestFileReadWriteDeleteRename(t *testing.T) {
	dir := t.TempDir()

	if err := WriteFile(dir, "a/b/c.spl", "print 1;"); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	content, err := ReadFile(dir, "a/b/c.spl")
	if err != nil || content != "print 1;" {
		t.Fatalf("ReadFile = %q, %v", content, err)
	}

	tree, err := FileTree(dir)
	if err != nil {
		t.Fatalf("FileTree: %v", err)
	}
	if len(tree.Children) != 1 || tree.Children[0].Name != "a" {
		t.Fatalf("unexpected tree: %+v", tree)
	}

	if err := RenameFile(dir, "a/b/c.spl", "a/b/d.spl"); err != nil {
		t.Fatalf("RenameFile: %v", err)
	}
	if _, err := ReadFile(dir, "a/b/d.spl"); err != nil {
		t.Fatalf("ReadFile after rename: %v", err)
	}
	if _, err := ReadFile(dir, "a/b/c.spl"); err == nil {
		t.Fatalf("expected old path to be gone after rename")
	}

	if err := DeleteFile(dir, "a"); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Fatalf("expected dir removed, err=%v", err)
	}
}

func TestDeleteFileRejectsProjectRoot(t *testing.T) {
	dir := t.TempDir()
	if err := DeleteFile(dir, "."); err == nil {
		t.Fatalf("expected deleting the project root to be rejected")
	}
	if err := DeleteFile(dir, "/"); err == nil {
		t.Fatalf("expected deleting the project root to be rejected")
	}
}

func TestReadFileRejectsBinary(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bin.dat"), []byte{0x00, 0x01, 0xff, 0xfe}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(dir, "bin.dat"); err == nil {
		t.Fatalf("expected binary file to be rejected")
	}
}
