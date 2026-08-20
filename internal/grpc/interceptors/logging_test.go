package interceptors

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type logLine struct {
	Level      string `json:"level"`
	Msg        string `json:"msg"`
	Method     string `json:"method"`
	Code       string `json:"code"`
	DurationMS int64  `json:"duration_ms"`
	Peer       string `json:"peer"`
}

func parseLines(t *testing.T, buf *bytes.Buffer) []logLine {
	t.Helper()
	var out []logLine
	sc := bufio.NewScanner(buf)
	for sc.Scan() {
		var l logLine
		require.NoError(t, json.Unmarshal(sc.Bytes(), &l))
		out = append(out, l)
	}

	return out
}

func newCapturingLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestLoggingInterceptor_Unary_OK_LogsDebug(t *testing.T) {
	t.Parallel()
	// ARRANGE
	buf := &bytes.Buffer{}
	interceptor := NewLoggingInterceptor(newCapturingLogger(buf))
	info := &grpc.UnaryServerInfo{FullMethod: "/gameap.DaemonGateway/Ping"}
	handler := func(_ context.Context, _ any) (any, error) {
		return "ok", nil
	}

	// ACT
	resp, err := interceptor.UnaryServerInterceptor()(context.Background(), "req", info, handler)

	// ASSERT
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)

	lines := parseLines(t, buf)
	require.Len(t, lines, 2, "expected start and complete log lines")
	assert.Equal(t, "DEBUG", lines[0].Level)
	assert.Equal(t, "gRPC request started", lines[0].Msg)
	assert.Equal(t, "/gameap.DaemonGateway/Ping", lines[0].Method)
	assert.Equal(t, "DEBUG", lines[1].Level)
	assert.Equal(t, "gRPC request completed", lines[1].Msg)
	assert.Equal(t, "/gameap.DaemonGateway/Ping", lines[1].Method)
	assert.Equal(t, "OK", lines[1].Code)
	assert.GreaterOrEqual(t, lines[1].DurationMS, int64(0))
}

func TestLoggingInterceptor_Unary_Error_LogsWarn(t *testing.T) {
	t.Parallel()
	// ARRANGE
	buf := &bytes.Buffer{}
	interceptor := NewLoggingInterceptor(newCapturingLogger(buf))
	info := &grpc.UnaryServerInfo{FullMethod: "/gameap.DaemonGateway/Get"}
	handler := func(_ context.Context, _ any) (any, error) {
		return nil, status.Error(codes.NotFound, "x")
	}

	// ACT
	resp, err := interceptor.UnaryServerInterceptor()(context.Background(), "req", info, handler)

	// ASSERT
	require.Error(t, err)
	assert.Nil(t, resp)

	lines := parseLines(t, buf)
	require.Len(t, lines, 2)
	assert.Equal(t, "DEBUG", lines[0].Level, "start line should remain DEBUG")
	assert.Equal(t, "WARN", lines[1].Level, "completion with non-OK code should log WARN")
	assert.Equal(t, "NotFound", lines[1].Code)
}

func TestLoggingInterceptor_Unary_PeerUnknown_WhenMissing(t *testing.T) {
	t.Parallel()
	// ARRANGE
	buf := &bytes.Buffer{}
	interceptor := NewLoggingInterceptor(newCapturingLogger(buf))
	info := &grpc.UnaryServerInfo{FullMethod: "/gameap.DaemonGateway/Ping"}
	handler := func(_ context.Context, _ any) (any, error) {
		return "ok", nil
	}

	// ACT
	_, err := interceptor.UnaryServerInterceptor()(context.Background(), "req", info, handler)

	// ASSERT
	require.NoError(t, err)
	lines := parseLines(t, buf)
	require.Len(t, lines, 2)
	assert.Equal(t, "unknown", lines[0].Peer, "peer must default to 'unknown' when no peer.Peer is in context")
	assert.Equal(t, "unknown", lines[1].Peer)
}

func TestLoggingInterceptor_Unary_PeerAddrSet(t *testing.T) {
	t.Parallel()
	// ARRANGE
	buf := &bytes.Buffer{}
	interceptor := NewLoggingInterceptor(newCapturingLogger(buf))
	info := &grpc.UnaryServerInfo{FullMethod: "/gameap.DaemonGateway/Ping"}
	handler := func(_ context.Context, _ any) (any, error) {
		return "ok", nil
	}

	addr := &net.IPAddr{IP: net.IPv4(10, 0, 0, 1)}
	ctx := peer.NewContext(context.Background(), &peer.Peer{Addr: addr})

	// ACT
	_, err := interceptor.UnaryServerInterceptor()(ctx, "req", info, handler)

	// ASSERT
	require.NoError(t, err)
	lines := parseLines(t, buf)
	require.Len(t, lines, 2)
	assert.Equal(t, addr.String(), lines[0].Peer, "peer field must reflect peer.Peer.Addr.String()")
	assert.Equal(t, addr.String(), lines[1].Peer)
}

func TestLoggingInterceptor_Stream_OK_LogsDebug(t *testing.T) {
	t.Parallel()
	// ARRANGE
	buf := &bytes.Buffer{}
	interceptor := NewLoggingInterceptor(newCapturingLogger(buf))
	info := &grpc.StreamServerInfo{FullMethod: "/gameap.DaemonGateway/Stream"}
	handler := func(_ any, _ grpc.ServerStream) error {
		return nil
	}
	ss := &stubStream{ctx: context.Background()}

	// ACT
	err := interceptor.StreamServerInterceptor()(nil, ss, info, handler)

	// ASSERT
	require.NoError(t, err)
	lines := parseLines(t, buf)
	require.Len(t, lines, 2)
	assert.Equal(t, "DEBUG", lines[0].Level)
	assert.Equal(t, "gRPC stream started", lines[0].Msg)
	assert.Equal(t, "DEBUG", lines[1].Level)
	assert.Equal(t, "gRPC stream ended", lines[1].Msg)
	assert.Equal(t, "OK", lines[1].Code)
}

func TestLoggingInterceptor_Stream_Error_LogsWarn(t *testing.T) {
	t.Parallel()
	// ARRANGE
	buf := &bytes.Buffer{}
	interceptor := NewLoggingInterceptor(newCapturingLogger(buf))
	info := &grpc.StreamServerInfo{FullMethod: "/gameap.DaemonGateway/Stream"}
	handler := func(_ any, _ grpc.ServerStream) error {
		return status.Error(codes.Unavailable, "x")
	}
	ss := &stubStream{ctx: context.Background()}

	// ACT
	err := interceptor.StreamServerInterceptor()(nil, ss, info, handler)

	// ASSERT
	require.Error(t, err)
	lines := parseLines(t, buf)
	require.Len(t, lines, 2)
	assert.Equal(t, "WARN", lines[1].Level, "completion with non-OK, non-Canceled code should log WARN")
	assert.Equal(t, "Unavailable", lines[1].Code)
}

func TestLoggingInterceptor_Stream_Canceled_NotWarn(t *testing.T) {
	t.Parallel()
	// ARRANGE
	buf := &bytes.Buffer{}
	interceptor := NewLoggingInterceptor(newCapturingLogger(buf))
	info := &grpc.StreamServerInfo{FullMethod: "/gameap.DaemonGateway/Stream"}
	handler := func(_ any, _ grpc.ServerStream) error {
		return status.Error(codes.Canceled, "x")
	}
	ss := &stubStream{ctx: context.Background()}

	// ACT
	err := interceptor.StreamServerInterceptor()(nil, ss, info, handler)

	// ASSERT
	require.Error(t, err)
	lines := parseLines(t, buf)
	require.Len(t, lines, 2)
	assert.Equal(t, "DEBUG", lines[1].Level, "Canceled is treated as non-warn — client cancellations are routine")
	assert.Equal(t, "Canceled", lines[1].Code)
}

func TestNewLoggingInterceptor_NilLoggerUsesDefault(t *testing.T) {
	t.Parallel()
	// ARRANGE & ACT
	interceptor := NewLoggingInterceptor(nil)

	// ASSERT
	require.NotNil(t, interceptor)
	require.NotNil(t, interceptor.UnaryServerInterceptor())
	require.NotNil(t, interceptor.StreamServerInterceptor())

	// Sanity check: invoking the unary interceptor with nil logger must not panic.
	info := &grpc.UnaryServerInfo{FullMethod: "/x/y"}
	handler := func(_ context.Context, _ any) (any, error) { return "ok", nil }
	resp, err := interceptor.UnaryServerInterceptor()(context.Background(), "req", info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
}
