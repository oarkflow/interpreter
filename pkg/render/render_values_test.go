package render

import (
	"testing"

	"github.com/oarkflow/interpreter/pkg/object"
)

func TestArtifactFromObjectSupportsWorkingValues(t *testing.T) {
	fileArt, ok := ArtifactFromObject(&object.FileValue{
		Name: "note.txt",
		MIME: "text/plain",
		Data: []byte("hello"),
		Size: 5,
	})
	if !ok || fileArt.Kind != "text" || fileArt.Source != "hello" {
		t.Fatalf("unexpected file artifact: %#v ok=%v", fileArt, ok)
	}

	tableArt, ok := ArtifactFromObject(&object.TableValue{
		Name:    "people.csv",
		Columns: []string{"name", "age"},
		Rows: []map[string]object.Object{
			{"name": &object.String{Value: "Ada"}, "age": &object.String{Value: "31"}},
		},
	})
	if !ok || tableArt.MIME != "text/csv" {
		t.Fatalf("unexpected table artifact: %#v ok=%v", tableArt, ok)
	}

	imageArt, ok := ArtifactFromObject(&object.ImageValue{
		Name:   "dot.png",
		MIME:   "image/png",
		Format: "png",
		Data:   []byte{0x89, 0x50, 0x4e, 0x47},
		Width:  1,
		Height: 1,
	})
	if !ok || imageArt.Kind != "image" || imageArt.SourceTyp != "data" {
		t.Fatalf("unexpected image artifact: %#v ok=%v", imageArt, ok)
	}
}
