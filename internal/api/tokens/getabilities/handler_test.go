package getabilities

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories/inmemory"
	"github.com/gameap/gameap/internal/services"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testUser1 = domain.User{
	ID:    1,
	Login: "testuser",
	Email: "test@example.com",
}

var testAdminUser = domain.User{
	ID:    2,
	Login: "adminuser",
	Email: "admin@example.com",
}

func TestHandler_ServeHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		setupAuth       func() context.Context
		setupRBAC       func(*inmemory.RBACRepository)
		wantStatus      int
		wantError       string
		wantGroupsCount int
		wantAdminGroup  bool
	}{
		{
			name: "successful abilities retrieval for regular user",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "testuser",
					Email: "test@example.com",
					User:  &testUser1,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRBAC: func(rbacRepo *inmemory.RBACRepository) {
				// Setup regular user role without admin permissions
				role := &domain.Role{
					ID:   1,
					Name: "user",
				}
				_ = rbacRepo.SaveRole(context.Background(), role)

				// Assign role to user
				assignedRole := &domain.AssignedRole{
					RoleID:     role.ID,
					EntityID:   1,
					EntityType: domain.EntityTypeUser,
				}
				_ = rbacRepo.SaveAssignedRole(context.Background(), assignedRole)
			},
			wantStatus:      http.StatusOK,
			wantGroupsCount: 1, // Only server group
			wantAdminGroup:  false,
		},
		{
			name: "successful abilities retrieval for admin user",
			setupAuth: func() context.Context {
				session := &auth.Session{
					Login: "adminuser",
					Email: "admin@example.com",
					User:  &testAdminUser,
				}

				return auth.ContextWithSession(context.Background(), session)
			},
			setupRBAC: func(rbacRepo *inmemory.RBACRepository) {
				// Setup admin role
				role := &domain.Role{
					ID:   1,
					Name: "admin",
				}
				_ = rbacRepo.SaveRole(context.Background(), role)

				// Setup admin ability
				ability := &domain.Ability{
					ID:   1,
					Name: domain.AbilityNameAdminRolesPermissions,
				}
				_ = rbacRepo.SaveAbility(context.Background(), ability)

				// Create permission linking role to ability
				entityType := domain.EntityTypeRole
				permission := &domain.Permission{
					AbilityID:  ability.ID,
					EntityID:   &role.ID,
					EntityType: &entityType,
					Ability:    ability,
				}
				_ = rbacRepo.SavePermission(context.Background(), permission)

				// Assign role to user
				assignedRole := &domain.AssignedRole{
					RoleID:     role.ID,
					EntityID:   2,
					EntityType: domain.EntityTypeUser,
				}
				_ = rbacRepo.SaveAssignedRole(context.Background(), assignedRole)
			},
			wantStatus:      http.StatusOK,
			wantGroupsCount: 5, // server, gdaemon-task, user, node, game
			wantAdminGroup:  true,
		},
		{
			name: "unauthenticated user",
			setupAuth: func() context.Context {
				session := &auth.Session{}

				return auth.ContextWithSession(context.Background(), session)
			},
			wantStatus: http.StatusUnauthorized,
			wantError:  "user not authenticated",
		},
		{
			name:       "no session context",
			wantStatus: http.StatusUnauthorized,
			wantError:  "user not authenticated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rbacRepo := inmemory.NewRBACRepository()
			if tt.setupRBAC != nil {
				tt.setupRBAC(rbacRepo)
			}
			rbacService := rbac.NewRBAC(services.NewNilTransactionManager(), rbacRepo, 0)
			responder := api.NewResponder()
			handler := NewHandler(rbacService, responder)

			ctx := context.Background()
			if tt.setupAuth != nil {
				ctx = tt.setupAuth()
			}

			req := httptest.NewRequest(http.MethodGet, "/api/tokens/abilities", nil)
			req = req.WithContext(ctx)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.wantStatus, rr.Code)

			if tt.wantError != "" {
				var response map[string]any
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
				assert.Equal(t, "error", response["status"])
				errorMsg, ok := response["error"].(string)
				require.True(t, ok)
				assert.Contains(t, errorMsg, tt.wantError)

				return
			}

			var abilities map[string]map[string]string
			err := json.Unmarshal(rr.Body.Bytes(), &abilities)
			require.NoError(t, err)

			assert.Len(t, abilities, tt.wantGroupsCount)

			// Check that server group exists
			serverAbilities, exists := abilities["server"]
			assert.True(t, exists, "server group should exist")
			assert.NotEmpty(t, serverAbilities, "server abilities should not be empty")

			// Check admin group presence
			if tt.wantAdminGroup {
				gdaemonAbilities, exists := abilities["gdaemon-task"]
				assert.True(t, exists, "gdaemon-task group should exist for admin")
				assert.NotEmpty(t, gdaemonAbilities, "gdaemon-task abilities should not be empty")

				// Every admin group has to survive the response builder, not just
				// the two it was originally written for: an ability that never
				// reaches this payload cannot be ticked in the panel at all.
				for _, group := range []string{"user", "node", "game"} {
					groupAbilities, exists := abilities[group]
					assert.Truef(t, exists, "%s group should exist for admin", group)
					assert.NotEmptyf(t, groupAbilities, "%s abilities should not be empty", group)
				}

				_, hasSSOAbility := abilities["user"][string(domain.PATAbilitySSOIssue)]
				assert.True(t, hasSSOAbility, "user group should offer the single sign-on ability")

				// Check that server group contains admin abilities
				_, hasCreateAbility := serverAbilities[string(domain.PATAbilityServerCreate)]
				assert.True(t, hasCreateAbility, "server group should contain create ability for admin")
			} else {
				for _, group := range []string{"gdaemon-task", "user", "node", "game"} {
					_, exists := abilities[group]
					assert.Falsef(t, exists, "%s group should not exist for regular user", group)
				}

				// Check that server group doesn't contain admin abilities
				_, hasCreateAbility := serverAbilities[string(domain.PATAbilityServerCreate)]
				assert.False(t, hasCreateAbility, "server group should not contain create ability for regular user")
			}

			// Validate response structure
			for group, groupAbilities := range abilities {
				assert.NotEmpty(t, group, "group name should not be empty")
				for abilityName, description := range groupAbilities {
					assert.NotEmpty(t, abilityName, "ability name should not be empty")
					assert.NotEmpty(t, description, "ability description should not be empty")
				}
			}
		})
	}
}
