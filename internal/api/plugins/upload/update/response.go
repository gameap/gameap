package update

import (
	"time"

	"github.com/gameap/gameap/internal/domain"
)

type updateResponse struct {
	ID        domain.Uint64ID `json:"id"`
	Name      string          `json:"name"`
	Version   string          `json:"version"`
	Status    string          `json:"status"`
	UpdatedAt time.Time       `json:"updated_at"`
}

func newUpdateResponse(plugin *domain.Plugin) *updateResponse {
	var updatedAt time.Time
	if plugin.UpdatedAt != nil {
		updatedAt = *plugin.UpdatedAt
	}

	return &updateResponse{
		ID:        plugin.ID,
		Name:      plugin.Name,
		Version:   plugin.Version,
		Status:    string(plugin.Status),
		UpdatedAt: updatedAt,
	}
}
