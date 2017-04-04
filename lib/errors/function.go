package errors

import (
	"fmt"
	"strings"
)

// Newf creates a new raw error and trace it.
func Newf(format string, args ...interface{}) error {
	err := &wrap{
		previous: fmt.Errorf(format, args...),
	}
	err.setLocation(1)
	return err
}

// Trace attach a location to the error. It should be called each time an error
// is returned. If the error is nil, it returns nil.
func Trace(other error) error {
	if other == nil {
		return nil
	}
	err := &wrap{
		previous: other,
	}
	err.setLocation(1)
	return err
}

// Tracef attach a location and an annotation to the error. If the error is nil
// it returns nil.
func Tracef(other error, format string, args ...interface{}) error {
	if other == nil {
		return nil
	}
	err := &wrap{
		traceMessage: fmt.Sprintf(format, args...),
		previous:     other,
	}
	err.setLocation(1)
	return err
}

// Cause returns the underlying cause error of the passed error if it exists.
func Cause(e error) error {
	switch e := e.(type) {
	case *wrap:
		return e.Cause()
	case UserError:
		return e.Cause()
	default:
		return e
	}
}

// ExtractUserError returns the underlying UserError or nil otherwise
func ExtractUserError(err error) UserError {
	if err == nil {
		return nil
	}
	switch e := err.(type) {
	case *wrap:
		return e.UserError()
	case UserError:
		return e
	default:
		return nil
	}
}

// ErrorStack returns the full stack of information attached to this error.
func ErrorStack(err error) []string {
	if err == nil {
		return []string{}
	}

	var lines []string
	for {
		var buff []byte
		if e, ok := err.(*wrap); ok {
			buff = append(buff, fmt.Sprintf("[trace] ")...)
			file, line := e.location()
			if file != "" {
				buff = append(buff, fmt.Sprintf("%s:%d", file, line)...)
			}
			if len(e.traceMessage) > 0 {
				buff = append(buff, fmt.Sprintf(": %s", e.traceMessage)...)
			}
			err = e.previous
		} else {
			buff = append(buff, fmt.Sprintf("[error] ")...)
			buff = append(buff, err.Error()...)
			err = nil
		}
		if len(string(buff)) > 0 {
			lines = append(lines, string(buff))
		}
		if err == nil {
			break
		}
	}

	// reverse the lines to get the original error, which was at the end of
	// the list, back to the start.
	var result []string
	for i := len(lines); i > 0; i-- {
		result = append(result, lines[i-1])
	}
	return result
}

// Details returned a formatted ErrorStack string
func Details(err error) string {
	return strings.Join(ErrorStack(err), "\n")
}
