package getsummary

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/rbac"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	serverRepo repositories.ServerRepository
	rbac       *rbac.RBAC
	responder  base.Responder
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	rbac *rbac.RBAC,
	responder base.Responder,
) *Handler {
	return &Handler{
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

	isAdmin, err := h.rbac.Can(ctx, session.User.ID, []domain.AbilityName{
		domain.AbilityNameAdminRolesPermissions,
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check admin permissions"))

		return
	}

	var servers []domain.Server
	if isAdmin {
		servers, err = h.serverRepo.FindAll(ctx, nil, nil)
	} else {
		servers, err = h.serverRepo.FindUserServers(ctx, session.User.ID, nil, nil, nil)
	}

	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to get servers"))

		return
	}

	response := h.calculateSummary(servers)

	h.responder.Write(ctx, rw, response)
}

func (h *Handler) calculateSummary(servers []domain.Server) summaryResponse {
	total := len(servers)
	online := 0

	for i := range servers {
		if servers[i].IsOnline() {
			online++
		}
	}

	return summaryResponse{
		Total:   total,
		Online:  online,
		Offline: total - online,
	}
}
