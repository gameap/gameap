package inmemory_test

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	repotesting "github.com/gameap/gameap/internal/repositories/testing"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestNodeRepository(t *testing.T) {
	suite.Run(t, repotesting.NewNodeRepositorySuite(
		func(_ *testing.T) repositories.NodeRepository {
			return inmemory.NewNodeRepository()
		},
	))
}

func TestNodeRepository_DeletedAtFiltering(t *testing.T) {
	repo := inmemory.NewNodeRepository()
	now := time.Now()
	deletedTime := now.Add(-1 * time.Hour)

	node1 := &domain.Node{
		ID:          1,
		Name:        "Active Node",
		OS:          "linux",
		Location:    "US",
		GdaemonHost: "localhost",
		GdaemonPort: 8080,
		WorkPath:    "/var/gameap",
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}

	node2 := &domain.Node{
		ID:          2,
		Name:        "Deleted Node",
		OS:          "linux",
		Location:    "EU",
		GdaemonHost: "localhost",
		GdaemonPort: 8081,
		WorkPath:    "/var/gameap",
		CreatedAt:   &now,
		UpdatedAt:   &now,
		DeletedAt:   &deletedTime,
	}

	require.NoError(t, repo.Save(context.Background(), node1))
	require.NoError(t, repo.Save(context.Background(), node2))

	t.Run("FindAll_excludes_deleted_nodes", func(t *testing.T) {
		nodes, err := repo.FindAll(context.Background(), nil, nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 1)
		assert.Equal(t, "Active Node", nodes[0].Name)
	})

	t.Run("Find_without_WithDeleted_excludes_deleted_nodes", func(t *testing.T) {
		filter := &filters.FindNode{}
		nodes, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 1)
		assert.Equal(t, "Active Node", nodes[0].Name)
	})

	t.Run("Find_with_WithDeleted_includes_deleted_nodes", func(t *testing.T) {
		filter := &filters.FindNode{WithDeleted: true}
		nodes, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 2)
	})

	t.Run("Find_by_ID_without_WithDeleted_excludes_deleted_nodes", func(t *testing.T) {
		filter := &filters.FindNode{IDs: []uint{1, 2}}
		nodes, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 1)
		assert.Equal(t, uint(1), nodes[0].ID)
	})

	t.Run("Find_by_ID_with_WithDeleted_includes_deleted_nodes", func(t *testing.T) {
		filter := &filters.FindNode{IDs: []uint{1, 2}, WithDeleted: true}
		nodes, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 2)
	})
}

func TestNodeRepository_GDaemonAPIKeyFiltering(t *testing.T) {
	repo := inmemory.NewNodeRepository()
	now := time.Now()
	apiKey1 := "api-key-1"
	apiKey2 := "api-key-2"
	apiToken1 := "api-token-1"

	node1 := &domain.Node{
		ID:              1,
		Name:            "Node 1",
		OS:              "linux",
		Location:        "US",
		GdaemonHost:     "localhost",
		GdaemonPort:     8080,
		GdaemonAPIKey:   apiKey1,
		GdaemonAPIToken: &apiToken1,
		WorkPath:        "/var/gameap",
		CreatedAt:       &now,
		UpdatedAt:       &now,
	}

	node2 := &domain.Node{
		ID:            2,
		Name:          "Node 2",
		OS:            "linux",
		Location:      "EU",
		GdaemonHost:   "localhost",
		GdaemonPort:   8081,
		GdaemonAPIKey: apiKey2,
		WorkPath:      "/var/gameap",
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}

	node3 := &domain.Node{
		ID:            3,
		Name:          "Node 3",
		OS:            "windows",
		Location:      "ASIA",
		GdaemonHost:   "localhost",
		GdaemonPort:   8082,
		GdaemonAPIKey: apiKey1,
		WorkPath:      "/var/gameap",
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}

	require.NoError(t, repo.Save(context.Background(), node1))
	require.NoError(t, repo.Save(context.Background(), node2))
	require.NoError(t, repo.Save(context.Background(), node3))

	t.Run("Find_by_GDaemonAPIKey_returns_matching_nodes", func(t *testing.T) {
		filter := &filters.FindNode{GDaemonAPIKey: &apiKey1}
		nodes, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 2)

		nodeIDs := make([]uint, len(nodes))
		for i, node := range nodes {
			nodeIDs[i] = node.ID
		}
		assert.ElementsMatch(t, []uint{1, 3}, nodeIDs)
	})

	t.Run("Find_by_GDaemonAPIKey_returns_single_node", func(t *testing.T) {
		filter := &filters.FindNode{GDaemonAPIKey: &apiKey2}
		nodes, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 1)
		assert.Equal(t, uint(2), nodes[0].ID)
		assert.Equal(t, "Node 2", nodes[0].Name)
	})

	t.Run("Find_by_GDaemonAPIKey_with_no_matches_returns_empty", func(t *testing.T) {
		nonExistentKey := "non-existent-key"
		filter := &filters.FindNode{GDaemonAPIKey: &nonExistentKey}
		nodes, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 0)
	})

	t.Run("Find_by_GDaemonAPIKey_and_IDs", func(t *testing.T) {
		filter := &filters.FindNode{
			IDs:           []uint{1, 2},
			GDaemonAPIKey: &apiKey1,
		}
		nodes, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 1)
		assert.Equal(t, uint(1), nodes[0].ID)
	})

	t.Run("Find_by_GDaemonAPIKey_and_GDaemonAPIToken", func(t *testing.T) {
		filter := &filters.FindNode{
			GDaemonAPIKey:   &apiKey1,
			GDaemonAPIToken: &apiToken1,
		}
		nodes, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 1)
		assert.Equal(t, uint(1), nodes[0].ID)
	})
}

func TestNodeRepository_GDaemonAPIKeyFiltering_WithDeletedNodes(t *testing.T) {
	repo := inmemory.NewNodeRepository()
	now := time.Now()
	deletedTime := now.Add(-1 * time.Hour)
	apiKey := "api-key-1"

	activeNode := &domain.Node{
		ID:            1,
		Name:          "Active Node",
		OS:            "linux",
		Location:      "US",
		GdaemonHost:   "localhost",
		GdaemonPort:   8080,
		GdaemonAPIKey: apiKey,
		WorkPath:      "/var/gameap",
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}

	deletedNode := &domain.Node{
		ID:            2,
		Name:          "Deleted Node",
		OS:            "linux",
		Location:      "EU",
		GdaemonHost:   "localhost",
		GdaemonPort:   8081,
		GdaemonAPIKey: apiKey,
		WorkPath:      "/var/gameap",
		CreatedAt:     &now,
		UpdatedAt:     &now,
		DeletedAt:     &deletedTime,
	}

	require.NoError(t, repo.Save(context.Background(), activeNode))
	require.NoError(t, repo.Save(context.Background(), deletedNode))

	t.Run("Find_by_GDaemonAPIKey_excludes_deleted_nodes_by_default", func(t *testing.T) {
		filter := &filters.FindNode{GDaemonAPIKey: &apiKey}
		nodes, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 1)
		assert.Equal(t, uint(1), nodes[0].ID)
		assert.Equal(t, "Active Node", nodes[0].Name)
	})

	t.Run("Find_by_GDaemonAPIKey_includes_deleted_nodes_with_WithDeleted", func(t *testing.T) {
		filter := &filters.FindNode{
			GDaemonAPIKey: &apiKey,
			WithDeleted:   true,
		}
		nodes, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 2)

		nodeIDs := make([]uint, len(nodes))
		for i, node := range nodes {
			nodeIDs[i] = node.ID
		}
		assert.ElementsMatch(t, []uint{1, 2}, nodeIDs)
	})
}

func TestNodeRepository_FilterByNodeIDs(t *testing.T) {
	repo := inmemory.NewNodeRepository()
	now := time.Now()

	node1 := &domain.Node{
		ID:            1,
		Name:          "Node 1",
		OS:            "linux",
		Location:      "US",
		GdaemonHost:   "localhost",
		GdaemonPort:   8080,
		GdaemonAPIKey: "key1",
		WorkPath:      "/var/gameap",
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}

	node2 := &domain.Node{
		ID:            2,
		Name:          "Node 2",
		OS:            "linux",
		Location:      "EU",
		GdaemonHost:   "localhost",
		GdaemonPort:   8081,
		GdaemonAPIKey: "key2",
		WorkPath:      "/var/gameap",
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}

	require.NoError(t, repo.Save(context.Background(), node1))
	require.NoError(t, repo.Save(context.Background(), node2))

	t.Run("Filter_by_Node_1", func(t *testing.T) {
		filter := &filters.FindNode{IDs: []uint{1}}
		nodes, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 1)
		assert.Equal(t, "Node 1", nodes[0].Name)
	})

	t.Run("Filter_by_Node_2", func(t *testing.T) {
		filter := &filters.FindNode{IDs: []uint{2}}
		nodes, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 1)
		assert.Equal(t, "Node 2", nodes[0].Name)
	})

	t.Run("Filter_by_Multiple_Nodes", func(t *testing.T) {
		filter := &filters.FindNode{IDs: []uint{1, 2}}
		nodes, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 2)
	})

	t.Run("Filter_by_Non-existent_Node", func(t *testing.T) {
		filter := &filters.FindNode{IDs: []uint{999}}
		nodes, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 0)
	})

	t.Run("Filter_by_Node_and_GDaemonAPIKey", func(t *testing.T) {
		key := "key1"
		filter := &filters.FindNode{
			IDs:           []uint{1, 2},
			GDaemonAPIKey: lo.ToPtr(key),
		}
		nodes, err := repo.Find(context.Background(), filter, nil, nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 1)
		assert.Equal(t, "Node 1", nodes[0].Name)
	})
}
