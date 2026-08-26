package channels_test

import (
	"math"
	"strings"
	"testing"

	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/stretchr/testify/assert"
)

func TestUint64ChannelBuilders(t *testing.T) {
	t.Parallel()

	const happyID uint64 = 123

	tests := []struct {
		name      string
		builder   func(uint64) string
		inputID   uint64
		wantExact string
	}{
		{
			name:      "daemon_task_dispatch_happy",
			builder:   channels.BuildDaemonTaskDispatchChannel,
			inputID:   happyID,
			wantExact: "gameap:daemon:task:dispatch:123",
		},
		{
			name:      "daemon_command_dispatch_happy",
			builder:   channels.BuildDaemonCommandDispatchChannel,
			inputID:   happyID,
			wantExact: "gameap:daemon:command:dispatch:123",
		},
		{
			name:      "realtime_task_status_happy",
			builder:   channels.BuildRealtimeTaskStatusChannel,
			inputID:   happyID,
			wantExact: "gameap:realtime:task:status:123",
		},
		{
			name:      "realtime_task_output_happy",
			builder:   channels.BuildRealtimeTaskOutputChannel,
			inputID:   happyID,
			wantExact: "gameap:realtime:task:output:123",
		},
		{
			name:      "realtime_console_output_happy",
			builder:   channels.BuildRealtimeConsoleOutputChannel,
			inputID:   happyID,
			wantExact: "gameap:realtime:console:output:123",
		},
		{
			name:      "realtime_console_result_happy",
			builder:   channels.BuildRealtimeConsoleResultChannel,
			inputID:   happyID,
			wantExact: "gameap:realtime:console:result:123",
		},
		{
			name:      "daemon_file_request_happy",
			builder:   channels.BuildDaemonFileRequestChannel,
			inputID:   happyID,
			wantExact: "gameap:daemon:file:request:123",
		},
		{
			name:      "daemon_command_request_happy",
			builder:   channels.BuildDaemonCommandRequestChannel,
			inputID:   happyID,
			wantExact: "gameap:daemon:command:request:123",
		},
		{
			name:      "daemon_status_request_happy",
			builder:   channels.BuildDaemonStatusRequestChannel,
			inputID:   happyID,
			wantExact: "gameap:daemon:status:request:123",
		},
		{
			name:      "daemon_attach_dispatch_happy",
			builder:   channels.BuildDaemonAttachDispatchChannel,
			inputID:   happyID,
			wantExact: "gameap:daemon:attach:dispatch:123",
		},
		{
			name:      "daemon_console_log_request_happy",
			builder:   channels.BuildDaemonConsoleLogRequestChannel,
			inputID:   happyID,
			wantExact: "gameap:daemon:consolelog:request:123",
		},
		{
			name:      "daemon_http_proxy_request_happy",
			builder:   channels.BuildDaemonHTTPProxyRequestChannel,
			inputID:   happyID,
			wantExact: "gameap:daemon:httpproxy:request:123",
		},
		{
			name:      "realtime_metrics_happy",
			builder:   channels.BuildRealtimeMetricsChannel,
			inputID:   happyID,
			wantExact: "gameap:realtime:metrics:123",
		},
		{
			name:      "daemon_metrics_request_happy",
			builder:   channels.BuildDaemonMetricsRequestChannel,
			inputID:   happyID,
			wantExact: "gameap:daemon:metrics:request:123",
		},
		{
			name:      "metrics_subscribers_happy",
			builder:   channels.BuildMetricsSubscribersChannel,
			inputID:   happyID,
			wantExact: "gameap:metrics:subscribers:123",
		},
		{
			name:      "daemon_server_task_delta_happy",
			builder:   channels.BuildDaemonServerTaskDeltaChannel,
			inputID:   happyID,
			wantExact: "gameap:daemon:server_task:delta:123",
		},
		{
			name:      "daemon_server_task_resync_happy",
			builder:   channels.BuildDaemonServerTaskResyncChannel,
			inputID:   happyID,
			wantExact: "gameap:daemon:server_task:resync:123",
		},
		{
			name:      "realtime_server_task_execution_happy",
			builder:   channels.BuildRealtimeServerTaskExecutionChannel,
			inputID:   happyID,
			wantExact: "gameap:realtime:server_task:execution:123",
		},
		{
			name:      "daemon_task_dispatch_zero_id",
			builder:   channels.BuildDaemonTaskDispatchChannel,
			inputID:   0,
			wantExact: "gameap:daemon:task:dispatch:0",
		},
		{
			name:      "daemon_task_dispatch_max_uint64",
			builder:   channels.BuildDaemonTaskDispatchChannel,
			inputID:   math.MaxUint64,
			wantExact: "gameap:daemon:task:dispatch:18446744073709551615",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.builder(tt.inputID)

			assert.Equal(t, tt.wantExact, got, "channel string mismatch")
		})
	}
}

func TestStringChannelBuilders(t *testing.T) {
	t.Parallel()

	const happyInput = "abc-123"

	tests := []struct {
		name      string
		builder   func(string) string
		input     string
		wantExact string
	}{
		{
			name:      "daemon_file_response_happy",
			builder:   channels.BuildDaemonFileResponseChannel,
			input:     happyInput,
			wantExact: "gameap:daemon:file:response:abc-123",
		},
		{
			name:      "daemon_file_transfer_complete_happy",
			builder:   channels.BuildDaemonFileTransferCompleteChannel,
			input:     happyInput,
			wantExact: "gameap:daemon:file:transfer:complete:abc-123",
		},
		{
			name:      "daemon_command_response_happy",
			builder:   channels.BuildDaemonCommandResponseChannel,
			input:     happyInput,
			wantExact: "gameap:daemon:command:response:abc-123",
		},
		{
			name:      "daemon_status_response_happy",
			builder:   channels.BuildDaemonStatusResponseChannel,
			input:     happyInput,
			wantExact: "gameap:daemon:status:response:abc-123",
		},
		{
			name:      "realtime_attach_started_happy",
			builder:   channels.BuildRealtimeAttachStartedChannel,
			input:     happyInput,
			wantExact: "gameap:realtime:attach:started:abc-123",
		},
		{
			name:      "realtime_attach_output_happy",
			builder:   channels.BuildRealtimeAttachOutputChannel,
			input:     happyInput,
			wantExact: "gameap:realtime:attach:output:abc-123",
		},
		{
			name:      "realtime_attach_closed_happy",
			builder:   channels.BuildRealtimeAttachClosedChannel,
			input:     happyInput,
			wantExact: "gameap:realtime:attach:closed:abc-123",
		},
		{
			name:      "daemon_console_log_response_happy",
			builder:   channels.BuildDaemonConsoleLogResponseChannel,
			input:     happyInput,
			wantExact: "gameap:daemon:consolelog:response:abc-123",
		},
		{
			name:      "daemon_http_proxy_response_happy",
			builder:   channels.BuildDaemonHTTPProxyResponseChannel,
			input:     happyInput,
			wantExact: "gameap:daemon:httpproxy:response:abc-123",
		},
		{
			name:      "daemon_metrics_response_happy",
			builder:   channels.BuildDaemonMetricsResponseChannel,
			input:     happyInput,
			wantExact: "gameap:daemon:metrics:response:abc-123",
		},
		{
			name:      "realtime_server_task_log_happy",
			builder:   channels.BuildRealtimeServerTaskLogChannel,
			input:     happyInput,
			wantExact: "gameap:realtime:server_task:log:abc-123",
		},
		{
			name:      "daemon_file_response_empty_input",
			builder:   channels.BuildDaemonFileResponseChannel,
			input:     "",
			wantExact: "gameap:daemon:file:response:",
		},
		{
			name:      "daemon_file_response_special_chars",
			builder:   channels.BuildDaemonFileResponseChannel,
			input:     "foo:bar*baz",
			wantExact: "gameap:daemon:file:response:foo:bar*baz",
		},
		{
			name:      "daemon_file_response_unicode",
			builder:   channels.BuildDaemonFileResponseChannel,
			input:     "node-äöü",
			wantExact: "gameap:daemon:file:response:node-äöü",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.builder(tt.input)

			assert.Equal(t, tt.wantExact, got, "channel string mismatch")
		})
	}
}

func TestBuildCacheInvalidateChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		entityType string
		entityID   string
		wantExact  string
	}{
		{
			name:       "both_populated",
			entityType: "games",
			entityID:   "5",
			wantExact:  "gameap:cache:invalidate:games:5",
		},
		{
			name:       "empty_entity_id_omits_trailing_colon",
			entityType: "games",
			entityID:   "",
			wantExact:  "gameap:cache:invalidate:games",
		},
		{
			name:       "empty_entity_type_keeps_colon_separator",
			entityType: "",
			entityID:   "5",
			wantExact:  "gameap:cache:invalidate::5",
		},
		{
			name:       "both_empty",
			entityType: "",
			entityID:   "",
			wantExact:  "gameap:cache:invalidate:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := channels.BuildCacheInvalidateChannel(tt.entityType, tt.entityID)

			assert.Equal(t, tt.wantExact, got, "cache invalidate channel string mismatch")
		})
	}
}

func TestBuildPluginEventChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantExact string
	}{
		{
			name:      "happy_path",
			input:     "server.created",
			wantExact: "gameap:plugin:events:server.created",
		},
		{
			name:      "empty_input",
			input:     "",
			wantExact: "gameap:plugin:events:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := channels.BuildPluginEventChannel(tt.input)

			assert.Equal(t, tt.wantExact, got, "plugin event channel string mismatch")
		})
	}
}

func TestChannelConstants(t *testing.T) {
	t.Parallel()

	t.Run("root_prefix_is_gameap_colon", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "gameap:", channels.Prefix)
	})

	t.Run("group_prefixes_extend_root_prefix", func(t *testing.T) {
		t.Parallel()

		assert.True(
			t,
			strings.HasPrefix(channels.CachePrefix, channels.Prefix),
			"CachePrefix must start with root Prefix",
		)
		assert.True(
			t,
			strings.HasPrefix(channels.PluginPrefix, channels.Prefix),
			"PluginPrefix must start with root Prefix",
		)
		assert.True(
			t,
			strings.HasPrefix(channels.RealtimePrefix, channels.Prefix),
			"RealtimePrefix must start with root Prefix",
		)
		assert.True(
			t,
			strings.HasPrefix(channels.SystemPrefix, channels.Prefix),
			"SystemPrefix must start with root Prefix",
		)
		assert.True(
			t,
			strings.HasPrefix(channels.DaemonPrefix, channels.Prefix),
			"DaemonPrefix must start with root Prefix",
		)
	})

	t.Run("cache_invalidate_constants_descend_from_cache_prefix", func(t *testing.T) {
		t.Parallel()

		assert.True(t, strings.HasPrefix(channels.CacheInvalidate, channels.CachePrefix))
		assert.True(t, strings.HasPrefix(channels.CacheInvalidateGames, channels.CachePrefix))
		assert.True(t, strings.HasPrefix(channels.CacheInvalidateUsers, channels.CachePrefix))
		assert.True(t, strings.HasPrefix(channels.CacheInvalidateNodes, channels.CachePrefix))
		assert.True(t, strings.HasPrefix(channels.CacheInvalidateRBAC, channels.CachePrefix))
		assert.True(t, strings.HasPrefix(channels.CacheInvalidateAll, channels.CachePrefix))
	})

	t.Run("wildcard_constants_end_with_star", func(t *testing.T) {
		t.Parallel()

		assert.True(t, strings.HasSuffix(channels.CacheInvalidateAll, ":*"))
		assert.True(t, strings.HasSuffix(channels.RealtimeTaskAll, ":*"))
		assert.True(t, strings.HasSuffix(channels.RealtimeConsoleAll, ":*"))
		assert.True(t, strings.HasSuffix(channels.DaemonSessionAll, ":*"))
		assert.True(t, strings.HasSuffix(channels.DaemonTaskDispatchAll, ":*"))
		assert.True(t, strings.HasSuffix(channels.DaemonFileRequestAll, ":*"))
		assert.True(t, strings.HasSuffix(channels.DaemonCommandRequestAll, ":*"))
		assert.True(t, strings.HasSuffix(channels.RealtimeAttachAll, ":*"))
		assert.True(t, strings.HasSuffix(channels.DaemonAttachDispatchAll, ":*"))
		assert.True(t, strings.HasSuffix(channels.DaemonConsoleLogRequestAll, ":*"))
		assert.True(t, strings.HasSuffix(channels.DaemonStatusRequestAll, ":*"))
		assert.True(t, strings.HasSuffix(channels.DaemonHTTPProxyRequestAll, ":*"))
		assert.True(t, strings.HasSuffix(channels.RealtimeMetricsAll, ":*"))
		assert.True(t, strings.HasSuffix(channels.DaemonMetricsRequestAll, ":*"))
		assert.True(t, strings.HasSuffix(channels.MetricsSubscribersAll, ":*"))
	})

	t.Run("id_appended_constants_end_with_colon", func(t *testing.T) {
		t.Parallel()

		assert.True(t, strings.HasSuffix(channels.RealtimeTaskStatus, ":"))
		assert.True(t, strings.HasSuffix(channels.RealtimeTaskOutput, ":"))
		assert.True(t, strings.HasSuffix(channels.RealtimeConsoleOutput, ":"))
		assert.True(t, strings.HasSuffix(channels.RealtimeConsoleResult, ":"))
		assert.True(t, strings.HasSuffix(channels.DaemonTaskDispatch, ":"))
		assert.True(t, strings.HasSuffix(channels.DaemonCommandDispatch, ":"))
		assert.True(t, strings.HasSuffix(channels.DaemonFileRequest, ":"))
		assert.True(t, strings.HasSuffix(channels.DaemonFileResponse, ":"))
		assert.True(t, strings.HasSuffix(channels.DaemonFileTransferComplete, ":"))
		assert.True(t, strings.HasSuffix(channels.DaemonCommandRequest, ":"))
		assert.True(t, strings.HasSuffix(channels.DaemonCommandResponse, ":"))
		assert.True(t, strings.HasSuffix(channels.RealtimeAttachStarted, ":"))
		assert.True(t, strings.HasSuffix(channels.RealtimeAttachOutput, ":"))
		assert.True(t, strings.HasSuffix(channels.RealtimeAttachClosed, ":"))
		assert.True(t, strings.HasSuffix(channels.DaemonAttachDispatch, ":"))
		assert.True(t, strings.HasSuffix(channels.DaemonConsoleLogRequest, ":"))
		assert.True(t, strings.HasSuffix(channels.DaemonConsoleLogResponse, ":"))
		assert.True(t, strings.HasSuffix(channels.DaemonStatusRequest, ":"))
		assert.True(t, strings.HasSuffix(channels.DaemonStatusResponse, ":"))
		assert.True(t, strings.HasSuffix(channels.DaemonHTTPProxyRequest, ":"))
		assert.True(t, strings.HasSuffix(channels.DaemonHTTPProxyResponse, ":"))
		assert.True(t, strings.HasSuffix(channels.RealtimeMetrics, ":"))
		assert.True(t, strings.HasSuffix(channels.DaemonMetricsRequest, ":"))
		assert.True(t, strings.HasSuffix(channels.DaemonMetricsResponse, ":"))
		assert.True(t, strings.HasSuffix(channels.MetricsSubscribers, ":"))
	})

	t.Run("plugin_event_constants_descend_from_plugin_prefix", func(t *testing.T) {
		t.Parallel()

		assert.True(t, strings.HasPrefix(channels.PluginEvents, channels.PluginPrefix))
		assert.True(t, strings.HasPrefix(channels.PluginServerEvents, channels.PluginPrefix))
		assert.True(t, strings.HasPrefix(channels.PluginTaskEvents, channels.PluginPrefix))
	})

	t.Run("realtime_constants_descend_from_realtime_prefix", func(t *testing.T) {
		t.Parallel()

		assert.True(t, strings.HasPrefix(channels.RealtimeServerStatus, channels.RealtimePrefix))
		assert.True(t, strings.HasPrefix(channels.RealtimeTaskProgress, channels.RealtimePrefix))
		assert.True(t, strings.HasPrefix(channels.RealtimeNotifications, channels.RealtimePrefix))
		assert.True(t, strings.HasPrefix(channels.RealtimeMetrics, channels.RealtimePrefix))
	})

	t.Run("system_constants_descend_from_system_prefix", func(t *testing.T) {
		t.Parallel()

		assert.True(t, strings.HasPrefix(channels.SystemShutdown, channels.SystemPrefix))
		assert.True(t, strings.HasPrefix(channels.SystemConfigReload, channels.SystemPrefix))
	})

	t.Run("daemon_constants_descend_from_daemon_prefix", func(t *testing.T) {
		t.Parallel()

		assert.True(t, strings.HasPrefix(channels.DaemonSessionConnected, channels.DaemonPrefix))
		assert.True(t, strings.HasPrefix(channels.DaemonSessionClosed, channels.DaemonPrefix))
		assert.True(t, strings.HasPrefix(channels.DaemonTaskDispatch, channels.DaemonPrefix))
		assert.True(t, strings.HasPrefix(channels.DaemonMetricsRequest, channels.DaemonPrefix))
	})

	t.Run("metrics_subscribers_descends_from_root_prefix", func(t *testing.T) {
		t.Parallel()

		assert.True(t, strings.HasPrefix(channels.MetricsSubscribers, channels.Prefix))
		assert.True(t, strings.HasPrefix(channels.MetricsSubscribersAll, channels.Prefix))
	})

	t.Run("server_task_constants_descend_from_their_group_prefix", func(t *testing.T) {
		t.Parallel()

		assert.True(
			t,
			strings.HasPrefix(channels.DaemonServerTaskDelta, channels.DaemonPrefix),
			"DaemonServerTaskDelta must start with DaemonPrefix",
		)
		assert.True(
			t,
			strings.HasPrefix(channels.DaemonServerTaskDeltaAll, channels.DaemonPrefix),
			"DaemonServerTaskDeltaAll must start with DaemonPrefix",
		)
		assert.True(
			t,
			strings.HasPrefix(channels.DaemonServerTaskResync, channels.DaemonPrefix),
			"DaemonServerTaskResync must start with DaemonPrefix",
		)
		assert.True(
			t,
			strings.HasPrefix(channels.DaemonServerTaskResyncAll, channels.DaemonPrefix),
			"DaemonServerTaskResyncAll must start with DaemonPrefix",
		)
		assert.True(
			t,
			strings.HasPrefix(channels.RealtimeServerTaskExecution, channels.RealtimePrefix),
			"RealtimeServerTaskExecution must start with RealtimePrefix",
		)
		assert.True(
			t,
			strings.HasPrefix(channels.RealtimeServerTaskLog, channels.RealtimePrefix),
			"RealtimeServerTaskLog must start with RealtimePrefix",
		)
		assert.True(
			t,
			strings.HasPrefix(channels.RealtimeServerTaskAll, channels.RealtimePrefix),
			"RealtimeServerTaskAll must start with RealtimePrefix",
		)
	})

	t.Run("server_task_wildcard_constants_end_with_star", func(t *testing.T) {
		t.Parallel()

		assert.True(t, strings.HasSuffix(channels.DaemonServerTaskDeltaAll, ":*"))
		assert.True(t, strings.HasSuffix(channels.DaemonServerTaskResyncAll, ":*"))
		assert.True(t, strings.HasSuffix(channels.RealtimeServerTaskAll, ":*"))
	})

	t.Run("server_task_id_appended_constants_end_with_colon", func(t *testing.T) {
		t.Parallel()

		assert.True(t, strings.HasSuffix(channels.DaemonServerTaskDelta, ":"))
		assert.True(t, strings.HasSuffix(channels.DaemonServerTaskResync, ":"))
		assert.True(t, strings.HasSuffix(channels.RealtimeServerTaskExecution, ":"))
		assert.True(t, strings.HasSuffix(channels.RealtimeServerTaskLog, ":"))
	})
}

func TestBuildDaemonServerTaskDeltaChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		nodeID    uint64
		wantExact string
	}{
		{
			name:      "formats_with_node_id",
			nodeID:    42,
			wantExact: "gameap:daemon:server_task:delta:42",
		},
		{
			name:      "zero_node_id",
			nodeID:    0,
			wantExact: "gameap:daemon:server_task:delta:0",
		},
		{
			name:      "max_uint64_node_id",
			nodeID:    math.MaxUint64,
			wantExact: "gameap:daemon:server_task:delta:18446744073709551615",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := channels.BuildDaemonServerTaskDeltaChannel(tt.nodeID)

			assert.Equal(t, tt.wantExact, got, "delta channel string mismatch")
			assert.True(
				t,
				strings.HasPrefix(got, channels.DaemonServerTaskDelta),
				"result must start with DaemonServerTaskDelta prefix",
			)
		})
	}
}

func TestBuildDaemonServerTaskResyncChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		nodeID    uint64
		wantExact string
	}{
		{
			name:      "formats_with_node_id",
			nodeID:    42,
			wantExact: "gameap:daemon:server_task:resync:42",
		},
		{
			name:      "zero_node_id",
			nodeID:    0,
			wantExact: "gameap:daemon:server_task:resync:0",
		},
		{
			name:      "max_uint64_node_id",
			nodeID:    math.MaxUint64,
			wantExact: "gameap:daemon:server_task:resync:18446744073709551615",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := channels.BuildDaemonServerTaskResyncChannel(tt.nodeID)

			assert.Equal(t, tt.wantExact, got, "resync channel string mismatch")
			assert.True(
				t,
				strings.HasPrefix(got, channels.DaemonServerTaskResync),
				"result must start with DaemonServerTaskResync prefix",
			)
		})
	}
}

func TestBuildRealtimeServerTaskExecutionChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		taskID    uint64
		wantExact string
	}{
		{
			name:      "formats_with_task_id",
			taskID:    7,
			wantExact: "gameap:realtime:server_task:execution:7",
		},
		{
			name:      "zero_task_id",
			taskID:    0,
			wantExact: "gameap:realtime:server_task:execution:0",
		},
		{
			name:      "max_uint64_task_id",
			taskID:    math.MaxUint64,
			wantExact: "gameap:realtime:server_task:execution:18446744073709551615",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := channels.BuildRealtimeServerTaskExecutionChannel(tt.taskID)

			assert.Equal(t, tt.wantExact, got, "execution channel string mismatch")
			assert.True(
				t,
				strings.HasPrefix(got, channels.RealtimeServerTaskExecution),
				"result must start with RealtimeServerTaskExecution prefix",
			)
		})
	}
}

func TestBuildRealtimeServerTaskLogChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		executionID string
		wantExact   string
	}{
		{
			name:        "formats_with_xid_execution_id",
			executionID: "cqu1f5e0abcdef012345",
			wantExact:   "gameap:realtime:server_task:log:cqu1f5e0abcdef012345",
		},
		{
			name:        "empty_execution_id",
			executionID: "",
			wantExact:   "gameap:realtime:server_task:log:",
		},
		{
			name:        "execution_id_with_special_chars",
			executionID: "id:with:colons*and-star",
			wantExact:   "gameap:realtime:server_task:log:id:with:colons*and-star",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := channels.BuildRealtimeServerTaskLogChannel(tt.executionID)

			assert.Equal(t, tt.wantExact, got, "log channel string mismatch")
			assert.True(
				t,
				strings.HasPrefix(got, channels.RealtimeServerTaskLog),
				"result must start with RealtimeServerTaskLog prefix",
			)
		})
	}
}

func TestServerTaskWildcardConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		prefix string
		all    string
	}{
		{
			name:   "daemon_server_task_delta_wildcard_matches_prefix_plus_star",
			prefix: channels.DaemonServerTaskDelta,
			all:    channels.DaemonServerTaskDeltaAll,
		},
		{
			name:   "daemon_server_task_resync_wildcard_matches_prefix_plus_star",
			prefix: channels.DaemonServerTaskResync,
			all:    channels.DaemonServerTaskResyncAll,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.prefix+"*", tt.all,
				"wildcard constant must equal `prefix + \"*\"` to keep psubscribe consistent with builders")
		})
	}

	t.Run("realtime_server_task_all_covers_execution_and_log_prefixes", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "gameap:realtime:server_task:*", channels.RealtimeServerTaskAll,
			"RealtimeServerTaskAll wildcard must stay stable")
		assert.Equal(t, "gameap:realtime:server_task:execution:", channels.RealtimeServerTaskExecution,
			"RealtimeServerTaskExecution must share the server_task: stem with the wildcard")
		assert.Equal(t, "gameap:realtime:server_task:log:", channels.RealtimeServerTaskLog,
			"RealtimeServerTaskLog must share the server_task: stem with the wildcard")
	})
}
