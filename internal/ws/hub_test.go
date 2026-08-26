package ws

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newOfflineClient builds a Client with no real connection. Suitable for Hub
// tests that exercise registry / fan-out without needing the read or write
// pumps. Run must NOT be called on the returned client.
func newOfflineClient(t *testing.T, hub *Hub) *Client {
	t.Helper()

	return NewClient(context.Background(), nil, hub, nil, nil)
}

func TestNewHub(t *testing.T) {
	t.Parallel()
	hub := NewHub(nil)

	require.NotNil(t, hub)
	require.NotNil(t, hub.clients)
	require.NotNil(t, hub.topics)
	require.NotNil(t, hub.logger)
	assert.Empty(t, hub.clients)
	assert.Empty(t, hub.topics)
}

func TestHub_Register(t *testing.T) {
	t.Parallel()
	t.Run("without_topics", func(t *testing.T) {
		t.Parallel()
		hub := NewHub(nil)
		client := newOfflineClient(t, hub)

		hub.Register(client)

		assert.Len(t, hub.clients, 1)
		assert.Empty(t, hub.topics)
	})

	t.Run("with_single_topic", func(t *testing.T) {
		t.Parallel()
		hub := NewHub(nil)
		client := newOfflineClient(t, hub)

		hub.Register(client, "topic-a")

		assert.Len(t, hub.clients, 1)
		assert.Equal(t, 1, hub.TopicSubscriberCount("topic-a"))
	})

	t.Run("with_multiple_topics", func(t *testing.T) {
		t.Parallel()
		hub := NewHub(nil)
		client := newOfflineClient(t, hub)

		hub.Register(client, "topic-a", "topic-b", "topic-c")

		assert.Len(t, hub.clients, 1)
		assert.Equal(t, 1, hub.TopicSubscriberCount("topic-a"))
		assert.Equal(t, 1, hub.TopicSubscriberCount("topic-b"))
		assert.Equal(t, 1, hub.TopicSubscriberCount("topic-c"))
	})

	t.Run("multiple_clients_same_topic", func(t *testing.T) {
		t.Parallel()
		hub := NewHub(nil)
		c1 := newOfflineClient(t, hub)
		c2 := newOfflineClient(t, hub)

		hub.Register(c1, "shared")
		hub.Register(c2, "shared")

		assert.Len(t, hub.clients, 2)
		assert.Equal(t, 2, hub.TopicSubscriberCount("shared"))
	})
}

func TestHub_Subscribe(t *testing.T) {
	t.Parallel()
	t.Run("creates_new_topic_bucket", func(t *testing.T) {
		t.Parallel()
		hub := NewHub(nil)
		client := newOfflineClient(t, hub)
		hub.Register(client)

		hub.Subscribe(client, "new-topic")

		assert.Equal(t, 1, hub.TopicSubscriberCount("new-topic"))
	})

	t.Run("adds_to_existing_topic_bucket", func(t *testing.T) {
		t.Parallel()
		hub := NewHub(nil)
		c1 := newOfflineClient(t, hub)
		c2 := newOfflineClient(t, hub)

		hub.Register(c1, "shared")
		hub.Register(c2)
		hub.Subscribe(c2, "shared")

		assert.Equal(t, 2, hub.TopicSubscriberCount("shared"))
	})

	t.Run("re_subscribe_is_no_op", func(t *testing.T) {
		t.Parallel()
		hub := NewHub(nil)
		client := newOfflineClient(t, hub)
		hub.Register(client, "topic")

		hub.Subscribe(client, "topic")
		hub.Subscribe(client, "topic")

		assert.Equal(t, 1, hub.TopicSubscriberCount("topic"))
	})

	t.Run("multiple_topics_at_once", func(t *testing.T) {
		t.Parallel()
		hub := NewHub(nil)
		client := newOfflineClient(t, hub)
		hub.Register(client)

		hub.Subscribe(client, "a", "b", "c")

		assert.Equal(t, 1, hub.TopicSubscriberCount("a"))
		assert.Equal(t, 1, hub.TopicSubscriberCount("b"))
		assert.Equal(t, 1, hub.TopicSubscriberCount("c"))
	})
}

func TestHub_Unregister(t *testing.T) {
	t.Parallel()
	t.Run("removes_from_clients_and_topics", func(t *testing.T) {
		t.Parallel()
		hub := NewHub(nil)
		client := newOfflineClient(t, hub)
		hub.Register(client, "topic-a", "topic-b")

		hub.Unregister(client)

		assert.Empty(t, hub.clients)
		assert.Equal(t, 0, hub.TopicSubscriberCount("topic-a"))
		assert.Equal(t, 0, hub.TopicSubscriberCount("topic-b"))
	})

	t.Run("empty_topic_buckets_are_deleted", func(t *testing.T) {
		t.Parallel()
		hub := NewHub(nil)
		client := newOfflineClient(t, hub)
		hub.Register(client, "lonely")

		hub.Unregister(client)

		hub.mu.RLock()
		_, exists := hub.topics["lonely"]
		hub.mu.RUnlock()
		assert.False(t, exists, "empty topic bucket must be deleted")
	})

	t.Run("does_not_affect_other_clients", func(t *testing.T) {
		t.Parallel()
		hub := NewHub(nil)
		c1 := newOfflineClient(t, hub)
		c2 := newOfflineClient(t, hub)
		hub.Register(c1, "shared")
		hub.Register(c2, "shared")

		hub.Unregister(c1)

		assert.Len(t, hub.clients, 1)
		assert.Equal(t, 1, hub.TopicSubscriberCount("shared"))
	})

	t.Run("unknown_client_is_safe", func(t *testing.T) {
		t.Parallel()
		hub := NewHub(nil)
		client := newOfflineClient(t, hub)

		assert.NotPanics(t, func() {
			hub.Unregister(client)
		})
	})
}

func TestHub_Broadcast(t *testing.T) {
	t.Parallel()
	t.Run("delivers_to_single_subscriber", func(t *testing.T) {
		t.Parallel()
		hub := NewHub(nil)
		client := newOfflineClient(t, hub)
		hub.Register(client, "topic")

		hub.Broadcast("topic", []byte("hello"))

		require.Len(t, client.send, 1)
		assert.Equal(t, []byte("hello"), <-client.send)
	})

	t.Run("delivers_to_multiple_subscribers", func(t *testing.T) {
		t.Parallel()
		hub := NewHub(nil)
		c1 := newOfflineClient(t, hub)
		c2 := newOfflineClient(t, hub)
		c3 := newOfflineClient(t, hub)
		hub.Register(c1, "topic")
		hub.Register(c2, "topic")
		hub.Register(c3, "other")

		hub.Broadcast("topic", []byte("hi"))

		require.Len(t, c1.send, 1)
		require.Len(t, c2.send, 1)
		assert.Empty(t, c3.send, "client subscribed to a different topic must not receive the message")
	})

	t.Run("no_op_for_empty_topic", func(t *testing.T) {
		t.Parallel()
		hub := NewHub(nil)

		assert.NotPanics(t, func() {
			hub.Broadcast("nobody-listens", []byte("payload"))
		})
	})

	t.Run("non_blocking_when_subscriber_buffer_full", func(t *testing.T) {
		t.Parallel()
		hub := NewHub(nil)
		slow := newOfflineClient(t, hub)
		fast := newOfflineClient(t, hub)
		hub.Register(slow, "topic")
		hub.Register(fast, "topic")

		for range defaultSendBufferSize {
			slow.send <- []byte("preload")
		}

		done := make(chan struct{})
		go func() {
			hub.Broadcast("topic", []byte("real"))
			close(done)
		}()

		select {
		case <-done:
		default:
			<-done
		}

		require.Len(t, fast.send, 1)
		assert.Equal(t, []byte("real"), <-fast.send)
		require.Len(t, slow.send, defaultSendBufferSize, "slow client buffer must remain full; new message dropped")
	})

	t.Run("delivers_only_to_subscribers_of_topic", func(t *testing.T) {
		t.Parallel()
		hub := NewHub(nil)
		c1 := newOfflineClient(t, hub)
		c2 := newOfflineClient(t, hub)
		hub.Register(c1, "a")
		hub.Register(c2, "b")

		hub.Broadcast("a", []byte("for-a"))

		require.Len(t, c1.send, 1)
		assert.Empty(t, c2.send)
	})
}

func TestHub_TopicSubscriberCount(t *testing.T) {
	t.Parallel()
	hub := NewHub(nil)

	assert.Equal(t, 0, hub.TopicSubscriberCount("missing"))

	c1 := newOfflineClient(t, hub)
	c2 := newOfflineClient(t, hub)
	hub.Register(c1, "topic")
	hub.Register(c2, "topic")

	assert.Equal(t, 2, hub.TopicSubscriberCount("topic"))

	hub.Unregister(c1)
	assert.Equal(t, 1, hub.TopicSubscriberCount("topic"))
}

func TestHub_Close(t *testing.T) {
	t.Parallel()
	hub := NewHub(nil)
	c1 := newOfflineClient(t, hub)
	c2 := newOfflineClient(t, hub)
	hub.Register(c1, "topic")
	hub.Register(c2, "topic")

	hub.Close()

	select {
	case <-c1.Done():
	default:
		t.Fatal("c1 must be closed after Hub.Close")
	}
	select {
	case <-c2.Done():
	default:
		t.Fatal("c2 must be closed after Hub.Close")
	}
}

func TestHub_ConcurrentRegisterUnregisterBroadcast(t *testing.T) {
	t.Parallel()
	hub := NewHub(nil)

	const goroutines = 50
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()

			client := newOfflineClient(t, hub)
			hub.Register(client, "shared", "private")

			for i := range opsPerGoroutine {
				switch i % 4 {
				case 0:
					hub.Subscribe(client, "extra")
				case 1:
					hub.Broadcast("shared", []byte("ping"))
				case 2:
					_ = hub.TopicSubscriberCount("shared")
				case 3:
					hub.Broadcast("private", []byte("pong"))
				}
			}

			hub.Unregister(client)
		}()
	}

	wg.Wait()

	assert.Empty(t, hub.clients, "all clients must be unregistered")
	assert.Equal(t, 0, hub.TopicSubscriberCount("shared"))
	assert.Equal(t, 0, hub.TopicSubscriberCount("private"))
	assert.Equal(t, 0, hub.TopicSubscriberCount("extra"))
}
