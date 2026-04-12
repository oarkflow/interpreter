package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/oarkflow/interpreter"
)

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "fmt":
		return runFmt(args[1:], stdin, stdout, stderr)
	case "check":
		return runCheck(args[1:], stdin, stdout, stderr)
	case "mod":
		return runMod(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runMod(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: spltool mod <init|tidy>")
		return 2
	}
	switch args[0] {
	case "init":
		fs := flag.NewFlagSet("mod init", flag.ContinueOnError)
		fs.SetOutput(stderr)
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		projectDir, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "failed to get cwd: %v\n", err)
			return 1
		}
		moduleName := ""
		if len(fs.Args()) > 0 {
			moduleName = fs.Args()[0]
		}
		if _, err := interpreter.InitModuleManifest(projectDir, moduleName); err != nil {
			fmt.Fprintf(stderr, "failed to write %s: %v\n", interpreter.SPLManifestFileName, err)
			return 1
		}
		fmt.Fprintf(stdout, "created %s\n", filepath.Join(projectDir, interpreter.SPLManifestFileName))
		return 0
	case "tidy":
		fs := flag.NewFlagSet("mod tidy", flag.ContinueOnError)
		fs.SetOutput(stderr)
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		projectDir, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "failed to get cwd: %v\n", err)
			return 1
		}
		lock, err := interpreter.SyncModuleLock(projectDir)
		if err != nil {
			fmt.Fprintf(stderr, "failed to sync %s: %v\n", interpreter.SPLLockFileName, err)
			return 1
		}
		fmt.Fprintf(stdout, "synced %s with %d dependencies\n", interpreter.SPLLockFileName, len(lock.Dependencies))
		return 0
	default:
		fmt.Fprintf(stderr, "unknown mod command %q\n", args[0])
		return 2
	}
}

func runFmt(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("fmt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	write := fs.Bool("w", false, "write result back to files")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	targets := fs.Args()
	if len(targets) == 0 {
		targets = []string{"-"}
	}

	reports, code := processTargets(targets, stdin, *write, true)
	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(reports); err != nil {
			fmt.Fprintf(stderr, "failed to encode JSON: %v\n", err)
			return 1
		}
		return code
	}

	for _, rep := range reports {
		if len(rep.Diagnostics) > 0 {
			for _, d := range rep.Diagnostics {
				fmt.Fprintln(stderr, formatDiagnostic(d))
			}
			continue
		}
		if *write {
			if rep.Changed {
				fmt.Fprintf(stdout, "formatted %s\n", rep.Path)
			}
			continue
		}
		if rep.Formatted != "" {
			fmt.Fprint(stdout, rep.Formatted)
		}
	}
	return code
}

func runCheck(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	targets := fs.Args()
	if len(targets) == 0 {
		targets = []string{"-"}
	}

	reports, code := processTargets(targets, stdin, false, false)
	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(reports); err != nil {
			fmt.Fprintf(stderr, "failed to encode JSON: %v\n", err)
			return 1
		}
		return code
	}

	for _, rep := range reports {
		for _, d := range rep.Diagnostics {
			fmt.Fprintln(stderr, formatDiagnostic(d))
		}
	}
	return code
}

func processTargets(targets []string, stdin io.Reader, write bool, format bool) ([]Report, int) {
	reports := make([]Report, 0, len(targets))
	exitCode := 0
	for _, target := range targets {
		path, src, err := readTarget(target, stdin)
		if err != nil {
			exitCode = 1
			reports = append(reports, Report{
				Path: target,
				OK:   false,
				Diagnostics: []Diagnostic{{
					Severity: SeverityError,
					Path:     target,
					Message:  err.Error(),
				}},
			})
			continue
		}
		if format {
			report := FormatSource(path, src)
			reports = append(reports, report)
			if len(report.Diagnostics) > 0 {
				exitCode = 1
				continue
			}
			if write && report.Changed && target != "-" {
				if err := os.WriteFile(path, []byte(report.Formatted), 0o644); err != nil {
					exitCode = 1
					reports[len(reports)-1].OK = false
					reports[len(reports)-1].Diagnostics = append(reports[len(reports)-1].Diagnostics, Diagnostic{
						Severity: SeverityError,
						Path:     path,
						Message:  err.Error(),
					})
				}
			}
			continue
		}
		report := CheckSource(path, src)
		reports = append(reports, report)
		if !report.OK {
			exitCode = 1
		}
	}
	return reports, exitCode
}

func readTarget(target string, stdin io.Reader) (string, string, error) {
	if target == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", "", err
		}
		return DefaultStdinPath(), string(data), nil
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", "", err
	}
	return filepath.Clean(target), string(data), nil
}

func formatDiagnostic(d Diagnostic) string {
	var b strings.Builder
	b.WriteString(string(d.Severity))
	b.WriteString(": ")
	if d.Path != "" {
		b.WriteString(d.Path)
		if d.Line > 0 {
			fmt.Fprintf(&b, ":%d", d.Line)
			if d.Column > 0 {
				fmt.Fprintf(&b, ":%d", d.Column)
			}
		}
		b.WriteString(": ")
	}
	b.WriteString(d.Message)
	if d.Snippet != "" {
		b.WriteByte('\n')
		b.WriteString(d.Snippet)
	}
	return b.String()
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: spltool <fmt|check|mod> [flags] [files...]")
	fmt.Fprintln(w, "Use '-' to read from stdin.")
}
