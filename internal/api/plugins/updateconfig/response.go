package updateconfig

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/plugin/pluginconfig"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
)

// updateResponse is the saved configuration (secrets masked) together with
// the outcome of the reload it triggered.
type updateResponse struct {
	pluginconfig.View

	ID           string     `json:"id"`
	Reloaded     bool       `json:"reloaded"`
	Status       string     `json:"status"`
	Error        *string    `json:"error"`
	ErrorAt      *time.Time `json:"error_at"`
	LastLoadedAt *time.Time `json:"last_loaded_at"`
	ReloadError  string     `json:"reload_error,omitempty"`
}

func newUpdateResponse(record *domain.Plugin, reloaded bool, reloadError string) updateResponse {
	return updateResponse{
		ID:           pkgplugin.CompactPluginID(record.ID),
		View:         pluginconfig.NewView(record),
		Reloaded:     reloaded,
		Status:       string(record.Status),
		Error:        record.LastError,
		ErrorAt:      record.LastErrorAt,
		LastLoadedAt: record.LastLoadedAt,
		ReloadError:  reloadError,
	}
}
