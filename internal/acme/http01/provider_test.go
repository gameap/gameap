package http01_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/acme/http01"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvider_PresentServesKeyAuthorization(t *testing.T) {
	t.Parallel()

	p := http01.New()

	require.NoError(t, p.Present("example.com", "tok123", "key.auth.value"))

	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, http01.ChallengePathPrefix+"tok123", nil)

	p.Handler().ServeHTTP(rw, req)

	require.Equal(t, http.StatusOK, rw.Code)
	body, err := io.ReadAll(rw.Body)
	require.NoError(t, err)
	assert.Equal(t, "key.auth.value", string(body))
	assert.Equal(t, "text/plain; charset=utf-8", rw.Header().Get("Content-Type"))
	assert.Equal(t, "no-store", rw.Header().Get("Cache-Control"))
}

func TestProvider_CleanUpRemovesToken(t *testing.T) {
	t.Parallel()

	p := http01.New()

	require.NoError(t, p.Present("example.com", "tok123", "key.auth.value"))
	require.NoError(t, p.CleanUp("example.com", "tok123", "key.auth.value"))

	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, http01.ChallengePathPrefix+"tok123", nil)

	p.Handler().ServeHTTP(rw, req)

	assert.Equal(t, http.StatusNotFound, rw.Code)
}

func TestProvider_HandlerRejectsUnknownToken(t *testing.T) {
	t.Parallel()

	p := http01.New()

	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, http01.ChallengePathPrefix+"unknown", nil)

	p.Handler().ServeHTTP(rw, req)

	assert.Equal(t, http.StatusNotFound, rw.Code)
}

func TestProvider_HandlerRejectsBadMethod(t *testing.T) {
	t.Parallel()

	p := http01.New()

	require.NoError(t, p.Present("example.com", "tok", "key"))

	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, http01.ChallengePathPrefix+"tok", nil)

	p.Handler().ServeHTTP(rw, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rw.Code)
}

func TestProvider_HandlerRejectsPathTraversal(t *testing.T) {
	t.Parallel()

	p := http01.New()

	require.NoError(t, p.Present("example.com", "tok", "key"))

	tests := []string{
		http01.ChallengePathPrefix + "tok/extra",
		http01.ChallengePathPrefix,
	}

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			rw := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)

			p.Handler().ServeHTTP(rw, req)

			assert.Equal(t, http.StatusNotFound, rw.Code)
		})
	}
}

func TestProvider_HandlerHEADReturnsKeyAuth(t *testing.T) {
	t.Parallel()

	p := http01.New()

	require.NoError(t, p.Present("example.com", "tok", "key"))

	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, http01.ChallengePathPrefix+"tok", nil)

	p.Handler().ServeHTTP(rw, req)

	assert.Equal(t, http.StatusOK, rw.Code)
}

func TestProvider_ConcurrentPresentCleanUp(t *testing.T) {
	t.Parallel()

	p := http01.New()

	const n = 50

	done := make(chan struct{}, n*2)

	for i := range n {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			tok := "t" + string(rune('a'+i%26))
			_ = p.Present("example.com", tok, "key")
		}(i)

		go func(i int) {
			defer func() { done <- struct{}{} }()
			tok := "t" + string(rune('a'+i%26))
			_ = p.CleanUp("example.com", tok, "key")
		}(i)
	}

	for range n * 2 {
		<-done
	}
}
