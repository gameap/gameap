package grpc

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/stats"
)

func newBufferLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

func decodeLogLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	var lines []map[string]any

	for raw := range bytes.SplitSeq(buf.Bytes(), []byte("\n")) {
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}

		var entry map[string]any
		require.NoError(t, json.Unmarshal(raw, &entry))
		lines = append(lines, entry)
	}

	return lines
}

func TestConnectionLogger_TagConn_StoresMetadata(t *testing.T) {
	t.Parallel()
	handler := NewConnectionLogger(slog.Default())

	remote := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 54321}
	local := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 31718}

	ctx := handler.TagConn(context.Background(), &stats.ConnTagInfo{
		RemoteAddr: remote,
		LocalAddr:  local,
	})

	cc, ok := ctx.Value(connContextKey{}).(*connContext)
	require.True(t, ok, "connContext must be stored in context")
	assert.Equal(t, remote.String(), cc.remoteAddr)
	assert.Equal(t, local.String(), cc.localAddr)
	assert.False(t, cc.startedAt.IsZero())
}

func TestConnectionLogger_TagConn_NilAddrs(t *testing.T) {
	t.Parallel()
	handler := NewConnectionLogger(slog.Default())

	ctx := handler.TagConn(context.Background(), &stats.ConnTagInfo{})

	cc, ok := ctx.Value(connContextKey{}).(*connContext)
	require.True(t, ok)
	assert.Equal(t, "unknown", cc.remoteAddr)
	assert.Equal(t, "unknown", cc.localAddr)
}

func TestConnectionLogger_HandleConn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		event        stats.ConnStats
		wantMessage  string
		wantDuration bool
	}{
		{
			name:        "conn_begin_logs_opened",
			event:       &stats.ConnBegin{},
			wantMessage: "gRPC connection opened",
		},
		{
			name:         "conn_end_logs_closed_with_duration",
			event:        &stats.ConnEnd{},
			wantMessage:  "gRPC connection closed",
			wantDuration: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			logger := newBufferLogger(&buf)
			handler := NewConnectionLogger(logger)

			ctx := handler.TagConn(context.Background(), &stats.ConnTagInfo{
				RemoteAddr: &net.TCPAddr{IP: net.ParseIP("10.0.0.5"), Port: 4444},
				LocalAddr:  &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 31718},
			})

			handler.HandleConn(ctx, tt.event)

			entries := decodeLogLines(t, &buf)
			require.Len(t, entries, 1)

			entry := entries[0]
			assert.Equal(t, tt.wantMessage, entry["msg"])
			assert.Equal(t, "INFO", entry["level"])
			assert.Equal(t, "10.0.0.5:4444", entry["peer"])
			assert.Equal(t, "127.0.0.1:31718", entry["local"])

			if tt.wantDuration {
				_, hasDuration := entry["duration_ms"]
				assert.True(t, hasDuration, "ConnEnd entry must include duration_ms")
			}
		})
	}
}

func TestConnectionLogger_HandleConn_NoContextSkipped(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	handler := NewConnectionLogger(newBufferLogger(&buf))

	handler.HandleConn(context.Background(), &stats.ConnBegin{})

	assert.Empty(t, decodeLogLines(t, &buf))
}

func TestConnectionLogger_HandleRPC_NoOp(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	handler := NewConnectionLogger(newBufferLogger(&buf))

	ctx := handler.TagRPC(context.Background(), &stats.RPCTagInfo{})
	assert.NotNil(t, ctx)

	handler.HandleRPC(ctx, &stats.Begin{})
	assert.Empty(t, buf.Bytes())
}
