package getserversettings

import (
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	settingsbase "github.com/gameap/gameap/internal/api/serversettings/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type Handler struct {
	serverSettingsRepo repositories.ServerSettingRepository
	serverFinder       *serversbase.ServerFinder
	abilityChecker     *serversbase.AbilityChecker
	gameModsRepo       repositories.GameModRepository
	rbac               base.RBAC
	responder          base.Responder
}

func NewHandler(
	serverSettingsRepo repositories.ServerSettingRepository,
	serverRepo repositories.ServerRepository,
	gameModsRepo repositories.GameModRepository,
	rbac base.RBAC,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverSettingsRepo: serverSettingsRepo,
		serverFinder:       serversbase.NewServerFinder(serverRepo, rbac),
		abilityChecker:     serversbase.NewAbilityChecker(rbac),
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

	if !isAdmin {
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

func (h *Handler) buildSettingsResponse(
	_ *domain.Server,
	gameMod *domain.GameMod,
	serverSettings []domain.ServerSetting,
	isAdmin bool,
) []SettingResponse {
	settingsMap := make(map[string]SettingResponse)
	order := make([]string, 0, len(serverSettings)+2)

	// The two panel-owned settings are real booleans and carry no game mod
	// definition. The client localizes their labels by name.
	settingsMap[settingsbase.AutostartSettingKey] = SettingResponse{
		Name:    settingsbase.AutostartSettingKey,
		Value:   false,
		Default: false,
		Type:    string(domain.GameModVarTypeBool),
		Label:   "Autostart",
	}
	order = append(order, settingsbase.AutostartSettingKey)

	settingsMap[settingsbase.UpdateBeforeStartSettingKey] = SettingResponse{
		Name:    settingsbase.UpdateBeforeStartSettingKey,
		Value:   false,
		Default: false,
		Type:    string(domain.GameModVarTypeBool),
		Label:   "Update before start",
	}
	order = append(order, settingsbase.UpdateBeforeStartSettingKey)

	definitions := make(map[string]*domain.GameModVar)

	if gameMod != nil {
		for i := range gameMod.Vars {
			gmVar := &gameMod.Vars[i]

			if gmVar.AdminVar && !isAdmin {
				continue
			}

			definitions[gmVar.Var] = gmVar
			settingsMap[gmVar.Var] = newSettingResponseFromVar(gmVar)
			order = append(order, gmVar.Var)
		}
	}

	for _, setting := range serverSettings {
		existing, exists := settingsMap[setting.Name]
		if !exists {
			continue
		}

		// Only the value is replaced: rebuilding the entry here would drop the
		// whole definition that was just assembled from the game mod.
		if definition, isVar := definitions[setting.Name]; isVar {
			raw, present := setting.Value.Raw()
			if !present {
				continue
			}

			existing.Value = definition.FormatValue(raw)
		} else {
			value, _ := setting.Value.Bool()
			existing.Value = value
		}

		settingsMap[setting.Name] = existing
	}

	delete(settingsMap, settingsbase.AutostartCurrentSettingKey)

	// Build result using order slice
	result := make([]SettingResponse, 0, len(order))
	for _, key := range order {
		if setting, exists := settingsMap[key]; exists {
			result = append(result, setting)
		}
	}

	return result
}
