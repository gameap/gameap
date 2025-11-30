package getserverid

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

	serverID, err := api.NewInputReader(r).ReadUint("server")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid server ID"),
			http.StatusBadRequest,
		))

		return
	}

	// Find the server
	filter := &filters.FindServer{
		IDs:         []uint{serverID},
		DSIDs:       []uint{node.ID},
		WithDeleted: true,
	}

	servers, err := h.serverRepo.Find(ctx, filter, nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to find server"),
			http.StatusInternalServerError,
		))

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

	// Get game
	games, err := h.gameRepo.Find(ctx, &filters.FindGame{Codes: []string{server.GameID}}, nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to find game"),
			http.StatusInternalServerError,
		))

		return
	}

	game := &domain.Game{}
	if len(games) > 0 {
		game = &games[0]
	}

	// Get game mod
	gameMods, err := h.gameModRepo.Find(ctx, &filters.FindGameMod{IDs: []uint{server.GameModID}}, nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to find game mod"),
			http.StatusInternalServerError,
		))

		return
	}

	gameMod := &domain.GameMod{}
	if len(gameMods) > 0 {
		gameMod = &gameMods[0]
	}

	// Get server settings
	settings, err := h.serverSettingRepo.Find(
		ctx,
		&filters.FindServerSetting{ServerIDs: []uint{server.ID}},
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

	if settings == nil {
		settings = make([]domain.ServerSetting, 0)
	}

	response := newServerResponse(
		server,
		game,
		gameMod,
		settings,
		node.OS,
	)

	h.responder.Write(ctx, rw, response)
}
