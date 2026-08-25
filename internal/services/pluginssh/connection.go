package pluginssh

import (
	"log/slog"
	"sync"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

const keepaliveReplyTimeout = 15 * time.Second

// connection is one host-opened SSH client held on behalf of a plugin.
type connection struct {
	handle      uint64
	client      *ssh.Client
	host        string
	fingerprint string
	idleTimeout time.Duration

	mu       sync.Mutex
	lastUsed time.Time

	closeOnce sync.Once
	closed    chan struct{}
}

func (c *connection) touch() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastUsed = time.Now()
}

func (c *connection) idleFor() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()

	return time.Since(c.lastUsed)
}

func (c *connection) close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		_ = c.client.Close()
	})
}

// run watches the connection: it notices a dropped transport, keeps an idle
// connection alive only while the plugin still needs it, and retires it once
// nothing has run on it for the idle timeout.
func (c *connection) run(sessions *Sessions) {
	interval := sessions.svc.cfg.KeepaliveInterval
	if c.idleTimeout > 0 && c.idleTimeout/2 < interval {
		interval = c.idleTimeout / 2
	}
	// A sub-second sweep would flood the remote host with keepalive probes
	// while a command is running (and time.NewTicker refuses zero outright).
	if interval < time.Second {
		interval = time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	waitErr := make(chan error, 1)
	go func() { waitErr <- c.client.Wait() }()

	for {
		select {
		case <-c.closed:
			return
		case err := <-waitErr:
			sessions.connectionLost(c, err)

			return
		case <-ticker.C:
			if sessions.retireIfIdle(c) {
				return
			}

			if !c.keepalive() {
				sessions.connectionLost(c, errConnectionLost)

				return
			}
		}
	}
}

// keepalive probes the connection so a silently dropped transport is noticed
// before the plugin tries to use the handle.
func (c *connection) keepalive() bool {
	result := make(chan error, 1)

	go func() {
		_, _, err := c.client.SendRequest("keepalive@openssh.com", true, nil)
		result <- err
	}()

	timer := time.NewTimer(keepaliveReplyTimeout)
	defer timer.Stop()

	select {
	case err := <-result:
		return err == nil
	case <-timer.C:
		return false
	case <-c.closed:
		return true
	}
}

// retireIfIdle closes a connection nothing has used for the idle timeout,
// unless operations are still running on it.
func (s *Sessions) retireIfIdle(conn *connection) bool {
	if conn.idleTimeout <= 0 || conn.idleFor() < conn.idleTimeout {
		return false
	}

	s.mu.Lock()
	for _, op := range s.ops {
		if op.handle == conn.handle && !op.finishedNow() {
			s.mu.Unlock()

			return false
		}
	}

	if _, ok := s.conns[conn.handle]; !ok {
		s.mu.Unlock()

		return true
	}
	delete(s.conns, conn.handle)
	s.mu.Unlock()

	s.svc.logger.Info("plugin ssh connection closed after idle timeout",
		slog.Uint64("plugin_id", s.pluginID),
		slog.String("host", conn.host),
		slog.Uint64("handle", conn.handle))

	conn.close()

	return true
}

// connectionLost retires a connection whose transport went away and fails the
// operations that were running on it.
func (s *Sessions) connectionLost(conn *connection, err error) {
	s.mu.Lock()
	_, tracked := s.conns[conn.handle]
	delete(s.conns, conn.handle)
	s.mu.Unlock()

	conn.close()

	if !tracked {
		return
	}

	reason := "connection closed"
	if err != nil && !errors.Is(err, errConnectionLost) {
		reason = "connection closed: " + err.Error()
	}

	s.svc.logger.Warn("plugin ssh connection lost",
		slog.Uint64("plugin_id", s.pluginID),
		slog.String("host", conn.host),
		slog.Uint64("handle", conn.handle),
		slog.String("reason", reason))

	s.cancelConnectionOperations(conn.handle, reason, errConnectionLost)
}
