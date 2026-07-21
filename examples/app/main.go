// Command spl-todo-app is a single-binary build of the examples/app SPL
// application: `go build -o spl-todo-app .` (from this directory) produces
// one executable with the entire app (main.spl, app/, public/) embedded -
// no source tree needs to be present at runtime.
//
// It's a thin Go wrapper, not a rewrite: it extracts the embedded files to
// a temp directory (the interpreter's module/template/static-file
// resolution needs real files on disk - see README.md's "Run it" section
// on why cwd matters), then runs main.spl exactly like
// `go run ./cmd/interpreter-full main.spl` would. The extracted storage/
// directory holds a fresh SQLite file per run - see README.md's
// "Single-binary build" section for how to make that persist.
package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/object"

	_ "github.com/oarkflow/interpreter/pkg/builtins/reactive"
	_ "github.com/oarkflow/interpreter/pkg/builtins/scheduler"
	_ "github.com/oarkflow/interpreter/pkg/builtins/tools"
	_ "github.com/oarkflow/interpreter/pkg/builtins/watcher"
	_ "github.com/oarkflow/interpreter/plugins"
)

// app/models/todo.spl uses the database builtins (SQLite), so this build
// links builtins/database directly - that's also why this directory has
// its own go.mod (see the module comment there): builtins/database pulls
// in real driver dependencies the root module intentionally stays free of.
//
//go:embed all:app all:public all:storage main.spl spl.mod .env.example
var appFiles embed.FS

func main() {
	os.Exit(run())
}

func run() int {
	launchDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	dir, err := os.MkdirTemp("", "spl-todo-app-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	// Only covers the path where run() returns normally: the signal
	// handler below calls os.Exit directly (to actually terminate a
	// process blocked in listen()), which skips defers, so it removes
	// dir itself before exiting.
	defer os.RemoveAll(dir)

	if err := extractEmbedded(appFiles, dir); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	// A .env next to wherever the binary is launched from overrides the
	// embedded .env.example, same as running from source with a local
	// .env - the compiled binary still works with none present (see
	// app/config/app.spl's fallbacks).
	if data, readErr := os.ReadFile(filepath.Join(launchDir, ".env")); readErr == nil {
		_ = os.WriteFile(filepath.Join(dir, ".env"), data, 0o600)
	}

	if err := os.Chdir(dir); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	installSignalHandler(dir)

	if _, err := interpreter.ExecFile(filepath.Join(dir, "main.spl"), nil); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

// installSignalHandler mirrors pkg/eval/cli.go's StartCLI handler: this
// binary bypasses StartCLI entirely (it calls interpreter.ExecFile
// directly), so it needs its own Ctrl+C/SIGTERM handling to get the same
// graceful shutdown (drain in-flight requests via object.RunShutdownHooks
// before exiting, instead of the process just being killed mid-request).
func installSignalHandler(dir string) {
	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nShutting down (press Ctrl+C again to force quit)...")

		done := make(chan struct{})
		go func() {
			object.RunShutdownHooks(5 * time.Second)
			close(done)
		}()

		select {
		case <-done:
		case <-sigChan:
			fmt.Println("\nForcing exit...")
		}
		os.RemoveAll(dir) // os.Exit below skips the defer in run()
		os.Exit(130)
	}()
}

func extractEmbedded(f embed.FS, dest string) error {
	return fs.WalkDir(f, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dest, path)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, readErr := f.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(target, data, 0o644)
	})
}
