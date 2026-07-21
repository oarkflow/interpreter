package repl

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	fn()

	_ = w.Close()
	os.Stdout = orig

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	return buf.String()
}

func TestFileMetaCommandsPreviewAndApply(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "IMG-7.jpg")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() { HandleReplMetaCommand(":rename 'IMG-{number}.jpg' 'Photo-{number}.jpg' '"+dir+"'", nil, nil) })
	if !strings.Contains(out, "planned rename") {
		t.Fatalf("preview output: %q", out)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("preview changed file: %v", err)
	}
	captureStdout(t, func() {
		HandleReplMetaCommand(":rename 'IMG-{number}.jpg' 'Photo-{number}.jpg' '"+dir+"' --apply", nil, nil)
	})
	renamed := filepath.Join(dir, "Photo-7.jpg")
	if _, err := os.Stat(renamed); err != nil {
		t.Fatalf("rename was not applied: %v", err)
	}
	dst := filepath.Join(dir, "moved", "Photo-7.jpg")
	captureStdout(t, func() { HandleReplMetaCommand(":move '"+renamed+"' '"+dst+"' --apply", nil, nil) })
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("move was not applied: %v", err)
	}
}

func TestReplPrintLineStartsWithCR(t *testing.T) {
	out := captureStdout(t, func() {
		ReplPrintLine("hello")
	})
	if !strings.HasPrefix(out, "\rhello") {
		t.Fatalf("expected output to start with CR, got %q", out)
	}
}

func TestHelpMetaCommandOutputUsesCRPerLine(t *testing.T) {
	out := captureStdout(t, func() {
		handled := HandleReplMetaCommand(":help", nil, nil)
		if !handled {
			t.Fatalf(":help was not handled")
		}
	})

	if !strings.Contains(out, "\rInteractive features:") {
		t.Fatalf("missing CR-prefixed heading: %q", out)
	}
	if !strings.Contains(out, "\r- Arrow keys: history and cursor movement") {
		t.Fatalf("missing CR-prefixed bullet: %q", out)
	}
	if !strings.Contains(out, ":debug <expr>") || !strings.Contains(out, ":mem") || !strings.Contains(out, ":install <alias> <path>") || !strings.Contains(out, ":config set <key> <value>") {
		t.Fatalf("missing newly documented commands: %q", out)
	}
	if !strings.Contains(out, "Alt+Left") || !strings.Contains(out, "Ctrl+U/Ctrl+K/Ctrl+W") || !strings.Contains(out, ":palette <query>") {
		t.Fatalf("missing navigation/discovery help: %q", out)
	}
}

func TestDiscoveryMetaCommands(t *testing.T) {
	out := captureStdout(t, func() {
		if !HandleReplMetaCommand(":commands checkpoint", nil, nil) {
			t.Fatalf(":commands was not handled")
		}
	})
	if !strings.Contains(out, ":checkpoint") || !strings.Contains(out, "save a session checkpoint") {
		t.Fatalf("unexpected commands output: %q", out)
	}

	out = captureStdout(t, func() {
		if !HandleReplMetaCommand(":tips", nil, nil) {
			t.Fatalf(":tips was not handled")
		}
	})
	if !strings.Contains(out, "Press Tab") || !strings.Contains(out, ":palette") {
		t.Fatalf("unexpected tips output: %q", out)
	}

	out = captureStdout(t, func() {
		if !HandleReplMetaCommand(":examples", nil, nil) {
			t.Fatalf(":examples was not handled")
		}
	})
	if !strings.Contains(out, "examples/all_in_one.spl") {
		t.Fatalf("unexpected examples output: %q", out)
	}

	out = captureStdout(t, func() {
		if !HandleReplMetaCommand(":tools", nil, nil) {
			t.Fatalf(":tools was not handled")
		}
	})
	if !strings.Contains(out, "tools/files") || !strings.Contains(out, "bulk_rename") || !strings.Contains(out, "ffmpeg_status") || !strings.Contains(out, "native/os") || !strings.Contains(out, "os.run") {
		t.Fatalf("unexpected tools output: %q", out)
	}
}
