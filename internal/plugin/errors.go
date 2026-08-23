package plugin

import (
	"strings"
	"unicode/utf8"

	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/pkg/errors"
)

var (
	ErrPluginNotInstalled = errors.New("plugin not installed")
	// ErrPluginDisabled: the operator switched the plugin off; it is never
	// loaded or reloaded until the status changes.
	ErrPluginDisabled = errors.New("plugin is disabled")
	ErrPluginUpdating = errors.New("plugin is being updated")
	// ErrPluginHeld: a handler is in the middle of a multi-step operation on
	// the plugin (update, uninstall, configuration); the reconciler must not
	// touch it until the hold is released.
	ErrPluginHeld = errors.New("plugin is held by an operation in progress")
)

// maxLoadErrorLen bounds the reason persisted in plugins.last_error and shown
// in the admin UI.
const maxLoadErrorLen = 1024

// LoadErrorText turns a load failure into the text stored as the plugin's
// last error: wasm runtime errors lose their Go stack trace, everything is
// capped at maxLoadErrorLen bytes on a rune boundary.
func LoadErrorText(err error) string {
	if err == nil {
		return ""
	}

	text := err.Error()
	if strings.Contains(text, "wasm stack trace:") {
		text = pkgplugin.SanitizeLoadError(err).Error()
	}

	text = strings.TrimSpace(text)
	if len(text) <= maxLoadErrorLen {
		return text
	}

	cut := text[:maxLoadErrorLen]
	for !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}

	return cut + "…"
}
