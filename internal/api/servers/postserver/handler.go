package postserver

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/pkg/errors"
)

type Handler struct {
	serverRepo     repositories.ServerRepository
	nodeRepo       repositories.NodeRepository
	gameModRepo    repositories.GameModRepository
	daemonTaskRepo repositories.DaemonTaskRepository
	responder      base.Responder
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	nodeRepo repositories.NodeRepository,
	gameModRepo repositories.GameModRepository,
	daemonTaskRepo repositories.DaemonTaskRepository,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverRepo:     serverRepo,
		nodeRepo:       nodeRepo,
		gameModRepo:    gameModRepo,
		daemonTaskRepo: daemonTaskRepo,
		responder:      responder,
	}
}

const defaultRconPasswordLength = 10

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	input := &serverInput{}

	err := json.NewDecoder(r.Body).Decode(&input)
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

	err = h.prepareServer(ctx, server, input)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	err = h.serverRepo.Save(ctx, server)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to save server"))

		return
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
) error {
	if server.Rcon == nil || *server.Rcon == "" {
		rconPassword, err := pkgstrings.CryptoRandomString(defaultRconPasswordLength)
		if err != nil {
			return errors.WithMessage(err, "failed to generate rcon password")
		}
		server.Rcon = &rconPassword
	}

	nodes, err := h.nodeRepo.Find(ctx, &filters.FindNode{IDs: []uint{server.DSID}}, nil, nil)
	if err != nil {
		return errors.WithMessage(err, "failed to find node")
	}

	if len(nodes) == 0 {
		return errors.New("node not found")
	}

	node := &nodes[0]

	if server.StartCommand == nil || *server.StartCommand == "" {
		gameMods, err := h.gameModRepo.Find(ctx, &filters.FindGameMod{IDs: []uint{server.GameModID}}, nil, nil)
		if err != nil {
			return errors.WithMessage(err, "failed to find game mod")
		}

		if len(gameMods) == 0 {
			return errors.New("game mod not found")
		}

		gameMod := &gameMods[0]

		switch node.OS {
		case domain.NodeOSLinux:
			server.StartCommand = gameMod.StartCmdLinux
		case domain.NodeOSWindows:
			server.StartCommand = gameMod.StartCmdWindows
		}
	}

	if server.Dir == "" {
		server.Dir = "servers/" + server.UUID.String()
	}

	if input.Install != nil && *input.Install {
		server.Installed = domain.ServerInstalledStatusNotInstalled
	}

	return nil
}

func (h *Handler) createInstallTask(ctx context.Context, server *domain.Server) (uint, error) {
	task := &domain.DaemonTask{
		DedicatedServerID: server.DSID,
		ServerID:          &server.ID,
		Task:              domain.DaemonTaskTypeServerInstall,
		Status:            domain.DaemonTaskStatusWaiting,
	}

	err := h.daemonTaskRepo.Save(ctx, task)
	if err != nil {
		return 0, errors.WithMessage(err, "failed to save daemon task")
	}

	return task.ID, nil
}
