package hostlibrary

import (
	"context"
	"log/slog"

	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/log"
	"github.com/tetratelabs/wazero"
)

type LogServiceImpl struct {
	logger *slog.Logger
}

func NewLogService(logger *slog.Logger) *LogServiceImpl {
	return &LogServiceImpl{
		logger: logger,
	}
}

func (s *LogServiceImpl) Log(ctx context.Context, req *log.LogRequest) (*log.LogResponse, error) {
	attrs := make([]slog.Attr, 0, len(req.Fields))
	for k, v := range req.Fields {
		attrs = append(attrs, slog.String(k, v))
	}

	switch req.Level {
	case "debug":
		s.logger.LogAttrs(ctx, slog.LevelDebug, req.Message, attrs...)
	case "info":
		s.logger.LogAttrs(ctx, slog.LevelInfo, req.Message, attrs...)
	case "warn":
		s.logger.LogAttrs(ctx, slog.LevelWarn, req.Message, attrs...)
	case "error":
		s.logger.LogAttrs(ctx, slog.LevelError, req.Message, attrs...)
	default:
		s.logger.LogAttrs(ctx, slog.LevelInfo, req.Message, attrs...)
	}

	return &log.LogResponse{}, nil
}

type LogHostLibrary struct {
	impl *LogServiceImpl
}

func NewLogHostLibrary(logger *slog.Logger) *LogHostLibrary {
	return &LogHostLibrary{
		impl: NewLogService(logger),
	}
}

func (l *LogHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return log.Instantiate(ctx, r, l.impl)
}

// LogHostLibraryFactory builds per-plugin log libraries so every log line
// carries the plugin it came from.
type LogHostLibraryFactory struct {
	logger *slog.Logger
}

func NewLogHostLibraryFactory(logger *slog.Logger) *LogHostLibraryFactory {
	return &LogHostLibraryFactory{logger: logger}
}

func (f *LogHostLibraryFactory) Create(pluginID uint64) pkgplugin.HostLibrary {
	logger := f.logger
	if pluginID != 0 {
		logger = logger.With(slog.Uint64("plugin_id", pluginID))
	}

	return NewLogHostLibrary(logger)
}
