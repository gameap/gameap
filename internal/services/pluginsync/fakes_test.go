package pluginsync_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/locker"
	internalplugin "github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/services/pluginsync"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
)

type fakeRepo struct {
	mu      sync.Mutex
	rows    map[domain.Uint64ID]domain.Plugin
	readErr error
	reads   int
}

func newFakeRepo(rows ...domain.Plugin) *fakeRepo {
	repo := &fakeRepo{rows: make(map[domain.Uint64ID]domain.Plugin)}
	for _, row := range rows {
		repo.rows[row.ID] = row
	}

	return repo
}

func (r *fakeRepo) FindAll(context.Context, []filters.Sorting, *filters.Pagination) ([]domain.Plugin, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.reads++

	if r.readErr != nil {
		return nil, r.readErr
	}

	rows := make([]domain.Plugin, 0, len(r.rows))
	for _, row := range r.rows {
		rows = append(rows, row)
	}

	return rows, nil
}

func (r *fakeRepo) readCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.reads
}

func (r *fakeRepo) put(row domain.Plugin) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[row.ID] = row
}

func (r *fakeRepo) remove(id domain.Uint64ID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rows, id)
}

func (r *fakeRepo) update(id domain.Uint64ID, mutate func(*domain.Plugin)) {
	r.mu.Lock()
	defer r.mu.Unlock()

	row := r.rows[id]
	mutate(&row)
	r.rows[id] = row
}

// runningModule is what the fake loader holds for one plugin.
type runningModule struct {
	loaded      *pkgplugin.LoadedPlugin
	fingerprint string
}

// fakeLoader mirrors the contract of internalplugin.Loader: modules keyed by
// database id, fingerprints of what was loaded, the last attempt, holds.
type fakeLoader struct {
	mu        sync.Mutex
	running   map[domain.Uint64ID]*runningModule
	attempted map[domain.Uint64ID]string
	held      map[domain.Uint64ID]bool
	applyErr  map[domain.Uint64ID]error
	applies   []domain.Uint64ID
	unloads   []domain.Uint64ID
	triggers  []string
}

func newFakeLoader() *fakeLoader {
	return &fakeLoader{
		running:   make(map[domain.Uint64ID]*runningModule),
		attempted: make(map[domain.Uint64ID]string),
		held:      make(map[domain.Uint64ID]bool),
		applyErr:  make(map[domain.Uint64ID]error),
	}
}

// preload registers a module as if the startup load had built it from the
// row.
func (l *fakeLoader) preload(row domain.Plugin) *pkgplugin.LoadedPlugin {
	l.mu.Lock()
	defer l.mu.Unlock()

	loaded := &pkgplugin.LoadedPlugin{
		Info:    &proto.PluginInfo{Id: pkgplugin.CompactPluginID(row.ID), Name: row.Name, Version: row.Version},
		Enabled: true,
		DBID:    uint64(row.ID),
	}
	l.running[row.ID] = &runningModule{loaded: loaded, fingerprint: internalplugin.Fingerprint(&row)}
	l.attempted[row.ID] = internalplugin.Fingerprint(&row)

	return loaded
}

func (l *fakeLoader) ApplyRecord(_ context.Context, row *domain.Plugin) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.held[row.ID] {
		return false, internalplugin.ErrPluginHeld
	}

	fingerprint := internalplugin.Fingerprint(row)
	l.applies = append(l.applies, row.ID)

	if module, ok := l.running[row.ID]; ok && module.fingerprint == fingerprint && module.loaded.IsEnabled() {
		return false, nil
	}

	l.attempted[row.ID] = fingerprint

	if err := l.applyErr[row.ID]; err != nil {
		delete(l.running, row.ID)

		return false, err
	}

	l.running[row.ID] = &runningModule{
		loaded: &pkgplugin.LoadedPlugin{
			Info:    &proto.PluginInfo{Id: pkgplugin.CompactPluginID(row.ID), Name: row.Name, Version: row.Version},
			Enabled: true,
			DBID:    uint64(row.ID),
		},
		fingerprint: fingerprint,
	}

	return true, nil
}

func (l *fakeLoader) UnloadRecord(_ context.Context, dbID domain.Uint64ID, trigger string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.unloads = append(l.unloads, dbID)
	l.triggers = append(l.triggers, trigger)

	_, present := l.running[dbID]
	delete(l.running, dbID)

	return present, nil
}

func (l *fakeLoader) RuntimeState(dbID domain.Uint64ID) internalplugin.RuntimeState {
	l.mu.Lock()
	defer l.mu.Unlock()

	state := internalplugin.RuntimeState{Attempted: l.attempted[dbID]}
	if module, ok := l.running[dbID]; ok {
		state.Present = true
		state.Enabled = module.loaded.IsEnabled()
		state.Fingerprint = module.fingerprint
	}

	return state
}

func (l *fakeLoader) GetDBPluginID(managerID string) (domain.Uint64ID, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for id, module := range l.running {
		if module.loaded.Info.Id == managerID {
			return id, true
		}
	}

	return 0, false
}

// GetPlugins makes the fake loader double as the plugin provider.
func (l *fakeLoader) GetPlugins() []*pkgplugin.LoadedPlugin {
	l.mu.Lock()
	defer l.mu.Unlock()

	plugins := make([]*pkgplugin.LoadedPlugin, 0, len(l.running))
	for _, module := range l.running {
		plugins = append(plugins, module.loaded)
	}

	return plugins
}

func (l *fakeLoader) isRunning(dbID domain.Uint64ID) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	_, ok := l.running[dbID]

	return ok
}

func (l *fakeLoader) applyCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return len(l.applies)
}

type fakeSubs struct {
	mu       sync.Mutex
	refreshs int
	err      error
}

func (f *fakeSubs) RefreshSubscriptions(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshs++

	return f.err
}

func (f *fakeSubs) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.refreshs
}

type fakeArchive struct {
	mu      sync.Mutex
	removed []uint64
}

func (f *fakeArchive) RemovePlugin(pluginID uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, pluginID)
}

type fakeFiles struct {
	mu    sync.Mutex
	files map[string][]byte
}

func newFakeFiles() *fakeFiles { return &fakeFiles{files: make(map[string][]byte)} }

func (f *fakeFiles) Exists(_ context.Context, path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.files[path]

	return ok
}

func (f *fakeFiles) Read(_ context.Context, path string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	data, ok := f.files[path]
	if !ok {
		return nil, errors.New("not found")
	}

	return data, nil
}

func (f *fakeFiles) Write(_ context.Context, path string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[path] = data

	return nil
}

type fakeStore struct {
	mu        sync.Mutex
	data      []byte
	err       error
	downloads []string
}

func (f *fakeStore) DownloadPlugin(_ context.Context, pluginID, version string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.downloads = append(f.downloads, pluginID+"@"+version)

	return f.data, f.err
}

type fakeLock struct{ released bool }

func (l *fakeLock) Release(context.Context) error {
	l.released = true

	return nil
}

func (l *fakeLock) Refresh(context.Context, time.Duration) error { return nil }

type fakeLocker struct {
	mu     sync.Mutex
	locked map[string]bool
	keys   []string
}

func (f *fakeLocker) Acquire(_ context.Context, key string, _ time.Duration) (locker.Lock, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.keys = append(f.keys, key)

	if f.locked[key] {
		return nil, locker.ErrLocked
	}

	return &fakeLock{}, nil
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *fakeClock) After(time.Duration) <-chan time.Time { return make(chan time.Time) }

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type auditCapture struct {
	mu     sync.Mutex
	events []audit.Event
}

func (a *auditCapture) Record(_ context.Context, e audit.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
}

func (a *auditCapture) all() []audit.Event {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]audit.Event(nil), a.events...)
}

type passRecorder struct {
	mu      sync.Mutex
	results []string
}

func (p *passRecorder) SyncPass(result string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.results = append(p.results, result)
}

func newServiceWithoutStore(t *testing.T, e *env) *pluginsync.Service {
	t.Helper()

	return pluginsync.New(pluginsync.Deps{
		Repo: e.repo, Loader: e.loader, Plugins: e.loader, Subs: e.subs, Archive: e.archive,
		Files: e.files, Locks: e.locks, Audit: e.audit, Metrics: e.passes, PluginsDir: "plugins",
	}, pluginsync.Options{Clock: e.clock}, slog.New(slog.DiscardHandler))
}
