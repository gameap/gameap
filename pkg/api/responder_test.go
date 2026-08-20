package api //nolint:revive,nolintlint

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type descriptionError struct {
	msg         string
	description string
}

func (e *descriptionError) Error() string {
	return e.msg
}

func (e *descriptionError) Description() string {
	return e.description
}

func TestNewResponder(t *testing.T) {
	t.Parallel()

	responder := NewResponder()
	require.NotNil(t, responder)
}

func TestResponder_Write(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		result   any
		expected string
	}{
		{
			name:     "simple_struct",
			result:   map[string]string{"key": "value"},
			expected: `{"key":"value"}`,
		},
		{
			name:     "slice",
			result:   []int{1, 2, 3},
			expected: `[1,2,3]`,
		},
		{
			name:     "nil_value",
			result:   nil,
			expected: `null`,
		},
		{
			name: "nested_struct",
			result: struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			}{ID: 1, Name: "test"},
			expected: `{"id":1,"name":"test"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			responder := NewResponder()
			rec := httptest.NewRecorder()

			responder.Write(context.Background(), rec, tt.result)

			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			assert.JSONEq(t, tt.expected, rec.Body.String())
		})
	}
}

func TestResponder_WriteError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		err              error
		expectedStatus   int
		expectedContains string
	}{
		{
			name:             "generic_error",
			err:              errors.New("something went wrong"),
			expectedStatus:   http.StatusInternalServerError,
			expectedContains: "Internal Server Error",
		},
		{
			name:             "custom_status_error_not_found",
			err:              NewNotFoundError("resource not found"),
			expectedStatus:   http.StatusNotFound,
			expectedContains: "resource not found",
		},
		{
			name:             "custom_status_error_bad_request",
			err:              NewError(http.StatusBadRequest, "bad request"),
			expectedStatus:   http.StatusBadRequest,
			expectedContains: "bad request",
		},
		{
			name:             "custom_status_error_validation",
			err:              NewValidationError("invalid input"),
			expectedStatus:   http.StatusUnprocessableEntity,
			expectedContains: "invalid input",
		},
		{
			name:             "wrapped_error",
			err:              WrapHTTPError(errors.New("wrapped error"), http.StatusForbidden),
			expectedStatus:   http.StatusForbidden,
			expectedContains: "wrapped error",
		},
		{
			name:             "eof_error",
			err:              io.EOF,
			expectedStatus:   http.StatusBadRequest,
			expectedContains: "EOF",
		},
		{
			name:             "json_syntax_error",
			err:              &json.SyntaxError{},
			expectedStatus:   http.StatusBadRequest,
			expectedContains: "",
		},
		{
			name:             "missing_boundary_error",
			err:              http.ErrMissingBoundary,
			expectedStatus:   http.StatusBadRequest,
			expectedContains: "no multipart boundary",
		},
		{
			name:             "not_multipart_error",
			err:              http.ErrNotMultipart,
			expectedStatus:   http.StatusBadRequest,
			expectedContains: "multipart/form-data",
		},
		{
			name:             "missing_file_error",
			err:              http.ErrMissingFile,
			expectedStatus:   http.StatusBadRequest,
			expectedContains: "no such file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			responder := NewResponder()
			rec := httptest.NewRecorder()

			responder.WriteError(context.Background(), rec, tt.err)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

			var resp response
			err := json.NewDecoder(rec.Body).Decode(&resp)
			require.NoError(t, err)
			assert.Equal(t, "error", resp.Status)
			assert.Equal(t, tt.expectedStatus, resp.HTTPCode)
			if tt.expectedContains != "" {
				assert.Contains(t, resp.Error, tt.expectedContains)
			}
		})
	}
}

type capturedLogRecord struct {
	level     slog.Level
	message   string
	status    int64
	errorAttr string
}

type capturingLogHandler struct {
	mu      sync.Mutex
	records []capturedLogRecord
}

func (h *capturingLogHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *capturingLogHandler) Handle(_ context.Context, r slog.Record) error {
	captured := capturedLogRecord{level: r.Level, message: r.Message, status: -1}
	r.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "status":
			captured.status = a.Value.Int64()
		case "error":
			captured.errorAttr = a.Value.String()
		}

		return true
	})

	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, captured)

	return nil
}

func (h *capturingLogHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *capturingLogHandler) WithGroup(string) slog.Handler { return h }

func TestResponder_WriteError_LogLevels(t *testing.T) { //nolint:paralleltest // mutates the global slog default logger
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantLevel  slog.Level
		wantMsg    string
	}{
		{
			name:       "internal_error_logged_as_error",
			err:        errors.New("database exploded"),
			wantStatus: http.StatusInternalServerError,
			wantLevel:  slog.LevelError,
			wantMsg:    "database exploded",
		},
		{
			name:       "bad_request_logged_as_warn",
			err:        NewError(http.StatusBadRequest, "invalid chunk index"),
			wantStatus: http.StatusBadRequest,
			wantLevel:  slog.LevelWarn,
			wantMsg:    "invalid chunk index",
		},
		{
			name:       "request_entity_too_large_logged_as_warn",
			err:        NewError(http.StatusRequestEntityTooLarge, "chunk size mismatch"),
			wantStatus: http.StatusRequestEntityTooLarge,
			wantLevel:  slog.LevelWarn,
			wantMsg:    "chunk size mismatch",
		},
		{
			name:       "not_found_logged_as_warn",
			err:        NewNotFoundError("upload session not found"),
			wantStatus: http.StatusNotFound,
			wantLevel:  slog.LevelWarn,
			wantMsg:    "upload session not found",
		},
		{
			name:       "unauthorized_logged_as_debug",
			err:        NewError(http.StatusUnauthorized, "token expired"),
			wantStatus: http.StatusUnauthorized,
			wantLevel:  slog.LevelDebug,
			wantMsg:    "token expired",
		},
	}

	for _, tt := range tests { //nolint:paralleltest // subtests mutate the global slog default logger
		t.Run(tt.name, func(t *testing.T) {
			handler := &capturingLogHandler{}
			prev := slog.Default()
			slog.SetDefault(slog.New(handler))
			t.Cleanup(func() { slog.SetDefault(prev) })

			responder := NewResponder()
			rec := httptest.NewRecorder()

			responder.WriteError(context.Background(), rec, tt.err)

			assert.Equal(t, tt.wantStatus, rec.Code)
			require.Len(t, handler.records, 1)
			logged := handler.records[0]
			assert.Equal(t, tt.wantLevel, logged.level)
			assert.Equal(t, tt.wantMsg, logged.message)
			assert.Equal(t, int64(tt.wantStatus), logged.status)
		})
	}
}

func TestResponder_WriteError_WithDescription(t *testing.T) {
	t.Parallel()

	responder := NewResponder()
	rec := httptest.NewRecorder()
	err := &descriptionError{
		msg:         "internal error message",
		description: "user-friendly description",
	}

	responder.WriteError(context.Background(), rec, err)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// statusDescriptionError mirrors daemon.FileError: a client-safe Error(), an
// HTTP status and a verbose Description meant for the log line only.
type statusDescriptionError struct {
	descriptionError

	status int
}

func (e *statusDescriptionError) HTTPStatus() int { return e.status }

func TestResponder_WriteError_DescriptionStaysOutOfBody(t *testing.T) { //nolint:paralleltest // mutates the global slog default logger
	handler := &capturingLogHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(prev) })

	responder := NewResponder()
	rec := httptest.NewRecorder()
	err := errors.WithMessage(&statusDescriptionError{
		descriptionError: descriptionError{
			msg:         "file not found",
			description: "stat: lstatat servers/q2/server.cfg: no such file or directory",
		},
		status: http.StatusNotFound,
	}, "failed to get file info")

	responder.WriteError(context.Background(), rec, err)

	assert.Equal(t, http.StatusNotFound, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "failed to get file info: file not found", body["error"])
	assert.NotContains(t, rec.Body.String(), "servers/q2", "description must never reach the client")

	require.Len(t, handler.records, 1)
	logged := handler.records[0]
	assert.Equal(t, slog.LevelWarn, logged.level)
	assert.Equal(t, "stat: lstatat servers/q2/server.cfg: no such file or directory", logged.message)
	assert.Equal(t, "failed to get file info: file not found", logged.errorAttr)
	assert.Equal(t, int64(http.StatusNotFound), logged.status)
}

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		result   any
		expected string
	}{
		{
			name:     "map",
			result:   map[string]int{"count": 42},
			expected: `{"count":42}`,
		},
		{
			name:     "string",
			result:   "hello",
			expected: `"hello"`,
		},
		{
			name:     "bool",
			result:   true,
			expected: `true`,
		},
		{
			name:     "empty_slice",
			result:   []string{},
			expected: `[]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()

			WriteJSON(rec, tt.result)

			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			assert.JSONEq(t, tt.expected, rec.Body.String())
		})
	}
}

func TestWriteErr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		code          int
		err           error
		expectedError string
		hideRealError bool
	}{
		{
			name:          "bad_request",
			code:          http.StatusBadRequest,
			err:           errors.New("invalid input"),
			expectedError: "invalid input",
		},
		{
			name:          "not_found",
			code:          http.StatusNotFound,
			err:           errors.New("resource not found"),
			expectedError: "resource not found",
		},
		{
			name:          "internal_server_error_hides_message",
			code:          http.StatusInternalServerError,
			err:           errors.New("database connection lost"),
			expectedError: "Internal Server Error",
			hideRealError: true,
		},
		{
			name:          "bad_gateway_hides_message",
			code:          http.StatusBadGateway,
			err:           errors.New("upstream error"),
			expectedError: "Bad Gateway",
			hideRealError: true,
		},
		{
			name:          "service_unavailable_hides_message",
			code:          http.StatusServiceUnavailable,
			err:           errors.New("service down"),
			expectedError: "Service Unavailable",
			hideRealError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()

			WriteErr(rec, tt.code, tt.err)

			assert.Equal(t, tt.code, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

			var resp response
			err := json.NewDecoder(rec.Body).Decode(&resp)
			require.NoError(t, err)
			assert.Equal(t, "error", resp.Status)
			assert.Equal(t, tt.expectedError, resp.Error)
			assert.Equal(t, tt.expectedError, resp.Message)
			assert.Equal(t, tt.code, resp.HTTPCode)

			if tt.hideRealError {
				assert.NotContains(t, resp.Error, tt.err.Error())
			}
		})
	}
}

func TestWriteErr_ResponseStructure(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	err := errors.New("test error")

	WriteErr(rec, http.StatusBadRequest, err)

	var resp map[string]any
	decodeErr := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, decodeErr)

	assert.Contains(t, resp, "status")
	assert.Contains(t, resp, "error")
	assert.Contains(t, resp, "message")
	assert.Contains(t, resp, "http_code")

	assert.Equal(t, "error", resp["status"])
	assert.Equal(t, "test error", resp["error"])
	assert.Equal(t, "test error", resp["message"])
	assert.Equal(t, float64(http.StatusBadRequest), resp["http_code"])
}
