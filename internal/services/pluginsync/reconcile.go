package pluginsync

import (
	"context"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	internalplugin "github.com/gameap/gameap/internal/plugin"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/pkg/errors"
)

// ReconcileNow brings this instance's loaded plugins in line with the database
// and returns once it has. Passes never overlap.
//
// The decisions, by row status and what runs here:
//
//   - a row that is gone or "disabled" while a module runs: unload;
//   - an "active" row: load when absent (with backoff after failures), replace
//     when the running module was built from a different row or is disabled
//     at runtime, leave alone otherwise;
//   - an "error" row: the last recorded attempt failed somewhere. A module
//     running here stays (a peer's failure is not this instance's); an absent
//     one is attempted again only when the row changed or the file was just
//     repaired — timed retries of runtime disables belong to the recovery
//     supervisor, which honours PLUGIN_RECOVERY_*;
//   - "updating" rows are skipped: an operator's update is in flight.
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

	rowsChanged, grantsChanged := s.reconcileRows(ctx, desired)
	changed = changed || rowsChanged

	s.rememberSeen(desired)

	if changed || grantsChanged {
		s.refreshSubscriptions(ctx)
	}

	return nil
}

// reconcileLoaded stops the modules the database no longer wants: rows that
// disappeared after this instance saw them, and rows switched off by the
// operator.
func (s *Service) reconcileLoaded(ctx context.Context, desired map[domain.Uint64ID]*domain.Plugin) bool {
	changed := false

	for _, loaded := range s.plugins.GetPlugins() {
		dbID := s.loadedDBID(loaded)
		if dbID == 0 {
			continue
		}

		row, inDatabase := desired[dbID]

		switch {
		case !inDatabase:
			if !s.wasSeen(dbID) {
				// Never observed in the database, so this instance has no
				// grounds to claim it was removed.
				s.logger.Debug("loaded plugin has no database row, leaving it alone",
					slog.Uint64("plugin_id", uint64(dbID)))

				continue
			}

			s.unload(ctx, dbID, "row deleted")

			changed = true
		case row.Status == domain.PluginStatusDisabled:
			s.unload(ctx, dbID, "status disabled")

			changed = true
		}
	}

	return changed
}

// loadedDBID resolves the database id of a running module: the id it was
// loaded for, falling back to the loader's mapping for modules loaded
// before that field existed.
func (s *Service) loadedDBID(loaded *pkgplugin.LoadedPlugin) domain.Uint64ID {
	if loaded.DBID != 0 {
		return domain.Uint64ID(loaded.DBID)
	}

	if loaded.Info == nil {
		return 0
	}

	dbID, known := s.loader.GetDBPluginID(loaded.Info.Id)
	if !known {
		return 0
	}

	return dbID
}

// reconcileRows walks the rows in a stable order and reports whether modules
// moved and whether a present plugin's listen_events grant changed (which
// needs a subscription rebuild without a reload).
func (s *Service) reconcileRows(ctx context.Context, desired map[domain.Uint64ID]*domain.Plugin) (bool, bool) {
	ids := make([]domain.Uint64ID, 0, len(desired))
	for id := range desired {
		ids = append(ids, id)
	}

	slices.Sort(ids)

	changed, grantsChanged := false, false

	for _, id := range ids {
		row := desired[id]
		state := s.loader.RuntimeState(id)

		var moved bool

		switch row.Status {
		case domain.PluginStatusActive:
			moved = s.reconcileActive(ctx, row, state)
		case domain.PluginStatusError:
			moved = s.reconcileError(ctx, row, state)
		case domain.PluginStatusDisabled, domain.PluginStatusUpdating:
			continue
		}

		changed = changed || moved

		if !moved && state.Present && s.trackListenEvents(row) {
			grantsChanged = true
		}
	}

	return changed, grantsChanged
}

// reconcileActive makes an active row run here.
func (s *Service) reconcileActive(ctx context.Context, row *domain.Plugin, state internalplugin.RuntimeState) bool {
	fingerprint := internalplugin.Fingerprint(row)

	if state.Present && state.Enabled && state.Fingerprint == fingerprint {
		s.recordInSync(row, fingerprint)

		return false
	}

	if !state.Present && !s.dueForAttempt(row, fingerprint) {
		return false
	}

	return s.apply(ctx, row, state, true)
}

// reconcileError handles a row whose last recorded load failed somewhere.
func (s *Service) reconcileError(ctx context.Context, row *domain.Plugin, state internalplugin.RuntimeState) bool {
	fingerprint := internalplugin.Fingerprint(row)

	if state.Present {
		if state.Fingerprint == fingerprint {
			s.recordInSync(row, fingerprint)

			return false
		}

		return s.apply(ctx, row, state, false)
	}

	repaired, err := s.ensureFile(ctx, row)
	if err != nil {
		s.recordFailure(row, fingerprint, err, false)

		return false
	}

	if !repaired && state.Attempted == fingerprint {
		return false
	}

	return s.apply(ctx, row, state, false)
}

// dueForAttempt reports whether a failing active row may be retried yet. A
// changed fingerprint clears the backoff immediately, so re-uploading a fixed
// plugin takes effect on the next pass rather than after the current delay.
func (s *Service) dueForAttempt(row *domain.Plugin, fingerprint string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.state[row.ID]
	if !ok {
		return true
	}

	if st.fingerprint != fingerprint {
		st.failures = 0
		st.nextAttempt = time.Time{}
		st.scheduled = false

		return true
	}

	return !st.scheduled || !s.clock.Now().Before(st.nextAttempt)
}

// apply makes the file present and asks the loader to build the module for
// the row. schedule says whether a failure is retried on a timer (active rows)
// or waits for the row to change (error rows).
func (s *Service) apply(
	ctx context.Context,
	row *domain.Plugin,
	state internalplugin.RuntimeState,
	schedule bool,
) bool {
	fingerprint := internalplugin.Fingerprint(row)

	if _, err := s.ensureFile(ctx, row); err != nil {
		s.recordFailure(row, fingerprint, err, schedule)

		return false
	}

	changed, err := s.loader.ApplyRecord(ctx, row)
	if err != nil {
		if errors.Is(err, internalplugin.ErrPluginHeld) {
			s.recordContention(row, fingerprint, err)

			return false
		}

		s.recordFailure(row, fingerprint, err, schedule)

		return false
	}

	s.recordSuccess(row, fingerprint)

	if !changed {
		return false
	}

	action := "load"
	if state.Present {
		action = "reload"

		audit.SystemOp(ctx, s.audit, audit.EventPluginReloaded, audit.CategoryPluginOp, audit.OutcomeSuccess,
			"plugin", strconv.FormatUint(uint64(row.ID), 10), "reload", "", slog.String("trigger", "sync"))
	}

	s.logger.Info("plugin applied",
		slog.Uint64("plugin_id", uint64(row.ID)),
		slog.String("plugin_name", row.Name),
		slog.String("plugin_version", row.Version),
		slog.String("action", action))

	return true
}

func (s *Service) unload(ctx context.Context, dbID domain.Uint64ID, reason string) {
	unloadCtx, cancel := context.WithTimeout(ctx, s.opts.UnloadTimeout)
	defer cancel()

	if _, err := s.loader.UnloadRecord(unloadCtx, dbID, internalplugin.TriggerSync); err != nil {
		s.logger.Warn("failed to unload plugin",
			slog.Uint64("plugin_id", uint64(dbID)),
			slog.String("reason", reason),
			slog.String("error", err.Error()))
	}

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

// trackListenEvents records the listen_events grant of a present plugin and
// reports whether it differs from the last pass.
func (s *Service) trackListenEvents(row *domain.Plugin) bool {
	granted := row.HasPermission(domain.PluginPermissionListenEvents)

	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.state[row.ID]
	if !ok {
		return false
	}

	changed := st.listenEvents != granted
	st.listenEvents = granted

	return changed
}

func (s *Service) recordInSync(row *domain.Plugin, fingerprint string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.state[row.ID]
	if !ok || st.fingerprint != fingerprint || st.lastErr != "" {
		s.state[row.ID] = &pluginState{
			fingerprint:  fingerprint,
			listenEvents: row.HasPermission(domain.PluginPermissionListenEvents),
		}
	}
}

func (s *Service) recordSuccess(row *domain.Plugin, fingerprint string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state[row.ID] = &pluginState{
		fingerprint:  fingerprint,
		listenEvents: row.HasPermission(domain.PluginPermissionListenEvents),
		lastAttempt:  s.clock.Now(),
	}
}

func (s *Service) recordContention(row *domain.Plugin, fingerprint string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.stateFor(row, fingerprint)
	st.lastErr = err.Error()
	st.lastAttempt = s.clock.Now()
	st.nextAttempt = s.clock.Now().Add(s.opts.ContentionBackoff)
	st.scheduled = true
}

func (s *Service) recordFailure(row *domain.Plugin, fingerprint string, err error, schedule bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.stateFor(row, fingerprint)

	// Contention is somebody else making progress, so it neither counts as a
	// failure nor stretches the delay.
	if errors.Is(err, ErrDownloadLocked) {
		st.lastErr = err.Error()
		st.lastAttempt = s.clock.Now()
		st.nextAttempt = s.clock.Now().Add(s.opts.ContentionBackoff)
		st.scheduled = true

		return
	}

	st.failures++
	st.lastErr = err.Error()
	st.lastAttempt = s.clock.Now()
	st.scheduled = schedule
	st.nextAttempt = time.Time{}

	if schedule {
		st.nextAttempt = s.clock.Now().Add(s.backoffFor(st.failures))
	}

	s.logger.Error("failed to apply plugin",
		slog.Uint64("plugin_id", uint64(row.ID)),
		slog.String("plugin_name", row.Name),
		slog.Int("attempt", st.failures),
		slog.Bool("retry_scheduled", schedule),
		slog.String("error", err.Error()))
}

// stateFor answers the state entry for the row, starting a fresh one when
// the row changed since the last attempt. The caller holds s.mu.
func (s *Service) stateFor(row *domain.Plugin, fingerprint string) *pluginState {
	st, ok := s.state[row.ID]
	if !ok || st.fingerprint != fingerprint {
		st = &pluginState{
			fingerprint:  fingerprint,
			listenEvents: row.HasPermission(domain.PluginPermissionListenEvents),
		}
		s.state[row.ID] = st
	}

	return st
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
