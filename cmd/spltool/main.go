package main

import (
	"os"

	"github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/spltoolcli"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--spl-worker" {
		os.Exit(interpreter.RunUntrustedWorker(os.Stdin, os.Stdout))
	}
	os.Exit(spltoolcli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
