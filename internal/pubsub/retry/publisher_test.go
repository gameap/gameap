package retry_test

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/retry"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPublisher struct {
	publishFunc func(ctx context.Context, channel string, msg *pubsub.Message) error
	callCount   int
}

func (m *mockPublisher) Publish(ctx context.Context, channel string, msg *pubsub.Message) error {
	m.callCount++

	return m.publishFunc(ctx, channel, msg)
}

type mockDLQ struct {
	recordedMessages []*pubsub.Message
	recordedErrors   []error
}

func (m *mockDLQ) RecordFailure(
	_ context.Context,
	msg *pubsub.Message,
	_ string,
	err error,
	_ int,
) error {
	m.recordedMessages = append(m.recordedMessages, msg)
	m.recordedErrors = append(m.recordedErrors, err)

	return nil
}

func TestPublisher_Publish_success_on_first_attempt(t *testing.T) {
	t.Parallel()

	mock := &mockPublisher{
		publishFunc: func(_ context.Context, _ string, _ *pubsub.Message) error {
			return nil
		},
	}

	p := retry.NewPublisher(mock, retry.Config{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	msg := &pubsub.Message{ID: "1", Channel: "test", Type: "test"}
	err := p.Publish(context.Background(), "test", msg)

	require.NoError(t, err)
	assert.Equal(t, 1, mock.callCount)
}

func TestPublisher_Publish_success_after_retry(t *testing.T) {
	t.Parallel()

	attempts := 0
	mock := &mockPublisher{
		publishFunc: func(_ context.Context, _ string, _ *pubsub.Message) error {
			attempts++
			if attempts < 3 {
				return errors.New("transient error")
			}

			return nil
		},
	}

	p := retry.NewPublisher(mock, retry.Config{
		MaxRetries:   5,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	msg := &pubsub.Message{ID: "1", Channel: "test", Type: "test"}
	err := p.Publish(context.Background(), "test", msg)

	require.NoError(t, err)
	assert.Equal(t, 3, mock.callCount)
}

func TestPublisher_Publish_returns_ErrClosed_immediately(t *testing.T) {
	t.Parallel()

	mock := &mockPublisher{
		publishFunc: func(_ context.Context, _ string, _ *pubsub.Message) error {
			return pubsub.ErrClosed
		},
	}

	p := retry.NewPublisher(mock, retry.Config{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	msg := &pubsub.Message{ID: "1", Channel: "test", Type: "test"}
	err := p.Publish(context.Background(), "test", msg)

	assert.ErrorIs(t, err, pubsub.ErrClosed)
	assert.Equal(t, 1, mock.callCount)
}

func TestPublisher_Publish_returns_ErrPayloadTooLarge_immediately(t *testing.T) {
	t.Parallel()

	mock := &mockPublisher{
		publishFunc: func(_ context.Context, _ string, _ *pubsub.Message) error {
			return pubsub.ErrPayloadTooLarge
		},
	}

	p := retry.NewPublisher(mock, retry.Config{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	})

	msg := &pubsub.Message{ID: "1", Channel: "test", Type: "test"}
	err := p.Publish(context.Background(), "test", msg)

	assert.ErrorIs(t, err, pubsub.ErrPayloadTooLarge)
	assert.Equal(t, 1, mock.callCount)
}

func TestPublisher_Publish_context_cancelled(t *testing.T) {
	t.Parallel()

	mock := &mockPublisher{
		publishFunc: func(_ context.Context, _ string, _ *pubsub.Message) error {
			return errors.New("transient error")
		},
	}

	p := retry.NewPublisher(mock, retry.Config{
		MaxRetries:   10,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	msg := &pubsub.Message{ID: "1", Channel: "test", Type: "test"}
	err := p.Publish(ctx, "test", msg)

	assert.ErrorIs(t, err, context.Canceled)
}

func TestPublisher_Publish_records_in_DLQ_after_max_retries(t *testing.T) {
	t.Parallel()

	mock := &mockPublisher{
		publishFunc: func(_ context.Context, _ string, _ *pubsub.Message) error {
			return errors.New("persistent error")
		},
	}

	dlqMock := &mockDLQ{}

	p := retry.NewPublisher(mock, retry.Config{
		MaxRetries:   2,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}, retry.WithDLQ(dlqMock))

	msg := &pubsub.Message{ID: "1", Channel: "test", Type: "test"}
	err := p.Publish(context.Background(), "test", msg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "publish failed after retries")
	assert.Equal(t, 3, mock.callCount)
	require.Len(t, dlqMock.recordedMessages, 1)
	assert.Equal(t, msg.ID, dlqMock.recordedMessages[0].ID)
}

func TestPublisher_Publish_uses_default_config_values(t *testing.T) {
	t.Parallel()

	mock := &mockPublisher{
		publishFunc: func(_ context.Context, _ string, _ *pubsub.Message) error {
			return nil
		},
	}

	p := retry.NewPublisher(mock, retry.Config{})

	msg := &pubsub.Message{ID: "1", Channel: "test", Type: "test"}
	err := p.Publish(context.Background(), "test", msg)

	require.NoError(t, err)
	assert.Equal(t, 1, mock.callCount)
}
