package putserversettings

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/gameap/gameap/internal/api/base"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/serverconfigpush"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

const (
	autostartSettingKey         = "autostart"
	autostartCurrentSettingKey  = "autostart_current"
	updateBeforeStartSettingKey = "update_before_start"
)

// PluginDispatcher publishes the settings a save changed to plugins;
// satisfied by *plugin.Dispatcher.
type PluginDispatcher interface {
	DispatchServerSettingsEventAsync(
		ctx context.Context,
		serverID uint,
		settings []domain.ServerSetting,
		extraData map[string]string,
	)
}

type Handler struct {
	serverSettingsRepo repositories.ServerSettingRepository
	serverFinder       *serversbase.ServerFinder
	abilityChecker     *serversbase.AbilityChecker
	gameModsRepo       repositories.GameModRepository
	configPusher       *serverconfigpush.Pusher
	pluginDispatcher   PluginDispatcher
	rbac               base.RBAC
	responder          base.Responder
}

func NewHandler(
	serverSettingsRepo repositories.ServerSettingRepository,
	serverRepo repositories.ServerRepository,
	gameModsRepo repositories.GameModRepository,
	configPusher *serverconfigpush.Pusher,
	pluginDispatcher PluginDispatcher,
	rbac base.RBAC,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverSettingsRepo: serverSettingsRepo,
		serverFinder:       serversbase.NewServerFinder(serverRepo, rbac),
		abilityChecker:     serversbase.NewAbilityChecker(rbac),
		gameModsRepo:       gameModsRepo,
		configPusher:       configPusher,
		pluginDispatcher:   pluginDispatcher,
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

	changed, err := h.saveSettings(ctx, server, settingsInputMap, isAdmin)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to save settings"))

		return
	}

	if h.configPusher != nil {
		h.configPusher.PushServerConfig(ctx, server.ID)
	}

	h.dispatchChanged(ctx, server.ID, changed)

	h.responder.Write(ctx, rw, SuccessResponse{})
}

// dispatchChanged tells plugins which settings a save created or changed; a
// save that touched nothing publishes nothing.
func (h *Handler) dispatchChanged(ctx context.Context, serverID uint, changed []domain.ServerSetting) {
	if h.pluginDispatcher == nil || len(changed) == 0 {
		return
	}

	names := make([]string, 0, len(changed))
	for _, setting := range changed {
		names = append(names, setting.Name)
	}

	h.pluginDispatcher.DispatchServerSettingsEventAsync(ctx, serverID, changed,
		map[string]string{"changed_fields": strings.Join(names, ",")})
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

// saveSettings persists the allowed settings and reports the ones that are
// new or whose value changed, in a stable (name) order.
func (h *Handler) saveSettings(
	ctx context.Context,
	server *domain.Server,
	settingsInputMap map[string]any,
	isAdmin bool,
) ([]domain.ServerSetting, error) {
	gameMod, err := h.findGameMod(ctx, server.GameModID)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find game mod")
	}
	if gameMod == nil {
		return nil, api.NewNotFoundError("game mod not found")
	}

	allowedSettings := h.buildAllowedSettings(gameMod, isAdmin)

	existingSettings, err := h.serverSettingsRepo.Find(ctx, &filters.FindServerSetting{
		ServerIDs: []uint{server.ID},
	}, nil, nil)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find server settings")
	}

	existingSettingsMap := make(map[string]*domain.ServerSetting)
	for i := range existingSettings {
		existingSettingsMap[existingSettings[i].Name] = &existingSettings[i]
	}

	changed := make([]domain.ServerSetting, 0)

	for settingName, settingValue := range settingsInputMap {
		allowedSetting, isAllowed := allowedSettings[settingName]
		if !isAllowed {
			continue
		}

		if allowedSetting.adminVar && !isAdmin {
			continue
		}

		setting := domain.ServerSetting{
			ServerID: server.ID,
			Name:     settingName,
			Value:    domain.NewServerSettingValue(settingValue),
		}

		existingSetting, exists := existingSettingsMap[settingName]
		if exists {
			setting.ID = existingSetting.ID
		}

		if err := h.serverSettingsRepo.Save(ctx, &setting); err != nil {
			if exists {
				return nil, errors.WithMessage(err, "failed to update setting")
			}

			return nil, errors.WithMessage(err, "failed to create setting")
		}

		if !exists || !sameSettingValue(existingSetting.Value, setting.Value) {
			changed = append(changed, setting)
		}
	}

	sort.Slice(changed, func(i, j int) bool { return changed[i].Name < changed[j].Name })

	return changed, nil
}

func sameSettingValue(a, b domain.ServerSettingValue) bool {
	left, _ := a.String()
	right, _ := b.String()

	return left == right
}

type settingMetadata struct {
	name     string
	adminVar bool
}
