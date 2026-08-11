package hash

import (
	"context"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/proto"
)

type fileHasher interface {
	Hash(
		ctx context.Context, node *domain.Node, paths []string, algorithm proto.HashAlgorithm,
	) (*proto.HashResult, error)
}
