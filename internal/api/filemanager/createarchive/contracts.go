package createarchive

import (
	"context"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
)

type archiveStarter interface {
	StartCreate(ctx context.Context, node *domain.Node, p daemon.CreateArchiveParams) (string, error)
}
