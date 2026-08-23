package updateconfig

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strconv"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	internalplugin "github.com/gameap/gameap/internal/plugin"
	"github.com/gameap/gameap/internal/plugin/pluginconfig"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/plugininstall"
	"github.com/gameap/gameap/pkg/api"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/configschema"
	"github.com/gameap/gameap/pkg/secret"
	"github.com/pkg/errors"
)

// maxBody bounds the JSON body: a configuration is a handful of scalars,
// each capped by configschema.MaxValueBytes.
const maxBody = 256 << 10

const (
	invalidTitle            = "plugins.config_invalid_title"
	encryptionRequiredTitle = "plugins.config_encryption_required"
	encryptionRequiredText  = "encryption is not configured"
)

var (
	errPluginNotInstalled = errors.New("plugin is not installed")
	errInvalidConfig      = errors.New("plugin configuration is invalid")
	errEncryptionRequired = errors.New("storing secret configuration values requires ENCRYPTION_KEY")
)

type input struct {
	Values map[string]any `json:"values"`
}

// Handler replaces a plugin's configuration and restarts the plugin so
// Initialize sees the new values. Secret-format values are encrypted before
// they reach the database; an omitted secret keeps its stored value and an
// empty string clears it. Key names reach the audit log, values never do.
type Handler struct {
	pluginRepo        repositories.PluginRepository
	loader            PluginLoader
	cipher            *secret.Cipher
	requireEncryption bool
	notifier          plugininstall.SyncNotifier
	responder         base.Responder
	audit             audit.Logger
}

func NewHandler(
	pluginRepo repositories.PluginRepository,
	loader PluginLoader,
	cipher *secret.Cipher,
	requireEncryption bool,
	notifier plugininstall.SyncNotifier,
	responder base.Responder,
	auditLogger audit.Logger,
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	if cipher == nil {
		cipher = secret.Disabled()
	}

	return &Handler{
		pluginRepo:        pluginRepo,
		loader:            loader,
		cipher:            cipher,
		requireEncryption: requireEncryption,
		notifier:          notifier,
		responder:         responder,
		audit:             auditLogger,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := api.NewInputReader(r).ReadString("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to read plugin ID"))

		return
	}

	if id == "" {
		h.responder.WriteError(ctx, rw, api.NewValidationError("plugin ID is required"))

		return
	}

	in, err := decodeInput(rw, r)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	dbID := h.resolveDBID(id)

	record, err := h.findPlugin(ctx, dbID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	next, change, err := h.buildConfig(record, in.Values)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	// Held so the reconciler does not rebuild the module between the save
	// and the reload below.
	release := h.loader.Hold(dbID)
	defer release()

	record.Config = next

	if err := h.pluginRepo.Save(ctx, record); err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to save plugin configuration"))

		return
	}

	resourceID := strconv.FormatUint(uint64(dbID), 10)

	audit.SensitiveOp(ctx, h.audit, audit.EventPluginConfigUpdate, audit.CategoryPluginOp,
		"plugin", resourceID, "update",
		slog.Any("changed_keys", change.changed),
		slog.Any("removed_keys", change.removed),
		slog.Any("secret_keys", change.secrets))

	reloaded, reloadError := h.reload(ctx, record, resourceID)

	plugininstall.Notify(ctx, h.notifier, dbID, plugininstall.ActionConfig)

	h.responder.Write(ctx, rw, newUpdateResponse(record, reloaded, reloadError))
}

// decodeInput reads the body with numbers kept as json.Number so integers
// survive the round trip untouched.
func decodeInput(rw http.ResponseWriter, r *http.Request) (*input, error) {
	dec := json.NewDecoder(http.MaxBytesReader(rw, r.Body, maxBody))
	dec.UseNumber()

	var in input
	if err := dec.Decode(&in); err != nil {
		return nil, api.WrapHTTPError(errors.WithMessage(err, "invalid request body"), http.StatusBadRequest)
	}

	if in.Values == nil {
		in.Values = map[string]any{}
	}

	return &in, nil
}

// configChange names what a save did, for the audit record.
type configChange struct {
	changed []string
	removed []string
	secrets []string
}

// buildConfig validates the request against the row's schema and produces
// the stored form: typed scalars for declared keys, strings for the rest,
// encrypted envelopes for secrets.
func (h *Handler) buildConfig(record *domain.Plugin, values map[string]any) (map[string]any, configChange, error) {
	var change configChange

	schema, err := parseRowSchema(record)
	if err != nil {
		slog.Warn("plugin row carries an invalid config_schema, validating as free-form",
			slog.Uint64("plugin_id", uint64(record.ID)),
			slog.String("error", err.Error()))
	}

	if validation := schema.Validate(values); len(validation) > 0 {
		return nil, change, api.NewFieldsErrorWithTitle(errInvalidConfig.Error(), invalidTitle, validation.Map())
	}

	secrets, plain := splitSecrets(schema, values)

	next, normErrs := schema.Normalize(plain)
	if len(normErrs) > 0 {
		return nil, change, api.NewFieldsErrorWithTitle(errInvalidConfig.Error(), invalidTitle, normErrs.Map())
	}

	if err := h.applySecrets(schema, record, secrets, next, &change); err != nil {
		return nil, change, err
	}

	change.changed, change.removed = diffConfig(record.Config, next)
	sort.Strings(change.secrets)

	return next, change, nil
}

// splitSecrets separates the secret-format keys from the rest of the
// request; only keys present in the request are returned.
func splitSecrets(schema *configschema.Schema, values map[string]any) (map[string]any, map[string]any) {
	secrets := make(map[string]any)
	plain := make(map[string]any, len(values))

	for key, value := range values {
		if property, ok := schema.Property(key); ok && property.Secret {
			secrets[key] = value

			continue
		}

		plain[key] = value
	}

	return secrets, plain
}

// applySecrets resolves every secret property: omitted or null keeps the
// stored envelope, an empty string clears it, a string is sealed.
func (h *Handler) applySecrets(
	schema *configschema.Schema,
	record *domain.Plugin,
	requested map[string]any,
	next map[string]any,
	change *configChange,
) error {
	if schema == nil {
		return nil
	}

	for _, property := range schema.Properties {
		if !property.Secret {
			continue
		}

		value, present := requested[property.Name]
		if !present || value == nil {
			if stored, ok := record.Config[property.Name]; ok {
				if _, isEnvelope := pluginconfig.IsSecretEnvelope(stored); isEnvelope {
					next[property.Name] = stored
				}
			}

			continue
		}

		text, _ := value.(string)
		if text == "" {
			continue
		}

		if h.requireEncryption && !h.cipher.Enabled() {
			return api.NewFieldsErrorWithTitle(errEncryptionRequired.Error(), encryptionRequiredTitle,
				map[string]string{property.Name: encryptionRequiredText})
		}

		envelope, err := pluginconfig.EncryptSecret(h.cipher, uint64(record.ID), property.Name, text)
		if err != nil {
			return errors.WithMessage(err, "failed to encrypt configuration value")
		}

		next[property.Name] = envelope
		change.secrets = append(change.secrets, property.Name)
	}

	return nil
}

func parseRowSchema(record *domain.Plugin) (*configschema.Schema, error) {
	if record.ConfigSchema == nil {
		return nil, nil //nolint:nilnil
	}

	return configschema.Parse(*record.ConfigSchema)
}

// diffConfig names the keys whose stored value changed or appeared, and the
// keys that disappeared. Envelopes compare by ciphertext, so a kept secret
// is not a change and a re-sealed one is.
func diffConfig(previous, next map[string]any) (changed, removed []string) {
	changed, removed = []string{}, []string{}

	for key, value := range next {
		old, existed := previous[key]
		if !existed || !sameStoredValue(old, value) {
			changed = append(changed, key)
		}
	}

	for key := range previous {
		if _, kept := next[key]; !kept {
			removed = append(removed, key)
		}
	}

	sort.Strings(changed)
	sort.Strings(removed)

	return changed, removed
}

func sameStoredValue(a, b any) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}

	right, err := json.Marshal(b)
	if err != nil {
		return false
	}

	return string(left) == string(right)
}

// reload restarts the plugin unless the operator switched it off; a load
// failure is reported in the response, the save itself stands.
func (h *Handler) reload(ctx context.Context, record *domain.Plugin, resourceID string) (bool, string) {
	if record.Status == domain.PluginStatusDisabled || record.Status == domain.PluginStatusUpdating {
		return false, ""
	}

	reloaded, loaded, err := h.loader.Reload(ctx, record.ID)
	if reloaded != nil {
		*record = *reloaded
	}

	if err != nil {
		slog.ErrorContext(ctx, "plugin reload after configuration change failed",
			slog.String("plugin_id", resourceID),
			slog.String("error", err.Error()))

		audit.SensitiveOpFailed(ctx, h.audit, audit.EventPluginReloaded, audit.CategoryPluginOp,
			"plugin", resourceID, "reload", "load_failed", slog.String("trigger", "config"))

		return false, internalplugin.LoadErrorText(err)
	}

	audit.SensitiveOp(ctx, h.audit, audit.EventPluginReloaded, audit.CategoryPluginOp,
		"plugin", resourceID, "reload", slog.String("trigger", "config"))

	return loaded != nil, ""
}

func (h *Handler) findPlugin(ctx context.Context, dbID domain.Uint64ID) (*domain.Plugin, error) {
	plugins, err := h.pluginRepo.Find(ctx, filters.FindPluginByIDs(dbID), nil, &filters.Pagination{Limit: 1})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find plugin")
	}

	if len(plugins) == 0 {
		return nil, api.WrapHTTPError(errPluginNotInstalled, http.StatusNotFound)
	}

	return &plugins[0], nil
}

func (h *Handler) resolveDBID(id string) domain.Uint64ID {
	if resolver, ok := h.loader.(DBIDResolver); ok {
		if dbID, found := resolver.GetDBPluginID(id); found {
			return dbID
		}
	}

	return pkgplugin.ParsePluginID(id)
}
