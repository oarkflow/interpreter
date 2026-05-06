package repl

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode"

	"github.com/oarkflow/interpreter/pkg/object"
	renderpkg "github.com/oarkflow/interpreter/pkg/render"
)

// ---------------------------------------------------------------------------
// ANSI colour constants
// ---------------------------------------------------------------------------

const (
	ColorReset   = "\033[0m"
	ColorBold    = "\033[1m"
	ColorDim     = "\033[2m"
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorGray    = "\033[90m"
)

// ---------------------------------------------------------------------------
// Function variables – set by the host package
// ---------------------------------------------------------------------------

// BuiltinNames returns all registered builtin names.
var BuiltinNames func() map[string]struct{}

// ColorEnabled checks whether the terminal supports colours.
func ColorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	term := os.Getenv("TERM")
	if term == "" || term == "dumb" {
		return false
	}
	return IsTerminal(os.Stdout)
}

// Paint wraps s in ANSI colour codes when colours are enabled.
func Paint(s, code string) string {
	if !ColorEnabled() {
		return s
	}
	return code + s + ColorReset
}

// StylePrompt styles the primary REPL prompt.
func StylePrompt(p string) string {
	return Paint(p, ColorBold+ColorCyan)
}

// StyleContinuationPrompt styles the continuation prompt.
func StyleContinuationPrompt(p string) string {
	return Paint(p, ColorBold+ColorBlue)
}

// ColorizeInputLine applies syntax highlighting to a single input line.
func ColorizeInputLine(line string) string {
	if !ColorEnabled() {
		return line
	}

	kw := map[string]struct{}{
		"let": {}, "if": {}, "else": {}, "while": {}, "for": {}, "in": {}, "break": {}, "continue": {},
		"function": {}, "return": {}, "print": {}, "const": {}, "import": {}, "export": {},
		"true": {}, "false": {}, "null": {}, "do": {},
		"try": {}, "catch": {}, "throw": {},
		"switch": {}, "case": {}, "default": {},
	}

	builtinMap := map[string]struct{}{}
	if BuiltinNames != nil {
		builtinMap = BuiltinNames()
	}

	runes := []rune(line)
	var out strings.Builder
	i := 0
	for i < len(runes) {
		r := runes[i]

		if r == '"' {
			start := i
			i++
			for i < len(runes) {
				if runes[i] == '\\' && i+1 < len(runes) {
					i += 2
					continue
				}
				if runes[i] == '"' {
					i++
					break
				}
				i++
			}
			token := string(runes[start:i])
			if LooksLikeDateString(token) {
				out.WriteString(Paint(token, ColorCyan))
			} else {
				out.WriteString(Paint(token, ColorGreen))
			}
			continue
		}

		if unicode.IsDigit(r) {
			start := i
			i++
			for i < len(runes) && (unicode.IsDigit(runes[i]) || runes[i] == '.') {
				i++
			}
			out.WriteString(Paint(string(runes[start:i]), ColorYellow))
			continue
		}

		if unicode.IsLetter(r) || r == '_' || r == ':' {
			start := i
			i++
			for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_' || runes[i] == ':') {
				i++
			}
			tok := string(runes[start:i])
			if _, ok := kw[tok]; ok {
				out.WriteString(Paint(tok, ColorCyan))
				continue
			}
			if _, ok := builtinMap[tok]; ok {
				out.WriteString(Paint(tok, ColorMagenta))
				continue
			}
			j := i
			for j < len(runes) && unicode.IsSpace(runes[j]) {
				j++
			}
			if j < len(runes) && runes[j] == '(' {
				out.WriteString(Paint(tok, ColorBlue))
				continue
			}
			out.WriteString(tok)
			continue
		}

		if strings.ContainsRune("=+-*/%<>!&|", r) {
			out.WriteString(Paint(string(r), ColorGray))
			i++
			continue
		}

		out.WriteRune(r)
		i++
	}
	return out.String()
}

// LooksLikeDateString checks if a string looks like a date literal.
func LooksLikeDateString(s string) bool {
	raw := strings.Trim(s, "\"")
	if len(raw) >= 10 && raw[4] == '-' && raw[7] == '-' {
		return true
	}
	return strings.Contains(raw, "T") && strings.Contains(raw, ":")
}

// ---------------------------------------------------------------------------
// Object formatting for display
// ---------------------------------------------------------------------------

// FormatObjectPlain produces a plain-text representation of an object,
// suitable for REPL output.
func FormatObjectPlain(obj object.Object) string {
	return formatObjectPlainDepth(obj, 0)
}

func formatObjectPlainDepth(obj object.Object, depth int) string {
	if obj == nil {
		return "null"
	}
	switch v := obj.(type) {
	case *object.OwnedValue:
		return formatObjectPlainDepth(v.Inner, depth)
	case *object.ImmutableValue:
		return formatObjectPlainDepth(v.Inner, depth)
	case *object.GeneratorValue:
		return formatObjectPlainDepth(&object.Array{Elements: v.Elements}, depth)
	case *object.Array:
		return formatArrayPlain(v, depth)
	case *object.Hash:
		return formatHashPlain(v, depth)
	case *object.Function:
		return v.Inspect()
	default:
		return obj.Inspect()
	}
}

func formatArrayPlain(arr *object.Array, depth int) string {
	if arr == nil || len(arr.Elements) == 0 {
		return "[]"
	}
	indent := strings.Repeat("  ", depth)
	childIndent := strings.Repeat("  ", depth+1)
	parts := make([]string, 0, len(arr.Elements))
	for _, el := range arr.Elements {
		parts = append(parts, childIndent+formatObjectPlainDepth(el, depth+1))
	}
	return "[\n" + strings.Join(parts, ",\n") + "\n" + indent + "]"
}

func formatHashPlain(h *object.Hash, depth int) string {
	if h == nil || len(h.Pairs) == 0 {
		return "{}"
	}
	indent := strings.Repeat("  ", depth)
	childIndent := strings.Repeat("  ", depth+1)
	keys := make([]string, 0, len(h.Pairs))
	keyToPair := make(map[string]object.HashPair, len(h.Pairs))
	for _, pair := range h.Pairs {
		key := pair.Key.Inspect()
		keys = append(keys, key)
		keyToPair[key] = pair
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		pair := keyToPair[key]
		parts = append(parts, childIndent+key+": "+formatObjectPlainDepth(pair.Value, depth+1))
	}
	return "{\n" + strings.Join(parts, ",\n") + "\n" + indent + "}"
}

// FormatObjectForDisplay returns a colourised representation of an object
// for interactive REPL display.
func FormatObjectForDisplay(obj object.Object) string {
	return FormatObjectForDisplayWithEnv(obj, nil)
}

func FormatObjectForDisplayWithEnv(obj object.Object, env *object.Environment) string {
	if obj == nil {
		return ""
	}
	switch v := obj.(type) {
	case *object.Error:
		return Paint("ERROR: "+v.Message, ColorBold+ColorRed)
	case *object.RenderArtifact:
		text, err := renderpkg.RenderObjectForTerminal(env, v)
		if err != nil {
			return Paint("render error: "+err.Error(), ColorBold+ColorRed)
		}
		return text
	case *object.String:
		if LooksLikeDateString(v.Inspect()) {
			return Paint(v.Inspect(), ColorCyan)
		}
		return Paint(v.Inspect(), ColorGreen)
	case *object.Integer, *object.Float:
		return Paint(obj.Inspect(), ColorYellow)
	case *object.Boolean:
		return Paint(obj.Inspect(), ColorCyan)
	case *object.Array, *object.Hash:
		return Paint(FormatObjectPlain(obj), ColorBlue)
	case *object.Null:
		return Paint(obj.Inspect(), ColorGray)
	case *object.Function:
		return Paint(FormatObjectPlain(obj), ColorBlue)
	default:
		_ = v
		return fmt.Sprint(FormatObjectPlain(obj))
	}
}

// IsTerminal returns true if the given file is connected to a terminal.
func IsTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
