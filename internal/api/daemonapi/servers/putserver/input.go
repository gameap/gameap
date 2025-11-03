package putserver

import (
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/flexible"
)

var (
	ErrInvalidInstalledStatus = api.NewValidationError("invalid installed status")
)

type updateServerInput struct {
	Installed        *int           `json:"installed,omitempty"`
	ProcessActive    *flexible.Bool `json:"process_active,omitempty"`
	LastProcessCheck *flexible.Time `json:"last_process_check,omitempty"`
}

func (in *updateServerInput) Validate() error {
	if in.Installed != nil && (*in.Installed < 0 || *in.Installed >= 3) {
		return ErrInvalidInstalledStatus
	}

	return nil
}
