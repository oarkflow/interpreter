package main

import (
	"os"

	"github.com/oarkflow/interpreter"
	_ "github.com/oarkflow/interpreter/pkg/builtins/tools"
)

func main() {
	os.Exit(interpreter.RunUntrustedWorker(os.Stdin, os.Stdout))
}
