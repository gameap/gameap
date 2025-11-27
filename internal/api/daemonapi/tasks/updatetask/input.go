package updatetask

import (
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/flexible"
	"github.com/samber/lo"
)

var (
	ErrEmptyStatus   = api.NewValidationError("empty status")
	ErrInvalidStatus = api.NewValidationError("invalid status")
)

type updateTaskInput struct {
	Status *flexible.Int `json:"status"`
}

func (in *updateTaskInput) Validate() error {
	if in.Status == nil {
		return ErrEmptyStatus
	}

	isValid := false
	for _, validStatus := range domain.DaemonTaskStatusNums {
		if in.Status.Int() == validStatus {
			isValid = true

			break
		}
	}

	if !isValid {
		return ErrInvalidStatus
	}

	return nil
}

func (in *updateTaskInput) ToStatus() domain.DaemonTaskStatus {
	if in.Status == nil {
		return ""
	}

	return lo.Invert(domain.DaemonTaskStatusNums)[in.Status.Int()]
}
