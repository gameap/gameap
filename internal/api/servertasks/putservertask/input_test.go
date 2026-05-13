package putservertask

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
	createdAt := time.Now().Add(-24 * time.Hour).Truncate(time.Second)
	updatedAt := time.Now().Add(-1 * time.Hour).Truncate(time.Second)
	deletedAt := time.Now().Add(-30 * time.Minute).Truncate(time.Second)

	newExistingTask := func() *domain.ServerTask {
		return &domain.ServerTask{
			ID:              99,
			Command:         domain.ServerTaskCommandStop,
			ServerID:        1,
			NodeID:          new(uint(7)),
			Name:            new("oldname"),
			Enabled:         false,
			OverlapPolicy:   domain.ServerTaskOverlapPolicyQueue,
			CatchupPolicy:   domain.ServerTaskCatchupPolicyRunOnce,
			CreatedByUserID: new(uint(11)),
			Version:         3,
			Timezone:        new("Europe/Moscow"),
			Repeat:          4,
			RepeatPeriod:    30 * time.Minute,
			Counter:         5,
			CreatedAt:       &createdAt,
			UpdatedAt:       &updatedAt,
			DeletedAt:       &deletedAt,
		}
	}

	tests := []struct {
		name         string
		input        serverTaskInput
		existingTask *domain.ServerTask
		wantError    string
		checkResult  func(t *testing.T, task *domain.ServerTask)
	}{
		{
			name: "minimal_input_applies_defaults_from_existing",
			input: serverTaskInput{
				Command:     "start",
				ExecuteDate: futureWrapped,
				Repeat:      new(1),
			},
			existingTask: &domain.ServerTask{
				ID:      99,
				Enabled: true,
				Version: 3,
			},
			checkResult: func(t *testing.T, task *domain.ServerTask) {
				t.Helper()

				assert.Equal(t, domain.ServerTaskCommandStart, task.Command)
				assert.Equal(t, serverID, task.ServerID)
				assert.True(t, task.ExecuteDate.Equal(future))
				assert.Equal(t, uint(99), task.ID, "id must be preserved from existing")
				assert.Equal(t, uint64(3), task.Version, "version must be preserved from existing")
				assert.True(t, task.Enabled, "enabled must come from existing when input nil")
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
			existingTask: newExistingTask(),
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
			existingTask: &domain.ServerTask{Enabled: true},
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
			existingTask: newExistingTask(),
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
			existingTask: &domain.ServerTask{OverlapPolicy: domain.ServerTaskOverlapPolicySkip},
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
			existingTask: newExistingTask(),
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
			existingTask: &domain.ServerTask{CatchupPolicy: domain.ServerTaskCatchupPolicySkip},
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
			existingTask: newExistingTask(),
			checkResult: func(t *testing.T, task *domain.ServerTask) {
				t.Helper()

				require.NotNil(t, task.Timezone)
				assert.Equal(t, "UTC", *task.Timezone)
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
			existingTask: &domain.ServerTask{},
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
			existingTask: &domain.ServerTask{},
			wantError:    "10 minutes is minimum repeat period",
		},
		{
			name: "repeat_period_too_long_returns_error",
			input: serverTaskInput{
				Command:      "start",
				ExecuteDate:  futureWrapped,
				Repeat:       new(2),
				RepeatPeriod: new("400 days"),
			},
			existingTask: &domain.ServerTask{},
			wantError:    "repeat period is too long",
		},
		{
			name: "unparseable_repeat_period_returns_error",
			input: serverTaskInput{
				Command:      "start",
				ExecuteDate:  futureWrapped,
				Repeat:       new(2),
				RepeatPeriod: new("5 zonks"),
			},
			existingTask: &domain.ServerTask{},
			wantError:    "repeat_period must match format",
		},
		{
			name: "repeat_zero_with_short_period_returns_error",
			input: serverTaskInput{
				Command:      "start",
				ExecuteDate:  futureWrapped,
				Repeat:       new(0),
				RepeatPeriod: new("5 minutes"),
			},
			existingTask: &domain.ServerTask{},
			wantError:    "10 minutes is minimum repeat period",
		},
		{
			name: "preserves_existing_task_metadata",
			input: serverTaskInput{
				Command:     "start",
				ExecuteDate: futureWrapped,
				Repeat:      new(1),
			},
			existingTask: newExistingTask(),
			checkResult: func(t *testing.T, task *domain.ServerTask) {
				t.Helper()

				assert.Equal(t, uint(99), task.ID, "id must be preserved")
				require.NotNil(t, task.NodeID)
				assert.Equal(t, uint(7), *task.NodeID, "node_id must be preserved")
				require.NotNil(t, task.CreatedByUserID)
				assert.Equal(t, uint(11), *task.CreatedByUserID, "created_by must be preserved")
				assert.Equal(t, uint64(3), task.Version, "version must be preserved")
				assert.Equal(t, uint(5), task.Counter, "counter must be preserved")
				require.NotNil(t, task.CreatedAt)
				assert.True(t, task.CreatedAt.Equal(createdAt))
				require.NotNil(t, task.UpdatedAt)
				assert.True(t, task.UpdatedAt.Equal(updatedAt))
				require.NotNil(t, task.DeletedAt)
				assert.True(t, task.DeletedAt.Equal(deletedAt))
				assert.Equal(t, domain.ServerTaskCommandStart, task.Command, "command must be overwritten from input")
				assert.Equal(t, serverID, task.ServerID, "server_id must be overwritten from input")
				assert.True(t, task.ExecuteDate.Equal(future), "execute_date must be overwritten from input")
			},
		},
		{
			name: "name_preserved_from_existing_when_input_nil",
			input: serverTaskInput{
				Command:     "start",
				ExecuteDate: futureWrapped,
				Repeat:      new(1),
			},
			existingTask: newExistingTask(),
			checkResult: func(t *testing.T, task *domain.ServerTask) {
				t.Helper()

				require.NotNil(t, task.Name)
				assert.Equal(t, "oldname", *task.Name)
			},
		},
		{
			name: "enabled_preserved_from_existing_when_input_nil",
			input: serverTaskInput{
				Command:     "start",
				ExecuteDate: futureWrapped,
				Repeat:      new(1),
			},
			existingTask: &domain.ServerTask{Enabled: false},
			checkResult: func(t *testing.T, task *domain.ServerTask) {
				t.Helper()

				assert.False(t, task.Enabled, "enabled must come from existing when input nil")
			},
		},
		{
			name: "overlap_policy_preserved_when_input_nil",
			input: serverTaskInput{
				Command:     "start",
				ExecuteDate: futureWrapped,
				Repeat:      new(1),
			},
			existingTask: &domain.ServerTask{OverlapPolicy: domain.ServerTaskOverlapPolicyQueue},
			checkResult: func(t *testing.T, task *domain.ServerTask) {
				t.Helper()

				assert.Equal(t, domain.ServerTaskOverlapPolicyQueue, task.OverlapPolicy)
			},
		},
		{
			name: "catchup_policy_preserved_when_input_nil",
			input: serverTaskInput{
				Command:     "start",
				ExecuteDate: futureWrapped,
				Repeat:      new(1),
			},
			existingTask: &domain.ServerTask{CatchupPolicy: domain.ServerTaskCatchupPolicyRunOnce},
			checkResult: func(t *testing.T, task *domain.ServerTask) {
				t.Helper()

				assert.Equal(t, domain.ServerTaskCatchupPolicyRunOnce, task.CatchupPolicy)
			},
		},
		{
			name: "timezone_preserved_when_input_nil",
			input: serverTaskInput{
				Command:     "start",
				ExecuteDate: futureWrapped,
				Repeat:      new(1),
			},
			existingTask: newExistingTask(),
			checkResult: func(t *testing.T, task *domain.ServerTask) {
				t.Helper()

				require.NotNil(t, task.Timezone)
				assert.Equal(t, "Europe/Moscow", *task.Timezone)
			},
		},
		{
			name: "payload_overwritten_from_input",
			input: serverTaskInput{
				Command:     "start",
				ExecuteDate: futureWrapped,
				Repeat:      new(1),
				Payload:     new("newpayload"),
			},
			existingTask: newExistingTask(),
			checkResult: func(t *testing.T, task *domain.ServerTask) {
				t.Helper()

				require.NotNil(t, task.Payload)
				assert.Equal(t, "newpayload", *task.Payload)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, err := tt.input.ToDomain(serverID, tt.existingTask)

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
