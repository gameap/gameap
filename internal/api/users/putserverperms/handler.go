package putserverperms

import (
	"context"
	"encoding/json"
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
	GetAllServerAbilities() []plugin.PluginServerAbility
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
	if !session.IsAuthenticated() {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("user not authenticated"),
			http.StatusUnauthorized,
		))

		return
	}

	input := api.NewInputReader(r)

	userID, err := input.ReadUint("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid user id"),
			http.StatusBadRequest,
		))

		return
	}

	serverID, err := input.ReadUint("server")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid server id"),
			http.StatusBadRequest,
		))

		return
	}

	var permissionsInput UpdatePermissionsInput
	err = json.NewDecoder(r.Body).Decode(&permissionsInput)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request body"),
			http.StatusBadRequest,
		))

		return
	}

	var pluginAbilities []plugin.PluginServerAbility
	if h.pluginProvider != nil {
		pluginAbilities = h.pluginProvider.GetAllServerAbilities()
	}

	err = permissionsInput.ValidateWithPluginAbilities(pluginAbilities)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid input"),
			http.StatusBadRequest,
		))

		return
	}

	users, err := h.userRepo.Find(ctx, &filters.FindUser{IDs: []uint{userID}}, nil, &filters.Pagination{Limit: 1})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find user"))

		return
	}

	if len(users) == 0 {
		h.responder.WriteError(ctx, rw, api.NewNotFoundError("user not found"))

		return
	}

	serverExists, err := h.serverRepo.Exists(ctx, &filters.FindServer{IDs: []uint{serverID}})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check server existence"))

		return
	}
	if !serverExists {
		h.responder.WriteError(ctx, rw, api.NewNotFoundError("server not found"))

		return
	}

	err = h.updatePermissions(ctx, userID, serverID, permissionsInput)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to update permissions"))

		return
	}

	permissions, err := h.buildPermissions(ctx, &users[0], serverID, pluginAbilities)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to build permissions"))

		return
	}

	h.responder.Write(ctx, rw, permissions)
}

func (h *Handler) updatePermissions(
	ctx context.Context,
	userID uint,
	serverID uint,
	permissionsInput UpdatePermissionsInput,
) error {
	allowAbilities, revokeAbilities := permissionsInput.ToAbilities()

	if len(allowAbilities) > 0 {
		err := h.rbac.AllowUserAbilitiesForEntity(
			ctx, userID, serverID, domain.EntityTypeServer, allowAbilities,
		)
		if err != nil {
			return errors.WithMessage(err, "failed to allow permissions")
		}
	}

	if len(revokeAbilities) > 0 {
		err := h.rbac.RevokeOrForbidUserAbilitiesForEntity(
			ctx, userID, serverID, domain.EntityTypeServer, revokeAbilities,
		)
		if err != nil {
			return errors.WithMessage(err, "failed to revoke permissions")
		}
	}

	return nil
}

func (h *Handler) buildPermissions(
	ctx context.Context,
	user *domain.User,
	serverID uint,
	pluginAbilities []plugin.PluginServerAbility,
) ([]PermissionResponse, error) {
	isAdmin, err := h.rbac.Can(ctx, user.ID, []domain.AbilityName{domain.AbilityNameAdminRolesPermissions})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to check admin permission")
	}

	totalAbilities := len(domain.ServersAbilities) + len(pluginAbilities)
	permissions := make([]PermissionResponse, 0, totalAbilities)

	for _, abilityName := range domain.ServersAbilities {
		if isAdmin {
			permissions = append(permissions, NewPermissionResponse(abilityName, true))

			continue
		}

		can, err := h.rbac.CanForEntity(
			ctx,
			user.ID,
			domain.EntityTypeServer,
			serverID,
			[]domain.AbilityName{abilityName},
		)
		if err != nil {
			return nil, errors.WithMessagef(err, "failed to check permission %s", abilityName)
		}

		permissions = append(permissions, NewPermissionResponse(abilityName, can))
	}

	for _, pluginAbility := range pluginAbilities {
		abilityName := domain.AbilityName(pluginAbility.Name)

		if isAdmin {
			permissions = append(permissions, NewPluginPermissionResponse(pluginAbility, true))

			continue
		}

		can, err := h.rbac.CanForEntity(
			ctx,
			user.ID,
			domain.EntityTypeServer,
			serverID,
			[]domain.AbilityName{abilityName},
		)
		if err != nil {
			return nil, errors.WithMessagef(err, "failed to check permission %s", abilityName)
		}

		permissions = append(permissions, NewPluginPermissionResponse(pluginAbility, can))
	}

	return permissions, nil
}
