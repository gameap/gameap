package updateservertask

import (
	"math"

	"github.com/gameap/gameap/pkg/flexible"
	"github.com/pkg/errors"
)

type updateServerTaskInput struct {
	Counter      *flexible.Uint `json:"counter"`
	Repeat       *flexible.Int  `json:"repeat"`
	RepeatPeriod *flexible.Int  `json:"repeat_period"`
	ExecuteDate  *flexible.Time `json:"execute_date"`
}

func (in *updateServerTaskInput) Validate() error {
	if in.ExecuteDate == nil {
		return errors.New("execute_date is required")
	}

	if in.RepeatPeriod != nil && in.RepeatPeriod.Int() < 0 {
		return errors.New("repeat_period must be non-negative")
	}

	if in.Repeat != nil {
		if in.Repeat.Int() < 0 {
			return errors.New("repeat must be non-negative")
		}

		if in.Repeat.Int() > math.MaxUint8 {
			return errors.New("repeat exceeds maximum value of 255")
		}
	}

	return nil
}
