package postserver

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	settingsbase "github.com/gameap/gameap/internal/api/serversettings/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/servercontrol"
	"github.com/gameap/gameap/pkg/api"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/pkg/errors"
)

// TaskDispatcher is an interface for dispatching daemon tasks via gRPC.
type TaskDispatcher interface {
	Dispatch(ctx context.Context, task *domain.DaemonTask) error
}

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
	serverRepo         repositories.ServerRepository
	nodeRepo           repositories.NodeRepository
	gameRepo           repositories.GameRepository
	gameModRepo        repositories.GameModRepository
	daemonTaskRepo     repositories.DaemonTaskRepository
	serverSettingsRepo repositories.ServerSettingRepository
	taskDispatcher     TaskDispatcher
	pluginDispatcher   PluginDispatcher
	responder          base.Responder
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	nodeRepo repositories.NodeRepository,
	gameRepo repositories.GameRepository,
	gameModRepo repositories.GameModRepository,
	daemonTaskRepo repositories.DaemonTaskRepository,
	serverSettingsRepo repositories.ServerSettingRepository,
	taskDispatcher TaskDispatcher,
	pluginDispatcher PluginDispatcher,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverRepo:         serverRepo,
		nodeRepo:           nodeRepo,
		gameRepo:           gameRepo,
		gameModRepo:        gameModRepo,
		daemonTaskRepo:     daemonTaskRepo,
		serverSettingsRepo: serverSettingsRepo,
		taskDispatcher:     taskDispatcher,
		pluginDispatcher:   pluginDispatcher,
		responder:          responder,
	}
}

const defaultRconPasswordLength = 10

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	input := &serverInput{}

	// UseNumber keeps an integer out of float64, which would silently lose
	// precision on large setting values before the variable type is even known.
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()

	err := decoder.Decode(&input)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "invalid request"))

		return
	}

	err = input.Validate()
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "invalid input"))

		return
	}

	server := input.ToDomain()

	gameMod, err := h.prepareServer(ctx, server, input)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	// Settings are validated before the server row is written so a rejected value
	// cannot leave a half-created server behind. The route is admin-only, hence
	// isAdmin = true.
	normalizedSettings, err := settingsbase.Normalize(gameMod, input.Settings, true)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	err = h.serverRepo.Save(ctx, server)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to save server"))

		return
	}

	if h.pluginDispatcher != nil {
		h.pluginDispatcher.DispatchServerEventAsync(ctx, servercontrol.PluginEventServerCreated, server, nil)
	}

	if len(normalizedSettings) > 0 {
		err = h.saveSettings(ctx, server.ID, normalizedSettings)
		if err != nil {
			h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to save settings"))

			return
		}
	}

	taskID := uint(0)

	if input.Install != nil && *input.Install {
		taskID, err = h.createInstallTask(ctx, server)
		if err != nil {
			h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to create install task"))

			return
		}
	}

	response := createServerResponse{
		Message: "success",
		Result: createServerResult{
			TaskID:   taskID,
			ServerID: server.ID,
		},
	}
	rw.WriteHeader(http.StatusCreated)
	h.responder.Write(ctx, rw, response)
}

func (h *Handler) prepareServer(
	ctx context.Context,
	server *domain.Server,
	input *serverInput,
) (*domain.GameMod, error) {
	if server.Rcon == nil || *server.Rcon == "" {
		rconPassword, err := pkgstrings.CryptoRandomString(defaultRconPasswordLength)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to generate rcon password")
		}
		server.Rcon = &rconPassword
	}

	nodes, err := h.nodeRepo.Find(ctx, &filters.FindNode{IDs: []uint{server.DSID}}, nil, nil)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find node")
	}

	if len(nodes) == 0 {
		return nil, errors.New("node not found")
	}

	node := &nodes[0]

	games, err := h.gameRepo.Find(ctx, &filters.FindGame{Codes: []string{input.GameID}}, nil, nil)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find game")
	}

	if len(games) == 0 {
		return nil, errors.New("game not found")
	}

	gameMods, err := h.gameModRepo.Find(ctx, &filters.FindGameMod{IDs: []uint{server.GameModID}}, nil, nil)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find game mod")
	}

	if len(gameMods) == 0 {
		return nil, errors.New("game mod not found")
	}

	gameMod := &gameMods[0]

	if gameMod.GameCode != input.GameID {
		return nil, api.NewValidationError("game mod does not belong to the specified game")
	}

	if server.StartCommand == nil || *server.StartCommand == "" {
		switch node.OS {
		case domain.NodeOSLinux:
			server.StartCommand = gameMod.StartCmdLinux
		case domain.NodeOSWindows:
			server.StartCommand = gameMod.StartCmdWindows
		}
	}

	if server.Dir == "" {
		server.Dir = "servers/" + server.XID().String()
	}

	if input.Install != nil && *input.Install {
		server.Installed = domain.ServerInstalledStatusNotInstalled
	}

	return gameMod, nil
}

func (h *Handler) createInstallTask(ctx context.Context, server *domain.Server) (uint, error) {
	now := time.Now()
	task := &domain.DaemonTask{
		DedicatedServerID: server.DSID,
		ServerID:          &server.ID,
		Task:              domain.DaemonTaskTypeServerInstall,
		Status:            domain.DaemonTaskStatusWaiting,
		CreatedAt:         &now,
		UpdatedAt:         &now,
	}

	var err error
	if h.taskDispatcher != nil {
		err = h.taskDispatcher.Dispatch(ctx, task)
	} else {
		err = h.daemonTaskRepo.Save(ctx, task)
	}

	if err != nil {
		return 0, errors.WithMessage(err, "failed to dispatch daemon task")
	}

	return task.ID, nil
}

func (h *Handler) saveSettings(
	ctx context.Context,
	serverID uint,
	settings []settingsbase.NormalizedSetting,
) error {
	for _, setting := range settings {
		err := h.serverSettingsRepo.Save(ctx, &domain.ServerSetting{
			ServerID: serverID,
			Name:     setting.Name,
			Value:    setting.Value,
		})
		if err != nil {
			return errors.WithMessage(err, "failed to save setting")
		}
	}

	return nil
}
