package getfrontendstyles

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gameap/gameap/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubPluginProvider struct {
	plugins []*plugin.LoadedPlugin
}

func (p *stubPluginProvider) GetPlugins() []*plugin.LoadedPlugin {
	return p.plugins
}

func TestHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name            string
		provider        PluginProvider
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:         "nil_provider_serves_header_only",
			provider:     nil,
			wantContains: []string{"GameAP Frontend Plugin Styles"},
		},
		{
			name:         "no_plugins_serves_header_only",
			provider:     &stubPluginProvider{},
			wantContains: []string{"GameAP Frontend Plugin Styles"},
		},
		{
			name: "plugin_styles_are_appended",
			provider: &stubPluginProvider{
				plugins: []*plugin.LoadedPlugin{
					{FrontendStyles: []byte(".widget { color: red; }")},
				},
			},
			wantContains: []string{"GameAP Frontend Plugin Styles", ".widget { color: red; }"},
		},
		{
			name: "plugins_without_styles_are_skipped",
			provider: &stubPluginProvider{
				plugins: []*plugin.LoadedPlugin{
					{FrontendStyles: nil},
					{FrontendStyles: []byte{}},
					{FrontendStyles: []byte(".kept { color: blue; }")},
				},
			},
			wantContains: []string{".kept { color: blue; }"},
		},
		{
			name: "styles_of_several_plugins_are_concatenated",
			provider: &stubPluginProvider{
				plugins: []*plugin.LoadedPlugin{
					{FrontendStyles: []byte(".first { color: red; }")},
					{FrontendStyles: []byte(".second { color: green; }")},
				},
			},
			wantContains: []string{".first { color: red; }", ".second { color: green; }"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			handler := NewHandler(tt.provider)
			req := httptest.NewRequest(http.MethodGet, "/plugins.css", nil)
			rec := httptest.NewRecorder()

			// ACT
			handler.ServeHTTP(rec, req)

			// ASSERT
			resp := rec.Result()
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, "text/css; charset=utf-8", resp.Header.Get("Content-Type"))
			assert.Equal(t, "no-cache", resp.Header.Get("Cache-Control"),
				"generated stylesheet must not be cached, plugins can be reloaded at runtime")

			body := rec.Body.String()
			for _, want := range tt.wantContains {
				assert.Contains(t, body, want)
			}

			for _, notWant := range tt.wantNotContains {
				assert.NotContains(t, body, notWant)
			}
		})
	}
}

// The order plugins are loaded in is the order their styles cascade, so later
// plugins can override earlier ones.
func TestHandler_ServeHTTP_PreservesPluginOrder(t *testing.T) {
	// ARRANGE
	handler := NewHandler(&stubPluginProvider{
		plugins: []*plugin.LoadedPlugin{
			{FrontendStyles: []byte(".a {}")},
			{FrontendStyles: []byte(".b {}")},
			{FrontendStyles: []byte(".c {}")},
		},
	})
	rec := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/plugins.css", nil))

	// ASSERT
	body := rec.Body.String()
	posA := strings.Index(body, ".a {}")
	posB := strings.Index(body, ".b {}")
	posC := strings.Index(body, ".c {}")

	require.NotEqual(t, -1, posA)
	require.NotEqual(t, -1, posB)
	require.NotEqual(t, -1, posC)
	assert.Less(t, posA, posB, "styles must cascade in plugin load order")
	assert.Less(t, posB, posC, "styles must cascade in plugin load order")
}

func TestHandler_ServeHTTP_HeaderPrecedesPluginStyles(t *testing.T) {
	// ARRANGE
	handler := NewHandler(&stubPluginProvider{
		plugins: []*plugin.LoadedPlugin{{FrontendStyles: []byte(".widget {}")}},
	})
	rec := httptest.NewRecorder()

	// ACT
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/plugins.css", nil))

	// ASSERT
	body := rec.Body.String()
	assert.True(t, strings.HasPrefix(body, "/* GameAP Frontend Plugin Styles */"),
		"the generated banner must open the stylesheet")

	warningPos := strings.Index(body, "Do not edit manually")
	stylesPos := strings.Index(body, ".widget {}")
	require.NotEqual(t, -1, warningPos, "the banner must warn that the file is generated")
	require.NotEqual(t, -1, stylesPos, "plugin styles must be present")
	assert.Less(t, warningPos, stylesPos, "the warning must precede the plugin styles")
}
