package gettokens

import (
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

	filter := &filters.FindPersonalAccessToken{
		TokenableTypes: []string{string(domain.EntityTypeUser)},
		TokenableIDs:   []uint{session.User.ID},
	}

	tokens, err := h.tokensRepo.Find(ctx, filter, []filters.Sorting{
		{
			Field:     "created_at",
			Direction: filters.SortDirectionDesc,
		},
	}, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find user tokens"))

		return
	}

	tokensResponse := newTokensResponseFromTokens(tokens)

	h.responder.Write(ctx, rw, tokensResponse)
}
