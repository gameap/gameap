package patchservers

import (
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/flexible"
)

var (
	ErrInvalidServerID        = api.NewValidationError("server ID is required")
	ErrInvalidInstalledStatus = api.NewValidationError("invalid installed status")
)

type bulkUpdateServerInput struct {
	ID               uint           `json:"id"`
	Installed        *int           `json:"installed,omitempty"`
	ProcessActive    *flexible.Bool `json:"process_active,omitempty"`
	LastProcessCheck *flexible.Time `json:"last_process_check,omitempty"`
}

func (in *bulkUpdateServerInput) Validate() error {
	if in.ID == 0 {
		return ErrInvalidServerID
	}

	if in.Installed != nil && (*in.Installed < 0 || *in.Installed >= 3) {
		return ErrInvalidInstalledStatus
	}

	return nil
}
