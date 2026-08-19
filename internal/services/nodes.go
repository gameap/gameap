package services

import (
	"context"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/pkg/errors"
)

var (
	ErrNodeNotFound = errors.New("node not found")
	// ErrNodeHasServers guards the delete: dropping a node under running game
	// servers would orphan them, so the caller must move or delete them first.
	ErrNodeHasServers = errors.New("cannot delete node with existing game servers")
)

// NodeService holds the node write logic shared by the HTTP API and the
// gameap-nodes host library, so a rule added on one path cannot go missing on
// the other.
type NodeService struct {
	nodes   repositories.NodeRepository
	servers repositories.ServerRepository
}

func NewNodeService(nodes repositories.NodeRepository, servers repositories.ServerRepository) *NodeService {
	return &NodeService{
		nodes:   nodes,
		servers: servers,
	}
}

func (s *NodeService) Get(ctx context.Context, id uint) (*domain.Node, error) {
	nodes, err := s.nodes.Find(ctx, filters.FindNodeByIDs(id), nil, &filters.Pagination{Limit: 1})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find node")
	}

	if len(nodes) == 0 {
		return nil, ErrNodeNotFound
	}

	return &nodes[0], nil
}

// Patch validates and applies a partial update. The node is read first and
// saved whole, so fields the patch cannot express keep their stored values.
func (s *NodeService) Patch(ctx context.Context, id uint, patch domain.NodePatch) (*domain.Node, error) {
	if err := patch.Validate(); err != nil {
		return nil, err
	}

	node, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	patch.ApplyTo(node)

	if err := s.nodes.Save(ctx, node); err != nil {
		return nil, errors.WithMessage(err, "failed to save node")
	}

	return node, nil
}

// SoftDelete marks the node deleted without dropping the row, mirroring the
// admin API. NodeRepository.Delete is a hard delete and is deliberately not
// used here.
func (s *NodeService) SoftDelete(ctx context.Context, id uint) error {
	node, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	hasServers, err := s.servers.Exists(ctx, &filters.FindServer{DSIDs: []uint{id}})
	if err != nil {
		return errors.WithMessage(err, "failed to check for associated servers")
	}

	if hasServers {
		return ErrNodeHasServers
	}

	now := time.Now()
	node.DeletedAt = &now

	if err := s.nodes.Save(ctx, node); err != nil {
		return errors.WithMessage(err, "failed to delete node")
	}

	return nil
}
