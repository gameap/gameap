package hostlibrary

import (
	"context"
	"net"
	"testing"
	"time"

	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	netsdk "github.com/gameap/gameap/pkg/plugin/sdk/net"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newNetService(t *testing.T, pluginID string, cfg NetConfig) (*NetServiceImpl, *pkgplugin.ConnRegistry) {
	t.Helper()

	reg := pkgplugin.NewConnRegistry(8)

	return &NetServiceImpl{pluginID: pluginID, registry: reg, cfg: cfg.withDefaults()}, reg
}

func TestNet_SendAndRecv(t *testing.T) {
	t.Parallel()
	svc, reg := newNetService(t, "p1", NetConfig{})
	local, remote := net.Pipe()
	t.Cleanup(func() { _ = remote.Close() })

	handle, err := reg.Register(local, "p1", time.Now().Add(time.Minute))
	require.NoError(t, err)

	go func() {
		buf := make([]byte, 5)
		_, _ = remote.Read(buf)
		_, _ = remote.Write([]byte("pong"))
	}()

	sendResp, err := svc.Send(context.Background(), &netsdk.NetSendRequest{Handle: handle, Data: []byte("hello")})
	require.NoError(t, err)
	assert.Nil(t, sendResp.Error)
	assert.Equal(t, int32(5), sendResp.Written)

	recvResp, err := svc.Recv(context.Background(), &netsdk.NetRecvRequest{Handle: handle, MaxBytes: 64, TimeoutMs: 1000})
	require.NoError(t, err)
	assert.Nil(t, recvResp.Error)
	assert.Equal(t, "pong", string(recvResp.Data))
}

func TestNet_RejectsForeignHandle(t *testing.T) {
	t.Parallel()
	svc, reg := newNetService(t, "attacker", NetConfig{})
	local, remote := net.Pipe()
	t.Cleanup(func() { _ = local.Close(); _ = remote.Close() })

	// Handle belongs to victim, but the service acts for "attacker".
	handle, err := reg.Register(local, "victim", time.Now().Add(time.Minute))
	require.NoError(t, err)

	sendResp, err := svc.Send(context.Background(), &netsdk.NetSendRequest{Handle: handle, Data: []byte("x")})
	require.NoError(t, err)
	require.NotNil(t, sendResp.Error)
	assert.Contains(t, *sendResp.Error, "not found")

	recvResp, err := svc.Recv(context.Background(), &netsdk.NetRecvRequest{Handle: handle, MaxBytes: 8})
	require.NoError(t, err)
	require.NotNil(t, recvResp.Error)
}

func TestNet_RecvClampsMaxBytes(t *testing.T) {
	t.Parallel()
	svc, reg := newNetService(t, "p1", NetConfig{MaxReadBytes: 4})
	local, remote := net.Pipe()
	t.Cleanup(func() { _ = remote.Close() })

	handle, err := reg.Register(local, "p1", time.Now().Add(time.Minute))
	require.NoError(t, err)

	go func() { _, _ = remote.Write([]byte("abcdefghij")) }()

	// Requests 64 but the cap is 4, so at most 4 bytes come back.
	recvResp, err := svc.Recv(context.Background(), &netsdk.NetRecvRequest{Handle: handle, MaxBytes: 64, TimeoutMs: 1000})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(recvResp.Data), 4)
	assert.NotEmpty(t, recvResp.Data)
}

func TestNet_RecvTimeout(t *testing.T) {
	t.Parallel()
	svc, reg := newNetService(t, "p1", NetConfig{})
	local, remote := net.Pipe()
	t.Cleanup(func() { _ = local.Close(); _ = remote.Close() })

	handle, err := reg.Register(local, "p1", time.Now().Add(time.Minute))
	require.NoError(t, err)

	// Nothing is written to the peer, so the read must time out.
	recvResp, err := svc.Recv(context.Background(), &netsdk.NetRecvRequest{Handle: handle, MaxBytes: 8, TimeoutMs: 50})
	require.NoError(t, err)
	assert.True(t, recvResp.Timeout)
	assert.Nil(t, recvResp.Error)
}

func TestNet_CloseRemovesHandle(t *testing.T) {
	t.Parallel()
	svc, reg := newNetService(t, "p1", NetConfig{})
	local, remote := net.Pipe()
	t.Cleanup(func() { _ = remote.Close() })

	handle, err := reg.Register(local, "p1", time.Now().Add(time.Minute))
	require.NoError(t, err)

	closeResp, err := svc.Close(context.Background(), &netsdk.NetCloseRequest{Handle: handle})
	require.NoError(t, err)
	assert.Nil(t, closeResp.Error)
	assert.Equal(t, 0, reg.Len())

	// Subsequent I/O on the closed handle is rejected.
	recvResp, err := svc.Recv(context.Background(), &netsdk.NetRecvRequest{Handle: handle, MaxBytes: 8})
	require.NoError(t, err)
	assert.NotNil(t, recvResp.Error)
}
