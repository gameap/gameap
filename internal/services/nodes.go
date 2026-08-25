package services

import (
	"context"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
)

var (
	ErrNodeNotFound = errors.New("node not found")
	// ErrNodeHasServers guards the delete: dropping a node under running game
	// servers would orphan them, so the caller must move or delete them first.
	ErrNodeHasServers = errors.New("cannot delete node with existing game servers")
	// ErrNodeDeleteCancelledByPlugin reports a NODE_PRE_DELETE veto. It is the
	// node counterpart of servercontrol.ErrCancelledByPlugin, declared here
	// because servercontrol's tests import this package.
	ErrNodeDeleteCancelledByPlugin = errors.New("node deletion cancelled by plugin")
)

// NodeService holds the node write logic shared by the HTTP API and the
// gameap-nodes host library, so a rule added on one path cannot go missing on
// the other.
type NodeService struct {
	nodes            repositories.NodeRepository
	servers          repositories.ServerRepository
	pluginDispatcher NodePluginDispatcher
}

// NodeServiceOption tunes a NodeService.
type NodeServiceOption func(*NodeService)

// WithNodePluginEvents makes SoftDelete fire the NODE_PRE_DELETE veto and the
// NODE_DELETED notification, the pair the admin delete handler already sends.
func WithNodePluginEvents(dispatcher NodePluginDispatcher) NodeServiceOption {
	return func(s *NodeService) {
		s.pluginDispatcher = dispatcher
	}
}

func NewNodeService(
	nodes repositories.NodeRepository,
	servers repositories.ServerRepository,
	opts ...NodeServiceOption,
) *NodeService {
	service := &NodeService{
		nodes:   nodes,
		servers: servers,
	}

	for _, opt := range opts {
		opt(service)
	}

	return service
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

	if err := patch.ApplyTo(node); err != nil {
		return nil, err
	}

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

	if err := s.dispatchPreDelete(ctx, node); err != nil {
		return err
	}

	now := time.Now()
	node.DeletedAt = &now

	if err := s.nodes.Save(ctx, node); err != nil {
		return errors.WithMessage(err, "failed to delete node")
	}

	if s.pluginDispatcher != nil {
		s.pluginDispatcher.DispatchNodeEventAsync(ctx, pluginproto.EventType_EVENT_TYPE_NODE_DELETED, node, nil)
	}

	return nil
}

// dispatchPreDelete gives plugins a chance to cancel the deletion, mirroring
// the admin delete handler.
func (s *NodeService) dispatchPreDelete(ctx context.Context, node *domain.Node) error {
	if s.pluginDispatcher == nil {
		return nil
	}

	result := s.pluginDispatcher.DispatchNodeEvent(
		ctx, pluginproto.EventType_EVENT_TYPE_NODE_PRE_DELETE, node, nil)
	if !result.Cancelled {
		return nil
	}

	reason := "cancelled by " + result.CancelledBy
	if result.CancelMessage != "" {
		reason += ": " + result.CancelMessage
	}

	return errors.Wrap(ErrNodeDeleteCancelledByPlugin, reason)
}
