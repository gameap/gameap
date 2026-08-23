// Package pluginconfig turns the configuration stored on a plugin row into
// what the guest receives and back: schema defaults overlaid by the
// operator's values, secret values kept as encrypted envelopes, and the
// masked view the admin API answers with.
package pluginconfig

import (
	"sort"
	"strconv"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/plugin/configschema"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/gameap/gameap/pkg/secret"
	"github.com/pkg/errors"
)

// SecretEnvelopeKey is the single key of the JSON object a secret value is
// stored as: {"$secret": "<ciphertext>"}. An operator value is never an
// object, so the two cannot be confused.
const SecretEnvelopeKey = "$secret"

// IsSecretEnvelope reports whether a stored value is a secret envelope and
// returns the ciphertext it holds.
func IsSecretEnvelope(value any) (string, bool) {
	envelope, ok := value.(map[string]any)
	if !ok || len(envelope) != 1 {
		return "", false
	}

	ciphertext, ok := envelope[SecretEnvelopeKey].(string)

	return ciphertext, ok
}

// SecretAAD binds a ciphertext to the row and key that hold it, so a value
// copied into another plugin's configuration, or under another key, no
// longer decrypts.
func SecretAAD(pluginID uint64, key string) string {
	return "plugin-config:" + strconv.FormatUint(pluginID, 10) + ":" + key
}

// EncryptSecret seals a secret value into its stored envelope. With a
// disabled cipher the envelope holds the plaintext; callers decide whether
// that is acceptable (PLUGIN_SECRETS_REQUIRE_ENCRYPTION).
func EncryptSecret(cipher *secret.Cipher, pluginID uint64, key, plain string) (map[string]any, error) {
	sealed, err := cipherOrDisabled(cipher).EncryptWithAAD(plain, SecretAAD(pluginID, key))
	if err != nil {
		return nil, errors.WithMessage(err, "failed to encrypt configuration value")
	}

	return map[string]any{SecretEnvelopeKey: sealed}, nil
}

// DecryptSecret opens a stored envelope.
func DecryptSecret(cipher *secret.Cipher, pluginID uint64, key, ciphertext string) (string, error) {
	return cipherOrDisabled(cipher).DecryptWithAAD(ciphertext, SecretAAD(pluginID, key))
}

// Resolve converts the stored configuration into the string map the guest
// receives: secrets decrypted, scalars rendered, nil values dropped. A
// decryption failure names the key and never the value.
func Resolve(cipher *secret.Cipher, pluginID uint64, stored map[string]any) (map[string]string, error) {
	if len(stored) == 0 {
		return nil, nil
	}

	resolved := make(map[string]string, len(stored))

	for key, value := range stored {
		if ciphertext, isSecret := IsSecretEnvelope(value); isSecret {
			plain, err := DecryptSecret(cipher, pluginID, key, ciphertext)
			if err != nil {
				return nil, errors.Errorf("failed to decrypt configuration key %q: %v", key, err)
			}

			resolved[key] = plain

			continue
		}

		if text, ok := configschema.ValueToString(value); ok {
			resolved[key] = text
		}
	}

	return resolved, nil
}

// Effective is the map Initialize receives: the schema defaults overlaid by
// the operator's stored values.
func Effective(cipher *secret.Cipher, plugin *domain.Plugin) (map[string]string, error) {
	resolved, err := Resolve(cipher, uint64(plugin.ID), plugin.Config)
	if err != nil {
		return nil, err
	}

	var schemaText string
	if plugin.ConfigSchema != nil {
		schemaText = *plugin.ConfigSchema
	}

	return configschema.ApplyDefaults(schemaText, resolved), nil
}

// SchemaFromManifest copies the manifest's config_schema onto the row so
// the admin UI can render the form while the plugin is not loaded here.
// Reports whether the row changed.
func SchemaFromManifest(plugin *domain.Plugin, info *proto.PluginInfo) bool {
	var declared string
	if info != nil {
		declared = info.ConfigSchema
	}

	switch {
	case declared == "" && plugin.ConfigSchema == nil:
		return false
	case declared == "":
		plugin.ConfigSchema = nil

		return true
	case plugin.ConfigSchema != nil && *plugin.ConfigSchema == declared:
		return false
	default:
		plugin.ConfigSchema = new(declared)

		return true
	}
}

// SecretKeys lists the keys holding a secret envelope, sorted.
func SecretKeys(stored map[string]any) []string {
	keys := make([]string, 0)

	for key, value := range stored {
		if _, isSecret := IsSecretEnvelope(value); isSecret {
			keys = append(keys, key)
		}
	}

	sort.Strings(keys)

	return keys
}

// Keys lists every stored key, sorted.
func Keys(stored map[string]any) []string {
	keys := make([]string, 0, len(stored))
	for key := range stored {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

// MaskedValues copies the stored configuration without the secret
// envelopes; what remains is safe to show an operator.
func MaskedValues(stored map[string]any) map[string]any {
	masked := make(map[string]any, len(stored))

	for key, value := range stored {
		if _, isSecret := IsSecretEnvelope(value); isSecret {
			continue
		}

		masked[key] = value
	}

	return masked
}

// View is what the admin API answers for a plugin's configuration: the
// parsed schema (or why it could not be parsed), the non-secret values and
// the names of the secrets that hold a value.
type View struct {
	Schema      *configschema.SchemaJSON `json:"schema"`
	SchemaError string                   `json:"schema_error,omitempty"`
	Values      map[string]any           `json:"values"`
	SecretsSet  []string                 `json:"secrets_set"`
}

// NewView builds the view of a plugin row.
func NewView(plugin *domain.Plugin) View {
	view := View{
		Values:     MaskedValues(plugin.Config),
		SecretsSet: SecretKeys(plugin.Config),
	}

	if plugin.ConfigSchema != nil {
		schema, err := configschema.Parse(*plugin.ConfigSchema)
		if err != nil {
			view.SchemaError = err.Error()
		} else {
			view.Schema = schema.JSON()
		}
	}

	return view
}

func cipherOrDisabled(cipher *secret.Cipher) *secret.Cipher {
	if cipher == nil {
		return secret.Disabled()
	}

	return cipher
}
