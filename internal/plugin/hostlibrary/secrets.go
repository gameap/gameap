package hostlibrary

import (
	"context"
	"log/slog"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/sdk/secrets"
	"github.com/gameap/gameap/pkg/secret"
	"github.com/tetratelabs/wazero"
)

// secretsPermissionDeniedMessage is what a plugin without the grant sees. It
// names the missing permission so plugin authors know what to declare.
const secretsPermissionDeniedMessage = "plugin permission " + string(domain.PluginPermissionSecrets) + " required"

// encryptionDisabledMessage is returned instead of writing a credential in
// plaintext: a secrets store that silently degrades to plaintext is worse than
// one that refuses to accept the value.
const encryptionDisabledMessage = "gameap-secrets requires ENCRYPTION_KEY to be configured"

// Storage failures are reported to the guest as fixed messages: the plugin can
// act on none of them, and a raw driver error would hand it details about the
// panel's database. The cause is logged instead.
const (
	secretReadFailureMessage   = "failed to read secret"
	secretWriteFailureMessage  = "failed to store secret"
	secretDeleteFailureMessage = "failed to delete secret"
	secretListFailureMessage   = "failed to list secrets"
)

var secretKeyRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)

const (
	defaultMaxSecretsPerPlugin = 64
	defaultMaxSecretValueBytes = 8192
)

// SecretsConfig caps what one plugin may keep in the store. Non-positive
// values fall back to the defaults above so a partially filled config cannot
// silently remove a quota.
type SecretsConfig struct {
	MaxKeysPerPlugin  int
	MaxValueBytes     int
	RequireEncryption bool
}

type SecretsServiceImpl struct {
	pluginID uint64
	repo     repositories.PluginSecretRepository
	cipher   *secret.Cipher
	checker  PluginPermissionChecker
	cfg      SecretsConfig
}

func NewSecretsService(
	pluginID uint64,
	repo repositories.PluginSecretRepository,
	cipher *secret.Cipher,
	checker PluginPermissionChecker,
	cfg SecretsConfig,
) *SecretsServiceImpl {
	if cfg.MaxKeysPerPlugin <= 0 {
		cfg.MaxKeysPerPlugin = defaultMaxSecretsPerPlugin
	}

	if cfg.MaxValueBytes <= 0 {
		cfg.MaxValueBytes = defaultMaxSecretValueBytes
	}

	return &SecretsServiceImpl{
		pluginID: pluginID,
		repo:     repo,
		cipher:   cipher,
		checker:  checker,
		cfg:      cfg,
	}
}

// authorize gates every method of this module on the "secrets" grant. Plugin
// ID 0 (transient dry-run loads) is never granted anything.
func (s *SecretsServiceImpl) authorize(ctx context.Context) (bool, string) {
	allowed, err := s.checker.Has(ctx, s.pluginID, domain.PluginPermissionSecrets)
	if err != nil {
		slog.ErrorContext(ctx, "failed to check plugin secrets permission",
			slog.Uint64("plugin_id", s.pluginID),
			slog.String("error", err.Error()))

		return false, "failed to check plugin permission: " + err.Error()
	}

	if !allowed {
		slog.WarnContext(ctx, "plugin denied access to the secrets host library",
			slog.Uint64("plugin_id", s.pluginID),
			slog.String("permission", string(domain.PluginPermissionSecrets)))

		return false, secretsPermissionDeniedMessage
	}

	return true, ""
}

func (s *SecretsServiceImpl) Get(
	ctx context.Context,
	req *secrets.SecretGetRequest,
) (*secrets.SecretGetResponse, error) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return &secrets.SecretGetResponse{Error: new(msg)}, nil
	}

	stored, err := s.findByKey(ctx, req.Key)
	if err != nil {
		s.logStorageFailure(ctx, "failed to read plugin secret", err)

		return &secrets.SecretGetResponse{Error: new(secretReadFailureMessage)}, nil
	}

	if stored == nil {
		return &secrets.SecretGetResponse{Found: false}, nil
	}

	value, err := s.cipher.DecryptWithAAD(stored.Value, secretAAD(s.pluginID, stored.Key))
	if err != nil {
		// The failure is either a misconfigured key or a tampered row; the
		// plugin can act on neither, so the detail stays in the panel log.
		slog.ErrorContext(ctx, "failed to decrypt plugin secret",
			slog.Uint64("plugin_id", s.pluginID),
			slog.String("key", stored.Key),
			slog.String("error", err.Error()))

		return &secrets.SecretGetResponse{Error: new("failed to decrypt secret")}, nil
	}

	return &secrets.SecretGetResponse{
		Value: value,
		Found: true,
	}, nil
}

func (s *SecretsServiceImpl) Set(
	ctx context.Context,
	req *secrets.SecretSetRequest,
) (*secrets.SecretSetResponse, error) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return &secrets.SecretSetResponse{Error: new(msg)}, nil
	}

	if !secretKeyRegex.MatchString(req.Key) {
		return secretSetFailure("secret key must match " + secretKeyRegex.String()), nil
	}

	if len(req.Value) > s.cfg.MaxValueBytes {
		return secretSetFailure("secret value exceeds " +
			strconv.Itoa(s.cfg.MaxValueBytes) + " bytes"), nil
	}

	if s.cfg.RequireEncryption && !s.cipher.Enabled() {
		slog.WarnContext(ctx, "plugin secret write rejected: encryption is not configured",
			slog.Uint64("plugin_id", s.pluginID))

		return secretSetFailure(encryptionDisabledMessage), nil
	}

	stored, err := s.findByKey(ctx, req.Key)
	if err != nil {
		s.logStorageFailure(ctx, "failed to read plugin secret", err)

		return secretSetFailure(secretReadFailureMessage), nil
	}

	if stored == nil {
		count, countErr := s.repo.CountByPlugin(ctx, domain.Uint64ID(s.pluginID))
		if countErr != nil {
			s.logStorageFailure(ctx, "failed to count plugin secrets", countErr)

			return secretSetFailure(secretReadFailureMessage), nil
		}

		if count >= s.cfg.MaxKeysPerPlugin {
			return secretSetFailure("at most " +
				strconv.Itoa(s.cfg.MaxKeysPerPlugin) + " secrets per plugin"), nil
		}
	}

	encrypted, err := s.cipher.EncryptWithAAD(req.Value, secretAAD(s.pluginID, req.Key))
	if err != nil {
		slog.ErrorContext(ctx, "failed to encrypt plugin secret",
			slog.Uint64("plugin_id", s.pluginID),
			slog.String("error", err.Error()))

		return secretSetFailure("failed to encrypt secret"), nil
	}

	entry := &domain.PluginSecret{
		PluginID: domain.Uint64ID(s.pluginID),
		Key:      req.Key,
		Value:    encrypted,
	}

	if err := s.repo.Upsert(ctx, entry); err != nil {
		s.logStorageFailure(ctx, "failed to store plugin secret", err)

		return secretSetFailure(secretWriteFailureMessage), nil
	}

	return &secrets.SecretSetResponse{Success: true}, nil
}

func (s *SecretsServiceImpl) Delete(
	ctx context.Context,
	req *secrets.SecretDeleteRequest,
) (*secrets.SecretDeleteResponse, error) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return &secrets.SecretDeleteResponse{Error: new(msg)}, nil
	}

	if err := s.repo.Delete(ctx, domain.Uint64ID(s.pluginID), req.Key); err != nil {
		s.logStorageFailure(ctx, "failed to delete plugin secret", err)

		return &secrets.SecretDeleteResponse{Error: new(secretDeleteFailureMessage)}, nil
	}

	return &secrets.SecretDeleteResponse{Success: true}, nil
}

func (s *SecretsServiceImpl) ListKeys(
	ctx context.Context,
	req *secrets.SecretListKeysRequest,
) (*secrets.SecretListKeysResponse, error) {
	if allowed, msg := s.authorize(ctx); !allowed {
		return &secrets.SecretListKeysResponse{Error: new(msg)}, nil
	}

	stored, err := s.repo.Find(ctx, &filters.FindPluginSecret{
		PluginIDs: []domain.Uint64ID{domain.Uint64ID(s.pluginID)},
	}, nil, nil)
	if err != nil {
		s.logStorageFailure(ctx, "failed to list plugin secrets", err)

		return &secrets.SecretListKeysResponse{Error: new(secretListFailureMessage)}, nil
	}

	keys := make([]string, 0, len(stored))

	for _, entry := range stored {
		if req.KeyPrefix != nil && !strings.HasPrefix(entry.Key, *req.KeyPrefix) {
			continue
		}

		keys = append(keys, entry.Key)
	}

	slices.Sort(keys)

	return &secrets.SecretListKeysResponse{Keys: keys}, nil
}

func (s *SecretsServiceImpl) findByKey(ctx context.Context, key string) (*domain.PluginSecret, error) {
	stored, err := s.repo.Find(ctx, &filters.FindPluginSecret{
		PluginIDs: []domain.Uint64ID{domain.Uint64ID(s.pluginID)},
		Keys:      []string{key},
	}, nil, &filters.Pagination{Limit: 1})
	if err != nil {
		return nil, err
	}

	if len(stored) == 0 {
		return nil, nil
	}

	return &stored[0], nil
}

func (s *SecretsServiceImpl) logStorageFailure(ctx context.Context, message string, err error) {
	slog.ErrorContext(ctx, message,
		slog.Uint64("plugin_id", s.pluginID),
		slog.String("error", err.Error()))
}

func secretSetFailure(message string) *secrets.SecretSetResponse {
	return &secrets.SecretSetResponse{
		Success: false,
		Error:   new(message),
	}
}

// secretAAD binds a ciphertext to the row that holds it: a value copied into
// another plugin's row, under another key, or from a column encrypted with the
// same process key no longer decrypts.
func secretAAD(pluginID uint64, key string) string {
	return "plugin-secret:" + strconv.FormatUint(pluginID, 10) + ":" + key
}

type SecretsHostLibrary struct {
	impl *SecretsServiceImpl
}

func NewSecretsHostLibrary(
	pluginID uint64,
	repo repositories.PluginSecretRepository,
	cipher *secret.Cipher,
	checker PluginPermissionChecker,
	cfg SecretsConfig,
) *SecretsHostLibrary {
	return &SecretsHostLibrary{
		impl: NewSecretsService(pluginID, repo, cipher, checker, cfg),
	}
}

func (l *SecretsHostLibrary) Instantiate(ctx context.Context, r wazero.Runtime) error {
	return secrets.Instantiate(ctx, r, l.impl)
}

type SecretsHostLibraryFactory struct {
	repo    repositories.PluginSecretRepository
	cipher  *secret.Cipher
	checker PluginPermissionChecker
	cfg     SecretsConfig
}

func NewSecretsHostLibraryFactory(
	repo repositories.PluginSecretRepository,
	cipher *secret.Cipher,
	checker PluginPermissionChecker,
	cfg SecretsConfig,
) *SecretsHostLibraryFactory {
	return &SecretsHostLibraryFactory{
		repo:    repo,
		cipher:  cipher,
		checker: checker,
		cfg:     cfg,
	}
}

func (f *SecretsHostLibraryFactory) Create(pluginID uint64) pkgplugin.HostLibrary {
	return NewSecretsHostLibrary(pluginID, f.repo, f.cipher, f.checker, f.cfg)
}
