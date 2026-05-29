// White-box tests for the pure-logic, fallback and driver-selection paths of
// the DI Container. They use in-memory drivers and require no live infra.
//
// Security-relevant accessors are covered with explicit OWASP API Security
// Top 10:2023 / OWASP ASVS 4.0.3 references:
//   - SecretCipher          — secrets at rest (API8:2023 Security Misconfiguration; ASVS §6.2)
//   - PasswordBlocklist     — common-password blocklist (API2:2023 Broken Authentication; ASVS §2.1.7)
//   - createTwoFactorManager — multi-factor primitives (API2:2023 Broken Authentication; ASVS §2.8)

package application

import (
	"context"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/cache"
	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/pubsub"
	"github.com/gameap/gameap/internal/pubsub/dlq"
	"github.com/gameap/gameap/internal/pubsub/retry"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	dlqDriverDatabase = "database"
	dlqDriverDBAlias  = "db"
	dlqDriverMemory   = "memory"
)

func TestContainerShutdown(t *testing.T) {
	t.Run("runs_shutdown_then_late_shutdown_hooks_in_registration_order", func(t *testing.T) {
		// ARRANGE
		c := newMinimalContainer(&config.Config{})
		var order []string

		c.appendShutdownFunc(func() error {
			order = append(order, "early-1")

			return nil
		})
		c.appendShutdownFunc(func() error {
			order = append(order, "early-2")

			return nil
		})
		c.appendLateShutdownFunc(func() error {
			order = append(order, "late-1")

			return nil
		})

		// ACT
		err := c.Shutdown()

		// ASSERT
		require.NoError(t, err)
		assert.Equal(t, []string{"early-1", "early-2", "late-1"}, order,
			"hooks must run early-then-late in registration order")
	})

	t.Run("erroring_hook_is_logged_but_does_not_abort_remaining_hooks", func(t *testing.T) {
		// ARRANGE
		c := newMinimalContainer(&config.Config{})
		var ran []string

		c.appendShutdownFunc(func() error {
			ran = append(ran, "first")

			return assert.AnError
		})
		c.appendShutdownFunc(func() error {
			ran = append(ran, "second")

			return nil
		})
		c.appendLateShutdownFunc(func() error {
			ran = append(ran, "late")

			return assert.AnError
		})

		// ACT
		err := c.Shutdown()

		// ASSERT
		require.NoError(t, err, "Shutdown swallows hook errors and always returns nil")
		assert.Equal(t, []string{"first", "second", "late"}, ran,
			"an erroring hook must not stop later hooks from running")
	})

	t.Run("invokes_cancel_when_set", func(t *testing.T) {
		// ARRANGE
		c := newMinimalContainer(&config.Config{})
		ctx, cancel := context.WithCancel(context.Background())
		c.SetContext(ctx, cancel)

		// ACT
		err := c.Shutdown()

		// ASSERT
		require.NoError(t, err)
		require.Error(t, ctx.Err(), "Shutdown must cancel the context")
		assert.ErrorIs(t, ctx.Err(), context.Canceled)
	})

	t.Run("is_safe_with_nil_session_registry_and_servers", func(t *testing.T) {
		// ARRANGE
		c := newMinimalContainer(&config.Config{})
		require.Nil(t, c.sessionRegistry)
		require.Nil(t, c.httpServer)
		require.Nil(t, c.grpcServer)

		// ACT
		err := c.Shutdown()

		// ASSERT
		require.NoError(t, err, "nil registry / servers must be skipped without panic")
	})
}

// TestContainerSecretCipher covers at-rest secret encryption selection.
// OWASP API8:2023 Security Misconfiguration / ASVS §6.2 (cryptography at rest):
// the cipher must be disabled (logged plaintext passthrough) only when no key is
// configured, and enabled whenever a key is present.
func TestContainerSecretCipher(t *testing.T) {
	t.Run("empty_encryption_key_yields_disabled_cipher", func(t *testing.T) {
		// ARRANGE
		c := newMinimalContainer(&config.Config{EncryptionKey: ""})

		// ACT
		cipher := c.SecretCipher()

		// ASSERT
		require.NotNil(t, cipher)
		assert.False(t, cipher.Enabled(), "no ENCRYPTION_KEY must disable the cipher")
	})

	t.Run("valid_32_byte_key_yields_enabled_cipher", func(t *testing.T) {
		// ARRANGE
		c := newMinimalContainer(&config.Config{EncryptionKey: wiredEncryptionKey})

		// ACT
		cipher := c.SecretCipher()

		// ASSERT
		require.NotNil(t, cipher)
		assert.True(t, cipher.Enabled(), "a configured ENCRYPTION_KEY must enable the cipher")
	})

	t.Run("returns_same_cached_instance_on_second_call", func(t *testing.T) {
		// ARRANGE
		c := newMinimalContainer(&config.Config{EncryptionKey: wiredEncryptionKey})

		// ACT
		first := c.SecretCipher()
		second := c.SecretCipher()

		// ASSERT
		assert.Same(t, first, second, "SecretCipher must memoise the cipher instance")
	})
}

// TestContainerPasswordBlocklist covers the common-password blocklist accessor.
// OWASP API2:2023 Broken Authentication / ASVS §2.1.7: registrations and
// password changes must be checked against a breached-password list unless the
// operator has explicitly opted out.
func TestContainerPasswordBlocklist(t *testing.T) {
	t.Run("allow_weak_passwords_yields_noop_blocklist", func(t *testing.T) {
		// ARRANGE
		cfg := &config.Config{}
		cfg.Auth.AllowWeakPasswords = true
		c := newMinimalContainer(cfg)

		// ACT
		bl := c.PasswordBlocklist()

		// ASSERT
		require.NotNil(t, bl)
		assert.IsType(t, auth.NoopBlocklist{}, bl,
			"AUTH_ALLOW_WEAK_PASSWORDS must skip the loader entirely")
		assert.Zero(t, bl.Len(), "noop blocklist contains no entries")
	})

	t.Run("default_config_loads_embedded_blocklist", func(t *testing.T) {
		// ARRANGE
		c := newMinimalContainer(&config.Config{})

		// ACT
		bl := c.PasswordBlocklist()

		// ASSERT
		require.NotNil(t, bl)
		assert.Greater(t, bl.Len(), 0, "embedded blocklist must contain entries")
		assert.IsType(t, &auth.MapBlocklist{}, bl,
			"default config must load the embedded map-backed blocklist, not the noop")
	})

	t.Run("returns_same_instance_on_second_call", func(t *testing.T) {
		// ARRANGE
		c := newMinimalContainer(&config.Config{})

		// ACT
		first := c.PasswordBlocklist()
		second := c.PasswordBlocklist()

		// ASSERT
		assert.Same(t, first, second, "sync.Once must load the blocklist exactly once")
	})
}

// TestCreateTwoFactorManager covers two-factor primitive key derivation.
// OWASP API2:2023 Broken Authentication / ASVS §2.8: the 2FA secret-at-rest key
// derives from ENCRYPTION_KEY when present, otherwise AUTH_SECRET, and the
// manager must refuse to initialise when neither is configured.
func TestCreateTwoFactorManager(t *testing.T) {
	t.Run("uses_encryption_key_when_set", func(t *testing.T) {
		// ARRANGE
		cfg := &config.Config{EncryptionKey: wiredEncryptionKey, AuthSecret: wiredAuthSecret}
		c := newMinimalContainer(cfg)

		// ACT
		mgr := c.createTwoFactorManager()

		// ASSERT
		require.NotNil(t, mgr, "a valid ENCRYPTION_KEY must yield a manager")
	})

	t.Run("falls_back_to_auth_secret_when_encryption_key_empty", func(t *testing.T) {
		// ARRANGE
		cfg := &config.Config{EncryptionKey: "", AuthSecret: wiredAuthSecret}
		c := newMinimalContainer(cfg)

		// ACT
		mgr := c.createTwoFactorManager()

		// ASSERT
		require.NotNil(t, mgr, "an empty ENCRYPTION_KEY must fall back to AUTH_SECRET")
	})

	t.Run("panics_when_neither_key_is_set", func(t *testing.T) {
		// ARRANGE
		c := newMinimalContainer(&config.Config{EncryptionKey: "", AuthSecret: ""})

		// ACT / ASSERT
		assert.PanicsWithValue(t,
			"neither ENCRYPTION_KEY nor AUTH_SECRET is set; cannot initialise two-factor manager",
			func() { c.createTwoFactorManager() },
			"both keys empty must panic")
	})
}

func TestBuildRetryConfig(t *testing.T) {
	t.Run("copies_max_retries_and_multiplier_and_parses_valid_durations", func(t *testing.T) {
		// ARRANGE
		cfg := &config.Config{}
		cfg.PubSub.Retry.MaxRetries = 7
		cfg.PubSub.Retry.Multiplier = 3.5
		cfg.PubSub.Retry.InitialDelay = "250ms"
		cfg.PubSub.Retry.MaxDelay = "30s"
		c := newMinimalContainer(cfg)

		// ACT
		got := c.buildRetryConfig()

		// ASSERT
		assert.Equal(t, 7, got.MaxRetries, "MaxRetries must be copied from config")
		assert.InEpsilon(t, 3.5, got.Multiplier, 1e-9, "Multiplier must be copied from config")
		assert.Equal(t, 250*time.Millisecond, got.InitialDelay, "valid InitialDelay must be parsed")
		assert.Equal(t, 30*time.Second, got.MaxDelay, "valid MaxDelay must be parsed")
	})

	t.Run("invalid_duration_strings_keep_default_config_values", func(t *testing.T) {
		// ARRANGE
		cfg := &config.Config{}
		cfg.PubSub.Retry.MaxRetries = 2
		cfg.PubSub.Retry.Multiplier = 1.5
		cfg.PubSub.Retry.InitialDelay = "not-a-duration"
		cfg.PubSub.Retry.MaxDelay = ""
		c := newMinimalContainer(cfg)

		defaults := retry.DefaultConfig()

		// ACT
		got := c.buildRetryConfig()

		// ASSERT
		assert.Equal(t, 2, got.MaxRetries, "MaxRetries is still copied even with bad durations")
		assert.InEpsilon(t, 1.5, got.Multiplier, 1e-9)
		assert.Equal(t, defaults.InitialDelay, got.InitialDelay,
			"unparseable InitialDelay must leave the default intact")
		assert.Equal(t, defaults.MaxDelay, got.MaxDelay,
			"empty MaxDelay must leave the default intact")
	})
}

func TestCreateCache(t *testing.T) {
	tests := []struct {
		name      string
		driver    string
		wantPanic string
	}{
		{name: "memory_driver_yields_in_memory_cache", driver: cacheDriverMemory},
		{name: "inmemory_driver_yields_in_memory_cache", driver: cacheDriverInmemory},
		{name: "unknown_driver_panics", driver: "weird", wantPanic: "invalid cache driver: weird"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			cfg := &config.Config{}
			cfg.Cache.Driver = tt.driver
			c := newMinimalContainer(cfg)

			// ACT / ASSERT
			if tt.wantPanic != "" {
				assert.PanicsWithValue(t, tt.wantPanic, func() { c.createCache() })

				return
			}

			got := c.createCache()
			require.NotNil(t, got)
			assert.IsType(t, &cache.InMemory{}, got, "memory drivers must build an in-memory cache")
		})
	}
}

func TestCreateBasePubSub(t *testing.T) {
	tests := []struct {
		name      string
		driver    string
		wantPanic string
	}{
		{name: "memory_driver_yields_memory_pubsub", driver: pubsubDriverMemory},
		{name: "empty_driver_defaults_to_memory_pubsub", driver: ""},
		{name: "unknown_driver_panics", driver: "weird", wantPanic: "invalid pub-sub driver: weird"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			cfg := &config.Config{}
			cfg.PubSub.Driver = tt.driver
			c := newMinimalContainer(cfg)

			// ACT / ASSERT
			if tt.wantPanic != "" {
				assert.PanicsWithValue(t, tt.wantPanic, func() { c.createBasePubSub() })

				return
			}

			got := c.createBasePubSub()
			require.NotNil(t, got, "memory pub-sub must be constructed")
		})
	}
}

func TestCreatePubSub(t *testing.T) {
	t.Run("retry_disabled_returns_base_pubsub_directly", func(t *testing.T) {
		// ARRANGE
		cfg := &config.Config{}
		cfg.PubSub.Driver = pubsubDriverMemory
		cfg.PubSub.Retry.Enabled = false
		c := newMinimalContainer(cfg)

		// ACT
		got := c.createPubSub()

		// ASSERT
		require.NotNil(t, got)
		_, wrapped := got.(*wrappedPubSub)
		assert.False(t, wrapped, "with retry disabled the base pub-sub must be returned unwrapped")
	})

	t.Run("retry_enabled_without_dlq_returns_wrapped_pubsub", func(t *testing.T) {
		// ARRANGE
		cfg := &config.Config{}
		cfg.PubSub.Driver = pubsubDriverMemory
		cfg.PubSub.Retry.Enabled = true
		cfg.PubSub.DLQ.Enabled = false
		c := newMinimalContainer(cfg)

		// ACT
		got := c.createPubSub()

		// ASSERT
		require.IsType(t, &wrappedPubSub{}, got, "retry must wrap the base pub-sub")
		require.NotNil(t, got.(*wrappedPubSub).publisher, "retry publisher must be wired")
	})

	t.Run("retry_enabled_with_dlq_returns_wrapped_pubsub", func(t *testing.T) {
		// ARRANGE
		cfg := &config.Config{}
		cfg.PubSub.Driver = pubsubDriverMemory
		cfg.PubSub.Retry.Enabled = true
		cfg.PubSub.DLQ.Enabled = true
		cfg.PubSub.DLQ.Driver = dlqDriverMemory
		cfg.PubSub.DLQ.MaxSize = 16
		c := newMinimalContainer(cfg)

		// ACT
		got := c.createPubSub()

		// ASSERT
		require.IsType(t, &wrappedPubSub{}, got, "retry + DLQ must still yield a wrapped pub-sub")
		require.NotNil(t, got.(*wrappedPubSub).publisher)
	})
}

func TestCreateDLQStore(t *testing.T) {
	t.Run("default_driver_yields_in_memory_store", func(t *testing.T) {
		// ARRANGE
		cfg := &config.Config{}
		cfg.PubSub.DLQ.Driver = dlqDriverMemory
		cfg.PubSub.DLQ.MaxSize = 8
		c := newMinimalContainer(cfg)

		// ACT
		got := c.createDLQStore()

		// ASSERT
		require.NotNil(t, got)
		assert.IsType(t, &dlq.MemoryStore{}, got, "non-database driver must use the in-memory store")
	})

	t.Run("database_driver_yields_dlq_repository", func(t *testing.T) {
		// ARRANGE
		c := newWiredContainer(t, func(cfg *config.Config) {
			cfg.PubSub.DLQ.Driver = dlqDriverDatabase
		})

		// ACT
		got := c.createDLQStore()

		// ASSERT
		require.NotNil(t, got)
		assert.Same(t, c.DLQRepository(), got,
			"database driver must return the wired DLQ repository as the store")
	})

	t.Run("db_alias_yields_dlq_repository", func(t *testing.T) {
		// ARRANGE
		c := newWiredContainer(t, func(cfg *config.Config) {
			cfg.PubSub.DLQ.Driver = dlqDriverDBAlias
		})

		// ACT
		got := c.createDLQStore()

		// ASSERT
		assert.Same(t, c.DLQRepository(), got, "the db alias must behave like database")
	})
}

// recordingPublisher captures Publish calls so wrappedPubSub delegation can be
// asserted without touching the embedded PubSub.
type recordingPublisher struct {
	calls    int
	channels []string
	msgs     []*pubsub.Message
	ret      error
}

func (p *recordingPublisher) Publish(_ context.Context, channel string, msg *pubsub.Message) error {
	p.calls++
	p.channels = append(p.channels, channel)
	p.msgs = append(p.msgs, msg)

	return p.ret
}

// panicPubSub is an embedded PubSub whose Publish must never be reached by a
// wrappedPubSub (it overrides Publish with the retry publisher).
type panicPubSub struct {
	pubsub.PubSub
}

func (panicPubSub) Publish(_ context.Context, _ string, _ *pubsub.Message) error {
	panic("wrappedPubSub.Publish must delegate to the retry publisher, not the embedded PubSub")
}

func TestWrappedPubSubPublish(t *testing.T) {
	t.Run("delegates_to_retry_publisher_not_embedded_pubsub", func(t *testing.T) {
		// ARRANGE
		rec := &recordingPublisher{}
		w := &wrappedPubSub{
			PubSub:    panicPubSub{},
			publisher: rec,
		}
		msg := &pubsub.Message{Type: "task.created"}

		// ACT
		err := w.Publish(context.Background(), "daemon.42", msg)

		// ASSERT
		require.NoError(t, err)
		require.Equal(t, 1, rec.calls, "Publish must reach the retry publisher exactly once")
		require.Len(t, rec.channels, 1)
		assert.Equal(t, "daemon.42", rec.channels[0], "channel must be forwarded verbatim")
		assert.Same(t, msg, rec.msgs[0], "the message pointer must be forwarded unchanged")
	})

	t.Run("propagates_publisher_error", func(t *testing.T) {
		// ARRANGE
		rec := &recordingPublisher{ret: assert.AnError}
		w := &wrappedPubSub{
			PubSub:    panicPubSub{},
			publisher: rec,
		}

		// ACT
		err := w.Publish(context.Background(), "ch", &pubsub.Message{})

		// ASSERT
		require.Error(t, err)
		assert.ErrorIs(t, err, assert.AnError, "publisher errors must propagate to the caller")
	})
}

func TestContainerConfigAccessors(t *testing.T) {
	t.Run("returns_the_configured_values", func(t *testing.T) {
		// ARRANGE
		cfg := &config.Config{}
		cfg.GRPC.Port = 31718
		cfg.GRPC.ExternalHost = "grpc.example.com"
		cfg.GRPC.ExternalPort = 9100
		c := newMinimalContainer(cfg)

		// ACT / ASSERT
		assert.Same(t, cfg, c.Config(), "Config must return the same pointer it was built with")
		assert.Equal(t, uint16(31718), c.GRPCPort())
		assert.Equal(t, "grpc.example.com", c.GRPCExternalHost())
		assert.Equal(t, uint16(9100), c.GRPCExternalPort())
		assert.Equal(t, "plugins", c.PluginsDir(), "PluginsDir is a fixed relative path")
	})
}

func TestGetMultiplexerAddress(t *testing.T) {
	t.Run("uses_https_port_when_tls_enabled", func(t *testing.T) {
		// ARRANGE
		cfg := &config.Config{}
		cfg.HTTPBindIP = "127.0.0.1"
		cfg.HTTPPort = 8025
		cfg.HTTPSPort = 8443
		cfg.TLS.Cert = "cert-pem"
		cfg.TLS.Key = "key-pem"
		c := newMinimalContainer(cfg)

		require.True(t, cfg.TLSEnabled(), "sanity: inline cert must enable TLS")

		// ACT
		got := c.getMultiplexerAddress()

		// ASSERT
		assert.Equal(t, "127.0.0.1:8443", got, "TLS-enabled mux must bind the HTTPS port")
	})

	t.Run("uses_http_port_when_tls_disabled", func(t *testing.T) {
		// ARRANGE
		cfg := &config.Config{}
		cfg.HTTPBindIP = "127.0.0.1"
		cfg.HTTPPort = 8025
		cfg.HTTPSPort = 8443
		c := newMinimalContainer(cfg)

		require.False(t, cfg.TLSEnabled(), "sanity: no cert source must disable TLS")

		// ACT
		got := c.getMultiplexerAddress()

		// ASSERT
		assert.Equal(t, "127.0.0.1:8025", got, "TLS-disabled mux must bind the HTTP port")
	})
}

func TestNewContainerAndSetContext(t *testing.T) {
	t.Run("new_container_retains_config_and_no_context", func(t *testing.T) {
		// ARRANGE
		cfg := &config.Config{}

		// ACT
		c := NewContainer(cfg)

		// ASSERT
		require.NotNil(t, c)
		assert.Same(t, cfg, c.Config())
		assert.Nil(t, c.context, "a fresh container must not carry a context until SetContext")
	})

	t.Run("set_context_stores_context_and_cancel", func(t *testing.T) {
		// ARRANGE
		c := NewContainer(&config.Config{})
		ctx, cancel := context.WithCancel(context.Background())

		// ACT
		c.SetContext(ctx, cancel)

		// ASSERT
		assert.Equal(t, ctx, c.context, "SetContext must store the provided context")
		require.NotNil(t, c.cancel, "SetContext must store the cancel function")

		c.cancel()
		assert.ErrorIs(t, ctx.Err(), context.Canceled, "the stored cancel must cancel the context")
	})
}
