package ide

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/oarkflow/interpreter/pkg/pkgmgr"
)

// Scaffold kinds. Both run under cmd/interpreter: "minimal" is a bare
// single-file script with no database/template dependency; "app" mirrors
// examples/app and showcases the richer SQLite + builtins/template surface.
const (
	ScaffoldMinimal = "minimal"
	ScaffoldApp     = "app"
)

//go:embed all:testdata/scaffold/minimal all:testdata/scaffold/app
var scaffoldFS embed.FS

// ValidScaffoldKind reports whether kind is a known scaffold.
func ValidScaffoldKind(kind string) bool {
	switch kind {
	case ScaffoldMinimal, ScaffoldApp:
		return true
	default:
		return false
	}
}

// Scaffold copies the embedded template tree for kind into projectDir (which
// must already exist) and rewrites spl.mod's module field to moduleName.
func Scaffold(kind, projectDir, moduleName string) error {
	if !ValidScaffoldKind(kind) {
		return fmt.Errorf("unknown scaffold kind %q", kind)
	}
	root := "testdata/scaffold/" + kind
	err := fs.WalkDir(scaffoldFS, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(path, root), "/")
		if rel == "" {
			return nil
		}
		target := filepath.Join(projectDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, readErr := scaffoldFS.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		return fmt.Errorf("copy scaffold: %w", err)
	}

	manifestPath := filepath.Join(projectDir, pkgmgr.SPLManifestFileName)
	manifest, err := pkgmgr.ReadModuleManifestFromFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read scaffolded spl.mod: %w", err)
	}
	manifest.Module = moduleName
	if err := pkgmgr.WriteModuleManifest(projectDir, manifest); err != nil {
		return fmt.Errorf("rewrite spl.mod: %w", err)
	}

	envExample := filepath.Join(projectDir, ".env.example")
	if data, err := os.ReadFile(envExample); err == nil {
		envPath := filepath.Join(projectDir, ".env")
		if _, statErr := os.Stat(envPath); os.IsNotExist(statErr) {
			if err := os.WriteFile(envPath, data, 0o644); err != nil {
				return fmt.Errorf("seed .env: %w", err)
			}
		}
	}
	return nil
}
