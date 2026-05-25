package repl

import (
	"bytes"
	"io"
	"os"
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
	if !strings.Contains(out, "examples_runtime_workspace.spl") {
		t.Fatalf("unexpected examples output: %q", out)
	}
}
