package grpc

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAdapter(buf *bytes.Buffer, verbosity int) *grpclogAdapter {
	return &grpclogAdapter{
		logger:    newBufferLogger(buf).With("component", "grpc-internal"),
		verbosity: verbosity,
	}
}

func TestGrpclogAdapter_Info(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		call    func(a *grpclogAdapter)
		wantMsg string
	}{
		{
			name:    "info_sprint",
			call:    func(a *grpclogAdapter) { a.Info("hello", " ", "world") },
			wantMsg: "hello world",
		},
		{
			name:    "infoln_strips_newline",
			call:    func(a *grpclogAdapter) { a.Infoln("hello") },
			wantMsg: "hello",
		},
		{
			name:    "infof_formats",
			call:    func(a *grpclogAdapter) { a.Infof("value=%d", 42) },
			wantMsg: "value=42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			a := newTestAdapter(&buf, 0)

			tt.call(a)

			entries := decodeLogLines(t, &buf)
			require.Len(t, entries, 1)
			assert.Equal(t, tt.wantMsg, entries[0]["msg"])
			assert.Equal(t, "INFO", entries[0]["level"])
			assert.Equal(t, "grpc-internal", entries[0]["component"])
		})
	}
}

func TestGrpclogAdapter_Warning(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	a := newTestAdapter(&buf, 0)

	a.Warning("careful")
	a.Warningln("also careful")
	a.Warningf("value=%s", "x")

	entries := decodeLogLines(t, &buf)
	require.Len(t, entries, 3)

	for _, entry := range entries {
		assert.Equal(t, "WARN", entry["level"])
	}

	assert.Equal(t, "careful", entries[0]["msg"])
	assert.Equal(t, "also careful", entries[1]["msg"])
	assert.Equal(t, "value=x", entries[2]["msg"])
}

func TestGrpclogAdapter_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	a := newTestAdapter(&buf, 0)

	a.Error("boom")
	a.Errorln("line boom")
	a.Errorf("err=%d", 500)

	entries := decodeLogLines(t, &buf)
	require.Len(t, entries, 3)

	for _, entry := range entries {
		assert.Equal(t, "ERROR", entry["level"])
	}

	assert.Equal(t, "boom", entries[0]["msg"])
	assert.Equal(t, "line boom", entries[1]["msg"])
	assert.Equal(t, "err=500", entries[2]["msg"])
}

func TestGrpclogAdapter_V(t *testing.T) {
	t.Parallel()
	a := newTestAdapter(&bytes.Buffer{}, 1)

	assert.True(t, a.V(0))
	assert.True(t, a.V(1))
	assert.False(t, a.V(2))
}

func TestTrimNewline(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "hello", trimNewline("hello\n"))
	assert.Equal(t, "hello", trimNewline("hello\n\n"))
	assert.Equal(t, "hello", trimNewline("hello"))
	assert.Equal(t, "", trimNewline("\n"))
}
