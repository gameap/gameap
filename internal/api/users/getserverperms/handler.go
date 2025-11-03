package getserverperms

import (
	"context"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	userRepo   repositories.UserRepository
	serverRepo repositories.ServerRepository
	rbac       base.RBAC
	responder  base.Responder
}

func NewHandler(
	userRepo repositories.UserRepository,
	serverRepo repositories.ServerRepository,
	rbac base.RBAC,
	responder base.Responder,
) *Handler {
	return &Handler{
		userRepo:   userRepo,
		serverRepo: serverRepo,
		rbac:       rbac,
		responder:  responder,
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

	users, err := h.userRepo.Find(ctx, &filters.FindUser{IDs: []uint{userID}}, nil, &filters.Pagination{Limit: 1})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find user"))

		return
	}

	if len(users) == 0 {
		h.responder.WriteError(ctx, rw, api.NewNotFoundError("user not found"))

		return
	}

	user := &users[0]

	serverExists, err := h.serverRepo.Exists(ctx, &filters.FindServer{IDs: []uint{serverID}})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check server existence"))

		return
	}
	if !serverExists {
		h.responder.WriteError(ctx, rw, api.NewNotFoundError("server not found"))

		return
	}

	permissions, err := h.buildPermissions(ctx, user, serverID)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to build permissions"))

		return
	}

	h.responder.Write(ctx, rw, permissions)
}

func (h *Handler) buildPermissions(
	ctx context.Context,
	user *domain.User,
	serverID uint,
) ([]PermissionResponse, error) {
	isAdmin, err := h.rbac.Can(ctx, user.ID, []domain.AbilityName{domain.AbilityNameAdminRolesPermissions})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to check admin permission")
	}

	permissions := make([]PermissionResponse, 0, len(domain.ServersAbilities))

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

	return permissions, nil
}
