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
