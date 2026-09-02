package application

import (
	"context"
	"database/sql"
	"testing"

	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/migrations"
	"github.com/gameap/gameap/pkg/testcontainer"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// wiredEncryptionKey is a deterministic, high-entropy-shaped value used as the
// at-rest ENCRYPTION_KEY in wired test containers. Any non-empty value yields an
// enabled secret.Cipher (the key is SHA-256-derived), so the exact bytes are not
// load-bearing — only that the value is non-empty and 32 bytes wide.
const wiredEncryptionKey = "0123456789abcdef0123456789abcdef"

// wiredAuthSecret is a non-empty AUTH_SECRET so createAuthService /
// createTwoFactorManager do not panic in wired containers.
const wiredAuthSecret = "test-auth-secret-not-for-production"

// sqliteWiredConfig builds the config shared by every SQLite-backed test
// container: a per-test in-memory shared-cache database, in-memory cache and
// pub-sub, a per-test temp filesystem root, valid auth/encryption secrets and
// zero ports so nothing binds. The database itself is not opened here — callers
// either open cfg.DatabaseURL themselves (newWiredContainer, which needs a
// handle to run migrations against) or let the container's own createDB do it.
func sqliteWiredConfig(t *testing.T) *config.Config {
	t.Helper()

	cfg := &config.Config{
		DatabaseDriver: databaseDriverSQLite,
		DatabaseURL:    "file:" + uuid.NewString() + "?mode=memory&cache=shared",
		EncryptionKey:  wiredEncryptionKey,
		AuthSecret:     wiredAuthSecret,
	}
	cfg.RBAC.CacheTTL = "1s"
	cfg.Files.Driver = filesDriverLocal
	cfg.Files.Local.BasePath = t.TempDir()
	cfg.Cache.Driver = cacheDriverMemory
	cfg.PubSub.Driver = pubsubDriverMemory
	cfg.GRPC.ExternalHost = "grpc.example.com"
	cfg.GRPC.Port = 0
	cfg.GRPC.ExternalPort = 0
	cfg.HTTPPort = 0
	cfg.HTTPSPort = 0

	return cfg
}

// newWiredContainer builds a Container backed by a fresh in-memory SQLite
// shared-cache database with all migrations applied, in-memory cache + pub-sub,
// a per-test temp filesystem root and valid auth/encryption secrets. It is the
// shared harness for white-box tests that touch the database, repositories or
// any wired collaborator. opts allow a test to tweak the config before wiring.
//
// The database handle is injected directly, so the container's own createDB is
// never reached; container_sqlite_smoke_test.go covers that construction path.
//
// It mirrors setupSeederContainer in seeder_test.go but provides the richer
// config the factory/logic sweeps need; setupSeederContainer is left untouched.
func newWiredContainer(t *testing.T, opts ...func(*config.Config)) *Container {
	t.Helper()

	cfg := sqliteWiredConfig(t)

	db, err := sql.Open("sqlite", cfg.DatabaseURL)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	for _, opt := range opts {
		opt(cfg)
	}

	require.NoError(t, migrations.Run(context.Background(), testcontainer.NewContainer(
		testcontainer.WithDB(db),
		testcontainer.WithConfig(cfg),
	)))

	c := NewContainer(cfg)
	c.db = db
	c.context = context.Background()

	t.Cleanup(func() {
		if c.rbac != nil {
			c.rbac.Close()
		}
	})

	return c
}

// newMinimalContainer wires a Container around cfg with a background context but
// no database. It is for pure-logic methods (config accessors, driver selection,
// fallbacks) that never reach createDB.
func newMinimalContainer(cfg *config.Config) *Container {
	c := NewContainer(cfg)
	c.context = context.Background()

	return c
}
