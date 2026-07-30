package plugin

import (
	"crypto/rand"
	"encoding/binary"
	"net"
	"sync"
	"time"

	"github.com/pkg/errors"
)

var (
	// ErrConnNotFound is returned when a handle is unknown, already released,
	// or not owned by the calling plugin.
	ErrConnNotFound = errors.New("connection handle not found")
	// ErrConnExpired is returned when a handle's operation deadline has passed.
	ErrConnExpired = errors.New("connection handle expired")
	// ErrTooManyConns is returned when a plugin exceeds its open-connection cap.
	ErrTooManyConns = errors.New("too many open connections for plugin")
)

const defaultMaxConnsPerPlugin = 8

// registeredConn is a host-opened connection handed to a plugin protocol via a
// handle. The plugin performs I/O through the gameap-net host library; it never
// dials or sees the underlying socket.
type registeredConn struct {
	conn     net.Conn
	pluginID string
	deadline time.Time
}

// ConnRegistry maps opaque handles to host-opened connections. It is shared
// between the gameap-net host library (which does I/O on handles) and the
// protocol runner (which opens, registers and tears down connections). All
// methods are safe for concurrent use.
type ConnRegistry struct {
	mu                sync.Mutex
	conns             map[uint64]*registeredConn
	maxConnsPerPlugin int
}

// NewConnRegistry creates an empty registry. maxConnsPerPlugin <= 0 selects the
// default cap.
func NewConnRegistry(maxConnsPerPlugin int) *ConnRegistry {
	if maxConnsPerPlugin <= 0 {
		maxConnsPerPlugin = defaultMaxConnsPerPlugin
	}

	return &ConnRegistry{
		conns:             make(map[uint64]*registeredConn),
		maxConnsPerPlugin: maxConnsPerPlugin,
	}
}

// Register stores a host-opened connection for the given plugin and returns an
// unpredictable, non-zero handle. deadline bounds the whole operation.
func (r *ConnRegistry) Register(conn net.Conn, pluginID string, deadline time.Time) (uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := 0
	for _, c := range r.conns {
		if c.pluginID == pluginID {
			count++
		}
	}

	if count >= r.maxConnsPerPlugin {
		return 0, errors.Wrapf(ErrTooManyConns, "plugin %s has %d open", pluginID, count)
	}

	handle, err := r.newHandle()
	if err != nil {
		return 0, err
	}

	r.conns[handle] = &registeredConn{conn: conn, pluginID: pluginID, deadline: deadline}

	return handle, nil
}

// Conn resolves a handle to its connection, verifying that the calling plugin
// owns it and its deadline has not passed.
func (r *ConnRegistry) Conn(handle uint64, pluginID string) (net.Conn, time.Time, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.conns[handle]
	if !ok || c.pluginID != pluginID {
		return nil, time.Time{}, ErrConnNotFound
	}

	if !c.deadline.IsZero() && time.Now().After(c.deadline) {
		return nil, time.Time{}, ErrConnExpired
	}

	return c.conn, c.deadline, nil
}

// CloseOwned closes and removes a handle on behalf of the owning plugin (the
// gameap-net Close call). Unknown or unowned handles yield ErrConnNotFound.
func (r *ConnRegistry) CloseOwned(handle uint64, pluginID string) error {
	r.mu.Lock()
	c, ok := r.conns[handle]
	if !ok || c.pluginID != pluginID {
		r.mu.Unlock()

		return ErrConnNotFound
	}
	delete(r.conns, handle)
	r.mu.Unlock()

	return c.conn.Close()
}

// Discard closes and removes a handle regardless of ownership. It is used by the
// host-side runner to tear down a connection after a protocol call returns; a
// handle the plugin already closed is a no-op.
func (r *ConnRegistry) Discard(handle uint64) {
	r.mu.Lock()
	c, ok := r.conns[handle]
	if ok {
		delete(r.conns, handle)
	}
	r.mu.Unlock()

	if ok {
		_ = c.conn.Close()
	}
}

// Len reports the number of live handles (used by tests).
func (r *ConnRegistry) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.conns)
}

// newHandle returns a non-zero, unpredictable handle not currently in use.
// The caller holds r.mu.
func (r *ConnRegistry) newHandle() (uint64, error) {
	var buf [8]byte

	for range 16 {
		if _, err := rand.Read(buf[:]); err != nil {
			return 0, errors.Wrap(err, "failed to generate connection handle")
		}

		handle := binary.BigEndian.Uint64(buf[:])
		if handle == 0 {
			continue
		}

		if _, exists := r.conns[handle]; !exists {
			return handle, nil
		}
	}

	return 0, errors.New("failed to allocate unique connection handle")
}
