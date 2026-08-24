package putprofile

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	pluginproto "github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
)

// sessionTokenDuration mirrors the login handler's default session lifetime;
// the token re-issued after a password change is an ordinary session token.
const sessionTokenDuration = 24 * time.Hour

// PluginDispatcher publishes user events to plugins; satisfied by
// *plugin.Dispatcher.
type PluginDispatcher interface {
	DispatchUserEventAsync(
		ctx context.Context,
		eventType pluginproto.EventType,
		user *domain.User,
		extraData map[string]string,
	)
}

type Handler struct {
	userRepo         repositories.UserRepository
	tokenIssuer      tokenIssuer
	pluginDispatcher PluginDispatcher
	responder        base.Responder
}

func NewHandler(
	userRepo repositories.UserRepository,
	tokenIssuer tokenIssuer,
	pluginDispatcher PluginDispatcher,
	responder base.Responder,
) *Handler {
	return &Handler{
		userRepo:         userRepo,
		tokenIssuer:      tokenIssuer,
		pluginDispatcher: pluginDispatcher,
		responder:        responder,
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

	input := &updateProfileInput{}

	err := json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request"),
			http.StatusBadRequest,
		))

		return
	}

	err = input.Validate()
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid input"),
			http.StatusBadRequest,
		))

		return
	}

	users, err := h.userRepo.Find(ctx, &filters.FindUser{
		Logins: []string{session.Login},
	}, nil, &filters.Pagination{
		Limit: 1,
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

	if input.Password != nil {
		if err := applyPasswordChange(input, user); err != nil {
			h.responder.WriteError(ctx, rw, err)

			return
		}
	}

	if input.Name != nil {
		user.Name = input.Name
	}

	err = h.userRepo.Save(ctx, user)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to update profile"))

		return
	}

	h.dispatchUpdated(ctx, user, input)

	response := newUpdateProfileResponse()

	// A password change invalidates every previously-issued session token,
	// including the one authenticating this very request. Re-issue a fresh
	// token so the caller's session survives while all others stay revoked.
	// Both the password-changed cutoff and the token's iat are recorded at
	// second precision, so a token minted here always passes the cutoff.
	if input.Password != nil {
		token, err := h.tokenIssuer.GenerateTokenForUser(user, sessionTokenDuration)
		if err != nil {
			h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to generate token"))

			return
		}

		response.Token = token
	}

	h.responder.Write(ctx, rw, response)
}

// applyPasswordChange verifies the current password and replaces the stored
// hash, stamping password_changed_at so pre-existing credentials are revoked.
// The returned error is ready for the responder (HTTP-wrapped where needed).
func applyPasswordChange(input *updateProfileInput, user *domain.User) error {
	if input.CurrentPassword == nil {
		return api.WrapHTTPError(
			errors.New("current password is required for password change"),
			http.StatusBadRequest,
		)
	}

	// The new password (set below) will overwrite the stored hash either
	// way, so the rehash signal from VerifyPassword is intentionally
	// discarded — the upgrade happens implicitly via the new HashPassword.
	_, err := auth.VerifyPassword(user.Password, *input.CurrentPassword)
	if err != nil {
		return api.WrapHTTPError(
			errors.New("current password is incorrect"),
			http.StatusBadRequest,
		)
	}

	hashedPassword, err := auth.HashPassword(*input.Password)
	if err != nil {
		return errors.WithMessage(err, "failed to hash password")
	}

	user.Password = hashedPassword
	user.SetPasswordChangedAt(new(time.Now()))

	return nil
}

// dispatchUpdated tells plugins the user edited their own profile; field
// names only, never values.
func (h *Handler) dispatchUpdated(ctx context.Context, user *domain.User, input *updateProfileInput) {
	if h.pluginDispatcher == nil {
		return
	}

	fields := make([]string, 0, 2)
	if input.Name != nil {
		fields = append(fields, "name")
	}

	if input.Password != nil {
		fields = append(fields, "password")
	}

	h.pluginDispatcher.DispatchUserEventAsync(ctx, pluginproto.EventType_EVENT_TYPE_USER_UPDATED, user,
		map[string]string{"source": "profile", "changed_fields": strings.Join(fields, ",")})
}
