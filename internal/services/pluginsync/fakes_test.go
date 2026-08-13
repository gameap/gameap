package pluginsync_test

import (
	"context"
	"sync"
	"time"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/locker"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
)

// fakeClock mirrors the one in pluginscheduler: After hands back a closed
// channel so the loop never actually sleeps in a test.
type fakeClock struct {
	mu             sync.Mutex
	now            time.Time
	afterDurations []time.Duration
	afterCh        chan time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{
		now:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		afterCh: make(chan time.Time),
	}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.afterDurations = append(c.afterDurations, d)

	return c.afterCh
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)
}

// fakeRepo serves plugin rows and can be told to fail.
type fakeRepo struct {
	mu   sync.Mutex
	rows []domain.Plugin
	err  error
}

func (r *fakeRepo) FindAll(
	_ context.Context,
	_ []filters.Sorting,
	_ *filters.Pagination,
) ([]domain.Plugin, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.err != nil {
		return nil, r.err
	}

	out := make([]domain.Plugin, len(r.rows))
	copy(out, r.rows)

	return out, nil
}

func (r *fakeRepo) set(rows ...domain.Plugin) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.rows = rows
}

func (r *fakeRepo) setErr(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.err = err
}

type loadCall struct {
	filename string
	pluginID uint64
	reload   bool
}

// fakeLoader stands in for internalplugin.Loader and keeps the registry the
// provider reads, so load and unload are visible to both.
type fakeLoader struct {
	mu        sync.Mutex
	provider  *fakeProvider
	calls     []loadCall
	unloaded  []string
	unmapped  []domain.Uint64ID
	loadErr   error
	reloadErr error
	unloadErr error
	// managerIDFor overrides the manager ID a load registers, modelling a wasm
	// whose own ID differs from the store ID.
	managerIDFor map[uint64]string
	ids          map[domain.Uint64ID]string
	// entering is signalled on every load; used by the overlap test.
	entering chan struct{}
	// release blocks a load until closed; used by the overlap test.
	release chan struct{}
}

func newFakeLoader(provider *fakeProvider) *fakeLoader {
	return &fakeLoader{
		provider:     provider,
		managerIDFor: make(map[uint64]string),
		ids:          make(map[domain.Uint64ID]string),
	}
}

func (l *fakeLoader) managerID(pluginID uint64) string {
	if id, ok := l.managerIDFor[pluginID]; ok {
		return id
	}

	return pkgplugin.CompactPluginID(domain.Uint64ID(pluginID))
}

func (l *fakeLoader) load(filename string, pluginID uint64, reload bool, err error) (*pkgplugin.LoadedPlugin, error) {
	l.mu.Lock()
	l.calls = append(l.calls, loadCall{filename: filename, pluginID: pluginID, reload: reload})
	entering, release := l.entering, l.release
	l.mu.Unlock()

	if entering != nil {
		entering <- struct{}{}
		<-release
	}

	if err != nil {
		return nil, err
	}

	managerID := l.managerID(pluginID)

	loaded := &pkgplugin.LoadedPlugin{
		Info:    &proto.PluginInfo{Id: managerID, Name: "plugin", Version: l.provider.versionFor(pluginID)},
		Enabled: true,
	}

	l.provider.add(managerID, loaded)

	l.mu.Lock()
	l.ids[domain.Uint64ID(pluginID)] = pkgplugin.NormalizePluginID(managerID)
	l.mu.Unlock()

	return loaded, nil
}

func (l *fakeLoader) LoadWithID(
	_ context.Context,
	filename string,
	pluginID uint64,
) (*pkgplugin.LoadedPlugin, error) {
	return l.load(filename, pluginID, false, l.loadErr)
}

func (l *fakeLoader) Reload(_ context.Context, filename string, pluginID uint64) (*pkgplugin.LoadedPlugin, error) {
	return l.load(filename, pluginID, true, l.reloadErr)
}

func (l *fakeLoader) Unload(_ context.Context, managerID string) error {
	l.mu.Lock()
	l.unloaded = append(l.unloaded, managerID)
	l.mu.Unlock()

	if l.unloadErr != nil {
		return l.unloadErr
	}

	l.provider.remove(managerID)

	return nil
}

func (l *fakeLoader) GetPluginManagerID(dbID domain.Uint64ID) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	id, ok := l.ids[dbID]

	return id, ok
}

func (l *fakeLoader) GetDBPluginID(managerID string) (domain.Uint64ID, bool) {
	normalized := pkgplugin.NormalizePluginID(managerID)

	l.mu.Lock()
	defer l.mu.Unlock()

	for dbID, mgrID := range l.ids {
		if mgrID == normalized {
			return dbID, true
		}
	}

	return 0, false
}

func (l *fakeLoader) UnregisterPluginID(dbID domain.Uint64ID) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.unmapped = append(l.unmapped, dbID)
	delete(l.ids, dbID)
}

func (l *fakeLoader) loadCalls() []loadCall {
	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]loadCall, len(l.calls))
	copy(out, l.calls)

	return out
}

func (l *fakeLoader) unloadedIDs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]string, len(l.unloaded))
	copy(out, l.unloaded)

	return out
}

// fakeProvider is the manager's registry.
type fakeProvider struct {
	mu       sync.Mutex
	plugins  map[string]*pkgplugin.LoadedPlugin
	versions map[uint64]string
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{
		plugins:  make(map[string]*pkgplugin.LoadedPlugin),
		versions: make(map[uint64]string),
	}
}

func (p *fakeProvider) versionFor(pluginID uint64) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	if v, ok := p.versions[pluginID]; ok {
		return v
	}

	return "1.0.0"
}

func (p *fakeProvider) setVersion(pluginID uint64, version string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.versions[pluginID] = version
}

func (p *fakeProvider) add(managerID string, loaded *pkgplugin.LoadedPlugin) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.plugins[pkgplugin.NormalizePluginID(managerID)] = loaded
}

func (p *fakeProvider) remove(managerID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.plugins, pkgplugin.NormalizePluginID(managerID))
}

func (p *fakeProvider) GetPlugin(managerID string) (*pkgplugin.LoadedPlugin, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	loaded, ok := p.plugins[pkgplugin.NormalizePluginID(managerID)]

	return loaded, ok
}

func (p *fakeProvider) GetPlugins() []*pkgplugin.LoadedPlugin {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([]*pkgplugin.LoadedPlugin, 0, len(p.plugins))
	for _, loaded := range p.plugins {
		out = append(out, loaded)
	}

	return out
}

type fakeSubs struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeSubs) RefreshSubscriptions(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++

	return f.err
}

func (f *fakeSubs) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.calls
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

func (f *fakeArchive) removedIDs() []uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]uint64, len(f.removed))
	copy(out, f.removed)

	return out
}

type downloadCall struct {
	pluginID string
	version  string
}

type fakeDownloader struct {
	mu    sync.Mutex
	calls []downloadCall
	data  []byte
	err   error
}

func (f *fakeDownloader) DownloadPlugin(_ context.Context, pluginID, version string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, downloadCall{pluginID: pluginID, version: version})

	if f.err != nil {
		return nil, f.err
	}

	return f.data, nil
}

func (f *fakeDownloader) downloadCalls() []downloadCall {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]downloadCall, len(f.calls))
	copy(out, f.calls)

	return out
}

type fakeLock struct {
	released bool
}

func (l *fakeLock) Release(_ context.Context) error {
	l.released = true

	return nil
}

func (l *fakeLock) Refresh(_ context.Context, _ time.Duration) error { return nil }

type fakeLocker struct {
	mu       sync.Mutex
	acquired []string
	// deny returns a non-nil error for keys that must not be granted.
	deny func(key string) error
	// onAcquire runs while the lock is held, letting a test simulate a peer
	// writing the file first.
	onAcquire func()
}

func (f *fakeLocker) Acquire(_ context.Context, key string, _ time.Duration) (locker.Lock, error) {
	f.mu.Lock()
	f.acquired = append(f.acquired, key)
	deny, onAcquire := f.deny, f.onAcquire
	f.mu.Unlock()

	if deny != nil {
		if err := deny(key); err != nil {
			return nil, err
		}
	}

	if onAcquire != nil {
		onAcquire()
	}

	return &fakeLock{}, nil
}

func (f *fakeLocker) acquiredKeys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]string, len(f.acquired))
	copy(out, f.acquired)

	return out
}

var errRepoUnavailable = errors.New("database unavailable")
