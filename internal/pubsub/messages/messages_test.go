package messages

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/pubsub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	xidPattern  = regexp.MustCompile(`^[0-9a-v]{20}$`)
	typePattern = regexp.MustCompile(`^[a-z][a-z_]*(\.[a-z_]+)*$`)
)

func TestNewMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		channel   string
		msgType   string
		payload   any
		wantError string
		check     func(t *testing.T, msg *pubsub.Message)
	}{
		{
			name:    "simple_payload_marshals_correctly",
			channel: "cache:invalidate",
			msgType: TypeCacheInvalidate,
			payload: CacheInvalidatePayload{
				EntityType: "games",
				EntityIDs:  []string{"1", "2"},
			},
			check: func(t *testing.T, msg *pubsub.Message) {
				t.Helper()

				require.NotNil(t, msg)
				assert.True(t, xidPattern.MatchString(msg.ID), "ID %q must match XID format", msg.ID)
				assert.Equal(t, "cache:invalidate", msg.Channel)
				assert.Equal(t, TypeCacheInvalidate, msg.Type)
				assert.Empty(t, msg.Source, "Source must not be set by NewMessage")
				assert.WithinDuration(t, time.Now(), msg.Timestamp, time.Second)

				var raw map[string]any
				require.NoError(t, json.Unmarshal(msg.Payload, &raw))
				assert.Equal(t, "games", raw["entity_type"])

				ids, ok := raw["entity_ids"].([]any)
				require.True(t, ok, "entity_ids must be present as array")
				require.Len(t, ids, 2)
				assert.Equal(t, "1", ids[0])
				assert.Equal(t, "2", ids[1])
			},
		},
		{
			name:    "nil_payload_marshals_as_null",
			channel: "any",
			msgType: TypeNotification,
			payload: nil,
			check: func(t *testing.T, msg *pubsub.Message) {
				t.Helper()

				require.NotNil(t, msg)
				assert.JSONEq(t, "null", string(msg.Payload))
			},
		},
		{
			name:    "empty_struct_payload_marshals_as_object",
			channel: "any",
			msgType: TypeServerStatus,
			payload: struct{}{},
			check: func(t *testing.T, msg *pubsub.Message) {
				t.Helper()

				require.NotNil(t, msg)
				assert.JSONEq(t, "{}", string(msg.Payload))
			},
		},
		{
			name:    "payload_with_nil_pointer_omits_field",
			channel: "plugin:events",
			msgType: TypePluginEvent,
			payload: PluginEventPayload{EventType: 5},
			check: func(t *testing.T, msg *pubsub.Message) {
				t.Helper()

				require.NotNil(t, msg)

				body := string(msg.Payload)
				assert.NotContains(t, body, `"server_id"`, "nil *uint must be omitted")
				assert.NotContains(t, body, `"task_id"`, "nil *uint must be omitted")
				assert.NotContains(t, body, `"node_id"`, "nil *uint must be omitted")
				assert.NotContains(t, body, `"extra_data"`, "nil map must be omitted")
				assert.Contains(t, body, `"event_type":5`)
			},
		},
		{
			name:    "payload_with_byte_slice_base64_encodes",
			channel: "daemon:tasks",
			msgType: TypeDaemonTask,
			payload: DaemonTaskDispatchPayload{
				NodeID:    1,
				RequestID: "req-1",
				TaskID:    42,
				TaskData:  []byte{0x01, 0x02, 0x03},
			},
			check: func(t *testing.T, msg *pubsub.Message) {
				t.Helper()

				require.NotNil(t, msg)

				var raw map[string]any
				require.NoError(t, json.Unmarshal(msg.Payload, &raw))

				expected := base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03})
				assert.Equal(t, expected, raw["task_data"], "byte slices must be base64-encoded by encoding/json")
			},
		},
		{
			name:    "payload_with_time_field_uses_rfc3339",
			channel: "metrics:subs",
			msgType: TypeMetricsSubscribers,
			payload: MetricsSubscribersPayload{
				InstanceID: "inst-1",
				NodeID:     7,
				Count:      3,
				Timestamp:  time.Date(2024, 5, 1, 10, 30, 0, 0, time.UTC),
			},
			check: func(t *testing.T, msg *pubsub.Message) {
				t.Helper()

				require.NotNil(t, msg)
				assert.Contains(t, string(msg.Payload), `"timestamp":"2024-05-01T10:30:00Z"`)
			},
		},
		{
			name:    "payload_with_map_marshals_correctly",
			channel: "plugin:events",
			msgType: TypePluginEvent,
			payload: PluginEventPayload{
				EventType: 1,
				ExtraData: map[string]string{"k": "v"},
			},
			check: func(t *testing.T, msg *pubsub.Message) {
				t.Helper()

				require.NotNil(t, msg)

				var raw map[string]any
				require.NoError(t, json.Unmarshal(msg.Payload, &raw))

				extra, ok := raw["extra_data"].(map[string]any)
				require.True(t, ok, "extra_data must be present as object")
				assert.Equal(t, "v", extra["k"])
			},
		},
		{
			name:      "marshal_error_returns_nil_message",
			channel:   "any",
			msgType:   "any",
			payload:   make(chan int),
			wantError: "chan",
			check: func(t *testing.T, msg *pubsub.Message) {
				t.Helper()

				assert.Nil(t, msg, "msg must be nil when marshal errors")
			},
		},
		{
			name:    "empty_channel_and_type_accepted",
			channel: "",
			msgType: "",
			payload: nil,
			check: func(t *testing.T, msg *pubsub.Message) {
				t.Helper()

				require.NotNil(t, msg)
				assert.Empty(t, msg.Channel)
				assert.Empty(t, msg.Type)
				assert.True(t, xidPattern.MatchString(msg.ID), "ID still generated regardless")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			before := time.Now()

			// ACT
			msg, err := NewMessage(tt.channel, tt.msgType, tt.payload)

			// ASSERT
			if tt.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
			} else {
				require.NoError(t, err)
				assert.False(t, msg.Timestamp.Before(before.Add(-time.Second)),
					"timestamp must be at-or-after the moment NewMessage was called")
			}

			tt.check(t, msg)
		})
	}
}

func TestNewMessage_GeneratesUniqueIDs(t *testing.T) {
	t.Parallel()

	// ARRANGE
	const iterations = 100
	seen := make(map[string]struct{}, iterations)

	// ACT + ASSERT
	for i := range iterations {
		msg, err := NewMessage("c", "t", nil)
		require.NoError(t, err)
		require.NotNil(t, msg)

		_, dup := seen[msg.ID]
		require.False(t, dup, "duplicate ID %q at iteration %d", msg.ID, i)

		seen[msg.ID] = struct{}{}
	}

	require.Len(t, seen, iterations)
}

func TestParsePayload_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		payload   []byte
		parse     func(*pubsub.Message) error
		wantError string
	}{
		{
			name:    "parse_empty_payload_returns_error",
			payload: []byte(""),
			parse: func(m *pubsub.Message) error {
				_, err := ParsePayload[CacheInvalidatePayload](m)

				return err
			},
			wantError: "unexpected end of JSON input",
		},
		{
			name:    "parse_invalid_json_returns_error",
			payload: []byte("{not valid json"),
			parse: func(m *pubsub.Message) error {
				_, err := ParsePayload[CacheInvalidatePayload](m)

				return err
			},
			wantError: "invalid character",
		},
		{
			name:    "parse_into_wrong_type_returns_error",
			payload: mustMarshal(t, TaskStatusPayload{TaskID: 1, Status: "ok"}),
			parse: func(m *pubsub.Message) error {
				_, err := ParsePayload[int](m)

				return err
			},
			wantError: "cannot unmarshal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			msg := &pubsub.Message{Payload: tt.payload}

			// ACT
			err := tt.parse(msg)

			// ASSERT
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError, "error message mismatch")
		})
	}
}

func TestParsePayload_NullReturnsZeroValue(t *testing.T) {
	t.Parallel()

	// ARRANGE
	msg := &pubsub.Message{Payload: []byte("null")}

	// ACT
	got, err := ParsePayload[CacheInvalidatePayload](msg)

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, CacheInvalidatePayload{}, got, "null must yield the type's zero value")
}

func TestParsePayload_IgnoresUnknownFields(t *testing.T) {
	t.Parallel()

	// ARRANGE
	msg := &pubsub.Message{
		Payload: []byte(`{"entity_type":"games","extra_unknown":"value"}`),
	}

	// ACT
	got, err := ParsePayload[CacheInvalidatePayload](msg)

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, "games", got.EntityType)
	assert.Nil(t, got.EntityIDs)
}

func TestParsePayload_RoundTrips(t *testing.T) {
	t.Parallel()

	t.Run("roundtrip_cache_invalidate", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		want := CacheInvalidatePayload{
			EntityType: "users",
			EntityIDs:  []string{"a", "b", "c"},
			Pattern:    "users:*",
		}

		// ACT
		got := roundTrip[CacheInvalidatePayload](t, want)

		// ASSERT
		assert.Equal(t, want, got)
	})

	t.Run("roundtrip_daemon_session_with_time", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		want := DaemonSessionPayload{
			NodeID:      7,
			InstanceID:  "inst-1",
			Version:     "1.0",
			ConnectedAt: time.Now().UTC().Truncate(time.Second),
		}

		// ACT
		got := roundTrip[DaemonSessionPayload](t, want)

		// ASSERT
		assert.Equal(t, want.NodeID, got.NodeID)
		assert.Equal(t, want.InstanceID, got.InstanceID)
		assert.Equal(t, want.Version, got.Version)
		assert.True(t, want.ConnectedAt.Equal(got.ConnectedAt),
			"timestamp round-trip mismatch: want %s got %s", want.ConnectedAt, got.ConnectedAt)
	})

	t.Run("roundtrip_plugin_event_with_pointers_and_map", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		serverID := uint(11)
		taskID := uint(22)
		nodeID := uint(33)

		want := PluginEventPayload{
			EventType: 9,
			ServerID:  &serverID,
			TaskID:    &taskID,
			NodeID:    &nodeID,
			ExtraData: map[string]string{"hello": "world", "lang": "go"},
		}

		// ACT
		got := roundTrip[PluginEventPayload](t, want)

		// ASSERT
		assert.Equal(t, want.EventType, got.EventType)
		require.NotNil(t, got.ServerID)
		require.NotNil(t, got.TaskID)
		require.NotNil(t, got.NodeID)
		assert.Equal(t, serverID, *got.ServerID)
		assert.Equal(t, taskID, *got.TaskID)
		assert.Equal(t, nodeID, *got.NodeID)
		assert.Equal(t, want.ExtraData, got.ExtraData)
	})

	t.Run("roundtrip_byte_slice", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		want := DaemonTaskDispatchPayload{
			NodeID:    1,
			RequestID: "req-1",
			TaskID:    42,
			TaskData:  []byte("hello world"),
		}

		// ACT
		got := roundTrip[DaemonTaskDispatchPayload](t, want)

		// ASSERT
		assert.Equal(t, want.NodeID, got.NodeID)
		assert.Equal(t, want.RequestID, got.RequestID)
		assert.Equal(t, want.TaskID, got.TaskID)
		assert.Equal(t, want.TaskData, got.TaskData, "byte slice must round-trip identically")
	})

	t.Run("roundtrip_metrics_subscribers", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		want := MetricsSubscribersPayload{
			InstanceID: "inst-2",
			NodeID:     5,
			Count:      12,
			Timestamp:  time.Date(2024, 5, 1, 10, 30, 0, 0, time.UTC),
		}

		// ACT
		got := roundTrip[MetricsSubscribersPayload](t, want)

		// ASSERT
		assert.Equal(t, want.InstanceID, got.InstanceID)
		assert.Equal(t, want.NodeID, got.NodeID)
		assert.Equal(t, want.Count, got.Count)
		assert.True(t, want.Timestamp.Equal(got.Timestamp),
			"timestamp round-trip mismatch: want %s got %s", want.Timestamp, got.Timestamp)
	})
}

func TestTypeConstants(t *testing.T) {
	t.Parallel()

	all := map[string]string{
		"TypeCacheInvalidate":             TypeCacheInvalidate,
		"TypePluginEvent":                 TypePluginEvent,
		"TypeServerStatus":                TypeServerStatus,
		"TypeTaskProgress":                TypeTaskProgress,
		"TypeNotification":                TypeNotification,
		"TypeDaemonConnected":             TypeDaemonConnected,
		"TypeDaemonClosed":                TypeDaemonClosed,
		"TypeDaemonTask":                  TypeDaemonTask,
		"TypeDaemonCommand":               TypeDaemonCommand,
		"TypeDaemonServerConfig":          TypeDaemonServerConfig,
		"TypeTaskStatus":                  TypeTaskStatus,
		"TypeTaskOutput":                  TypeTaskOutput,
		"TypeTaskComplete":                TypeTaskComplete,
		"TypeConsoleOutput":               TypeConsoleOutput,
		"TypeConsoleResult":               TypeConsoleResult,
		"TypeDaemonFileRequest":           TypeDaemonFileRequest,
		"TypeDaemonFileResponse":          TypeDaemonFileResponse,
		"TypeDaemonFileTransferComplete":  TypeDaemonFileTransferComplete,
		"TypeDaemonCommandRequest":        TypeDaemonCommandRequest,
		"TypeDaemonCommandResponse":       TypeDaemonCommandResponse,
		"TypeDaemonStatusRequest":         TypeDaemonStatusRequest,
		"TypeDaemonStatusResponse":        TypeDaemonStatusResponse,
		"TypeDaemonConsoleLogRequest":     TypeDaemonConsoleLogRequest,
		"TypeDaemonConsoleLogResponse":    TypeDaemonConsoleLogResponse,
		"TypeAttachStarted":               TypeAttachStarted,
		"TypeAttachOutput":                TypeAttachOutput,
		"TypeAttachClosed":                TypeAttachClosed,
		"TypeDaemonAttach":                TypeDaemonAttach,
		"TypeDaemonHTTPProxyRequest":      TypeDaemonHTTPProxyRequest,
		"TypeDaemonHTTPProxyResponse":     TypeDaemonHTTPProxyResponse,
		"TypeMetricsLive":                 TypeMetricsLive,
		"TypeDaemonMetricsRequest":        TypeDaemonMetricsRequest,
		"TypeDaemonMetricsResponse":       TypeDaemonMetricsResponse,
		"TypeMetricsSubscribers":          TypeMetricsSubscribers,
		"TypeDaemonServerTaskDelta":       TypeDaemonServerTaskDelta,
		"TypeDaemonServerTaskResync":      TypeDaemonServerTaskResync,
		"TypeServerTaskExecutionStarted":  TypeServerTaskExecutionStarted,
		"TypeServerTaskExecutionFinished": TypeServerTaskExecutionFinished,
		"TypeServerTaskExecutionLog":      TypeServerTaskExecutionLog,
	}

	t.Run("all_constants_are_non_empty", func(t *testing.T) {
		t.Parallel()

		for name, value := range all {
			assert.NotEmpty(t, value, "constant %s must not be empty", name)
		}
	})

	t.Run("all_constants_match_subsystem_subtype_shape", func(t *testing.T) {
		t.Parallel()

		for name, value := range all {
			assert.Regexp(t, typePattern, value,
				"constant %s = %q must match shape `^[a-z][a-z_]*(\\.[a-z_]+)*$`", name, value)
		}
	})

	t.Run("all_constants_are_distinct", func(t *testing.T) {
		t.Parallel()

		seen := make(map[string]string, len(all))
		for name, value := range all {
			if other, dup := seen[value]; dup {
				t.Errorf("constants %s and %s share value %q", name, other, value)
			}

			seen[value] = name
		}
	})

	t.Run("known_values_are_stable", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "cache.invalidate", TypeCacheInvalidate)
		assert.Equal(t, "daemon.connected", TypeDaemonConnected)
		assert.Equal(t, "daemon.file.transfer.complete", TypeDaemonFileTransferComplete)
		assert.Equal(t, "metrics.subscribers", TypeMetricsSubscribers)
		assert.Equal(t, "daemon.server_task.delta", TypeDaemonServerTaskDelta)
		assert.Equal(t, "daemon.server_task.resync", TypeDaemonServerTaskResync)
		assert.Equal(t, "server_task.execution.started", TypeServerTaskExecutionStarted)
		assert.Equal(t, "server_task.execution.finished", TypeServerTaskExecutionFinished)
		assert.Equal(t, "server_task.execution.log", TypeServerTaskExecutionLog)
	})
}

func TestPayloadStructs_AreJSONRoundTrippable(t *testing.T) {
	t.Parallel()

	cases := []any{
		ServerStatusPayload{ServerID: 1, Status: "online", PlayersOnline: 4, MaxPlayers: 32},
		TaskProgressPayload{TaskID: 1, Status: "running", Progress: 50, Message: "halfway"},
		DaemonCommandDispatchPayload{NodeID: 1, RequestID: "r", CommandID: "c", ServerID: 2, Command: "ls", Timeout: 30},
		TaskStatusPayload{TaskID: 1, Status: "ok", ServerID: 2, Message: "msg"},
		TaskOutputPayload{TaskID: 1, Chunk: "out", IsFinal: true},
		TaskCompletePayload{TaskID: 1, Status: "done", ServerID: 2},
		ConsoleOutputPayload{ServerID: 1, CommandID: "c", Chunk: "out"},
		ConsoleResultPayload{ServerID: 1, CommandID: "c", ExitCode: 0, Error: ""},
		DaemonFileRequestPayload{NodeID: 1, RequestID: "r", InstanceID: "i", Operation: "read", Data: []byte{0x01}, TransferID: "t", StoragePath: "/a"},
		DaemonFileResponsePayload{RequestID: "r", Error: "", Data: []byte{0x02}, StoragePath: "/b"},
		FileTransferCompletePayload{TransferID: "t", Success: true, Error: "", Checksum: "abc"},
		DaemonCommandRequestPayload{NodeID: 1, RequestID: "r", InstanceID: "i", Data: []byte{0x03}},
		DaemonCommandResponsePayload{RequestID: "r", Error: "", Data: []byte{0x04}},
		DaemonStatusRequestPayload{NodeID: 1, RequestID: "r", InstanceID: "i"},
		DaemonStatusResponsePayload{RequestID: "r", Error: "", Data: []byte{0x05}},
		AttachStartedPayload{SessionID: "s", ServerID: 1},
		AttachOutputPayload{SessionID: "s", Data: []byte("hi")},
		AttachClosedPayload{SessionID: "s", Reason: "eof", ExitCode: 0},
		DaemonAttachDispatchPayload{NodeID: 1, RequestID: "r", Data: []byte{0x06}},
		DaemonConsoleLogRequestPayload{NodeID: 1, RequestID: "r", InstanceID: "i", ServerID: 2, MaxBytes: 1024},
		DaemonConsoleLogResponsePayload{RequestID: "r", Error: "", Data: []byte{0x07}},
		DaemonServerConfigPayload{NodeID: 1, RequestID: "r", ConfigData: []byte{0x08}},
		DaemonHTTPProxyRequestPayload{NodeID: 1, RequestID: "r", InstanceID: "i", Data: []byte{0x09}},
		DaemonHTTPProxyResponsePayload{RequestID: "r", Error: "", Data: []byte{0x0A}, StoragePath: "/c"},
		MetricsLivePayload{NodeID: 1, Data: []byte{0x0B}},
		DaemonMetricsRequestPayload{NodeID: 1, RequestID: "r", InstanceID: "i", Data: []byte{0x0C}},
		DaemonMetricsResponsePayload{RequestID: "r", NodeID: 1, Error: "", Data: []byte{0x0D}},
		DaemonServerTaskDeltaPayload{NodeID: 1, RequestID: "r", TaskID: 42, Data: []byte{0x0E}},
		DaemonServerTaskResyncPayload{NodeID: 1, LastKnownVersion: 99},
		ServerTaskExecutionStatusPayload{
			ExecutionID:  "exec-1",
			TaskID:       42,
			ServerID:     7,
			NodeID:       3,
			Status:       "finished",
			ExitCode:     new(int32(137)),
			ErrorMessage: "killed",
		},
		ServerTaskExecutionLogPayload{ExecutionID: "exec-1", TaskID: 42, Sequence: 5, Chunk: []byte("hello"), IsFinal: true},
	}

	for _, want := range cases {
		t.Run(reflect.TypeOf(want).Name(), func(t *testing.T) {
			t.Parallel()

			// ARRANGE
			data, err := json.Marshal(want)
			require.NoError(t, err)

			// ACT
			got := reflect.New(reflect.TypeOf(want)).Interface()
			require.NoError(t, json.Unmarshal(data, got))

			// ASSERT
			assert.Equal(t, want, reflect.ValueOf(got).Elem().Interface(),
				"round-tripping %T must preserve all fields", want)
		})
	}
}

func TestServerTaskExecutionStatusPayload_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want ServerTaskExecutionStatusPayload
	}{
		{
			name: "finished_with_exit_code_and_error",
			want: ServerTaskExecutionStatusPayload{
				ExecutionID:  "exec-abc",
				TaskID:       42,
				ServerID:     7,
				NodeID:       3,
				Status:       "failed",
				ExitCode:     new(int32(137)),
				ErrorMessage: "killed by signal",
			},
		},
		{
			name: "started_without_exit_code_or_error",
			want: ServerTaskExecutionStatusPayload{
				ExecutionID: "exec-xyz",
				TaskID:      1,
				ServerID:    2,
				NodeID:      3,
				Status:      "started",
			},
		},
		{
			name: "exit_code_zero_pointer_must_survive_round_trip",
			want: ServerTaskExecutionStatusPayload{
				ExecutionID: "exec-zero",
				TaskID:      1,
				ServerID:    2,
				NodeID:      3,
				Status:      "finished",
				ExitCode:    new(int32(0)),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE + ACT
			got := roundTrip[ServerTaskExecutionStatusPayload](t, tt.want)

			// ASSERT
			assert.Equal(t, tt.want.ExecutionID, got.ExecutionID, "ExecutionID must survive round-trip")
			assert.Equal(t, tt.want.TaskID, got.TaskID, "TaskID must survive round-trip")
			assert.Equal(t, tt.want.ServerID, got.ServerID, "ServerID must survive round-trip")
			assert.Equal(t, tt.want.NodeID, got.NodeID, "NodeID must survive round-trip")
			assert.Equal(t, tt.want.Status, got.Status, "Status must survive round-trip")
			assert.Equal(t, tt.want.ErrorMessage, got.ErrorMessage, "ErrorMessage must survive round-trip")

			if tt.want.ExitCode == nil {
				assert.Nil(t, got.ExitCode, "nil ExitCode must round-trip as nil")
			} else {
				require.NotNil(t, got.ExitCode, "non-nil ExitCode must round-trip as non-nil")
				assert.Equal(t, *tt.want.ExitCode, *got.ExitCode, "ExitCode value must round-trip")
			}
		})
	}
}

func TestServerTaskExecutionStatusPayload_OmitsNilExitCodeAndErrorMessage(t *testing.T) {
	t.Parallel()

	// ARRANGE
	payload := ServerTaskExecutionStatusPayload{
		ExecutionID: "exec-1",
		TaskID:      42,
		ServerID:    7,
		NodeID:      3,
		Status:      "started",
	}

	// ACT
	data, err := json.Marshal(payload)
	require.NoError(t, err)

	// ASSERT
	body := string(data)
	assert.NotContains(t, body, `"exit_code"`, "nil *int32 ExitCode must be omitted via omitempty")
	assert.NotContains(t, body, `"error_message"`, "empty ErrorMessage must be omitted via omitempty")
	assert.Contains(t, body, `"execution_id":"exec-1"`, "ExecutionID must be present")
	assert.Contains(t, body, `"status":"started"`, "Status must be present")
}

func TestServerTaskExecutionStatusPayload_EncodesExitCodeZero(t *testing.T) {
	t.Parallel()

	// ARRANGE
	payload := ServerTaskExecutionStatusPayload{
		ExecutionID: "exec-1",
		TaskID:      1,
		ServerID:    1,
		NodeID:      1,
		Status:      "finished",
		ExitCode:    new(int32(0)),
	}

	// ACT
	data, err := json.Marshal(payload)
	require.NoError(t, err)

	// ASSERT
	assert.Contains(t, string(data), `"exit_code":0`,
		"non-nil pointer to zero must still be encoded (this is why ExitCode is a pointer)")
}

func TestServerTaskExecutionLogPayload_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want ServerTaskExecutionLogPayload
	}{
		{
			name: "binary_chunk_with_non_final_flag",
			want: ServerTaskExecutionLogPayload{
				ExecutionID: "exec-1",
				TaskID:      42,
				Sequence:    5,
				Chunk:       []byte{0x00, 0x01, 0xff, 'h', 'i'},
				IsFinal:     true,
			},
		},
		{
			name: "empty_chunk",
			want: ServerTaskExecutionLogPayload{
				ExecutionID: "exec-1",
				TaskID:      1,
				Sequence:    0,
				Chunk:       []byte{},
				IsFinal:     false,
			},
		},
		{
			name: "utf8_text_chunk",
			want: ServerTaskExecutionLogPayload{
				ExecutionID: "exec-2",
				TaskID:      2,
				Sequence:    100,
				Chunk:       []byte("hello, ä-world"),
				IsFinal:     false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ARRANGE + ACT
			got := roundTrip[ServerTaskExecutionLogPayload](t, tt.want)

			// ASSERT
			assert.Equal(t, tt.want.ExecutionID, got.ExecutionID, "ExecutionID must survive round-trip")
			assert.Equal(t, tt.want.TaskID, got.TaskID, "TaskID must survive round-trip")
			assert.Equal(t, tt.want.Sequence, got.Sequence, "Sequence must survive round-trip")
			assert.Equal(t, tt.want.IsFinal, got.IsFinal, "IsFinal must survive round-trip")
			assert.Equal(t, tt.want.Chunk, got.Chunk,
				"Chunk bytes must round-trip identically (encoding/json base64)")
		})
	}
}

func TestServerTaskExecutionLogPayload_ChunkEncodedAsBase64(t *testing.T) {
	t.Parallel()

	// ARRANGE
	payload := ServerTaskExecutionLogPayload{
		ExecutionID: "exec-1",
		TaskID:      1,
		Sequence:    1,
		Chunk:       []byte{0x01, 0x02, 0x03},
		IsFinal:     true,
	}

	// ACT
	data, err := json.Marshal(payload)
	require.NoError(t, err)

	// ASSERT
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))

	expected := base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03})
	assert.Equal(t, expected, raw["chunk"],
		"[]byte must be base64-encoded by encoding/json")
}

func TestDaemonServerTaskDeltaPayload_RoundTrip(t *testing.T) {
	t.Parallel()

	// ARRANGE
	want := DaemonServerTaskDeltaPayload{
		NodeID:    7,
		RequestID: "req-1",
		TaskID:    42,
		Data:      []byte{0x10, 0x20, 0x30, 0xff},
	}

	// ACT
	got := roundTrip[DaemonServerTaskDeltaPayload](t, want)

	// ASSERT
	assert.Equal(t, want.NodeID, got.NodeID, "NodeID must survive round-trip")
	assert.Equal(t, want.RequestID, got.RequestID, "RequestID must survive round-trip")
	assert.Equal(t, want.TaskID, got.TaskID, "TaskID must survive round-trip")
	assert.Equal(t, want.Data, got.Data, "opaque Data bytes must round-trip identically")
}

func TestDaemonServerTaskResyncPayload_RoundTrip(t *testing.T) {
	t.Parallel()

	// ARRANGE
	want := DaemonServerTaskResyncPayload{
		NodeID:           7,
		LastKnownVersion: 12345,
	}

	// ACT
	got := roundTrip[DaemonServerTaskResyncPayload](t, want)

	// ASSERT
	assert.Equal(t, want, got, "all fields must survive round-trip")
}

func TestNewMessage_BuildsServerTaskExecutionStatus(t *testing.T) {
	t.Parallel()

	// ARRANGE
	channel := "gameap:realtime:server_task:execution:42"
	payload := ServerTaskExecutionStatusPayload{
		ExecutionID: "exec-1",
		TaskID:      42,
		ServerID:    7,
		NodeID:      3,
		Status:      "started",
	}

	// ACT
	msg, err := NewMessage(channel, TypeServerTaskExecutionStarted, payload)

	// ASSERT
	require.NoError(t, err)
	require.NotNil(t, msg)
	assert.Equal(t, channel, msg.Channel, "channel must be preserved on the Message")
	assert.Equal(t, TypeServerTaskExecutionStarted, msg.Type, "type must be preserved on the Message")
	assert.True(t, xidPattern.MatchString(msg.ID), "ID %q must match XID format", msg.ID)
	assert.WithinDuration(t, time.Now(), msg.Timestamp, time.Second, "timestamp must be set to ~now")

	parsed, err := ParsePayload[ServerTaskExecutionStatusPayload](msg)
	require.NoError(t, err)
	assert.Equal(t, payload, parsed, "round-tripping the payload via the Message envelope must preserve all fields")
}

func roundTrip[T any](t *testing.T, in T) T {
	t.Helper()

	msg, err := NewMessage("ch", "type", in)
	require.NoError(t, err)
	require.NotNil(t, msg)

	out, err := ParsePayload[T](msg)
	require.NoError(t, err)

	return out
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()

	data, err := json.Marshal(v)
	require.NoError(t, err)

	return data
}
