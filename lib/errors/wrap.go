package errors

import (
	"runtime"
)

// wrap is the internal error type used to attach information to an underlying
// error. As information is attached, the error is wrapped in wrap structures
// each containing details about the error.
type wrap struct {
	traceFile    string
	traceLine    int
	traceMessage string
	previous     error
}

func (e *wrap) UserError() UserError {
	switch e := e.previous.(type) {
	case *wrap:
		return e.UserError()
	case UserError:
		return e
	default:
		return nil
	}
}

// Cause returns the underlying error if not nil
func (e *wrap) Cause() error {
	switch e := e.previous.(type) {
	case *wrap:
		return e.Cause()
	case UserError:
		return e.Cause()
	default:
		return e
	}
}

// Error returns the error message of the underlying error if not nil otherwise
// it returns the error stack consumable message.
func (e *wrap) Error() string {
	err := e.Cause()
	if err != nil {
		return err.Error()
	}
	return ""
}

// StackTrace returns the full stack of information attached to the error
func (e *wrap) StackTrace() []string {
	return ErrorStack(e)
}

func (e *wrap) setLocation(callDepth int) {
	_, file, line, _ := runtime.Caller(callDepth + 1)
	e.traceFile = file
	e.traceLine = line
}

func (e *wrap) location() (filename string, line int) {
	return e.traceFile, e.traceLine
}
