package eval

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oarkflow/interpreter/pkg/lexer"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/parser"
)

// ---------------------------------------------------------------------------
// CLI configuration
// ---------------------------------------------------------------------------

var (
	cliTimeoutDur time.Duration
	cliMaxDepth   int
	cliMaxSteps   int64
	cliMaxHeapMB  int64
)

// RunFileFn is a hook that the root package can set to provide sandbox-aware
// file execution. If nil, StartCLI falls back to a basic evaluator.
var RunFileFn func(filename string, args []string)

// RunReplFn is a hook for the REPL. If nil, StartCLI uses a basic read-eval loop.
var RunReplFn func()

// StartCLI parses CLI flags and runs the interpreter.
func StartCLI() {
	rand.Seed(time.Now().UnixNano())
	timeout := flag.Duration("timeout", 0, "Execution timeout (0 = no limit)")
	maxDepth := flag.Int("max-depth", 0, "Max recursion depth (0 = unlimited)")
	maxSteps := flag.Int64("max-steps", 0, "Max evaluation steps (0 = unlimited)")
	maxHeapMB := flag.Int64("max-heap-mb", 0, "Max heap usage in MB (0 = unlimited)")
	flag.Parse()
	cliTimeoutDur = *timeout
	cliMaxDepth = *maxDepth
	cliMaxSteps = *maxSteps
	cliMaxHeapMB = *maxHeapMB

	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "Runtime Panic: %v\n", r)
			os.Exit(2)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nCancelling execution...")
		os.Exit(130)
	}()

	if cliTimeoutDur > 0 {
		time.AfterFunc(cliTimeoutDur, func() {
			fmt.Println("\nTimeout reached.")
			os.Exit(3)
		})
	}

	args := flag.Args()
	if len(args) > 0 {
		if RunFileFn != nil {
			RunFileFn(args[0], args[1:])
		} else {
			runFileBasic(args[0], args[1:])
		}
	} else {
		if RunReplFn != nil {
			RunReplFn()
		} else {
			fmt.Println("No file specified. Provide a filename to run.")
		}
	}
}

// runFileBasic is a minimal file runner when no sandbox hook is configured.
func runFileBasic(filename string, scriptArgs []string) {
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	env := object.NewGlobalEnvironment(scriptArgs)
	env.SourcePath = filename
	if cliMaxDepth > 0 || cliMaxSteps > 0 || cliMaxHeapMB > 0 || cliTimeoutDur > 0 {
		env.RuntimeLimits = &object.RuntimeLimits{
			HeapCheckEvery: 128,
		}
		if cliMaxDepth > 0 {
			env.RuntimeLimits.MaxDepth = cliMaxDepth
		}
		if cliMaxSteps > 0 {
			env.RuntimeLimits.MaxSteps = cliMaxSteps
		}
		if cliMaxHeapMB > 0 {
			env.RuntimeLimits.MaxHeapBytes = uint64(cliMaxHeapMB) * 1024 * 1024
		}
		if cliTimeoutDur > 0 {
			env.RuntimeLimits.Deadline = time.Now().Add(cliTimeoutDur)
		}
	}

	l := lexer.NewLexer(string(content))
	p := parser.NewParser(l)
	program := p.ParseProgram()

	if len(p.Errors()) != 0 {
		for _, msg := range p.Errors() {
			fmt.Fprintln(os.Stderr, msg)
		}
		os.Exit(1)
	}

	evaluated := Eval(program, env)
	if evaluated != nil {
		if object.IsError(evaluated) {
			fmt.Fprintf(os.Stderr, "ERROR: %s\n", objectErrorString(evaluated))
			os.Exit(1)
		} else if evaluated.Type() == object.RETURN_VALUE_OBJ {
			val := evaluated.(*object.ReturnValue).Value
			if val.Type() == object.INTEGER_OBJ {
				os.Exit(int(val.(*object.Integer).Value))
			}
		}
	}
}
