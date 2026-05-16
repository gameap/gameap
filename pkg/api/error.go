package api //nolint:revive,nolintlint

import "net/http"

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func NewError(code int, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

func (e *Error) Error() string {
	return e.Message
}

func (e *Error) HTTPStatus() int {
	return e.Code
}

func NewNotFoundError(message string) *Error {
	if message == "" {
		message = "Not found"
	}

	return NewError(http.StatusNotFound, message)
}

func NewValidationError(message string) *Error {
	if message == "" {
		message = "Validation error"
	}

	return NewError(http.StatusUnprocessableEntity, message)
}

type WrappedError struct {
	code  int
	title string
	cause error
}

func WrapHTTPError(err error, code int) *WrappedError {
	return &WrappedError{
		code:  code,
		cause: err,
	}
}

func WrapHTTPErrorWithTitle(err error, code int, title string) *WrappedError {
	return &WrappedError{
		code:  code,
		title: title,
		cause: err,
	}
}

func (e *WrappedError) HTTPStatus() int {
	return e.code
}

func (e *WrappedError) Title() string {
	return e.title
}

func (e *WrappedError) Error() string {
	return e.cause.Error()
}

// Unwrap exposes the wrapped cause so errors.Is / errors.As can match
// against the original sentinel error.
func (e *WrappedError) Unwrap() error {
	return e.cause
}
