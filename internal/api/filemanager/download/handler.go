package download

import (
	"context"
	"io"
	"log/slog"
	"mime"
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

var (
	errUserNotAuthenticated     = errors.New("user not authenticated")
	errDiskRequired             = errors.New("disk parameter is required")
	errPathRequired             = errors.New("path parameter is required")
	errPathContainsTraversal    = errors.New("path contains invalid directory traversal")
	errPathEscapesBaseDirectory = errors.New("path attempts to escape base directory")
)

type fileService interface {
	DownloadStream(ctx context.Context, node *domain.Node, filePath string) (io.ReadCloser, error)
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
			errUserNotAuthenticated,
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
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errDiskRequired,
			http.StatusBadRequest,
		))

		return
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
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errPathRequired,
			http.StatusBadRequest,
		))

		return
	}

	if err = validatePath(path); err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			err,
			http.StatusBadRequest,
		))

		return
	}

	fullPath := filepath.Join(server.Dir, path)

	node, err := h.getNode(ctx, server.DSID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	fileStream, err := h.daemonFiles.DownloadStream(ctx, node, fullPath)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to download file"))

		return
	}
	defer func() {
		closeErr := fileStream.Close()
		if closeErr != nil {
			slog.WarnContext(
				ctx,
				"failed to close file stream",
				slog.String("error", closeErr.Error()),
			)
		}
	}()

	filename := filepath.Base(path)
	contentType := getContentType(filename)

	rw.Header().Set("Content-Type", contentType)
	rw.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")

	_, err = io.Copy(rw, fileStream)
	if err != nil {
		slog.ErrorContext(
			ctx,
			"failed to write file to response",
			slog.String("error", err.Error()),
		)
	}
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

func validatePath(path string) error {
	if strings.Contains(path, "..") {
		return errPathContainsTraversal
	}

	cleanPath := filepath.Clean(path)
	if strings.HasPrefix(cleanPath, "..") {
		return errPathEscapesBaseDirectory
	}

	return nil
}

func getContentType(filename string) string {
	ext := filepath.Ext(filename)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return contentType
}
