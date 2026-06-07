package tools

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func names(files []FileInfo) map[string]FileInfo {
	out := map[string]FileInfo{}
	for _, file := range files {
		out[file.Name] = file
	}
	return out
}

func TestSearchExtendedFilters(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "alpha.txt"), "hello finder")
	writeTestFile(t, filepath.Join(dir, "beta.log"), "log data")
	writeTestFile(t, filepath.Join(dir, "nested", "gamma.txt"), "deep finder")

	files, err := Search(dir, map[string]any{"ext": "txt", "content": "finder", "sort": "name"}, Hooks{})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(files) != 2 || files[0].Name != "alpha.txt" || files[1].Name != "gamma.txt" {
		t.Fatalf("unexpected txt content results: %#v", files)
	}

	files, err = Search(dir, map[string]any{"include_dirs": true, "type": "any", "name": "nested"}, Hooks{})
	if err != nil {
		t.Fatalf("directory search failed: %v", err)
	}
	found := names(files)
	if !found["nested"].IsDir {
		t.Fatalf("expected nested directory in results: %#v", files)
	}

	files, err = Search(dir, map[string]any{"recursive": false, "match": "*.txt"}, Hooks{})
	if err != nil {
		t.Fatalf("non-recursive search failed: %v", err)
	}
	if len(files) != 1 || files[0].Name != "alpha.txt" {
		t.Fatalf("expected only root txt file, got %#v", files)
	}
}

func TestSearchSizeTimeDepthSortAndLimit(t *testing.T) {
	dir := t.TempDir()
	small := filepath.Join(dir, "small.txt")
	large := filepath.Join(dir, "large.txt")
	deep := filepath.Join(dir, "nested", "deep.txt")
	writeTestFile(t, small, "abc")
	writeTestFile(t, large, "abcdefghijklmnopqrstuvwxyz")
	writeTestFile(t, deep, "deep")

	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(small, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	files, err := Search(dir, map[string]any{"min_size": int64(10), "sort": "size", "desc": true}, Hooks{})
	if err != nil {
		t.Fatalf("size search failed: %v", err)
	}
	if len(files) != 1 || files[0].Name != "large.txt" {
		t.Fatalf("expected only large file, got %#v", files)
	}

	files, err = Search(dir, map[string]any{"modified_after": time.Now().Add(-time.Hour).Unix(), "max_depth": 1, "sort": "name", "limit": 1}, Hooks{})
	if err != nil {
		t.Fatalf("time/depth search failed: %v", err)
	}
	if len(files) != 1 || files[0].Name != "large.txt" {
		t.Fatalf("expected limited newest root file, got %#v", files)
	}
}

func TestSearchRegexPatterns(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "alpha-001.txt"), "status: ready\nowner: Ada")
	writeTestFile(t, filepath.Join(dir, "beta-002.log"), "status: waiting\nowner: Linus")
	writeTestFile(t, filepath.Join(dir, "nested", "gamma-003.txt"), "status: ready\nowner: Grace")

	files, err := Search(dir, map[string]any{"regex": `^alpha-\d+\.txt$`}, Hooks{})
	if err != nil {
		t.Fatalf("filename regex search failed: %v", err)
	}
	if len(files) != 1 || files[0].Name != "alpha-001.txt" {
		t.Fatalf("unexpected filename regex results: %#v", files)
	}

	files, err = Search(dir, map[string]any{"match": `.*-\d+\.txt$`, "pattern_type": "regex", "sort": "name"}, Hooks{})
	if err != nil {
		t.Fatalf("match regex search failed: %v", err)
	}
	if len(files) != 2 || files[0].Name != "alpha-001.txt" || files[1].Name != "gamma-003.txt" {
		t.Fatalf("unexpected match regex results: %#v", files)
	}

	files, err = Search(dir, map[string]any{"path_regex": `/nested/`, "content_regex": `owner:\s+Grace`}, Hooks{})
	if err != nil {
		t.Fatalf("path/content regex search failed: %v", err)
	}
	if len(files) != 1 || files[0].Name != "gamma-003.txt" {
		t.Fatalf("unexpected path/content regex results: %#v", files)
	}
}
