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
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
)

func TestServerTaskRepository(t *testing.T) {
	suite.Run(t, repotesting.NewServerTaskRepositorySuite(
		func(_ *testing.T) repositories.ServerTaskRepository {
			return inmemory.NewServerTaskRepository(inmemory.NewServerRepository())
		},
	))
}

func TestServerTaskRepository_FilterByNodeIDs(t *testing.T) {
	serverRepo := inmemory.NewServerRepository()
	taskRepo := inmemory.NewServerTaskRepository(serverRepo)

	ctx := context.Background()

	server1 := &domain.Server{
		UUID:      uuid.New(),
		UUIDShort: "test1",
		Name:      "Server 1",
		DSID:      1,
		GameID:    "game1",
	}
	server2 := &domain.Server{
		UUID:      uuid.New(),
		UUIDShort: "test2",
		Name:      "Server 2",
		DSID:      1,
		GameID:    "game1",
	}

	server3 := &domain.Server{
		UUID:      uuid.New(),
		UUIDShort: "test3",
		Name:      "Server 3",
		DSID:      2,
		GameID:    "game1",
	}

	if err := serverRepo.Save(ctx, server1); err != nil {
		t.Fatalf("Failed to save server1: %v", err)
	}
	if err := serverRepo.Save(ctx, server2); err != nil {
		t.Fatalf("Failed to save server2: %v", err)
	}
	if err := serverRepo.Save(ctx, server3); err != nil {
		t.Fatalf("Failed to save server3: %v", err)
	}

	task1 := &domain.ServerTask{
		Command:     domain.ServerTaskCommandStart,
		ServerID:    server1.ID,
		ExecuteDate: time.Now(),
	}
	task2 := &domain.ServerTask{
		Command:     domain.ServerTaskCommandStop,
		ServerID:    server2.ID,
		ExecuteDate: time.Now(),
	}
	task3 := &domain.ServerTask{
		Command:     domain.ServerTaskCommandRestart,
		ServerID:    server3.ID,
		ExecuteDate: time.Now(),
	}

	if err := taskRepo.Save(ctx, task1); err != nil {
		t.Fatalf("Failed to save task1: %v", err)
	}
	if err := taskRepo.Save(ctx, task2); err != nil {
		t.Fatalf("Failed to save task2: %v", err)
	}
	if err := taskRepo.Save(ctx, task3); err != nil {
		t.Fatalf("Failed to save task3: %v", err)
	}

	t.Run("Filter_by_Node_1", func(t *testing.T) {
		filter := &filters.FindServerTask{
			NodeIDs: []uint{1},
		}
		tasks, err := taskRepo.Find(ctx, filter, nil, nil)
		if err != nil {
			t.Fatalf("Failed to find tasks: %v", err)
		}

		if len(tasks) != 2 {
			t.Errorf("Expected 2 tasks for node 1, got %d", len(tasks))
		}

		for _, task := range tasks {
			if task.ServerID != server1.ID && task.ServerID != server2.ID {
				t.Errorf("Task %d belongs to unexpected server %d", task.ID, task.ServerID)
			}
		}
	})

	t.Run("Filter_by_Node_2", func(t *testing.T) {
		filter := &filters.FindServerTask{
			NodeIDs: []uint{2},
		}
		tasks, err := taskRepo.Find(ctx, filter, nil, nil)
		if err != nil {
			t.Fatalf("Failed to find tasks: %v", err)
		}

		if len(tasks) != 1 {
			t.Errorf("Expected 1 task for node 2, got %d", len(tasks))
		}

		if len(tasks) > 0 && tasks[0].ServerID != server3.ID {
			t.Errorf("Task belongs to unexpected server %d", tasks[0].ServerID)
		}
	})

	t.Run("Filter_by_Multiple_Nodes", func(t *testing.T) {
		filter := &filters.FindServerTask{
			NodeIDs: []uint{1, 2},
		}
		tasks, err := taskRepo.Find(ctx, filter, nil, nil)
		if err != nil {
			t.Fatalf("Failed to find tasks: %v", err)
		}

		if len(tasks) != 3 {
			t.Errorf("Expected 3 tasks for nodes 1 and 2, got %d", len(tasks))
		}
	})

	t.Run("Filter_by_Non_existent_Node", func(t *testing.T) {
		filter := &filters.FindServerTask{
			NodeIDs: []uint{999},
		}
		tasks, err := taskRepo.Find(ctx, filter, nil, nil)
		if err != nil {
			t.Fatalf("Failed to find tasks: %v", err)
		}

		if len(tasks) != 0 {
			t.Errorf("Expected 0 tasks for non-existent node, got %d", len(tasks))
		}
	})

	t.Run("Filter_by_Node_and_Command", func(t *testing.T) {
		filter := &filters.FindServerTask{
			NodeIDs:  []uint{1},
			Commands: []domain.ServerTaskCommand{domain.ServerTaskCommandStart},
		}
		tasks, err := taskRepo.Find(ctx, filter, nil, nil)
		if err != nil {
			t.Fatalf("Failed to find tasks: %v", err)
		}

		if len(tasks) != 1 {
			t.Errorf("Expected 1 task (start command on node 1), got %d", len(tasks))
		}

		if len(tasks) > 0 {
			if tasks[0].Command != domain.ServerTaskCommandStart {
				t.Errorf("Expected start command, got %s", tasks[0].Command)
			}
			if tasks[0].ServerID != server1.ID {
				t.Errorf("Expected server %d, got %d", server1.ID, tasks[0].ServerID)
			}
		}
	})
}

func TestServerTaskRepository_FilterByNodeIDs_WithoutServerRepo(t *testing.T) {
	taskRepo := inmemory.NewServerTaskRepository(nil)

	ctx := context.Background()

	task := &domain.ServerTask{
		Command:     domain.ServerTaskCommandStart,
		ServerID:    1,
		ExecuteDate: time.Now(),
	}

	if err := taskRepo.Save(ctx, task); err != nil {
		t.Fatalf("Failed to save task: %v", err)
	}

	filter := &filters.FindServerTask{
		NodeIDs: []uint{1},
	}
	tasks, err := taskRepo.Find(ctx, filter, nil, nil)
	if err != nil {
		t.Fatalf("Failed to find tasks: %v", err)
	}

	if len(tasks) != 0 {
		t.Errorf("Expected 0 tasks when ServerRepository is not set, got %d", len(tasks))
	}
}
