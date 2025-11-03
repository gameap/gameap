package tree

import (
	"context"
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
	ReadDir(ctx context.Context, node *domain.Node, directory string) ([]*daemon.FileInfo, error)
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

//nolint:funlen
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

	disk := r.URL.Query().Get("disk")
	if disk == "" {
		disk = "server"
	}

	if disk != "server" {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.Errorf("unsupported disk: %s, only 'server' disk is supported", disk),
			http.StatusBadRequest,
		))

		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		path = "."
	}

	if err = validatePath(path); err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			err,
			http.StatusBadRequest,
		))

		return
	}

	fullPath := filepath.Join(server.Dir, path)

	nodes, err := h.nodeRepo.Find(ctx, &filters.FindNode{
		IDs: []uint{server.DSID},
	}, nil, &filters.Pagination{
		Limit: 1,
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to find node"))

		return
	}

	if len(nodes) == 0 {
		h.responder.WriteError(ctx, rw, api.NewNotFoundError("node not found"))

		return
	}

	node := &nodes[0]

	fileInfoList, err := h.daemonFiles.ReadDir(ctx, node, fullPath)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to read directory"))

		return
	}

	directories := make([]directoryTreeItem, 0)
	for _, fileInfo := range fileInfoList {
		if fileInfo.Type != daemon.FileTypeDir {
			continue
		}

		hasSubdirectories, err := h.checkHasSubdirectories(ctx, node, filepath.Join(fullPath, fileInfo.Name))
		if err != nil {
			h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to check subdirectories"))

			return
		}

		directories = append(directories, directoryTreeItem{
			Path:      fileInfo.Name,
			Timestamp: fileInfo.TimeModified,
			Type:      "dir",
			Dirname:   extractDirname(fileInfo.Name),
			Basename:  filepath.Base(fileInfo.Name),
			Props: directoryProps{
				HasSubdirectories: hasSubdirectories,
			},
		})
	}

	h.responder.Write(ctx, rw, newTreeResponse(directories))
}

func (h *Handler) checkHasSubdirectories(ctx context.Context, node *domain.Node, directory string) (bool, error) {
	fileInfoList, err := h.daemonFiles.ReadDir(ctx, node, directory)
	if err != nil {
		return false, errors.WithMessage(err, "failed to read directory")
	}

	for _, fileInfo := range fileInfoList {
		if fileInfo.Type == daemon.FileTypeDir {
			return true, nil
		}
	}

	return false, nil
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

func extractDirname(path string) string {
	dirname := filepath.Dir(path)
	if dirname == "." {
		return ""
	}

	return dirname
}
