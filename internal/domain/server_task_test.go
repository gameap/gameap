package domain

import (
	"testing"
	"time"

	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

func TestServerTaskCommandConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, ServerTaskCommand("start"), ServerTaskCommandStart)
	assert.Equal(t, ServerTaskCommand("stop"), ServerTaskCommandStop)
	assert.Equal(t, ServerTaskCommand("restart"), ServerTaskCommandRestart)
	assert.Equal(t, ServerTaskCommand("update"), ServerTaskCommandUpdate)
	assert.Equal(t, ServerTaskCommand("reinstall"), ServerTaskCommandReinstall)
}

func TestNewServerTaskCommandFromString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected ServerTaskCommand
	}{
		{
			name:     "start_command",
			input:    "start",
			expected: ServerTaskCommandStart,
		},
		{
			name:     "stop_command",
			input:    "stop",
			expected: ServerTaskCommandStop,
		},
		{
			name:     "restart_command",
			input:    "restart",
			expected: ServerTaskCommandRestart,
		},
		{
			name:     "update_command",
			input:    "update",
			expected: ServerTaskCommandUpdate,
		},
		{
			name:     "reinstall_command",
			input:    "reinstall",
			expected: ServerTaskCommandReinstall,
		},
		{
			name:     "unknown_command_returns_empty",
			input:    "unknown",
			expected: "",
		},
		{
			name:     "empty_string_returns_empty",
			input:    "",
			expected: "",
		},
		{
			name:     "case_sensitive_uppercase",
			input:    "START",
			expected: "",
		},
		{
			name:     "invalid_command",
			input:    "invalid",
			expected: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result := NewServerTaskCommandFromString(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestNewServerTaskCommandFromString_AllValidCommands(t *testing.T) {
	t.Parallel()

	validCommands := []string{"start", "stop", "restart", "update", "reinstall"}

	for _, cmd := range validCommands {
		t.Run(cmd, func(t *testing.T) {
			t.Parallel()

			result := NewServerTaskCommandFromString(cmd)
			assert.NotEmpty(t, result, "valid command should not return empty")
			assert.Equal(t, ServerTaskCommand(cmd), result)
		})
	}
}

func TestServerTask_Fields(t *testing.T) {
	t.Parallel()

	now := time.Now()
	executeDate := time.Now().Add(1 * time.Hour)
	payload := testJSONPayload

	task := ServerTask{
		ID:           1,
		Command:      ServerTaskCommandStart,
		ServerID:     42,
		Repeat:       5,
		RepeatPeriod: 30 * time.Minute,
		Counter:      2,
		ExecuteDate:  executeDate,
		Payload:      &payload,
		CreatedAt:    &now,
		UpdatedAt:    &now,
	}

	assert.Equal(t, uint(1), task.ID)
	assert.Equal(t, ServerTaskCommandStart, task.Command)
	assert.Equal(t, uint(42), task.ServerID)
	assert.Equal(t, uint8(5), task.Repeat)
	assert.Equal(t, 30*time.Minute, task.RepeatPeriod)
	assert.Equal(t, uint(2), task.Counter)
	assert.Equal(t, executeDate, task.ExecuteDate)
	assert.Equal(t, &payload, task.Payload)
	assert.Equal(t, &now, task.CreatedAt)
	assert.Equal(t, &now, task.UpdatedAt)
}

func TestServerTask_WithoutOptionalFields(t *testing.T) {
	t.Parallel()

	executeDate := time.Now().Add(1 * time.Hour)

	task := ServerTask{
		ID:           1,
		Command:      ServerTaskCommandStop,
		ServerID:     10,
		Repeat:       0,
		RepeatPeriod: 0,
		Counter:      0,
		ExecuteDate:  executeDate,
		Payload:      nil,
		CreatedAt:    nil,
		UpdatedAt:    nil,
	}

	assert.Equal(t, uint(1), task.ID)
	assert.Equal(t, ServerTaskCommandStop, task.Command)
	assert.Equal(t, uint(10), task.ServerID)
	assert.Equal(t, uint8(0), task.Repeat)
	assert.Equal(t, time.Duration(0), task.RepeatPeriod)
	assert.Equal(t, uint(0), task.Counter)
	assert.Nil(t, task.Payload)
	assert.Nil(t, task.CreatedAt)
	assert.Nil(t, task.UpdatedAt)
}

func TestServerTask_DifferentCommands(t *testing.T) {
	t.Parallel()

	commands := []ServerTaskCommand{
		ServerTaskCommandStart,
		ServerTaskCommandStop,
		ServerTaskCommandRestart,
		ServerTaskCommandUpdate,
		ServerTaskCommandReinstall,
	}

	for _, cmd := range commands {
		t.Run(string(cmd), func(t *testing.T) {
			t.Parallel()

			task := ServerTask{
				ID:          1,
				Command:     cmd,
				ServerID:    42,
				ExecuteDate: time.Now(),
			}

			assert.Equal(t, cmd, task.Command)
		})
	}
}

func TestServerTask_RepeatPeriodDurations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		duration time.Duration
	}{
		{
			name:     "one_minute",
			duration: 1 * time.Minute,
		},
		{
			name:     "fifteen_minutes",
			duration: 15 * time.Minute,
		},
		{
			name:     "one_hour",
			duration: 1 * time.Hour,
		},
		{
			name:     "one_day",
			duration: 24 * time.Hour,
		},
		{
			name:     "one_week",
			duration: 7 * 24 * time.Hour,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			task := ServerTask{
				ID:           1,
				Command:      ServerTaskCommandStart,
				ServerID:     42,
				RepeatPeriod: test.duration,
				ExecuteDate:  time.Now(),
			}

			assert.Equal(t, test.duration, task.RepeatPeriod)
		})
	}
}

func TestServerTaskExecution_Fields(t *testing.T) {
	t.Parallel()

	now := time.Now()
	output := "Error: connection timeout"
	execID := xid.New()

	exec := ServerTaskExecution{
		ID:           1,
		ExecutionID:  execID,
		ServerTaskID: 10,
		Status:       ServerTaskExecutionStatusFailed,
		ErrorMessage: &output,
		StartedAt:    now,
		FinishedAt:   &now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	assert.Equal(t, uint(1), exec.ID)
	assert.Equal(t, execID, exec.ExecutionID)
	assert.Equal(t, ServerTaskExecutionStatusFailed, exec.Status)
	assert.NotNil(t, exec.ErrorMessage)
	assert.Equal(t, output, *exec.ErrorMessage)
}

func TestNewServerTaskOverlapPolicyFromString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  ServerTaskOverlapPolicy
	}{
		{name: "skip", input: "skip", want: ServerTaskOverlapPolicySkip},
		{name: "queue", input: "queue", want: ServerTaskOverlapPolicyQueue},
		{name: "empty_string_falls_back_to_skip", input: "", want: ServerTaskOverlapPolicySkip},
		{name: "unknown_value_falls_back_to_skip", input: "parallel", want: ServerTaskOverlapPolicySkip},
		{name: "uppercase_is_not_recognised", input: "QUEUE", want: ServerTaskOverlapPolicySkip},
		{name: "padded_value_is_not_recognised", input: " queue ", want: ServerTaskOverlapPolicySkip},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			stored := tt.input

			// ACT
			got := NewServerTaskOverlapPolicyFromString(stored)

			// ASSERT
			assert.Equal(t, tt.want, got, "overlap policy for %q", stored)
		})
	}
}

func TestNewServerTaskCatchupPolicyFromString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  ServerTaskCatchupPolicy
	}{
		{name: "skip", input: "skip", want: ServerTaskCatchupPolicySkip},
		{name: "run_once", input: "run_once", want: ServerTaskCatchupPolicyRunOnce},
		{name: "empty_string_falls_back_to_skip", input: "", want: ServerTaskCatchupPolicySkip},
		{name: "unknown_value_falls_back_to_skip", input: "run_all", want: ServerTaskCatchupPolicySkip},
		{name: "uppercase_is_not_recognised", input: "RUN_ONCE", want: ServerTaskCatchupPolicySkip},
		{name: "hyphenated_variant_is_not_recognised", input: "run-once", want: ServerTaskCatchupPolicySkip},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			stored := tt.input

			// ACT
			got := NewServerTaskCatchupPolicyFromString(stored)

			// ASSERT
			assert.Equal(t, tt.want, got, "catchup policy for %q", stored)
		})
	}
}
