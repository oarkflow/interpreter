package pdf

import (
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

	// No algorithm argument -> defaults to AES-128, which
	// github.com/oarkflow/pdf@v0.0.2 supports end-to-end. AES-256 is
	// accepted by some lower-level paths in that library version but
	// Protect() itself rejects it ("is not supported yet") - regression
	// test for that default choice.
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
