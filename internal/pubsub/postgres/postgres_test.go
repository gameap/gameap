package postgres_test

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/pubsub"
	pubsubpg "github.com/gameap/gameap/internal/pubsub/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getPostgresDSN(t *testing.T) string {
	t.Helper()

	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("Skipping PostgreSQL pubsub tests because TEST_POSTGRES_DSN is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Skipf("Skipping PostgreSQL pubsub tests because PostgreSQL is not available: %v", err)
	}
	_ = conn.Close(ctx)

	return dsn
}

func TestPostgres_PublishSubscribe(t *testing.T) {
	dsn := getPostgresDSN(t)

	ps, err := pubsubpg.New(pubsubpg.Config{
		ConnStr:    dsn,
		InstanceID: "test-instance",
	})
	require.NoError(t, err)
	defer ps.Close()

	var received atomic.Int32
	receivedCh := make(chan *pubsub.Message, 1)

	err = ps.Subscribe(context.Background(), "test:channel", func(_ context.Context, msg *pubsub.Message) error {
		received.Add(1)
		receivedCh <- msg

		return nil
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = ps.Start(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	msg := &pubsub.Message{
		ID:        "1",
		Channel:   "test:channel",
		Type:      "test.type",
		Payload:   []byte(`{"key":"value"}`),
		Timestamp: time.Now(),
	}

	err = ps.Publish(context.Background(), "test:channel", msg)
	require.NoError(t, err)

	select {
	case receivedMsg := <-receivedCh:
		assert.Equal(t, "test:channel", receivedMsg.Channel)
		assert.Equal(t, "test.type", receivedMsg.Type)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for message")
	}

	assert.Equal(t, int32(1), received.Load())
}

func TestPostgres_PatternMatching(t *testing.T) {
	dsn := getPostgresDSN(t)

	ps, err := pubsubpg.New(pubsubpg.Config{
		ConnStr:    dsn,
		InstanceID: "test-instance",
	})
	require.NoError(t, err)
	defer ps.Close()

	var received atomic.Int32
	receivedCh := make(chan *pubsub.Message, 3)

	err = ps.Subscribe(context.Background(), "test:*", func(_ context.Context, msg *pubsub.Message) error {
		received.Add(1)
		receivedCh <- msg

		return nil
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = ps.Start(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	channels := []string{"test:one", "test:two", "test:three"}
	for _, ch := range channels {
		msg := &pubsub.Message{
			ID:        ch,
			Channel:   ch,
			Type:      "test",
			Timestamp: time.Now(),
		}
		err = ps.Publish(context.Background(), ch, msg)
		require.NoError(t, err)
	}

	timeout := time.After(3 * time.Second)
	for range 3 {
		select {
		case <-receivedCh:
		case <-timeout:
			t.Fatal("timeout waiting for messages")
		}
	}

	assert.Equal(t, int32(3), received.Load())
}

func TestPostgres_PayloadTooLarge(t *testing.T) {
	dsn := getPostgresDSN(t)

	ps, err := pubsubpg.New(pubsubpg.Config{
		ConnStr:    dsn,
		InstanceID: "test-instance",
	})
	require.NoError(t, err)
	defer ps.Close()

	largePayload := strings.Repeat("x", 8000)

	msg := &pubsub.Message{
		ID:        "1",
		Channel:   "test:channel",
		Type:      "test",
		Payload:   []byte(largePayload),
		Timestamp: time.Now(),
	}

	err = ps.Publish(context.Background(), "test:channel", msg)
	assert.ErrorIs(t, err, pubsub.ErrPayloadTooLarge)
}

func TestPostgres_EmptyPattern(t *testing.T) {
	dsn := getPostgresDSN(t)

	ps, err := pubsubpg.New(pubsubpg.Config{
		ConnStr:    dsn,
		InstanceID: "test-instance",
	})
	require.NoError(t, err)
	defer ps.Close()

	err = ps.Subscribe(context.Background(), "", func(_ context.Context, _ *pubsub.Message) error {
		return nil
	})

	assert.ErrorIs(t, err, pubsub.ErrEmptyPattern)
}

func TestPostgres_ClosedPubSub(t *testing.T) {
	dsn := getPostgresDSN(t)

	ps, err := pubsubpg.New(pubsubpg.Config{
		ConnStr:    dsn,
		InstanceID: "test-instance",
	})
	require.NoError(t, err)
	_ = ps.Close()

	err = ps.Subscribe(context.Background(), "test", func(_ context.Context, _ *pubsub.Message) error {
		return nil
	})
	assert.ErrorIs(t, err, pubsub.ErrClosed)

	msg := &pubsub.Message{ID: "1", Channel: "test", Type: "test", Timestamp: time.Now()}
	err = ps.Publish(context.Background(), "test", msg)
	assert.ErrorIs(t, err, pubsub.ErrClosed)
}

func TestPostgres_Unsubscribe(t *testing.T) {
	dsn := getPostgresDSN(t)

	ps, err := pubsubpg.New(pubsubpg.Config{
		ConnStr:    dsn,
		InstanceID: "test-instance",
	})
	require.NoError(t, err)
	defer ps.Close()

	var received atomic.Int32

	err = ps.Subscribe(context.Background(), "unsub:channel", func(_ context.Context, _ *pubsub.Message) error {
		received.Add(1)

		return nil
	})
	require.NoError(t, err)

	err = ps.Unsubscribe(context.Background(), "unsub:channel")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = ps.Start(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	msg := &pubsub.Message{
		ID:        "1",
		Channel:   "unsub:channel",
		Type:      "test",
		Timestamp: time.Now(),
	}

	err = ps.Publish(context.Background(), "unsub:channel", msg)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	assert.Equal(t, int32(0), received.Load())
}

func TestPostgres_DynamicSubscription(t *testing.T) {
	dsn := getPostgresDSN(t)

	ps, err := pubsubpg.New(pubsubpg.Config{
		ConnStr:    dsn,
		InstanceID: "test-instance",
	})
	require.NoError(t, err)
	defer ps.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = ps.Start(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	var received atomic.Int32
	receivedCh := make(chan *pubsub.Message, 1)

	err = ps.Subscribe(context.Background(), "dynamic:channel", func(_ context.Context, msg *pubsub.Message) error {
		received.Add(1)
		receivedCh <- msg

		return nil
	})
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	msg := &pubsub.Message{
		ID:        "1",
		Channel:   "dynamic:channel",
		Type:      "test.type",
		Payload:   []byte(`{"key":"value"}`),
		Timestamp: time.Now(),
	}

	err = ps.Publish(context.Background(), "dynamic:channel", msg)
	require.NoError(t, err)

	select {
	case receivedMsg := <-receivedCh:
		assert.Equal(t, "dynamic:channel", receivedMsg.Channel)
		assert.Equal(t, "test.type", receivedMsg.Type)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for message")
	}

	assert.Equal(t, int32(1), received.Load())
}
