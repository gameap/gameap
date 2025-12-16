package getserverabilities

import (
	"context"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	"github.com/gameap/gameap/internal/domain"
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
	serverFinder   *serversbase.ServerFinder
	rbac           base.RBAC
	responder      base.Responder
	pluginProvider PluginServerAbilityProvider
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	rbac base.RBAC,
	responder base.Responder,
	pluginProvider PluginServerAbilityProvider,
) *Handler {
	return &Handler{
		serverFinder:   serversbase.NewServerFinder(serverRepo, rbac),
		rbac:           rbac,
		responder:      responder,
		pluginProvider: pluginProvider,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	session := auth.SessionFromContext(ctx)
	if !session.IsAuthenticated() {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("user not authenticated"),
			http.StatusUnauthorized,
		))

		return
	}

	input := api.NewInputReader(r)

	serverID, err := input.ReadUint("server")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid server id"),
			http.StatusBadRequest,
		))

		return
	}

	server, err := h.serverFinder.FindUserServer(ctx, session.User, serverID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	abilities, err := h.buildServerAbilities(ctx, server, session.User)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to build server abilities"))

		return
	}

	response := newAbilitiesResponse(abilities)
	h.responder.Write(ctx, rw, response)
}

func (h *Handler) buildServerAbilities(
	ctx context.Context,
	server *domain.Server,
	user *domain.User,
) (map[domain.AbilityName]bool, error) {
	isAdmin, err := h.rbac.Can(ctx, user.ID, []domain.AbilityName{domain.AbilityNameAdminRolesPermissions})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to check admin permissions")
	}

	var pluginAbilities []plugin.ServerAbility
	if h.pluginProvider != nil {
		pluginAbilities = h.pluginProvider.GetAllServerAbilities()
	}

	totalAbilities := len(domain.ServersAbilities) + len(pluginAbilities)
	abilities := make(map[domain.AbilityName]bool, totalAbilities)

	for _, abilityName := range domain.ServersAbilities {
		if isAdmin {
			abilities[abilityName] = true
		} else {
			hasAbility, err := h.rbac.CanForEntity(
				ctx,
				user.ID,
				domain.EntityTypeServer,
				server.ID,
				[]domain.AbilityName{abilityName},
			)
			if err != nil {
				return nil, errors.WithMessagef(
					err,
					"failed to check ability %s for server %d",
					abilityName,
					server.ID,
				)
			}
			abilities[abilityName] = hasAbility
		}
	}

	for _, pluginAbility := range pluginAbilities {
		abilityName := domain.AbilityName(pluginAbility.Name)

		if isAdmin {
			abilities[abilityName] = true
		} else {
			hasAbility, err := h.rbac.CanForEntity(
				ctx,
				user.ID,
				domain.EntityTypeServer,
				server.ID,
				[]domain.AbilityName{abilityName},
			)
			if err != nil {
				return nil, errors.WithMessagef(
					err,
					"failed to check ability %s for server %d",
					abilityName,
					server.ID,
				)
			}
			abilities[abilityName] = hasAbility
		}
	}

	return abilities, nil
}
