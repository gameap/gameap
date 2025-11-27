package updateservertask

import (
	"github.com/gameap/gameap/pkg/flexible"
	"github.com/pkg/errors"
)

type updateServerTaskInput struct {
	Counter      *uint          `json:"counter"`
	Repeat       *uint8         `json:"repeat"`
	RepeatPeriod *int           `json:"repeat_period"`
	ExecuteDate  *flexible.Time `json:"execute_date"`
}

func (in *updateServerTaskInput) Validate() error {
	if in.ExecuteDate == nil {
		return errors.New("execute_date is required")
	}

	if in.Repeat != nil && *in.Repeat < 1 {
		return errors.New("repeat must be at least 1")
	}

	if in.RepeatPeriod != nil && *in.RepeatPeriod < 0 {
		return errors.New("repeat_period must be non-negative")
	}

	return nil
}
