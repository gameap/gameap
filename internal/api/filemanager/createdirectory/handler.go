package createdirectory

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

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

type fileService interface {
	MkDir(ctx context.Context, node *domain.Node, directory string) error
	GetFileInfo(ctx context.Context, node *domain.Node, path string) (*daemon.FileDetails, error)
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

	var req createDirectoryRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request body"),
			http.StatusBadRequest,
		))

		return
	}

	if err = req.Validate(); err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(err, http.StatusBadRequest))

		return
	}

	node, err := h.getNode(ctx, server.DSID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	response, err := h.createDirectory(ctx, node, server.Dir, &req)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	h.responder.Write(ctx, rw, response)
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

func (h *Handler) createDirectory(
	ctx context.Context,
	node *domain.Node,
	serverDir string,
	req *createDirectoryRequest,
) (createDirectoryResponse, error) {
	if err := validatePath(req.Path); err != nil {
		return createDirectoryResponse{}, api.WrapHTTPError(err, http.StatusBadRequest)
	}

	if err := validatePath(req.Name); err != nil {
		return createDirectoryResponse{}, api.WrapHTTPError(err, http.StatusBadRequest)
	}

	relativePath := filepath.Join(req.Path, req.Name)
	fullPath := filepath.Join(serverDir, relativePath)

	err := h.daemonFiles.MkDir(ctx, node, fullPath)
	if err != nil {
		return createDirectoryResponse{}, errors.WithMessage(err, "failed to create directory")
	}

	fileInfo, err := h.daemonFiles.GetFileInfo(ctx, node, fullPath)
	if err != nil {
		return createDirectoryResponse{}, errors.WithMessage(err, "failed to get directory info")
	}

	return newCreateDirectoryResponse(fileInfo, relativePath), nil
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
