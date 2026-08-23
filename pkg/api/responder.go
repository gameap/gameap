package api //nolint:revive,nolintlint

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

type withTitleError interface {
	error
	Title() string
}

type withDescriptionError interface {
	error
	Description() string
}

type withFieldsError interface {
	error
	Fields() map[string]string
}

type response struct {
	Status      string            `json:"status"`
	Title       string            `json:"title,omitempty"`
	Error       string            `json:"error,omitempty"`
	Message     string            `json:"message,omitempty"`
	Description string            `json:"description,omitempty"`
	HTTPCode    int               `json:"http_code,omitempty"`
	Errors      map[string]string `json:"errors,omitempty"`
	Result      any               `json:"result,omitempty"`
}

type Responder struct{}

func NewResponder() *Responder {
	return &Responder{}
}

func (r *Responder) WriteError(ctx context.Context, rw http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	title := ""
	description := err.Error()

	var errCustomStatus customStatusError
	var errWithTitle withTitleError
	var errWithDescription withDescriptionError
	var errJSONSyntax *json.SyntaxError

	if errors.As(err, &errWithTitle) {
		title = errWithTitle.Title()
	}

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

	logErrorResponse(ctx, code, err, description)

	var errWithFields withFieldsError
	if errors.As(err, &errWithFields) {
		writeErrResponse(rw, code, title, err, errWithFields.Fields())

		return
	}

	WriteErrWithTitle(rw, code, title, err)
}

// logErrorResponse makes error responses visible in logs: client errors would
// otherwise leave no server-side trace at all (e.g. rejected file-manager
// uploads). 401 is kept at debug to avoid flooding logs with expired tokens.
func logErrorResponse(ctx context.Context, code int, err error, description string) {
	var level slog.Level
	switch {
	case code >= http.StatusInternalServerError:
		level = slog.LevelError
	case code == http.StatusUnauthorized:
		level = slog.LevelDebug
	default:
		level = slog.LevelWarn
	}

	errMsg := err.Error()
	if errMsg != description {
		slog.LogAttrs(ctx, level, description, slog.Int("status", code), slog.String("error", errMsg))
	} else {
		slog.LogAttrs(ctx, level, errMsg, slog.Int("status", code))
	}
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
	WriteErrWithTitle(rw, code, "", err)
}

func WriteErrWithTitle(rw http.ResponseWriter, code int, title string, err error) {
	writeErrResponse(rw, code, title, err, nil)
}

func writeErrResponse(rw http.ResponseWriter, code int, title string, err error, fields map[string]string) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(code)

	errMsg := err.Error()

	if code >= http.StatusInternalServerError {
		errMsg = http.StatusText(code)
	}

	resp := response{
		Status:   "error",
		Title:    title,
		Error:    errMsg,
		Message:  errMsg, // for backward compatibility
		HTTPCode: code,   // for backward compatibility
		Errors:   fields,
	}

	if errEncode := json.NewEncoder(rw).Encode(resp); errEncode != nil {
		// Headers already sent, just write error to body
		_, _ = rw.Write([]byte(`{"status":"error","error":"internal server error"}`))
	}
}
