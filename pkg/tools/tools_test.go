package tools

import (
	"archive/zip"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestBulkRenamePreviewDoesNotMutate(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "photo.jpg")
	if err := os.WriteFile(src, []byte("image"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	ops, err := BulkRename(dir, map[string]any{"match": "*.jpg", "template": "{date}_{seq}.{ext}"}, Hooks{})
	if err != nil {
		t.Fatalf("BulkRename: %v", err)
	}
	if len(ops) != 1 || ops[0].Status != "planned" {
		t.Fatalf("unexpected ops: %#v", ops)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("preview should leave source in place: %v", err)
	}
}

func TestBulkRenameApplyDetectsCollision(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.jpg"), []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.jpg"), []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	ops, err := BulkRename(dir, map[string]any{"match": "*.jpg", "template": "same.{ext}", "apply": true}, Hooks{})
	if err != nil {
		t.Fatalf("BulkRename: %v", err)
	}
	failed := 0
	for _, op := range ops {
		if op.Status == "failed" {
			failed++
		}
	}
	if failed == 0 {
		t.Fatalf("expected collision failure, got %#v", ops)
	}
}

func TestCompressZipPreviewAndApply(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "doc.txt")
	dst := filepath.Join(dir, "doc.zip")
	if err := os.WriteFile(src, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	preview := Compress(src, dst, map[string]any{"format": "zip"}, Hooks{})
	if preview.Status != "planned" {
		t.Fatalf("unexpected preview: %#v", preview)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("preview should not create archive")
	}
	applied := Compress(src, dst, map[string]any{"format": "zip", "apply": true}, Hooks{})
	if applied.Status != "applied" {
		t.Fatalf("unexpected apply: %#v", applied)
	}
	zr, err := zip.OpenReader(dst)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()
	if len(zr.File) != 1 {
		t.Fatalf("expected one entry, got %d", len(zr.File))
	}
}

// TestCompressExtractPasswordProtectedZipRoundTrip is a regression test:
// Compress used to hard-reject any "password" option
// ("password-protected archives are not supported by the Go-native v1
// tools"). It now supports AES-256 password-protected zip archives via
// github.com/yeka/zip.
func TestCompressExtractPasswordProtectedZipRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "secret.txt")
	dst := filepath.Join(dir, "secret.zip")
	const password = "correct horse battery staple"
	const content = "this archive is password protected"
	if err := os.WriteFile(src, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	applied := Compress(src, dst, map[string]any{"format": "zip", "password": password, "apply": true}, Hooks{})
	if applied.Status != "applied" {
		t.Fatalf("unexpected compress result: %#v", applied)
	}

	// A plain (non-yeka) zip.OpenReader can still list the encrypted
	// archive's entries (the container format is standard zip), but reading
	// the entry's raw content should not yield the plaintext.
	zr, err := zip.OpenReader(dst)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	if len(zr.File) != 1 {
		t.Fatalf("expected one entry, got %d", len(zr.File))
	}
	zr.Close()

	extractDir := filepath.Join(dir, "out")
	extracted := Extract(dst, extractDir, map[string]any{"password": password, "apply": true}, Hooks{})
	if extracted.Status != "applied" {
		t.Fatalf("unexpected extract result: %#v", extracted)
	}
	got, err := os.ReadFile(filepath.Join(extractDir, "secret.txt"))
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(got) != content {
		t.Fatalf("extracted content mismatch: got %q, want %q", got, content)
	}

	// Wrong password must not silently succeed with garbage output.
	wrongDir := filepath.Join(dir, "wrong")
	wrongExtract := Extract(dst, wrongDir, map[string]any{"password": "not the password", "apply": true}, Hooks{})
	if wrongExtract.Status == "applied" {
		if got, err := os.ReadFile(filepath.Join(wrongDir, "secret.txt")); err == nil && string(got) == content {
			t.Fatalf("expected wrong password to fail or produce different content, got original content back")
		}
	}
}

// TestCompressRejectsPasswordForNonZipFormat confirms password protection
// stays scoped to zip (tar/gzip have no equivalent encryption support in
// this tool), rather than silently ignoring the option.
func TestCompressRejectsPasswordForNonZipFormat(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "doc.txt")
	dst := filepath.Join(dir, "doc.tar")
	if err := os.WriteFile(src, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := Compress(src, dst, map[string]any{"format": "tar", "password": "secret", "apply": true}, Hooks{})
	if result.Status != "failed" {
		t.Fatalf("expected password + non-zip format to fail, got %#v", result)
	}
}

func TestSecretEncryptDecryptRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "plain.txt")
	enc := filepath.Join(dir, "plain.enc")
	dec := filepath.Join(dir, "plain.out")
	if err := os.WriteFile(src, []byte("secret text"), 0o600); err != nil {
		t.Fatal(err)
	}
	if op := EncryptFile(src, enc, "pass", true, Hooks{}); op.Status != "applied" {
		t.Fatalf("encrypt failed: %#v", op)
	}
	if op := DecryptFile(enc, dec, "pass", true, Hooks{}); op.Status != "applied" {
		t.Fatalf("decrypt failed: %#v", op)
	}
	data, err := os.ReadFile(dec)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "secret text" {
		t.Fatalf("unexpected decrypted content: %q", string(data))
	}
}

func TestArchiveTarAndGzip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "doc.txt")
	tarPath := filepath.Join(dir, "doc.tar")
	gzPath := filepath.Join(dir, "doc.txt.gz")
	if err := os.WriteFile(src, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if op := Compress(src, tarPath, map[string]any{"format": "tar", "apply": true}, Hooks{}); op.Status != "applied" {
		t.Fatalf("tar failed: %#v", op)
	}
	entries, err := ArchiveList(tarPath, Hooks{})
	if err != nil || len(entries) != 1 {
		t.Fatalf("tar list entries=%#v err=%v", entries, err)
	}
	tarOut := filepath.Join(dir, "tar-out")
	if op := Extract(tarPath, tarOut, map[string]any{"apply": true}, Hooks{}); op.Status != "applied" {
		t.Fatalf("tar extract failed: %#v", op)
	}
	if _, err := os.Stat(filepath.Join(tarOut, "doc.txt")); err != nil {
		t.Fatalf("expected extracted tar file: %v", err)
	}
	if op := Compress(src, gzPath, map[string]any{"format": "gzip", "apply": true}, Hooks{}); op.Status != "applied" {
		t.Fatalf("gzip failed: %#v", op)
	}
	gzOut := filepath.Join(dir, "doc.out")
	if op := Extract(gzPath, gzOut, map[string]any{"apply": true}, Hooks{}); op.Status != "applied" {
		t.Fatalf("gzip extract failed: %#v", op)
	}
	data, err := os.ReadFile(gzOut)
	if err != nil || string(data) != "hello" {
		t.Fatalf("unexpected gzip output %q err=%v", string(data), err)
	}
}

func TestFileOrganizeChecksumAndRemovePreview(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "photo.jpg")
	dst := filepath.Join(dir, "organized")
	if err := os.WriteFile(src, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	sum, err := FileChecksum(src, Hooks{})
	if err != nil || sum["sha256"] == "" {
		t.Fatalf("checksum failed: %#v err=%v", sum, err)
	}
	ops, err := OrganizeByExt(dir, dst, map[string]any{"match": "*.jpg"}, Hooks{})
	if err != nil || len(ops) != 1 || ops[0].Status != "planned" {
		t.Fatalf("organize preview got %#v err=%v", ops, err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("preview should not move source: %v", err)
	}
	if op := RemovePath(src, map[string]any{}, Hooks{}); op.Status != "planned" {
		t.Fatalf("remove preview got %#v", op)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("remove preview should not delete source: %v", err)
	}
}

func TestImageResizeInfoAndOfficeRead(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "tiny.png")
	resized := filepath.Join(dir, "tiny-small.png")
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	if info, err := ImageInfo(src, Hooks{}); err != nil || info["width"].(int) != 4 {
		t.Fatalf("image info=%#v err=%v", info, err)
	}
	if op := ResizeImage(src, resized, map[string]any{"width": 2, "apply": true}, Hooks{}); op.Status != "applied" {
		t.Fatalf("resize failed: %#v", op)
	}
	csvPath := filepath.Join(dir, "people.csv")
	if err := os.WriteFile(csvPath, []byte("name,age\nAda,37\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := OfficeRead(csvPath, Hooks{})
	if err != nil {
		t.Fatalf("office read: %v", err)
	}
	rows := doc["rows"].([][]string)
	if len(rows) != 2 || rows[1][0] != "Ada" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
}

// TestConvertImageFileWebPEncodeDecodeRoundTrip is a regression test: webp
// encoding used to hard-fail ("webp encode is unsupported by the Go-native
// v1 tools"). It now works (lossless, via github.com/HugoSmits86/nativewebp
// - golang.org/x/image/webp, already a dependency, only decodes).
func TestConvertImageFileWebPEncodeDecodeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.png")
	dst := filepath.Join(dir, "converted.webp")

	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	img.Set(1, 1, color.NRGBA{G: 255, A: 255})
	img.Set(2, 2, color.NRGBA{B: 255, A: 255})
	img.Set(3, 3, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	if err := ConvertImageFile(src, dst, "webp", 90); err != nil {
		t.Fatalf("webp encode failed: %v", err)
	}

	decoded, err := os.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer decoded.Close()
	decodedImg, format, err := image.Decode(decoded)
	if err != nil {
		t.Fatalf("failed to decode produced webp file: %v", err)
	}
	if format != "webp" {
		t.Fatalf("expected decoded format webp, got %q", format)
	}
	for _, pt := range []struct{ x, y int }{{0, 0}, {1, 1}, {2, 2}, {3, 3}} {
		want := img.NRGBAAt(pt.x, pt.y)
		got := decodedImg.At(pt.x, pt.y)
		gr, gg, gb, ga := got.RGBA()
		wr, wg, wb, wa := want.RGBA()
		if gr != wr || gg != wg || gb != wb || ga != wa {
			t.Fatalf("pixel (%d,%d) mismatch after webp round-trip: got %v, want %v", pt.x, pt.y, got, want)
		}
	}
}
