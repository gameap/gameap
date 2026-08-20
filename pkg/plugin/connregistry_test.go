package plugin

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeConn struct {
	net.Conn

	closed bool
}

func (c *fakeConn) Close() error {
	c.closed = true

	return nil
}

func TestConnRegistry_RegisterAndResolve(t *testing.T) {
	t.Parallel()
	reg := NewConnRegistry(8)
	conn := &fakeConn{}

	handle, err := reg.Register(conn, "p1", time.Now().Add(time.Minute))
	require.NoError(t, err)
	assert.NotZero(t, handle)
	assert.Equal(t, 1, reg.Len())

	got, _, err := reg.Conn(handle, "p1")
	require.NoError(t, err)
	assert.Same(t, conn, got)
}

func TestConnRegistry_OwnershipEnforced(t *testing.T) {
	t.Parallel()
	reg := NewConnRegistry(8)
	handle, err := reg.Register(&fakeConn{}, "p1", time.Time{})
	require.NoError(t, err)

	_, _, err = reg.Conn(handle, "p2")
	assert.ErrorIs(t, err, ErrConnNotFound)

	err = reg.CloseOwned(handle, "p2")
	assert.ErrorIs(t, err, ErrConnNotFound)
}

func TestConnRegistry_Expiry(t *testing.T) {
	t.Parallel()
	reg := NewConnRegistry(8)
	handle, err := reg.Register(&fakeConn{}, "p1", time.Now().Add(-time.Second))
	require.NoError(t, err)

	_, _, err = reg.Conn(handle, "p1")
	assert.ErrorIs(t, err, ErrConnExpired)
}

func TestConnRegistry_CloseOwnedClosesAndRemoves(t *testing.T) {
	t.Parallel()
	reg := NewConnRegistry(8)
	conn := &fakeConn{}
	handle, err := reg.Register(conn, "p1", time.Time{})
	require.NoError(t, err)

	require.NoError(t, reg.CloseOwned(handle, "p1"))
	assert.True(t, conn.closed)
	assert.Equal(t, 0, reg.Len())

	_, _, err = reg.Conn(handle, "p1")
	assert.ErrorIs(t, err, ErrConnNotFound)
}

func TestConnRegistry_DiscardIsIdempotent(t *testing.T) {
	t.Parallel()
	reg := NewConnRegistry(8)
	conn := &fakeConn{}
	handle, err := reg.Register(conn, "p1", time.Time{})
	require.NoError(t, err)

	reg.Discard(handle)
	assert.True(t, conn.closed)
	assert.Equal(t, 0, reg.Len())

	reg.Discard(handle) // no panic, no-op
}

func TestConnRegistry_PerPluginCap(t *testing.T) {
	t.Parallel()
	reg := NewConnRegistry(2)

	_, err := reg.Register(&fakeConn{}, "p1", time.Time{})
	require.NoError(t, err)
	_, err = reg.Register(&fakeConn{}, "p1", time.Time{})
	require.NoError(t, err)

	_, err = reg.Register(&fakeConn{}, "p1", time.Time{})
	assert.ErrorIs(t, err, ErrTooManyConns)

	// A different plugin still gets its own budget.
	_, err = reg.Register(&fakeConn{}, "p2", time.Time{})
	assert.NoError(t, err)
}
