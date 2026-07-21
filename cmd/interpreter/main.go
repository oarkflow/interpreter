// Command interpreter is the SPL CLI, REPL, and (via --playground) browser
// playground server, with the full builtin/plugin surface linked in
// (database, images, PDF, secrets, crypto extras, XQL, natural-language
// dates, money, phone, wuid, IP, integrations, and more - see
// github.com/oarkflow/interpreter/plugins). It's its own Go module (separate
// go.mod) because several of those plugins pull in heavy or private
// dependencies that the root github.com/oarkflow/interpreter module
// deliberately does not carry.
package main

import (
	"os"
	"path/filepath"

	"github.com/oarkflow/interpreter"
	_ "github.com/oarkflow/interpreter/pkg/builtins/reactive"
	_ "github.com/oarkflow/interpreter/pkg/builtins/scheduler"
	_ "github.com/oarkflow/interpreter/pkg/builtins/tools"
	_ "github.com/oarkflow/interpreter/pkg/builtins/watcher"
	"github.com/oarkflow/interpreter/pkg/ide"
	"github.com/oarkflow/interpreter/pkg/playgroundserver"
	"github.com/oarkflow/interpreter/pkg/security"
	_ "github.com/oarkflow/interpreter/plugins"
)

func main() {
	if playground, rest := stripPlaygroundFlag(os.Args); playground {
		os.Args = rest
		runPlayground()
		return
	}
	interpreter.RunCLIMain()
}

// stripPlaygroundFlag reports whether --playground/-playground is the first
// argument and, if so, returns argv with it removed. This is checked before
// any flag package touches os.Args (mirroring how RunCLIMain already
// special-cases --spl-worker as argv[1]), so the normal CLI/REPL flags are
// completely unaffected when --playground is absent, and
// playgroundserver.Run's own flag parsing sees a clean argv when it is
// present.
func stripPlaygroundFlag(args []string) (bool, []string) {
	if len(args) > 1 && (args[1] == "--playground" || args[1] == "-playground") {
		rest := append([]string{args[0]}, args[2:]...)
		return true, rest
	}
	return false, args
}

func runPlayground() {
	// This binary is its own Go module (separate go.mod, replacing the root
	// module at ../..), so `go build` for it must run with cmd.Dir set to
	// this module's own directory and BuildPackage "." - not
	// "./cmd/interpreter" from the root repo, which is a different module
	// boundary from the root's perspective. This precomputed default can
	// still be overridden by PLAYGROUND_INTERPRETER_REPO_ROOT (applied
	// centrally in playgroundserver.Run).
	repoRoot := ""
	if root, err := ide.DetectRepoRoot(); err == nil {
		repoRoot = filepath.Join(root, "cmd", "interpreter")
	}

	playgroundserver.Run(playgroundserver.Variant{
		Name:              "full",
		ExtraCapabilities: []string{security.CapabilityDB, security.CapabilityNetwork, security.CapabilityPolicy},
		ExampleOverrides:  fullExampleOverrides,
		ExtraExamples:     fullExtraExamples,
		IDERunner: ide.RunnerConfig{
			Variant:      "full",
			BuildPackage: ".",
			RepoRoot:     repoRoot,
			ScaffoldKind: ide.ScaffoldApp,
		},
	})
}
