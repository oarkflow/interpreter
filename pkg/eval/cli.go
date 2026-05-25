package eval

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strings"
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

type CLIOptions struct {
	Profile                string
	RequireOSIsolation     bool
	AllowInProcessFallback bool
	AllowedCapabilities    []string
	AllowedExecCommands    []string
	AllowedNetworkHosts    []string
	AllowedDBDrivers       []string
	AllowedDBDSNPatterns   []string
	AllowedFileReadPaths   []string
	AllowedFileWritePaths  []string
	AllowedImportPaths     []string
	DeniedImportPaths      []string
	AllowedImportPackages  []string
	DeniedImportPackages   []string
	DenyDynamicImports     bool
	Timeout                time.Duration
	MaxDepth               int
	MaxSteps               int64
	MaxHeapMB              int64
	MaxSourceBytes         int64
	MaxStringBytes         int64
	MaxArrayLength         int
	MaxHashEntries         int
	MaxImportDepth         int
	MaxImportCount         int
}

// RunFileFn is a hook that the root package can set to provide sandbox-aware
// file execution. If nil, StartCLI falls back to a basic evaluator.
var RunFileFn func(filename string, args []string, opts CLIOptions)

// RunReplFn is a hook for the REPL. If nil, StartCLI uses a basic read-eval loop.
var RunReplFn func(opts CLIOptions)

// StartCLI parses CLI flags and runs the interpreter.
func StartCLI() {
	rand.Seed(time.Now().UnixNano())
	timeout := flag.Duration("timeout", 0, "Execution timeout (0 = no limit)")
	maxDepth := flag.Int("max-depth", 0, "Max recursion depth (0 = unlimited)")
	maxSteps := flag.Int64("max-steps", 0, "Max evaluation steps (0 = unlimited)")
	maxHeapMB := flag.Int64("max-heap-mb", 0, "Max heap usage in MB (0 = unlimited)")
	maxSourceBytes := flag.Int64("max-source-bytes", 0, "Max source size in bytes for untrusted execution (0 = default)")
	maxStringBytes := flag.Int64("max-string-bytes", 0, "Max string literal size in bytes (0 = unlimited)")
	maxArrayLength := flag.Int("max-array-length", 0, "Max array literal length (0 = unlimited)")
	maxHashEntries := flag.Int("max-hash-entries", 0, "Max hash literal entries (0 = unlimited)")
	maxImportDepth := flag.Int("max-import-depth", 0, "Max module import depth (0 = unlimited)")
	maxImportCount := flag.Int("max-import-count", 0, "Max module import count (0 = unlimited)")
	profile := flag.String("profile", "trusted", "Execution profile: trusted or untrusted")
	requireOSIsolation := flag.Bool("require-os-isolation", false, "Require OS process isolation for untrusted execution")
	allowInProcessFallback := flag.Bool("allow-in-process-fallback", false, "Allow in-process fallback if the untrusted worker cannot start")
	allowCap := flag.String("allow-cap", "", "Comma-separated untrusted capability allowlist")
	allowExec := flag.String("allow-exec", "", "Comma-separated exec command allowlist")
	allowNetwork := flag.String("allow-network", "", "Comma-separated network host allowlist")
	allowDBDriver := flag.String("allow-db-driver", "", "Comma-separated DB driver allowlist")
	allowDBDSN := flag.String("allow-db-dsn", "", "Comma-separated DB DSN pattern allowlist")
	allowRead := flag.String("allow-read", "", "Comma-separated file read root allowlist")
	allowWrite := flag.String("allow-write", "", "Comma-separated file write root allowlist")
	allowImportPath := flag.String("allow-import-path", "", "Comma-separated import path root allowlist")
	denyImportPath := flag.String("deny-import-path", "", "Comma-separated import path root denylist")
	allowImportPackage := flag.String("allow-import-package", "", "Comma-separated import package allowlist")
	denyImportPackage := flag.String("deny-import-package", "", "Comma-separated import package denylist")
	denyDynamicImports := flag.Bool("deny-dynamic-imports", false, "Deny imports whose path is not a string literal")
	flag.Parse()
	cliTimeoutDur = *timeout
	cliMaxDepth = *maxDepth
	cliMaxSteps = *maxSteps
	cliMaxHeapMB = *maxHeapMB
	opts := CLIOptions{
		Profile:                *profile,
		RequireOSIsolation:     *requireOSIsolation,
		AllowInProcessFallback: *allowInProcessFallback,
		AllowedCapabilities:    parseCSVFlag(*allowCap),
		AllowedExecCommands:    parseCSVFlag(*allowExec),
		AllowedNetworkHosts:    parseCSVFlag(*allowNetwork),
		AllowedDBDrivers:       parseCSVFlag(*allowDBDriver),
		AllowedDBDSNPatterns:   parseCSVFlag(*allowDBDSN),
		AllowedFileReadPaths:   parseCSVFlag(*allowRead),
		AllowedFileWritePaths:  parseCSVFlag(*allowWrite),
		AllowedImportPaths:     parseCSVFlag(*allowImportPath),
		DeniedImportPaths:      parseCSVFlag(*denyImportPath),
		AllowedImportPackages:  parseCSVFlag(*allowImportPackage),
		DeniedImportPackages:   parseCSVFlag(*denyImportPackage),
		DenyDynamicImports:     *denyDynamicImports,
		Timeout:                *timeout,
		MaxDepth:               *maxDepth,
		MaxSteps:               *maxSteps,
		MaxHeapMB:              *maxHeapMB,
		MaxSourceBytes:         *maxSourceBytes,
		MaxStringBytes:         *maxStringBytes,
		MaxArrayLength:         *maxArrayLength,
		MaxHashEntries:         *maxHashEntries,
		MaxImportDepth:         *maxImportDepth,
		MaxImportCount:         *maxImportCount,
	}

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
			RunFileFn(args[0], args[1:], opts)
		} else {
			runFileBasic(args[0], args[1:])
		}
	} else {
		if RunReplFn != nil {
			RunReplFn(opts)
		} else {
			fmt.Println("No file specified. Provide a filename to run.")
		}
	}
}

func parseCSVFlag(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
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
