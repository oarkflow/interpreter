package yaml

import (
	"testing"

	"github.com/oarkflow/interpreter/pkg/config"
)

func TestYAMLParserRegistersOnImport(t *testing.T) {
	if config.FormatParsers["yaml"] == nil {
		t.Fatal("expected yaml parser to be registered")
	}
	if config.FormatParsers["yml"] == nil {
		t.Fatal("expected yml parser to be registered")
	}
}
