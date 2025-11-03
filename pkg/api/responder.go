package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/pkg/errors"
)

type customStatusError interface {
	error
	HTTPStatus() int
}

type withDescriptionError interface {
	error
	Description() string
}

type response struct {
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
	Message     string `json:"message,omitempty"`
	Description string `json:"description,omitempty"`
	HTTPCode    int    `json:"http_code,omitempty"`
	Result      any    `json:"result,omitempty"`
}

type Responder struct{}

func NewResponder() *Responder {
	return &Responder{}
}

func (r *Responder) WriteError(ctx context.Context, rw http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	description := err.Error()

	var errCustomStatus customStatusError
	var errWithDescription withDescriptionError
	var errJSONSyntax *json.SyntaxError

	if errors.As(err, &errWithDescription) {
		description = errWithDescription.Description()
	}

	switch {
	case errors.As(err, &errCustomStatus):
		code = errCustomStatus.HTTPStatus()
	// case errors.As(err, &validationErrors):
	//	code = http.StatusUnprocessableEntity
	case errors.Is(err, http.ErrMissingBoundary),
		errors.Is(err, http.ErrNotMultipart),
		errors.Is(err, http.ErrMissingFile),
		errors.As(err, &errJSONSyntax),
		errors.Is(err, io.EOF):
		code = http.StatusBadRequest
	}

	if code >= http.StatusInternalServerError {
		errMsg := err.Error()

		if errMsg != description {
			slog.ErrorContext(
				ctx,
				description,
				slog.String("error", err.Error()),
			)
		} else {
			slog.ErrorContext(ctx, errMsg)
		}
	}

	WriteErr(rw, code, err)
}

func (r *Responder) Write(_ context.Context, rw http.ResponseWriter, result any) {
	WriteJSON(rw, result)
}

func WriteJSON(rw http.ResponseWriter, result any) {
	rw.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(rw).Encode(result); err != nil {
		// If encoding fails after headers are sent, just write error to body
		_, _ = rw.Write([]byte(`{"status":"error","error":"encoding failed"}`))
	}
}

func WriteErr(rw http.ResponseWriter, code int, err error) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(code)

	errMsg := err.Error()

	if code >= http.StatusInternalServerError {
		errMsg = http.StatusText(code)
	}

	resp := response{
		Status:   "error",
		Error:    errMsg,
		Message:  errMsg, // for backward compatibility
		HTTPCode: code,   // for backward compatibility
	}

	if errEncode := json.NewEncoder(rw).Encode(resp); errEncode != nil {
		// Headers already sent, just write error to body
		_, _ = rw.Write([]byte(`{"status":"error","error":"internal server error"}`))
	}
}
