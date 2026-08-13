package pluginsync

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/gameap/gameap/internal/domain"
	internalplugin "github.com/gameap/gameap/internal/plugin"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/pkg/errors"
)

// ReconcileNow brings this instance's loaded plugins in line with the database
// and returns once it has. Passes never overlap.
func (s *Service) ReconcileNow(ctx context.Context) error {
	s.runMu.Lock()
	defer s.runMu.Unlock()

	rows, err := s.repo.FindAll(ctx, nil, nil)
	if err != nil {
		// Nothing is unloaded on a read failure. An unreachable database must
		// not drain the plugins of every instance that notices.
		return errors.WithMessage(err, "failed to read plugins")
	}

	desired := make(map[domain.Uint64ID]*domain.Plugin, len(rows))
	for i := range rows {
		desired[rows[i].ID] = &rows[i]
	}

	changed := s.reconcileLoaded(ctx, desired)

	if s.reconcileDesired(ctx, desired) {
		changed = true
	}

	s.rememberSeen(desired)

	if changed {
		s.refreshSubscriptions(ctx)
	}

	return nil
}

// reconcileLoaded handles modules that are running: those that must stop, and
// those whose row no longer describes what is loaded.
func (s *Service) reconcileLoaded(ctx context.Context, desired map[domain.Uint64ID]*domain.Plugin) bool {
	changed := false

	for _, loaded := range s.plugins.GetPlugins() {
		managerID := pkgplugin.NormalizePluginID(loaded.Info.Id)

		dbID, known := s.loader.GetDBPluginID(managerID)
		if !known {
			dbID = pkgplugin.ParsePluginID(loaded.Info.Id)
		}

		row, inDatabase := desired[dbID]

		switch {
		case !inDatabase:
			if !s.wasSeen(dbID) {
				// Never observed in the database, so this instance has no
				// grounds to claim it was removed — an autoload or externally
				// registered module would look exactly the same.
				s.logger.Debug("loaded plugin has no database row, leaving it alone",
					slog.String("manager_id", managerID))

				continue
			}

			s.unload(ctx, dbID, managerID, "row deleted")

			changed = true
		case row.Status != domain.PluginStatusActive:
			s.unload(ctx, dbID, managerID, "status "+string(row.Status))

			changed = true
		case s.needsReload(dbID, row, loaded):
			if s.apply(ctx, row, true) {
				changed = true
			}
		}
	}

	return changed
}

// needsReload reports whether a running module no longer matches its row.
//
// With no recorded fingerprint the module is adopted rather than rebuilt: it
// was put there either by the startup load or by an admin handler that already
// applied the current row, and rebuilding it would be downtime for nothing. The
// version is still cross-checked, which closes the narrow window where a peer
// changed the row between that load and this instance's first pass.
func (s *Service) needsReload(dbID domain.Uint64ID, row *domain.Plugin, loaded *pkgplugin.LoadedPlugin) bool {
	fingerprint := Fingerprint(row)

	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.state[dbID]
	if !ok {
		if loaded.Info.Version == row.Version {
			s.state[dbID] = &pluginState{fingerprint: fingerprint, loadedAt: s.clock.Now()}

			return false
		}

		return true
	}

	return st.fingerprint != fingerprint
}

// reconcileDesired loads the active rows that are not running here.
func (s *Service) reconcileDesired(ctx context.Context, desired map[domain.Uint64ID]*domain.Plugin) bool {
	changed := false

	// Sorted so a pass behaves the same way twice and its log reads in a stable
	// order; map iteration would not.
	ids := make([]domain.Uint64ID, 0, len(desired))
	for id := range desired {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	for _, id := range ids {
		row := desired[id]
		if row.Status != domain.PluginStatusActive {
			continue
		}

		if s.isLoaded(row.ID) {
			continue
		}

		if !s.dueForAttempt(row) {
			continue
		}

		if s.apply(ctx, row, false) {
			changed = true
		}
	}

	return changed
}

func (s *Service) isLoaded(dbID domain.Uint64ID) bool {
	managerID, ok := s.loader.GetPluginManagerID(dbID)
	if !ok {
		managerID = pkgplugin.CompactPluginID(dbID)
	}

	_, loaded := s.plugins.GetPlugin(managerID)

	return loaded
}

// dueForAttempt reports whether a failing plugin may be retried yet. A changed
// fingerprint clears the backoff immediately, so re-uploading a fixed plugin
// takes effect on the next pass rather than after the current delay.
func (s *Service) dueForAttempt(row *domain.Plugin) bool {
	fingerprint := Fingerprint(row)

	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.state[row.ID]
	if !ok {
		return true
	}

	if st.fingerprint != fingerprint {
		st.failures = 0
		st.nextAttempt = time.Time{}

		return true
	}

	return !s.clock.Now().Before(st.nextAttempt)
}

// apply brings one plugin up, replacing a running module when reloading.
func (s *Service) apply(ctx context.Context, row *domain.Plugin, reload bool) bool {
	fingerprint := Fingerprint(row)
	filename := internalplugin.ResolveFilename(row)

	if err := s.ensureFile(ctx, row, filename); err != nil {
		s.recordFailure(row, fingerprint, err)

		return false
	}

	loadCtx, cancel := context.WithTimeout(ctx, s.opts.LoadTimeout)
	defer cancel()

	var err error
	if reload {
		_, err = s.loader.Reload(loadCtx, filename, uint64(row.ID))
	} else {
		_, err = s.loader.LoadWithID(loadCtx, filename, uint64(row.ID))
	}

	if err != nil {
		if errors.Is(err, pkgplugin.ErrPluginAlreadyLoaded) {
			// Something registered the module between the check and the load —
			// an admin handler on this instance, most likely. Take it as
			// applied instead of retrying into the same collision.
			s.recordSuccess(row, fingerprint)

			return false
		}

		s.recordFailure(row, fingerprint, err)

		return false
	}

	s.recordSuccess(row, fingerprint)

	s.logger.Info("plugin applied",
		slog.Uint64("plugin_id", uint64(row.ID)),
		slog.String("plugin_name", row.Name),
		slog.String("plugin_version", row.Version),
		slog.String("action", applyAction(reload)))

	return true
}

func applyAction(reload bool) string {
	if reload {
		return "reload"
	}

	return "load"
}

func (s *Service) unload(ctx context.Context, dbID domain.Uint64ID, managerID, reason string) {
	unloadCtx, cancel := context.WithTimeout(ctx, s.opts.UnloadTimeout)
	defer cancel()

	if err := s.loader.Unload(unloadCtx, managerID); err != nil && !errors.Is(err, pkgplugin.ErrPluginNotFound) {
		s.logger.Warn("failed to unload plugin",
			slog.Uint64("plugin_id", uint64(dbID)),
			slog.String("manager_id", managerID),
			slog.String("reason", reason),
			slog.String("error", err.Error()))
	}

	s.loader.UnregisterPluginID(dbID)

	if s.archive != nil {
		s.archive.RemovePlugin(uint64(dbID))
	}

	s.mu.Lock()
	delete(s.state, dbID)
	s.mu.Unlock()

	s.logger.Info("plugin unloaded",
		slog.Uint64("plugin_id", uint64(dbID)),
		slog.String("action", "unload"),
		slog.String("reason", reason))
}

// refreshSubscriptions runs once per pass, and only when something moved: it
// rebuilds the whole subscription map by calling every enabled guest while
// holding the dispatcher's write lock, so doing it per plugin would multiply
// both the guest calls and the time event delivery is blocked.
func (s *Service) refreshSubscriptions(ctx context.Context) {
	if s.subs == nil {
		return
	}

	refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.opts.RefreshSubsTimeout)
	defer cancel()

	if err := s.subs.RefreshSubscriptions(refreshCtx); err != nil {
		// Never fails the pass: the modules are in place, only event delivery
		// lags until the next refresh.
		s.logger.Warn("failed to refresh plugin subscriptions after reconcile",
			slog.String("error", err.Error()))
	}
}

func (s *Service) recordSuccess(row *domain.Plugin, fingerprint string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state[row.ID] = &pluginState{
		fingerprint: fingerprint,
		loadedAt:    s.clock.Now(),
	}
}

func (s *Service) recordFailure(row *domain.Plugin, fingerprint string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.state[row.ID]
	if !ok || st.fingerprint != fingerprint {
		st = &pluginState{fingerprint: fingerprint}
		s.state[row.ID] = st
	}

	// Contention is somebody else making progress, so it neither counts as a
	// failure nor stretches the delay.
	if errors.Is(err, ErrDownloadLocked) {
		st.lastErr = err.Error()
		st.nextAttempt = s.clock.Now().Add(s.opts.ContentionBackoff)

		return
	}

	st.failures++
	st.lastErr = err.Error()
	st.nextAttempt = s.clock.Now().Add(s.backoffFor(st.failures))

	s.logger.Error("failed to apply plugin",
		slog.Uint64("plugin_id", uint64(row.ID)),
		slog.String("plugin_name", row.Name),
		slog.Int("attempt", st.failures),
		slog.Duration("next_attempt_in", st.nextAttempt.Sub(s.clock.Now())),
		slog.String("error", err.Error()))
}

func (s *Service) backoffFor(failures int) time.Duration {
	delay := s.opts.MinBackoff
	for range failures - 1 {
		delay *= 2
		if delay >= s.opts.MaxBackoff {
			return s.opts.MaxBackoff
		}
	}

	return delay
}

func (s *Service) rememberSeen(desired map[domain.Uint64ID]*domain.Plugin) {
	seen := make(map[domain.Uint64ID]struct{}, len(desired))
	for id := range desired {
		seen[id] = struct{}{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.seen = seen

	for id := range s.state {
		if _, ok := seen[id]; !ok {
			delete(s.state, id)
		}
	}
}

func (s *Service) wasSeen(dbID domain.Uint64ID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.seen[dbID]

	return ok
}
