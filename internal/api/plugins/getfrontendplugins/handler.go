package getfrontendplugins

import (
	"net/http"
	"strings"

	"github.com/gameap/gameap/pkg/plugin"
)

type PluginProvider interface {
	GetPlugins() []*plugin.LoadedPlugin
}

type Handler struct {
	pluginProvider PluginProvider
}

func NewHandler(pluginProvider PluginProvider) *Handler {
	return &Handler{
		pluginProvider: pluginProvider,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")

	js := h.generatePluginsJS()

	_, _ = w.Write([]byte(js))
}

func (h *Handler) generatePluginsJS() string {
	var sb strings.Builder

	sb.WriteString(pluginsHeader)

	// Collect frontend bundles from loaded plugins
	pluginBundles := h.collectPluginBundles()
	for _, bundle := range pluginBundles {
		sb.WriteString("\n")
		sb.WriteString(bundle)
		sb.WriteString("\n")
	}

	return sb.String()
}

// collectPluginBundles iterates over loaded plugins and collects their frontend bundles.
func (h *Handler) collectPluginBundles() []string {
	var bundles []string

	if h.pluginProvider == nil {
		return bundles
	}

	plugins := h.pluginProvider.GetPlugins()
	for _, p := range plugins {
		if len(p.FrontendBundle) > 0 {
			bundles = append(bundles, string(p.FrontendBundle))
		}
	}

	return bundles
}

const pluginsHeader = `// GameAP Frontend Plugins Module
// Auto-generated - Do not edit manually
//
// Plugins are pre-compiled Vue components using @gameap/plugin-sdk
// Vue and related libraries are available globally:
//   - window.Vue (ref, computed, defineComponent, etc.)
//   - window.VueRouter
//   - window.Pinia
`
