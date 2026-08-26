package api //nolint:revive,nolintlint

import (
	"net/http"
	"sort"
	"strings"
)

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

// FieldValidationError reports validation failures for several named fields at
// once so the client can highlight each of them instead of showing a single
// sentence. The field name is whatever the client submitted the value under:
// for server settings that is the game mod variable name.
type FieldValidationError struct {
	fields map[string][]string
}

func NewFieldValidationError(fields map[string][]string) *FieldValidationError {
	return &FieldValidationError{fields: fields}
}

func (e *FieldValidationError) HTTPStatus() int {
	return http.StatusUnprocessableEntity
}

// FieldErrors is read by the responder to fill the "errors" object in the body.
func (e *FieldValidationError) FieldErrors() map[string][]string {
	return e.fields
}

func (e *FieldValidationError) Error() string {
	if len(e.fields) == 0 {
		return "Validation error"
	}

	names := make([]string, 0, len(e.fields))
	for name := range e.fields {
		names = append(names, name)
	}
	// Sorted so the message stays stable across map iterations.
	sort.Strings(names)

	parts := make([]string, 0, len(e.fields))
	for _, name := range names {
		for _, message := range e.fields[name] {
			parts = append(parts, name+": "+message)
		}
	}

	return strings.Join(parts, "; ")
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
