package testing

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/base"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RunPluginStorageScopeCollapseTests covers the two ways a global scope ends up
// holding more than one row. The unique index cannot prevent either — NULLs
// never conflict there — so the repository has to collapse the scope itself.
func RunPluginStorageScopeCollapseTests(
	t *testing.T,
	db base.DB,
	newRepo func(db base.DB) repositories.PluginStorageRepository,
) {
	t.Helper()

	ctx := context.Background()
	repo := newRepo(db)

	newEntry := func(pluginID uint64, payload string) *domain.PluginStorageEntry {
		return &domain.PluginStorageEntry{
			PluginID: pluginID,
			Key:      "rsp:account",
			Payload:  []byte(payload),
		}
	}
	scopeOf := func(pluginID uint64) []domain.PluginStorageEntry {
		results, err := repo.Find(ctx, &filters.FindPluginStorage{PluginIDs: []uint64{pluginID}}, nil, nil)
		require.NoError(t, err)

		return results
	}

	t.Run("save_racing_another_instance_leaves_one_row", func(t *testing.T) {
		pluginID := uint64(920)

		racing := &scriptedDB{DB: db, beforeInsert: func() {
			require.NoError(t, repo.Save(ctx, newEntry(pluginID, `{"state":"registering"}`)))
		}}

		saved := newEntry(pluginID, `{"state":"active"}`)
		require.NoError(t, newRepo(racing).Save(ctx, saved))

		results := scopeOf(pluginID)
		require.Len(t, results, 1, "the row the racing save left behind is dropped")
		assert.Equal(t, []byte(`{"state":"active"}`), results[0].Payload)
		assert.Equal(t, saved.ID, results[0].ID, "the save reports the row that survived")
	})

	t.Run("cleanup_is_retried_by_the_next_save", func(t *testing.T) {
		pluginID := uint64(921)
		cleanupErr := errors.New("connection reset")

		racing := &scriptedDB{
			DB: db,
			beforeInsert: func() {
				require.NoError(t, repo.Save(ctx, newEntry(pluginID, `{"state":"registering"}`)))
			},
			deleteErr: cleanupErr,
		}

		err := newRepo(racing).Save(ctx, newEntry(pluginID, `{"state":"active"}`))
		require.ErrorIs(t, err, cleanupErr)
		require.ErrorContains(t, err, "failed to execute scope cleanup query")
		require.Len(t, scopeOf(pluginID), 2, "the cleanup that errored leaves the scope duplicated")

		retried := newEntry(pluginID, `{"state":"suspended"}`)
		require.NoError(t, repo.Save(ctx, retried))

		results := scopeOf(pluginID)
		require.Len(t, results, 1, "the next save collapses the scope it found duplicated")
		assert.Equal(t, []byte(`{"state":"suspended"}`), results[0].Payload)
		assert.Equal(t, retried.ID, results[0].ID)
	})
}

// scriptedDB drives the windows a save has to survive: beforeInsert runs once
// right before the first insert reaches the database — where a save from
// another instance slips in — and deleteErr replaces the first cleanup delete
// with the error it holds.
type scriptedDB struct {
	base.DB

	insertOnce   sync.Once
	beforeInsert func()

	deleteOnce sync.Once
	deleteErr  error
}

func (d *scriptedDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	d.race(query)

	if err := d.interceptDelete(query); err != nil {
		return nil, err
	}

	return d.DB.ExecContext(ctx, query, args...)
}

func (d *scriptedDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	d.race(query)

	return d.DB.QueryRowContext(ctx, query, args...)
}

func (d *scriptedDB) race(query string) {
	if d.beforeInsert == nil || !strings.Contains(query, "INSERT INTO") {
		return
	}

	d.insertOnce.Do(d.beforeInsert)
}

func (d *scriptedDB) interceptDelete(query string) error {
	if d.deleteErr == nil || !strings.Contains(query, "DELETE FROM") {
		return nil
	}

	var err error
	d.deleteOnce.Do(func() {
		err = d.deleteErr
	})

	return err
}
