package pluginsync

import (
	"context"
	"log/slog"
	"path"
	"strings"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/locker"
	internalplugin "github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/services/pluginstore"
	"github.com/pkg/errors"
)

// uploadedSourcePrefix marks a plugin that arrived through the upload endpoint.
// Only the instance that received the upload has the file.
const uploadedSourcePrefix = "file://"

// ensureFile makes sure the plugin file is present and is the one the row
// describes, fetching it from the store when it is not; repaired reports
// whether a download took place.
//
// The downloaded bytes are checked against the checksum recorded in the
// database rather than against the store's own metadata: the recorded value is
// what this deployment installed and verified, so a store that later serves
// something else cannot slip it past. A row with no checksum is therefore not
// recoverable — writing unverifiable bytes into storage that other instances
// may share would spread the problem instead of containing it.
func (s *Service) ensureFile(ctx context.Context, row *domain.Plugin) (bool, error) {
	pluginPath := path.Join(s.pluginsDir, internalplugin.ResolveFilename(row))

	if s.fileMatches(ctx, row, pluginPath) {
		return false, nil
	}

	storeID, err := storePluginID(row)
	if err != nil {
		return false, err
	}

	if row.Checksum == nil || *row.Checksum == "" {
		return false, ErrChecksumUnknown
	}

	if s.store == nil {
		return false, ErrNoStoreConfigured
	}

	return s.download(ctx, row, storeID, pluginPath)
}

// fileMatches reports whether the file on this instance is the one the row
// describes. A row with no checksum is trusted: it predates the column, and
// there is nothing to compare against.
func (s *Service) fileMatches(ctx context.Context, row *domain.Plugin, pluginPath string) bool {
	if !s.files.Exists(ctx, pluginPath) {
		return false
	}

	if row.Checksum == nil || *row.Checksum == "" {
		return true
	}

	data, err := s.files.Read(ctx, pluginPath)
	if err != nil {
		s.logger.Warn("failed to read plugin file, treating it as missing",
			slog.String("path", pluginPath),
			slog.String("error", err.Error()))

		return false
	}

	if pluginstore.VerifyHash(data, *row.Checksum) {
		return true
	}

	s.logger.Warn("plugin file does not match the recorded checksum, refetching",
		slog.Uint64("plugin_id", uint64(row.ID)),
		slog.String("path", pluginPath))

	return false
}

// storePluginID recovers the store's identifier for a plugin from the source
// URL recorded at install time.
func storePluginID(row *domain.Plugin) (string, error) {
	if row.Source == nil || *row.Source == "" {
		return "", ErrNotStoreSourced
	}

	if strings.HasPrefix(*row.Source, uploadedSourcePrefix) {
		return "", ErrNotStoreSourced
	}

	storeID := path.Base(*row.Source)
	if storeID == "" || storeID == "." || storeID == "/" {
		return "", ErrNotStoreSourced
	}

	return storeID, nil
}

// download fetches the plugin file under a cluster-wide lock so a fleet that
// all notices the same missing file at once does not turn into a stampede
// against the store. On shared storage the winner's write is enough for
// everybody; on local disks each instance still needs its own copy, so the lock
// turns the burst into a queue rather than eliminating the work.
func (s *Service) download(ctx context.Context, row *domain.Plugin, storeID, pluginPath string) (bool, error) {
	lock, err := s.acquireDownloadLock(ctx, storeID, row.Version)
	if err != nil {
		return false, err
	}

	if lock != nil {
		defer func() {
			if releaseErr := lock.Release(context.WithoutCancel(ctx)); releaseErr != nil {
				s.logger.Warn("failed to release plugin download lock",
					slog.String("store_id", storeID),
					slog.String("error", releaseErr.Error()))
			}
		}()

		// The instance that held the lock may have just written the file.
		if s.fileMatches(ctx, row, pluginPath) {
			return true, nil
		}
	}

	downloadCtx, cancel := context.WithTimeout(ctx, s.opts.DownloadTimeout)
	defer cancel()

	data, err := s.store.DownloadPlugin(downloadCtx, storeID, row.Version)
	if err != nil {
		return false, errors.WithMessage(err, "failed to download plugin file")
	}

	if !pluginstore.VerifyHash(data, *row.Checksum) {
		return false, ErrChecksumMismatch
	}

	if err := s.files.Write(ctx, pluginPath, data); err != nil {
		return false, errors.WithMessage(err, "failed to write plugin file")
	}

	s.logger.Info("plugin file restored from the store",
		slog.Uint64("plugin_id", uint64(row.ID)),
		slog.String("plugin_name", row.Name),
		slog.String("action", "download"),
		slog.String("path", pluginPath))

	return true, nil
}

// acquireDownloadLock returns a held lock, or a nil lock when no locker is
// configured. ErrDownloadLocked means a peer is already downloading; the caller
// backs off briefly rather than waiting, because blocking here would hold up
// every other plugin in the pass.
func (s *Service) acquireDownloadLock(ctx context.Context, storeID, version string) (locker.Lock, error) {
	if s.locks == nil {
		return nil, nil //nolint:nilnil // no locker configured: single instance, nothing to coordinate
	}

	key := "pluginsync:download:" + storeID + ":" + version

	lock, err := s.locks.Acquire(ctx, key, s.opts.DownloadLockTTL)
	if err != nil {
		if errors.Is(err, locker.ErrLocked) {
			return nil, ErrDownloadLocked
		}

		return nil, errors.WithMessage(err, "failed to acquire plugin download lock")
	}

	return lock, nil
}
