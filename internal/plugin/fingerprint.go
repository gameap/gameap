package plugin

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"strconv"

	"github.com/gameap/gameap/internal/domain"
)

// Fingerprint hashes the fields that require tearing a plugin's module down
// and building it again. Two rows with the same fingerprint describe the same
// running module, so a reconcile pass that sees no change does nothing.
//
// Deliberately excluded:
//
//   - allowed_permissions and required_permissions: grants are re-read from the
//     database on every host library call, so a permission change already
//     takes effect without touching the runtime;
//   - priority: nothing in the runtime reads it;
//   - status: a load or unload decision, handled separately;
//   - last_loaded_at, updated_at, created_at: bookkeeping.
//
// config is included because a module can only pick a changed configuration
// up by being rebuilt; generation is included so an operator reload on one
// instance restarts the module everywhere.
func Fingerprint(plugin *domain.Plugin) string {
	h := sha256.New()

	writeField(h, plugin.Version)
	writeField(h, derefOrEmpty(plugin.Filename))
	writeField(h, derefOrEmpty(plugin.Checksum))
	writeField(h, canonicalConfig(plugin.Config))
	writeField(h, strconv.Itoa(plugin.Generation))

	return hex.EncodeToString(h.Sum(nil))
}

// FileChecksum is the sha256 of a wasm module in the form the plugin store
// publishes and plugins.checksum records.
func FileChecksum(wasm []byte) string {
	sum := sha256.Sum256(wasm)

	return hex.EncodeToString(sum[:])
}

// ResolveFilename returns the plugin file name recorded on the row, falling
// back to the decimal ID for rows installed before the column existed.
func ResolveFilename(plugin *domain.Plugin) string {
	if plugin.Filename != nil && *plugin.Filename != "" {
		return *plugin.Filename
	}

	return strconv.FormatUint(uint64(plugin.ID), 10) + ".wasm"
}

// canonicalConfig renders the stored configuration deterministically
// (encoding/json sorts map keys); nil and empty both become "".
func canonicalConfig(config map[string]any) string {
	if len(config) == 0 {
		return ""
	}

	encoded, err := json.Marshal(config)
	if err != nil {
		return "unmarshalable:" + err.Error()
	}

	return string(encoded)
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
