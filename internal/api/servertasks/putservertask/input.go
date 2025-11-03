package putservertask

import (
	"log/slog"
	"math"
	"regexp"
	"slices"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/carbon"
	"github.com/gameap/gameap/pkg/flexible"
)

var (
	ErrCommandIsRequired = api.NewValidationError("command is required")
	ErrInvalidCommand    = api.NewValidationError(
		"invalid command, must be one of: start, stop, restart, update, reinstall",
	)
	ErrExecuteDateIsRequired = api.NewValidationError("execute_date is required")
	ErrInvalidRepeat         = api.NewValidationError("repeat must be between 0 and 255")
	ErrRepeatPeriodRequired  = api.NewValidationError("repeat_period is required when repeat is not 1")
	ErrInvalidRepeatPeriod   = api.NewValidationError(
		"repeat_period must match format: '<number> <unit>' (e.g., '1 hour', '30 minutes')",
	)
	ErrRepeatPeriodIsTooShort = api.NewValidationError("10 minutes is minimum repeat period")
	ErrRepeatPeriodIsTooLong  = api.NewValidationError("repeat period is too long")
)

var validCommands = []string{"start", "stop", "restart", "update", "reinstall"}
var repeatPeriodRegex = regexp.MustCompile(`^\d+\s\w+$`)

type serverTaskInput struct {
	Command      string         `json:"command"`
	Repeat       *int           `json:"repeat"`
	RepeatPeriod *string        `json:"repeat_period,omitempty"`
	ExecuteDate  *flexible.Time `json:"execute_date"`
	Payload      *string        `json:"payload,omitempty"`
}

func (s *serverTaskInput) Validate() error {
	if s.Command == "" {
		return ErrCommandIsRequired
	}

	if !isValidCommand(s.Command) {
		return ErrInvalidCommand
	}

	if s.ExecuteDate == nil {
		return ErrExecuteDateIsRequired
	}

	if s.Repeat != nil { //nolint:nestif
		if *s.Repeat > 255 || *s.Repeat < 0 {
			return ErrInvalidRepeat
		}

		if *s.Repeat != 1 && (s.RepeatPeriod == nil || *s.RepeatPeriod == "") {
			return ErrRepeatPeriodRequired
		}

		if s.RepeatPeriod != nil && *s.RepeatPeriod != "" {
			if !repeatPeriodRegex.MatchString(*s.RepeatPeriod) {
				return ErrInvalidRepeatPeriod
			}
		}
	}

	return nil
}

func (s *serverTaskInput) ToDomain(serverID uint, existingTask *domain.ServerTask) (*domain.ServerTask, error) {
	var err error

	task := &domain.ServerTask{
		ID:          existingTask.ID,
		Command:     domain.NewServerTaskCommandFromString(s.Command),
		ServerID:    serverID,
		ExecuteDate: s.ExecuteDate.Time,
		Payload:     s.Payload,
		Counter:     existingTask.Counter,
		CreatedAt:   existingTask.CreatedAt,
		UpdatedAt:   existingTask.UpdatedAt,
	}

	if s.Repeat != nil && *s.Repeat < math.MaxUint8 {
		task.Repeat = uint8(*s.Repeat) //nolint:gosec // overflow validation above
	}

	if s.RepeatPeriod != nil && *s.RepeatPeriod != "" {
		task.RepeatPeriod, err = carbon.ParseInterval(*s.RepeatPeriod)
		if err != nil {
			slog.Warn("failed to parse repeat_period", "error", err)

			return nil, ErrInvalidRepeatPeriod
		}
	}

	if task.Repeat > 1 || task.Repeat <= 0 {
		if task.RepeatPeriod < 10*time.Minute {
			return nil, ErrRepeatPeriodIsTooShort
		}

		if task.RepeatPeriod > 365*24*time.Hour {
			return nil, ErrRepeatPeriodIsTooLong
		}
	}

	return task, nil
}

func isValidCommand(command string) bool {
	return slices.Contains(validCommands, command)
}
