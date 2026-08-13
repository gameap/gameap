package pluginsync

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"

	"github.com/gameap/gameap/internal/domain"
)

// Fingerprint hashes the fields that require tearing a plugin's module down and
// building it again. Two rows with the same fingerprint describe the same
// running module, so a reconcile pass that sees no change does nothing.
//
// Deliberately excluded:
//
//   - allowed_permissions and required_permissions. Grants are re-read from the
//     database on every host library call (see
//     internal/plugin/hostlibrary/pluginpermissions.go), so a permission change
//     already takes effect without touching the runtime. Reloading on it would
//     be downtime that buys nothing.
//   - priority. Nothing in the runtime reads it; it only orders repository
//     queries. Including it would make reordering the admin list restart every
//     plugin on every instance.
//   - status. That is a load or unload decision, handled separately, not a
//     reason to rebuild a module in place.
//   - last_loaded_at, updated_at, created_at. Bookkeeping, written on paths that
//     have nothing to do with what the module contains.
//
// config is absent because it never reaches the guest: the loader passes a nil
// config to the manager. When that is wired up, config belongs here, because
// the guest's Initialize is its only delivery point.
func Fingerprint(plugin *domain.Plugin) string {
	h := sha256.New()

	writeField(h, plugin.Version)
	writeField(h, derefOrEmpty(plugin.Filename))
	writeField(h, derefOrEmpty(plugin.Checksum))

	return hex.EncodeToString(h.Sum(nil))
}

// writeField length-prefixes every value so no combination of field contents
// can produce the same digest as a different combination.
func writeField(h interface{ Write([]byte) (int, error) }, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))

	_, _ = h.Write(length[:])
	_, _ = h.Write([]byte(value))
}

func derefOrEmpty(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}
