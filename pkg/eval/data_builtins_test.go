package eval_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/oarkflow/interpreter/pkg/object"
)

func TestFileValueRoundTripBuiltins(t *testing.T) {
	dir := makeWorkspaceTempDir(t)
	inputPath := filepath.Join(dir, "note.txt")
	outputPath := filepath.Join(dir, "copy.txt")
	if err := os.WriteFile(inputPath, []byte("hello artifact"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	obj := evalWithParserCheck(t, fmt.Sprintf(`
		let f = file_load(%q);
		let saved = file_save(f, %q);
		[file_text(saved), file_name(saved), file_size(saved), render(saved)];
	`, inputPath, outputPath), object.NewGlobalEnvironment(nil))

	result, ok := obj.(*object.Array)
	if !ok || len(result.Elements) != 4 {
		t.Fatalf("expected array result, got %T: %s", obj, obj.Inspect())
	}
	if got := result.Elements[0].Inspect(); got != "hello artifact" {
		t.Fatalf("unexpected text payload: %q", got)
	}
	if got := result.Elements[1].Inspect(); got != "copy.txt" {
		t.Fatalf("unexpected saved name: %q", got)
	}
	if size, ok := result.Elements[2].(*object.Integer); !ok || size.Value != int64(len("hello artifact")) {
		t.Fatalf("unexpected file size: %#v", result.Elements[2])
	}
	art, ok := result.Elements[3].(*object.RenderArtifact)
	if !ok || art.MIME != "text/plain; charset=utf-8" && art.MIME != "text/plain" {
		t.Fatalf("unexpected render artifact: %#v", result.Elements[3])
	}
	if data, err := os.ReadFile(outputPath); err != nil || string(data) != "hello artifact" {
		t.Fatalf("unexpected saved file content: %q err=%v", string(data), err)
	}
}

func TestImageValueResizeAndRenderBuiltins(t *testing.T) {
	dir := makeWorkspaceTempDir(t)
	imagePath := filepath.Join(dir, "sample.png")
	writePNGFixture(t, imagePath, 4, 3)

	obj := evalWithParserCheck(t, fmt.Sprintf(`
		let img = image_load(file(%q));
		let resized = image_resize(img, 2, 1);
		[image_info(resized), image_render(resized), file_load(image_render(resized))];
	`, imagePath), object.NewGlobalEnvironment(nil))

	result, ok := obj.(*object.Array)
	if !ok || len(result.Elements) != 3 {
		t.Fatalf("expected array result, got %T: %s", obj, obj.Inspect())
	}

	info, ok := result.Elements[0].(*object.Hash)
	if !ok {
		t.Fatalf("expected image info hash, got %T", result.Elements[0])
	}
	width := info.Pairs[(&object.String{Value: "width"}).HashKey()].Value.(*object.Integer).Value
	height := info.Pairs[(&object.String{Value: "height"}).HashKey()].Value.(*object.Integer).Value
	if width != 2 || height != 1 {
		t.Fatalf("unexpected resized dimensions: %dx%d", width, height)
	}

	art, ok := result.Elements[1].(*object.RenderArtifact)
	if !ok || art.Kind != "image" || art.SourceTyp != "data" {
		t.Fatalf("unexpected image render artifact: %#v", result.Elements[1])
	}

	file, ok := result.Elements[2].(*object.FileValue)
	if !ok || file.Size == 0 || file.MIME == "" {
		t.Fatalf("unexpected rendered file value: %#v", result.Elements[2])
	}
}

func TestJSONAndCSVWorkingValueBuiltins(t *testing.T) {
	dir := makeWorkspaceTempDir(t)
	jsonPath := filepath.Join(dir, "sample.json")
	csvPath := filepath.Join(dir, "people.csv")

	obj := evalWithParserCheck(t, fmt.Sprintf(`
		write_json(%q, {"name": "Ada", "roles": ["dev", "ops"]}, {"pretty": true});
		write_csv(%q, [
			{"name": "Ada", "age": "31"},
			{"name": "Bob", "age": "27"}
		]);
		let person = read_json(%q);
		let table = read_csv(%q);
		let filtered = table_filter(table, function(row) { return row.age == "31"; });
		let mapped = table_map(filtered, function(row) { return {"name": row.name, "tag": row.name + "-ok"}; });
		[person.name, table_columns(mapped), table_rows(mapped), render(mapped)];
	`, jsonPath, csvPath, jsonPath, csvPath), object.NewGlobalEnvironment(nil))

	result, ok := obj.(*object.Array)
	if !ok || len(result.Elements) != 4 {
		t.Fatalf("expected array result, got %T: %s", obj, obj.Inspect())
	}
	if got := result.Elements[0].Inspect(); got != "Ada" {
		t.Fatalf("unexpected JSON value: %q", got)
	}
	cols, ok := result.Elements[1].(*object.Array)
	if !ok || len(cols.Elements) != 2 {
		t.Fatalf("unexpected mapped columns: %#v", result.Elements[1])
	}
	rows, ok := result.Elements[2].(*object.Array)
	if !ok || len(rows.Elements) != 1 {
		t.Fatalf("unexpected mapped rows: %#v", result.Elements[2])
	}
	art, ok := result.Elements[3].(*object.RenderArtifact)
	if !ok || art.MIME != "text/csv" {
		t.Fatalf("unexpected mapped render artifact: %#v", result.Elements[3])
	}
}

func TestFilePathOperationsBuiltins(t *testing.T) {
	dir := makeWorkspaceTempDir(t)
	inputPath := filepath.Join(dir, "move-me.txt")
	if err := os.WriteFile(inputPath, []byte("shift"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	copyPath := filepath.Join(dir, "copy.txt")
	movedPath := filepath.Join(dir, "moved.txt")

	obj := evalWithParserCheck(t, fmt.Sprintf(`
		let copied = file_copy(%q, %q);
		let moved = file_move(copied, %q);
		let renamed = file_rename(moved, "done.txt");
		[copied, moved, renamed];
	`, inputPath, copyPath, movedPath), object.NewGlobalEnvironment(nil))

	result, ok := obj.(*object.Array)
	if !ok || len(result.Elements) != 3 {
		t.Fatalf("expected array result, got %T: %s", obj, obj.Inspect())
	}
	finalPath := filepath.Join(dir, "done.txt")
	if _, err := os.Stat(copyPath); !os.IsNotExist(err) {
		t.Fatalf("expected copy path to be moved away, stat err=%v", err)
	}
	if data, err := os.ReadFile(finalPath); err != nil || string(data) != "shift" {
		t.Fatalf("unexpected final renamed file: %q err=%v", string(data), err)
	}
}

func makeWorkspaceTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(".", "spl-data-*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("abs temp dir: %v", err)
	}
	return abs
}

func writePNGFixture(t *testing.T, path string, width, height int) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(20 * x), G: uint8(30 * y), B: 160, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write png fixture: %v", err)
	}
}
