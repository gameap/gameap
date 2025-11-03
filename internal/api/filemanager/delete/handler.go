package deletefiles

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gameap/gameap/internal/api/base"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

type fileService interface {
	Remove(ctx context.Context, node *domain.Node, path string, recursive bool) error
}

type Handler struct {
	serverFinder *serversbase.ServerFinder
	nodeRepo     repositories.NodeRepository
	daemonFiles  fileService
	responder    base.Responder
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	nodeRepo repositories.NodeRepository,
	rbac base.RBAC,
	daemonFiles fileService,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverFinder: serversbase.NewServerFinder(serverRepo, rbac),
		nodeRepo:     nodeRepo,
		daemonFiles:  daemonFiles,
		responder:    responder,
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

	// Parse and validate request
	var req deleteRequest
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

	// Get node
	node, err := h.getNode(ctx, server.DSID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	// Process each item
	if err = h.processItems(ctx, node, server.Dir, req.Items); err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	h.responder.Write(ctx, rw, newDeleteResponse())
}

func (h *Handler) validateRequest(req *deleteRequest) error {
	if req.Disk != "server" {
		return errors.Errorf("unsupported disk: %s, only 'server' disk is supported", req.Disk)
	}

	if len(req.Items) == 0 {
		return errors.New("items array is empty")
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
	items []deleteItem,
) error {
	for _, item := range items {
		if err := validatePath(item.Path); err != nil {
			return api.WrapHTTPError(err, http.StatusBadRequest)
		}

		fullPath := filepath.Join(serverDir, item.Path)
		recursive := item.Type == "dir"

		err := h.daemonFiles.Remove(ctx, node, fullPath, recursive)
		if err != nil {
			return errors.WithMessage(err, "failed to delete file or directory")
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
