package install

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
)

type installResponse struct {
	ID          domain.Uint64ID `json:"id"`
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Author      string          `json:"author"`
	Status      string          `json:"status"`
	InstalledAt time.Time       `json:"installed_at"`
	// Updated tells the caller the upload replaced an installed plugin
	// instead of adding one; PreviousVersion is what it replaced.
	Updated         bool   `json:"updated"`
	PreviousVersion string `json:"previous_version,omitempty"`
}

func newInstallResponse(plugin *domain.Plugin) *installResponse {
	var installedAt time.Time
	if plugin.InstalledAt != nil {
		installedAt = *plugin.InstalledAt
	}

	return &installResponse{
		ID:          plugin.ID,
		Name:        plugin.Name,
		Version:     plugin.Version,
		Description: plugin.Description,
		Author:      plugin.Author,
		Status:      string(plugin.Status),
		InstalledAt: installedAt,
	}
}

func newUpdateResponse(plugin *domain.Plugin, previousVersion string) *installResponse {
	resp := newInstallResponse(plugin)
	resp.Updated = true
	resp.PreviousVersion = previousVersion

	return resp
}
