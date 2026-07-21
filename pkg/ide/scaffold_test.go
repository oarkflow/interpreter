package ide

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oarkflow/interpreter/pkg/pkgmgr"
)

func TestScaffoldMinimalProducesValidModuleManifest(t *testing.T) {
	dir := t.TempDir()
	if err := Scaffold(ScaffoldMinimal, dir, "my-minimal-app"); err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	manifest, err := pkgmgr.ReadModuleManifestFromFile(filepath.Join(dir, "spl.mod"))
	if err != nil {
		t.Fatalf("ReadModuleManifestFromFile: %v", err)
	}
	if manifest.Module != "my-minimal-app" {
		t.Fatalf("expected module name to be rewritten, got %q", manifest.Module)
	}
	if manifest.Dependencies["@"] != "app" {
		t.Fatalf("expected @ alias -> app, got %+v", manifest.Dependencies)
	}
	for _, must := range []string{"main.spl", ".env", "app/config/app.spl", "app/models/todo.spl"} {
		if _, err := os.Stat(filepath.Join(dir, must)); err != nil {
			t.Fatalf("expected %s to exist: %v", must, err)
		}
	}
	// Minimal scaffold must not reference database/template builtins so it
	// stays runnable under the lightweight cmd/interpreter.
	if _, err := os.Stat(filepath.Join(dir, "app/views/todos/index.html")); err == nil {
		t.Fatalf("minimal scaffold should not include the @for/@component template views")
	}
}

func TestScaffoldAppMatchesExamplesAppShape(t *testing.T) {
	dir := t.TempDir()
	if err := Scaffold(ScaffoldApp, dir, "my-full-app"); err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	for _, must := range []string{
		"main.spl", "spl.mod", ".env", ".env.example",
		"app/config/app.spl", "app/models/todo.spl",
		"app/controllers/home_controller.spl", "app/controllers/todo_controller.spl",
		"app/middleware/logger.spl", "app/routes/web.spl", "app/routes/api.spl",
		"app/views/layout.html", "app/views/home.html", "app/views/todos/index.html",
		"public/css/app.css", "storage/logs/.gitkeep",
	} {
		if _, err := os.Stat(filepath.Join(dir, must)); err != nil {
			t.Fatalf("expected %s to exist in the app scaffold: %v", must, err)
		}
	}
	manifest, err := pkgmgr.ReadModuleManifestFromFile(filepath.Join(dir, "spl.mod"))
	if err != nil {
		t.Fatalf("ReadModuleManifestFromFile: %v", err)
	}
	if manifest.Module != "my-full-app" {
		t.Fatalf("expected module name to be rewritten, got %q", manifest.Module)
	}
}

func TestScaffoldRejectsUnknownKind(t *testing.T) {
	dir := t.TempDir()
	if err := Scaffold("bogus", dir, "x"); err == nil {
		t.Fatalf("expected error for unknown scaffold kind")
	}
}

func TestScaffoldDoesNotOverwriteExistingEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("APP_PORT=9999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Scaffold(ScaffoldMinimal, dir, "x"); err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "APP_PORT=9999\n" {
		t.Fatalf("expected pre-existing .env to be preserved, got %q", data)
	}
}
