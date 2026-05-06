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
}
