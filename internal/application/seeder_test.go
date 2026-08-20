package application

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gameap/gameap/internal/config"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/migrations"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/testcontainer"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// setupSeederContainer wires a Container backed by a fresh in-memory SQLite database, a
// per-test temp filesystem root, and a stubbed Global API endpoint. The Global API is
// pointed at a server that always returns 500 so that seedGamesAndMods exercises the
// fallback path with the embedded games.json.
func setupSeederContainer(t *testing.T) *Container {
	t.Helper()

	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	cfg := &config.Config{
		DatabaseDriver: "sqlite",
		DatabaseURL:    dsn,
	}
	cfg.RBAC.CacheTTL = "1s"
	cfg.Files.Driver = filesDriverLocal
	cfg.Files.Local.BasePath = t.TempDir()
	cfg.Cache.Driver = cacheDriverMemory

	stubAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(stubAPI.Close)
	cfg.GlobalAPI.URL = stubAPI.URL

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

func TestSeedClientCertificates(t *testing.T) {
	t.Parallel()

	t.Run("creates_client_certificate_when_none_exist", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		c := setupSeederContainer(t)
		ctx := context.Background()

		// ACT
		err := seedClientCertificates(ctx, c)

		// ASSERT
		require.NoError(t, err)

		certs, err := c.ClientCertificateRepository().FindAll(ctx, nil, nil)
		require.NoError(t, err)
		require.Len(t, certs, 1, "exactly one certificate should be seeded")

		got := certs[0]
		assert.NotEmpty(t, got.Fingerprint, "fingerprint must be populated")
		assert.True(t, strings.HasPrefix(got.Certificate, "certs/client/"),
			"certificate path should be under certs/client/")
		assert.True(t, strings.HasSuffix(got.Certificate, ".crt"),
			"certificate path should end with .crt")
		assert.True(t, strings.HasSuffix(got.PrivateKey, ".key"),
			"private key path should end with .key")
		assert.True(t, got.Expires.After(time.Now()),
			"expiration must be in the future")
	})

	t.Run("is_idempotent_when_certificate_already_exists", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		c := setupSeederContainer(t)
		ctx := context.Background()
		require.NoError(t, seedClientCertificates(ctx, c))

		before, err := c.ClientCertificateRepository().FindAll(ctx, nil, nil)
		require.NoError(t, err)
		require.Len(t, before, 1)

		// ACT
		err = seedClientCertificates(ctx, c)

		// ASSERT
		require.NoError(t, err)

		after, err := c.ClientCertificateRepository().FindAll(ctx, nil, nil)
		require.NoError(t, err)
		require.Len(t, after, 1, "second seed must not create another certificate")
		assert.Equal(t, before[0].Fingerprint, after[0].Fingerprint,
			"existing certificate must remain unchanged")
	})
}

func TestSeedRoles(t *testing.T) {
	t.Parallel()

	t.Run("creates_admin_and_user_roles_when_empty", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		c := setupSeederContainer(t)
		ctx := context.Background()

		// ACT
		err := seedRoles(ctx, c)

		// ASSERT
		require.NoError(t, err)

		roles, err := c.RBACRepository().GetRoles(ctx)
		require.NoError(t, err)
		require.Len(t, roles, 2, "admin and user roles must be created")

		names := map[string]bool{}
		for _, r := range roles {
			names[r.Name] = true
		}

		assert.True(t, names["admin"], "admin role must exist")
		assert.True(t, names["user"], "user role must exist")
	})

	t.Run("admin_role_has_admin_permissions_ability", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		c := setupSeederContainer(t)
		ctx := context.Background()

		// ACT
		require.NoError(t, seedRoles(ctx, c))

		// ASSERT
		roles, err := c.RBACRepository().GetRoles(ctx)
		require.NoError(t, err)

		var adminID uint
		for _, r := range roles {
			if r.Name == "admin" {
				adminID = r.ID
			}
		}
		require.NotZero(t, adminID, "admin role must be persisted with an ID")

		perms, err := c.RBACRepository().GetPermissions(ctx, adminID, domain.EntityTypeRole)
		require.NoError(t, err)
		require.Len(t, perms, 1, "admin role must have exactly one ability assigned")

		require.NotNil(t, perms[0].Ability, "ability must be hydrated")
		assert.Equal(t, domain.AbilityNameAdminRolesPermissions, perms[0].Ability.Name)
		assert.False(t, perms[0].Forbidden, "admin permission must not be forbidden")
	})

	t.Run("is_idempotent_when_roles_already_exist", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		c := setupSeederContainer(t)
		ctx := context.Background()
		require.NoError(t, seedRoles(ctx, c))

		before, err := c.RBACRepository().GetRoles(ctx)
		require.NoError(t, err)
		require.Len(t, before, 2)

		// ACT
		err = seedRoles(ctx, c)

		// ASSERT
		require.NoError(t, err)

		after, err := c.RBACRepository().GetRoles(ctx)
		require.NoError(t, err)
		require.Len(t, after, 2, "second seed must not duplicate roles")
	})
}

func TestSeedUsers(t *testing.T) {
	t.Run("creates_admin_user_with_env_password", func(t *testing.T) {
		// ARRANGE
		c := setupSeederContainer(t)
		ctx := context.Background()
		require.NoError(t, seedRoles(ctx, c))

		t.Setenv("ADMIN_LOGIN", "")
		t.Setenv("ADMIN_EMAIL", "")
		t.Setenv("ADMIN_PASSWORD", "supersecret123")

		// ACT
		err := seedUsers(ctx, c)

		// ASSERT
		require.NoError(t, err)

		users, err := c.UserRepository().FindAll(ctx, nil, nil)
		require.NoError(t, err)
		require.Len(t, users, 1, "exactly one admin user must be created")

		got := users[0]
		assert.Equal(t, "admin", got.Login)
		assert.Equal(t, "admin@localhost", got.Email)
		require.NotNil(t, got.Name)
		assert.Equal(t, "Admin", *got.Name)

		assert.NotEqual(t, "supersecret123", got.Password, "password must be hashed, not stored as plaintext")
		_, vErr := auth.VerifyPassword(got.Password, "supersecret123")
		assert.NoError(t, vErr, "hash must match the configured password")
	})

	t.Run("respects_admin_login_and_email_env_overrides", func(t *testing.T) {
		// ARRANGE
		c := setupSeederContainer(t)
		ctx := context.Background()
		require.NoError(t, seedRoles(ctx, c))

		t.Setenv("ADMIN_LOGIN", "root")
		t.Setenv("ADMIN_EMAIL", "root@example.com")
		t.Setenv("ADMIN_PASSWORD", "anotherpass")

		// ACT
		err := seedUsers(ctx, c)

		// ASSERT
		require.NoError(t, err)

		users, err := c.UserRepository().FindAll(ctx, nil, nil)
		require.NoError(t, err)
		require.Len(t, users, 1)

		assert.Equal(t, "root", users[0].Login)
		assert.Equal(t, "root@example.com", users[0].Email)
	})

	t.Run("generates_random_password_when_env_missing", func(t *testing.T) {
		// ARRANGE
		c := setupSeederContainer(t)
		ctx := context.Background()
		require.NoError(t, seedRoles(ctx, c))

		t.Setenv("ADMIN_LOGIN", "")
		t.Setenv("ADMIN_EMAIL", "")
		t.Setenv("ADMIN_PASSWORD", "")

		// ACT
		err := seedUsers(ctx, c)

		// ASSERT
		require.NoError(t, err)

		users, err := c.UserRepository().FindAll(ctx, nil, nil)
		require.NoError(t, err)
		require.Len(t, users, 1)

		assert.NotEmpty(t, users[0].Password, "a random password hash must still be persisted")
		_, vErr := auth.VerifyPassword(users[0].Password, "")
		assert.Error(t, vErr, "empty password must not authenticate against the random hash")
	})

	t.Run("assigns_admin_role_to_seeded_user", func(t *testing.T) {
		// ARRANGE
		c := setupSeederContainer(t)
		ctx := context.Background()
		require.NoError(t, seedRoles(ctx, c))

		t.Setenv("ADMIN_PASSWORD", "rolecheck123")

		// ACT
		require.NoError(t, seedUsers(ctx, c))

		// ASSERT
		users, err := c.UserRepository().FindAll(ctx, nil, nil)
		require.NoError(t, err)
		require.Len(t, users, 1)

		userID := users[0].ID

		assignedRoles, err := c.RBACRepository().GetRolesForEntity(ctx, userID, domain.EntityTypeUser)
		require.NoError(t, err)
		require.Len(t, assignedRoles, 1, "the new user must have exactly the admin role")
		assert.Equal(t, "admin", assignedRoles[0].Name)
	})

	t.Run("is_idempotent_when_user_already_exists", func(t *testing.T) {
		// ARRANGE
		c := setupSeederContainer(t)
		ctx := context.Background()
		require.NoError(t, seedRoles(ctx, c))

		t.Setenv("ADMIN_PASSWORD", "firstpass")
		require.NoError(t, seedUsers(ctx, c))

		before, err := c.UserRepository().FindAll(ctx, nil, nil)
		require.NoError(t, err)
		require.Len(t, before, 1)

		// ACT — re-seed with a different password
		t.Setenv("ADMIN_PASSWORD", "secondpass")
		err = seedUsers(ctx, c)

		// ASSERT
		require.NoError(t, err)

		after, err := c.UserRepository().FindAll(ctx, nil, nil)
		require.NoError(t, err)
		require.Len(t, after, 1, "second seed must not create another user")
		assert.Equal(t, before[0].Password, after[0].Password,
			"existing password hash must not be overwritten on re-seed")
	})
}

func TestSeedGamesAndMods(t *testing.T) {
	t.Parallel()

	t.Run("populates_games_from_fallback_when_global_api_unreachable", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		c := setupSeederContainer(t)
		ctx := context.Background()

		// ACT
		err := seedGamesAndMods(ctx, c)

		// ASSERT
		require.NoError(t, err)

		games, err := c.GameRepository().FindAll(ctx, nil, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, games, "fallback should seed at least one game")
	})

	t.Run("is_idempotent_when_games_already_exist", func(t *testing.T) {
		t.Parallel()

		// ARRANGE
		c := setupSeederContainer(t)
		ctx := context.Background()
		require.NoError(t, seedGamesAndMods(ctx, c))

		before, err := c.GameRepository().FindAll(ctx, nil, nil)
		require.NoError(t, err)
		require.NotEmpty(t, before)

		// ACT
		err = seedGamesAndMods(ctx, c)

		// ASSERT
		require.NoError(t, err)

		after, err := c.GameRepository().FindAll(ctx, nil, nil)
		require.NoError(t, err)
		assert.Len(t, after, len(before), "second seed must not duplicate games")
	})
}

func TestSeed(t *testing.T) {
	t.Run("end_to_end_seeds_certificates_roles_users_and_games", func(t *testing.T) {
		// ARRANGE
		c := setupSeederContainer(t)
		ctx := context.Background()

		t.Setenv("ADMIN_PASSWORD", "endtoend123")

		// ACT
		err := seed(ctx, c)

		// ASSERT
		require.NoError(t, err)

		certs, err := c.ClientCertificateRepository().FindAll(ctx, nil, nil)
		require.NoError(t, err)
		assert.Len(t, certs, 1, "client certificate must be seeded")

		roles, err := c.RBACRepository().GetRoles(ctx)
		require.NoError(t, err)
		assert.Len(t, roles, 2, "admin and user roles must be seeded")

		users, err := c.UserRepository().FindAll(ctx, nil, nil)
		require.NoError(t, err)
		require.Len(t, users, 1)
		_, vErr := auth.VerifyPassword(users[0].Password, "endtoend123")
		assert.NoError(t, vErr, "end-to-end admin password must round-trip")

		games, err := c.GameRepository().FindAll(ctx, nil, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, games, "fallback games must be seeded")
	})

	t.Run("is_safe_to_run_twice", func(t *testing.T) {
		// ARRANGE
		c := setupSeederContainer(t)
		ctx := context.Background()
		t.Setenv("ADMIN_PASSWORD", "twice123")

		require.NoError(t, seed(ctx, c))

		certsBefore, err := c.ClientCertificateRepository().FindAll(ctx, nil, nil)
		require.NoError(t, err)
		usersBefore, err := c.UserRepository().FindAll(ctx, nil, nil)
		require.NoError(t, err)
		rolesBefore, err := c.RBACRepository().GetRoles(ctx)
		require.NoError(t, err)
		gamesBefore, err := c.GameRepository().FindAll(ctx, nil, nil)
		require.NoError(t, err)

		// ACT
		err = seed(ctx, c)

		// ASSERT
		require.NoError(t, err)

		certsAfter, err := c.ClientCertificateRepository().FindAll(ctx, nil, nil)
		require.NoError(t, err)
		usersAfter, err := c.UserRepository().FindAll(ctx, nil, nil)
		require.NoError(t, err)
		rolesAfter, err := c.RBACRepository().GetRoles(ctx)
		require.NoError(t, err)
		gamesAfter, err := c.GameRepository().FindAll(ctx, nil, nil)
		require.NoError(t, err)

		assert.Len(t, certsAfter, len(certsBefore))
		assert.Len(t, usersAfter, len(usersBefore))
		assert.Len(t, rolesAfter, len(rolesBefore))
		assert.Len(t, gamesAfter, len(gamesBefore))
	})
}
