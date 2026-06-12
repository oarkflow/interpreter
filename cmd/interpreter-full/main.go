package main

import (
	"os"

	"github.com/oarkflow/interpreter"
	_ "github.com/oarkflow/interpreter/builtins/cryptoextra"
	_ "github.com/oarkflow/interpreter/builtins/database"
	_ "github.com/oarkflow/interpreter/builtins/images"
	_ "github.com/oarkflow/interpreter/builtins/integrations"
	_ "github.com/oarkflow/interpreter/builtins/tools"
	_ "github.com/oarkflow/interpreter/builtins/xql"
	_ "github.com/oarkflow/interpreter/config/yaml"
	_ "github.com/oarkflow/interpreter/pkg/builtins/reactive"
	_ "github.com/oarkflow/interpreter/pkg/builtins/scheduler"
	_ "github.com/oarkflow/interpreter/pkg/builtins/server"
	_ "github.com/oarkflow/interpreter/pkg/builtins/watcher"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--spl-worker" {
		os.Exit(interpreter.RunUntrustedWorker(os.Stdin, os.Stdout))
	}
	interpreter.StartCLI()
}
