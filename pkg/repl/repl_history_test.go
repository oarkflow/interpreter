package repl

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseHistoryData(t *testing.T) {
	data := []byte("first\n\n second \r\nthird\n")
	got := ParseHistoryData(data)
	want := []string{"first", " second ", "third"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected history parse result: got=%#v want=%#v", got, want)
	}
}

func TestHistoryEntriesToPersist(t *testing.T) {
	history := []string{"loaded", "", "   ", "new-one", "new-two"}
	got := HistoryEntriesToPersist(history, 1)
	want := []string{"new-one", "new-two"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected entries to persist: got=%#v want=%#v", got, want)
	}
}

func TestLoadAndAppendHistoryEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.txt")

	loaded, err := LoadHistoryEntries(path)
	if err != nil {
		t.Fatalf("load missing history returned error: %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("expected empty history for missing file, got %#v", loaded)
	}

	if err := AppendHistoryEntries(path, []string{"cmd1", "", "cmd2"}); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if err := AppendHistoryEntries(path, []string{"cmd3"}); err != nil {
		t.Fatalf("second append failed: %v", err)
	}

	got, err := LoadHistoryEntries(path)
	if err != nil {
		t.Fatalf("load after append failed: %v", err)
	}
	want := []string{"cmd1", "cmd2", "cmd3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected loaded history: got=%#v want=%#v", got, want)
	}
}

func TestAppendHistoryEntriesError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "readonly")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	if err := AppendHistoryEntries(path, []string{"cmd"}); err == nil {
		t.Fatalf("expected append error when path is a directory")
	}
}
