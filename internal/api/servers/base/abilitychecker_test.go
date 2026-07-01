package base_test

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// auditCapture is a concurrency-safe audit.Logger that records every event
// the ability checker emits (mirrors router_security_auditlog_test.go).
type auditCapture struct {
	mu     sync.Mutex
	events []audit.Event
}

func (a *auditCapture) Record(_ context.Context, e audit.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
}

func (a *auditCapture) snapshot() []audit.Event {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]audit.Event(nil), a.events...)
}

func findEvent(events []audit.Event, t audit.EventType) (audit.Event, bool) {
	for _, e := range events {
		if e.Type == t {
			return e, true
		}
	}

	return audit.Event{}, false
}

func countEvents(events []audit.Event, t audit.EventType) int {
	n := 0
	for _, e := range events {
		if e.Type == t {
			n++
		}
	}

	return n
}

func extraString(e audit.Event, key string) (string, bool) {
	for _, a := range e.Extra {
		if a.Key == key {
			return a.Value.String(), true
		}
	}

	return "", false
}

func setupRBAC(t *testing.T) (*rbac.RBAC, *inmemory.RBACRepository) {
	t.Helper()

	repo := inmemory.NewRBACRepository()
	tm := services.NewNilTransactionManager()
	rbacService := rbac.NewRBAC(tm, repo, 1*time.Minute)

	return rbacService, repo
}

func createAdminRole(t *testing.T, repo *inmemory.RBACRepository) domain.Role {
	t.Helper()

	role := domain.Role{
		Name:  "admin",
		Title: new("Administrator"),
	}

	err := repo.SaveRole(context.Background(), &role)
	require.NoError(t, err)

	ability := domain.Ability{
		Name: domain.AbilityNameAdminRolesPermissions,
	}
	err = repo.SaveAbility(context.Background(), &ability)
	require.NoError(t, err)

	err = repo.Allow(
		context.Background(),
		role.ID,
		domain.EntityTypeRole,
		[]domain.Ability{ability},
	)
	require.NoError(t, err)

	return role
}

func assignRoleToUser(t *testing.T, repo *inmemory.RBACRepository, userID uint, role domain.Role) {
	t.Helper()

	err := repo.AssignRolesForEntity(
		context.Background(),
		userID,
		domain.EntityTypeUser,
		[]domain.RestrictedRole{domain.NewRestrictedRoleFromRole(role)},
	)
	require.NoError(t, err)
}

func allowUserAbilityForServer(
	t *testing.T,
	repo *inmemory.RBACRepository,
	userID uint,
	serverID uint,
	abilityName domain.AbilityName,
) {
	t.Helper()

	ability := domain.CreateAbilityForEntity(abilityName, serverID, domain.EntityTypeServer)
	err := repo.SaveAbility(context.Background(), &ability)
	require.NoError(t, err)

	err = repo.Allow(
		context.Background(),
		userID,
		domain.EntityTypeUser,
		[]domain.Ability{ability},
	)
	require.NoError(t, err)
}

func TestAbilityChecker_Check(t *testing.T) {
	tests := []struct {
		name          string
		userID        uint
		serverID      uint
		abilities     []domain.AbilityName
		setup         func(t *testing.T, rbacService *rbac.RBAC, repo *inmemory.RBACRepository)
		expected      bool
		expectedError string
	}{
		{
			name:      "admin_user_has_access",
			userID:    1,
			serverID:  10,
			abilities: []domain.AbilityName{domain.AbilityNameGameServerStart},
			setup: func(t *testing.T, _ *rbac.RBAC, repo *inmemory.RBACRepository) {
				t.Helper()
				adminRole := createAdminRole(t, repo)
				assignRoleToUser(t, repo, 1, adminRole)
			},
			expected:      true,
			expectedError: "",
		},
		{
			name:      "non_admin_user_with_single_ability",
			userID:    2,
			serverID:  20,
			abilities: []domain.AbilityName{domain.AbilityNameGameServerStop},
			setup: func(t *testing.T, _ *rbac.RBAC, repo *inmemory.RBACRepository) {
				t.Helper()
				allowUserAbilityForServer(t, repo, 2, 20, domain.AbilityNameGameServerStop)
			},
			expected:      true,
			expectedError: "",
		},
		{
			name:      "non_admin_user_with_multiple_abilities",
			userID:    3,
			serverID:  30,
			abilities: []domain.AbilityName{domain.AbilityNameGameServerStart, domain.AbilityNameGameServerStop},
			setup: func(t *testing.T, _ *rbac.RBAC, repo *inmemory.RBACRepository) {
				t.Helper()
				allowUserAbilityForServer(t, repo, 3, 30, domain.AbilityNameGameServerStart)
				allowUserAbilityForServer(t, repo, 3, 30, domain.AbilityNameGameServerStop)
			},
			expected:      true,
			expectedError: "",
		},
		{
			name:      "non_admin_user_without_ability",
			userID:    4,
			serverID:  40,
			abilities: []domain.AbilityName{domain.AbilityNameGameServerFiles},
			setup: func(t *testing.T, _ *rbac.RBAC, _ *inmemory.RBACRepository) {
				t.Helper()
			},
			expected:      false,
			expectedError: "",
		},
		{
			name:      "non_admin_user_with_partial_abilities",
			userID:    5,
			serverID:  50,
			abilities: []domain.AbilityName{domain.AbilityNameGameServerStart, domain.AbilityNameGameServerStop},
			setup: func(t *testing.T, _ *rbac.RBAC, repo *inmemory.RBACRepository) {
				t.Helper()
				allowUserAbilityForServer(t, repo, 5, 50, domain.AbilityNameGameServerStart)
			},
			expected:      false,
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rbacService, repo := setupRBAC(t)
			defer rbacService.Close()

			if tt.setup != nil {
				tt.setup(t, rbacService, repo)
			}

			checker := serversbase.NewAbilityChecker(rbacService)
			result, err := checker.Check(context.Background(), tt.userID, tt.serverID, tt.abilities)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestAbilityChecker_CheckOrError(t *testing.T) {
	tests := []struct {
		name               string
		userID             uint
		serverID           uint
		abilities          []domain.AbilityName
		setup              func(t *testing.T, rbacService *rbac.RBAC, repo *inmemory.RBACRepository)
		expectedError      string
		expectedStatusCode int
	}{
		{
			name:      "user_has_single_ability",
			userID:    1,
			serverID:  10,
			abilities: []domain.AbilityName{domain.AbilityNameGameServerStart},
			setup: func(t *testing.T, _ *rbac.RBAC, repo *inmemory.RBACRepository) {
				t.Helper()
				adminRole := createAdminRole(t, repo)
				assignRoleToUser(t, repo, 1, adminRole)
			},
			expectedError:      "",
			expectedStatusCode: 0,
		},
		{
			name:      "user_has_multiple_abilities",
			userID:    2,
			serverID:  20,
			abilities: []domain.AbilityName{domain.AbilityNameGameServerStart, domain.AbilityNameGameServerStop},
			setup: func(t *testing.T, _ *rbac.RBAC, repo *inmemory.RBACRepository) {
				t.Helper()
				allowUserAbilityForServer(t, repo, 2, 20, domain.AbilityNameGameServerStart)
				allowUserAbilityForServer(t, repo, 2, 20, domain.AbilityNameGameServerStop)
			},
			expectedError:      "",
			expectedStatusCode: 0,
		},
		{
			name:      "user_does_not_have_ability",
			userID:    3,
			serverID:  30,
			abilities: []domain.AbilityName{domain.AbilityNameGameServerStop},
			setup: func(t *testing.T, _ *rbac.RBAC, _ *inmemory.RBACRepository) {
				t.Helper()
			},
			expectedError:      "user does not have required permissions",
			expectedStatusCode: http.StatusForbidden,
		},
		{
			name:      "user_has_partial_abilities",
			userID:    4,
			serverID:  40,
			abilities: []domain.AbilityName{domain.AbilityNameGameServerStart, domain.AbilityNameGameServerStop},
			setup: func(t *testing.T, _ *rbac.RBAC, repo *inmemory.RBACRepository) {
				t.Helper()
				allowUserAbilityForServer(t, repo, 4, 40, domain.AbilityNameGameServerStart)
			},
			expectedError:      "user does not have required permissions",
			expectedStatusCode: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rbacService, repo := setupRBAC(t)
			defer rbacService.Close()

			if tt.setup != nil {
				tt.setup(t, rbacService, repo)
			}

			checker := serversbase.NewAbilityChecker(rbacService)
			err := checker.CheckOrError(context.Background(), tt.userID, tt.serverID, tt.abilities)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)

				if tt.expectedStatusCode != 0 {
					type httpStatusError interface {
						HTTPStatus() int
					}
					httpErr, ok := err.(httpStatusError)
					require.True(t, ok, "error should have HTTPStatus method")
					assert.Equal(t, tt.expectedStatusCode, httpErr.HTTPStatus())
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAbilityChecker_CheckAny(t *testing.T) {
	tests := []struct {
		name          string
		userID        uint
		serverID      uint
		abilities     []domain.AbilityName
		setup         func(t *testing.T, rbacService *rbac.RBAC, repo *inmemory.RBACRepository)
		expected      bool
		expectedError string
	}{
		{
			name:      "admin_user_has_access",
			userID:    1,
			serverID:  10,
			abilities: []domain.AbilityName{domain.AbilityNameGameServerRconConsole, domain.AbilityNameGameServerRconPlayers},
			setup: func(t *testing.T, _ *rbac.RBAC, repo *inmemory.RBACRepository) {
				t.Helper()
				adminRole := createAdminRole(t, repo)
				assignRoleToUser(t, repo, 1, adminRole)
			},
			expected: true,
		},
		{
			name:      "non_admin_user_with_first_of_two",
			userID:    2,
			serverID:  20,
			abilities: []domain.AbilityName{domain.AbilityNameGameServerRconConsole, domain.AbilityNameGameServerRconPlayers},
			setup: func(t *testing.T, _ *rbac.RBAC, repo *inmemory.RBACRepository) {
				t.Helper()
				allowUserAbilityForServer(t, repo, 2, 20, domain.AbilityNameGameServerRconConsole)
			},
			expected: true,
		},
		{
			name:      "non_admin_user_with_second_of_two",
			userID:    3,
			serverID:  30,
			abilities: []domain.AbilityName{domain.AbilityNameGameServerRconConsole, domain.AbilityNameGameServerRconPlayers},
			setup: func(t *testing.T, _ *rbac.RBAC, repo *inmemory.RBACRepository) {
				t.Helper()
				allowUserAbilityForServer(t, repo, 3, 30, domain.AbilityNameGameServerRconPlayers)
			},
			expected: true,
		},
		{
			name:      "non_admin_user_with_none_of_two",
			userID:    4,
			serverID:  40,
			abilities: []domain.AbilityName{domain.AbilityNameGameServerRconConsole, domain.AbilityNameGameServerRconPlayers},
			setup: func(t *testing.T, _ *rbac.RBAC, _ *inmemory.RBACRepository) {
				t.Helper()
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rbacService, repo := setupRBAC(t)
			defer rbacService.Close()

			if tt.setup != nil {
				tt.setup(t, rbacService, repo)
			}

			checker := serversbase.NewAbilityChecker(rbacService)
			result, err := checker.CheckAny(context.Background(), tt.userID, tt.serverID, tt.abilities)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestAbilityChecker_CheckAnyOrError(t *testing.T) {
	tests := []struct {
		name               string
		userID             uint
		serverID           uint
		abilities          []domain.AbilityName
		setup              func(t *testing.T, rbacService *rbac.RBAC, repo *inmemory.RBACRepository)
		expectedError      string
		expectedStatusCode int
	}{
		{
			name:      "admin_user_is_allowed",
			userID:    1,
			serverID:  10,
			abilities: []domain.AbilityName{domain.AbilityNameGameServerRconConsole, domain.AbilityNameGameServerRconPlayers},
			setup: func(t *testing.T, _ *rbac.RBAC, repo *inmemory.RBACRepository) {
				t.Helper()
				adminRole := createAdminRole(t, repo)
				assignRoleToUser(t, repo, 1, adminRole)
			},
		},
		{
			name:      "user_has_one_of_the_abilities",
			userID:    2,
			serverID:  20,
			abilities: []domain.AbilityName{domain.AbilityNameGameServerRconConsole, domain.AbilityNameGameServerRconPlayers},
			setup: func(t *testing.T, _ *rbac.RBAC, repo *inmemory.RBACRepository) {
				t.Helper()
				allowUserAbilityForServer(t, repo, 2, 20, domain.AbilityNameGameServerRconPlayers)
			},
		},
		{
			name:      "user_has_none_of_the_abilities",
			userID:    3,
			serverID:  30,
			abilities: []domain.AbilityName{domain.AbilityNameGameServerRconConsole, domain.AbilityNameGameServerRconPlayers},
			setup: func(t *testing.T, _ *rbac.RBAC, _ *inmemory.RBACRepository) {
				t.Helper()
			},
			expectedError:      "user does not have required permissions",
			expectedStatusCode: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rbacService, repo := setupRBAC(t)
			defer rbacService.Close()

			if tt.setup != nil {
				tt.setup(t, rbacService, repo)
			}

			checker := serversbase.NewAbilityChecker(rbacService)
			err := checker.CheckAnyOrError(context.Background(), tt.userID, tt.serverID, tt.abilities)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)

				if tt.expectedStatusCode != 0 {
					type httpStatusError interface {
						HTTPStatus() int
					}
					httpErr, ok := err.(httpStatusError)
					require.True(t, ok, "error should have HTTPStatus method")
					assert.Equal(t, tt.expectedStatusCode, httpErr.HTTPStatus())
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Security audit-trail tests.
//
// OWASP API Security Top 10:2023:
//   - API1:2023 Broken Object Level Authorization — a per-server ability
//     check that denies access must leave an access.denied audit event scoped
//     to the exact server (resource_type "server", resource_id = serverID) so
//     an operator can detect horizontal-privilege probing (OWASP ASVS §4.1.5).
//   - API5:2023 Broken Function Level Authorization — the missing function
//     abilities must be recorded (Extra.required_abilities) for forensics.
//
// Reference: https://owasp.org/API-Security/editions/2023/
// ---------------------------------------------------------------------------

// TestAbilityChecker_Audit_DenialEmitsScopedEvent covers OWASP API1:2023 /
// API5:2023. When CheckOrError denies a user it must emit exactly one
// access.denied event with outcome denied, category authorization, the
// targeted server as the resource, the stable missing_ability reason, and the
// required abilities listed in Extra. The logger is injected per-checker via
// WithAuditLogger so the test does not depend on the process-global default.
func TestAbilityChecker_Audit_DenialEmitsScopedEvent(t *testing.T) {
	tests := []struct {
		name           string
		userID         uint
		serverID       uint
		abilities      []domain.AbilityName
		setup          func(t *testing.T, repo *inmemory.RBACRepository)
		wantResourceID string
		wantAbilities  string
	}{
		{
			name:      "no_ability_at_all_is_denied_and_scoped_to_server",
			userID:    3,
			serverID:  30,
			abilities: []domain.AbilityName{domain.AbilityNameGameServerStop},
			setup: func(t *testing.T, _ *inmemory.RBACRepository) {
				t.Helper()
			},
			wantResourceID: "30",
			wantAbilities:  string(domain.AbilityNameGameServerStop),
		},
		{
			name:      "partial_abilities_still_denied_and_lists_all_required",
			userID:    4,
			serverID:  40,
			abilities: []domain.AbilityName{domain.AbilityNameGameServerStart, domain.AbilityNameGameServerStop},
			setup: func(t *testing.T, repo *inmemory.RBACRepository) {
				t.Helper()
				allowUserAbilityForServer(t, repo, 4, 40, domain.AbilityNameGameServerStart)
			},
			wantResourceID: "40",
			wantAbilities: string(domain.AbilityNameGameServerStart) + "," +
				string(domain.AbilityNameGameServerStop),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ARRANGE
			rbacService, repo := setupRBAC(t)
			defer rbacService.Close()
			tt.setup(t, repo)

			recorder := &auditCapture{}
			checker := serversbase.NewAbilityChecker(
				rbacService, serversbase.WithAuditLogger(recorder),
			)

			// ACT
			err := checker.CheckOrError(context.Background(), tt.userID, tt.serverID, tt.abilities)

			// ASSERT
			require.Error(t, err, "a user without the required abilities must be denied")
			assert.Contains(t, err.Error(), "user does not have required permissions")

			events := recorder.snapshot()
			require.Equal(t, 1, countEvents(events, audit.EventAccessDenied),
				"exactly one access-denied event must be emitted per denial")

			ev, ok := findEvent(events, audit.EventAccessDenied)
			require.True(t, ok, "a denial must leave an access.denied audit event")
			assert.Equal(t, audit.OutcomeDenied, ev.Outcome, "an authorization refusal is a denial")
			assert.Equal(t, audit.CategoryAuthorization, ev.Category)
			assert.Equal(t, "server", ev.ResourceType,
				"a per-server ability check guards the server resource")
			assert.Equal(t, tt.wantResourceID, ev.ResourceID,
				"the denial must be scoped to the exact target server id")
			assert.Equal(t, "missing_ability", ev.Reason,
				"the stable missing_ability reason must be recorded")
			gotAbilities, hasAbilities := extraString(ev, "required_abilities")
			require.True(t, hasAbilities,
				"the abilities that were required must be recorded for forensics")
			assert.Equal(t, tt.wantAbilities, gotAbilities,
				"required_abilities must list every ability the check demanded")
		})
	}
}

// TestAbilityChecker_Audit_AuthorisedAccessIsNotDenied covers OWASP
// API1:2023. A user who actually holds the required ability for the server
// must pass and must NOT leave an access.denied event (no false-positive
// denials in the audit trail).
func TestAbilityChecker_Audit_AuthorisedAccessIsNotDenied(t *testing.T) {
	// ARRANGE
	rbacService, repo := setupRBAC(t)
	defer rbacService.Close()
	allowUserAbilityForServer(t, repo, 2, 20, domain.AbilityNameGameServerStop)

	recorder := &auditCapture{}
	checker := serversbase.NewAbilityChecker(rbacService, serversbase.WithAuditLogger(recorder))

	// ACT
	err := checker.CheckOrError(
		context.Background(), 2, 20, []domain.AbilityName{domain.AbilityNameGameServerStop},
	)

	// ASSERT
	require.NoError(t, err, "a user with the required ability must be allowed")
	assert.Equal(t, 0, countEvents(recorder.snapshot(), audit.EventAccessDenied),
		"an authorised access must not produce a false-positive access-denied event")
}

// TestAbilityChecker_Audit_CheckAnyDenialEmitsScopedEvent covers OWASP
// API1:2023 / API5:2023 for the any-of check. When CheckAnyOrError denies a
// user who holds none of the acceptable abilities it must emit exactly one
// access.denied event scoped to the target server, with the missing_ability
// reason and every acceptable ability listed in Extra.required_abilities.
func TestAbilityChecker_Audit_CheckAnyDenialEmitsScopedEvent(t *testing.T) {
	// ARRANGE
	rbacService, _ := setupRBAC(t)
	defer rbacService.Close()

	recorder := &auditCapture{}
	checker := serversbase.NewAbilityChecker(rbacService, serversbase.WithAuditLogger(recorder))

	abilities := []domain.AbilityName{
		domain.AbilityNameGameServerRconConsole,
		domain.AbilityNameGameServerRconPlayers,
	}

	// ACT
	err := checker.CheckAnyOrError(context.Background(), 3, 30, abilities)

	// ASSERT
	require.Error(t, err, "a user holding none of the acceptable abilities must be denied")
	assert.Contains(t, err.Error(), "user does not have required permissions")

	events := recorder.snapshot()
	require.Equal(t, 1, countEvents(events, audit.EventAccessDenied),
		"exactly one access-denied event must be emitted per denial")

	ev, ok := findEvent(events, audit.EventAccessDenied)
	require.True(t, ok, "a denial must leave an access.denied audit event")
	assert.Equal(t, audit.OutcomeDenied, ev.Outcome)
	assert.Equal(t, audit.CategoryAuthorization, ev.Category)
	assert.Equal(t, "server", ev.ResourceType)
	assert.Equal(t, "30", ev.ResourceID, "the denial must be scoped to the exact target server id")
	assert.Equal(t, "missing_ability", ev.Reason)
	gotAbilities, hasAbilities := extraString(ev, "required_abilities")
	require.True(t, hasAbilities)
	assert.Equal(t,
		string(domain.AbilityNameGameServerRconConsole)+","+string(domain.AbilityNameGameServerRconPlayers),
		gotAbilities,
		"required_abilities must list every acceptable ability the any-of check demanded")
}
