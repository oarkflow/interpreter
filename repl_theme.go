package interpreter

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode"
)

const (
	colorReset   = "\033[0m"
	colorBold    = "\033[1m"
	colorDim     = "\033[2m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorGray    = "\033[90m"
)

func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	term := os.Getenv("TERM")
	if term == "" || term == "dumb" {
		return false
	}
	return isTerminal(os.Stdout)
}

func paint(s, code string) string {
	if !colorEnabled() {
		return s
	}
	return code + s + colorReset
}

func stylePrompt(p string) string {
	return paint(p, colorBold+colorCyan)
}

func styleContinuationPrompt(p string) string {
	return paint(p, colorBold+colorBlue)
}

func colorizeInputLine(line string) string {
	if !colorEnabled() {
		return line
	}

	kw := map[string]struct{}{
		"let": {}, "if": {}, "else": {}, "while": {}, "for": {}, "in": {}, "break": {}, "continue": {},
		"function": {}, "return": {}, "print": {}, "const": {}, "import": {}, "export": {},
		"true": {}, "false": {}, "null": {}, "do": {},
		"try": {}, "catch": {}, "throw": {},
		"switch": {}, "case": {}, "default": {},
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
			if looksLikeDateString(token) {
				out.WriteString(paint(token, colorCyan))
			} else {
				out.WriteString(paint(token, colorGreen))
			}
			continue
		}

		if unicode.IsDigit(r) {
			start := i
			i++
			for i < len(runes) && (unicode.IsDigit(runes[i]) || runes[i] == '.') {
				i++
			}
			out.WriteString(paint(string(runes[start:i]), colorYellow))
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
				out.WriteString(paint(tok, colorCyan))
				continue
			}
			if _, ok := builtins[tok]; ok {
				out.WriteString(paint(tok, colorMagenta))
				continue
			}
			j := i
			for j < len(runes) && unicode.IsSpace(runes[j]) {
				j++
			}
			if j < len(runes) && runes[j] == '(' {
				out.WriteString(paint(tok, colorBlue))
				continue
			}
			out.WriteString(tok)
			continue
		}

		if strings.ContainsRune("=+-*/%<>!&|", r) {
			out.WriteString(paint(string(r), colorGray))
			i++
			continue
		}

		out.WriteRune(r)
		i++
	}
	return out.String()
}

func looksLikeDateString(s string) bool {
	raw := strings.Trim(s, "\"")
	if len(raw) >= 10 && raw[4] == '-' && raw[7] == '-' {
		return true
	}
	return strings.Contains(raw, "T") && strings.Contains(raw, ":")
}

func formatObjectPlain(obj Object) string {
	return formatObjectPlainDepth(obj, 0)
}

func formatObjectPlainDepth(obj Object, depth int) string {
	if obj == nil {
		return "null"
	}
	switch v := obj.(type) {
	case *OwnedValue:
		return formatObjectPlainDepth(v.inner, depth)
	case *ImmutableValue:
		return formatObjectPlainDepth(v.inner, depth)
	case *GeneratorValue:
		return formatObjectPlainDepth(&Array{Elements: v.elements}, depth)
	case *Array:
		return formatArrayPlain(v, depth)
	case *Hash:
		return formatHashPlain(v, depth)
	case *Function:
		return v.Inspect()
	default:
		return obj.Inspect()
	}
}

func formatArrayPlain(arr *Array, depth int) string {
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

func formatHashPlain(h *Hash, depth int) string {
	if h == nil || len(h.Pairs) == 0 {
		return "{}"
	}
	indent := strings.Repeat("  ", depth)
	childIndent := strings.Repeat("  ", depth+1)
	keys := make([]string, 0, len(h.Pairs))
	keyToPair := make(map[string]HashPair, len(h.Pairs))
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

func formatObjectForDisplay(obj Object) string {
	if obj == nil {
		return ""
	}
	switch v := obj.(type) {
	case *Error:
		return paint("ERROR: "+v.Message, colorBold+colorRed)
	case *String:
		if looksLikeDateString(v.Inspect()) {
			return paint(v.Inspect(), colorCyan)
		}
		return paint(v.Inspect(), colorGreen)
	case *Integer, *Float:
		return paint(obj.Inspect(), colorYellow)
	case *Boolean:
		return paint(obj.Inspect(), colorCyan)
	case *Array, *Hash:
		return paint(formatObjectPlain(obj), colorBlue)
	case *Null:
		return paint(obj.Inspect(), colorGray)
	case *Function:
		return paint(formatObjectPlain(obj), colorBlue)
	default:
		return fmt.Sprint(formatObjectPlain(obj))
	}
}
