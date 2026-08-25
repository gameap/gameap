package pluginssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

const connTestHandle = 0xC0FFEE

var errConnTestTransport = errors.New("transport went away")

// connTestSilentClient returns a client whose peer completes the handshake and
// then never answers a global request. A keepalive probe over it stays pending
// for as long as the test needs, which is what makes the lifecycle branches
// observable instead of racing a probe that fails on its own.
//
// The transport is a loopback socket rather than net.Pipe on purpose: the SSH
// handshake writes its version line before reading the peer's, so both sides of
// an unbuffered pipe block in the write and the test hangs until its deadline.
func connTestSilentClient(t *testing.T) *ssh.Client {
	t.Helper()

	_, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	signer, err := ssh.NewSignerFromKey(private)
	require.NoError(t, err)

	serverConfig := &ssh.ServerConfig{NoClientAuth: true}
	serverConfig.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		peer, err := listener.Accept()
		if err != nil {
			return
		}

		// The channel carrying global requests is never read, so a keepalive
		// lands in the mux buffer and is never replied to.
		serverConn, channels, _, err := ssh.NewServerConn(peer, serverConfig)
		if err != nil {
			_ = peer.Close()

			return
		}
		defer func() { _ = serverConn.Close() }()

		for newChannel := range channels {
			_ = newChannel.Reject(ssh.Prohibited, "unsupported")
		}
	}()

	client, err := ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{
		User:            "gameap",
		HostKeyCallback: func(string, net.Addr, ssh.PublicKey) error { return nil },
		Timeout:         10 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	return client
}

// connTestRegisterConnection puts a connection into the set without starting
// the watchdog goroutine, so the test drives retireIfIdle and connectionLost
// itself instead of racing the real sweep. It has been idle for an hour: the
// decision under test must come from the set, never from the clock.
func connTestRegisterConnection(t *testing.T, sessions *Sessions, idleTimeout time.Duration) *connection {
	t.Helper()

	conn := &connection{
		handle:      connTestHandle,
		client:      connTestSilentClient(t),
		host:        "127.0.0.1",
		idleTimeout: idleTimeout,
		lastUsed:    time.Now().Add(-time.Hour),
		closed:      make(chan struct{}),
	}

	sessions.mu.Lock()
	sessions.conns[connTestHandle] = conn
	sessions.mu.Unlock()

	return conn
}

// connTestRegisterOperation registers a running operation on the connection
// handle and returns the context whose cause tells whether the engine cancelled
// it. Closing the session set releases the context.
func connTestRegisterOperation(sessions *Sessions) context.Context {
	op := execTestOperation(connTestHandle, false)

	opCtx, cancel := context.WithCancelCause(context.Background())
	op.cancelFn = cancel

	sessions.mu.Lock()
	sessions.ops[op.id] = op
	sessions.mu.Unlock()

	return opCtx
}

// TestConnection_RetireIfIdle covers the three ways the idle sweep must decide
// without closing anything: a connection still in use, one the panel already
// dropped, and one the plugin asked to keep open indefinitely.
func TestConnection_RetireIfIdle(t *testing.T) {
	t.Parallel()

	t.Run("connection_with_a_running_operation_is_kept", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		sessions := newTestSessions(t, Config{})
		conn := connTestRegisterConnection(t, sessions, time.Millisecond)
		connTestRegisterOperation(sessions)

		// ACT
		retired := sessions.retireIfIdle(conn)

		// ASSERT
		assert.False(t, retired, "a connection a command is still running on must stay open")

		_, err := sessions.connection(connTestHandle)
		assert.NoError(t, err, "the handle must remain usable")
	})

	t.Run("connection_already_dropped_from_the_set_stops_the_sweep", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		sessions := newTestSessions(t, Config{})
		conn := connTestRegisterConnection(t, sessions, time.Millisecond)
		require.NoError(t, sessions.Disconnect(connTestHandle))

		// ACT
		retired := sessions.retireIfIdle(conn)

		// ASSERT
		assert.True(t, retired, "the watchdog must stop once its connection is no longer tracked")
	})

	t.Run("connection_without_an_idle_timeout_is_never_retired", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		sessions := newTestSessions(t, Config{})
		conn := connTestRegisterConnection(t, sessions, 0)

		// ACT
		retired := sessions.retireIfIdle(conn)

		// ASSERT
		assert.False(t, retired, "no idle timeout means the plugin keeps the connection")

		_, err := sessions.connection(connTestHandle)
		assert.NoError(t, err, "the handle must remain usable")
	})
}

// TestConnection_Lost: a transport that goes away fails the operations of the
// connection it belonged to, while a connection the panel already dropped must
// not be processed a second time — its operations were dealt with then, with a
// reason of their own.
func TestConnection_Lost(t *testing.T) {
	t.Parallel()

	t.Run("tracked_connection_cancels_its_operations", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		sessions := newTestSessions(t, Config{})
		conn := connTestRegisterConnection(t, sessions, 0)
		opCtx := connTestRegisterOperation(sessions)

		// ACT
		sessions.connectionLost(conn, errConnTestTransport)

		// ASSERT
		require.Error(t, opCtx.Err(), "a command on a dead transport cannot be left running")
		require.Error(t, context.Cause(opCtx))
		assert.Contains(t, context.Cause(opCtx).Error(), errConnTestTransport.Error(),
			"the plugin must learn why the command stopped")

		_, err := sessions.connection(connTestHandle)
		assert.ErrorIs(t, err, ErrConnectionNotFound, "a lost connection must not stay usable")
	})

	t.Run("untracked_connection_leaves_operations_alone", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		sessions := newTestSessions(t, Config{})
		conn := connTestRegisterConnection(t, sessions, 0)
		require.NoError(t, sessions.Disconnect(connTestHandle))

		// Registered after the disconnect, the way an operation started on a
		// freshly reused handle would be: the late watchdog must not touch it.
		opCtx := connTestRegisterOperation(sessions)

		// ACT
		sessions.connectionLost(conn, errConnTestTransport)

		// ASSERT
		require.NoError(t, opCtx.Err(), "a connection the panel already dropped must not cancel anything")
	})
}

// TestConnection_RunUsesASecondTickWhenTheIdleTimeoutIsTiny: halving an idle
// timeout this small rounds to a zero interval, which time.NewTicker refuses.
// The watchdog has to fall back to a one-second sweep and still retire the
// connection.
func TestConnection_RunUsesASecondTickWhenTheIdleTimeoutIsTiny(t *testing.T) {
	t.Parallel()

	// ARRANGE
	server := newTestSSHServer(t)
	sessions := newTestSessions(t, Config{})
	host, port := server.addr()

	// ACT
	result, err := sessions.Connect(context.Background(), ConnectParams{
		Host:        host,
		Port:        port,
		User:        "gameap",
		Password:    testPassword,
		HostKey:     HostKeyPolicy{AcceptAny: true},
		IdleTimeout: time.Nanosecond,
	})
	require.NoError(t, err)

	// ASSERT
	assert.Eventually(t, func() bool {
		_, err := sessions.connection(result.Handle)

		return errors.Is(err, ErrConnectionNotFound)
	}, 5*time.Second, 20*time.Millisecond,
		"the sweep must keep running even when the halved idle timeout rounds to zero")
}

// TestConnection_KeepaliveTreatsAClosedConnectionAsAlive: the panel closing a
// connection is not the transport dying. Reporting it as lost would cancel
// operations with a misleading reason on the way out.
func TestConnection_KeepaliveTreatsAClosedConnectionAsAlive(t *testing.T) {
	t.Parallel()

	// ARRANGE
	// Only the closed channel is closed, not the client: an unanswered probe
	// keeps the other select case from ever becoming ready.
	conn := &connection{client: connTestSilentClient(t), closed: make(chan struct{})}
	close(conn.closed)

	// ACT
	alive := conn.keepalive()

	// ASSERT
	assert.True(t, alive, "a connection the panel retired must not be reported as lost")
}
