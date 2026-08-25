package reload

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
)

type reloadResponse struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Version      string     `json:"version"`
	Status       string     `json:"status"`
	Error        *string    `json:"error"`
	ErrorAt      *time.Time `json:"error_at"`
	Loaded       bool       `json:"loaded"`
	LastLoadedAt *time.Time `json:"last_loaded_at"`
}

func newReloadResponse(plugin *domain.Plugin, loaded bool) *reloadResponse {
	return &reloadResponse{
		ID:           pkgplugin.CompactPluginID(plugin.ID),
		Name:         plugin.Name,
		Version:      plugin.Version,
		Status:       string(plugin.Status),
		Error:        plugin.LastError,
		ErrorAt:      plugin.LastErrorAt,
		Loaded:       loaded,
		LastLoadedAt: plugin.LastLoadedAt,
	}
}
