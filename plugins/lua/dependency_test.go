package lua

import (
	goparser "go/parser"
	gotoken "go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Keep the Lua implementation native to this repository. Standard-library
// imports and the host interpreter API are allowed; external Lua engines and
// cgo are not.
func TestNoCgoOrThirdPartySourceDependencies(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	files := gotoken.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		file, err := goparser.ParseFile(files, entry.Name(), nil, goparser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if path == "C" {
				t.Fatalf("%s imports C", entry.Name())
			}
			if strings.Contains(path, ".") && !strings.HasPrefix(path, "github.com/oarkflow/interpreter") {
				t.Fatalf("%s imports third-party package %q", entry.Name(), path)
			}
		}
	}
}
