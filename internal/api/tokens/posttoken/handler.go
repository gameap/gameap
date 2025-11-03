package posttoken

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

const tokenLength = 48

type Handler struct {
	tokenRepo repositories.PersonalAccessTokenRepository
	rbac      base.RBAC
	responder base.Responder
}

func NewHandler(
	tokenRepo repositories.PersonalAccessTokenRepository,
	rbac base.RBAC,
	responder base.Responder,
) *Handler {
	return &Handler{
		tokenRepo: tokenRepo,
		rbac:      rbac,
		responder: responder,
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

	if session.IsTokenSession() {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("token sessions cannot create new tokens"),
			http.StatusForbidden,
		))

		return
	}

	input := &tokenInput{}

	err := json.NewDecoder(r.Body).Decode(input)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request body"),
			http.StatusBadRequest,
		))

		return
	}

	err = input.Validate()
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	err = h.validateAdminAbilities(ctx, session.User, input.Abilities)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	plainToken, err := generateRandomToken()
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to generate token"),
			http.StatusInternalServerError,
		))

		return
	}

	hashedToken := pkgstrings.SHA256(plainToken)

	abilities := domain.ParseAbilities(input.Abilities)
	now := time.Now()

	token := &domain.PersonalAccessToken{
		TokenableType: domain.EntityTypeUser,
		TokenableID:   session.User.ID,
		Name:          input.TokenName,
		Token:         hashedToken,
		Abilities:     &abilities,
		LastUsedAt:    nil,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	}

	err = h.tokenRepo.Save(ctx, token)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to save token"),
			http.StatusInternalServerError,
		))

		return
	}

	response := newTokenResponse(token, plainToken)
	h.responder.Write(ctx, rw, response)
}

func (h *Handler) validateAdminAbilities(ctx context.Context, user *domain.User, abilities []string) error {
	isAdmin, err := h.rbac.Can(
		ctx, user.ID, []domain.AbilityName{domain.AbilityNameAdminRolesPermissions},
	)

	if err != nil {
		return errors.WithMessage(err, "failed to check user permissions")
	}

	if !isAdmin {
		adminAbilities := lo.Keyify(domain.GetAdminAbilities())

		for _, ability := range abilities {
			if _, ok := adminAbilities[domain.PATAbility(ability)]; ok {
				return api.NewValidationError("admin abilities require admin role")
			}
		}
	}

	return nil
}

func generateRandomToken() (string, error) {
	bytes := make([]byte, tokenLength)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate random token")
	}

	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	for i, b := range bytes {
		bytes[i] = letters[b%byte(len(letters))]
	}

	return string(bytes), nil
}
