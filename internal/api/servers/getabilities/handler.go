package getabilities

import (
	"context"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/plugin"
	"github.com/pkg/errors"
)

type PluginServerAbilityProvider interface {
	GetAllServerAbilities() []plugin.ServerAbility
}

type Handler struct {
	userRepo       repositories.UserRepository
	serverRepo     repositories.ServerRepository
	rbac           base.RBAC
	responder      base.Responder
	pluginProvider PluginServerAbilityProvider
}

func NewHandler(
	userRepo repositories.UserRepository,
	serverRepo repositories.ServerRepository,
	rbac base.RBAC,
	responder base.Responder,
	pluginProvider PluginServerAbilityProvider,
) *Handler {
	return &Handler{
		userRepo:       userRepo,
		serverRepo:     serverRepo,
		rbac:           rbac,
		responder:      responder,
		pluginProvider: pluginProvider,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	session := auth.SessionFromContext(ctx)
	if session == nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("user not authenticated"),
			http.StatusUnauthorized,
		))

		return
	}

	users, err := h.userRepo.Find(ctx, &filters.FindUser{
		Logins: []string{session.Login},
	}, nil, &filters.Pagination{
		Limit:  1,
		Offset: 0,
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find user"))

		return
	}

	if len(users) == 0 {
		h.responder.WriteError(ctx, rw, api.NewNotFoundError("user not found"))

		return
	}

	user := &users[0]

	// Check if user has admin permissions
	isAdmin, err := h.rbac.Can(ctx, user.ID, []domain.AbilityName{domain.AbilityNameAdminRolesPermissions})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check admin permissions"))

		return
	}

	// Get servers based on admin status
	var servers []domain.Server
	if isAdmin {
		servers, err = h.serverRepo.FindAll(ctx, nil, nil)
	} else {
		servers, err = h.serverRepo.FindUserServers(ctx, user.ID, nil, nil, nil)
	}

	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find servers"))

		return
	}

	// Build abilities response for each server
	abilities, err := h.buildServerAbilities(ctx, servers, user, isAdmin)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to build server abilities"))

		return
	}

	response := NewServersAbilitiesResponse(abilities)
	h.responder.Write(ctx, rw, response)
}

func (h *Handler) buildServerAbilities(
	ctx context.Context,
	servers []domain.Server,
	user *domain.User,
	isAdmin bool,
) (map[uint]map[domain.AbilityName]bool, error) {
	abilities := make(map[uint]map[domain.AbilityName]bool)

	var pluginAbilities []plugin.ServerAbility
	if h.pluginProvider != nil {
		pluginAbilities = h.pluginProvider.GetAllServerAbilities()
	}

	totalAbilities := len(domain.ServersAbilities) + len(pluginAbilities)

	for _, server := range servers {
		serverAbilities := make(map[domain.AbilityName]bool, totalAbilities)

		for _, abilityName := range domain.ServersAbilities {
			if isAdmin {
				serverAbilities[abilityName] = true
			} else {
				hasAbility, err := h.rbac.CanForEntity(
					ctx,
					user.ID,
					domain.EntityTypeServer,
					server.ID,
					[]domain.AbilityName{
						abilityName,
					},
				)
				if err != nil {
					return nil, errors.WithMessagef(err, "failed to check ability %s for server %d", abilityName, server.ID)
				}
				serverAbilities[abilityName] = hasAbility
			}
		}

		for _, pluginAbility := range pluginAbilities {
			abilityName := domain.AbilityName(pluginAbility.Name)

			if isAdmin {
				serverAbilities[abilityName] = true
			} else {
				hasAbility, err := h.rbac.CanForEntity(
					ctx,
					user.ID,
					domain.EntityTypeServer,
					server.ID,
					[]domain.AbilityName{abilityName},
				)
				if err != nil {
					return nil, errors.WithMessagef(err, "failed to check ability %s for server %d", abilityName, server.ID)
				}
				serverAbilities[abilityName] = hasAbility
			}
		}

		abilities[server.ID] = serverAbilities
	}

	return abilities, nil
}
