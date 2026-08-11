//go:build wasip1

package main

import (
	"context"
	"log/slog"

	"github.com/gameap/gameap/pkg/plugin/sdk/nodefs"
)

// archiveEventsHandler demonstrates the archive event callbacks: a plugin
// that starts an operation with report_progress receives HandleArchiveProgress
// pushes and always a single HandleArchiveCompleted. Embedding
// EmptyArchiveEventsHandler would let a plugin override only one of them.
type archiveEventsHandler struct {
	nodefs.EmptyArchiveEventsHandler
}

func (h *archiveEventsHandler) HandleArchiveProgress(
	_ context.Context,
	req *nodefs.HandleArchiveProgressRequest,
) (*nodefs.HandleArchiveProgressResponse, error) {
	logger.Info("archive progress",
		slog.String("operation_id", req.OperationId),
		slog.Uint64("files_processed", uint64(req.FilesProcessed)),
		slog.Uint64("bytes_processed", req.BytesProcessed),
		slog.String("current_entry", req.CurrentEntry),
	)

	return &nodefs.HandleArchiveProgressResponse{}, nil
}

func (h *archiveEventsHandler) HandleArchiveCompleted(
	_ context.Context,
	req *nodefs.HandleArchiveCompletedRequest,
) (*nodefs.HandleArchiveCompletedResponse, error) {
	errMsg := ""
	if req.Error != nil {
		errMsg = *req.Error
	}

	logger.Info("archive completed",
		slog.String("operation_id", req.OperationId),
		slog.Bool("success", req.Success),
		slog.String("error", errMsg),
		slog.Uint64("files_processed", uint64(req.FilesProcessed)),
		slog.Uint64("archive_size", req.ArchiveSize),
	)

	return &nodefs.HandleArchiveCompletedResponse{}, nil
}
