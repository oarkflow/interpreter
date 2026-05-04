package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func run(label string, args ...string) error {
	fmt.Printf("\n== %s ==\n", label)
	cmd := exec.Command("go", args...)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Printf("$ go %s\n", strings.Join(args, " "))
	return cmd.Run()
}

func main() {
	if err := run("Full benchmark suite", "test", "./...", "-run", "^$", "-bench", ".", "-benchmem", "-count", "5"); err != nil {
		fmt.Fprintf(os.Stderr, "benchmark run failed: %v\n", err)
		os.Exit(1)
	}

	if err := run("Allocation hot paths", "test", "./...", "-run", "^$", "-bench", "Benchmark(EvalRunOnlyPreparsed|ImportCached|BuiltinsStringAndJSON)", "-benchmem", "-count", "10"); err != nil {
		fmt.Fprintf(os.Stderr, "focused benchmark run failed: %v\n", err)
		os.Exit(1)
	}

	if err := run("Competitor interpreter suite", "test", ".", "-run", "^$", "-bench", "^BenchmarkCompetitor", "-benchmem", "-count", "5"); err != nil {
		fmt.Fprintf(os.Stderr, "competitor benchmark run failed: %v\n", err)
		os.Exit(1)
	}
}
