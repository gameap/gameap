package rcon

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPool(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid_source_config",
			config: Config{
				Address:  "127.0.0.1:27015",
				Password: "test",
				Protocol: ProtocolSource,
				Timeout:  5 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "valid_goldsource_config",
			config: Config{
				Address:  "127.0.0.1:27015",
				Password: "test",
				Protocol: ProtocolGoldSrc,
				Timeout:  5 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "invalid_protocol",
			config: Config{
				Address:  "127.0.0.1:27015",
				Password: "test",
				Protocol: "invalid",
				Timeout:  5 * time.Second,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pool, err := NewPool(tt.config)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, pool)
			} else {
				require.NoError(t, err)
				require.NotNil(t, pool)
				defer pool.Close()
				assert.NotNil(t, pool.p)
			}
		})
	}
}

func TestPool_Close(t *testing.T) {
	t.Parallel()
	config := Config{
		Address:  "127.0.0.1:27015",
		Password: "test",
		Protocol: ProtocolSource,
		Timeout:  5 * time.Second,
	}

	pool, err := NewPool(config)
	require.NoError(t, err)
	require.NotNil(t, pool)

	pool.Close()

	// After close, pool should not allow new acquisitions
	ctx := context.Background()
	_, err = pool.TryAcquire(ctx)
	assert.Error(t, err)
}

func TestPool_Stat(t *testing.T) {
	t.Parallel()
	config := Config{
		Address:  "127.0.0.1:27015",
		Password: "test",
		Protocol: ProtocolSource,
		Timeout:  5 * time.Second,
	}

	pool, err := NewPool(config)
	require.NoError(t, err)
	require.NotNil(t, pool)
	defer pool.Close()

	stat := pool.Stat()
	require.NotNil(t, stat)

	// Initially pool should be empty
	assert.Equal(t, int32(0), stat.AcquiredResources())
	assert.Equal(t, int32(0), stat.TotalResources())
}

func TestPooledClient_Open(t *testing.T) {
	t.Parallel()
	config := Config{
		Address:  "127.0.0.1:27015",
		Password: "test",
		Protocol: ProtocolSource,
		Timeout:  5 * time.Second,
	}

	pool, err := NewPool(config)
	require.NoError(t, err)
	require.NotNil(t, pool)
	defer pool.Close()

	// Open should always succeed for pooled clients (already authenticated)
	client := &PooledClient{}
	err = client.Open(context.Background())
	assert.NoError(t, err)
}

func TestPooledClient_Close(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		client  *PooledClient
		wantErr bool
	}{
		{
			name:    "nil_resource",
			client:  &PooledClient{r: nil},
			wantErr: false,
		},
		{
			name: "close_twice",
			client: &PooledClient{
				r: nil, // Already released
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.client.Close()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPooledClient_Execute_NilResource(t *testing.T) {
	t.Parallel()
	client := &PooledClient{r: nil}

	_, err := client.Execute(context.Background(), "status")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection not established")
}

func TestPuddlePanicError(t *testing.T) {
	t.Parallel()
	details := "test panic details"
	err := newPuddlePanicError(details)

	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "panic in puddle")
	assert.Contains(t, err.Error(), details)

	// Check type using errors.As
	var panicErr puddlePanicError
	require.True(t, errors.As(err, &panicErr))
	assert.Equal(t, details, panicErr.details)
}

func TestPool_Acquire_OpensRealConnectionViaConstructor(t *testing.T) {
	t.Parallel()
	srv := newScriptedTCPServer(t, sourceAuthOKThenEcho(t))

	pool, err := NewPool(Config{
		Address:  srv.addr,
		Password: "secret",
		Protocol: ProtocolSource,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)
	defer pool.Close()

	client, err := pool.Acquire(context.Background())
	require.NoError(t, err, "acquire must succeed against a server that authenticates")
	defer func() { _ = client.Close() }()

	stat := pool.Stat()
	assert.Equal(t, int32(1), stat.AcquiredResources(),
		"after a successful acquire there must be exactly one acquired resource")
}

func TestPool_Acquire_PropagatesAuthFailure(t *testing.T) {
	t.Parallel()
	srv := newScriptedTCPServer(t, func(conn net.Conn) {
		_, _, _, err := readSourcePacket(conn)
		if err != nil {
			return
		}
		_, _ = conn.Write(buildSourcePacket(t, -1, serverDataAuthResponse, ""))
	})

	pool, err := NewPool(Config{
		Address:  srv.addr,
		Password: "wrong",
		Protocol: ProtocolSource,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)
	defer pool.Close()

	_, err = pool.Acquire(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed",
		"a server rejecting auth must surface as an acquire error")
}

func TestPool_TryAcquire_ReusesIdleConnection(t *testing.T) {
	t.Parallel()
	srv := newScriptedTCPServer(t, sourceAuthOKThenEcho(t))

	pool, err := NewPool(Config{
		Address:  srv.addr,
		Password: "secret",
		Protocol: ProtocolSource,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)
	defer pool.Close()

	// Prime the pool by Acquire+Close — TryAcquire only returns an already-idle resource.
	primed, err := pool.Acquire(context.Background())
	require.NoError(t, err, "first Acquire must construct the underlying resource")
	require.NoError(t, primed.Close())

	client, err := pool.TryAcquire(context.Background())
	require.NoError(t, err, "TryAcquire must succeed when an idle resource exists")
	defer func() { _ = client.Close() }()

	got, err := client.Execute(context.Background(), "status")
	require.NoError(t, err)
	assert.Equal(t, "status-reply", got, "the reused pooled client must still execute commands")
}

func TestPooledClient_Execute_DestroysResourceOnError(t *testing.T) {
	t.Parallel()
	var connectCount atomic.Int32

	srv := newScriptedTCPServer(t, func(conn net.Conn) {
		connectCount.Add(1)
		authID, _, _, err := readSourcePacket(conn)
		if err != nil {
			return
		}
		_, _ = conn.Write(buildSourcePacket(t, authID, serverDataAuthResponse, ""))

		// Drop the connection mid-command on first session; on subsequent sessions, behave normally.
		if connectCount.Load() == 1 {
			_, _, _, _ = readSourcePacket(conn)
			_ = conn.Close()

			return
		}
		for {
			id, _, _, err := readSourcePacket(conn)
			if err != nil {
				return
			}
			_, _ = conn.Write(buildSourcePacket(t, id, serverDataResponseValue, "ok"))
		}
	})

	pool, err := NewPool(Config{
		Address:  srv.addr,
		Password: "secret",
		Protocol: ProtocolSource,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)
	defer pool.Close()

	first, err := pool.Acquire(context.Background())
	require.NoError(t, err)

	_, err = first.Execute(context.Background(), "status")
	require.Error(t, err, "first execute must fail because the server drops the connection")
	// Note: do NOT call first.Close() here — Execute already invoked c.r.Destroy() and a follow-up
	// Release would race with the destroy goroutine, double-decrementing puddle's destructWG.

	// Wait for puddle's async destroy goroutine to release the semaphore slot before re-acquiring.
	require.Eventually(t, func() bool {
		s := pool.Stat()

		return s.AcquiredResources() == 0 && s.TotalResources() == 0
	}, 2*time.Second, 10*time.Millisecond,
		"the failed Execute must asynchronously destroy the underlying resource")

	second, err := pool.Acquire(context.Background())
	require.NoError(t, err, "after a destroyed resource, a fresh one must be created on demand")
	defer func() { _ = second.Close() }()

	got, err := second.Execute(context.Background(), "status")
	require.NoError(t, err, "the freshly created resource must work")
	assert.Equal(t, "ok", got)

	assert.GreaterOrEqual(t, connectCount.Load(), int32(2),
		"a destroyed resource must trigger a brand-new TCP dial on next acquire")
}

func TestPool_Close_StopsBackgroundCleanupGoroutine(t *testing.T) {
	t.Parallel()
	srv := newScriptedTCPServer(t, sourceAuthOKThenEcho(t))

	pool, err := NewPool(Config{
		Address:  srv.addr,
		Password: "secret",
		Protocol: ProtocolSource,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		pool.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Pool.Close blocked — background cleanup goroutine likely did not exit")
	}
}

// sourceAuthOKThenEcho returns a connection handler that authenticates the client and then echoes
// every subsequent EXECCOMMAND with body "<command>-reply". Use it whenever the test cares about
// pool behaviour, not protocol details.
func sourceAuthOKThenEcho(t *testing.T) func(net.Conn) {
	t.Helper()

	return func(conn net.Conn) {
		authID, _, _, err := readSourcePacket(conn)
		if err != nil {
			return
		}
		_, _ = conn.Write(buildSourcePacket(t, authID, serverDataAuthResponse, ""))

		for {
			id, _, body, err := readSourcePacket(conn)
			if err != nil {
				return
			}
			_, _ = conn.Write(buildSourcePacket(t, id, serverDataResponseValue, body+"-reply"))
		}
	}
}

// TestPool_BackgroundCleanup_DestroysStaleIdleConnections waits for the cleanup goroutine to
// fire at least once and verifies that an idle resource whose lastUsedAt has been backdated past
// maxIdleTime is destroyed. The wait is bounded by cleanupInterval (5s) plus a small slack.
func TestPool_BackgroundCleanup_DestroysStaleIdleConnections(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("background cleanup test waits for the cleanup ticker (>cleanupInterval); skipped in short mode")
	}

	// ARRANGE
	srv := newScriptedTCPServer(t, sourceAuthOKThenEcho(t))

	pool, err := NewPool(Config{
		Address:  srv.addr,
		Password: "secret",
		Protocol: ProtocolSource,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)
	defer pool.Close()

	require.NoError(t, pool.p.CreateResource(context.Background()),
		"pre-creating a resource gives the cleanup tick something to find without an explicit Acquire+Close cycle")

	idle := pool.p.AcquireAllIdle()
	require.Len(t, idle, 1, "exactly one idle resource must exist after CreateResource")

	wrapper := idle[0].Value()
	require.NotNil(t, wrapper)

	wrapper.mu.Lock()
	wrapper.lastUsedAt = time.Now().Add(-2 * maxIdleTime)
	wrapper.mu.Unlock()

	idle[0].Release()

	// ACT — wait for one cleanup tick (cleanupInterval) plus slack.
	require.Eventually(t, func() bool {
		s := pool.Stat()

		return s.TotalResources() == 0
	}, cleanupInterval+3*time.Second, 100*time.Millisecond,
		"a backdated idle resource must be destroyed by the cleanup goroutine within one tick")

	// ASSERT
	stat := pool.Stat()
	assert.Equal(t, int32(0), stat.TotalResources(),
		"after the cleanup tick destroys the stale idle resource the pool must be empty")
	assert.Equal(t, int32(0), stat.AcquiredResources(),
		"no resource should remain acquired after the cleanup runs")
}

// TestPool_BackgroundCleanup_KeepsRecentlyUsedConnections verifies the dual branch: a fresh idle
// resource survives a cleanup tick.
func TestPool_BackgroundCleanup_KeepsRecentlyUsedConnections(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("background cleanup test waits for the cleanup ticker (>cleanupInterval); skipped in short mode")
	}

	// ARRANGE
	srv := newScriptedTCPServer(t, sourceAuthOKThenEcho(t))

	pool, err := NewPool(Config{
		Address:  srv.addr,
		Password: "secret",
		Protocol: ProtocolSource,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)
	defer pool.Close()

	require.NoError(t, pool.p.CreateResource(context.Background()))

	require.Equal(t, int32(1), pool.Stat().TotalResources(),
		"sanity: one idle resource exists before the cleanup tick")

	// ACT — wait long enough for at least one cleanup tick to occur.
	time.Sleep(cleanupInterval + time.Second)

	// ASSERT
	stat := pool.Stat()
	assert.Equal(t, int32(1), stat.TotalResources(),
		"a fresh idle resource (within maxIdleTime) must survive the cleanup tick")
}

// TestResourceWrapper_LastUsedAccessors round-trips the time through the mutex-guarded accessors.
// Without this, getLastUsed (only invoked from cleanupIdleConnections) is unreachable in short test runs.
func TestResourceWrapper_LastUsedAccessors(t *testing.T) {
	t.Parallel()
	// ARRANGE
	w := &resourceWrapper{lastUsedAt: time.Unix(1, 0)}

	// ACT
	w.updateLastUsed()
	got := w.getLastUsed()

	// ASSERT
	assert.WithinDuration(t, time.Now(), got, time.Second,
		"updateLastUsed must move lastUsedAt to (approximately) now and getLastUsed must observe it")
	assert.NotEqual(t, time.Unix(1, 0), got,
		"the original timestamp must have been overwritten")
}
