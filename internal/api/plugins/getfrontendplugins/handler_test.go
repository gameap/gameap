package getfrontendplugins_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/api/plugins/getfrontendplugins"
	"github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/stretchr/testify/assert"
)

type mockPluginProvider struct {
	plugins []*plugin.LoadedPlugin
}

func (m *mockPluginProvider) GetPlugins() []*plugin.LoadedPlugin {
	return m.plugins
}

func TestHandler_ServeHTTP_NilProvider(t *testing.T) {
	// ARRANGE
	handler := getfrontendplugins.NewHandler(nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/plugins.js", nil)

	// ACT
	handler.ServeHTTP(recorder, request)

	// ASSERT
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "application/javascript; charset=utf-8", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", recorder.Header().Get("Cache-Control"))
	assert.Contains(t, recorder.Body.String(), "// GameAP Frontend Plugins Module")
	assert.Contains(t, recorder.Body.String(), "window.Vue")
}

func TestHandler_ServeHTTP_Headers(t *testing.T) {
	tests := []struct {
		name            string
		expectedHeaders map[string]string
	}{
		{
			name: "correct_content_type_and_cache_headers",
			expectedHeaders: map[string]string{
				"Content-Type":  "application/javascript; charset=utf-8",
				"Cache-Control": "no-cache",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			handler := getfrontendplugins.NewHandler(nil)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/plugins.js", nil)

			// ACT
			handler.ServeHTTP(recorder, request)

			// ASSERT
			for header, expectedValue := range tt.expectedHeaders {
				assert.Equal(t, expectedValue, recorder.Header().Get(header))
			}
		})
	}
}

func TestHandler_ServeHTTP_PluginsHeader(t *testing.T) {
	// ARRANGE
	handler := getfrontendplugins.NewHandler(nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/plugins.js", nil)

	// ACT
	handler.ServeHTTP(recorder, request)

	// ASSERT
	body := recorder.Body.String()
	assert.Contains(t, body, "// GameAP Frontend Plugins Module")
	assert.Contains(t, body, "// Auto-generated - Do not edit manually")
	assert.Contains(t, body, "window.Vue")
	assert.Contains(t, body, "window.VueRouter")
	assert.Contains(t, body, "window.Pinia")
}

func TestHandler_ServeHTTP_WithPlugins(t *testing.T) {
	tests := []struct {
		name           string
		plugins        []*plugin.LoadedPlugin
		expectedInBody []string
		notInBody      []string
	}{
		{
			name:           "no_plugins",
			plugins:        []*plugin.LoadedPlugin{},
			expectedInBody: []string{"// GameAP Frontend Plugins Module"},
			notInBody:      []string{"export const testPlugin"},
		},
		{
			name: "single_plugin_with_bundle",
			plugins: []*plugin.LoadedPlugin{
				{
					Info: &proto.PluginInfo{
						Id:      "test-plugin",
						Name:    "Test Plugin",
						Version: "1.0.0",
					},
					FrontendBundle: []byte(`export const testPlugin = { id: 'test-plugin', name: 'Test Plugin' };`),
				},
			},
			expectedInBody: []string{
				"// GameAP Frontend Plugins Module",
				"export const testPlugin",
				"id: 'test-plugin'",
			},
		},
		{
			name: "plugin_without_bundle",
			plugins: []*plugin.LoadedPlugin{
				{
					Info: &proto.PluginInfo{
						Id:      "no-frontend-plugin",
						Name:    "No Frontend Plugin",
						Version: "1.0.0",
					},
					FrontendBundle: nil,
				},
			},
			expectedInBody: []string{"// GameAP Frontend Plugins Module"},
			notInBody:      []string{"no-frontend-plugin"},
		},
		{
			name: "multiple_plugins_with_bundles",
			plugins: []*plugin.LoadedPlugin{
				{
					Info: &proto.PluginInfo{
						Id:      "plugin-one",
						Name:    "Plugin One",
						Version: "1.0.0",
					},
					FrontendBundle: []byte(`export const pluginOne = { id: 'plugin-one' };`),
				},
				{
					Info: &proto.PluginInfo{
						Id:      "plugin-two",
						Name:    "Plugin Two",
						Version: "2.0.0",
					},
					FrontendBundle: []byte(`export const pluginTwo = { id: 'plugin-two' };`),
				},
			},
			expectedInBody: []string{
				"// GameAP Frontend Plugins Module",
				"export const pluginOne",
				"export const pluginTwo",
				"id: 'plugin-one'",
				"id: 'plugin-two'",
			},
		},
		{
			name: "mixed_plugins_with_and_without_bundles",
			plugins: []*plugin.LoadedPlugin{
				{
					Info: &proto.PluginInfo{
						Id:      "with-bundle",
						Name:    "With Bundle",
						Version: "1.0.0",
					},
					FrontendBundle: []byte(`export const withBundle = { id: 'with-bundle' };`),
				},
				{
					Info: &proto.PluginInfo{
						Id:      "without-bundle",
						Name:    "Without Bundle",
						Version: "1.0.0",
					},
					FrontendBundle: nil,
				},
				{
					Info: &proto.PluginInfo{
						Id:      "empty-bundle",
						Name:    "Empty Bundle",
						Version: "1.0.0",
					},
					FrontendBundle: []byte{},
				},
			},
			expectedInBody: []string{
				"// GameAP Frontend Plugins Module",
				"export const withBundle",
			},
			notInBody: []string{
				"without-bundle",
				"empty-bundle",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			provider := &mockPluginProvider{plugins: tt.plugins}
			handler := getfrontendplugins.NewHandler(provider)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/plugins.js", nil)

			// ACT
			handler.ServeHTTP(recorder, request)

			// ASSERT
			assert.Equal(t, http.StatusOK, recorder.Code)
			body := recorder.Body.String()

			for _, expected := range tt.expectedInBody {
				assert.Contains(t, body, expected)
			}

			for _, notExpected := range tt.notInBody {
				assert.NotContains(t, body, notExpected)
			}
		})
	}
}

func TestNewHandler(t *testing.T) {
	t.Run("with_nil_provider", func(t *testing.T) {
		// ACT
		handler := getfrontendplugins.NewHandler(nil)

		// ASSERT
		assert.NotNil(t, handler)
	})

	t.Run("with_provider", func(t *testing.T) {
		// ARRANGE
		provider := &mockPluginProvider{}

		// ACT
		handler := getfrontendplugins.NewHandler(provider)

		// ASSERT
		assert.NotNil(t, handler)
	})
}
