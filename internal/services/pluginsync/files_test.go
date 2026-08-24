package pluginsync_test

import (
	"testing"
	"time"

	"github.com/gameap/gameap/internal/domain"
	internalplugin "github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/services/pluginsync"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFiles_missing_file_is_restored_from_the_store_under_a_lock(t *testing.T) {
	t.Parallel()
	e := newEnv(t, activeRow(1))
	e.files.files = map[string][]byte{}

	e.pass(t)

	assert.True(t, e.loader.isRunning(1))
	assert.Equal(t, []string{"store-b@1.0.0"}, e.store.downloads)
	assert.Equal(t, []string{"pluginsync:download:store-b:1.0.0"}, e.locks.keys)
	assert.True(t, e.files.Exists(e.ctx, "plugins/pb.wasm"))
}

func TestFiles_mismatching_file_is_refetched(t *testing.T) {
	t.Parallel()
	e := newEnv(t, activeRow(1))
	require.NoError(t, e.files.Write(e.ctx, "plugins/pb.wasm", []byte("stale")))

	e.pass(t)

	assert.Len(t, e.store.downloads, 1)
	data, err := e.files.Read(e.ctx, "plugins/pb.wasm")
	require.NoError(t, err)
	assert.Equal(t, wasmContent, string(data))
}

func TestFiles_unrecoverable_cases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*env)
		wantErr error
	}{
		{
			name: "uploaded_by_hand",
			mutate: func(e *env) {
				e.repo.update(1, func(p *domain.Plugin) { p.Source = new("file://pb.wasm") })
			},
			wantErr: pluginsync.ErrNotStoreSourced,
		},
		{
			name: "no_source",
			mutate: func(e *env) {
				e.repo.update(1, func(p *domain.Plugin) { p.Source = nil })
			},
			wantErr: pluginsync.ErrNotStoreSourced,
		},
		{
			name: "no_checksum",
			mutate: func(e *env) {
				e.repo.update(1, func(p *domain.Plugin) { p.Checksum = nil })
			},
			wantErr: pluginsync.ErrChecksumUnknown,
		},
		{
			name: "store_serves_other_bytes",
			mutate: func(e *env) {
				e.store.data = []byte("tampered")
			},
			wantErr: pluginsync.ErrChecksumMismatch,
		},
		{
			name: "store_unreachable",
			mutate: func(e *env) {
				e.store.err = errors.New("503")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t, activeRow(1))
			e.files.files = map[string][]byte{}
			tt.mutate(e)

			e.pass(t)

			assert.False(t, e.loader.isRunning(1))
			assert.Equal(t, 0, e.loader.applyCount(), "nothing is loaded without the file")

			status := e.service.Snapshot()[1]
			assert.Equal(t, pluginsync.StateRetrying, status.State)
			if tt.wantErr != nil {
				assert.Equal(t, tt.wantErr.Error(), status.LastError)
			}
		})
	}
}

func TestFiles_download_locked_by_a_peer_is_contention(t *testing.T) {
	t.Parallel()
	e := newEnv(t, activeRow(1))
	e.files.files = map[string][]byte{}
	e.locks.locked["pluginsync:download:store-b:1.0.0"] = true

	e.pass(t)

	status := e.service.Snapshot()[1]
	assert.Equal(t, pluginsync.StateRetrying, status.State)
	assert.Equal(t, 0, status.Failures)
	assert.Equal(t, pluginsync.ErrDownloadLocked.Error(), status.LastError)
	assert.Equal(t, e.clock.Now().Add(10*time.Second), status.NextAttempt)
	assert.Empty(t, e.store.downloads)

	// The peer finished: the file is there on the next pass.
	require.NoError(t, e.files.Write(e.ctx, "plugins/pb.wasm", []byte(wasmContent)))
	e.clock.advance(10 * time.Second)
	e.pass(t)
	assert.True(t, e.loader.isRunning(1))
	assert.Empty(t, e.store.downloads)
}

func TestFiles_row_without_checksum_trusts_the_present_file(t *testing.T) {
	t.Parallel()
	row := activeRow(1)
	row.Checksum = nil
	e := newEnv(t, row)
	require.NoError(t, e.files.Write(e.ctx, "plugins/pb.wasm", []byte("whatever")))

	e.pass(t)
	assert.True(t, e.loader.isRunning(1))
	assert.Empty(t, e.store.downloads)
}

func TestFiles_no_store_configured(t *testing.T) {
	t.Parallel()
	e := newEnv(t, activeRow(1))
	e.files.files = map[string][]byte{}
	e.service = nil

	svc := newServiceWithoutStore(t, e)
	require.NoError(t, svc.ReconcileNow(e.ctx))

	status := svc.Snapshot()[1]
	assert.Equal(t, pluginsync.ErrNoStoreConfigured.Error(), status.LastError)
	assert.Equal(t, internalplugin.RuntimeState{}, e.loader.RuntimeState(1))
}
