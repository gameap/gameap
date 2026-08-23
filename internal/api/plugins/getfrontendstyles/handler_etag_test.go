package getfrontendstyles

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"testing"

	"github.com/gameap/gameap/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func conditionalGet(t *testing.T, handler http.Handler, ifNoneMatch string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/plugins.css", nil)
	if ifNoneMatch != "" {
		request.Header.Set("If-None-Match", ifNoneMatch)
	}

	handler.ServeHTTP(recorder, request)

	return recorder
}

func TestHandler_ServeHTTP_ConditionalGet(t *testing.T) {
	t.Parallel()

	provider := &stubPluginProvider{plugins: []*plugin.LoadedPlugin{
		{FrontendStyles: []byte(".a{color:red}")},
	}}
	handler := NewHandler(provider)

	first := conditionalGet(t, handler, "")
	require.Equal(t, http.StatusOK, first.Code)
	etag := first.Header().Get("ETag")
	require.Regexp(t, regexp.MustCompile(`^"[0-9a-f]{64}"$`), etag)
	assert.Equal(t, strconv.Itoa(first.Body.Len()), first.Header().Get("Content-Length"))
	assert.Equal(t, "private, no-cache", first.Header().Get("Cache-Control"))

	t.Run("etag_is_stable", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, etag, conditionalGet(t, handler, "").Header().Get("ETag"))
	})

	t.Run("etag_changes_with_the_bundle", func(t *testing.T) {
		t.Parallel()
		other := NewHandler(&stubPluginProvider{plugins: []*plugin.LoadedPlugin{
			{FrontendStyles: []byte(".a{color:red} changed")},
		}})

		assert.NotEqual(t, etag, conditionalGet(t, other, "").Header().Get("ETag"))
	})

	t.Run("matching_if_none_match_returns_304", func(t *testing.T) {
		t.Parallel()
		recorder := conditionalGet(t, handler, etag)

		assert.Equal(t, http.StatusNotModified, recorder.Code)
		assert.Empty(t, recorder.Body.String())
		assert.Equal(t, etag, recorder.Header().Get("ETag"))
		assert.Equal(t, "text/css; charset=utf-8", recorder.Header().Get("Content-Type"))
		assert.Empty(t, recorder.Header().Get("Content-Length"))
	})

	t.Run("weak_and_listed_validators_match", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, http.StatusNotModified, conditionalGet(t, handler, "W/"+etag).Code)
		assert.Equal(t, http.StatusNotModified, conditionalGet(t, handler, `"stale", `+etag).Code)
		assert.Equal(t, http.StatusNotModified, conditionalGet(t, handler, "*").Code)
	})

	t.Run("mismatch_returns_full_body", func(t *testing.T) {
		t.Parallel()
		recorder := conditionalGet(t, handler, `"stale"`)

		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), ".a{color:red}")
	})
}
