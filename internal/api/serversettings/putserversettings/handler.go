package putserversettings

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	settingsbase "github.com/gameap/gameap/internal/api/serversettings/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/serverconfigpush"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	serverSettingsRepo repositories.ServerSettingRepository
	serverFinder       *serversbase.ServerFinder
	abilityChecker     *serversbase.AbilityChecker
	gameModsRepo       repositories.GameModRepository
	configPusher       *serverconfigpush.Pusher
	rbac               base.RBAC
	responder          base.Responder
}

func NewHandler(
	serverSettingsRepo repositories.ServerSettingRepository,
	serverRepo repositories.ServerRepository,
	gameModsRepo repositories.GameModRepository,
	configPusher *serverconfigpush.Pusher,
	rbac base.RBAC,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverSettingsRepo: serverSettingsRepo,
		serverFinder:       serversbase.NewServerFinder(serverRepo, rbac),
		abilityChecker:     serversbase.NewAbilityChecker(rbac),
		gameModsRepo:       gameModsRepo,
		configPusher:       configPusher,
		rbac:               rbac,
		responder:          responder,
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

	serverID, err := input.ReadUint("server")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid server id"),
			http.StatusBadRequest,
		))

		return
	}

	server, err := h.serverFinder.FindUserServer(ctx, session.User, serverID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	isAdmin, err := h.rbac.Can(ctx, session.User.ID, []domain.AbilityName{domain.AbilityNameAdminRolesPermissions})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check admin permissions"))

		return
	}

	err = h.abilityChecker.CheckOrError(
		ctx,
		session.User.ID,
		server.ID,
		[]domain.AbilityName{domain.AbilityNameGameServerCommon, domain.AbilityNameGameServerSettings},
	)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	var settingsInput saveSettingsInput

	// UseNumber keeps an integer out of float64, which would silently lose
	// precision on large values before the variable type is even known.
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()

	err = decoder.Decode(&settingsInput)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to read request body"),
			http.StatusBadRequest,
		))

		return
	}

	err = settingsInput.Validate()
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			err,
			http.StatusBadRequest,
		))

		return
	}

	err = h.saveSettings(ctx, server, settingsInput, isAdmin)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	if h.configPusher != nil {
		h.configPusher.PushServerConfig(ctx, server.ID)
	}

	h.responder.Write(ctx, rw, SuccessResponse{})
}

func (h *Handler) findGameMod(ctx context.Context, gameModID uint) (*domain.GameMod, error) {
	gameMods, err := h.gameModsRepo.Find(ctx, &filters.FindGameMod{
		IDs: []uint{gameModID},
	}, nil, &filters.Pagination{
		Limit:  1,
		Offset: 0,
	})
	if err != nil {
		return nil, err
	}

	if len(gameMods) == 0 {
		return nil, nil
	}

	return &gameMods[0], nil
}

func (h *Handler) saveSettings(
	ctx context.Context,
	server *domain.Server,
	settingsInput saveSettingsInput,
	isAdmin bool,
) error {
	gameMod, err := h.findGameMod(ctx, server.GameModID)
	if err != nil {
		return errors.WithMessage(err, "failed to find game mod")
	}
	if gameMod == nil {
		return api.NewNotFoundError("game mod not found")
	}

	// Everything is validated before anything is written: a violation halfway
	// through must not leave the server with a half-applied configuration.
	normalized, err := settingsbase.Normalize(gameMod, settingsInput, isAdmin)
	if err != nil {
		return err
	}

	existingSettings, err := h.serverSettingsRepo.Find(ctx, &filters.FindServerSetting{
		ServerIDs: []uint{server.ID},
	}, nil, nil)
	if err != nil {
		return errors.WithMessage(err, "failed to find server settings")
	}

	existingIDs := make(map[string]uint, len(existingSettings))
	for i := range existingSettings {
		existingIDs[existingSettings[i].Name] = existingSettings[i].ID
	}

	for _, setting := range normalized {
		err := h.serverSettingsRepo.Save(ctx, &domain.ServerSetting{
			ID:       existingIDs[setting.Name],
			ServerID: server.ID,
			Name:     setting.Name,
			Value:    setting.Value,
		})
		if err != nil {
			return errors.WithMessage(err, "failed to save setting")
		}
	}

	return nil
}
