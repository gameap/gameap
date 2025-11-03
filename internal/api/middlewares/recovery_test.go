package middlewares

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecoveryMiddleware_Middleware(t *testing.T) {
	tests := []struct {
		name             string
		handler          http.HandlerFunc
		expectPanic      bool
		expectedStatus   int
		expectedResponse map[string]any
	}{
		{
			name: "recovers_from_string_panic",
			handler: func(_ http.ResponseWriter, _ *http.Request) {
				panic("something went wrong")
			},
			expectPanic:    true,
			expectedStatus: http.StatusInternalServerError,
			expectedResponse: map[string]any{
				"status":    "error",
				"error":     "Internal Server Error",
				"message":   "Internal Server Error",
				"http_code": float64(http.StatusInternalServerError),
			},
		},
		{
			name: "recovers_from_error_panic",
			handler: func(_ http.ResponseWriter, _ *http.Request) {
				panic(errors.New("unexpected error"))
			},
			expectPanic:    true,
			expectedStatus: http.StatusInternalServerError,
			expectedResponse: map[string]any{
				"status":    "error",
				"error":     "Internal Server Error",
				"message":   "Internal Server Error",
				"http_code": float64(http.StatusInternalServerError),
			},
		},
		{
			name: "recovers_from_custom_type_panic",
			handler: func(_ http.ResponseWriter, _ *http.Request) {
				panic(struct{ Code int }{Code: 123})
			},
			expectPanic:    true,
			expectedStatus: http.StatusInternalServerError,
			expectedResponse: map[string]any{
				"status":    "error",
				"error":     "Internal Server Error",
				"message":   "Internal Server Error",
				"http_code": float64(http.StatusInternalServerError),
			},
		},
		{
			name: "does_not_interfere_with_normal_handlers",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status":"success"}`))
			},
			expectPanic:    false,
			expectedStatus: http.StatusOK,
			expectedResponse: map[string]any{
				"status": "success",
			},
		},
		{
			name: "recovers_from_nil_panic",
			handler: func(_ http.ResponseWriter, _ *http.Request) {
				var ptr *string
				_ = *ptr // This will panic
			},
			expectPanic:    true,
			expectedStatus: http.StatusInternalServerError,
			expectedResponse: map[string]any{
				"status":    "error",
				"error":     "Internal Server Error",
				"message":   "Internal Server Error",
				"http_code": float64(http.StatusInternalServerError),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			responder := api.NewResponder()
			recoveryMiddleware := NewRecoveryMiddleware(responder)

			// Create test request
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()

			// Wrap handler with recovery middleware
			handler := recoveryMiddleware.Middleware(tt.handler)

			// Execute
			handler.ServeHTTP(rec, req)

			// Assert status code
			assert.Equal(t, tt.expectedStatus, rec.Code, "unexpected status code")

			// Assert response body
			var response map[string]any
			err := json.Unmarshal(rec.Body.Bytes(), &response)
			require.NoError(t, err, "failed to unmarshal response")

			for key, expectedValue := range tt.expectedResponse {
				assert.Equal(t, expectedValue, response[key], "unexpected value for key %s", key)
			}

			// Verify Content-Type header for panic cases
			if tt.expectPanic {
				assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			}
		})
	}
}

func TestRecoveryMiddleware_MultipleRequests(t *testing.T) {
	// Test that the middleware can handle multiple requests,
	// including recovery from panics without affecting subsequent requests
	responder := api.NewResponder()
	recoveryMiddleware := NewRecoveryMiddleware(responder)

	panicHandler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("test panic")
	})

	normalHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	wrappedPanicHandler := recoveryMiddleware.Middleware(panicHandler)
	wrappedNormalHandler := recoveryMiddleware.Middleware(normalHandler)

	// First request - panic
	req1 := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec1 := httptest.NewRecorder()
	wrappedPanicHandler.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusInternalServerError, rec1.Code)

	// Second request - normal (should work fine after panic)
	req2 := httptest.NewRequest(http.MethodGet, "/normal", nil)
	rec2 := httptest.NewRecorder()
	wrappedNormalHandler.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)

	// Third request - panic again (should still be recoverable)
	req3 := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec3 := httptest.NewRecorder()
	wrappedPanicHandler.ServeHTTP(rec3, req3)
	assert.Equal(t, http.StatusInternalServerError, rec3.Code)
}
