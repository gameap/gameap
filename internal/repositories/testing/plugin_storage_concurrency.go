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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RunPluginStorageConcurrentInsertTest saves a global entry while another
// instance fills the same scope in the window between the scope lookup and the
// insert. The unique index lets both rows in — NULLs never conflict there — so
// the repository has to drop the loser itself.
func RunPluginStorageConcurrentInsertTest(
	t *testing.T,
	db base.DB,
	newRepo func(db base.DB) repositories.PluginStorageRepository,
) {
	t.Helper()

	ctx := context.Background()
	pluginID := uint64(920)

	newEntry := func(payload string) *domain.PluginStorageEntry {
		return &domain.PluginStorageEntry{
			PluginID: pluginID,
			Key:      "rsp:account",
			Payload:  []byte(payload),
		}
	}

	otherInstance := newRepo(db)
	racing := &insertRacerDB{
		DB: db,
		onInsert: func() {
			require.NoError(t, otherInstance.Save(ctx, newEntry(`{"state":"registering"}`)))
		},
	}

	saved := newEntry(`{"state":"active"}`)
	require.NoError(t, newRepo(racing).Save(ctx, saved))

	results, err := otherInstance.Find(ctx, &filters.FindPluginStorage{PluginIDs: []uint64{pluginID}}, nil, nil)
	require.NoError(t, err)
	require.Len(t, results, 1, "the row the racing save left behind is dropped")
	assert.Equal(t, []byte(`{"state":"active"}`), results[0].Payload)
	assert.Equal(t, saved.ID, results[0].ID, "the save reports the row that survived")
}

// insertRacerDB runs onInsert once, right before the first insert reaches the
// database — the window a concurrent save slips through.
type insertRacerDB struct {
	base.DB

	once     sync.Once
	onInsert func()
}

func (d *insertRacerDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	d.race(query)

	return d.DB.ExecContext(ctx, query, args...)
}

func (d *insertRacerDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	d.race(query)

	return d.DB.QueryRowContext(ctx, query, args...)
}

func (d *insertRacerDB) race(query string) {
	if !strings.Contains(query, "INSERT INTO") {
		return
	}

	d.once.Do(d.onInsert)
}
