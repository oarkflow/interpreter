package tools

import (
	"testing"

	"github.com/oarkflow/interpreter/pkg/eval"
)

func TestToolsBuiltinsRegisterOnImport(t *testing.T) {
	for _, name := range []string{"bulk_rename", "file_organize", "file_checksum", "archive_compress", "image_convert_batch", "image_resize_file", "office_read", "secret_generate", "media_info", "ffmpeg_status"} {
		if !eval.HasBuiltin(name) {
			t.Fatalf("expected tools builtin %q to be registered", name)
		}
	}
}
