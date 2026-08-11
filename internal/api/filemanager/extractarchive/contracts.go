package extractarchive

import (
	"context"

	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
)

type archiveStarter interface {
	StartExtract(ctx context.Context, node *domain.Node, p daemon.ExtractArchiveParams) (string, error)
}
