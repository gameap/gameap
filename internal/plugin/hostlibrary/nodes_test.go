package hostlibrary

import (
	"context"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/plugin/sdk/common"
	"github.com/gameap/gameap/pkg/plugin/sdk/nodes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodesService_FindNodes(t *testing.T) {
	tests := []struct {
		name      string
		setupRepo func(*inmemory.NodeRepository)
		request   *nodes.FindNodesRequest
		wantTotal int
		wantIDs   []uint
	}{
		{
			name: "no_filter_returns_all",
			setupRepo: func(r *inmemory.NodeRepository) {
				_ = r.Save(context.Background(), &domain.Node{Name: "Node1", OS: domain.NodeOSLinux, Enabled: true})
				_ = r.Save(context.Background(), &domain.Node{Name: "Node2", OS: domain.NodeOSWindows, Enabled: true})
				_ = r.Save(context.Background(), &domain.Node{Name: "Node3", OS: domain.NodeOSLinux, Enabled: false})
			},
			request:   &nodes.FindNodesRequest{},
			wantTotal: 3,
			wantIDs:   []uint{1, 2, 3},
		},
		{
			name: "filter_by_ids",
			setupRepo: func(r *inmemory.NodeRepository) {
				_ = r.Save(context.Background(), &domain.Node{Name: "Node1", OS: domain.NodeOSLinux})
				_ = r.Save(context.Background(), &domain.Node{Name: "Node2", OS: domain.NodeOSWindows})
				_ = r.Save(context.Background(), &domain.Node{Name: "Node3", OS: domain.NodeOSLinux})
			},
			request: &nodes.FindNodesRequest{
				Filter: &nodes.NodeFilter{Ids: []uint64{1, 3}},
			},
			wantTotal: 2,
			wantIDs:   []uint{1, 3},
		},
		{
			name: "pagination_applied",
			setupRepo: func(r *inmemory.NodeRepository) {
				for i := 1; i <= 10; i++ {
					_ = r.Save(context.Background(), &domain.Node{Name: "Node" + string(rune('0'+i)), OS: domain.NodeOSLinux})
				}
			},
			request: &nodes.FindNodesRequest{
				Pagination: &common.Pagination{Limit: 3, Offset: 2},
			},
			wantTotal: 3,
			wantIDs:   []uint{3, 4, 5},
		},
		{
			name:      "empty_repository_returns_empty",
			setupRepo: func(_ *inmemory.NodeRepository) {},
			request:   &nodes.FindNodesRequest{},
			wantTotal: 0,
			wantIDs:   []uint{},
		},
		{
			name: "filter_nonexistent_ids",
			setupRepo: func(r *inmemory.NodeRepository) {
				_ = r.Save(context.Background(), &domain.Node{Name: "Node1", OS: domain.NodeOSLinux})
			},
			request: &nodes.FindNodesRequest{
				Filter: &nodes.NodeFilter{Ids: []uint64{999}},
			},
			wantTotal: 0,
			wantIDs:   []uint{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := inmemory.NewNodeRepository()
			tt.setupRepo(repo)

			svc := newReadOnlyNodesService(repo)
			resp, err := svc.FindNodes(context.Background(), tt.request)

			require.NoError(t, err)
			assert.Equal(t, int32(tt.wantTotal), resp.Total)
			require.Len(t, resp.Nodes, tt.wantTotal)

			for i, wantID := range tt.wantIDs {
				assert.Equal(t, uint64(wantID), resp.Nodes[i].Id)
			}
		})
	}
}

func TestNodesService_GetNode(t *testing.T) {
	tests := []struct {
		name      string
		setupRepo func(*inmemory.NodeRepository)
		nodeID    uint64
		wantFound bool
		wantName  string
	}{
		{
			name: "existing_returns_found",
			setupRepo: func(r *inmemory.NodeRepository) {
				_ = r.Save(context.Background(), &domain.Node{
					Name:        "MainNode",
					OS:          domain.NodeOSLinux,
					Location:    "US-East",
					Provider:    new("AWS"),
					GdaemonHost: "192.168.1.1",
					GdaemonPort: 31717,
				})
			},
			nodeID:    1,
			wantFound: true,
			wantName:  "MainNode",
		},
		{
			name:      "missing_returns_not_found",
			setupRepo: func(_ *inmemory.NodeRepository) {},
			nodeID:    999,
			wantFound: false,
		},
		{
			name: "wrong_id_returns_not_found",
			setupRepo: func(r *inmemory.NodeRepository) {
				_ = r.Save(context.Background(), &domain.Node{Name: "Node1", OS: domain.NodeOSLinux})
			},
			nodeID:    999,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := inmemory.NewNodeRepository()
			tt.setupRepo(repo)

			svc := newReadOnlyNodesService(repo)
			resp, err := svc.GetNode(context.Background(), &nodes.GetNodeRequest{Id: tt.nodeID})

			require.NoError(t, err)
			assert.Equal(t, tt.wantFound, resp.Found)

			if tt.wantFound {
				require.NotNil(t, resp.Node)
				assert.Equal(t, tt.wantName, resp.Node.Name)
				assert.Equal(t, tt.nodeID, resp.Node.Id)
			} else {
				assert.Nil(t, resp.Node)
			}
		})
	}
}

func TestConvertNodeToProto(t *testing.T) {
	node := &domain.Node{
		ID:          42,
		Name:        "TestNode",
		Enabled:     true,
		OS:          domain.NodeOSLinux,
		Location:    "US-East",
		Provider:    new("DigitalOcean"),
		IPs:         []string{"192.168.1.1", "10.0.0.1"},
		WorkPath:    "/home/gameap",
		GdaemonHost: "node.example.com",
		GdaemonPort: 31717,
	}

	result := convertNodeToProto(node)

	assert.Equal(t, uint64(42), result.Id)
	assert.Equal(t, "TestNode", result.Name)
	assert.True(t, result.Enabled)
	assert.Equal(t, string(domain.NodeOSLinux), result.Os)
	assert.Equal(t, "US-East", result.Location)
	require.NotNil(t, result.Provider)
	assert.Equal(t, "DigitalOcean", *result.Provider)
	assert.Equal(t, []string{"192.168.1.1", "10.0.0.1"}, result.Ips)
	assert.Equal(t, "/home/gameap", result.WorkPath)
	assert.Equal(t, "node.example.com", result.GdaemonHost)
	assert.Equal(t, int32(31717), result.GdaemonPort)
}

func TestConvertNodeToProto_MinimalFields(t *testing.T) {
	node := &domain.Node{
		ID:      1,
		Name:    "BasicNode",
		Enabled: false,
		OS:      domain.NodeOSWindows,
	}

	result := convertNodeToProto(node)

	assert.Equal(t, uint64(1), result.Id)
	assert.Equal(t, "BasicNode", result.Name)
	assert.False(t, result.Enabled)
	assert.Equal(t, string(domain.NodeOSWindows), result.Os)
	assert.Empty(t, result.Location)
	assert.Empty(t, result.Provider)
	assert.Nil(t, result.Ips)
}

func TestNodesHostLibraryFactory_Create(t *testing.T) {
	repo := inmemory.NewNodeRepository()
	factory := NewNodesHostLibraryFactory(repo, nil, nil, nil, stubPermissionChecker{}, nil)

	lib := factory.Create(nodesTestPluginID)

	require.NotNil(t, lib)
	impl, ok := lib.(*NodesHostLibrary)
	require.True(t, ok)
	assert.Equal(t, uint64(nodesTestPluginID), impl.impl.pluginID)
}

const nodesTestPluginID = 7

// newReadOnlyNodesService builds the module the way a plugin without the
// manage_nodes grant sees it: reads work, every write is refused.
func newReadOnlyNodesService(repo *inmemory.NodeRepository) *NodesServiceImpl {
	return NewNodesService(
		nodesTestPluginID,
		repo,
		nil,
		nil,
		nil,
		stubPermissionChecker{},
		nil,
	)
}
