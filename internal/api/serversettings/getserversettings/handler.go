package getserversettings

import (
	"context"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

const (
	autostartSettingKey         = "autostart"
	autostartCurrentSettingKey  = "autostart_current"
	updateBeforeStartSettingKey = "update_before_start"
)

type Handler struct {
	serverSettingsRepo repositories.ServerSettingRepository
	serversRepo        repositories.ServerRepository
	gameModsRepo       repositories.GameModRepository
	rbac               base.RBAC
	responder          base.Responder
}

func NewHandler(
	serverSettingsRepo repositories.ServerSettingRepository,
	serversRepo repositories.ServerRepository,
	gameModsRepo repositories.GameModRepository,
	rbac base.RBAC,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverSettingsRepo: serverSettingsRepo,
		serversRepo:        serversRepo,
		gameModsRepo:       gameModsRepo,
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

	server, err := h.findUserServer(ctx, session.User, serverID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	canSettings, err := h.rbac.CanForEntity(
		ctx,
		session.User.ID,
		domain.EntityTypeServer,
		server.ID,
		[]domain.AbilityName{domain.AbilityNameGameServerCommon, domain.AbilityNameGameServerSettings},
	)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check permissions"))

		return
	}

	if !canSettings {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("insufficient permissions"),
			http.StatusForbidden,
		))

		return
	}

	isAdmin, err := h.rbac.Can(ctx, session.User.ID, []domain.AbilityName{domain.AbilityNameAdminRolesPermissions})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check admin permissions"))

		return
	}

	gameMods, err := h.gameModsRepo.Find(ctx, &filters.FindGameMod{
		IDs: []uint{server.GameModID},
	}, nil, &filters.Pagination{
		Limit:  1,
		Offset: 0,
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find game mod"))

		return
	}
	if len(gameMods) == 0 {
		h.responder.WriteError(ctx, rw, api.NewNotFoundError("game mod not found"))

		return
	}

	var gameMod *domain.GameMod
	if len(gameMods) > 0 {
		gameMod = &gameMods[0]
	}

	serverSettings, err := h.serverSettingsRepo.Find(ctx, &filters.FindServerSetting{
		ServerIDs: []uint{serverID},
	}, nil, nil)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find server settings"))

		return
	}

	response := h.buildSettingsResponse(server, gameMod, serverSettings, isAdmin)

	h.responder.Write(ctx, rw, response)
}

func (h *Handler) findUserServer(ctx context.Context, user *domain.User, serverID uint) (*domain.Server, error) {
	isAdmin, err := h.rbac.Can(ctx, user.ID, []domain.AbilityName{domain.AbilityNameAdminRolesPermissions})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to check admin permissions")
	}

	filter := &filters.FindServer{
		IDs: []uint{serverID},
	}

	if !isAdmin {
		filter.UserIDs = []uint{user.ID}
	}

	servers, err := h.serversRepo.Find(ctx, filter, nil, &filters.Pagination{
		Limit:  1,
		Offset: 0,
	})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find server")
	}

	if len(servers) == 0 {
		return nil, api.NewNotFoundError("server not found")
	}

	return &servers[0], nil
}

func (h *Handler) buildSettingsResponse(
	_ *domain.Server,
	gameMod *domain.GameMod,
	serverSettings []domain.ServerSetting,
	isAdmin bool,
) []SettingResponse {
	settingsMap := make(map[string]SettingResponse)
	order := make([]string, 0, len(serverSettings)+2)

	settingsMap[autostartSettingKey] = SettingResponse{
		Name:  autostartSettingKey,
		Value: false,
		Type:  "bool",
		Label: "Autostart",
	}
	order = append(order, autostartSettingKey)

	settingsMap[updateBeforeStartSettingKey] = SettingResponse{
		Name:  updateBeforeStartSettingKey,
		Value: false,
		Type:  "bool",
		Label: "Update before start",
	}
	order = append(order, updateBeforeStartSettingKey)

	if gameMod != nil {
		for _, gmVar := range gameMod.Vars {
			if gmVar.AdminVar && !isAdmin {
				continue
			}

			settingsMap[gmVar.Var] = SettingResponse{
				Name:  gmVar.Var,
				Value: string(gmVar.Default),
				Type:  "string",
				Label: gmVar.Info,
			}
			order = append(order, gmVar.Var)
		}
	}

	for _, setting := range serverSettings {
		if existingSetting, exists := settingsMap[setting.Name]; exists {
			if existingSetting.AdminVar && !isAdmin {
				continue
			}

			settingsMap[setting.Name] = SettingResponse{
				Name:  setting.Name,
				Value: setting.Value,
				Label: existingSetting.Label,
				Type:  existingSetting.Type,
			}
		}
	}

	delete(settingsMap, autostartCurrentSettingKey)

	// Build result using order slice
	result := make([]SettingResponse, 0, len(order))
	for _, key := range order {
		if setting, exists := settingsMap[key]; exists {
			result = append(result, setting)
		}
	}

	return result
}
