package middlewares

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type PersonalAccessMiddleware struct {
	tokenRepo repositories.PersonalAccessTokenRepository
	responder base.Responder
}

func NewPersonalAccessMiddleware(
	personalAccessTokenRepo repositories.PersonalAccessTokenRepository,
	responder base.Responder,
) *PersonalAccessMiddleware {
	return &PersonalAccessMiddleware{
		tokenRepo: personalAccessTokenRepo,
		responder: responder,
	}
}

func (m *PersonalAccessMiddleware) Middleware(
	next http.Handler,
	abilities []domain.PATAbility,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		session := auth.SessionFromContext(ctx)

		if !session.IsTokenSession() {
			next.ServeHTTP(w, r)

			return
		}

		if abilities == nil {
			m.responder.WriteError(ctx, w, api.WrapHTTPError(
				errors.New("no abilities specified for this endpoint"),
				http.StatusInternalServerError,
			))

			return
		}

		if session.Token.Abilities == nil {
			m.responder.WriteError(ctx, w, api.WrapHTTPError(
				errors.New("token abilities are not configured"),
				http.StatusForbidden,
			))

			return
		}

		if len(*session.Token.Abilities) < len(abilities) {
			m.responder.WriteError(ctx, w, api.WrapHTTPError(
				errors.New("insufficient token abilities"),
				http.StatusForbidden,
			))

			return
		}

		tokenAbilitiesMap := make(map[domain.PATAbility]struct{}, len(*session.Token.Abilities))

		for _, ability := range *session.Token.Abilities {
			tokenAbilitiesMap[ability] = struct{}{}
		}

		for _, ability := range abilities {
			if _, ok := tokenAbilitiesMap[ability]; !ok {
				m.responder.WriteError(ctx, w, api.WrapHTTPError(
					errors.Errorf("missing required ability: %s", ability),
					http.StatusForbidden,
				))

				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
