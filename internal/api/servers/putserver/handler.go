package putserver

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	settingsbase "github.com/gameap/gameap/internal/api/serversettings/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/serverconfigpush"
	"github.com/gameap/gameap/internal/services/servercontrol"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
)

// PluginDispatcher dispatches server lifecycle events to plugins.
type PluginDispatcher interface {
	DispatchServerEventAsync(
		ctx context.Context,
		eventType servercontrol.PluginEventType,
		server *domain.Server,
		extraData map[string]string,
	)
}

type Handler struct {
	serverRepo       repositories.ServerRepository
	nodeRepo         repositories.NodeRepository
	gameRepo         repositories.GameRepository
	gameModRepo      repositories.GameModRepository
	configPusher     *serverconfigpush.Pusher
	pluginDispatcher PluginDispatcher
	rbac             base.RBAC
	responder        base.Responder
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	nodeRepo repositories.NodeRepository,
	gameRepo repositories.GameRepository,
	gameModRepo repositories.GameModRepository,
	configPusher *serverconfigpush.Pusher,
	pluginDispatcher PluginDispatcher,
	rbac base.RBAC,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverRepo:       serverRepo,
		nodeRepo:         nodeRepo,
		gameRepo:         gameRepo,
		gameModRepo:      gameModRepo,
		configPusher:     configPusher,
		pluginDispatcher: pluginDispatcher,
		rbac:             rbac,
		responder:        responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	serverID, err := api.NewInputReader(r).ReadUint("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid server id"),
			http.StatusBadRequest,
		))

		return
	}

	servers, err := h.serverRepo.Find(ctx, filters.FindServerByIDs(serverID), nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find server"))

		return
	}

	if len(servers) == 0 {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("server not found"),
			http.StatusNotFound,
		))

		return
	}

	server := &servers[0]

	input := &updateServerInput{}
	err = json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request body"),
			http.StatusBadRequest,
		))

		return
	}

	err = input.Validate()
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "validation failed"))

		return
	}

	err = h.prepareUpdate(ctx, server, input)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	err = input.Apply(server)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to apply input"))

		return
	}

	err = h.serverRepo.Save(ctx, server)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to save server"))

		return
	}

	if h.pluginDispatcher != nil {
		h.pluginDispatcher.DispatchServerEventAsync(ctx, servercontrol.PluginEventServerUpdated, server, nil)
	}

	if h.configPusher != nil {
		h.configPusher.PushServerConfig(ctx, server.ID)
	}

	h.responder.Write(ctx, rw, base.Success)
}

func (h *Handler) prepareUpdate(
	ctx context.Context,
	currentServer *domain.Server,
	input *updateServerInput,
) error {
	newDSID := uint(input.DSID.Int())
	newGameModID := uint(input.GameModID.Int())

	if newDSID != currentServer.DSID {
		nodes, err := h.nodeRepo.Find(ctx, &filters.FindNode{IDs: []uint{newDSID}}, nil, nil)
		if err != nil {
			return errors.WithMessage(err, "failed to find node")
		}

		if len(nodes) == 0 {
			return errors.New("node not found")
		}
	}

	gameChanged := input.GameID != currentServer.GameID
	gameModChanged := newGameModID != currentServer.GameModID

	if gameChanged {
		games, err := h.gameRepo.Find(ctx, &filters.FindGame{Codes: []string{input.GameID}}, nil, nil)
		if err != nil {
			return errors.WithMessage(err, "failed to find game")
		}

		if len(games) == 0 {
			return errors.New("game not found")
		}
	}

	var gameMod *domain.GameMod

	if gameChanged || gameModChanged || len(input.Vars) > 0 {
		gameMods, err := h.gameModRepo.Find(ctx, &filters.FindGameMod{IDs: []uint{newGameModID}}, nil, nil)
		if err != nil {
			return errors.WithMessage(err, "failed to find game mod")
		}

		if len(gameMods) > 0 {
			gameMod = &gameMods[0]
		}
	}

	if gameChanged || gameModChanged {
		if gameMod == nil {
			return errors.New("game mod not found")
		}

		if gameMod.GameCode != input.GameID {
			return api.NewValidationError("game mod does not belong to the specified game")
		}
	}

	// server.vars overrides the mod defaults for the daemon, so keys that name a
	// mod variable go through the same rules as the settings form. Keys that do
	// not are low-level administrator overrides and pass through untouched. With
	// no mod to check against there is nothing to validate.
	if gameMod != nil {
		normalizedVars, err := settingsbase.NormalizeVars(gameMod, input.Vars)
		if err != nil {
			return err
		}

		input.Vars = normalizedVars
	}

	return nil
}
