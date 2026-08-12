package cancelarchive

import (
	"context"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
)

type archiveCanceler interface {
	Cancel(ctx context.Context, node *domain.Node, operationID, reason string) error
	GetSnapshot(operationID string) (daemon.ArchiveOpSnapshot, bool)
}
