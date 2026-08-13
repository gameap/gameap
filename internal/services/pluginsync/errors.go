package pluginsync

import "github.com/pkg/errors"

var (
	// ErrNotStoreSourced means the plugin file is missing and cannot be
	// re-fetched: it was uploaded by hand, so no instance but the one that
	// received the upload has a copy. Shared plugin storage is the fix.
	ErrNotStoreSourced = errors.New("plugin file is missing and the plugin was not installed from the store")

	// ErrChecksumUnknown means the row predates the checksum column. Downloading
	// would mean writing bytes this instance cannot verify into storage other
	// instances may share, so it refuses.
	ErrChecksumUnknown = errors.New("plugin file is missing and the row has no recorded checksum")

	// ErrChecksumMismatch means the store served bytes that do not match the
	// checksum recorded at install time.
	ErrChecksumMismatch = errors.New("downloaded plugin file does not match the recorded checksum")

	// ErrDownloadLocked means another instance is already fetching the same
	// plugin file. It is contention, not failure, and does not count against
	// the retry budget.
	ErrDownloadLocked = errors.New("plugin file is being downloaded by another instance")

	// ErrNoStoreConfigured means the panel has no plugin store service wired,
	// so a missing file cannot be recovered automatically.
	ErrNoStoreConfigured = errors.New("plugin store is not configured")
)
