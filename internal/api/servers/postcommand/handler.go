package postcommand

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/gameap/gameap/internal/api/base"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	serverFinder   *serversbase.ServerFinder
	abilityChecker *serversbase.AbilityChecker
	responder      base.Responder

	commandMap   map[string]func(context.Context, *domain.Server) (uint, error)
	abilitiesMap map[string][]domain.AbilityName
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	serverManager serverManager,
	rbac base.RBAC,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverFinder:   serversbase.NewServerFinder(serverRepo, rbac),
		abilityChecker: serversbase.NewAbilityChecker(rbac),
		responder:      responder,

		commandMap: map[string]func(context.Context, *domain.Server) (uint, error){
			"start":     serverManager.Start,
			"stop":      serverManager.Stop,
			"restart":   serverManager.Restart,
			"update":    serverManager.Update,
			"install":   serverManager.Install,
			"reinstall": serverManager.Reinstall,
		},
		abilitiesMap: map[string][]domain.AbilityName{
			"start": {
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerStart,
			},
			"stop": {
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerStop,
			},
			"restart": {
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerRestart,
			},
			"update": {
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerUpdate,
			},
			"install": {
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerUpdate,
			},
			"reinstall": {
				domain.AbilityNameGameServerCommon,
				domain.AbilityNameGameServerUpdate,
			},
		},
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

	in := api.NewInputReader(r)

	serverID, err := in.ReadUint("server")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid server id"),
			http.StatusBadRequest,
		))

		return
	}

	command, err := h.readCommand(r)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	server, err := h.serverFinder.FindUserServer(ctx, session.User, serverID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	if err = h.abilityChecker.CheckOrError(ctx, session.User.ID, server.ID, h.abilitiesMap[command]); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	daemonTaskID, err := h.commandMap[command](ctx, server)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to execute command"))

		return
	}

	h.responder.Write(ctx, rw, newCommandResponse(daemonTaskID))
}

func (h *Handler) AuthorizeIdempotencyReplay(r *http.Request) error {
	command, err := h.readCommand(r)
	if err != nil {
		return err
	}

	return serversbase.AuthorizeIdempotencyReplay(r, h.serverFinder, h.abilityChecker, h.abilitiesMap[command])
}

func (h *Handler) readCommand(r *http.Request) (string, error) {
	parsedURI, err := url.ParseRequestURI(r.RequestURI)
	if err != nil {
		return "", api.WrapHTTPError(errors.WithMessage(err, "invalid command"), http.StatusBadRequest)
	}

	parts := strings.Split(parsedURI.Path, "/")
	command := parts[len(parts)-1]
	if _, exists := h.commandMap[command]; !exists {
		return "", api.WrapHTTPError(errors.New("invalid command"), http.StatusNotFound)
	}

	return command, nil
}
