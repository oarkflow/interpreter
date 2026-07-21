package ide

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// rootModuleLine is the first line of the interpreter repo's own root
// go.mod, used to recognize it while walking up from this file's compiled
// source location.
const rootModuleLine = "module github.com/oarkflow/interpreter"

// DetectRepoRoot locates the interpreter repo root (the directory holding
// the root go.mod for "github.com/oarkflow/interpreter") by walking up
// from this source file's own compile-time path. This only works for
// ordinary `go build`/`go run` (which embed real source paths); a binary
// built with -trimpath won't have a usable path here, so callers should
// fall back to RunnerConfig.RepoRoot (or BinaryPath) in that case.
//
// Note for cmd/interpreter (and any other nested package with its own
// go.mod): it is a separate Go module, so `go build` for it must run with
// cmd.Dir set to that module's own directory (e.g.
// filepath.Join(DetectRepoRoot(), "cmd", "interpreter")) with
// BuildPackage ".", not "./cmd/interpreter" from the root.
func DetectRepoRoot() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok || strings.TrimSpace(thisFile) == "" {
		return "", fmt.Errorf("could not determine compiled source path (built with -trimpath?); set RunnerConfig.RepoRoot or BinaryPath explicitly")
	}
	dir := filepath.Dir(thisFile)
	for {
		modPath := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(modPath); err == nil {
			firstLine := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)[0]
			if strings.TrimSpace(firstLine) == rootModuleLine {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("could not locate root go.mod (%s) above %s; set RunnerConfig.RepoRoot or BinaryPath explicitly", rootModuleLine, filepath.Dir(thisFile))
}
