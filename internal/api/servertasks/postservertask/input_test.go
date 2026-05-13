package postservertask

import (
	"strings"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/flexible"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerTaskInput_Validate(t *testing.T) {
	future := new(flexible.Time{Time: time.Now().Add(time.Hour)})
	pastBeyondGrace := new(flexible.Time{Time: time.Now().Add(-2 * time.Minute)})
	pastWithinGrace := new(flexible.Time{Time: time.Now().Add(-30 * time.Second)})
	name128 := strings.Repeat("a", 128)
	name129 := strings.Repeat("a", 129)

	tests := []struct {
		name      string
		input     serverTaskInput
		wantError string
	}{
		{
			name:      "empty_command_returns_error",
			input:     serverTaskInput{Command: "", ExecuteDate: future},
			wantError: "command is required",
		},
		{
			name:      "invalid_command_returns_error",
			input:     serverTaskInput{Command: "boom", ExecuteDate: future},
			wantError: "invalid command",
		},
		{
			name:  "valid_command_start",
			input: serverTaskInput{Command: "start", ExecuteDate: future},
		},
		{
			name:  "valid_command_stop",
			input: serverTaskInput{Command: "stop", ExecuteDate: future},
		},
		{
			name:  "valid_command_restart",
			input: serverTaskInput{Command: "restart", ExecuteDate: future},
		},
		{
			name:  "valid_command_update",
			input: serverTaskInput{Command: "update", ExecuteDate: future},
		},
		{
			name:  "valid_command_reinstall",
			input: serverTaskInput{Command: "reinstall", ExecuteDate: future},
		},
		{
			name:      "nil_execute_date_returns_error",
			input:     serverTaskInput{Command: "start"},
			wantError: "execute_date is required",
		},
		{
			name:      "execute_date_in_past_beyond_grace_returns_error",
			input:     serverTaskInput{Command: "start", ExecuteDate: pastBeyondGrace},
			wantError: "execute_date must not be in the past",
		},
		{
			name:  "execute_date_within_grace_window_passes",
			input: serverTaskInput{Command: "start", ExecuteDate: pastWithinGrace},
		},
		{
			name: "name_at_128_chars_passes",
			input: serverTaskInput{
				Command:     "start",
				ExecuteDate: future,
				Name:        &name128,
			},
		},
		{
			name: "name_at_129_chars_returns_error",
			input: serverTaskInput{
				Command:     "start",
				ExecuteDate: future,
				Name:        &name129,
			},
			wantError: "name must be at most 128 characters",
		},
		{
			name: "invalid_overlap_policy_returns_error",
			input: serverTaskInput{
				Command:       "start",
				ExecuteDate:   future,
				OverlapPolicy: new("foo"),
			},
			wantError: "invalid overlap_policy",
		},
		{
			name: "overlap_policy_skip_passes",
			input: serverTaskInput{
				Command:       "start",
				ExecuteDate:   future,
				OverlapPolicy: new("skip"),
			},
		},
		{
			name: "overlap_policy_queue_passes",
			input: serverTaskInput{
				Command:       "start",
				ExecuteDate:   future,
				OverlapPolicy: new("queue"),
			},
		},
		{
			name: "invalid_catchup_policy_returns_error",
			input: serverTaskInput{
				Command:       "start",
				ExecuteDate:   future,
				CatchupPolicy: new("foo"),
			},
			wantError: "invalid catchup_policy",
		},
		{
			name: "catchup_policy_skip_passes",
			input: serverTaskInput{
				Command:       "start",
				ExecuteDate:   future,
				CatchupPolicy: new("skip"),
			},
		},
		{
			name: "catchup_policy_run_once_passes",
			input: serverTaskInput{
				Command:       "start",
				ExecuteDate:   future,
				CatchupPolicy: new("run_once"),
			},
		},
		{
			name: "repeat_negative_returns_error",
			input: serverTaskInput{
				Command:     "start",
				ExecuteDate: future,
				Repeat:      new(-1),
			},
			wantError: "repeat must be between 0 and 255",
		},
		{
			name: "repeat_over_255_returns_error",
			input: serverTaskInput{
				Command:     "start",
				ExecuteDate: future,
				Repeat:      new(256),
			},
			wantError: "repeat must be between 0 and 255",
		},
		{
			name: "repeat_2_no_period_returns_error",
			input: serverTaskInput{
				Command:     "start",
				ExecuteDate: future,
				Repeat:      new(2),
			},
			wantError: "repeat_period is required",
		},
		{
			name: "repeat_2_empty_period_returns_error",
			input: serverTaskInput{
				Command:      "start",
				ExecuteDate:  future,
				Repeat:       new(2),
				RepeatPeriod: new(""),
			},
			wantError: "repeat_period is required",
		},
		{
			name: "repeat_2_invalid_period_format_returns_error",
			input: serverTaskInput{
				Command:      "start",
				ExecuteDate:  future,
				Repeat:       new(2),
				RepeatPeriod: new("not-a-period"),
			},
			wantError: "repeat_period must match format",
		},
		{
			name: "repeat_1_no_period_passes",
			input: serverTaskInput{
				Command:     "start",
				ExecuteDate: future,
				Repeat:      new(1),
			},
		},
		{
			name: "repeat_0_with_valid_period_passes",
			input: serverTaskInput{
				Command:      "start",
				ExecuteDate:  future,
				Repeat:       new(0),
				RepeatPeriod: new("1 hour"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()

			if tt.wantError == "" {
				assert.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
		})
	}
}

func TestServerTaskInput_ToDomain(t *testing.T) {
	const serverID = uint(42)
	future := time.Now().Add(time.Hour).Truncate(time.Second)
	futureWrapped := new(flexible.Time{Time: future})

	tests := []struct {
		name        string
		input       serverTaskInput
		wantError   string
		checkResult func(t *testing.T, task *domain.ServerTask)
	}{
		{
			name: "minimal_input_applies_defaults",
			input: serverTaskInput{
				Command:     "start",
				ExecuteDate: futureWrapped,
				Repeat:      new(1),
			},
			checkResult: func(t *testing.T, task *domain.ServerTask) {
				t.Helper()

				assert.Equal(t, domain.ServerTaskCommandStart, task.Command)
				assert.Equal(t, serverID, task.ServerID)
				assert.True(t, task.ExecuteDate.Equal(future), "execute_date must be copied")
				assert.True(t, task.Enabled, "enabled must default to true")
				assert.Equal(t, domain.ServerTaskOverlapPolicySkip, task.OverlapPolicy)
				assert.Equal(t, domain.ServerTaskCatchupPolicySkip, task.CatchupPolicy)
				assert.Equal(t, uint64(1), task.Version)
				assert.Nil(t, task.Name)
				assert.Nil(t, task.Payload)
				assert.Nil(t, task.Timezone)
				assert.Equal(t, uint8(1), task.Repeat)
				assert.Equal(t, time.Duration(0), task.RepeatPeriod)
			},
		},
		{
			name: "name_copied_when_provided",
			input: serverTaskInput{
				Command:     "start",
				ExecuteDate: futureWrapped,
				Repeat:      new(1),
				Name:        new("backup"),
			},
			checkResult: func(t *testing.T, task *domain.ServerTask) {
				t.Helper()

				require.NotNil(t, task.Name)
				assert.Equal(t, "backup", *task.Name)
			},
		},
		{
			name: "enabled_false_when_provided",
			input: serverTaskInput{
				Command:     "start",
				ExecuteDate: futureWrapped,
				Repeat:      new(1),
				Enabled:     new(false),
			},
			checkResult: func(t *testing.T, task *domain.ServerTask) {
				t.Helper()

				assert.False(t, task.Enabled)
			},
		},
		{
			name: "overlap_policy_skip_mapped",
			input: serverTaskInput{
				Command:       "start",
				ExecuteDate:   futureWrapped,
				Repeat:        new(1),
				OverlapPolicy: new("skip"),
			},
			checkResult: func(t *testing.T, task *domain.ServerTask) {
				t.Helper()

				assert.Equal(t, domain.ServerTaskOverlapPolicySkip, task.OverlapPolicy)
			},
		},
		{
			name: "overlap_policy_queue_mapped",
			input: serverTaskInput{
				Command:       "start",
				ExecuteDate:   futureWrapped,
				Repeat:        new(1),
				OverlapPolicy: new("queue"),
			},
			checkResult: func(t *testing.T, task *domain.ServerTask) {
				t.Helper()

				assert.Equal(t, domain.ServerTaskOverlapPolicyQueue, task.OverlapPolicy)
			},
		},
		{
			name: "catchup_policy_skip_mapped",
			input: serverTaskInput{
				Command:       "start",
				ExecuteDate:   futureWrapped,
				Repeat:        new(1),
				CatchupPolicy: new("skip"),
			},
			checkResult: func(t *testing.T, task *domain.ServerTask) {
				t.Helper()

				assert.Equal(t, domain.ServerTaskCatchupPolicySkip, task.CatchupPolicy)
			},
		},
		{
			name: "catchup_policy_run_once_mapped",
			input: serverTaskInput{
				Command:       "start",
				ExecuteDate:   futureWrapped,
				Repeat:        new(1),
				CatchupPolicy: new("run_once"),
			},
			checkResult: func(t *testing.T, task *domain.ServerTask) {
				t.Helper()

				assert.Equal(t, domain.ServerTaskCatchupPolicyRunOnce, task.CatchupPolicy)
			},
		},
		{
			name: "timezone_copied_when_provided",
			input: serverTaskInput{
				Command:     "start",
				ExecuteDate: futureWrapped,
				Repeat:      new(1),
				Timezone:    new("UTC"),
			},
			checkResult: func(t *testing.T, task *domain.ServerTask) {
				t.Helper()

				require.NotNil(t, task.Timezone)
				assert.Equal(t, "UTC", *task.Timezone)
			},
		},
		{
			name: "payload_copied_when_provided",
			input: serverTaskInput{
				Command:     "start",
				ExecuteDate: futureWrapped,
				Repeat:      new(1),
				Payload:     new("data"),
			},
			checkResult: func(t *testing.T, task *domain.ServerTask) {
				t.Helper()

				require.NotNil(t, task.Payload)
				assert.Equal(t, "data", *task.Payload)
			},
		},
		{
			name: "repeat_period_1_hour_parsed",
			input: serverTaskInput{
				Command:      "start",
				ExecuteDate:  futureWrapped,
				Repeat:       new(2),
				RepeatPeriod: new("1 hour"),
			},
			checkResult: func(t *testing.T, task *domain.ServerTask) {
				t.Helper()

				assert.Equal(t, uint8(2), task.Repeat)
				assert.Equal(t, time.Hour, task.RepeatPeriod)
			},
		},
		{
			name: "repeat_period_too_short_returns_error",
			input: serverTaskInput{
				Command:      "start",
				ExecuteDate:  futureWrapped,
				Repeat:       new(2),
				RepeatPeriod: new("5 minutes"),
			},
			wantError: "10 minutes is minimum repeat period",
		},
		{
			name: "repeat_period_too_long_returns_error",
			input: serverTaskInput{
				Command:      "start",
				ExecuteDate:  futureWrapped,
				Repeat:       new(2),
				RepeatPeriod: new("400 days"),
			},
			wantError: "repeat period is too long",
		},
		{
			name: "unparseable_repeat_period_returns_error",
			input: serverTaskInput{
				Command:      "start",
				ExecuteDate:  futureWrapped,
				Repeat:       new(2),
				RepeatPeriod: new("5 zonks"),
			},
			wantError: "repeat_period must match format",
		},
		{
			name: "repeat_zero_with_short_period_returns_error",
			input: serverTaskInput{
				Command:      "start",
				ExecuteDate:  futureWrapped,
				Repeat:       new(0),
				RepeatPeriod: new("5 minutes"),
			},
			wantError: "10 minutes is minimum repeat period",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, err := tt.input.ToDomain(serverID)

			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
				assert.Nil(t, task, "task must be nil when error returned")

				return
			}

			require.NoError(t, err)
			require.NotNil(t, task)
			if tt.checkResult != nil {
				tt.checkResult(t, task)
			}
		})
	}
}
