package lua

import "fmt"

type Error struct {
	Source string
	Line   int
	Msg    string
}

func (e *Error) Error() string {
	if e.Source == "" {
		return e.Msg
	}
	if e.Line <= 0 {
		return e.Source + ": " + e.Msg
	}
	return fmt.Sprintf("%s:%d: %s", e.Source, e.Line, e.Msg)
}

func runtimeError(format string, args ...any) error {
	return &Error{Msg: fmt.Sprintf(format, args...)}
}
