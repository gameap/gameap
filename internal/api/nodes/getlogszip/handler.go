package getlogszip

import (
	"archive/zip"
	"context"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

const (
	linuxLogPath   = "/var/log/gameap-daemon"
	windowsLogPath = "C:/gameap/daemon/logs"
)

type fileService interface {
	ReadDir(ctx context.Context, node *domain.Node, directory string) ([]*daemon.FileInfo, error)
	DownloadStream(ctx context.Context, node *domain.Node, filePath string) (io.ReadCloser, error)
}

type Handler struct {
	nodesRepo   repositories.NodeRepository
	daemonFiles fileService
	responder   base.Responder
}

func NewHandler(
	nodesRepo repositories.NodeRepository,
	daemonFiles fileService,
	responder base.Responder,
) *Handler {
	return &Handler{
		nodesRepo:   nodesRepo,
		daemonFiles: daemonFiles,
		responder:   responder,
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

	nodeID, err := input.ReadUint("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid node id"),
			http.StatusBadRequest,
		))

		return
	}

	filter := &filters.FindNode{
		IDs: []uint{nodeID},
	}

	nodes, err := h.nodesRepo.Find(ctx, filter, nil, &filters.Pagination{
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

	logPath := linuxLogPath
	if node.OS == domain.NodeOSWindows {
		logPath = windowsLogPath
	}

	fileInfos, err := h.daemonFiles.ReadDir(ctx, node, logPath)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to read log directory"))

		return
	}

	rw.Header().Set("Content-Type", "application/zip")
	rw.Header().Set("Content-Disposition", "attachment; filename=\"logs.zip\"")
	rw.WriteHeader(http.StatusOK)

	zipWriter := zip.NewWriter(rw)
	defer func() {
		if closeErr := zipWriter.Close(); closeErr != nil {
			// Log error but don't try to write response as headers are already sent
			slog.Warn("failed to close zip writer", "error", closeErr)
		}
	}()

	for _, fileInfo := range fileInfos {
		// Skip non-regular files. Skipping directories, symlinks, etc.
		if fileInfo.Type != daemon.FileTypeFile {
			continue
		}

		fullPath := filepath.Join(logPath, fileInfo.Name)

		fileReader, err := h.daemonFiles.DownloadStream(ctx, node, fullPath)
		if err != nil {
			continue
		}

		zipEntry, err := zipWriter.Create("daemon_logs/" + fileInfo.Name)
		if err != nil {
			_ = fileReader.Close()

			continue
		}

		_, err = io.Copy(zipEntry, fileReader)
		_ = fileReader.Close()

		if err != nil {
			continue
		}
	}
}
