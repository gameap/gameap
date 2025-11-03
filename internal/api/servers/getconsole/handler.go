package getconsole

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

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

const consoleMaxSymbols = 65536

type daemonCommands interface {
	ExecuteCommand(
		ctx context.Context,
		node *domain.Node,
		command string,
		opts ...daemon.CommandServiceOption,
	) (*daemon.CommandResult, error)
}

type fileService interface {
	Download(ctx context.Context, node *domain.Node, filePath string) ([]byte, error)
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

	canView, err := h.rbac.CanForEntity(
		ctx,
		session.User.ID,
		domain.EntityTypeServer,
		server.ID,
		[]domain.AbilityName{domain.AbilityNameGameServerConsoleView},
	)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check console view permission"))

		return
	}

	if !canView {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("user does not have permission to view console"),
			http.StatusForbidden,
		))

		return
	}

	consoleOutput, err := h.getConsoleLog(ctx, server)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to get console log"))

		return
	}

	h.responder.Write(ctx, rw, newConsoleResponse(consoleOutput))
}

func (h *Handler) getConsoleLog(ctx context.Context, server *domain.Server) (string, error) {
	nodes, err := h.nodeRepo.Find(ctx, &filters.FindNode{
		IDs: []uint{server.DSID},
	}, nil, &filters.Pagination{
		Limit: 1,
	})
	if err != nil {
		return "", errors.WithMessage(err, "failed to find node")
	}

	if len(nodes) == 0 {
		return "", api.NewNotFoundError("node not found")
	}

	node := &nodes[0]

	if node.ScriptGetConsole != nil && *node.ScriptGetConsole != "" {
		cmd := server.ReplaceServerShortcodes(node, *node.ScriptGetConsole, nil)

		result, err := h.daemonCommands.ExecuteCommand(ctx, node, cmd)
		if err != nil {
			return "", errors.WithMessage(err, "failed to execute get console script")
		}

		return result.Output, nil
	}

	return h.downloadOutputFile(ctx, node, server.Dir)
}

func (h *Handler) downloadOutputFile(ctx context.Context, node *domain.Node, filePath string) (string, error) {
	outputPath := filepath.Join(filePath, "output.txt")

	content, err := h.fileService.Download(ctx, node, outputPath)
	if err != nil {
		return "", errors.WithMessage(err, "failed to download console log")
	}

	result := string(content)

	if len(result) > consoleMaxSymbols {
		result = result[len(result)-consoleMaxSymbols:]
	}

	result = sanitizeUTF8(result)

	return result, nil
}

func sanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))

	for _, r := range s {
		if r == utf8.RuneError {
			continue
		}
		b.WriteRune(r)
	}

	return b.String()
}
