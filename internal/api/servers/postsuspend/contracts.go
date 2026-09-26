package postsuspend

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
)

type suspender interface {
	Suspend(ctx context.Context, server *domain.Server, reason *string) (uint, error)
}
