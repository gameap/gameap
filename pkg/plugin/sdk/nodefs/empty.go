package nodefs

import (
	"context"
)

// EmptyArchiveEventsHandler provides no-op defaults for the
// ArchiveEventsHandler interface. Plugins embed it and override only the
// callbacks they need — for example a fire-and-forget archiver only needs
// HandleArchiveCompleted.
type EmptyArchiveEventsHandler struct{}

func (EmptyArchiveEventsHandler) HandleArchiveProgress(
	context.Context,
	*HandleArchiveProgressRequest,
) (*HandleArchiveProgressResponse, error) {
	return &HandleArchiveProgressResponse{}, nil
}

func (EmptyArchiveEventsHandler) HandleArchiveCompleted(
	context.Context,
	*HandleArchiveCompletedRequest,
) (*HandleArchiveCompletedResponse, error) {
	return &HandleArchiveCompletedResponse{}, nil
}
