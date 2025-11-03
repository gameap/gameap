package testing

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type NodeRepositorySuite struct {
	suite.Suite

	repo repositories.NodeRepository
	fn   func(t *testing.T) repositories.NodeRepository
}

func NewNodeRepositorySuite(fn func(t *testing.T) repositories.NodeRepository) *NodeRepositorySuite {
	return &NodeRepositorySuite{
		fn: fn,
	}
}

func (s *NodeRepositorySuite) SetupTest() {
	s.repo = s.fn(s.T())
}

func (s *NodeRepositorySuite) TestNodeRepositorySave() {
	ctx := context.Background()

	s.T().Run("insert_new_node", func(t *testing.T) {
		// ARRANGE
		node := &domain.Node{
			Enabled:             true,
			Name:                "Test Node 1",
			OS:                  domain.NodeOSLinux,
			Location:            "US-East",
			Provider:            lo.ToPtr("AWS"),
			IPs:                 domain.IPList{"192.168.1.1", "10.0.0.1"},
			RAM:                 lo.ToPtr("16GB"),
			CPU:                 lo.ToPtr("8 cores"),
			WorkPath:            "/var/gameap",
			SteamcmdPath:        lo.ToPtr("/usr/games/steamcmd"),
			GdaemonHost:         "localhost",
			GdaemonPort:         31717,
			GdaemonAPIKey:       "test-api-key-1",
			GdaemonServerCert:   "cert-data",
			ClientCertificateID: 1,
			PreferInstallMethod: domain.NodePreferInstallMethodAuto,
			ScriptInstall:       lo.ToPtr("script-install"),
			ScriptReinstall:     lo.ToPtr("script-reinstall"),
			ScriptUpdate:        lo.ToPtr("script-update"),
			ScriptStart:         lo.ToPtr("script-start"),
			ScriptPause:         lo.ToPtr("script-pause"),
			ScriptUnpause:       lo.ToPtr("script-unpause"),
			ScriptStop:          lo.ToPtr("script-stop"),
			ScriptKill:          lo.ToPtr("script-kill"),
			ScriptRestart:       lo.ToPtr("script-restart"),
			ScriptStatus:        lo.ToPtr("script-status"),
			ScriptStats:         lo.ToPtr("script-stats"),
			ScriptGetConsole:    lo.ToPtr("script-get-console"),
			ScriptSendCommand:   lo.ToPtr("script-send-command"),
			ScriptDelete:        lo.ToPtr("script-delete"),
		}

		// ACT
		err := s.repo.Save(ctx, node)

		// ASSERT
		require.NoError(t, err)
		assert.NotZero(t, node.ID)
		assert.NotNil(t, node.CreatedAt)
		assert.NotNil(t, node.UpdatedAt)

		nodes, err := s.repo.Find(ctx, filters.FindNodeByIDs(node.ID), nil, nil)
		require.NoError(t, err)

		require.Len(t, nodes, 1)
		assert.Equal(t, "Test Node 1", nodes[0].Name)
		assert.Equal(t, domain.NodeOSLinux, nodes[0].OS)
		assert.Equal(t, "US-East", nodes[0].Location)
		assert.Equal(t, "AWS", *nodes[0].Provider)
		assert.Equal(t, 2, len(nodes[0].IPs))
		assert.Equal(t, "16GB", *nodes[0].RAM)
		assert.Equal(t, "8 cores", *nodes[0].CPU)
		assert.Equal(t, "/var/gameap", nodes[0].WorkPath)
		assert.Equal(t, "/usr/games/steamcmd", *nodes[0].SteamcmdPath)
		assert.Equal(t, "localhost", nodes[0].GdaemonHost)
		assert.Equal(t, 31717, nodes[0].GdaemonPort)
		assert.Equal(t, "test-api-key-1", nodes[0].GdaemonAPIKey)
		assert.Equal(t, "cert-data", nodes[0].GdaemonServerCert)
		assert.Equal(t, uint(1), nodes[0].ClientCertificateID)
		assert.Equal(t, domain.NodePreferInstallMethodAuto, nodes[0].PreferInstallMethod)
		require.NotNil(t, nodes[0].ScriptInstall)
		assert.Equal(t, "script-install", *nodes[0].ScriptInstall)
		require.NotNil(t, nodes[0].ScriptReinstall)
		assert.Equal(t, "script-reinstall", *nodes[0].ScriptReinstall)
		require.NotNil(t, nodes[0].ScriptUpdate)
		assert.Equal(t, "script-update", *nodes[0].ScriptUpdate)
		require.NotNil(t, nodes[0].ScriptStart)
		assert.Equal(t, "script-start", *nodes[0].ScriptStart)
		require.NotNil(t, nodes[0].ScriptPause)
		assert.Equal(t, "script-pause", *nodes[0].ScriptPause)
		require.NotNil(t, nodes[0].ScriptUnpause)
		assert.Equal(t, "script-unpause", *nodes[0].ScriptUnpause)
		require.NotNil(t, nodes[0].ScriptStop)
		assert.Equal(t, "script-stop", *nodes[0].ScriptStop)
		require.NotNil(t, nodes[0].ScriptKill)
		assert.Equal(t, "script-kill", *nodes[0].ScriptKill)
		require.NotNil(t, nodes[0].ScriptRestart)
		assert.Equal(t, "script-restart", *nodes[0].ScriptRestart)
		require.NotNil(t, nodes[0].ScriptStatus)
		assert.Equal(t, "script-status", *nodes[0].ScriptStatus)
		require.NotNil(t, nodes[0].ScriptStats)
		assert.Equal(t, "script-stats", *nodes[0].ScriptStats)
		require.NotNil(t, nodes[0].ScriptGetConsole)
		assert.Equal(t, "script-get-console", *nodes[0].ScriptGetConsole)
		require.NotNil(t, nodes[0].ScriptSendCommand)
		assert.Equal(t, "script-send-command", *nodes[0].ScriptSendCommand)
		require.NotNil(t, nodes[0].ScriptDelete)
		assert.Equal(t, "script-delete", *nodes[0].ScriptDelete)
	})

	s.T().Run("update_existing_node", func(t *testing.T) {
		node := &domain.Node{
			Enabled:             true,
			Name:                "Test Node 2",
			OS:                  domain.NodeOSLinux,
			Location:            "EU-West",
			IPs:                 domain.IPList{"192.168.2.1"},
			WorkPath:            "/opt/gameap",
			GdaemonHost:         "gdaemon.example.com",
			GdaemonPort:         31717,
			GdaemonAPIKey:       "test-api-key-2",
			GdaemonServerCert:   "cert-data-2",
			ClientCertificateID: 2,
			PreferInstallMethod: domain.NodePreferInstallMethodSteam,
		}

		err := s.repo.Save(ctx, node)
		require.NoError(t, err)
		originalID := node.ID
		originalUpdatedAt := node.UpdatedAt

		time.Sleep(10 * time.Millisecond)

		node.Name = "Updated Node 2"
		node.Location = "EU-Central"
		node.Enabled = false
		node.IPs = domain.IPList{"192.168.2.1", "192.168.2.2"}

		err = s.repo.Save(ctx, node)
		require.NoError(t, err)
		assert.Equal(t, originalID, node.ID)
		assert.True(t, node.UpdatedAt.After(*originalUpdatedAt) || node.UpdatedAt.Equal(*originalUpdatedAt))

		filter := &filters.FindNode{IDs: []uint{node.ID}}
		results, err := s.repo.Find(ctx, filter, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "Updated Node 2", results[0].Name)
		assert.Equal(t, "EU-Central", results[0].Location)
		assert.False(t, results[0].Enabled)
		assert.Len(t, results[0].IPs, 2)
	})

	s.T().Run("auto_set_timestamps", func(t *testing.T) {
		node := &domain.Node{
			Name:                "Timestamp Node",
			OS:                  domain.NodeOSWindows,
			Location:            "US-West",
			IPs:                 domain.IPList{},
			WorkPath:            "C:\\gameap",
			GdaemonHost:         "localhost",
			GdaemonPort:         31717,
			GdaemonAPIKey:       "test-key",
			GdaemonServerCert:   "cert",
			ClientCertificateID: 1,
			PreferInstallMethod: domain.NodePreferInstallMethodCopy,
		}

		beforeSave := time.Now()
		err := s.repo.Save(ctx, node)
		afterSave := time.Now()

		require.NoError(t, err)
		require.NotNil(t, node.CreatedAt)
		require.NotNil(t, node.UpdatedAt)
		assert.True(t, node.CreatedAt.After(beforeSave) || node.CreatedAt.Equal(beforeSave))
		assert.True(t, node.CreatedAt.Before(afterSave) || node.CreatedAt.Equal(afterSave))
	})
}

func (s *NodeRepositorySuite) TestNodeRepositoryFindAll() {
	ctx := context.Background()

	nodes := []*domain.Node{
		{
			Name: "Node A", OS: domain.NodeOSLinux, Location: "US", IPs: domain.IPList{"10.0.1.1"},
			WorkPath: "/var/gameap", GdaemonHost: "node-a", GdaemonPort: 31717,
			GdaemonAPIKey: "key-a", GdaemonServerCert: "cert-a", ClientCertificateID: 1,
			PreferInstallMethod: domain.NodePreferInstallMethodAuto,
		},
		{
			Name: "Node B", OS: domain.NodeOSWindows, Location: "EU", IPs: domain.IPList{"10.0.2.1"},
			WorkPath: "C:\\gameap", GdaemonHost: "node-b", GdaemonPort: 31717,
			GdaemonAPIKey: "key-b", GdaemonServerCert: "cert-b", ClientCertificateID: 2,
			PreferInstallMethod: domain.NodePreferInstallMethodSteam,
		},
		{
			Name: "Node C", OS: domain.NodeOSLinux, Location: "ASIA", IPs: domain.IPList{"10.0.3.1"},
			WorkPath: "/opt/gameap", GdaemonHost: "node-c", GdaemonPort: 31717,
			GdaemonAPIKey: "key-c", GdaemonServerCert: "cert-c", ClientCertificateID: 3,
			PreferInstallMethod: domain.NodePreferInstallMethodDownload,
		},
	}

	for _, n := range nodes {
		require.NoError(s.T(), s.repo.Save(ctx, n))
	}

	s.T().Run("find_all_nodes", func(t *testing.T) {
		results, err := s.repo.FindAll(ctx, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)

		names := make(map[string]bool)
		for _, r := range results {
			names[r.Name] = true
		}
		assert.True(t, names["Node A"])
		assert.True(t, names["Node B"])
		assert.True(t, names["Node C"])
	})

	s.T().Run("find_all_with_pagination", func(t *testing.T) {
		results, err := s.repo.FindAll(ctx, nil, &filters.Pagination{Limit: 2, Offset: 0})
		require.NoError(t, err)
		assert.LessOrEqual(t, len(results), 2)
	})

	s.T().Run("find_all_with_order", func(t *testing.T) {
		order := []filters.Sorting{{Field: "id", Direction: filters.SortDirectionDesc}}
		results, err := s.repo.FindAll(ctx, order, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(results), 2)
		for i := 0; i < len(results)-1; i++ {
			assert.GreaterOrEqual(t, results[i].ID, results[i+1].ID)
		}
	})
}

func (s *NodeRepositorySuite) TestNodeRepositoryFind() {
	ctx := context.Background()

	node1 := &domain.Node{
		Name: "Find Node 1", OS: domain.NodeOSLinux, Location: "US", IPs: domain.IPList{"10.1.1.1"},
		WorkPath: "/var/gameap", GdaemonHost: "find1", GdaemonPort: 31717,
		GdaemonAPIKey: "find-key-1", GdaemonAPIToken: lo.ToPtr("token-1"),
		GdaemonServerCert: "cert-1", ClientCertificateID: 1,
		PreferInstallMethod: domain.NodePreferInstallMethodAuto,
	}
	node2 := &domain.Node{
		Name: "Find Node 2", OS: domain.NodeOSWindows, Location: "EU", IPs: domain.IPList{"10.1.2.1"},
		WorkPath: "C:\\gameap", GdaemonHost: "find2", GdaemonPort: 31717,
		GdaemonAPIKey: "find-key-2", GdaemonServerCert: "cert-2",
		ClientCertificateID: 2, PreferInstallMethod: domain.NodePreferInstallMethodSteam,
	}

	require.NoError(s.T(), s.repo.Save(ctx, node1))
	require.NoError(s.T(), s.repo.Save(ctx, node2))

	s.T().Run("find_by_id", func(t *testing.T) {
		results, err := s.repo.Find(ctx, &filters.FindNode{IDs: []uint{node1.ID}}, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "Find Node 1", results[0].Name)
	})

	s.T().Run("find_by_gdaemon_api_key", func(t *testing.T) {
		results, err := s.repo.Find(ctx, &filters.FindNode{GDaemonAPIKey: lo.ToPtr("find-key-1")}, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, node1.ID, results[0].ID)
	})

	s.T().Run("find_by_gdaemon_api_token", func(t *testing.T) {
		results, err := s.repo.Find(ctx, &filters.FindNode{GDaemonAPIToken: lo.ToPtr("token-1")}, nil, nil)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, node1.ID, results[0].ID)
	})

	s.T().Run("find_with_nil_filter", func(t *testing.T) {
		results, err := s.repo.Find(ctx, nil, nil, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 2)
	})

	s.T().Run("find_non_existent", func(t *testing.T) {
		results, err := s.repo.Find(ctx, &filters.FindNode{IDs: []uint{99999}}, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("find_with_pagination", func(t *testing.T) {
		filter := &filters.FindNode{IDs: []uint{node1.ID, node2.ID}}
		pagination := &filters.Pagination{Limit: 1, Offset: 0}

		results, err := s.repo.Find(ctx, filter, nil, pagination)
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})

	s.T().Run("find_with_order", func(t *testing.T) {
		filter := &filters.FindNode{IDs: []uint{node1.ID, node2.ID}}
		order := []filters.Sorting{{Field: "id", Direction: filters.SortDirectionDesc}}

		results, err := s.repo.Find(ctx, filter, order, nil)
		require.NoError(t, err)
		require.Len(t, results, 2)

		for i := 0; i < len(results)-1; i++ {
			assert.GreaterOrEqual(t, results[i].ID, results[i+1].ID)
		}
	})
}

func (s *NodeRepositorySuite) TestNodeRepositoryDelete() {
	ctx := context.Background()

	s.T().Run("delete_existing_node", func(t *testing.T) {
		node := &domain.Node{
			Name: "Delete Node", OS: domain.NodeOSLinux, Location: "US", IPs: domain.IPList{},
			WorkPath: "/var/gameap", GdaemonHost: "del", GdaemonPort: 31717,
			GdaemonAPIKey: "del-key", GdaemonServerCert: "cert", ClientCertificateID: 1,
			PreferInstallMethod: domain.NodePreferInstallMethodAuto,
		}

		require.NoError(t, s.repo.Save(ctx, node))
		err := s.repo.Delete(ctx, node.ID)
		require.NoError(t, err)

		results, err := s.repo.Find(ctx, &filters.FindNode{IDs: []uint{node.ID}, WithDeleted: false}, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	s.T().Run("delete_non_existent", func(t *testing.T) {
		err := s.repo.Delete(ctx, 99999)
		require.NoError(t, err)
	})

	s.T().Run("delete_already_deleted_node", func(t *testing.T) {
		node := &domain.Node{
			Name: "Double Delete Node", OS: domain.NodeOSLinux, Location: "US", IPs: domain.IPList{},
			WorkPath: "/var/gameap", GdaemonHost: "dd", GdaemonPort: 31717,
			GdaemonAPIKey: "dd-key", GdaemonServerCert: "cert", ClientCertificateID: 1,
			PreferInstallMethod: domain.NodePreferInstallMethodAuto,
		}

		require.NoError(t, s.repo.Save(ctx, node))
		nodeID := node.ID

		err := s.repo.Delete(ctx, nodeID)
		require.NoError(t, err)

		err = s.repo.Delete(ctx, nodeID)
		require.NoError(t, err)
	})
}

func (s *NodeRepositorySuite) TestNodeRepositoryIntegration() {
	ctx := context.Background()

	s.T().Run("full_lifecycle", func(t *testing.T) {
		node := &domain.Node{
			Enabled: true, Name: "Lifecycle Node", OS: domain.NodeOSLinux, Location: "US-East",
			IPs: domain.IPList{"192.168.100.1"}, WorkPath: "/var/gameap",
			GdaemonHost: "gdaemon.test", GdaemonPort: 31717,
			GdaemonAPIKey: "lifecycle-key", GdaemonServerCert: "lifecycle-cert",
			ClientCertificateID: 1, PreferInstallMethod: domain.NodePreferInstallMethodAuto,
		}

		err := s.repo.Save(ctx, node)
		require.NoError(t, err)
		assert.NotZero(t, node.ID)

		results, err := s.repo.Find(ctx, &filters.FindNode{IDs: []uint{node.ID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "Lifecycle Node", results[0].Name)

		node.Name = "Updated Lifecycle Node"
		node.Location = "US-West"
		err = s.repo.Save(ctx, node)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, &filters.FindNode{IDs: []uint{node.ID}}, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "Updated Lifecycle Node", results[0].Name)
		assert.Equal(t, "US-West", results[0].Location)

		err = s.repo.Delete(ctx, node.ID)
		require.NoError(t, err)

		results, err = s.repo.Find(ctx, &filters.FindNode{IDs: []uint{node.ID}, WithDeleted: false}, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})
}
