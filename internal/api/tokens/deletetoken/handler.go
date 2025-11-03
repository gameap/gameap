package deletetoken

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	tokensRepo repositories.PersonalAccessTokenRepository
	responder  base.Responder
}

func NewHandler(
	tokensRepo repositories.PersonalAccessTokenRepository,
	responder base.Responder,
) *Handler {
	return &Handler{
		tokensRepo: tokensRepo,
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

	id, err := input.ReadUint("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.NewValidationError("invalid token id"))

		return
	}

	tokens, err := h.tokensRepo.Find(ctx, filters.FindPersonalAccessTokenByIDs(id), nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find token"))

		return
	}

	if len(tokens) == 0 {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("token not found"),
			http.StatusNotFound,
		))

		return
	}

	token := tokens[0]

	if token.TokenableID != session.User.ID {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("access denied"),
			http.StatusForbidden,
		))

		return
	}

	err = h.tokensRepo.Delete(ctx, id)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to delete token"))

		return
	}

	rw.WriteHeader(http.StatusNoContent)
}
