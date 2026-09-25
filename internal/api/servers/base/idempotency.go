package base

import (
	"net/http"

	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

// AuthorizeIdempotencyReplay checks current server ownership, block state and
// abilities without checking the existence of a deleted child entity (a task).
func AuthorizeIdempotencyReplay(
	r *http.Request, finder *ServerFinder, checker *AbilityChecker, abilities []domain.AbilityName,
) error {
	ctx := r.Context()
	session := auth.SessionFromContext(ctx)
	if !session.IsAuthenticated() {
		return api.WrapHTTPError(errors.New("user not authenticated"), http.StatusUnauthorized)
	}

	serverID, err := api.NewInputReader(r).ReadUint("server")
	if err != nil {
		return api.WrapHTTPError(errors.WithMessage(err, "invalid server id"), http.StatusBadRequest)
	}

	server, err := finder.FindUserServer(ctx, session.User, serverID)
	if err != nil {
		return err
	}

	return checker.CheckOrError(ctx, session.User.ID, server.ID, abilities)
}
