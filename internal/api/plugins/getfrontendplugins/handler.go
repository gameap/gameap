package getfrontendplugins

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gameap/gameap/pkg/api"
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

// ServeHTTP answers with the concatenation of every loaded plugin's bundle.
// The body is validated with a strong ETag so a browser that already holds
// the current bundles gets 304 instead of re-downloading megabytes on every
// panel visit; "no-cache" keeps revalidation mandatory, "private" keeps the
// authenticated response out of shared caches.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body := []byte(h.generatePluginsJS())
	etag := api.StrongETag(body)

	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("ETag", etag)

	if api.IfNoneMatchSatisfied(r, etag) {
		w.WriteHeader(http.StatusNotModified)

		return
	}

	w.Header().Set("Content-Length", strconv.Itoa(len(body)))

	_, _ = w.Write(body)
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
