package telemetry

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_Handler_serves_text_exposition(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	New().Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/plain")
	assert.Contains(t, rec.Body.String(), "go_gc_duration_seconds")
}

func TestRegistries_are_isolated(t *testing.T) {
	t.Parallel()

	first := New()
	second := New()

	// Registering the same collectors twice would panic on a shared registry.
	NewPluginMetrics(first, nil, nil, nil)
	NewPluginMetrics(second, nil, nil, nil)
}
