package postunsuspend

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
)

type unsuspender interface {
	Unsuspend(ctx context.Context, server *domain.Server) error
}
