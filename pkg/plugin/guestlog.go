package plugin

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// guestLogMaxLineBytes caps one guest line; the rest of a longer line is
	// dropped so a runaway print cannot grow the panel's log unboundedly.
	guestLogMaxLineBytes = 4096
	// guestLogRateLines per guestLogRateWindow is the per-stream budget;
	// lines above it are counted and reported once per window.
	guestLogRateLines  = 200
	guestLogRateWindow = 10 * time.Second
)

// guestLogWriter forwards one guest stream (stdout or stderr) to slog, line
// by line. Partial lines are kept until the next newline or a Flush, which
// the wrapper issues after every guest call.
type guestLogWriter struct {
	logger   *slog.Logger
	level    slog.Level
	stream   string
	pluginID atomic.Pointer[string]
	now      func() time.Time

	mu      sync.Mutex
	partial []byte
	// truncating is set once the current line exceeded the cap: the rest of
	// it is discarded up to the newline.
	truncating  bool
	windowStart time.Time
	windowLines int
	dropped     int
}

func newGuestLogWriter(logger *slog.Logger, level slog.Level, stream string) *guestLogWriter {
	return &guestLogWriter{
		logger: logger,
		level:  level,
		stream: stream,
		now:    time.Now,
	}
}

func (w *guestLogWriter) SetPluginID(pluginID string) {
	w.pluginID.Store(&pluginID)
}

// Write implements io.Writer for the WASI file descriptor; it never fails so
// the guest cannot observe the panel's logging state.
func (w *guestLogWriter) Write(p []byte) (int, error) {
	n := len(p)

	if !w.logger.Enabled(context.Background(), w.level) {
		return n, nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	for len(p) > 0 {
		newline := bytes.IndexByte(p, '\n')
		if newline < 0 {
			w.appendLocked(p)

			break
		}

		w.appendLocked(p[:newline])
		w.emitLocked()
		p = p[newline+1:]
	}

	return n, nil
}

// Flush emits a pending partial line.
func (w *guestLogWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.partial) == 0 && !w.truncating {
		return
	}

	w.emitLocked()
}

func (w *guestLogWriter) appendLocked(chunk []byte) {
	if w.truncating {
		return
	}

	room := guestLogMaxLineBytes - len(w.partial)
	if len(chunk) > room {
		w.partial = append(w.partial, chunk[:room]...)
		w.truncating = true

		return
	}

	w.partial = append(w.partial, chunk...)
}

func (w *guestLogWriter) emitLocked() {
	line := string(bytes.TrimRight(w.partial, "\r"))
	truncated := w.truncating
	w.partial = w.partial[:0]
	w.truncating = false

	if line == "" && !truncated {
		return
	}

	if !w.allowLocked() {
		return
	}

	attrs := []slog.Attr{
		slog.String("plugin_id", w.currentPluginID()),
		slog.String("stream", w.stream),
		slog.String("line", line),
	}
	if truncated {
		attrs = append(attrs, slog.Bool("truncated", true))
	}

	w.logger.LogAttrs(context.Background(), w.level, "plugin guest output", attrs...)
}

// allowLocked applies the fixed-window rate limit and reports the previous
// window's drops when a new window starts.
func (w *guestLogWriter) allowLocked() bool {
	now := w.now()

	if w.windowStart.IsZero() || now.Sub(w.windowStart) >= guestLogRateWindow {
		if w.dropped > 0 {
			w.logger.LogAttrs(context.Background(), slog.LevelWarn, "plugin guest output lines dropped",
				slog.String("plugin_id", w.currentPluginID()),
				slog.String("stream", w.stream),
				slog.Int("dropped", w.dropped),
			)
		}

		w.windowStart = now
		w.windowLines = 0
		w.dropped = 0
	}

	if w.windowLines >= guestLogRateLines {
		w.dropped++

		return false
	}

	w.windowLines++

	return true
}

func (w *guestLogWriter) currentPluginID() string {
	if id := w.pluginID.Load(); id != nil {
		return *id
	}

	return ""
}

// guestLogs holds both streams of one module. stdout is chatter (debug);
// stderr carries panics and runtime faults that are otherwise invisible.
type guestLogs struct {
	stdout *guestLogWriter
	stderr *guestLogWriter
}

func newGuestLogs(logger *slog.Logger) *guestLogs {
	if logger == nil {
		logger = slog.Default()
	}

	return &guestLogs{
		stdout: newGuestLogWriter(logger, slog.LevelDebug, "stdout"),
		stderr: newGuestLogWriter(logger, slog.LevelWarn, "stderr"),
	}
}

func (g *guestLogs) SetPluginID(pluginID string) {
	if g == nil {
		return
	}

	g.stdout.SetPluginID(pluginID)
	g.stderr.SetPluginID(pluginID)
}

func (g *guestLogs) Flush() {
	if g == nil {
		return
	}

	g.stdout.Flush()
	g.stderr.Flush()
}

func (g *guestLogs) stdoutWriter() io.Writer {
	if g == nil {
		return io.Discard
	}

	return g.stdout
}

func (g *guestLogs) stderrWriter() io.Writer {
	if g == nil {
		return io.Discard
	}

	return g.stderr
}
