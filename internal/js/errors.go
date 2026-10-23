package js

import "fmt"

// Error is a JavaScript error object thrown while evaluating: its constructor name (TypeError, RangeError,
// ReferenceError, SyntaxError or Error) and its message.
type Error struct {
	Name    string
	Message string
}

// Error returns String(error): the name, a colon and the message.
func (e *Error) Error() string {
	if e.Message == "" {
		return e.Name
	}
	return e.Name + ": " + e.Message
}

// Throw returns a new error of the given constructor name.
func Throw(name, format string, args ...any) *Error {
	return &Error{Name: name, Message: fmt.Sprintf(format, args...)}
}

func typeError(format string, args ...any) *Error {
	return Throw("TypeError", format, args...)
}

func rangeError(format string, args ...any) *Error {
	return Throw("RangeError", format, args...)
}
