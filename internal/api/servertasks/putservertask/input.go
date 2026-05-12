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
	ErrExecuteDateInPast     = api.NewValidationError("execute_date must not be in the past")
	ErrInvalidRepeat         = api.NewValidationError("repeat must be between 0 and 255")
	ErrRepeatPeriodRequired  = api.NewValidationError("repeat_period is required when repeat is not 1")
	ErrInvalidRepeatPeriod   = api.NewValidationError(
		"repeat_period must match format: '<number> <unit>' (e.g., '1 hour', '30 minutes')",
	)
	ErrRepeatPeriodIsTooShort = api.NewValidationError("10 minutes is minimum repeat period")
	ErrRepeatPeriodIsTooLong  = api.NewValidationError("repeat period is too long")
	ErrInvalidOverlapPolicy   = api.NewValidationError("invalid overlap_policy, must be one of: skip, queue")
	ErrInvalidCatchupPolicy   = api.NewValidationError("invalid catchup_policy, must be one of: skip, run_once")
	ErrInvalidName            = api.NewValidationError("name must be at most 128 characters")
)

var validCommands = []string{"start", "stop", "restart", "update", "reinstall"}
var validOverlapPolicies = []string{"skip", "queue"}
var validCatchupPolicies = []string{"skip", "run_once"}
var repeatPeriodRegex = regexp.MustCompile(`^\d+\s\w+$`)

type serverTaskInput struct {
	Command       string         `json:"command"`
	Name          *string        `json:"name,omitempty"`
	Enabled       *bool          `json:"enabled,omitempty"`
	OverlapPolicy *string        `json:"overlap_policy,omitempty"`
	CatchupPolicy *string        `json:"catchup_policy,omitempty"`
	Timezone      *string        `json:"timezone,omitempty"`
	Repeat        *int           `json:"repeat"`
	RepeatPeriod  *string        `json:"repeat_period,omitempty"`
	ExecuteDate   *flexible.Time `json:"execute_date"`
	Payload       *string        `json:"payload,omitempty"`
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

	if s.ExecuteDate.Before(time.Now().Add(-1 * time.Minute)) {
		return ErrExecuteDateInPast
	}

	if s.Name != nil && len(*s.Name) > 128 {
		return ErrInvalidName
	}

	if s.OverlapPolicy != nil && !slices.Contains(validOverlapPolicies, *s.OverlapPolicy) {
		return ErrInvalidOverlapPolicy
	}

	if s.CatchupPolicy != nil && !slices.Contains(validCatchupPolicies, *s.CatchupPolicy) {
		return ErrInvalidCatchupPolicy
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
		ID:              existingTask.ID,
		Command:         domain.NewServerTaskCommandFromString(s.Command),
		ServerID:        serverID,
		NodeID:          existingTask.NodeID,
		Name:            existingTask.Name,
		Enabled:         existingTask.Enabled,
		OverlapPolicy:   existingTask.OverlapPolicy,
		CatchupPolicy:   existingTask.CatchupPolicy,
		CreatedByUserID: existingTask.CreatedByUserID,
		Version:         existingTask.Version,
		Timezone:        existingTask.Timezone,
		ExecuteDate:     s.ExecuteDate.Time,
		Payload:         s.Payload,
		Counter:         existingTask.Counter,
		CreatedAt:       existingTask.CreatedAt,
		UpdatedAt:       existingTask.UpdatedAt,
		DeletedAt:       existingTask.DeletedAt,
	}

	if s.Name != nil {
		task.Name = s.Name
	}
	if s.Enabled != nil {
		task.Enabled = *s.Enabled
	}
	if s.OverlapPolicy != nil {
		task.OverlapPolicy = domain.NewServerTaskOverlapPolicyFromString(*s.OverlapPolicy)
	}
	if s.CatchupPolicy != nil {
		task.CatchupPolicy = domain.NewServerTaskCatchupPolicyFromString(*s.CatchupPolicy)
	}
	if s.Timezone != nil {
		task.Timezone = s.Timezone
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
