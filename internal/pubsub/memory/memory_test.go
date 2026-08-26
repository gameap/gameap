package memory

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/pubsub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemory_PublishSubscribe(t *testing.T) {
	t.Parallel()

	m := New()
	defer m.Close()

	var received atomic.Int32

	err := m.Subscribe(context.Background(), "test:channel", func(_ context.Context, msg *pubsub.Message) error {
		assert.Equal(t, "test:channel", msg.Channel)
		assert.Equal(t, "test.type", msg.Type)
		received.Add(1)

		return nil
	})
	require.NoError(t, err)

	msg := &pubsub.Message{
		ID:        "1",
		Channel:   "test:channel",
		Type:      "test.type",
		Payload:   []byte(`{"key":"value"}`),
		Timestamp: time.Now(),
	}

	err = m.Publish(context.Background(), "test:channel", msg)
	require.NoError(t, err)

	assert.Equal(t, int32(1), received.Load())
}

func TestMemory_PatternMatching(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		pattern     string
		channel     string
		shouldMatch bool
	}{
		{
			name:        "exact_match",
			pattern:     "test:channel",
			channel:     "test:channel",
			shouldMatch: true,
		},
		{
			name:        "wildcard_match",
			pattern:     "test:*",
			channel:     "test:channel",
			shouldMatch: true,
		},
		{
			name:        "wildcard_match_nested",
			pattern:     "cache:invalidate:*",
			channel:     "cache:invalidate:users",
			shouldMatch: true,
		},
		{
			name:        "no_match",
			pattern:     "other:channel",
			channel:     "test:channel",
			shouldMatch: false,
		},
		{
			name:        "wildcard_no_match",
			pattern:     "other:*",
			channel:     "test:channel",
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := New()
			defer m.Close()

			var received atomic.Int32

			err := m.Subscribe(context.Background(), tt.pattern, func(_ context.Context, _ *pubsub.Message) error {
				received.Add(1)

				return nil
			})
			require.NoError(t, err)

			msg := &pubsub.Message{
				ID:        "1",
				Channel:   tt.channel,
				Type:      "test",
				Timestamp: time.Now(),
			}

			err = m.Publish(context.Background(), tt.channel, msg)
			require.NoError(t, err)

			if tt.shouldMatch {
				assert.Equal(t, int32(1), received.Load())
			} else {
				assert.Equal(t, int32(0), received.Load())
			}
		})
	}
}

func TestMemory_Unsubscribe(t *testing.T) {
	t.Parallel()

	m := New()
	defer m.Close()

	var received atomic.Int32

	err := m.Subscribe(context.Background(), "test:channel", func(_ context.Context, _ *pubsub.Message) error {
		received.Add(1)

		return nil
	})
	require.NoError(t, err)

	err = m.Unsubscribe(context.Background(), "test:channel")
	require.NoError(t, err)

	msg := &pubsub.Message{
		ID:        "1",
		Channel:   "test:channel",
		Type:      "test",
		Timestamp: time.Now(),
	}

	err = m.Publish(context.Background(), "test:channel", msg)
	require.NoError(t, err)

	assert.Equal(t, int32(0), received.Load())
}

func TestMemory_EmptyPattern(t *testing.T) {
	t.Parallel()

	m := New()
	defer m.Close()

	err := m.Subscribe(context.Background(), "", func(_ context.Context, _ *pubsub.Message) error {
		return nil
	})

	assert.ErrorIs(t, err, pubsub.ErrEmptyPattern)
}

func TestMemory_ClosedPubSub(t *testing.T) {
	t.Parallel()

	m := New()
	_ = m.Close()

	err := m.Subscribe(context.Background(), "test", func(_ context.Context, _ *pubsub.Message) error {
		return nil
	})
	assert.ErrorIs(t, err, pubsub.ErrClosed)

	msg := &pubsub.Message{ID: "1", Channel: "test", Type: "test", Timestamp: time.Now()}
	err = m.Publish(context.Background(), "test", msg)
	assert.ErrorIs(t, err, pubsub.ErrClosed)
}

func TestMemory_MultipleSubscribers(t *testing.T) {
	t.Parallel()

	m := New()
	defer m.Close()

	var received1, received2 atomic.Int32

	err := m.Subscribe(context.Background(), "test:channel", func(_ context.Context, _ *pubsub.Message) error {
		received1.Add(1)

		return nil
	})
	require.NoError(t, err)

	err = m.Subscribe(context.Background(), "test:channel", func(_ context.Context, _ *pubsub.Message) error {
		received2.Add(1)

		return nil
	})
	require.NoError(t, err)

	msg := &pubsub.Message{
		ID:        "1",
		Channel:   "test:channel",
		Type:      "test",
		Timestamp: time.Now(),
	}

	err = m.Publish(context.Background(), "test:channel", msg)
	require.NoError(t, err)

	assert.Equal(t, int32(1), received1.Load())
	assert.Equal(t, int32(1), received2.Load())
}

func TestMemory_HandlerPanicRecovery(t *testing.T) {
	t.Parallel()

	m := New()
	defer m.Close()

	var received atomic.Int32

	err := m.Subscribe(context.Background(), "test:channel", func(_ context.Context, _ *pubsub.Message) error {
		panic("test panic")
	})
	require.NoError(t, err)

	err = m.Subscribe(context.Background(), "test:channel", func(_ context.Context, _ *pubsub.Message) error {
		received.Add(1)

		return nil
	})
	require.NoError(t, err)

	msg := &pubsub.Message{
		ID:        "1",
		Channel:   "test:channel",
		Type:      "test",
		Timestamp: time.Now(),
	}

	err = m.Publish(context.Background(), "test:channel", msg)
	require.NoError(t, err)

	assert.Equal(t, int32(1), received.Load())
}
