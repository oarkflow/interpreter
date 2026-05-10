package images

import (
	"testing"

	"github.com/oarkflow/interpreter/pkg/eval"
)

func TestImageBuiltinsRegisterOnImport(t *testing.T) {
	for _, name := range []string{"image_load", "image_resize", "image_info", "image_render"} {
		if !eval.HasBuiltin(name) {
			t.Fatalf("expected image builtin %q to be registered", name)
		}
	}
}
