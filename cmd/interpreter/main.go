package main

import (
	"os"

	"github.com/oarkflow/interpreter"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--spl-worker" {
		os.Exit(interpreter.RunUntrustedWorker(os.Stdin, os.Stdout))
	}
	interpreter.StartCLI()
}
