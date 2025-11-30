package getservers

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
	serverRepo        repositories.ServerRepository
	gameRepo          repositories.GameRepository
	gameModRepo       repositories.GameModRepository
	serverSettingRepo repositories.ServerSettingRepository
	responder         base.Responder
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	gameRepo repositories.GameRepository,
	gameModRepo repositories.GameModRepository,
	serverSettingRepo repositories.ServerSettingRepository,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverRepo:        serverRepo,
		gameRepo:          gameRepo,
		gameModRepo:       gameModRepo,
		serverSettingRepo: serverSettingRepo,
		responder:         responder,
	}
}

//nolint:funlen
func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	daemonSession := auth.DaemonSessionFromContext(ctx)
	if daemonSession == nil || daemonSession.Node == nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("daemon session not found"),
			http.StatusUnauthorized,
		))

		return
	}

	node := daemonSession.Node

	filter := parseFilters(r)
	filter.DSIDs = []uint{node.ID}
	filter.WithDeleted = true

	servers, err := h.serverRepo.Find(ctx, filter, nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to find servers"),
			http.StatusInternalServerError,
		))

		return
	}

	gameIDs := make([]string, 0)
	gameModIDs := make([]uint, 0)
	serverIDs := make([]uint, 0)

	for i := range servers {
		gameIDs = append(gameIDs, servers[i].GameID)
		gameModIDs = append(gameModIDs, servers[i].GameModID)
		serverIDs = append(serverIDs, servers[i].ID)
	}

	games, err := h.gameRepo.Find(ctx, &filters.FindGame{Codes: gameIDs}, nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to find games"),
			http.StatusInternalServerError,
		))

		return
	}

	gameMods, err := h.gameModRepo.Find(ctx, &filters.FindGameMod{IDs: gameModIDs}, nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to find game mods"),
			http.StatusInternalServerError,
		))

		return
	}

	settings, err := h.serverSettingRepo.Find(
		ctx,
		&filters.FindServerSetting{ServerIDs: serverIDs},
		nil,
		nil,
	)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to find server settings"),
			http.StatusInternalServerError,
		))

		return
	}

	gameMap := make(map[string]*domain.Game, len(games))
	for i := range games {
		gameMap[games[i].Code] = &games[i]
	}

	gameModMap := make(map[uint]*domain.GameMod, len(gameMods))
	for i := range gameMods {
		gameModMap[gameMods[i].ID] = &gameMods[i]
	}

	settingsMap := make(map[uint][]domain.ServerSetting, len(settings))
	for i := range settings {
		settingsMap[settings[i].ServerID] = append(
			settingsMap[settings[i].ServerID],
			settings[i],
		)
	}

	response := make([]ServerResponse, 0, len(servers))

	for i := range servers {
		server := &servers[i]

		game := gameMap[server.GameID]
		if game == nil {
			game = &domain.Game{}
		}

		gameMod := gameModMap[server.GameModID]
		if gameMod == nil {
			gameMod = &domain.GameMod{}
		}

		serverSettings := settingsMap[server.ID]
		if serverSettings == nil {
			serverSettings = make([]domain.ServerSetting, 0)
		}

		response = append(response, newServerResponse(
			server,
			game,
			gameMod,
			serverSettings,
			string(node.OS),
		))
	}

	h.responder.Write(ctx, rw, response)
}
