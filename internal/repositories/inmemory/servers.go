package inmemory

import (
	"cmp"
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

type ServerRepository struct {
	mu          sync.RWMutex
	servers     map[uint]*domain.Server
	userServers map[uint]map[uint]struct{} // userID -> serverID -> exists
	nextID      uint32

	// Hash indexes for efficient filtering
	uuidIndex      map[uuid.UUID]map[uint]struct{} // uuid -> serverIDs
	gameIDIndex    map[string]map[uint]struct{}    // gameID -> serverIDs
	dsidIndex      map[uint]map[uint]struct{}      // dsid -> serverIDs
	gameModIDIndex map[uint]map[uint]struct{}      // gameModID -> serverIDs
	nameIndex      map[string]map[uint]struct{}    // name -> serverIDs
	enabledIndex   map[bool]map[uint]struct{}      // enabled -> serverIDs
	blockedIndex   map[bool]map[uint]struct{}      // blocked -> serverIDs
}

func NewServerRepository() *ServerRepository {
	return &ServerRepository{
		servers:        make(map[uint]*domain.Server),
		userServers:    make(map[uint]map[uint]struct{}),
		uuidIndex:      make(map[uuid.UUID]map[uint]struct{}),
		gameIDIndex:    make(map[string]map[uint]struct{}),
		dsidIndex:      make(map[uint]map[uint]struct{}),
		gameModIDIndex: make(map[uint]map[uint]struct{}),
		nameIndex:      make(map[string]map[uint]struct{}),
		enabledIndex:   make(map[bool]map[uint]struct{}),
		blockedIndex:   make(map[bool]map[uint]struct{}),
	}
}

// AddUserServer adds a user-server relationship for testing FindUserServers.
func (r *ServerRepository) AddUserServer(userID uint, serverID uint) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.userServers[userID] == nil {
		r.userServers[userID] = make(map[uint]struct{})
	}
	r.userServers[userID][serverID] = struct{}{}
}

// RemoveUserServer removes a user-server relationship.
func (r *ServerRepository) RemoveUserServer(userID uint, serverID uint) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.userServers[userID] != nil {
		delete(r.userServers[userID], serverID)
		if len(r.userServers[userID]) == 0 {
			delete(r.userServers, userID)
		}
	}
}

// SetUserServers sets all server relationships for a user, replacing any existing ones.
func (r *ServerRepository) SetUserServers(_ context.Context, userID uint, serverIDs []uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear existing relationships for this user
	delete(r.userServers, userID)

	// Add new relationships if there are any
	if len(serverIDs) > 0 {
		r.userServers[userID] = make(map[uint]struct{}, len(serverIDs))
		for _, serverID := range serverIDs {
			r.userServers[userID][serverID] = struct{}{}
		}
	}

	return nil
}

func (r *ServerRepository) Exists(_ context.Context, filter *filters.FindServer) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if filter == nil {
		return false, nil
	}

	// Use the existing filtering logic to get candidate servers
	candidateIDs := r.getFilteredServerIDs(filter)

	// If filtering by IDs, verify all IDs exist
	if len(filter.IDs) > 0 {
		return len(candidateIDs) == len(filter.IDs), nil
	}

	// For other filters, return true if at least one server exists
	return len(candidateIDs) > 0, nil
}

func (r *ServerRepository) Search(_ context.Context, query string) ([]*domain.Server, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	servers := make([]*domain.Server, 0, 10)

	// If query length is less than 3 characters, return first 10 non-deleted servers
	if len(query) < 3 {
		count := 0
		for _, server := range r.servers {
			if server.DeletedAt == nil {
				servers = append(servers, &domain.Server{
					ID:         server.ID,
					Name:       server.Name,
					ServerIP:   server.ServerIP,
					ServerPort: server.ServerPort,
					GameID:     server.GameID,
					GameModID:  server.GameModID,
				})
				count++
				if count >= 10 {
					break
				}
			}
		}

		return servers, nil
	}

	// Search for servers where name, server_ip, or server_port contains the query
	queryLower := strings.ToLower(query)
	for _, server := range r.servers {
		if server.DeletedAt != nil {
			continue
		}

		// Convert server_port to string for comparison
		serverPortStr := strconv.Itoa(server.ServerPort)

		// Check if any of the searchable fields contain the query (case-insensitive)
		if strings.Contains(strings.ToLower(server.Name), queryLower) ||
			strings.Contains(strings.ToLower(server.ServerIP), queryLower) ||
			strings.Contains(serverPortStr, queryLower) {
			servers = append(servers, &domain.Server{
				ID:         server.ID,
				Name:       server.Name,
				ServerIP:   server.ServerIP,
				ServerPort: server.ServerPort,
				GameID:     server.GameID,
				GameModID:  server.GameModID,
			})
		}
	}

	return servers, nil
}

func (r *ServerRepository) FindAll(
	_ context.Context,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Server, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	servers := make([]domain.Server, 0, len(r.servers))
	for _, server := range r.servers {
		if server.DeletedAt != nil {
			continue
		}
		servers = append(servers, *server)
	}

	r.sortServers(servers, order)

	return r.applyPagination(servers, pagination), nil
}

func (r *ServerRepository) Find(
	_ context.Context,
	filter *filters.FindServer,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Server, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if filter == nil {
		filter = &filters.FindServer{}
	}

	// Use hash indexes for efficient filtering
	candidateIDs := r.getFilteredServerIDs(filter)

	servers := make([]domain.Server, 0, len(candidateIDs))
	for serverID := range candidateIDs {
		if server, exists := r.servers[serverID]; exists {
			servers = append(servers, *server)
		}
	}

	r.sortServers(servers, order)

	return r.applyPagination(servers, pagination), nil
}

func (r *ServerRepository) FindUserServers(
	_ context.Context,
	userID uint,
	filter *filters.FindServer,
	order []filters.Sorting,
	pagination *filters.Pagination,
) ([]domain.Server, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	servers := make([]domain.Server, 0)

	// Get servers for this user using hash index
	if serverMap, exists := r.userServers[userID]; exists {
		for serverID := range serverMap {
			if server, exists := r.servers[serverID]; exists {
				servers = append(servers, *server)
			}
		}
	}

	// Use hash indexes for efficient filtering
	candidateIDs := r.getFilteredServerIDs(filter)
	filteredServers := make([]domain.Server, 0, len(candidateIDs))
	for _, server := range servers {
		if _, exists := candidateIDs[server.ID]; exists {
			filteredServers = append(filteredServers, server)
		}
	}

	r.sortServers(filteredServers, order)

	return r.applyPagination(filteredServers, pagination), nil
}

func (r *ServerRepository) Save(_ context.Context, server *domain.Server) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if server.UpdatedAt == nil || server.UpdatedAt.IsZero() {
		server.UpdatedAt = lo.ToPtr(time.Now())
	}

	if server.ID == 0 && (server.CreatedAt == nil || server.CreatedAt.IsZero()) {
		server.CreatedAt = lo.ToPtr(time.Now())
	}

	// Remove old indexes if updating existing server
	if server.ID != 0 {
		if oldServer, exists := r.servers[server.ID]; exists {
			r.removeFromIndexes(oldServer)
		}
	} else {
		server.ID = uint(atomic.AddUint32(&r.nextID, 1))
	}

	// Save server
	r.servers[server.ID] = &domain.Server{
		ID:               server.ID,
		UUID:             server.UUID,
		UUIDShort:        server.UUIDShort,
		Enabled:          server.Enabled,
		Installed:        server.Installed,
		Blocked:          server.Blocked,
		Name:             server.Name,
		GameID:           server.GameID,
		DSID:             server.DSID,
		GameModID:        server.GameModID,
		Expires:          server.Expires,
		ServerIP:         server.ServerIP,
		ServerPort:       server.ServerPort,
		QueryPort:        server.QueryPort,
		RconPort:         server.RconPort,
		Rcon:             server.Rcon,
		Dir:              server.Dir,
		SuUser:           server.SuUser,
		CPULimit:         server.CPULimit,
		RAMLimit:         server.RAMLimit,
		NetLimit:         server.NetLimit,
		StartCommand:     server.StartCommand,
		StopCommand:      server.StopCommand,
		ForceStopCommand: server.ForceStopCommand,
		RestartCommand:   server.RestartCommand,
		ProcessActive:    server.ProcessActive,
		LastProcessCheck: server.LastProcessCheck,
		Vars:             server.Vars,
		CreatedAt:        server.CreatedAt,
		UpdatedAt:        server.UpdatedAt,
		DeletedAt:        server.DeletedAt,
	}

	// Add to indexes
	r.addToIndexes(r.servers[server.ID])

	return nil
}

func (r *ServerRepository) SaveBulk(_ context.Context, servers []*domain.Server) error {
	if len(servers) == 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, server := range servers {
		// Remove old indexes if updating existing server
		if server.ID != 0 {
			if oldServer, exists := r.servers[server.ID]; exists {
				r.removeFromIndexes(oldServer)
			}
		} else {
			server.ID = uint(atomic.AddUint32(&r.nextID, 1))
		}

		// Save server
		r.servers[server.ID] = &domain.Server{
			ID:               server.ID,
			UUID:             server.UUID,
			UUIDShort:        server.UUIDShort,
			Enabled:          server.Enabled,
			Installed:        server.Installed,
			Blocked:          server.Blocked,
			Name:             server.Name,
			GameID:           server.GameID,
			DSID:             server.DSID,
			GameModID:        server.GameModID,
			Expires:          server.Expires,
			ServerIP:         server.ServerIP,
			ServerPort:       server.ServerPort,
			QueryPort:        server.QueryPort,
			RconPort:         server.RconPort,
			Rcon:             server.Rcon,
			Dir:              server.Dir,
			SuUser:           server.SuUser,
			CPULimit:         server.CPULimit,
			RAMLimit:         server.RAMLimit,
			NetLimit:         server.NetLimit,
			StartCommand:     server.StartCommand,
			StopCommand:      server.StopCommand,
			ForceStopCommand: server.ForceStopCommand,
			RestartCommand:   server.RestartCommand,
			ProcessActive:    server.ProcessActive,
			LastProcessCheck: server.LastProcessCheck,
			Vars:             server.Vars,
			CreatedAt:        server.CreatedAt,
			UpdatedAt:        server.UpdatedAt,
			DeletedAt:        server.DeletedAt,
		}

		// Add to indexes
		r.addToIndexes(r.servers[server.ID])
	}

	return nil
}

func (r *ServerRepository) Delete(_ context.Context, id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if server, exists := r.servers[id]; exists {
		// Remove from indexes
		r.removeFromIndexes(server)
	}

	delete(r.servers, id)

	// Also remove from user-server relationships
	for userID, serverMap := range r.userServers {
		delete(serverMap, id)
		if len(serverMap) == 0 {
			delete(r.userServers, userID)
		}
	}

	return nil
}

func (r *ServerRepository) addToIndexes(server *domain.Server) {
	// UUID index
	if r.uuidIndex[server.UUID] == nil {
		r.uuidIndex[server.UUID] = make(map[uint]struct{})
	}
	r.uuidIndex[server.UUID][server.ID] = struct{}{}

	// GameID index
	if r.gameIDIndex[server.GameID] == nil {
		r.gameIDIndex[server.GameID] = make(map[uint]struct{})
	}
	r.gameIDIndex[server.GameID][server.ID] = struct{}{}

	// DSID index
	if r.dsidIndex[server.DSID] == nil {
		r.dsidIndex[server.DSID] = make(map[uint]struct{})
	}
	r.dsidIndex[server.DSID][server.ID] = struct{}{}

	// GameModID index
	if r.gameModIDIndex[server.GameModID] == nil {
		r.gameModIDIndex[server.GameModID] = make(map[uint]struct{})
	}
	r.gameModIDIndex[server.GameModID][server.ID] = struct{}{}

	// TokenName index
	if r.nameIndex[server.Name] == nil {
		r.nameIndex[server.Name] = make(map[uint]struct{})
	}
	r.nameIndex[server.Name][server.ID] = struct{}{}

	// Enabled index
	if r.enabledIndex[server.Enabled] == nil {
		r.enabledIndex[server.Enabled] = make(map[uint]struct{})
	}
	r.enabledIndex[server.Enabled][server.ID] = struct{}{}

	// Blocked index
	if r.blockedIndex[server.Blocked] == nil {
		r.blockedIndex[server.Blocked] = make(map[uint]struct{})
	}
	r.blockedIndex[server.Blocked][server.ID] = struct{}{}
}

func (r *ServerRepository) removeFromIndexes(server *domain.Server) {
	// UUID index
	if serverSet, exists := r.uuidIndex[server.UUID]; exists {
		delete(serverSet, server.ID)
		if len(serverSet) == 0 {
			delete(r.uuidIndex, server.UUID)
		}
	}

	// GameID index
	if serverSet, exists := r.gameIDIndex[server.GameID]; exists {
		delete(serverSet, server.ID)
		if len(serverSet) == 0 {
			delete(r.gameIDIndex, server.GameID)
		}
	}

	// DSID index
	if serverSet, exists := r.dsidIndex[server.DSID]; exists {
		delete(serverSet, server.ID)
		if len(serverSet) == 0 {
			delete(r.dsidIndex, server.DSID)
		}
	}

	// GameModID index
	if serverSet, exists := r.gameModIDIndex[server.GameModID]; exists {
		delete(serverSet, server.ID)
		if len(serverSet) == 0 {
			delete(r.gameModIDIndex, server.GameModID)
		}
	}

	// TokenName index
	if serverSet, exists := r.nameIndex[server.Name]; exists {
		delete(serverSet, server.ID)
		if len(serverSet) == 0 {
			delete(r.nameIndex, server.Name)
		}
	}

	// Enabled index
	if serverSet, exists := r.enabledIndex[server.Enabled]; exists {
		delete(serverSet, server.ID)
		if len(serverSet) == 0 {
			delete(r.enabledIndex, server.Enabled)
		}
	}

	// Blocked index
	if serverSet, exists := r.blockedIndex[server.Blocked]; exists {
		delete(serverSet, server.ID)
		if len(serverSet) == 0 {
			delete(r.blockedIndex, server.Blocked)
		}
	}
}

//nolint:gocognit,gocyclo,funlen
func (r *ServerRepository) getFilteredServerIDs(filter *filters.FindServer) map[uint]struct{} {
	resultIDs := make(map[uint]struct{}, len(r.servers))

	if filter == nil {
		// No filter, return all non-deleted server IDs
		for serverID, server := range r.servers {
			if server.DeletedAt == nil {
				resultIDs[serverID] = struct{}{}
			}
		}

		return resultIDs
	}

	// Start with the first available filter result
	switch {
	case len(filter.IDs) > 0:
		for _, id := range filter.IDs {
			if server, exists := r.servers[id]; exists {
				// Check deleted_at if WithDeleted is false
				if !filter.WithDeleted && server.DeletedAt != nil {
					continue
				}
				resultIDs[id] = struct{}{}
			}
		}
	case len(filter.UUIDs) > 0:
		for _, u := range filter.UUIDs {
			if serverSet, exists := r.uuidIndex[u]; exists {
				for serverID := range serverSet {
					if server, exists := r.servers[serverID]; exists {
						if !filter.WithDeleted && server.DeletedAt != nil {
							continue
						}
						resultIDs[serverID] = struct{}{}
					}
				}
			}
		}
	case len(filter.UserIDs) > 0:
		for _, userID := range filter.UserIDs {
			if serverSet, exists := r.userServers[userID]; exists {
				for serverID := range serverSet {
					if server, exists := r.servers[serverID]; exists {
						if !filter.WithDeleted && server.DeletedAt != nil {
							continue
						}
						resultIDs[serverID] = struct{}{}
					}
				}
			}
		}
	case filter.Enabled != nil:
		if serverSet, exists := r.enabledIndex[*filter.Enabled]; exists {
			for serverID := range serverSet {
				if server, exists := r.servers[serverID]; exists {
					if !filter.WithDeleted && server.DeletedAt != nil {
						continue
					}
					resultIDs[serverID] = struct{}{}
				}
			}
		}
	case filter.Blocked != nil:
		if serverSet, exists := r.blockedIndex[*filter.Blocked]; exists {
			for serverID := range serverSet {
				if server, exists := r.servers[serverID]; exists {
					if !filter.WithDeleted && server.DeletedAt != nil {
						continue
					}
					resultIDs[serverID] = struct{}{}
				}
			}
		}
	case len(filter.GameIDs) > 0:
		for _, gameID := range filter.GameIDs {
			if serverSet, exists := r.gameIDIndex[gameID]; exists {
				for serverID := range serverSet {
					if server, exists := r.servers[serverID]; exists {
						if !filter.WithDeleted && server.DeletedAt != nil {
							continue
						}
						resultIDs[serverID] = struct{}{}
					}
				}
			}
		}
	case len(filter.DSIDs) > 0:
		for _, dsid := range filter.DSIDs {
			if serverSet, exists := r.dsidIndex[dsid]; exists {
				for serverID := range serverSet {
					if server, exists := r.servers[serverID]; exists {
						if !filter.WithDeleted && server.DeletedAt != nil {
							continue
						}
						resultIDs[serverID] = struct{}{}
					}
				}
			}
		}
	case len(filter.GameModIDs) > 0:
		for _, gameModID := range filter.GameModIDs {
			if serverSet, exists := r.gameModIDIndex[gameModID]; exists {
				for serverID := range serverSet {
					if server, exists := r.servers[serverID]; exists {
						if !filter.WithDeleted && server.DeletedAt != nil {
							continue
						}
						resultIDs[serverID] = struct{}{}
					}
				}
			}
		}
	case len(filter.Names) > 0:
		for _, name := range filter.Names {
			if serverSet, exists := r.nameIndex[name]; exists {
				for serverID := range serverSet {
					if server, exists := r.servers[serverID]; exists {
						if !filter.WithDeleted && server.DeletedAt != nil {
							continue
						}
						resultIDs[serverID] = struct{}{}
					}
				}
			}
		}
	default:
		// No filters, return all non-deleted servers
		for serverID, server := range r.servers {
			if !filter.WithDeleted && server.DeletedAt != nil {
				continue
			}
			resultIDs[serverID] = struct{}{}
		}
	}

	if len(filter.UUIDs) > 0 && len(filter.IDs) > 0 {
		r.intersectWithUUIDs(resultIDs, filter.UUIDs)
	}
	if len(filter.UserIDs) > 0 && (len(filter.IDs) > 0 ||
		len(filter.UUIDs) > 0) {
		r.intersectWithUserIDs(resultIDs, filter.UserIDs)
	}
	if filter.Enabled != nil && (len(filter.IDs) > 0 ||
		len(filter.UUIDs) > 0 ||
		len(filter.UserIDs) > 0) {
		r.intersectWithEnabled(resultIDs, *filter.Enabled)
	}
	if filter.Blocked != nil && (len(filter.IDs) > 0 ||
		len(filter.UUIDs) > 0 ||
		len(filter.UserIDs) > 0 ||
		filter.Enabled != nil) {
		r.intersectWithBlocked(resultIDs, *filter.Blocked)
	}
	if len(filter.GameIDs) > 0 && (len(filter.IDs) > 0 ||
		len(filter.UUIDs) > 0 ||
		len(filter.UserIDs) > 0 ||
		filter.Enabled != nil ||
		filter.Blocked != nil) {
		r.intersectWithGameIDs(resultIDs, filter.GameIDs)
	}
	if len(filter.DSIDs) > 0 && (len(filter.IDs) > 0 ||
		len(filter.UUIDs) > 0 ||
		len(filter.UserIDs) > 0 ||
		filter.Enabled != nil ||
		filter.Blocked != nil ||
		len(filter.GameIDs) > 0) {
		r.intersectWithDSIDs(resultIDs, filter.DSIDs)
	}
	if len(filter.Names) > 0 && (len(filter.IDs) > 0 ||
		len(filter.UUIDs) > 0 ||
		len(filter.UserIDs) > 0 ||
		filter.Enabled != nil ||
		filter.Blocked != nil ||
		len(filter.GameIDs) > 0 ||
		len(filter.DSIDs) > 0) {
		r.intersectWithNames(resultIDs, filter.Names)
	}

	return resultIDs
}

func (r *ServerRepository) intersectWithUUIDs(resultIDs map[uint]struct{}, uuids []uuid.UUID) {
	validIDs := make(map[uint]struct{})
	for _, u := range uuids {
		if serverSet, exists := r.uuidIndex[u]; exists {
			for serverID := range serverSet {
				if _, exists := resultIDs[serverID]; exists {
					validIDs[serverID] = struct{}{}
				}
			}
		}
	}
	// Replace resultIDs with intersection
	for id := range resultIDs {
		delete(resultIDs, id)
	}
	for id := range validIDs {
		resultIDs[id] = struct{}{}
	}
}

func (r *ServerRepository) intersectWithUserIDs(resultIDs map[uint]struct{}, userIDs []uint) {
	validIDs := make(map[uint]struct{})
	for _, userID := range userIDs {
		if serverSet, exists := r.userServers[userID]; exists {
			for serverID := range serverSet {
				if _, exists := resultIDs[serverID]; exists {
					validIDs[serverID] = struct{}{}
				}
			}
		}
	}
	// Replace resultIDs with intersection
	for id := range resultIDs {
		delete(resultIDs, id)
	}
	for id := range validIDs {
		resultIDs[id] = struct{}{}
	}
}

func (r *ServerRepository) intersectWithEnabled(resultIDs map[uint]struct{}, enabled bool) {
	validIDs := make(map[uint]struct{})
	if serverSet, exists := r.enabledIndex[enabled]; exists {
		for serverID := range serverSet {
			if _, exists := resultIDs[serverID]; exists {
				validIDs[serverID] = struct{}{}
			}
		}
	}
	// Replace resultIDs with intersection
	for id := range resultIDs {
		delete(resultIDs, id)
	}
	for id := range validIDs {
		resultIDs[id] = struct{}{}
	}
}

func (r *ServerRepository) intersectWithBlocked(resultIDs map[uint]struct{}, blocked bool) {
	validIDs := make(map[uint]struct{})
	if serverSet, exists := r.blockedIndex[blocked]; exists {
		for serverID := range serverSet {
			if _, exists := resultIDs[serverID]; exists {
				validIDs[serverID] = struct{}{}
			}
		}
	}
	// Replace resultIDs with intersection
	for id := range resultIDs {
		delete(resultIDs, id)
	}
	for id := range validIDs {
		resultIDs[id] = struct{}{}
	}
}

func (r *ServerRepository) intersectWithGameIDs(resultIDs map[uint]struct{}, gameIDs []string) {
	validIDs := make(map[uint]struct{})
	for _, gameID := range gameIDs {
		if serverSet, exists := r.gameIDIndex[gameID]; exists {
			for serverID := range serverSet {
				if _, exists := resultIDs[serverID]; exists {
					validIDs[serverID] = struct{}{}
				}
			}
		}
	}
	// Replace resultIDs with intersection
	for id := range resultIDs {
		delete(resultIDs, id)
	}
	for id := range validIDs {
		resultIDs[id] = struct{}{}
	}
}

func (r *ServerRepository) intersectWithDSIDs(resultIDs map[uint]struct{}, dsids []uint) {
	validIDs := make(map[uint]struct{})
	for _, dsid := range dsids {
		if serverSet, exists := r.dsidIndex[dsid]; exists {
			for serverID := range serverSet {
				if _, exists := resultIDs[serverID]; exists {
					validIDs[serverID] = struct{}{}
				}
			}
		}
	}
	// Replace resultIDs with intersection
	for id := range resultIDs {
		delete(resultIDs, id)
	}
	for id := range validIDs {
		resultIDs[id] = struct{}{}
	}
}

func (r *ServerRepository) intersectWithNames(resultIDs map[uint]struct{}, names []string) {
	validIDs := make(map[uint]struct{})
	for _, name := range names {
		if serverSet, exists := r.nameIndex[name]; exists {
			for serverID := range serverSet {
				if _, exists := resultIDs[serverID]; exists {
					validIDs[serverID] = struct{}{}
				}
			}
		}
	}
	// Replace resultIDs with intersection
	for id := range resultIDs {
		delete(resultIDs, id)
	}
	for id := range validIDs {
		resultIDs[id] = struct{}{}
	}
}

func (r *ServerRepository) sortServers(servers []domain.Server, order []filters.Sorting) {
	if len(order) == 0 {
		sort.Slice(servers, func(i, j int) bool {
			return servers[i].ID < servers[j].ID
		})

		return
	}

	sort.Slice(servers, func(i, j int) bool {
		for _, o := range order {
			cm := r.compareServers(&servers[i], &servers[j], o.Field)
			if cm != 0 {
				if o.Direction == filters.SortDirectionDesc {
					return cm > 0
				}

				return cm < 0
			}
		}

		return false
	})
}

//nolint:gocognit,funlen,gocyclo
func (r *ServerRepository) compareServers(a, b *domain.Server, field string) int {
	switch field {
	case "id":
		return cmp.Compare(a.ID, b.ID)
	case "uuid":
		return strings.Compare(a.UUID.String(), b.UUID.String())
	case "uuid_short":
		return strings.Compare(a.UUIDShort, b.UUIDShort)
	case "enabled":
		if !a.Enabled && b.Enabled {
			return -1
		}
		if a.Enabled && !b.Enabled {
			return 1
		}

		return 0
	case "installed":
		if a.Installed < b.Installed {
			return -1
		}
		if a.Installed > b.Installed {
			return 1
		}

		return 0
	case "blocked":
		if !a.Blocked && b.Blocked {
			return -1
		}
		if a.Blocked && !b.Blocked {
			return 1
		}

		return 0
	case "name":
		return strings.Compare(a.Name, b.Name)
	case "game_id":
		return strings.Compare(a.GameID, b.GameID)
	case "ds_id":
		return cmp.Compare(a.DSID, b.DSID)
	case "game_mod_id":
		if a.GameModID < b.GameModID {
			return -1
		}
		if a.GameModID > b.GameModID {
			return 1
		}

		return 0
	case "server_ip":
		return strings.Compare(a.ServerIP, b.ServerIP)
	case "server_port":
		return cmp.Compare(a.ServerPort, b.ServerPort)
	case "dir":
		return strings.Compare(a.Dir, b.Dir)
	case "process_active":
		if !a.ProcessActive && b.ProcessActive {
			return -1
		}
		if a.ProcessActive && !b.ProcessActive {
			return 1
		}

		return 0
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

func (r *ServerRepository) applyPagination(servers []domain.Server, pagination *filters.Pagination) []domain.Server {
	if pagination == nil {
		return servers
	}

	limit := pagination.Limit
	if limit <= 0 {
		limit = filters.DefaultLimit
	}

	offset := max(pagination.Offset, 0)

	if offset >= len(servers) {
		return []domain.Server{}
	}

	end := min(offset+limit, len(servers))

	return servers[offset:end]
}
