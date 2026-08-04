package lua

import "fmt"

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
	if e.Line <= 0 {
		return e.Source + ": " + e.Msg
	}
	return fmt.Sprintf("%s:%d: %s", e.Source, e.Line, e.Msg)
}

func runtimeError(format string, args ...any) error {
	return &Error{Msg: fmt.Sprintf(format, args...)}
}
