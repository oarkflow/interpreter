package main

import (
	"io"
	"time"

	"github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/tooling"
)

type DiagnosticSeverity = tooling.DiagnosticSeverity
type Diagnostic = tooling.Diagnostic
type Report = tooling.Report
type ProjectConfig = tooling.ProjectConfig
type Symbol = tooling.Symbol
type CompletionItem = tooling.CompletionItem
type HoverInfo = tooling.HoverInfo
type Location = tooling.Location
type Reference = tooling.Reference
type IndexedDocument = tooling.IndexedDocument
type WorkspaceIndex = tooling.WorkspaceIndex
type EvaluationOptions = tooling.EvaluationOptions
type EvaluationResult = tooling.EvaluationResult

const (
	SeverityError   = tooling.SeverityError
	SeverityWarning = tooling.SeverityWarning
	SeverityInfo    = tooling.SeverityInfo
)

var (
	CheckSource              = tooling.CheckSource
	FormatSource             = tooling.FormatSource
	Analyze                  = tooling.Analyze
	DiagnosticsJSON          = tooling.DiagnosticsJSON
	LoadProjectConfig        = tooling.LoadProjectConfig
	DefaultProjectConfig     = tooling.DefaultProjectConfig
	StaticDiagnostics        = tooling.StaticDiagnostics
	SymbolsForSource         = tooling.SymbolsForSource
	CompletionItems          = tooling.CompletionItems
	DocsMarkdown             = tooling.DocsMarkdown
	HoverAt                  = tooling.HoverAt
	DefaultStdinPath         = tooling.DefaultStdinPath
	NewWorkspaceIndex        = tooling.NewWorkspaceIndex
	WorkspaceCompletionItems = tooling.WorkspaceCompletionItems
	KeywordMarkdown          = tooling.KeywordMarkdown
	RuntimeDocMarkdown       = tooling.RuntimeDocMarkdown
	HoverMarkdown            = tooling.HoverMarkdown
	EvaluateSPL              = tooling.EvaluateSPL
)

func init() {
	tooling.ExecuteSPLFn = func(path, src string, opts tooling.EvaluationOptions, output io.Writer) (string, error) {
		timeout := time.Duration(opts.TimeoutMS) * time.Millisecond
		obj, err := interpreter.ExecWithOptions(src, nil, interpreter.ExecOptions{
			Profile:                opts.Profile,
			ModuleDir:              tooling.ModuleDirForPath(path),
			Timeout:                timeout,
			MaxSteps:               opts.MaxSteps,
			MaxDepth:               opts.MaxDepth,
			MaxOutputBytes:         opts.MaxOutputBytes,
			Output:                 output,
			AllowInProcessFallback: true,
		})
		if err != nil || obj == nil {
			return "", err
		}
		return obj.Inspect(), nil
	}
}

func cleanAbsPath(path string) string {
	return tooling.CleanAbsPath(path)
}

func pathToURI(path string) string {
	return tooling.PathToURI(path)
}

func uriToPath(uri string) string {
	return tooling.URIToPath(uri)
}

func prefixAt(src string, line, col int) string {
	return tooling.PrefixAt(src, line, col)
}

func wordAt(src string, line, col int) string {
	return tooling.WordAt(src, line, col)
}

func runeColumn(s string, byteOffset int) int {
	return tooling.RuneColumn(s, byteOffset)
}

func completionDetail(item CompletionItem) string {
	return tooling.CompletionDetail(item)
}
