package tooling

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/oarkflow/interpreter"
)

type DiagnosticSeverity string

const (
	SeverityError   DiagnosticSeverity = "error"
	SeverityWarning DiagnosticSeverity = "warning"
	SeverityInfo    DiagnosticSeverity = "info"
)

type Diagnostic struct {
	Severity DiagnosticSeverity `json:"severity"`
	Path     string             `json:"path,omitempty"`
	Line     int                `json:"line,omitempty"`
	Column   int                `json:"column,omitempty"`
	Message  string             `json:"message"`
	Snippet  string             `json:"snippet,omitempty"`
}

type Report struct {
	Path        string       `json:"path,omitempty"`
	OK          bool         `json:"ok"`
	Changed     bool         `json:"changed,omitempty"`
	Formatted   string       `json:"formatted,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

var parserLineColRe = regexp.MustCompile(`^Line (\d+)(?::(\d+))?`)

func CheckSource(path, src string) Report {
	return analyzeSource(path, src, false)
}

func FormatSource(path, src string) Report {
	return analyzeSource(path, src, true)
}

func Analyze(path, src string, wantFormat bool) Report {
	return analyzeSource(path, src, wantFormat)
}

func DiagnosticsJSON(diags []Diagnostic) ([]byte, error) {
	return json.MarshalIndent(diags, "", "  ")
}

func analyzeSource(path, src string, format bool) Report {
	report := Report{Path: path, OK: true}

	l := interpreter.NewLexer(src)
	p := interpreter.NewParser(l)
	_ = p.ParseProgram()
	if len(p.Errors()) != 0 {
		report.OK = false
		report.Diagnostics = diagnosticsFromParserErrors(path, src, p.Errors())
		return report
	}

	if !format {
		return report
	}

	formatted := formatProgram(src)
	report.Formatted = formatted
	report.Changed = normalizeWhitespace(src) != normalizeWhitespace(formatted)
	return report
}

func diagnosticsFromParserErrors(path, src string, errs []string) []Diagnostic {
	diags := make([]Diagnostic, 0, len(errs))
	for _, raw := range errs {
		line, col := parseLineCol(raw)
		diag := Diagnostic{
			Severity: SeverityError,
			Path:     path,
			Line:     line,
			Column:   col,
			Message:  raw,
		}
		if line > 0 {
			diag.Snippet = sourceSnippet(src, line, col)
		}
		diags = append(diags, diag)
	}
	return diags
}

func parseLineCol(msg string) (int, int) {
	m := parserLineColRe.FindStringSubmatch(msg)
	if m == nil {
		return 0, 0
	}
	var line, col int
	fmt.Sscanf(m[1], "%d", &line)
	if m[2] != "" {
		fmt.Sscanf(m[2], "%d", &col)
	}
	return line, col
}

func sourceSnippet(src string, line, col int) string {
	if line <= 0 {
		return ""
	}
	lines := strings.Split(src, "\n")
	if line > len(lines) {
		return ""
	}
	text := lines[line-1]
	if strings.TrimSpace(text) == "" {
		return ""
	}
	if col < 1 {
		col = 1
	}
	return fmt.Sprintf("%s\n%s^", text, strings.Repeat(" ", col-1))
}

func formatProgram(src string) string {
	var out strings.Builder
	indent := 0
	inString := false
	escaped := false
	needSpace := false

	writeIndent := func() {
		if out.Len() == 0 {
			return
		}
		last := out.String()
		if len(last) == 0 || last[len(last)-1] == '\n' {
			out.WriteString(strings.Repeat("  ", indent))
		}
	}

	flushSpace := func() {
		if needSpace {
			out.WriteByte(' ')
			needSpace = false
		}
	}

	for _, r := range src {
		if inString {
			out.WriteRune(r)
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				inString = false
			}
			continue
		}

		switch r {
		case '"':
			writeIndent()
			flushSpace()
			inString = true
			out.WriteRune(r)
		case '{':
			flushSpace()
			out.WriteString(" {")
			out.WriteByte('\n')
			indent++
			out.WriteString(strings.Repeat("  ", indent))
		case '}':
			trimTrailingSpace(&out)
			out.WriteByte('\n')
			if indent > 0 {
				indent--
			}
			out.WriteString(strings.Repeat("  ", indent))
			out.WriteByte('}')
			needSpace = true
		case ';':
			out.WriteByte(';')
			out.WriteByte('\n')
			out.WriteString(strings.Repeat("  ", indent))
			needSpace = false
		case '\n', '\r', '\t':
			needSpace = true
		default:
			if unicode.IsSpace(r) {
				needSpace = true
				continue
			}
			writeIndent()
			flushSpace()
			out.WriteRune(r)
		}
	}

	trimTrailingSpace(&out)
	formatted := strings.TrimSpace(out.String())
	if formatted == "" {
		return ""
	}
	return formatted + "\n"
}

func trimTrailingSpace(out *strings.Builder) {
	s := out.String()
	s = strings.TrimRight(s, " \t")
	out.Reset()
	out.WriteString(s)
}

func normalizeWhitespace(src string) string {
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func DefaultStdinPath() string {
	return filepath.Clean("<stdin>")
}
