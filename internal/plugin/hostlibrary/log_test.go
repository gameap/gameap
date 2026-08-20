package hostlibrary

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/gameap/gameap/pkg/plugin/sdk/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type logEntry struct {
	Level  string            `json:"level"`
	Msg    string            `json:"msg"`
	Fields map[string]string `json:"-"`
}

func TestLogService_Log(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		level     string
		message   string
		fields    map[string]string
		wantLevel string
	}{
		{
			name:      "log_debug_level",
			level:     "debug",
			message:   "debug message",
			fields:    nil,
			wantLevel: "DEBUG",
		},
		{
			name:      "log_info_level",
			level:     "info",
			message:   "info message",
			fields:    nil,
			wantLevel: "INFO",
		},
		{
			name:      "log_warn_level",
			level:     "warn",
			message:   "warn message",
			fields:    nil,
			wantLevel: "WARN",
		},
		{
			name:      "log_error_level",
			level:     "error",
			message:   "error message",
			fields:    nil,
			wantLevel: "ERROR",
		},
		{
			name:      "log_unknown_level_defaults_to_info",
			level:     "unknown",
			message:   "unknown level message",
			fields:    nil,
			wantLevel: "INFO",
		},
		{
			name:      "log_empty_level_defaults_to_info",
			level:     "",
			message:   "empty level message",
			fields:    nil,
			wantLevel: "INFO",
		},
		{
			name:    "log_fields_included_as_attrs",
			level:   "info",
			message: "message with fields",
			fields: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			wantLevel: "INFO",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			}))

			svc := NewLogService(logger)
			resp, err := svc.Log(context.Background(), &log.LogRequest{
				Level:   tt.level,
				Message: tt.message,
				Fields:  tt.fields,
			})

			require.NoError(t, err)
			assert.NotNil(t, resp)

			var entry logEntry
			err = json.Unmarshal(buf.Bytes(), &entry)
			require.NoError(t, err)

			assert.Equal(t, tt.wantLevel, entry.Level)
			assert.Equal(t, tt.message, entry.Msg)
		})
	}
}

func TestLogService_LogFieldsPresent(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	svc := NewLogService(logger)
	_, err := svc.Log(context.Background(), &log.LogRequest{
		Level:   "info",
		Message: "test message",
		Fields: map[string]string{
			"user_id":    "123",
			"request_id": "abc-def",
		},
	})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "user_id")
	assert.Contains(t, output, "123")
	assert.Contains(t, output, "request_id")
	assert.Contains(t, output, "abc-def")
}

func TestLogService_LogEmptyMessage(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	svc := NewLogService(logger)
	resp, err := svc.Log(context.Background(), &log.LogRequest{
		Level:   "info",
		Message: "",
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)

	var entry logEntry
	err = json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)
	assert.Empty(t, entry.Msg)
}

func TestLogService_LogNilFields(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	svc := NewLogService(logger)
	resp, err := svc.Log(context.Background(), &log.LogRequest{
		Level:   "info",
		Message: "no fields",
		Fields:  nil,
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)

	var entry logEntry
	err = json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)
	assert.Equal(t, "no fields", entry.Msg)
}

func TestNewLogHostLibrary(t *testing.T) {
	t.Parallel()
	logger := slog.Default()
	lib := NewLogHostLibrary(logger)

	assert.NotNil(t, lib)
	assert.NotNil(t, lib.impl)
}
