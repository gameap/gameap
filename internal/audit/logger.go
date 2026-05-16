package audit

import (
	"context"
	"log/slog"
)

const (
	// logMessage is the constant slog message for every audit record; the
	// structured attributes carry the detail.
	logMessage = "audit"

	// componentAttr tags every record so operators can split the audit
	// stream out of the general application log (and, later, forward it).
	componentAttr  = "component"
	componentValue = "audit"
)

type slogLogger struct {
	logger *slog.Logger
}

// NewLogger returns a Logger that emits structured records via the given
// slog.Logger, falling back to slog.Default() when logger is nil.
func NewLogger(logger *slog.Logger) Logger {
	if logger == nil {
		logger = slog.Default()
	}

	return &slogLogger{logger: logger}
}

func (l *slogLogger) Record(ctx context.Context, e Event) {
	level := slog.LevelInfo
	switch e.Outcome {
	case OutcomeFailure, OutcomeDenied, OutcomeBlocked:
		level = slog.LevelWarn
	case OutcomeSuccess:
		level = slog.LevelInfo
	}

	attrs := make([]slog.Attr, 0, 16+len(e.Extra))
	attrs = append(attrs,
		slog.String(componentAttr, componentValue),
		slog.String("event_type", string(e.Type)),
	)
	attrs = appendNonEmpty(attrs, "category", string(e.Category))
	attrs = appendNonEmpty(attrs, "outcome", string(e.Outcome))
	if e.ActorID != 0 {
		attrs = append(attrs, slog.Uint64("actor_id", uint64(e.ActorID)))
	}
	attrs = appendNonEmpty(attrs, "actor_login", e.ActorLogin)
	attrs = appendNonEmpty(attrs, "auth_method", string(e.AuthMethod))
	attrs = appendNonEmpty(attrs, "resource_type", e.ResourceType)
	attrs = appendNonEmpty(attrs, "resource_id", e.ResourceID)
	attrs = appendNonEmpty(attrs, "action", e.Action)
	attrs = appendNonEmpty(attrs, "reason", e.Reason)

	if info := RequestInfoFromContext(ctx); info != nil {
		attrs = appendNonEmpty(attrs, "request_id", info.RequestID)
		attrs = appendNonEmpty(attrs, "ip", info.IP)
		attrs = appendNonEmpty(attrs, "user_agent", info.UserAgent)
		attrs = appendNonEmpty(attrs, "http_method", info.Method)
		attrs = appendNonEmpty(attrs, "path", info.Path)
	}

	attrs = append(attrs, e.Extra...)

	l.logger.LogAttrs(ctx, level, logMessage, attrs...)
}

func appendNonEmpty(attrs []slog.Attr, key, value string) []slog.Attr {
	if value == "" {
		return attrs
	}

	return append(attrs, slog.String(key, value))
}

// NopLogger discards every event. It is the safe default when audit logging
// is disabled and in tests that do not assert on audit output.
type NopLogger struct{}

func (NopLogger) Record(context.Context, Event) {}
