package lua

import (
	"fmt"
	"strconv"
	"strings"
)

type Error struct {
	Source string
	Line   int
	Msg    string
}

type luaValueError struct{ value Value }

func (e *luaValueError) Error() string { return e.value.Repr() }

func errorValue(err error) Value {
	if valueErr, ok := err.(*luaValueError); ok {
		return valueErr.value
	}
	return String(err.Error())
}

func (e *Error) Error() string {
	if e.Source == "" {
		return e.Msg
	}
	source := formatErrorSource(e.Source)
	message := formatSyntaxMessage(e.Msg)
	if e.Line <= 0 {
		return source + ": " + message
	}
	return fmt.Sprintf("%s:%d: %s", source, e.Line, message)
}

func formatErrorSource(source string) string {
	if strings.HasPrefix(source, "@") || strings.HasPrefix(source, "=") {
		return source[1:]
	}
	text := strings.ReplaceAll(source, "\n", " ")
	if len(text) > 48 {
		text = text[:45] + "..."
	}
	return "[string " + strconv.Quote(text) + "]"
}

func formatSyntaxMessage(message string) string {
	if rest, ok := strings.CutPrefix(message, "unexpected token "); ok {
		if token, err := strconv.Unquote(rest); err == nil {
			return fmt.Sprintf("unexpected symbol near '%s'", token)
		}
	}
	if before, quoted, ok := strings.Cut(message, ", got "); ok {
		if token, err := strconv.Unquote(quoted); err == nil {
			if token == "" {
				token = "<eof>"
			}
			return fmt.Sprintf("%s near '%s'", before, token)
		}
	}
	return message
}

func runtimeError(format string, args ...any) error {
	return &Error{Msg: fmt.Sprintf(format, args...)}
}
