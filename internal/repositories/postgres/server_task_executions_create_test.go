package postgres_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories/postgres"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServerTaskExecutionRepository_CreateSurfacesNonDuplicateError pins the
// half of the Create contract that the shared suite cannot exercise: a
// constraint violation that is NOT a duplicate execution_id must surface as an
// error rather than being swallowed.
//
// This lives in the PostgreSQL driver (not the shared suite) on purpose. The
// idempotent path is `ON CONFLICT (execution_id) DO NOTHING`; every other error
// has to propagate. SQLite is dynamically typed and enforces no length/range
// constraint reachable through Create, so it cannot produce a non-duplicate
// violation — a shared require.Error subtest would fail there. PostgreSQL
// deterministically rejects an over-length VARCHAR(16) regardless of any server
// mode, giving a stable, non-fragile trigger.
func TestServerTaskExecutionRepository_CreateSurfacesNonDuplicateError(t *testing.T) {
	testPostgresDSN := os.Getenv("TEST_POSTGRES_DSN")

	if testPostgresDSN == "" {
		t.Skip("Skipping PostgreSQL tests because TEST_POSTGRES_DSN is not set")
	}

	// ARRANGE — a valid execution establishes the happy path still works.
	ctx := context.Background()
	repo := postgres.NewServerTaskExecutionRepository(SetupTestDB(t, testPostgresDSN))

	startedAt := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, repo.Create(ctx, newExecutionForCreate(xid.New(), startedAt)))

	// ACT — a DISTINCT execution_id (so ON CONFLICT (execution_id) DO NOTHING
	// cannot claim it) whose command overflows the command VARCHAR(16) column.
	badID := xid.New()
	bad := newExecutionForCreate(badID, startedAt)
	bad.Command = domain.ServerTaskCommand(strings.Repeat("x", 32))
	err := repo.Create(ctx, bad)

	// ASSERT — the violation surfaces, and no partial row is left behind.
	require.Error(t, err, "a non-duplicate constraint violation must surface, not be swallowed")
	assert.Contains(t, err.Error(), "failed to insert execution", "the repository wrapper message must be preserved")

	got, err := repo.Find(ctx, &filters.FindServerTaskExecution{ExecutionIDs: []xid.ID{badID}}, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, got, "the rejected execution must not be persisted")
}

func newExecutionForCreate(execID xid.ID, startedAt time.Time) *domain.ServerTaskExecution {
	return &domain.ServerTaskExecution{
		ExecutionID:  execID,
		ServerTaskID: 1,
		ServerID:     1,
		NodeID:       1,
		Command:      domain.ServerTaskCommandStart,
		TaskVersion:  1,
		Status:       domain.ServerTaskExecutionStatusRunning,
		StartedAt:    startedAt,
	}
}
