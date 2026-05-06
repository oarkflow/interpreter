package main

import (
	"os"

	"github.com/oarkflow/interpreter"
)

func main() {
	os.Exit(interpreter.RunUntrustedWorker(os.Stdin, os.Stdout))
}
