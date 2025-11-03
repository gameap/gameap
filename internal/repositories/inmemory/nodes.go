package inmemory

import (
	"cmp"
	"context"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/samber/lo"
)

type NodeRepository struct {
	mu      sync.RWMutex
	nodes   map[uint]*domain.Node
	nextID  uint32
	idIndex map[uint]map[uint]struct{} // id -> nodeIDs
}

func NewNodeRepository() *NodeRepository {
	return &NodeRepository{
		nodes:   make(map[uint]*domain.Node),
		idIndex: make(map[uint]map[uint]struct{}),
	}
}

func (r *NodeRepository) FindAll(
	_ context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Node, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodes := make([]domain.Node, 0, len(r.nodes))
	for _, node := range r.nodes {
		// Skip soft-deleted nodes
		if node.DeletedAt != nil {
			continue
		}
		nodes = append(nodes, *node)
	}

	r.sortNodes(nodes, order)

	return r.applyPagination(nodes, pagination), nil
}

func (r *NodeRepository) Find(
	_ context.Context,
	filter *filters.FindNode,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Node, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if filter == nil {
		filter = &filters.FindNode{}
	}

	// Use hash indexes for efficient filtering
	candidateIDs := r.getFilteredNodeIDs(filter)

	nodes := make([]domain.Node, 0, len(candidateIDs))
	for nodeID := range candidateIDs {
		if node, exists := r.nodes[nodeID]; exists {
			nodes = append(nodes, *node)
		}
	}

	r.sortNodes(nodes, order)

	return r.applyPagination(nodes, pagination), nil
}

func (r *NodeRepository) Save(_ context.Context, node *domain.Node) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if node.UpdatedAt == nil || node.UpdatedAt.IsZero() {
		node.UpdatedAt = lo.ToPtr(time.Now())
	}

	if node.ID == 0 && (node.CreatedAt == nil || node.CreatedAt.IsZero()) {
		node.CreatedAt = lo.ToPtr(time.Now())
	}

	// Remove old indexes if updating existing node
	if node.ID != 0 {
		if oldNode, exists := r.nodes[node.ID]; exists {
			r.removeFromIndexes(oldNode)
		}
	} else {
		node.ID = uint(atomic.AddUint32(&r.nextID, 1))
	}

	// Save node (deep copy to prevent external modifications)
	r.nodes[node.ID] = &domain.Node{
		ID:                  node.ID,
		Enabled:             node.Enabled,
		Name:                node.Name,
		OS:                  node.OS,
		Location:            node.Location,
		Provider:            node.Provider,
		IPs:                 node.IPs,
		RAM:                 node.RAM,
		CPU:                 node.CPU,
		WorkPath:            node.WorkPath,
		SteamcmdPath:        node.SteamcmdPath,
		GdaemonHost:         node.GdaemonHost,
		GdaemonPort:         node.GdaemonPort,
		GdaemonAPIKey:       node.GdaemonAPIKey,
		GdaemonAPIToken:     node.GdaemonAPIToken,
		GdaemonLogin:        node.GdaemonLogin,
		GdaemonPassword:     node.GdaemonPassword,
		GdaemonServerCert:   node.GdaemonServerCert,
		ClientCertificateID: node.ClientCertificateID,
		PreferInstallMethod: node.PreferInstallMethod,
		ScriptInstall:       node.ScriptInstall,
		ScriptReinstall:     node.ScriptReinstall,
		ScriptUpdate:        node.ScriptUpdate,
		ScriptStart:         node.ScriptStart,
		ScriptPause:         node.ScriptPause,
		ScriptUnpause:       node.ScriptUnpause,
		ScriptStop:          node.ScriptStop,
		ScriptKill:          node.ScriptKill,
		ScriptRestart:       node.ScriptRestart,
		ScriptStatus:        node.ScriptStatus,
		ScriptStats:         node.ScriptStats,
		ScriptGetConsole:    node.ScriptGetConsole,
		ScriptSendCommand:   node.ScriptSendCommand,
		ScriptDelete:        node.ScriptDelete,
		CreatedAt:           node.CreatedAt,
		UpdatedAt:           node.UpdatedAt,
		DeletedAt:           node.DeletedAt,
	}

	// Add to indexes
	r.addToIndexes(r.nodes[node.ID])

	return nil
}

func (r *NodeRepository) Delete(_ context.Context, id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if node, exists := r.nodes[id]; exists {
		// Remove from indexes
		r.removeFromIndexes(node)
	}

	delete(r.nodes, id)

	return nil
}

func (r *NodeRepository) addToIndexes(node *domain.Node) {
	// ID index
	if r.idIndex[node.ID] == nil {
		r.idIndex[node.ID] = make(map[uint]struct{})
	}
	r.idIndex[node.ID][node.ID] = struct{}{}
}

func (r *NodeRepository) removeFromIndexes(node *domain.Node) {
	// ID index
	if nodeSet, exists := r.idIndex[node.ID]; exists {
		delete(nodeSet, node.ID)
		if len(nodeSet) == 0 {
			delete(r.idIndex, node.ID)
		}
	}
}

//nolint:gocognit
func (r *NodeRepository) getFilteredNodeIDs(filter *filters.FindNode) map[uint]struct{} {
	resultIDs := make(map[uint]struct{}, len(r.nodes))

	if filter == nil {
		// No filter, return all non-deleted node IDs
		for nodeID, node := range r.nodes {
			if node.DeletedAt == nil {
				resultIDs[nodeID] = struct{}{}
			}
		}

		return resultIDs
	}

	// Filter by IDs
	//
	//nolint:nestif
	if len(filter.IDs) > 0 {
		for _, id := range filter.IDs {
			if node, exists := r.nodes[id]; exists {
				if node.DeletedAt != nil && !filter.WithDeleted {
					continue
				}

				resultIDs[id] = struct{}{}
			}
		}
	} else {
		// No ID filter, return all nodes (respecting WithDeleted)
		for nodeID, node := range r.nodes {
			if !filter.WithDeleted && node.DeletedAt != nil {
				continue
			}
			resultIDs[nodeID] = struct{}{}
		}
	}

	// Filter by GDaemonAPIKey
	if filter.GDaemonAPIKey != nil {
		filtered := make(map[uint]struct{})
		for nodeID := range resultIDs {
			if node, exists := r.nodes[nodeID]; exists {
				if node.GdaemonAPIKey == *filter.GDaemonAPIKey {
					filtered[nodeID] = struct{}{}
				}
			}
		}
		resultIDs = filtered
	}

	// Filter by GDaemonAPIToken
	if filter.GDaemonAPIToken != nil {
		filtered := make(map[uint]struct{})
		for nodeID := range resultIDs {
			if node, exists := r.nodes[nodeID]; exists {
				if node.GdaemonAPIToken != nil && *node.GdaemonAPIToken == *filter.GDaemonAPIToken {
					filtered[nodeID] = struct{}{}
				}
			}
		}
		resultIDs = filtered
	}

	return resultIDs
}

func (r *NodeRepository) sortNodes(nodes []domain.Node, order []filters.Sorting) {
	if len(order) == 0 {
		sort.Slice(nodes, func(i, j int) bool {
			return nodes[i].ID < nodes[j].ID
		})

		return
	}

	sort.Slice(nodes, func(i, j int) bool {
		for _, o := range order {
			cmpRes := r.compareNodes(&nodes[i], &nodes[j], o.Field)
			if cmpRes != 0 {
				if o.Direction == filters.SortDirectionDesc {
					return cmpRes > 0
				}

				return cmpRes < 0
			}
		}

		return false
	})
}

func (r *NodeRepository) compareNodes(a, b *domain.Node, field string) int {
	switch field {
	case "id":
		return cmp.Compare(a.ID, b.ID)
	case "enabled":
		if !a.Enabled && b.Enabled {
			return -1
		}
		if a.Enabled && !b.Enabled {
			return 1
		}

		return 0
	case "name":
		return strings.Compare(a.Name, b.Name)
	case "os":
		return strings.Compare(string(a.OS), string(b.OS))
	case "created_at":
		if a.CreatedAt == nil && b.CreatedAt == nil {
			return 0
		}
		if a.CreatedAt == nil {
			return -1
		}
		if b.CreatedAt == nil {
			return 1
		}
		if a.CreatedAt.Before(*b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.After(*b.CreatedAt) {
			return 1
		}

		return 0
	case "updated_at":
		if a.UpdatedAt == nil && b.UpdatedAt == nil {
			return 0
		}
		if a.UpdatedAt == nil {
			return -1
		}
		if b.UpdatedAt == nil {
			return 1
		}
		if a.UpdatedAt.Before(*b.UpdatedAt) {
			return -1
		}
		if a.UpdatedAt.After(*b.UpdatedAt) {
			return 1
		}

		return 0
	default:
		return 0
	}
}

func (r *NodeRepository) applyPagination(nodes []domain.Node, pagination *filters.Pagination) []domain.Node {
	if pagination == nil {
		return nodes
	}

	limit := pagination.Limit
	if limit <= 0 {
		limit = filters.DefaultLimit
	}

	offset := max(pagination.Offset, 0)

	if offset >= len(nodes) {
		return []domain.Node{}
	}

	end := min(offset+limit, len(nodes))

	return nodes[offset:end]
}
