package getfrontendstyles

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
	body := []byte(h.generatePluginsCSS())
	etag := api.StrongETag(body)

	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("ETag", etag)

	if api.IfNoneMatchSatisfied(r, etag) {
		w.WriteHeader(http.StatusNotModified)

		return
	}

	w.Header().Set("Content-Length", strconv.Itoa(len(body)))

	_, _ = w.Write(body)
}

func (h *Handler) generatePluginsCSS() string {
	var sb strings.Builder

	sb.WriteString(stylesHeader)

	pluginStyles := h.collectPluginStyles()
	for _, styles := range pluginStyles {
		sb.WriteString("\n")
		sb.WriteString(styles)
		sb.WriteString("\n")
	}

	return sb.String()
}

// collectPluginStyles iterates over loaded plugins and collects their frontend styles.
func (h *Handler) collectPluginStyles() []string {
	var styles []string

	if h.pluginProvider == nil {
		return styles
	}

	plugins := h.pluginProvider.GetPlugins()
	for _, p := range plugins {
		if len(p.FrontendStyles) > 0 {
			styles = append(styles, string(p.FrontendStyles))
		}
	}

	return styles
}

const stylesHeader = `/* GameAP Frontend Plugin Styles */
/* Auto-generated - Do not edit manually */
`
