package streamfile

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/api/filemanager/filemanagerhttp"
	"github.com/gameap/gameap/internal/api/filemanager/filemanagerpath"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

var (
	errUserNotAuthenticated = errors.New("user not authenticated")
	errDiskRequired         = errors.New("disk parameter is required")
	errPathRequired         = errors.New("path parameter is required")
)

type fileService interface {
	DownloadStream(ctx context.Context, node *domain.Node, filePath string) (io.ReadCloser, error)
	GetFileInfo(ctx context.Context, node *domain.Node, path string) (*daemon.FileDetails, error)
}

type Handler struct {
	serverFinder   *serversbase.ServerFinder
	abilityChecker *serversbase.AbilityChecker
	nodeRepo       repositories.NodeRepository
	daemonFiles    fileService
	responder      base.Responder
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	nodeRepo repositories.NodeRepository,
	rbac base.RBAC,
	daemonFiles fileService,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverFinder:   serversbase.NewServerFinder(serverRepo, rbac),
		abilityChecker: serversbase.NewAbilityChecker(rbac),
		nodeRepo:       nodeRepo,
		daemonFiles:    daemonFiles,
		responder:      responder,
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

	if err = filemanagerpath.ValidatePath(path); err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			err,
			http.StatusBadRequest,
		))

		return
	}

	node, err := h.getNode(ctx, server.DSID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	fullPath := filepath.Join(node.WorkPath, server.Dir, path)

	fileInfo, err := h.daemonFiles.GetFileInfo(ctx, node, fullPath)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to get file info"))

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

	rc := http.NewResponseController(rw)
	if deadlineErr := rc.SetWriteDeadline(time.Time{}); deadlineErr != nil {
		slog.WarnContext(ctx, "failed to disable write deadline", slog.String("error", deadlineErr.Error()))
	}

	filemanagerhttp.SafeContentHeaders(rw.Header(), filename, fileInfo.Mime)
	rw.Header().Set("Accept-Ranges", "bytes")

	if fileInfo.Size > 0 {
		rw.Header().Set("Content-Length", strconv.FormatUint(fileInfo.Size, 10))
	}

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
