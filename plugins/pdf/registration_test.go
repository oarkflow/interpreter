package pdf

import (
	"testing"

	"github.com/oarkflow/interpreter/pkg/eval"
)

func TestPDFBuiltinsRegisterOnImport(t *testing.T) {
	for _, name := range []string{
		"pdf_info", "pdf_validate", "pdf_merge", "pdf_split", "pdf_delete_pages",
		"pdf_reorder", "pdf_rotate", "pdf_compress", "pdf_protect", "pdf_decrypt",
		"pdf_watermark", "pdf_add_page_numbers", "pdf_set_metadata", "pdf_stamp_image",
		"pdf_to_text", "pdf_to_html", "pdf_to_markdown", "pdf_to_json", "pdf_search",
		"pdf_extract_images", "pdf_from_html", "pdf_from_markdown", "pdf_from_url",
		"pdf_images_to_pdf", "pdf_fill_form", "pdf_list_form_fields", "pdf_quick",
	} {
		if !eval.HasBuiltin(name) {
			t.Fatalf("expected pdf builtin %q to be registered", name)
		}
	}
}
