// Security tests for the gameap-secrets host library.
//
// OWASP API Security Top 10:2023 — API1:2023 Broken Object Level
// Authorization (a plugin must only ever reach its own secrets), API3:2023
// Broken Object Property Level Authorization (the "secrets" grant gates every
// method) and API8:2023 Security Misconfiguration (a credential must never
// land in the database as plaintext).
package hostlibrary

import (
	"context"
	"strings"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/pkg/plugin/sdk/secrets"
	"github.com/gameap/gameap/pkg/secret"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

const secretsTestPluginID = uint64(21)

type secretsTestEnv struct {
	service *SecretsServiceImpl
	repo    *inmemory.PluginSecretRepository
	cipher  *secret.Cipher
}

func newSecretsEnv(
	t *testing.T,
	pluginID uint64,
	checker PluginPermissionChecker,
	cipher *secret.Cipher,
	cfg SecretsConfig,
) secretsTestEnv {
	t.Helper()

	repo := inmemory.NewPluginSecretRepository()

	return secretsTestEnv{
		service: NewSecretsService(pluginID, repo, cipher, checker, cfg),
		repo:    repo,
		cipher:  cipher,
	}
}

func newAllowedSecretsEnv(t *testing.T) secretsTestEnv {
	t.Helper()

	cipher, err := secret.NewCipher("test-encryption-key")
	require.NoError(t, err)

	return newSecretsEnv(t, secretsTestPluginID, stubPermissionChecker{allowed: true}, cipher, SecretsConfig{
		RequireEncryption: true,
	})
}

func (e secretsTestEnv) storedValue(t *testing.T, pluginID uint64, key string) string {
	t.Helper()

	stored, err := e.repo.Find(context.Background(), &filters.FindPluginSecret{
		PluginIDs: []domain.Uint64ID{domain.Uint64ID(pluginID)},
		Keys:      []string{key},
	}, nil, nil)
	require.NoError(t, err)
	require.Len(t, stored, 1)

	return stored[0].Value
}

func TestSecretsService_Set(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		key       string
		value     string
		cfg       SecretsConfig
		setupRepo func(t *testing.T, repo *inmemory.PluginSecretRepository)
		wantOK    bool
		wantError string
	}{
		{
			name:   "stores_new_secret",
			key:    "steam_api_key",
			value:  "sk-live-0123456789",
			wantOK: true,
		},
		{
			name:  "replaces_existing_value",
			key:   "steam_api_key",
			value: "second-value",
			setupRepo: func(t *testing.T, repo *inmemory.PluginSecretRepository) {
				t.Helper()
				require.NoError(t, repo.Upsert(context.Background(), &domain.PluginSecret{
					PluginID: domain.Uint64ID(secretsTestPluginID),
					Key:      "steam_api_key",
					Value:    "enc:whatever",
				}))
			},
			wantOK: true,
		},
		{
			name:      "rejects_key_with_spaces",
			key:       "my key",
			value:     "v",
			wantError: "secret key must match",
		},
		{
			name:      "rejects_empty_key",
			key:       "",
			value:     "v",
			wantError: "secret key must match",
		},
		{
			name:      "rejects_key_starting_with_separator",
			key:       "-leading",
			value:     "v",
			wantError: "secret key must match",
		},
		{
			name:      "rejects_oversized_value",
			key:       "token",
			value:     strings.Repeat("x", 33),
			cfg:       SecretsConfig{MaxValueBytes: 32, RequireEncryption: true},
			wantError: "secret value exceeds 32 bytes",
		},
		{
			name:  "quota_reached_for_new_key",
			key:   "third",
			value: "v",
			cfg:   SecretsConfig{MaxKeysPerPlugin: 2, RequireEncryption: true},
			setupRepo: func(t *testing.T, repo *inmemory.PluginSecretRepository) {
				t.Helper()
				for _, key := range []string{"first", "second"} {
					require.NoError(t, repo.Upsert(context.Background(), &domain.PluginSecret{
						PluginID: domain.Uint64ID(secretsTestPluginID),
						Key:      key,
						Value:    "enc:whatever",
					}))
				}
			},
			wantError: "at most 2 secrets per plugin",
		},
		{
			name:  "quota_does_not_block_overwrite",
			key:   "first",
			value: "rotated-secret-value",
			cfg:   SecretsConfig{MaxKeysPerPlugin: 1, RequireEncryption: true},
			setupRepo: func(t *testing.T, repo *inmemory.PluginSecretRepository) {
				t.Helper()
				require.NoError(t, repo.Upsert(context.Background(), &domain.PluginSecret{
					PluginID: domain.Uint64ID(secretsTestPluginID),
					Key:      "first",
					Value:    "enc:whatever",
				}))
			},
			wantOK: true,
		},
		{
			name:  "quota_counts_only_the_calling_plugin",
			key:   "mine",
			value: "my-own-secret-value",
			cfg:   SecretsConfig{MaxKeysPerPlugin: 1, RequireEncryption: true},
			setupRepo: func(t *testing.T, repo *inmemory.PluginSecretRepository) {
				t.Helper()
				require.NoError(t, repo.Upsert(context.Background(), &domain.PluginSecret{
					PluginID: domain.Uint64ID(secretsTestPluginID + 1),
					Key:      "theirs",
					Value:    "enc:whatever",
				}))
			},
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cipher, err := secret.NewCipher("test-encryption-key")
			require.NoError(t, err)

			cfg := tt.cfg
			if cfg == (SecretsConfig{}) {
				cfg = SecretsConfig{RequireEncryption: true}
			}

			env := newSecretsEnv(t, secretsTestPluginID, stubPermissionChecker{allowed: true}, cipher, cfg)

			if tt.setupRepo != nil {
				tt.setupRepo(t, env.repo)
			}

			resp, err := env.service.Set(context.Background(), &secrets.SecretSetRequest{
				Key:   tt.key,
				Value: tt.value,
			})

			require.NoError(t, err, "host functions must report failures in the response, not as an error")
			assert.Equal(t, tt.wantOK, resp.Success)

			if tt.wantError == "" {
				assert.Nil(t, resp.Error)

				stored := env.storedValue(t, secretsTestPluginID, tt.key)
				assert.True(t, strings.HasPrefix(stored, secret.EncPrefix),
					"the persisted value must be encrypted, not plaintext")
				assert.NotContains(t, stored, tt.value)

				return
			}

			require.NotNil(t, resp.Error)
			assert.Contains(t, *resp.Error, tt.wantError)
		})
	}
}

func TestSecretsService_Get(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		storeUnder uint64
		storeKey   string
		requestKey string
		wantFound  bool
	}{
		{
			name:       "returns_stored_value",
			storeUnder: secretsTestPluginID,
			storeKey:   "token",
			requestKey: "token",
			wantFound:  true,
		},
		{
			name:       "missing_key_is_not_found",
			storeUnder: secretsTestPluginID,
			storeKey:   "token",
			requestKey: "other",
			wantFound:  false,
		},
		{
			name:       "another_plugins_secret_is_not_found",
			storeUnder: secretsTestPluginID + 1,
			storeKey:   "token",
			requestKey: "token",
			wantFound:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newAllowedSecretsEnv(t)

			encrypted, err := env.cipher.EncryptWithAAD("s3cret", secretAAD(tt.storeUnder, tt.storeKey))
			require.NoError(t, err)

			require.NoError(t, env.repo.Upsert(context.Background(), &domain.PluginSecret{
				PluginID: domain.Uint64ID(tt.storeUnder),
				Key:      tt.storeKey,
				Value:    encrypted,
			}))

			resp, err := env.service.Get(context.Background(), &secrets.SecretGetRequest{Key: tt.requestKey})

			require.NoError(t, err)
			assert.Nil(t, resp.Error)
			assert.Equal(t, tt.wantFound, resp.Found)

			if tt.wantFound {
				assert.Equal(t, "s3cret", resp.Value)
			} else {
				assert.Empty(t, resp.Value)
			}
		})
	}
}

// TestSecretsService_Get_RejectsValueBoundToAnotherRow — OWASP API1:2023:
// moving a ciphertext to another plugin's row (or another key) must not make
// it readable, which is what the AAD binding buys.
func TestSecretsService_Get_RejectsValueBoundToAnotherRow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		aadFrom func() string
	}{
		{
			name:    "sealed_for_another_plugin",
			aadFrom: func() string { return secretAAD(secretsTestPluginID+1, "token") },
		},
		{
			name:    "sealed_for_another_key",
			aadFrom: func() string { return secretAAD(secretsTestPluginID, "other_token") },
		},
		{
			name:    "sealed_without_any_context",
			aadFrom: func() string { return "" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newAllowedSecretsEnv(t)

			foreign, err := env.cipher.EncryptWithAAD("s3cret", tt.aadFrom())
			require.NoError(t, err)

			require.NoError(t, env.repo.Upsert(context.Background(), &domain.PluginSecret{
				PluginID: domain.Uint64ID(secretsTestPluginID),
				Key:      "token",
				Value:    foreign,
			}))

			resp, err := env.service.Get(context.Background(), &secrets.SecretGetRequest{Key: "token"})

			require.NoError(t, err)
			assert.False(t, resp.Found)
			assert.Empty(t, resp.Value, "no plaintext may leak when authentication fails")
			require.NotNil(t, resp.Error)
			assert.Contains(t, *resp.Error, "failed to decrypt secret")
		})
	}
}

func TestSecretsService_Delete(t *testing.T) {
	t.Parallel()
	env := newAllowedSecretsEnv(t)
	ctx := context.Background()

	for _, pluginID := range []uint64{secretsTestPluginID, secretsTestPluginID + 1} {
		require.NoError(t, env.repo.Upsert(ctx, &domain.PluginSecret{
			PluginID: domain.Uint64ID(pluginID),
			Key:      "token",
			Value:    "enc:whatever",
		}))
	}

	resp, err := env.service.Delete(ctx, &secrets.SecretDeleteRequest{Key: "token"})

	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Nil(t, resp.Error)

	own, err := env.repo.CountByPlugin(ctx, domain.Uint64ID(secretsTestPluginID))
	require.NoError(t, err)
	assert.Equal(t, 0, own)

	// Deleting is scoped to the caller: another plugin's identically named
	// secret must survive.
	other, err := env.repo.CountByPlugin(ctx, domain.Uint64ID(secretsTestPluginID+1))
	require.NoError(t, err)
	assert.Equal(t, 1, other)
}

func TestSecretsService_Delete_MissingKeySucceeds(t *testing.T) {
	t.Parallel()
	env := newAllowedSecretsEnv(t)

	resp, err := env.service.Delete(context.Background(), &secrets.SecretDeleteRequest{Key: "absent"})

	require.NoError(t, err)
	assert.True(t, resp.Success, "deleting an absent key is not an error")
	assert.Nil(t, resp.Error)
}

func TestSecretsService_ListKeys(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		keyPrefix *string
		wantKeys  []string
	}{
		{
			name:     "lists_own_keys_sorted",
			wantKeys: []string{"alpha", "beta.one", "beta.two"},
		},
		{
			name:      "prefix_filters_keys",
			keyPrefix: new("beta."),
			wantKeys:  []string{"beta.one", "beta.two"},
		},
		{
			name:      "unmatched_prefix_returns_nothing",
			keyPrefix: new("gamma"),
			wantKeys:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newAllowedSecretsEnv(t)
			ctx := context.Background()

			for _, key := range []string{"beta.two", "alpha", "beta.one"} {
				require.NoError(t, env.repo.Upsert(ctx, &domain.PluginSecret{
					PluginID: domain.Uint64ID(secretsTestPluginID),
					Key:      key,
					Value:    "enc:whatever",
				}))
			}

			require.NoError(t, env.repo.Upsert(ctx, &domain.PluginSecret{
				PluginID: domain.Uint64ID(secretsTestPluginID + 1),
				Key:      "foreign",
				Value:    "enc:whatever",
			}))

			resp, err := env.service.ListKeys(ctx, &secrets.SecretListKeysRequest{KeyPrefix: tt.keyPrefix})

			require.NoError(t, err)
			assert.Nil(t, resp.Error)
			assert.Equal(t, tt.wantKeys, resp.Keys)
		})
	}
}

// TestSecretsService_PermissionGate — OWASP API3:2023: without the "secrets"
// grant every method answers with the missing permission and touches nothing.
func TestSecretsService_PermissionGate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		checker   PluginPermissionChecker
		wantError string
	}{
		{
			name:      "grant_missing",
			checker:   stubPermissionChecker{allowed: false},
			wantError: secretsPermissionDeniedMessage,
		},
		{
			name:      "grant_lookup_broken",
			checker:   stubPermissionChecker{err: errors.New("db down")},
			wantError: "failed to check plugin permission: db down",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cipher, err := secret.NewCipher("test-encryption-key")
			require.NoError(t, err)

			env := newSecretsEnv(t, secretsTestPluginID, tt.checker, cipher, SecretsConfig{RequireEncryption: true})
			ctx := context.Background()

			setResp, err := env.service.Set(ctx, &secrets.SecretSetRequest{Key: "token", Value: "v"})
			require.NoError(t, err)
			assert.False(t, setResp.Success)
			require.NotNil(t, setResp.Error)
			assert.Contains(t, *setResp.Error, tt.wantError)

			getResp, err := env.service.Get(ctx, &secrets.SecretGetRequest{Key: "token"})
			require.NoError(t, err)
			assert.False(t, getResp.Found)
			require.NotNil(t, getResp.Error)
			assert.Contains(t, *getResp.Error, tt.wantError)

			deleteResp, err := env.service.Delete(ctx, &secrets.SecretDeleteRequest{Key: "token"})
			require.NoError(t, err)
			assert.False(t, deleteResp.Success)
			require.NotNil(t, deleteResp.Error)
			assert.Contains(t, *deleteResp.Error, tt.wantError)

			listResp, err := env.service.ListKeys(ctx, &secrets.SecretListKeysRequest{})
			require.NoError(t, err)
			assert.Empty(t, listResp.Keys)
			require.NotNil(t, listResp.Error)
			assert.Contains(t, *listResp.Error, tt.wantError)

			count, err := env.repo.CountByPlugin(ctx, domain.Uint64ID(secretsTestPluginID))
			require.NoError(t, err)
			assert.Equal(t, 0, count, "a denied call must not write anything")
		})
	}
}

// TestSecretsService_TransientLoadIsDenied — a plugin loaded without a
// database record (dry-run, info discovery) has no grants, so the real
// repository-backed checker must deny it.
func TestSecretsService_TransientLoadIsDenied(t *testing.T) {
	t.Parallel()
	cipher, err := secret.NewCipher("test-encryption-key")
	require.NoError(t, err)

	checker := NewRepositoryPermissionChecker(inmemory.NewPluginRepository())
	env := newSecretsEnv(t, 0, checker, cipher, SecretsConfig{RequireEncryption: true})

	resp, err := env.service.Set(context.Background(), &secrets.SecretSetRequest{Key: "token", Value: "v"})

	require.NoError(t, err)
	assert.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	assert.Contains(t, *resp.Error, secretsPermissionDeniedMessage)
}

// TestSecretsService_Set_RequiresEncryption — OWASP API8:2023: with no
// ENCRYPTION_KEY configured the write is refused instead of silently storing
// the credential in plaintext.
func TestSecretsService_Set_RequiresEncryption(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		requireEncryption bool
		wantOK            bool
		wantError         string
	}{
		{
			name:              "refuses_write_by_default",
			requireEncryption: true,
			wantError:         encryptionDisabledMessage,
		},
		{
			name:              "operator_opt_out_stores_plaintext",
			requireEncryption: false,
			wantOK:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newSecretsEnv(t, secretsTestPluginID, stubPermissionChecker{allowed: true}, secret.Disabled(),
				SecretsConfig{RequireEncryption: tt.requireEncryption})
			ctx := context.Background()

			resp, err := env.service.Set(ctx, &secrets.SecretSetRequest{Key: "token", Value: "plain"})

			require.NoError(t, err)
			assert.Equal(t, tt.wantOK, resp.Success)

			count, err := env.repo.CountByPlugin(ctx, domain.Uint64ID(secretsTestPluginID))
			require.NoError(t, err)

			if tt.wantError != "" {
				require.NotNil(t, resp.Error)
				assert.Contains(t, *resp.Error, tt.wantError)
				assert.Equal(t, 0, count, "a refused write must not reach the database")

				return
			}

			assert.Nil(t, resp.Error)
			assert.Equal(t, 1, count)

			// The disabled cipher is a documented passthrough; reading the
			// value back still works so the opt-out stays usable.
			getResp, err := env.service.Get(ctx, &secrets.SecretGetRequest{Key: "token"})
			require.NoError(t, err)
			assert.True(t, getResp.Found)
			assert.Equal(t, "plain", getResp.Value)
		})
	}
}

// TestSecretsService_ConfigDefaults — a zero-valued config must not disable
// the quotas.
func TestSecretsService_ConfigDefaults(t *testing.T) {
	t.Parallel()
	service := NewSecretsService(secretsTestPluginID, inmemory.NewPluginSecretRepository(),
		secret.Disabled(), stubPermissionChecker{allowed: true}, SecretsConfig{})

	assert.Equal(t, defaultMaxSecretsPerPlugin, service.cfg.MaxKeysPerPlugin)
	assert.Equal(t, defaultMaxSecretValueBytes, service.cfg.MaxValueBytes)
}

// TestSecretsHostLibrary_Instantiate pins the wasm ABI a guest imports: the
// module name and the four function names/signatures. A rename here silently
// breaks every compiled plugin, which would otherwise only surface as a load
// failure in production.
func TestSecretsHostLibrary_Instantiate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	runtime := wazero.NewRuntime(ctx)
	t.Cleanup(func() {
		require.NoError(t, runtime.Close(ctx))
	})

	library := NewSecretsHostLibrary(secretsTestPluginID, inmemory.NewPluginSecretRepository(),
		secret.Disabled(), stubPermissionChecker{allowed: true}, SecretsConfig{})

	require.NoError(t, library.Instantiate(ctx, runtime))

	module := runtime.Module("gameap-secrets")
	require.NotNil(t, module, "guests import the module by this exact name")

	definitions := module.ExportedFunctionDefinitions()

	for _, name := range []string{"get", "set", "delete", "list_keys"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			definition, ok := definitions[name]
			require.True(t, ok, "the module must export the function a guest imports")

			assert.Equal(t, []api.ValueType{api.ValueTypeI32, api.ValueTypeI32}, definition.ParamTypes(),
				"host functions take (ptr, size)")
			assert.Equal(t, []api.ValueType{api.ValueTypeI64}, definition.ResultTypes(),
				"host functions return the packed (ptr << 32) | size")
		})
	}

	assert.Len(t, definitions, 4, "an extra export would be an undocumented ABI addition")
}

// brokenSecretRepo fails every storage call so the responses handed to the
// guest can be inspected.
type brokenSecretRepo struct {
	repositories.PluginSecretRepository

	err error
}

func (r *brokenSecretRepo) Find(
	context.Context,
	*filters.FindPluginSecret,
	[]filters.Sorting,
	*filters.Pagination,
) ([]domain.PluginSecret, error) {
	return nil, r.err
}

func (r *brokenSecretRepo) Upsert(context.Context, *domain.PluginSecret) error {
	return r.err
}

func (r *brokenSecretRepo) Delete(context.Context, domain.Uint64ID, string) error {
	return r.err
}

// TestSecretsService_StorageErrorsAreNotLeaked — OWASP API8:2023: the driver's
// error text can name tables, hosts or credentials, and the plugin can act on
// none of it, so responses carry a fixed message instead.
func TestSecretsService_StorageErrorsAreNotLeaked(t *testing.T) {
	t.Parallel()
	const driverDetail = "pq: relation \"plugin_secrets\" does not exist (host=db.internal)"

	cipher, err := secret.NewCipher("test-encryption-key")
	require.NoError(t, err)

	repo := &brokenSecretRepo{
		PluginSecretRepository: inmemory.NewPluginSecretRepository(),
		err:                    errors.New(driverDetail),
	}

	service := NewSecretsService(secretsTestPluginID, repo, cipher,
		stubPermissionChecker{allowed: true}, SecretsConfig{RequireEncryption: true})
	ctx := context.Background()

	tests := []struct {
		name        string
		call        func() (*string, error)
		wantMessage string
	}{
		{
			name: "set",
			call: func() (*string, error) {
				resp, err := service.Set(ctx, &secrets.SecretSetRequest{Key: "token", Value: "v"})

				return resp.Error, err
			},
			wantMessage: secretReadFailureMessage,
		},
		{
			name: "get",
			call: func() (*string, error) {
				resp, err := service.Get(ctx, &secrets.SecretGetRequest{Key: "token"})

				return resp.Error, err
			},
			wantMessage: secretReadFailureMessage,
		},
		{
			name: "delete",
			call: func() (*string, error) {
				resp, err := service.Delete(ctx, &secrets.SecretDeleteRequest{Key: "token"})

				return resp.Error, err
			},
			wantMessage: secretDeleteFailureMessage,
		},
		{
			name: "list_keys",
			call: func() (*string, error) {
				resp, err := service.ListKeys(ctx, &secrets.SecretListKeysRequest{})

				return resp.Error, err
			},
			wantMessage: secretListFailureMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			respError, err := tt.call()

			require.NoError(t, err)
			require.NotNil(t, respError)
			assert.Equal(t, tt.wantMessage, *respError)
			assert.NotContains(t, *respError, driverDetail)
		})
	}
}
