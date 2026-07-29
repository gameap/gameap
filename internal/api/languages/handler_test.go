package languages_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gameap/gameap/internal/api/languages"
	"github.com/gameap/gameap/internal/i18n"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ServeHTTP(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"en.json": &fstest.MapFile{Data: []byte(`{"_language":{"name":"English","native_name":"English"}}`)},
		"ru.json": &fstest.MapFile{Data: []byte(`{"_language":{"name":"Russian","native_name":"Русский"}}`)},
	}

	rr := httptest.NewRecorder()
	languages.NewHandler(fsys).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/lang", nil))

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json; charset=utf-8", rr.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", rr.Header().Get("Cache-Control"))

	var got []i18n.Language
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	assert.Equal(t, []i18n.Language{
		{Code: "en", Name: "English", NativeName: "English"},
		{Code: "ru", Name: "Russian", NativeName: "Русский"},
	}, got)
}
