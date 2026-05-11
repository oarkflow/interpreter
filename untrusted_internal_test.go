package interpreter

import (
	"encoding/base64"
	"testing"
)

func TestObjectFromWorkerJSONValueRehydratesRenderArtifact(t *testing.T) {
	obj := objectFromWorkerJSONValue(map[string]interface{}{
		untrustedJSONTypeKey: untrustedJSONRenderArtifact,
		"kind":               "image",
		"source":             "data:image/png;base64,AA==",
		"source_type":        "data",
		"mime":               "image/png",
		"name":               "dot.png",
		"alt":                "Dot",
		"width":              float64(8),
		"height":             float64(9),
		"max_bytes":          float64(32),
		"mode":               "metadata",
	})
	art, ok := obj.(*RenderArtifact)
	if !ok {
		t.Fatalf("expected render artifact, got %T", obj)
	}
	if art.Kind != "image" || art.SourceTyp != "data" || art.MIME != "image/png" || art.Name != "dot.png" || art.Alt != "Dot" {
		t.Fatalf("unexpected artifact fields: %#v", art)
	}
	if art.Width != 8 || art.Height != 9 || art.MaxBytes != 32 || art.Mode != "metadata" {
		t.Fatalf("unexpected artifact numeric/mode fields: %#v", art)
	}
}

func TestObjectFromWorkerJSONValueRehydratesNestedRenderArtifacts(t *testing.T) {
	obj := objectFromWorkerJSONValue([]interface{}{
		map[string]interface{}{
			"name": "bundle",
			"item": map[string]interface{}{
				untrustedJSONTypeKey: untrustedJSONRenderArtifact,
				"kind":               "html",
				"source":             "<html></html>",
				"source_type":        "data",
				"mime":               "text/html",
			},
		},
	})
	arr, ok := obj.(*Array)
	if !ok || len(arr.Elements) != 1 {
		t.Fatalf("expected one-item array, got %T %#v", obj, obj)
	}
	hash, ok := arr.Elements[0].(*Hash)
	if !ok {
		t.Fatalf("expected hash element, got %T", arr.Elements[0])
	}
	item, ok := hashGet(hash, "item")
	if !ok {
		t.Fatalf("missing nested item in %#v", hash)
	}
	if _, ok := item.(*RenderArtifact); !ok {
		t.Fatalf("expected nested render artifact, got %T", item)
	}
}

func TestObjectFromWorkerJSONValueKeepsPrimitiveDecodeBehavior(t *testing.T) {
	obj := objectFromWorkerJSONValue(float64(42))
	f, ok := obj.(*Float)
	if !ok || f.Value != 42 {
		t.Fatalf("expected existing JSON number decode behavior, got %T %#v", obj, obj)
	}
}

func TestObjectFromWorkerJSONValueRehydratesFileValue(t *testing.T) {
	obj := objectFromWorkerJSONValue(map[string]interface{}{
		untrustedJSONTypeKey: untrustedJSONFileValue,
		"name":               "note.txt",
		"path":               "/tmp/note.txt",
		"mime":               "text/plain",
		"encoding":           "utf-8",
		"source_type":        "data",
		"size":               float64(5),
		"data":               base64.StdEncoding.EncodeToString([]byte("hello")),
	})
	file, ok := obj.(*FileValue)
	if !ok {
		t.Fatalf("expected file value, got %T", obj)
	}
	if file.Name != "note.txt" || file.Path != "/tmp/note.txt" || file.MIME != "text/plain" || file.Encoding != "utf-8" || file.SourceType != "data" {
		t.Fatalf("unexpected file metadata: %#v", file)
	}
	if file.Size != 5 || string(file.Data) != "hello" {
		t.Fatalf("unexpected file payload: %#v", file)
	}
}

func TestObjectFromWorkerJSONValueRehydratesImageValue(t *testing.T) {
	obj := objectFromWorkerJSONValue(map[string]interface{}{
		untrustedJSONTypeKey: untrustedJSONImageValue,
		"name":               "dot.png",
		"path":               "/tmp/dot.png",
		"mime":               "image/png",
		"format":             "png",
		"source_type":        "data",
		"width":              float64(8),
		"height":             float64(9),
		"data":               base64.StdEncoding.EncodeToString([]byte{1, 2, 3}),
	})
	img, ok := obj.(*ImageValue)
	if !ok {
		t.Fatalf("expected image value, got %T", obj)
	}
	if img.Name != "dot.png" || img.Path != "/tmp/dot.png" || img.MIME != "image/png" || img.Format != "png" || img.SourceType != "data" {
		t.Fatalf("unexpected image metadata: %#v", img)
	}
	if img.Width != 8 || img.Height != 9 || string(img.Data) != string([]byte{1, 2, 3}) {
		t.Fatalf("unexpected image payload: %#v", img)
	}
}

func TestObjectFromWorkerJSONValueRehydratesTableValue(t *testing.T) {
	obj := objectFromWorkerJSONValue(map[string]interface{}{
		untrustedJSONTypeKey: untrustedJSONTableValue,
		"name":               "report",
		"path":               "/tmp/report.csv",
		"mime":               "text/csv",
		"source_type":        "data",
		"columns":            []interface{}{"name", "preview"},
		"rows": []interface{}{
			map[string]interface{}{
				"name": "alpha",
				"preview": map[string]interface{}{
					untrustedJSONTypeKey: untrustedJSONRenderArtifact,
					"kind":               "html",
					"source":             "<strong>alpha</strong>",
					"source_type":        "data",
					"mime":               "text/html",
				},
			},
		},
	})
	table, ok := obj.(*TableValue)
	if !ok {
		t.Fatalf("expected table value, got %T", obj)
	}
	if table.Name != "report" || table.Path != "/tmp/report.csv" || table.MIME != "text/csv" || table.SourceType != "data" {
		t.Fatalf("unexpected table metadata: %#v", table)
	}
	if len(table.Columns) != 2 || table.Columns[0] != "name" || table.Columns[1] != "preview" {
		t.Fatalf("unexpected table columns: %#v", table.Columns)
	}
	if len(table.Rows) != 1 {
		t.Fatalf("unexpected table rows: %#v", table.Rows)
	}
	if name, ok := table.Rows[0]["name"].(*String); !ok || name.Value != "alpha" {
		t.Fatalf("unexpected table row name: %#v", table.Rows[0]["name"])
	}
	if _, ok := table.Rows[0]["preview"].(*RenderArtifact); !ok {
		t.Fatalf("expected nested render artifact cell, got %T", table.Rows[0]["preview"])
	}
}
