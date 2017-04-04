package errors

import "fmt"

// UserError is the error interface that can be returned by the API
type UserError interface {
	Cause() error
	Status() int
	Code() string
	Message() string
	Error() string
}

// ConcreteUserError is a concrete type that implements the UserError
// interface.
type ConcreteUserError struct {
	ErrCause   error  `json:"-"`
	ErrStatus  int    `json:"-"`
	ErrCode    string `json:"code"`
	ErrMessage string `json:"message"`
}

// Build constructs a ConcreteUserError from a UserError.
func Build(err UserError) *ConcreteUserError {
	return &ConcreteUserError{
		ErrCause:   err.Cause(),
		ErrStatus:  err.Status(),
		ErrCode:    err.Code(),
		ErrMessage: err.Message(),
	}
}

// Error complies to the UserError and error interface.
func (e *ConcreteUserError) Error() string {
	if e.ErrCause != nil {
		return e.ErrCause.Error()
	}
	return ""
}

// Cause complies to the UserError interface.
func (e *ConcreteUserError) Cause() error {
	return e.ErrCause
}

// Status complies to the UserError interface.
func (e *ConcreteUserError) Status() int {
	return e.ErrStatus
}

// Code complies to the UserError interface.
func (e *ConcreteUserError) Code() string {
	return e.ErrCode
}

// Message complies to the UserError interface.
func (e *ConcreteUserError) Message() string {
	return e.ErrMessage
}

// NewUserError is an helper function to construct a new UserError.
func NewUserError(
	err error,
	status int,
	code string,
	message string,
) UserError {
	return &ConcreteUserError{
		ErrCause:   err,
		ErrStatus:  status,
		ErrCode:    code,
		ErrMessage: message,
	}
}

// NewUserErrorf is an helper function to construct a new UserError.
func NewUserErrorf(
	err error,
	status int,
	code string,
	format string,
	args ...interface{},
) UserError {
	message := fmt.Sprintf(format, args...)
	return NewUserError(err, status, code, message)
}
