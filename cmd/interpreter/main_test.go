package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/eval"
)

func TestCLILinksToolsBuiltins(t *testing.T) {
	for _, name := range []string{"bulk_rename", "archive_compress", "image_convert_batch", "media_convert", "ffmpeg_status"} {
		if !eval.HasBuiltin(name) {
			t.Fatalf("expected tools builtin %q to be linked into cmd/interpreter", name)
		}
	}
}

func TestCLILinksFullPluginSet(t *testing.T) {
	for _, name := range []string{"db_connect", "image_resize", "pdf_info", "bcrypt_hash", "xql_run", "naturaldate_parse", "wuid_new", "money_new", "phone_parse", "ip_is_private", "securetoken_encrypt"} {
		if !eval.HasBuiltin(name) {
			t.Fatalf("expected plugin builtin %q to be linked into the consolidated cmd/interpreter binary", name)
		}
	}
}

func TestStripPlaygroundFlag(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantMatch  bool
		wantResult []string
	}{
		{"absent", []string{"interpreter", "script.spl"}, false, []string{"interpreter", "script.spl"}},
		{"no args", []string{"interpreter"}, false, []string{"interpreter"}},
		{"long form", []string{"interpreter", "--playground"}, true, []string{"interpreter"}},
		{"short form", []string{"interpreter", "-playground"}, true, []string{"interpreter"}},
		{"with trailing flags", []string{"interpreter", "--playground", "--profile", "trusted"}, true, []string{"interpreter", "--profile", "trusted"}},
		{"not first arg", []string{"interpreter", "script.spl", "--playground"}, false, []string{"interpreter", "script.spl", "--playground"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotMatch, gotResult := stripPlaygroundFlag(tc.args)
			if gotMatch != tc.wantMatch {
				t.Fatalf("stripPlaygroundFlag(%v) match = %v, want %v", tc.args, gotMatch, tc.wantMatch)
			}
			if len(gotResult) != len(tc.wantResult) {
				t.Fatalf("stripPlaygroundFlag(%v) result = %v, want %v", tc.args, gotResult, tc.wantResult)
			}
			for i := range gotResult {
				if gotResult[i] != tc.wantResult[i] {
					t.Fatalf("stripPlaygroundFlag(%v) result = %v, want %v", tc.args, gotResult, tc.wantResult)
				}
			}
		})
	}
}

func TestCLIImportsTemplateRuntime(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "render_check.spl")
	tmplDir := filepath.Join(dir, "views")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmplDir, "page.html"), []byte(`<h1>${title}</h1>`), 0644); err != nil {
		t.Fatal(err)
	}
	script := `
import "server" as server;
let app = server.web_app("` + tmplDir + `");
let runtime = app;
let has_templates = runtime != null;
has_templates;
`
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	res, err := interpreter.ExecFile(scriptPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res != interpreter.TRUE {
		t.Fatalf("expected true result, got %v", res)
	}
}
