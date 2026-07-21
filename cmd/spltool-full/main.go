// Command spltool-full is the "full" SPL CLI/LSP tool: the same
// cli/lsp implementation as cmd/spltool, but with the entire plugin
// builtin surface blank-imported (github.com/oarkflow/interpreter/plugins:
// http server/router, xql, images, database, money, phone, ip, wuid,
// naturaldate, pdf, crypto extras, yaml, templates, secrets, integrations,
// metadata, shamir, and more). It's its own Go module (separate go.mod,
// mirroring cmd/interpreter) because those plugins pull in heavy or
// private dependencies that the root github.com/oarkflow/interpreter
// module deliberately does not carry.
//
// Point the VS Code extension's spl.toolPath setting at a built copy of
// this binary (instead of the default lightweight cmd/spltool) if your
// .spl scripts use any plugin builtin, so completion/hover/diagnostics
// recognize them too.
package main

import (
	"os"

	"github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/spltoolcli"
	_ "github.com/oarkflow/interpreter/plugins"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--spl-worker" {
		os.Exit(interpreter.RunUntrustedWorker(os.Stdin, os.Stdout))
	}
	os.Exit(spltoolcli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
