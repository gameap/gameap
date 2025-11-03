package putserversettings

import (
	"context"
	"encoding/json"
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

	canControl, err := h.rbac.CanForEntity(
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

	if !canControl {
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

	var settingsInput saveSettingsInput
	err = json.NewDecoder(r.Body).Decode(&settingsInput)
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

	settingsInputMap := settingsInput.ToSettingsMap()

	err = h.saveSettings(ctx, server, settingsInputMap, isAdmin)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to save settings"))

		return
	}

	h.responder.Write(ctx, rw, SuccessResponse{})
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

func (h *Handler) buildAllowedSettings(gameMod *domain.GameMod, isAdmin bool) map[string]settingMetadata {
	allowedSettings := make(map[string]settingMetadata)

	allowedSettings[autostartSettingKey] = settingMetadata{
		name:     autostartSettingKey,
		adminVar: false,
	}

	allowedSettings[updateBeforeStartSettingKey] = settingMetadata{
		name:     updateBeforeStartSettingKey,
		adminVar: false,
	}

	if gameMod != nil {
		for _, gmVar := range gameMod.Vars {
			if gmVar.AdminVar && !isAdmin {
				continue
			}

			allowedSettings[gmVar.Var] = settingMetadata{
				name:     gmVar.Var,
				adminVar: gmVar.AdminVar,
			}
		}
	}

	return allowedSettings
}

func (h *Handler) saveSettings(
	ctx context.Context,
	server *domain.Server,
	settingsInputMap map[string]any,
	isAdmin bool,
) error {
	gameMod, err := h.findGameMod(ctx, server.GameModID)
	if err != nil {
		return errors.WithMessage(err, "failed to find game mod")
	}
	if gameMod == nil {
		return api.NewNotFoundError("game mod not found")
	}

	allowedSettings := h.buildAllowedSettings(gameMod, isAdmin)

	existingSettings, err := h.serverSettingsRepo.Find(ctx, &filters.FindServerSetting{
		ServerIDs: []uint{server.ID},
	}, nil, nil)
	if err != nil {
		return errors.WithMessage(err, "failed to find server settings")
	}

	existingSettingsMap := make(map[string]*domain.ServerSetting)
	for i := range existingSettings {
		existingSettingsMap[existingSettings[i].Name] = &existingSettings[i]
	}

	for settingName, settingValue := range settingsInputMap {
		allowedSetting, isAllowed := allowedSettings[settingName]
		if !isAllowed {
			continue
		}

		if allowedSetting.adminVar && !isAdmin {
			continue
		}

		existingSetting, exists := existingSettingsMap[settingName]
		if exists {
			updatedSetting := &domain.ServerSetting{
				ID:       existingSetting.ID,
				ServerID: server.ID,
				Name:     settingName,
				Value:    domain.NewServerSettingValue(settingValue),
			}
			err := h.serverSettingsRepo.Save(ctx, updatedSetting)
			if err != nil {
				return errors.WithMessage(err, "failed to update setting")
			}
		} else {
			newSetting := &domain.ServerSetting{
				ServerID: server.ID,
				Name:     settingName,
				Value:    domain.NewServerSettingValue(settingValue),
			}
			err := h.serverSettingsRepo.Save(ctx, newSetting)
			if err != nil {
				return errors.WithMessage(err, "failed to create setting")
			}
		}
	}

	return nil
}

type settingMetadata struct {
	name     string
	adminVar bool
}
