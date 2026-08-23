package plugin

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestGuestLogWriter(level slog.Level, stream string) (*guestLogWriter, *bytes.Buffer) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	writer := newGuestLogWriter(logger, level, stream)
	writer.SetPluginID("tester")

	return writer, &buf
}

func logLines(buf *bytes.Buffer) []string {
	text := strings.TrimSpace(buf.String())
	if text == "" {
		return nil
	}

	return strings.Split(text, "\n")
}

func TestGuestLogWriter_splits_lines_across_writes(t *testing.T) {
	t.Parallel()
	writer, buf := newTestGuestLogWriter(slog.LevelDebug, "stdout")

	n, err := writer.Write([]byte("first\nsec"))
	require.NoError(t, err)
	assert.Equal(t, 9, n)

	_, err = writer.Write([]byte("ond\r\nthird"))
	require.NoError(t, err)

	lines := logLines(buf)
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0], "line=first")
	assert.Contains(t, lines[0], "stream=stdout")
	assert.Contains(t, lines[0], "plugin_id=tester")
	assert.Contains(t, lines[0], "level=DEBUG")
	assert.Contains(t, lines[1], "line=second")

	writer.Flush()

	lines = logLines(buf)
	require.Len(t, lines, 3)
	assert.Contains(t, lines[2], "line=third")

	writer.Flush()
	assert.Len(t, logLines(buf), 3, "a second flush has nothing to emit")
}

func TestGuestLogWriter_stderr_is_warn_and_skips_blank_lines(t *testing.T) {
	t.Parallel()
	writer, buf := newTestGuestLogWriter(slog.LevelWarn, "stderr")

	_, err := writer.Write([]byte("\n\npanic: boom\n\n"))
	require.NoError(t, err)

	lines := logLines(buf)
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "level=WARN")
	assert.Contains(t, lines[0], "stream=stderr")
	assert.Contains(t, lines[0], `line="panic: boom"`)
}

func TestGuestLogWriter_truncates_long_lines(t *testing.T) {
	t.Parallel()
	writer, buf := newTestGuestLogWriter(slog.LevelDebug, "stdout")

	_, err := writer.Write(bytes.Repeat([]byte("a"), guestLogMaxLineBytes))
	require.NoError(t, err)
	_, err = writer.Write([]byte("tail that is dropped\nnext\n"))
	require.NoError(t, err)

	lines := logLines(buf)
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0], "truncated=true")
	assert.NotContains(t, lines[0], "tail that is dropped")
	assert.Contains(t, lines[0], strings.Repeat("a", guestLogMaxLineBytes))
	assert.Contains(t, lines[1], "line=next")
	assert.NotContains(t, lines[1], "truncated")
}

func TestGuestLogWriter_rate_limits_and_reports_drops(t *testing.T) {
	t.Parallel()
	writer, buf := newTestGuestLogWriter(slog.LevelDebug, "stdout")

	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	writer.now = func() time.Time { return now }

	for range guestLogRateLines + 5 {
		_, err := writer.Write([]byte("spam\n"))
		require.NoError(t, err)
	}

	lines := logLines(buf)
	require.Len(t, lines, guestLogRateLines)

	now = now.Add(guestLogRateWindow)

	_, err := writer.Write([]byte("after window\n"))
	require.NoError(t, err)

	lines = logLines(buf)
	require.Len(t, lines, guestLogRateLines+2)
	assert.Contains(t, lines[guestLogRateLines], "lines dropped")
	assert.Contains(t, lines[guestLogRateLines], "dropped=5")
	assert.Contains(t, lines[guestLogRateLines], "level=WARN")
	assert.Contains(t, lines[guestLogRateLines+1], `line="after window"`)
}

func TestGuestLogWriter_skips_buffering_when_level_disabled(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	writer := newGuestLogWriter(logger, slog.LevelDebug, "stdout")

	n, err := writer.Write([]byte("partial without newline"))
	require.NoError(t, err)
	assert.Equal(t, 23, n)
	assert.Empty(t, writer.partial)

	writer.Flush()
	assert.Empty(t, buf.String())
}

func TestGuestLogs_nil_is_safe(t *testing.T) {
	t.Parallel()
	var logs *guestLogs

	logs.SetPluginID("x")
	logs.Flush()

	n, err := logs.stdoutWriter().Write([]byte("discarded"))
	require.NoError(t, err)
	assert.Equal(t, 9, n)

	n, err = logs.stderrWriter().Write([]byte("discarded"))
	require.NoError(t, err)
	assert.Equal(t, 9, n)
}

func TestNewGuestLogs_defaults_to_slog_default(t *testing.T) {
	t.Parallel()
	logs := newGuestLogs(nil)

	require.NotNil(t, logs.stdout)
	require.NotNil(t, logs.stderr)
	assert.Equal(t, slog.LevelDebug, logs.stdout.level)
	assert.Equal(t, slog.LevelWarn, logs.stderr.level)
	assert.Equal(t, "stdout", logs.stdout.stream)
	assert.Equal(t, "stderr", logs.stderr.stream)
}
