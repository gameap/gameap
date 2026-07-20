package redis_test

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/pubsub"
	pubsubredis "github.com/gameap/gameap/internal/pubsub/redis"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRedisClient(t *testing.T) *goredis.Client {
	t.Helper()

	testRedisAddr := os.Getenv("TEST_REDIS_ADDR")
	if testRedisAddr == "" {
		t.Skip("Skipping Redis pubsub tests because TEST_REDIS_ADDR is not set")
	}

	testRedisPassword := os.Getenv("TEST_REDIS_PASSWORD")

	client := goredis.NewClient(&goredis.Options{
		Addr:     testRedisAddr,
		Password: testRedisPassword,
		DB:       1,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Skipping Redis pubsub tests because Redis is not available: %v", err)
	}

	return client
}

// startListener launches ps.Start in a background goroutine and returns a
// teardown func that cancels the Start context, waits for the goroutine to
// exit, then closes ps. Deferring the returned func guarantees Close never
// races with the still-running Start goroutine.
func startListener(t *testing.T, ps *pubsubredis.Redis) func() {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = ps.Start(ctx)
	}()

	return func() {
		cancel()
		<-done
		_ = ps.Close()
	}
}

func TestRedis_PublishSubscribe(t *testing.T) {
	client := setupRedisClient(t)
	defer client.Close()

	ps := pubsubredis.NewFromClient(client, "test-instance")

	var received atomic.Int32
	receivedCh := make(chan *pubsub.Message, 1)

	err := ps.Subscribe(context.Background(), "test:channel", func(_ context.Context, msg *pubsub.Message) error {
		received.Add(1)
		receivedCh <- msg

		return nil
	})
	require.NoError(t, err)

	stop := startListener(t, ps)
	defer stop()

	time.Sleep(100 * time.Millisecond)

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
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}

	assert.Equal(t, int32(1), received.Load())
}

func TestRedis_PatternSubscription(t *testing.T) {
	client := setupRedisClient(t)
	defer client.Close()

	ps := pubsubredis.NewFromClient(client, "test-instance")

	var received atomic.Int32
	receivedCh := make(chan *pubsub.Message, 3)

	err := ps.Subscribe(context.Background(), "test:*", func(_ context.Context, msg *pubsub.Message) error {
		received.Add(1)
		receivedCh <- msg

		return nil
	})
	require.NoError(t, err)

	stop := startListener(t, ps)
	defer stop()

	time.Sleep(100 * time.Millisecond)

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

	timeout := time.After(2 * time.Second)
	for range 3 {
		select {
		case <-receivedCh:
		case <-timeout:
			t.Fatal("timeout waiting for messages")
		}
	}

	assert.Equal(t, int32(3), received.Load())
}

func TestRedis_DynamicSubscription(t *testing.T) {
	client := setupRedisClient(t)
	defer client.Close()

	ps := pubsubredis.NewFromClient(client, "test-instance")

	var received atomic.Int32
	receivedCh := make(chan struct{}, 1)

	stop := startListener(t, ps)
	defer stop()

	time.Sleep(100 * time.Millisecond)

	err := ps.Subscribe(context.Background(), "dynamic:channel", func(_ context.Context, _ *pubsub.Message) error {
		received.Add(1)
		receivedCh <- struct{}{}

		return nil
	})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	msg := &pubsub.Message{
		ID:        "1",
		Channel:   "dynamic:channel",
		Type:      "test",
		Timestamp: time.Now(),
	}

	err = ps.Publish(context.Background(), "dynamic:channel", msg)
	require.NoError(t, err)

	select {
	case <-receivedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}

	assert.Equal(t, int32(1), received.Load())
}

func TestRedis_Unsubscribe(t *testing.T) {
	client := setupRedisClient(t)
	defer client.Close()

	ps := pubsubredis.NewFromClient(client, "test-instance")

	var received atomic.Int32

	err := ps.Subscribe(context.Background(), "unsub:channel", func(_ context.Context, _ *pubsub.Message) error {
		received.Add(1)

		return nil
	})
	require.NoError(t, err)

	stop := startListener(t, ps)
	defer stop()

	time.Sleep(100 * time.Millisecond)

	err = ps.Unsubscribe(context.Background(), "unsub:channel")
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	msg := &pubsub.Message{
		ID:        "1",
		Channel:   "unsub:channel",
		Type:      "test",
		Timestamp: time.Now(),
	}

	err = ps.Publish(context.Background(), "unsub:channel", msg)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	assert.Equal(t, int32(0), received.Load())
}

func TestRedis_EmptyPattern(t *testing.T) {
	client := setupRedisClient(t)
	defer client.Close()

	ps := pubsubredis.NewFromClient(client, "test-instance")
	defer ps.Close()

	err := ps.Subscribe(context.Background(), "", func(_ context.Context, _ *pubsub.Message) error {
		return nil
	})

	assert.ErrorIs(t, err, pubsub.ErrEmptyPattern)
}

func TestRedis_ClosedPubSub(t *testing.T) {
	client := setupRedisClient(t)
	defer client.Close()

	ps := pubsubredis.NewFromClient(client, "test-instance")
	_ = ps.Close()

	err := ps.Subscribe(context.Background(), "test", func(_ context.Context, _ *pubsub.Message) error {
		return nil
	})
	assert.ErrorIs(t, err, pubsub.ErrClosed)

	msg := &pubsub.Message{ID: "1", Channel: "test", Type: "test", Timestamp: time.Now()}
	err = ps.Publish(context.Background(), "test", msg)
	assert.ErrorIs(t, err, pubsub.ErrClosed)
}
