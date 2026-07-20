package testing

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ServerTaskExecutionRepositorySuite struct {
	suite.Suite

	repo repositories.ServerTaskExecutionRepository
	fn   func(t *testing.T) repositories.ServerTaskExecutionRepository
}

func NewServerTaskExecutionRepositorySuite(
	fn func(t *testing.T) repositories.ServerTaskExecutionRepository,
) *ServerTaskExecutionRepositorySuite {
	return &ServerTaskExecutionRepositorySuite{
		fn: fn,
	}
}

func (s *ServerTaskExecutionRepositorySuite) SetupTest() {
	s.repo = s.fn(s.T())
}

// nowTrunc returns a UTC timestamp truncated to seconds. All drivers
// (especially SQLite which serialises to RFC3339) round-trip cleanly at
// second precision.
func nowTrunc() time.Time {
	return time.Now().UTC().Truncate(time.Second)
}

// newExecution returns a fully populated running execution suitable for
// Create. Tests override fields they care about.
func newExecution(executionID xid.ID, startedAt time.Time) *domain.ServerTaskExecution {
	return &domain.ServerTaskExecution{
		ExecutionID:  executionID,
		ServerTaskID: 1,
		ServerID:     1,
		NodeID:       1,
		Command:      domain.ServerTaskCommandStart,
		TaskVersion:  1,
		Status:       domain.ServerTaskExecutionStatusRunning,
		StartedAt:    startedAt,
	}
}

func (s *ServerTaskExecutionRepositorySuite) TestServerTaskExecutionRepositoryCreate() {
	ctx := context.Background()

	s.T().Run("inserts_execution_and_assigns_id", func(t *testing.T) {
		// ARRANGE
		execID := xid.New()
		exec := newExecution(execID, nowTrunc())

		// ACT
		err := s.repo.Create(ctx, exec)

		// ASSERT
		require.NoError(t, err)
		assert.NotZero(t, exec.ID, "Create must assign an auto-increment ID")
		assert.False(t, exec.CreatedAt.IsZero(), "Create must set CreatedAt")
		assert.False(t, exec.UpdatedAt.IsZero(), "Create must set UpdatedAt")

		results, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ExecutionIDs: []xid.ID{execID},
		}, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, execID, results[0].ExecutionID, "ExecutionID must round-trip")
		assert.Equal(t, domain.ServerTaskCommandStart, results[0].Command)
		assert.Equal(t, domain.ServerTaskExecutionStatusRunning, results[0].Status)
	})

	s.T().Run("idempotent_on_duplicate_execution_id", func(t *testing.T) {
		// ARRANGE
		execID := xid.New()
		startedAt := nowTrunc()
		first := newExecution(execID, startedAt)
		second := newExecution(execID, startedAt)
		second.Command = domain.ServerTaskCommandStop

		// ACT
		require.NoError(t, s.repo.Create(ctx, first))
		err := s.repo.Create(ctx, second)

		// ASSERT
		require.NoError(t, err, "duplicate ExecutionID must be silently ignored")

		results, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ExecutionIDs: []xid.ID{execID},
		}, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1, "duplicate Create must not insert a second row")
		assert.Equal(t, domain.ServerTaskCommandStart, results[0].Command,
			"first-write-wins: second Create must not overwrite")
	})

	s.T().Run("find_by_execution_ids_returns_match", func(t *testing.T) {
		// ARRANGE
		execA := xid.New()
		execB := xid.New()
		require.NoError(t, s.repo.Create(ctx, newExecution(execA, nowTrunc())))
		require.NoError(t, s.repo.Create(ctx, newExecution(execB, nowTrunc())))

		// ACT
		results, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ExecutionIDs: []xid.ID{execA},
		}, nil, nil)

		// ASSERT
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, execA, results[0].ExecutionID)
	})
}

func (s *ServerTaskExecutionRepositorySuite) TestServerTaskExecutionRepositoryUpdateFinish() {
	ctx := context.Background()

	s.T().Run("updates_all_fields", func(t *testing.T) {
		// ARRANGE
		execID := xid.New()
		require.NoError(t, s.repo.Create(ctx, newExecution(execID, nowTrunc())))

		finishedAt := nowTrunc().Add(time.Minute)
		exitCode := 0
		errorMsg := "completed"
		duration := int64(60000)
		outputInline := "all good"
		outputPath := "/tmp/out.log"

		patch := repositories.ServerTaskExecutionFinishPatch{
			Status:            domain.ServerTaskExecutionStatusSuccess,
			ExitCode:          &exitCode,
			ErrorMessage:      &errorMsg,
			FinishedAt:        finishedAt,
			DurationMS:        &duration,
			OutputInline:      &outputInline,
			OutputStoragePath: &outputPath,
		}

		// ACT
		err := s.repo.UpdateFinish(ctx, execID, patch)

		// ASSERT
		require.NoError(t, err)

		results, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ExecutionIDs: []xid.ID{execID},
		}, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)

		got := results[0]
		assert.Equal(t, domain.ServerTaskExecutionStatusSuccess, got.Status)
		require.NotNil(t, got.ExitCode)
		assert.Equal(t, exitCode, *got.ExitCode)
		require.NotNil(t, got.ErrorMessage)
		assert.Equal(t, errorMsg, *got.ErrorMessage)
		require.NotNil(t, got.FinishedAt)
		assert.True(t, got.FinishedAt.Equal(finishedAt),
			"finished_at must round-trip: want %s got %s", finishedAt, got.FinishedAt)
		require.NotNil(t, got.DurationMS)
		assert.Equal(t, duration, *got.DurationMS)
		require.NotNil(t, got.OutputInline)
		assert.Equal(t, outputInline, *got.OutputInline)
		require.NotNil(t, got.OutputStoragePath)
		assert.Equal(t, outputPath, *got.OutputStoragePath)
	})

	s.T().Run("nil_patch_fields_preserve_existing_values", func(t *testing.T) {
		// ARRANGE
		execID := xid.New()
		require.NoError(t, s.repo.Create(ctx, newExecution(execID, nowTrunc())))

		initialExit := 7
		initialErr := "boom"
		initialDuration := int64(123)
		initialOutput := "first run"
		initialPath := "/var/log/first"

		firstPatch := repositories.ServerTaskExecutionFinishPatch{
			Status:            domain.ServerTaskExecutionStatusFailed,
			ExitCode:          &initialExit,
			ErrorMessage:      &initialErr,
			FinishedAt:        nowTrunc().Add(time.Minute),
			DurationMS:        &initialDuration,
			OutputInline:      &initialOutput,
			OutputStoragePath: &initialPath,
		}
		require.NoError(t, s.repo.UpdateFinish(ctx, execID, firstPatch))

		// ACT — second call with nil pointers must leave existing columns alone
		secondPatch := repositories.ServerTaskExecutionFinishPatch{
			Status:     domain.ServerTaskExecutionStatusCanceled,
			FinishedAt: nowTrunc().Add(2 * time.Minute),
		}
		err := s.repo.UpdateFinish(ctx, execID, secondPatch)

		// ASSERT
		require.NoError(t, err)

		results, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ExecutionIDs: []xid.ID{execID},
		}, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)

		got := results[0]
		assert.Equal(t, domain.ServerTaskExecutionStatusCanceled, got.Status,
			"status must always be overwritten")
		require.NotNil(t, got.ExitCode)
		assert.Equal(t, initialExit, *got.ExitCode, "nil ExitCode must preserve prior value")
		require.NotNil(t, got.ErrorMessage)
		assert.Equal(t, initialErr, *got.ErrorMessage, "nil ErrorMessage must preserve prior value")
		require.NotNil(t, got.DurationMS)
		assert.Equal(t, initialDuration, *got.DurationMS, "nil DurationMS must preserve prior value")
		require.NotNil(t, got.OutputInline)
		assert.Equal(t, initialOutput, *got.OutputInline,
			"nil OutputInline must preserve prior value")
		require.NotNil(t, got.OutputStoragePath)
		assert.Equal(t, initialPath, *got.OutputStoragePath,
			"nil OutputStoragePath must preserve prior value")
	})

	s.T().Run("unknown_execution_id_is_noop_or_silent", func(t *testing.T) {
		// ARRANGE
		patch := repositories.ServerTaskExecutionFinishPatch{
			Status:     domain.ServerTaskExecutionStatusSuccess,
			FinishedAt: nowTrunc(),
		}

		// ACT
		err := s.repo.UpdateFinish(ctx, xid.New(), patch)

		// ASSERT
		require.NoError(t, err, "updating a non-existent execution must not fail")
	})
}

func (s *ServerTaskExecutionRepositorySuite) TestServerTaskExecutionRepositoryAppendOutputInline() {
	ctx := context.Background()

	s.T().Run("first_append_writes_chunk", func(t *testing.T) {
		// ARRANGE
		execID := xid.New()
		require.NoError(t, s.repo.Create(ctx, newExecution(execID, nowTrunc())))

		// ACT
		err := s.repo.AppendOutputInline(ctx, execID, []byte("hello"), 1024)

		// ASSERT
		require.NoError(t, err)

		results, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ExecutionIDs: []xid.ID{execID},
		}, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.NotNil(t, results[0].OutputInline)
		assert.Equal(t, "hello", *results[0].OutputInline)
	})

	s.T().Run("subsequent_append_concatenates", func(t *testing.T) {
		// ARRANGE
		execID := xid.New()
		require.NoError(t, s.repo.Create(ctx, newExecution(execID, nowTrunc())))

		// ACT
		require.NoError(t, s.repo.AppendOutputInline(ctx, execID, []byte("hello"), 1024))
		err := s.repo.AppendOutputInline(ctx, execID, []byte(" world"), 1024)

		// ASSERT
		require.NoError(t, err)

		results, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ExecutionIDs: []xid.ID{execID},
		}, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.NotNil(t, results[0].OutputInline)
		assert.Equal(t, "hello world", *results[0].OutputInline)
	})

	s.T().Run("truncates_to_max_bytes_keeping_tail", func(t *testing.T) {
		// ARRANGE
		execID := xid.New()
		require.NoError(t, s.repo.Create(ctx, newExecution(execID, nowTrunc())))

		// ACT — total appended = "0123456789abcdef" (16 bytes), maxBytes = 10,
		// so the stored tail must be "6789abcdef" (last 10).
		require.NoError(t, s.repo.AppendOutputInline(ctx, execID, []byte("0123456789"), 10))
		err := s.repo.AppendOutputInline(ctx, execID, []byte("abcdef"), 10)

		// ASSERT
		require.NoError(t, err)

		results, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ExecutionIDs: []xid.ID{execID},
		}, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.NotNil(t, results[0].OutputInline)
		assert.Equal(t, "6789abcdef", *results[0].OutputInline,
			"AppendOutputInline must keep only the trailing maxBytes characters")
		assert.Len(t, *results[0].OutputInline, 10,
			"stored value must equal maxBytes when over-flowing")
	})
}

func (s *ServerTaskExecutionRepositorySuite) TestServerTaskExecutionRepositoryFindAll() {
	ctx := context.Background()

	s.T().Run("returns_all_with_default_sort_started_at_desc", func(t *testing.T) {
		// ARRANGE — three executions with distinct started_at values.
		base := nowTrunc().Add(-time.Hour)
		execIDs := []xid.ID{xid.New(), xid.New(), xid.New()}
		execs := []*domain.ServerTaskExecution{
			newExecution(execIDs[0], base),
			newExecution(execIDs[1], base.Add(time.Minute)),
			newExecution(execIDs[2], base.Add(2*time.Minute)),
		}
		for _, e := range execs {
			require.NoError(t, s.repo.Create(ctx, e))
		}

		// ACT
		results, err := s.repo.FindAll(ctx, nil, nil)

		// ASSERT — default ordering is started_at DESC.
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(results), 3)

		for i := 0; i < len(results)-1; i++ {
			assert.False(t, results[i].StartedAt.Before(results[i+1].StartedAt),
				"FindAll default order must be started_at DESC")
		}
	})

	s.T().Run("applies_pagination_offset_limit", func(t *testing.T) {
		// ARRANGE — 5 executions, each with a distinct started_at.
		base := nowTrunc().Add(-2 * time.Hour)
		for i := range 5 {
			e := newExecution(xid.New(), base.Add(time.Duration(i)*time.Minute))
			e.ServerID = 9001
			require.NoError(t, s.repo.Create(ctx, e))
		}

		// ACT
		all, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ServerIDs: []uint{9001},
		}, nil, nil)
		require.NoError(t, err)
		require.Len(t, all, 5)

		page, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ServerIDs: []uint{9001},
		}, nil, &filters.Pagination{Limit: 2, Offset: 1})

		// ASSERT
		require.NoError(t, err)
		require.Len(t, page, 2, "pagination must respect Limit")
		assert.Equal(t, all[1].ExecutionID, page[0].ExecutionID,
			"pagination must skip exactly Offset records")
		assert.Equal(t, all[2].ExecutionID, page[1].ExecutionID)
	})
}

func (s *ServerTaskExecutionRepositorySuite) TestServerTaskExecutionRepositoryFind() {
	ctx := context.Background()

	// Build a varied population we can probe by every filter.
	base := nowTrunc().Add(-3 * time.Hour)

	execAID := xid.New()
	execA := newExecution(execAID, base)
	execA.ServerTaskID = 100
	execA.ServerID = 200
	execA.NodeID = 10
	execA.Status = domain.ServerTaskExecutionStatusRunning

	execBID := xid.New()
	execB := newExecution(execBID, base.Add(time.Hour))
	execB.ServerTaskID = 100
	execB.ServerID = 201
	execB.NodeID = 10
	execB.Status = domain.ServerTaskExecutionStatusSuccess

	execCID := xid.New()
	execC := newExecution(execCID, base.Add(2*time.Hour))
	execC.ServerTaskID = 101
	execC.ServerID = 202
	execC.NodeID = 11
	execC.Status = domain.ServerTaskExecutionStatusFailed

	execDID := xid.New()
	execD := newExecution(execDID, base.Add(3*time.Hour))
	execD.ServerTaskID = 101
	execD.ServerID = 203
	execD.NodeID = 11
	execD.Status = domain.ServerTaskExecutionStatusRunning

	for _, e := range []*domain.ServerTaskExecution{execA, execB, execC, execD} {
		require.NoError(s.T(), s.repo.Create(ctx, e))
	}

	s.T().Run("filters_by_ids", func(t *testing.T) {
		// ARRANGE
		needA, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ExecutionIDs: []xid.ID{execAID},
		}, nil, nil)
		require.NoError(t, err)
		require.Len(t, needA, 1)
		idA := needA[0].ID

		// ACT
		results, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			IDs: []uint{idA},
		}, nil, nil)

		// ASSERT
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, execAID, results[0].ExecutionID)
	})

	s.T().Run("filters_by_execution_ids", func(t *testing.T) {
		// ACT
		results, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ExecutionIDs: []xid.ID{execAID, execCID},
		}, nil, nil)

		// ASSERT
		require.NoError(t, err)
		require.Len(t, results, 2)
		ids := []xid.ID{results[0].ExecutionID, results[1].ExecutionID}
		assert.Contains(t, ids, execAID)
		assert.Contains(t, ids, execCID)
	})

	s.T().Run("filters_by_server_task_ids", func(t *testing.T) {
		// ACT
		results, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ServerTaskIDs: []uint{100},
		}, nil, nil)

		// ASSERT
		require.NoError(t, err)
		require.Len(t, results, 2)
		for _, got := range results {
			assert.Equal(t, uint(100), got.ServerTaskID)
		}
	})

	s.T().Run("filters_by_server_ids", func(t *testing.T) {
		// ACT
		results, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ServerIDs: []uint{201, 203},
		}, nil, nil)

		// ASSERT
		require.NoError(t, err)
		require.Len(t, results, 2)
		serverIDs := []uint{results[0].ServerID, results[1].ServerID}
		assert.Contains(t, serverIDs, uint(201))
		assert.Contains(t, serverIDs, uint(203))
	})

	s.T().Run("filters_by_node_ids", func(t *testing.T) {
		// ACT
		results, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			NodeIDs: []uint{10},
		}, nil, nil)

		// ASSERT
		require.NoError(t, err)
		require.Len(t, results, 2)
		for _, got := range results {
			assert.Equal(t, uint(10), got.NodeID)
		}
	})

	s.T().Run("filters_by_statuses_in", func(t *testing.T) {
		// ACT
		results, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			Statuses: []domain.ServerTaskExecutionStatus{
				domain.ServerTaskExecutionStatusFailed,
				domain.ServerTaskExecutionStatusSuccess,
			},
		}, nil, nil)

		// ASSERT — execB (success) + execC (failed).
		require.NoError(t, err)
		require.Len(t, results, 2)
		statuses := []domain.ServerTaskExecutionStatus{
			results[0].Status, results[1].Status,
		}
		assert.Contains(t, statuses, domain.ServerTaskExecutionStatusSuccess)
		assert.Contains(t, statuses, domain.ServerTaskExecutionStatusFailed)
	})

	s.T().Run("filters_by_not_statuses_excludes", func(t *testing.T) {
		// ACT
		results, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			NotStatuses: []domain.ServerTaskExecutionStatus{
				domain.ServerTaskExecutionStatusRunning,
			},
			ServerTaskIDs: []uint{100, 101},
		}, nil, nil)

		// ASSERT — execB (success) + execC (failed); execA and execD excluded.
		require.NoError(t, err)
		require.Len(t, results, 2)
		for _, got := range results {
			assert.NotEqual(t, domain.ServerTaskExecutionStatusRunning, got.Status,
				"NotStatuses must exclude listed statuses")
		}
	})

	s.T().Run("filters_by_started_after", func(t *testing.T) {
		// ARRANGE — strictly after execA.StartedAt, so we exclude execA.
		threshold := base.Add(30 * time.Minute)

		// ACT
		results, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ServerTaskIDs: []uint{100, 101},
			StartedAfter:  &threshold,
		}, nil, nil)

		// ASSERT
		require.NoError(t, err)
		require.Len(t, results, 3, "StartedAfter must include B, C, D and exclude A")
		for _, got := range results {
			assert.False(t, got.StartedAt.Before(threshold),
				"each result.StartedAt must be >= StartedAfter threshold")
		}
	})

	s.T().Run("filters_by_started_before", func(t *testing.T) {
		// ARRANGE — strictly before execD.StartedAt.
		threshold := base.Add(2*time.Hour + 30*time.Minute)

		// ACT
		results, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ServerTaskIDs: []uint{100, 101},
			StartedBefore: &threshold,
		}, nil, nil)

		// ASSERT
		require.NoError(t, err)
		require.Len(t, results, 3, "StartedBefore must include A, B, C and exclude D")
		for _, got := range results {
			assert.True(t, got.StartedAt.Before(threshold),
				"each result.StartedAt must be strictly less than StartedBefore")
		}
	})

	s.T().Run("combines_filters_with_and", func(t *testing.T) {
		// ARRANGE — server_task_id=100 AND status=running AND started_after=<execA-30s>
		// matches only execA.
		threshold := base.Add(-30 * time.Second)

		// ACT
		results, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ServerTaskIDs: []uint{100},
			Statuses:      []domain.ServerTaskExecutionStatus{domain.ServerTaskExecutionStatusRunning},
			StartedAfter:  &threshold,
		}, nil, nil)

		// ASSERT
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, execAID, results[0].ExecutionID,
			"combined filter must intersect, not union")
	})
}

func (s *ServerTaskExecutionRepositorySuite) TestServerTaskExecutionRepositoryMarkAbandoned() {
	ctx := context.Background()

	s.T().Run("marks_running_executions_for_node_as_failed", func(t *testing.T) {
		// ARRANGE — 3 running on node 1, 1 success on node 1, 2 running on node 2.
		nodeOneRunningIDs := []xid.ID{xid.New(), xid.New(), xid.New()}
		nodeOneSuccessID := xid.New()
		nodeTwoRunningIDs := []xid.ID{xid.New(), xid.New()}

		for _, id := range nodeOneRunningIDs {
			e := newExecution(id, nowTrunc())
			e.NodeID = 1
			e.ServerID = 7100
			require.NoError(t, s.repo.Create(ctx, e))
		}
		nodeOneSuccess := newExecution(nodeOneSuccessID, nowTrunc())
		nodeOneSuccess.NodeID = 1
		nodeOneSuccess.ServerID = 7100
		nodeOneSuccess.Status = domain.ServerTaskExecutionStatusSuccess
		require.NoError(t, s.repo.Create(ctx, nodeOneSuccess))
		for _, id := range nodeTwoRunningIDs {
			e := newExecution(id, nowTrunc())
			e.NodeID = 2
			e.ServerID = 7200
			require.NoError(t, s.repo.Create(ctx, e))
		}

		// ACT
		affected, err := s.repo.MarkAbandoned(ctx, 1, nil, "restart")

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, 3, affected, "must mark exactly the 3 running rows on node 1")

		// All three were running on node 1 → must now be failed with reason set.
		failed, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ExecutionIDs: nodeOneRunningIDs,
		}, nil, nil)
		require.NoError(t, err)
		require.Len(t, failed, 3)
		for _, got := range failed {
			assert.Equal(t, domain.ServerTaskExecutionStatusFailed, got.Status)
			require.NotNil(t, got.ErrorMessage)
			assert.Equal(t, "restart", *got.ErrorMessage)
			assert.NotNil(t, got.FinishedAt, "MarkAbandoned must set finished_at")
		}

		// The already-success row on node 1 must NOT be touched.
		successAfter, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ExecutionIDs: []xid.ID{nodeOneSuccessID},
		}, nil, nil)
		require.NoError(t, err)
		require.Len(t, successAfter, 1)
		assert.Equal(t, domain.ServerTaskExecutionStatusSuccess, successAfter[0].Status,
			"already-finished executions must not be re-marked")

		// Node-2 rows must remain untouched.
		nodeTwoAfter, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ExecutionIDs: nodeTwoRunningIDs,
		}, nil, nil)
		require.NoError(t, err)
		require.Len(t, nodeTwoAfter, 2)
		for _, got := range nodeTwoAfter {
			assert.Equal(t, domain.ServerTaskExecutionStatusRunning, got.Status,
				"MarkAbandoned on node 1 must not touch node 2")
		}
	})

	s.T().Run("preserves_keep_list_executions", func(t *testing.T) {
		// ARRANGE — 3 running on a fresh node.
		nodeID := uint(50)
		running := []xid.ID{xid.New(), xid.New(), xid.New()}
		for _, id := range running {
			e := newExecution(id, nowTrunc())
			e.NodeID = nodeID
			e.ServerID = 5050
			require.NoError(t, s.repo.Create(ctx, e))
		}

		// ACT — keep middle ID.
		affected, err := s.repo.MarkAbandoned(ctx, nodeID, []xid.ID{running[1]}, "restart")

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, 2, affected, "two must be marked, one kept")

		results, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ExecutionIDs: running,
		}, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 3)

		statusByExec := make(map[xid.ID]domain.ServerTaskExecutionStatus, 3)
		for _, got := range results {
			statusByExec[got.ExecutionID] = got.Status
		}
		assert.Equal(t, domain.ServerTaskExecutionStatusFailed, statusByExec[running[0]])
		assert.Equal(t, domain.ServerTaskExecutionStatusRunning, statusByExec[running[1]],
			"kept execution must remain in running state")
		assert.Equal(t, domain.ServerTaskExecutionStatusFailed, statusByExec[running[2]])
	})

	s.T().Run("untouched_when_no_running_on_node", func(t *testing.T) {
		// ARRANGE — only success/failed rows on this node.
		nodeID := uint(60)
		idSuccess := xid.New()
		idFailed := xid.New()

		successExec := newExecution(idSuccess, nowTrunc())
		successExec.NodeID = nodeID
		successExec.ServerID = 6060
		successExec.Status = domain.ServerTaskExecutionStatusSuccess
		require.NoError(t, s.repo.Create(ctx, successExec))

		failedExec := newExecution(idFailed, nowTrunc())
		failedExec.NodeID = nodeID
		failedExec.ServerID = 6060
		failedExec.Status = domain.ServerTaskExecutionStatusFailed
		require.NoError(t, s.repo.Create(ctx, failedExec))

		// ACT
		affected, err := s.repo.MarkAbandoned(ctx, nodeID, nil, "restart")

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, 0, affected, "no rows must be affected when nothing is running")
	})

	s.T().Run("does_not_touch_other_nodes", func(t *testing.T) {
		// ARRANGE — running on node 70 and node 71.
		idNode70 := xid.New()
		idNode71 := xid.New()

		execNode70 := newExecution(idNode70, nowTrunc())
		execNode70.NodeID = 70
		execNode70.ServerID = 7070
		require.NoError(t, s.repo.Create(ctx, execNode70))

		execNode71 := newExecution(idNode71, nowTrunc())
		execNode71.NodeID = 71
		execNode71.ServerID = 7171
		require.NoError(t, s.repo.Create(ctx, execNode71))

		// ACT
		affected, err := s.repo.MarkAbandoned(ctx, 70, nil, "restart")

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, 1, affected)

		results, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ExecutionIDs: []xid.ID{idNode71},
		}, nil, nil)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, domain.ServerTaskExecutionStatusRunning, results[0].Status,
			"executions on other nodes must remain untouched")
	})
}

func (s *ServerTaskExecutionRepositorySuite) TestServerTaskExecutionRepositoryDeleteOlderThan() {
	ctx := context.Background()

	// ARRANGE — a fresh nodeID isolates this test's population from sibling tests.
	const isolatedNode uint = 900
	cutoff := nowTrunc().Add(-time.Hour)

	// failedOld: status=failed, finished_at < cutoff → DELETE.
	failedOldID := xid.New()
	failedOld := newExecution(failedOldID, nowTrunc().Add(-3*time.Hour))
	failedOld.NodeID = isolatedNode
	failedOld.Status = domain.ServerTaskExecutionStatusFailed
	require.NoError(s.T(), s.repo.Create(ctx, failedOld))
	failedOldFinish := nowTrunc().Add(-2 * time.Hour)
	require.NoError(s.T(), s.repo.UpdateFinish(ctx, failedOldID, repositories.ServerTaskExecutionFinishPatch{
		Status:     domain.ServerTaskExecutionStatusFailed,
		FinishedAt: failedOldFinish,
	}))

	// failedNew: status=failed, finished_at > cutoff → KEEP.
	failedNewID := xid.New()
	failedNew := newExecution(failedNewID, nowTrunc().Add(-2*time.Hour))
	failedNew.NodeID = isolatedNode
	failedNew.Status = domain.ServerTaskExecutionStatusFailed
	require.NoError(s.T(), s.repo.Create(ctx, failedNew))
	require.NoError(s.T(), s.repo.UpdateFinish(ctx, failedNewID, repositories.ServerTaskExecutionFinishPatch{
		Status:     domain.ServerTaskExecutionStatusFailed,
		FinishedAt: nowTrunc().Add(-30 * time.Minute),
	}))

	// successOld: status=success, finished_at < cutoff → KEEP (wrong status).
	successOldID := xid.New()
	successOld := newExecution(successOldID, nowTrunc().Add(-3*time.Hour))
	successOld.NodeID = isolatedNode
	successOld.Status = domain.ServerTaskExecutionStatusSuccess
	require.NoError(s.T(), s.repo.Create(ctx, successOld))
	require.NoError(s.T(), s.repo.UpdateFinish(ctx, successOldID, repositories.ServerTaskExecutionFinishPatch{
		Status:     domain.ServerTaskExecutionStatusSuccess,
		FinishedAt: nowTrunc().Add(-2 * time.Hour),
	}))

	// failedOlder: status=failed, finished_at < cutoff (further back) → DELETE.
	failedOlderID := xid.New()
	failedOlder := newExecution(failedOlderID, nowTrunc().Add(-5*time.Hour))
	failedOlder.NodeID = isolatedNode
	failedOlder.Status = domain.ServerTaskExecutionStatusFailed
	require.NoError(s.T(), s.repo.Create(ctx, failedOlder))
	require.NoError(s.T(), s.repo.UpdateFinish(ctx, failedOlderID, repositories.ServerTaskExecutionFinishPatch{
		Status:     domain.ServerTaskExecutionStatusFailed,
		FinishedAt: nowTrunc().Add(-4 * time.Hour),
	}))

	s.T().Run("deletes_only_matching_status_before_cutoff", func(t *testing.T) {
		// ACT
		affected, err := s.repo.DeleteOlderThan(ctx, domain.ServerTaskExecutionStatusFailed, cutoff)

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, 2, affected,
			"only failed + finished_at<cutoff rows must be deleted")

		// failedOld + failedOlder were deleted.
		remaining, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ExecutionIDs: []xid.ID{failedOldID, failedOlderID},
		}, nil, nil)
		require.NoError(t, err)
		assert.Empty(t, remaining, "deleted rows must be gone")
	})

	s.T().Run("keeps_newer_records_of_same_status", func(t *testing.T) {
		// ACT
		remaining, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ExecutionIDs: []xid.ID{failedNewID},
		}, nil, nil)

		// ASSERT
		require.NoError(t, err)
		require.Len(t, remaining, 1,
			"records of the matching status but finished after cutoff must stay")
		assert.Equal(t, failedNewID, remaining[0].ExecutionID)
	})

	s.T().Run("keeps_other_status_even_if_older", func(t *testing.T) {
		// ACT
		remaining, err := s.repo.Find(ctx, &filters.FindServerTaskExecution{
			ExecutionIDs: []xid.ID{successOldID},
		}, nil, nil)

		// ASSERT
		require.NoError(t, err)
		require.Len(t, remaining, 1,
			"records with a different status must stay even when older than cutoff")
		assert.Equal(t, domain.ServerTaskExecutionStatusSuccess, remaining[0].Status)
	})
}

func (s *ServerTaskExecutionRepositorySuite) TestServerTaskExecutionRepositorySorting() {
	ctx := context.Background()

	// ARRANGE — one fixture for the whole method: subtests share the repo.
	base := nowTrunc().Add(-4 * time.Hour)

	execE1 := newExecution(xid.New(), base)
	execE1.ServerID = 9200
	execE1.Status = domain.ServerTaskExecutionStatusRunning

	execE2 := newExecution(xid.New(), base.Add(time.Minute))
	execE2.ServerID = 9200
	execE2.Status = domain.ServerTaskExecutionStatusCanceled

	execE3 := newExecution(xid.New(), base.Add(2*time.Minute))
	execE3.ServerID = 9200
	execE3.Status = domain.ServerTaskExecutionStatusSuccess

	for _, e := range []*domain.ServerTaskExecution{execE1, execE2, execE3} {
		require.NoError(s.T(), s.repo.Create(ctx, e))
	}

	filter := &filters.FindServerTaskExecution{ServerIDs: []uint{9200}}

	tests := []struct {
		name  string
		order []filters.Sorting
		want  []xid.ID
	}{
		{
			name: "sort_by_started_at_asc",
			order: []filters.Sorting{
				{Field: "started_at", Direction: filters.SortDirectionAsc},
			},
			want: []xid.ID{execE1.ExecutionID, execE2.ExecutionID, execE3.ExecutionID},
		},
		{
			name: "sort_by_started_at_desc",
			order: []filters.Sorting{
				{Field: "started_at", Direction: filters.SortDirectionDesc},
			},
			want: []xid.ID{execE3.ExecutionID, execE2.ExecutionID, execE1.ExecutionID},
		},
		{
			name: "sort_by_status_asc",
			order: []filters.Sorting{
				{Field: "status", Direction: filters.SortDirectionAsc},
			},
			want: []xid.ID{execE2.ExecutionID, execE1.ExecutionID, execE3.ExecutionID},
		},
		{
			name: "sort_by_status_desc",
			order: []filters.Sorting{
				{Field: "status", Direction: filters.SortDirectionDesc},
			},
			want: []xid.ID{execE3.ExecutionID, execE1.ExecutionID, execE2.ExecutionID},
		},
	}

	for _, tt := range tests {
		s.T().Run(tt.name, func(t *testing.T) {
			// ACT
			results, err := s.repo.Find(ctx, filter, tt.order, nil)

			// ASSERT
			require.NoError(t, err)
			require.Len(t, results, 3)

			gotIDs := []xid.ID{results[0].ExecutionID, results[1].ExecutionID, results[2].ExecutionID}
			assert.Equal(t, tt.want, gotIDs, "executions are in wrong order")
		})
	}
}
