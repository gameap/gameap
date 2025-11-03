package postcommand

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
)

type serverManager interface {
	Start(ctx context.Context, server *domain.Server) (taskID uint, err error)
	Stop(ctx context.Context, server *domain.Server) (taskID uint, err error)
	Restart(ctx context.Context, server *domain.Server) (taskID uint, err error)
	Update(ctx context.Context, server *domain.Server) (taskID uint, err error)
	Install(ctx context.Context, server *domain.Server) (taskID uint, err error)
	Reinstall(ctx context.Context, server *domain.Server) (taskID uint, err error)
}
