package getfrontendstyles

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
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")

	css := h.generatePluginsCSS()

	_, _ = w.Write([]byte(css))
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
