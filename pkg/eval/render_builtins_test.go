package eval_test

import (
	"strings"
	"testing"

	"github.com/oarkflow/interpreter/pkg/object"
)

func TestRenderBuiltinsCreateArtifacts(t *testing.T) {
	obj := testEval(`image("data:image/png;base64,AA==", {"name": "dot.png", "alt": "dot", "width": 8, "height": 9, "max_bytes": 32});`)
	art, ok := obj.(*object.RenderArtifact)
	if !ok {
		t.Fatalf("expected render artifact, got %T", obj)
	}
	if art.Kind != "image" || art.SourceTyp != "data" || art.Name != "dot.png" || art.Width != 8 || art.Height != 9 || art.MaxBytes != 32 {
		t.Fatalf("unexpected artifact: %#v", art)
	}
}

func TestRenderBuiltinWrapsHTMLString(t *testing.T) {
	obj := testEval(`render("<html><body>Hello</body></html>");`)
	art, ok := obj.(*object.RenderArtifact)
	if !ok {
		t.Fatalf("expected render artifact, got %T", obj)
	}
	if art.Kind != "html" || art.SourceTyp != "data" || art.MIME != "text/html" {
		t.Fatalf("unexpected html artifact: %#v", art)
	}
	if !strings.Contains(art.Inspect(), "<html") {
		t.Fatalf("expected inspect to mention source, got %q", art.Inspect())
	}
}

func TestRenderBuiltinRejectsNonHashOptions(t *testing.T) {
	obj := testEval(`file("x", "bad");`)
	if !object.IsError(obj) {
		t.Fatalf("expected error, got %T: %s", obj, obj.Inspect())
	}
}
