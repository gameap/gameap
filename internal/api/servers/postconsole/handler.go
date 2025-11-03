package postconsole

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gameap/gameap/internal/api/base"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type daemonCommands interface {
	ExecuteCommand(
		ctx context.Context,
		node *domain.Node,
		command string,
		opts ...daemon.CommandServiceOption,
	) (*daemon.CommandResult, error)
}

type fileService interface {
	Upload(ctx context.Context, node *domain.Node, filePath string, content []byte, perms os.FileMode) error
}

type Handler struct {
	serverFinder   *serversbase.ServerFinder
	nodeRepo       repositories.NodeRepository
	rbac           base.RBAC
	daemonCommands daemonCommands
	fileService    fileService
	responder      base.Responder
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	nodeRepo repositories.NodeRepository,
	rbac base.RBAC,
	daemonCommands daemonCommands,
	fs fileService,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverFinder:   serversbase.NewServerFinder(serverRepo, rbac),
		nodeRepo:       nodeRepo,
		rbac:           rbac,
		daemonCommands: daemonCommands,
		fileService:    fs,
		responder:      responder,
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

	canSend, err := h.rbac.CanForEntity(
		ctx,
		session.User.ID,
		domain.EntityTypeServer,
		server.ID,
		[]domain.AbilityName{domain.AbilityNameGameServerConsoleSend},
	)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check console send permission"))

		return
	}

	if !canSend {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("user does not have permission to send console commands"),
			http.StatusForbidden,
		))

		return
	}

	var in consoleInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to parse request body"),
			http.StatusBadRequest,
		))

		return
	}

	if err := in.validate(); err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			err,
			http.StatusBadRequest,
		))

		return
	}

	if err := h.sendConsoleCommand(ctx, server, in.Command); err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to send console command"))

		return
	}

	h.responder.Write(ctx, rw, newConsoleResponse())
}

func (h *Handler) sendConsoleCommand(ctx context.Context, server *domain.Server, command string) error {
	nodes, err := h.nodeRepo.Find(ctx, &filters.FindNode{
		IDs: []uint{server.DSID},
	}, nil, &filters.Pagination{
		Limit: 1,
	})
	if err != nil {
		return errors.WithMessage(err, "failed to find node")
	}

	if len(nodes) == 0 {
		return api.NewNotFoundError("node not found")
	}

	node := &nodes[0]

	if node.ScriptSendCommand != nil && *node.ScriptSendCommand != "" {
		cmd := server.ReplaceServerShortcodes(node, *node.ScriptSendCommand, map[string]string{
			"command": command,
		})

		_, err := h.daemonCommands.ExecuteCommand(ctx, node, cmd)
		if err != nil {
			return errors.WithMessage(err, "failed to execute send command script")
		}

		return nil
	}

	return h.uploadInputFile(ctx, node, server.Dir, command)
}

func (h *Handler) uploadInputFile(ctx context.Context, node *domain.Node, serverDir string, command string) error {
	inputPath := filepath.Join(serverDir, "input.txt")

	err := h.fileService.Upload(ctx, node, inputPath, []byte(command), 0644)
	if err != nil {
		return errors.WithMessage(err, "failed to upload console command")
	}

	return nil
}
