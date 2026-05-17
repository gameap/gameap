package publicconfig

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockResponder struct {
	writeCalled      bool
	writeErrorCalled bool
	lastResult       any
	lastError        error
}

func (m *mockResponder) WriteError(_ context.Context, _ http.ResponseWriter, err error) {
	m.writeErrorCalled = true
	m.lastError = err
}

func (m *mockResponder) Write(_ context.Context, _ http.ResponseWriter, result any) {
	m.writeCalled = true
	m.lastResult = result
}

func TestNewHandler(t *testing.T) {
	cfg := &config.Config{}
	responder := &mockResponder{}

	handler := NewHandler(cfg, responder)

	require.NotNil(t, handler)
	assert.Equal(t, cfg, handler.config)
	assert.Equal(t, responder, handler.responder)
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name            string
		defaultLanguage string
		expectedResult  Response
	}{
		{
			name:            "returns_empty_when_not_set",
			defaultLanguage: "",
			expectedResult:  Response{DefaultLanguage: ""},
		},
		{
			name:            "returns_en_when_set",
			defaultLanguage: "en",
			expectedResult:  Response{DefaultLanguage: "en"},
		},
		{
			name:            "returns_ru_when_set",
			defaultLanguage: "ru",
			expectedResult:  Response{DefaultLanguage: "ru"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.UI.DefaultLanguage = tt.defaultLanguage

			responder := &mockResponder{}
			handler := NewHandler(cfg, responder)

			req := httptest.NewRequest(http.MethodGet, "/api/config/public", nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			assert.False(t, responder.writeErrorCalled)
			assert.True(t, responder.writeCalled)
			assert.Equal(t, tt.expectedResult, responder.lastResult)
		})
	}
}

// TestHandler_ServeHTTP_Captcha pins that the public config advertises the
// provider and site key when captcha is configured, never the secret, and
// stays absent when no provider is set.
func TestHandler_ServeHTTP_Captcha(t *testing.T) {
	t.Run("captcha_absent_when_provider_unset", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Captcha.SecretKey = "should-not-leak"

		responder := &mockResponder{}
		NewHandler(cfg, responder).ServeHTTP(
			httptest.NewRecorder(),
			httptest.NewRequest(http.MethodGet, "/api/config/public", nil),
		)

		resp, ok := responder.lastResult.(Response)
		require.True(t, ok)
		assert.Nil(t, resp.Captcha)
	})

	t.Run("captcha_exposes_provider_and_site_key_only", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Captcha.Provider = "turnstile"
		cfg.Captcha.SiteKey = "site-key-123"
		cfg.Captcha.SecretKey = "secret-key-should-not-leak"

		responder := &mockResponder{}
		NewHandler(cfg, responder).ServeHTTP(
			httptest.NewRecorder(),
			httptest.NewRequest(http.MethodGet, "/api/config/public", nil),
		)

		resp, ok := responder.lastResult.(Response)
		require.True(t, ok)
		require.NotNil(t, resp.Captcha)
		assert.Equal(t, "turnstile", resp.Captcha.Provider)
		assert.Equal(t, "site-key-123", resp.Captcha.SiteKey)

		body, err := json.Marshal(resp)
		require.NoError(t, err)
		assert.NotContains(t, string(body), "secret-key-should-not-leak")
	})
}
