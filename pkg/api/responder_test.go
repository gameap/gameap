package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
	responder := NewResponder()
	require.NotNil(t, responder)
}

func TestResponder_Write(t *testing.T) {
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
			responder := NewResponder()
			rec := httptest.NewRecorder()

			responder.Write(context.Background(), rec, tt.result)

			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			assert.JSONEq(t, tt.expected, rec.Body.String())
		})
	}
}

func TestResponder_WriteError(t *testing.T) {
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

func TestResponder_WriteError_WithDescription(t *testing.T) {
	responder := NewResponder()
	rec := httptest.NewRecorder()
	err := &descriptionError{
		msg:         "internal error message",
		description: "user-friendly description",
	}

	responder.WriteError(context.Background(), rec, err)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestWriteJSON(t *testing.T) {
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
			rec := httptest.NewRecorder()

			WriteJSON(rec, tt.result)

			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			assert.JSONEq(t, tt.expected, rec.Body.String())
		})
	}
}

func TestWriteErr(t *testing.T) {
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
