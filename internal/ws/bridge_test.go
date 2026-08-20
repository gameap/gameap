package ws

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/channels"
	"github.com/gameap/gameap/internal/pubsub/memory"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBridge(t *testing.T) {
	t.Parallel()
	hub := NewHub(nil)
	ps := memory.New()
	defer func() { _ = ps.Close() }()

	bridge := NewBridge(hub, ps, nil)

	require.NotNil(t, bridge)
	assert.Same(t, hub, bridge.hub)
	assert.Same(t, ps, bridge.ps)
	assert.NotNil(t, bridge.logger, "nil logger must default to slog.Default")
}

func TestBridge_Start_SubscribesAllRealtimePatterns(t *testing.T) {
	t.Parallel()
	hub := NewHub(nil)
	ps := memory.New()
	t.Cleanup(func() { _ = ps.Close() })

	bridge := NewBridge(hub, ps, nil)

	require.NoError(t, bridge.Start(context.Background()))

	tests := []struct {
		name    string
		channel string
		topic   string
	}{
		{
			name:    "task_pattern",
			channel: channels.BuildRealtimeTaskStatusChannel(7),
			topic:   "realtime:task:status:7",
		},
		{
			name:    "console_pattern",
			channel: channels.BuildRealtimeConsoleOutputChannel(99),
			topic:   "realtime:console:output:99",
		},
		{
			name:    "attach_pattern",
			channel: channels.BuildRealtimeAttachOutputChannel("sess-1"),
			topic:   "realtime:attach:output:sess-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := newOfflineClient(t, hub)
			hub.Register(client, tt.topic)
			defer hub.Unregister(client)

			err := ps.Publish(context.Background(), tt.channel, &pubsub.Message{
				Channel:   tt.channel,
				Type:      "evt",
				Payload:   []byte(`{"ok":true}`),
				Timestamp: time.Now(),
			})
			require.NoError(t, err)

			require.Len(t, client.send, 1, "client subscribed to %s must receive the broadcast", tt.topic)
			<-client.send
		})
	}
}

func TestBridge_Start_ReturnsFirstSubscribeError(t *testing.T) {
	t.Parallel()
	hub := NewHub(nil)
	wantErr := errors.New("subscribe boom")
	ps := &fakeSubscriber{subscribeErr: wantErr}

	bridge := NewBridge(hub, ps, nil)

	err := bridge.Start(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
	assert.Equal(t, 1, ps.subscribeCalls, "Start must stop on the first subscribe error")
}

func TestBridge_HandleMessage_BroadcastsWrappedPayload(t *testing.T) {
	t.Parallel()
	hub := NewHub(nil)
	ps := memory.New()
	defer func() { _ = ps.Close() }()

	bridge := NewBridge(hub, ps, nil)
	require.NoError(t, bridge.Start(context.Background()))

	const taskID uint64 = 42
	channel := channels.BuildRealtimeTaskStatusChannel(taskID)
	topic := ChannelToTopic(channel)

	client := newOfflineClient(t, hub)
	hub.Register(client, topic)

	rawPayload := []byte(`{"task_id":42,"state":"running"}`)
	before := time.Now().Unix()
	err := ps.Publish(context.Background(), channel, &pubsub.Message{
		ID:        "msg-1",
		Channel:   channel,
		Type:      "task.status",
		Payload:   rawPayload,
		Timestamp: time.Now(),
	})
	require.NoError(t, err)
	after := time.Now().Unix()

	require.Len(t, client.send, 1)
	delivered := <-client.send

	var envelope OutboundMessage
	require.NoError(t, json.Unmarshal(delivered, &envelope))
	assert.Equal(t, "task.status", envelope.Type)
	assert.GreaterOrEqual(t, envelope.Timestamp, before)
	assert.LessOrEqual(t, envelope.Timestamp, after)

	rawJSON, ok := envelope.Payload.(map[string]any)
	require.True(t, ok, "payload should round-trip through JSON as a map")
	assert.Equal(t, float64(42), rawJSON["task_id"])
	assert.Equal(t, "running", rawJSON["state"])
}

func TestBridge_HandleMessage_SkipsEmptyTopic(t *testing.T) {
	t.Parallel()
	hub := NewHub(nil)
	ps := memory.New()
	defer func() { _ = ps.Close() }()

	bridge := NewBridge(hub, ps, nil)

	// Subscribe directly to the literal "gameap:" channel — ChannelToTopic
	// returns "" for this input, and handleMessage must skip the broadcast.
	require.NoError(t, ps.Subscribe(context.Background(), channels.Prefix, bridge.handleMessage))

	client := newOfflineClient(t, hub)
	hub.Register(client, "")

	err := ps.Publish(context.Background(), channels.Prefix, &pubsub.Message{
		Channel:   channels.Prefix,
		Type:      "evt",
		Payload:   []byte(`{}`),
		Timestamp: time.Now(),
	})
	require.NoError(t, err)

	assert.Empty(t, client.send, "empty topic must be skipped without broadcast")
}

func TestChannelToTopic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		channel string
		want    string
	}{
		{
			name:    "strips_gameap_prefix",
			channel: "gameap:realtime:task:status:42",
			want:    "realtime:task:status:42",
		},
		{
			name:    "keeps_unprefixed_channel_unchanged",
			channel: "no-prefix",
			want:    "no-prefix",
		},
		{
			name:    "prefix_only_returns_empty",
			channel: "gameap:",
			want:    "",
		},
		{
			name:    "empty_returns_empty",
			channel: "",
			want:    "",
		},
		{
			name:    "console_topic",
			channel: channels.BuildRealtimeConsoleOutputChannel(7),
			want:    "realtime:console:output:7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ChannelToTopic(tt.channel))
		})
	}
}

// fakeSubscriber is a pubsub.Subscriber that always returns the configured
// error from Subscribe and tracks the number of calls. Useful for negative
// tests of Bridge.Start.
type fakeSubscriber struct {
	subscribeErr   error
	subscribeCalls int
}

func (f *fakeSubscriber) Subscribe(_ context.Context, _ string, _ pubsub.Handler) error {
	f.subscribeCalls++

	return f.subscribeErr
}

func (f *fakeSubscriber) Unsubscribe(_ context.Context, _ string) error {
	return nil
}
