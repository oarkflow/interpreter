package pdf

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oarkflow/interpreter/pkg/object"
)

func str(s string) *object.String { return &object.String{Value: s} }

// chdirTemp creates a fresh temp dir and chdirs into it for the duration of
// the test, restoring the previous working directory on cleanup.
// SanitizePathLocal (used by checkRead/checkWrite) jails relative/absolute
// paths to the active sandbox root, falling back to the process's current
// working directory when no sandbox override is active - so PDF file I/O
// in these tests needs cwd to actually be the temp dir, not just an
// unrelated os.TempDir() path.
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	return dir
}

func requireOK(t *testing.T, result object.Object) {
	t.Helper()
	if errObj, ok := result.(*object.Error); ok {
		t.Fatalf("expected success, got error: %s", errObj.Message)
	}
}

func TestPDFQuickInfoAndText(t *testing.T) {
	dir := chdirTemp(t)
	out := filepath.Join(dir, "quick.pdf")

	requireOK(t, fnQuick(str("Hello from the PDF builtins test"), str(out)))
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected pdf_quick to create a file: %v", err)
	}

	info := fnInfo(str(out))
	requireOK(t, info)
	hash, ok := info.(*object.Hash)
	if !ok {
		t.Fatalf("expected pdf_info to return a HASH, got %T", info)
	}
	pagesKey := (&object.String{Value: "pages"}).HashKey()
	pair, ok := hash.Pairs[pagesKey]
	if !ok {
		t.Fatalf("expected pdf_info result to contain a pages field, got %s", hash.Inspect())
	}
	pagesInt, ok := pair.Value.(*object.Integer)
	if !ok || pagesInt.Value < 1 {
		t.Fatalf("expected at least one page, got %s", pair.Value.Inspect())
	}

	text := fnToText(str(out))
	requireOK(t, text)
	textStr, ok := text.(*object.String)
	if !ok {
		t.Fatalf("expected pdf_to_text to return a STRING, got %T", text)
	}
	if !strings.Contains(textStr.Value, "Hello from the PDF builtins test") {
		t.Fatalf("expected extracted text to contain the original content, got %q", textStr.Value)
	}
}

func TestPDFMergeAndValidate(t *testing.T) {
	dir := chdirTemp(t)
	a := filepath.Join(dir, "a.pdf")
	b := filepath.Join(dir, "b.pdf")
	merged := filepath.Join(dir, "merged.pdf")

	requireOK(t, fnQuick(str("Document A"), str(a)))
	requireOK(t, fnQuick(str("Document B"), str(b)))
	requireOK(t, fnMerge(str(merged), str(a), str(b)))

	validation := fnValidate(str(merged))
	requireOK(t, validation)
	hash, ok := validation.(*object.Hash)
	if !ok {
		t.Fatalf("expected pdf_validate to return a HASH, got %T", validation)
	}
	validKey := (&object.String{Value: "valid"}).HashKey()
	pagesKey := (&object.String{Value: "pages"}).HashKey()
	validPair, ok := hash.Pairs[validKey]
	if !ok {
		t.Fatalf("expected valid field in %s", hash.Inspect())
	}
	if b, ok := validPair.Value.(*object.Boolean); !ok || !b.Value {
		t.Fatalf("expected merged PDF to validate as valid, got %s", hash.Inspect())
	}
	pagesPair, ok := hash.Pairs[pagesKey]
	if !ok {
		t.Fatalf("expected pages field in %s", hash.Inspect())
	}
	pagesInt, ok := pagesPair.Value.(*object.Integer)
	if !ok || pagesInt.Value != 2 {
		t.Fatalf("expected merged PDF to have 2 pages, got %s", pagesPair.Value.Inspect())
	}
}

func TestPDFSplitAndRotate(t *testing.T) {
	dir := chdirTemp(t)
	merged := filepath.Join(dir, "merged.pdf")
	a := filepath.Join(dir, "a.pdf")
	b := filepath.Join(dir, "b.pdf")
	requireOK(t, fnQuick(str("Page one"), str(a)))
	requireOK(t, fnQuick(str("Page two"), str(b)))
	requireOK(t, fnMerge(str(merged), str(a), str(b)))

	firstPageOnly := filepath.Join(dir, "first.pdf")
	requireOK(t, fnSplit(str(merged), str(firstPageOnly), str("1")))
	info := fnInfo(str(firstPageOnly))
	requireOK(t, info)
	hash := info.(*object.Hash)
	pagesKey := (&object.String{Value: "pages"}).HashKey()
	if pagesInt, ok := hash.Pairs[pagesKey].Value.(*object.Integer); !ok || pagesInt.Value != 1 {
		t.Fatalf("expected split output to have exactly 1 page, got %s", hash.Inspect())
	}

	rotated := filepath.Join(dir, "rotated.pdf")
	requireOK(t, fnRotate(str(merged), str(rotated), str("1"), &object.Integer{Value: 90}))
	if _, err := os.Stat(rotated); err != nil {
		t.Fatalf("expected pdf_rotate to create a file: %v", err)
	}
}

func TestPDFProtectAndDecryptDefaultAlgorithm(t *testing.T) {
	dir := chdirTemp(t)
	plain := filepath.Join(dir, "plain.pdf")
	protected := filepath.Join(dir, "protected.pdf")
	decrypted := filepath.Join(dir, "decrypted.pdf")
	requireOK(t, fnQuick(str("Sensitive content"), str(plain)))

	// No algorithm argument -> defaults to AES-128 for backward
	// compatibility with existing callers.
	requireOK(t, fnProtect(str(plain), str(protected), str("user-pw"), str("owner-pw")))

	info := fnInfo(str(protected), str("user-pw"))
	requireOK(t, info)
	hash := info.(*object.Hash)
	encKey := (&object.String{Value: "encrypted"}).HashKey()
	if b, ok := hash.Pairs[encKey].Value.(*object.Boolean); !ok || !b.Value {
		t.Fatalf("expected protected.pdf to report encrypted=true, got %s", hash.Inspect())
	}

	requireOK(t, fnDecrypt(str(protected), str(decrypted), str("user-pw")))
	decInfo := fnInfo(str(decrypted))
	requireOK(t, decInfo)
	decHash := decInfo.(*object.Hash)
	if b, ok := decHash.Pairs[encKey].Value.(*object.Boolean); !ok || b.Value {
		t.Fatalf("expected decrypted.pdf to report encrypted=false, got %s", decHash.Inspect())
	}
}

// TestPDFProtectAndDecryptAES256 is a regression test: pdf_protect(...,
// "aes-256") used to fail with "AES-256 ... is not supported yet" from
// github.com/oarkflow/pdf's Protect(); AES-256 (Standard Security Handler
// revision 5) is now implemented end-to-end in that library (v0.0.4+).
func TestPDFProtectAndDecryptAES256(t *testing.T) {
	dir := chdirTemp(t)
	plain := filepath.Join(dir, "plain256.pdf")
	protected := filepath.Join(dir, "protected256.pdf")
	decrypted := filepath.Join(dir, "decrypted256.pdf")
	requireOK(t, fnQuick(str("Sensitive AES-256 content"), str(plain)))

	requireOK(t, fnProtect(str(plain), str(protected), str("user-pw-256"), str("owner-pw-256"), str("aes-256")))

	info := fnInfo(str(protected), str("user-pw-256"))
	requireOK(t, info)
	hash := info.(*object.Hash)
	encKey := (&object.String{Value: "encrypted"}).HashKey()
	if b, ok := hash.Pairs[encKey].Value.(*object.Boolean); !ok || !b.Value {
		t.Fatalf("expected protected256.pdf to report encrypted=true, got %s", hash.Inspect())
	}

	requireOK(t, fnDecrypt(str(protected), str(decrypted), str("user-pw-256")))
	decInfo := fnInfo(str(decrypted))
	requireOK(t, decInfo)
	decHash := decInfo.(*object.Hash)
	if b, ok := decHash.Pairs[encKey].Value.(*object.Boolean); !ok || b.Value {
		t.Fatalf("expected decrypted256.pdf to report encrypted=false, got %s", decHash.Inspect())
	}
}

func TestPDFToDocx(t *testing.T) {
	dir := chdirTemp(t)
	src := filepath.Join(dir, "source.pdf")
	out := filepath.Join(dir, "converted.docx")

	requireOK(t, fnQuick(str("Hello from the PDF to DOCX test"), str(src)))

	requireOK(t, fnToDocx(str(src), str(out)))
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("expected pdf_to_docx to create a file: %v", err)
	}
	if len(data) < 4 || string(data[:2]) != "PK" {
		t.Fatalf("expected a valid DOCX (zip) file, got %d bytes starting %q", len(data), data[:min(4, len(data))])
	}
}

func TestPDFMarkdownToDocx(t *testing.T) {
	dir := chdirTemp(t)
	out := filepath.Join(dir, "notes.docx")

	requireOK(t, fnMarkdownToDocx(str("# Title\n\nSome **bold** text."), str(out), &object.Hash{Pairs: map[object.HashKey]object.HashPair{
		(&object.String{Value: "title"}).HashKey(): {
			Key:   &object.String{Value: "title"},
			Value: &object.String{Value: "Notes"},
		},
	}}))
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("expected pdf_markdown_to_docx to create a file: %v", err)
	}
	if len(data) < 4 || string(data[:2]) != "PK" {
		t.Fatalf("expected a valid DOCX (zip) file, got %d bytes starting %q", len(data), data[:min(4, len(data))])
	}
}

func TestPDFToDocxArgumentValidationErrors(t *testing.T) {
	if _, ok := fnToDocx(str("only-one-arg.pdf")).(*object.Error); !ok {
		t.Fatalf("expected pdf_to_docx with only one argument to return an error")
	}
	if _, ok := fnMarkdownToDocx(str("only-one-arg")).(*object.Error); !ok {
		t.Fatalf("expected pdf_markdown_to_docx with only one argument to return an error")
	}
	if _, ok := fnMarkdownToDocx(str(""), str("out.docx")).(*object.Error); !ok {
		t.Fatalf("expected pdf_markdown_to_docx with empty markdown to return an error")
	}
}

func TestDocxToMarkdownRoundTrip(t *testing.T) {
	dir := chdirTemp(t)
	docxPath := filepath.Join(dir, "roundtrip.docx")

	// Note: github.com/oarkflow/pdf/md's DOCX exporter itself renders
	// paragraph text as plain text (its plainInline strips **/*/etc. rather
	// than emitting separate bold/italic <w:r> runs) - so bold/italic can't
	// round-trip through pdf_markdown_to_docx -> pdf_docx_to_markdown.
	// TestDocxImportPreservesRunFormatting below feeds the importer a
	// hand-built DOCX with real formatted runs (as a genuine Word document
	// would contain) to verify bold/italic detection independently of that
	// upstream limitation.
	source := "# Report Title\n\n" +
		"## Highlights\n\n" +
		"Some bold and italic text.\n\n" +
		"- first bullet\n" +
		"- second bullet\n\n" +
		"1. step one\n" +
		"2. step two\n"

	requireOK(t, fnMarkdownToDocx(str(source), str(docxPath), &object.Hash{Pairs: map[object.HashKey]object.HashPair{
		(&object.String{Value: "title"}).HashKey(): {
			Key:   &object.String{Value: "title"},
			Value: &object.String{Value: "Report"},
		},
	}}))

	mdResult := fnDocxToMarkdown(str(docxPath))
	requireOK(t, mdResult)
	md, ok := mdResult.(*object.String)
	if !ok {
		t.Fatalf("expected pdf_docx_to_markdown to return a STRING, got %T", mdResult)
	}
	for _, want := range []string{
		"# Report Title",
		"## Highlights",
		"Some bold and italic text.",
		"- first bullet",
		"- second bullet",
		"1. step one",
		"1. step two", // ordered items are both rendered "1." (renumbered by any real Markdown renderer)
	} {
		if !strings.Contains(md.Value, want) {
			t.Fatalf("expected round-tripped markdown to contain %q, got:\n%s", want, md.Value)
		}
	}

	textResult := fnDocxToText(str(docxPath))
	requireOK(t, textResult)
	text, ok := textResult.(*object.String)
	if !ok {
		t.Fatalf("expected pdf_docx_to_text to return a STRING, got %T", textResult)
	}
	if !strings.Contains(text.Value, "Report Title") || !strings.Contains(text.Value, "bold") {
		t.Fatalf("expected pdf_docx_to_text to preserve the underlying words, got: %s", text.Value)
	}
}

// syntheticWordDocx builds a minimal but real DOCX (zip of OOXML parts), the
// way a document.xml authored by actual Microsoft Word would look, to
// verify the importer against formatting our own limited exporter never
// produces: separate bold/italic runs and numPr-only list paragraphs (no
// distinguishing pStyle) resolved through numbering.xml.
func syntheticWordDocx(t *testing.T, path string) {
	t.Helper()
	const documentXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Formatting Demo</w:t></w:r></w:p>
<w:p><w:r><w:t xml:space="preserve">Some </w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t>bold</w:t></w:r><w:r><w:t xml:space="preserve"> and </w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t>italic</w:t></w:r><w:r><w:t xml:space="preserve"> and </w:t></w:r><w:r><w:rPr><w:b/><w:i/></w:rPr><w:t>both</w:t></w:r><w:r><w:t>.</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="ListParagraph"/><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr><w:r><w:t>bulleted via numbering.xml only</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="ListParagraph"/><w:numPr><w:ilvl w:val="0"/><w:numId w:val="2"/></w:numPr></w:pPr><w:r><w:t>numbered via numbering.xml only</w:t></w:r></w:p>
</w:body></w:document>`
	const numberingXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:abstractNum w:abstractNumId="10"><w:lvl w:ilvl="0"><w:numFmt w:val="bullet"/></w:lvl></w:abstractNum>
<w:abstractNum w:abstractNumId="20"><w:lvl w:ilvl="0"><w:numFmt w:val="decimal"/></w:lvl></w:abstractNum>
<w:num w:numId="1"><w:abstractNumId w:val="10"/></w:num>
<w:num w:numId="2"><w:abstractNumId w:val="20"/></w:num>
</w:numbering>`

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create synthetic docx: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	write := func(name, body string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("word/document.xml", documentXML)
	write("word/numbering.xml", numberingXML)
	if err := zw.Close(); err != nil {
		t.Fatalf("close synthetic docx: %v", err)
	}
}

func TestDocxImportPreservesRunFormatting(t *testing.T) {
	dir := chdirTemp(t)
	docxPath := filepath.Join(dir, "formatting.docx")
	syntheticWordDocx(t, docxPath)

	mdResult := fnDocxToMarkdown(str(docxPath))
	requireOK(t, mdResult)
	md := mdResult.(*object.String).Value

	for _, want := range []string{
		"# Formatting Demo",
		"**bold**",
		"*italic*",
		"***both***",
		"- bulleted via numbering.xml only",
		"1. numbered via numbering.xml only",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("expected imported markdown to contain %q, got:\n%s", want, md)
		}
	}

	textResult := fnDocxToText(str(docxPath))
	requireOK(t, textResult)
	text := textResult.(*object.String).Value
	if strings.Contains(text, "**") || strings.Contains(text, "*italic*") || strings.Contains(text, "***") {
		t.Fatalf("expected pdf_docx_to_text to strip markdown emphasis, got: %s", text)
	}
	if !strings.Contains(text, "bold") || !strings.Contains(text, "italic") || !strings.Contains(text, "both") {
		t.Fatalf("expected pdf_docx_to_text to preserve the underlying words, got: %s", text)
	}
}

func TestDocxWithTableToMarkdown(t *testing.T) {
	dir := chdirTemp(t)
	docxPath := filepath.Join(dir, "table.docx")

	source := "# Data\n\n| Name | Age |\n| --- | --- |\n| Ada | 36 |\n| Grace | 85 |\n"
	requireOK(t, fnMarkdownToDocx(str(source), str(docxPath)))

	mdResult := fnDocxToMarkdown(str(docxPath))
	requireOK(t, mdResult)
	md := mdResult.(*object.String).Value
	for _, want := range []string{"Name", "Age", "Ada", "36", "Grace", "85", "---"} {
		if !strings.Contains(md, want) {
			t.Fatalf("expected round-tripped table markdown to contain %q, got:\n%s", want, md)
		}
	}
}

func TestPDFFromDocx(t *testing.T) {
	dir := chdirTemp(t)
	docxPath := filepath.Join(dir, "source.docx")
	pdfPath := filepath.Join(dir, "converted.pdf")

	requireOK(t, fnMarkdownToDocx(str("# From DOCX\n\nThis paragraph should survive the round trip to PDF."), str(docxPath)))
	requireOK(t, fnFromDocx(str(docxPath), str(pdfPath)))

	text := fnToText(str(pdfPath))
	requireOK(t, text)
	textStr, ok := text.(*object.String)
	if !ok {
		t.Fatalf("expected pdf_to_text to return a STRING, got %T", text)
	}
	if !strings.Contains(textStr.Value, "From DOCX") || !strings.Contains(textStr.Value, "should survive the round trip") {
		t.Fatalf("expected the DOCX-derived PDF to contain the original text, got %q", textStr.Value)
	}
}

func TestDocxImportArgumentValidationErrors(t *testing.T) {
	if _, ok := fnDocxToMarkdown().(*object.Error); !ok {
		t.Fatalf("expected pdf_docx_to_markdown with no arguments to return an error")
	}
	if _, ok := fnDocxToText().(*object.Error); !ok {
		t.Fatalf("expected pdf_docx_to_text with no arguments to return an error")
	}
	if _, ok := fnFromDocx(str("only-one-arg.docx")).(*object.Error); !ok {
		t.Fatalf("expected pdf_from_docx with only one argument to return an error")
	}
}

func TestDocxToHTML(t *testing.T) {
	dir := chdirTemp(t)
	docxPath := filepath.Join(dir, "formatting.docx")
	syntheticWordDocx(t, docxPath)

	htmlResult := fnDocxToHTML(str(docxPath))
	requireOK(t, htmlResult)
	htmlStr, ok := htmlResult.(*object.String)
	if !ok {
		t.Fatalf("expected pdf_docx_to_html to return a STRING, got %T", htmlResult)
	}
	// github.com/oarkflow/pdf/md's HTML exporter (export.HTML) renders
	// headings as <h1>..<h6>, and **bold**/*italic* inline emphasis as
	// <strong>/<em> (see md/internal/export/inline.go's strongRe/emphasisRe
	// and html.go's writeHTML) - assert on those, not on any specific CSS or
	// wrapper markup.
	for _, want := range []string{
		"<h1", "Formatting Demo", "</h1>",
		"<strong>bold</strong>",
		"<em>italic</em>",
		"<em><strong>both</strong></em>",
		"<ul", "bulleted via numbering.xml only",
		"<ol", "numbered via numbering.xml only",
	} {
		if !strings.Contains(htmlStr.Value, want) {
			t.Fatalf("expected converted HTML to contain %q, got:\n%s", want, htmlStr.Value)
		}
	}
}

func TestDocxToHTMLArgumentValidationErrors(t *testing.T) {
	if _, ok := fnDocxToHTML().(*object.Error); !ok {
		t.Fatalf("expected pdf_docx_to_html with no arguments to return an error")
	}
	if _, ok := fnDocxToHTML(str("a"), str("b"), str("c")).(*object.Error); !ok {
		t.Fatalf("expected pdf_docx_to_html with too many arguments to return an error")
	}
}

func TestHTMLToDocx(t *testing.T) {
	dir := chdirTemp(t)
	out := filepath.Join(dir, "converted.docx")

	htmlContent := `<!doctype html><html><head><title>Report</title></head><body>` +
		`<h1>Quarterly Report</h1>` +
		`<p>This paragraph should survive the round trip through PDF and back.</p>` +
		`<ul><li>first bullet</li><li>second bullet</li></ul>` +
		`</body></html>`

	requireOK(t, fnHTMLToDocx(str(htmlContent), str(out)))
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("expected pdf_html_to_docx to create a file: %v", err)
	}
	if len(data) < 4 || string(data[:2]) != "PK" {
		t.Fatalf("expected a valid DOCX (zip) file, got %d bytes starting %q", len(data), data[:min(4, len(data))])
	}

	// No stray intermediate PDF should be left behind in the output
	// directory.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".pdf") {
			t.Fatalf("expected no leftover intermediate PDF file, found %s", e.Name())
		}
	}

	// This chain is lossy (HTML -> PDF -> Markdown -> DOCX), so only assert
	// the actual words/headings survive, not exact formatting fidelity -
	// mirroring how TestPDFFromDocx checks "does the text survive" rather
	// than a byte-exact comparison.
	mdResult := fnDocxToMarkdown(str(out))
	requireOK(t, mdResult)
	md := mdResult.(*object.String).Value
	for _, want := range []string{"Quarterly Report", "should survive the round trip", "first bullet", "second bullet"} {
		if !strings.Contains(md, want) {
			t.Fatalf("expected round-tripped markdown to contain %q, got:\n%s", want, md)
		}
	}

	textResult := fnDocxToText(str(out))
	requireOK(t, textResult)
	text := textResult.(*object.String).Value
	if !strings.Contains(text, "Quarterly Report") || !strings.Contains(text, "should survive the round trip") {
		t.Fatalf("expected pdf_docx_to_text to preserve the underlying words, got: %s", text)
	}
}

func TestHTMLToDocxArgumentValidationErrors(t *testing.T) {
	if _, ok := fnHTMLToDocx(str("only-one-arg")).(*object.Error); !ok {
		t.Fatalf("expected pdf_html_to_docx with only one argument to return an error")
	}
	if _, ok := fnHTMLToDocx(str(""), str("out.docx")).(*object.Error); !ok {
		t.Fatalf("expected pdf_html_to_docx with empty html content to return an error")
	}
}

func TestDocxToMarkdownRejectsNonDocx(t *testing.T) {
	dir := chdirTemp(t)
	notDocx := filepath.Join(dir, "not-a-docx.pdf")
	requireOK(t, fnQuick(str("plain pdf, not a docx"), str(notDocx)))

	if _, ok := fnDocxToMarkdown(str(notDocx)).(*object.Error); !ok {
		t.Fatalf("expected pdf_docx_to_markdown on a non-DOCX file to return an error")
	}
}

func TestPDFArgumentValidationErrors(t *testing.T) {
	if _, ok := fnInfo().(*object.Error); !ok {
		t.Fatalf("expected pdf_info with no arguments to return an error")
	}
	if _, ok := fnMerge(str("out.pdf")).(*object.Error); !ok {
		t.Fatalf("expected pdf_merge with only an output path to return an error")
	}
	if _, ok := fnRotate(str("a"), str("b"), str("1"), str("not-an-int")).(*object.Error); !ok {
		t.Fatalf("expected pdf_rotate with a non-integer degrees argument to return an error")
	}
}
