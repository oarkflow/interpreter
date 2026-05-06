package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	case "config":
		return runConfig(args[1:], stdout, stderr)
	case "symbols":
		return runSymbols(args[1:], stdin, stdout, stderr)
	case "complete":
		return runComplete(args[1:], stdin, stdout, stderr)
	case "hover":
		return runHover(args[1:], stdin, stdout, stderr)
	case "docs":
		return runDocs(args[1:], stdin, stdout, stderr)
	case "test":
		return runTest(args[1:], stdout, stderr)
	case "lsp":
		return runLSPInfo(stdout)
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runConfig(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "show" {
		cfg, path, err := LoadProjectConfig("")
		if err != nil {
			fmt.Fprintf(stderr, "config error: %v\n", err)
			return 1
		}
		if path == "" {
			cfg = DefaultProjectConfig()
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(map[string]any{"path": path, "config": cfg}); err != nil {
			fmt.Fprintf(stderr, "failed to encode JSON: %v\n", err)
			return 1
		}
		return 0
	}
	switch args[0] {
	case "init":
		path := "spl.config.json"
		if len(args) > 1 {
			path = args[1]
		}
		if _, err := os.Stat(path); err == nil {
			fmt.Fprintf(stderr, "%s already exists\n", path)
			return 1
		}
		cfg := DefaultProjectConfig()
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "failed to encode config: %v\n", err)
			return 1
		}
		data = append(data, '\n')
		if err := os.WriteFile(path, data, 0o644); err != nil {
			fmt.Fprintf(stderr, "failed to write %s: %v\n", path, err)
			return 1
		}
		fmt.Fprintf(stdout, "created %s\n", path)
		return 0
	default:
		fmt.Fprintln(stderr, "usage: spltool config <init|show> [path]")
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

func runSymbols(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	targets := args
	if len(targets) == 0 {
		targets = []string{"-"}
	}
	all := []Symbol{}
	for _, target := range targets {
		path, src, err := readTarget(target, stdin)
		if err != nil {
			fmt.Fprintf(stderr, "symbols error: %v\n", err)
			return 1
		}
		all = append(all, SymbolsForSource(path, src)...)
	}
	return encodeJSON(stdout, stderr, all)
}

func runComplete(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("complete", flag.ContinueOnError)
	fs.SetOutput(stderr)
	prefix := fs.String("prefix", "", "completion prefix")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	target := "-"
	if len(fs.Args()) > 0 {
		target = fs.Args()[0]
	}
	path, src, err := readTarget(target, stdin)
	if err != nil {
		fmt.Fprintf(stderr, "complete error: %v\n", err)
		return 1
	}
	return encodeJSON(stdout, stderr, CompletionItems(path, src, *prefix))
}

func runHover(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("hover", flag.ContinueOnError)
	fs.SetOutput(stderr)
	line := fs.Int("line", 1, "1-based line")
	col := fs.Int("col", 1, "1-based column")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	target := "-"
	if len(fs.Args()) > 0 {
		target = fs.Args()[0]
	}
	path, src, err := readTarget(target, stdin)
	if err != nil {
		fmt.Fprintf(stderr, "hover error: %v\n", err)
		return 1
	}
	return encodeJSON(stdout, stderr, HoverAt(path, src, *line, *col))
}

func runDocs(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	targets := args
	if len(targets) == 0 {
		targets = []string{"-"}
	}
	for i, target := range targets {
		path, src, err := readTarget(target, stdin)
		if err != nil {
			fmt.Fprintf(stderr, "docs error: %v\n", err)
			return 1
		}
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		fmt.Fprint(stdout, DocsMarkdown(path, src))
	}
	return 0
}

type TestReport struct {
	OK       bool             `json:"ok"`
	Total    int              `json:"total"`
	Passed   int              `json:"passed"`
	Failed   int              `json:"failed"`
	Duration int64            `json:"duration_ms"`
	Results  []TestFileResult `json:"results"`
}

type TestFileResult struct {
	Path       string `json:"path"`
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

func runTest(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	filter := fs.String("filter", "", "substring filter for discovered test files")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	targets := fs.Args()
	if len(targets) == 0 {
		targets = []string{"."}
	}
	files, err := discoverTestFiles(targets, *filter)
	if err != nil {
		fmt.Fprintf(stderr, "test discovery error: %v\n", err)
		return 1
	}
	report := TestReport{OK: true, Total: len(files)}
	start := time.Now()
	for _, file := range files {
		itemStart := time.Now()
		_, err := interpreter.ExecFileWithOptions(file, nil, interpreter.ExecOptions{
			ModuleDir: filepath.Dir(file),
			MaxSteps:  2_000_000,
			MaxDepth:  256,
			Timeout:   10 * time.Second,
		})
		item := TestFileResult{Path: file, OK: err == nil, DurationMS: time.Since(itemStart).Milliseconds()}
		if err != nil {
			item.Error = err.Error()
			report.OK = false
			report.Failed++
		} else {
			report.Passed++
		}
		report.Results = append(report.Results, item)
	}
	report.Duration = time.Since(start).Milliseconds()
	if *jsonOut {
		return encodeJSON(stdout, stderr, report)
	}
	for _, result := range report.Results {
		if result.OK {
			fmt.Fprintf(stdout, "PASS %s (%dms)\n", result.Path, result.DurationMS)
		} else {
			fmt.Fprintf(stdout, "FAIL %s (%dms)\n%s\n", result.Path, result.DurationMS, result.Error)
		}
	}
	fmt.Fprintf(stdout, "\n%d passed, %d failed, %d total\n", report.Passed, report.Failed, report.Total)
	if !report.OK {
		return 1
	}
	return 0
}

func runLSPInfo(stdout io.Writer) int {
	_ = json.NewEncoder(stdout).Encode(map[string]any{
		"status": "helpers",
		"commands": []string{
			"spltool check --json",
			"spltool symbols",
			"spltool complete --prefix <text>",
			"spltool hover --line <n> --col <n>",
			"spltool fmt",
		},
		"note": "These JSON surfaces are stable building blocks for an editor language server.",
	})
	return 0
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
	fmt.Fprintln(w, "Usage: spltool <fmt|check|symbols|complete|hover|docs|test|config|mod|lsp> [flags] [files...]")
	fmt.Fprintln(w, "Use '-' to read from stdin.")
}

func encodeJSON(stdout, stderr io.Writer, v any) int {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(stderr, "failed to encode JSON: %v\n", err)
		return 1
	}
	return 0
}

func discoverTestFiles(targets []string, filter string) ([]string, error) {
	seen := map[string]struct{}{}
	files := []string{}
	add := func(path string) {
		path = filepath.Clean(path)
		if filter != "" && !strings.Contains(path, filter) {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		files = append(files, path)
	}
	for _, target := range targets {
		info, err := os.Stat(target)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if strings.HasSuffix(target, ".spl") {
				add(target)
			}
			continue
		}
		if err := filepath.WalkDir(target, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == ".git" || d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(d.Name(), "_test.spl") || strings.Contains(filepath.ToSlash(path), "/tests/") && strings.HasSuffix(d.Name(), ".spl") {
				add(path)
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return files, nil
}
