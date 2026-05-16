package chmod

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gameap/gameap/internal/api/base"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

const maxMode = 0o777

type Handler struct {
	serverFinder   *serversbase.ServerFinder
	abilityChecker *serversbase.AbilityChecker
	nodeRepo       repositories.NodeRepository
	daemonFiles    fileService
	responder      base.Responder
	audit          audit.Logger
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	nodeRepo repositories.NodeRepository,
	rbac base.RBAC,
	daemonFiles fileService,
	responder base.Responder,
	auditLogger audit.Logger,
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &Handler{
		serverFinder:   serversbase.NewServerFinder(serverRepo, rbac),
		abilityChecker: serversbase.NewAbilityChecker(rbac),
		nodeRepo:       nodeRepo,
		daemonFiles:    daemonFiles,
		responder:      responder,
		audit:          auditLogger,
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

	err = h.abilityChecker.CheckOrError(
		ctx,
		session.User.ID,
		server.ID,
		[]domain.AbilityName{domain.AbilityNameGameServerFiles},
	)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	var req chmodRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request body"),
			http.StatusBadRequest,
		))

		return
	}

	if err = h.validateRequest(&req); err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(err, http.StatusBadRequest))

		return
	}

	node, err := h.getNode(ctx, server.DSID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	if err = h.processItems(ctx, node, server.Dir, &req); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	audit.SensitiveOp(ctx, h.audit, audit.EventFileChmod, audit.CategoryFileOp,
		"server", strconv.FormatUint(uint64(serverID), 10), "chmod",
		slog.Int("items", len(req.Items)), slog.Int("mode", req.Mode))

	h.responder.Write(ctx, rw, newChmodResponse())
}

func (h *Handler) validateRequest(req *chmodRequest) error {
	if req.Disk != "server" {
		return errors.Errorf("unsupported disk: %s, only 'server' disk is supported", req.Disk)
	}

	if len(req.Items) == 0 {
		return errors.New("items array is empty")
	}

	if req.Mode < 0 || req.Mode > maxMode {
		return errors.Errorf("invalid mode: %d, must be between 0 and 0o777", req.Mode)
	}

	return nil
}

func (h *Handler) getNode(ctx context.Context, nodeID uint) (*domain.Node, error) {
	nodes, err := h.nodeRepo.Find(ctx, &filters.FindNode{
		IDs: []uint{nodeID},
	}, nil, &filters.Pagination{
		Limit: 1,
	})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find node")
	}

	if len(nodes) == 0 {
		return nil, api.NewNotFoundError("node not found")
	}

	return &nodes[0], nil
}

func (h *Handler) processItems(
	ctx context.Context,
	node *domain.Node,
	serverDir string,
	req *chmodRequest,
) error {
	perm := uint32(req.Mode) //nolint:gosec // validateRequest guarantees 0..0o777 range

	for _, item := range req.Items {
		if err := validatePath(item.Path); err != nil {
			return api.WrapHTTPError(err, http.StatusBadRequest)
		}

		fullPath := filepath.Join(node.WorkPath, serverDir, item.Path)

		err := h.daemonFiles.Chmod(ctx, node, fullPath, perm)
		if err != nil {
			return errors.WithMessage(err, "failed to change file permissions")
		}
	}

	return nil
}

func validatePath(path string) error {
	if strings.Contains(path, "..") {
		return errors.New("path contains invalid directory traversal")
	}

	cleanPath := filepath.Clean(path)
	if strings.HasPrefix(cleanPath, "..") {
		return errors.New("path attempts to escape base directory")
	}

	return nil
}
