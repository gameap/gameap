package putserver

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
)

type suspensionCompleter interface {
	AfterUpdate(ctx context.Context, server *domain.Server, wasSuspended bool) uint
}
