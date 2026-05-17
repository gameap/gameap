package updatefile

import (
	"context"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gameap/gameap/internal/api/base"
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

const (
	maxMemory     = 32 << 20 // 32 MB
	defaultPerms  = 0o644
	maxUploadSize = 100 << 20 // 100 MB
)

var (
	errUserNotAuthenticated          = errors.New("user not authenticated")
	errNoFileUploaded                = errors.New("no file uploaded")
	errInvalidFileSize               = errors.New("invalid file size")
	errFilenameEmpty                 = errors.New("filename is empty")
	errFilenameContainsTraversal     = errors.New("filename contains invalid directory traversal")
	errFilenameContainsPathSeparator = errors.New("filename contains path separators")
)

type fileService interface {
	UploadStream(
		ctx context.Context,
		node *domain.Node,
		filePath string,
		r io.Reader,
		size uint64,
		perms os.FileMode,
		owner daemon.OwnerOptions,
	) error
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

	serverID, err := api.NewInputReader(r).ReadUint("server")
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

	r.Body = http.MaxBytesReader(rw, r.Body, maxUploadSize)
	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to parse multipart form"),
			http.StatusBadRequest,
		))

		return
	}

	fileHeader, path, err := h.parseFormData(r)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(err, http.StatusBadRequest))

		return
	}

	node, err := h.getNode(ctx, server.DSID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	response, err := h.updateFile(ctx, node, server, path, fileHeader)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	h.responder.Write(ctx, rw, response)
}

func (h *Handler) parseFormData(r *http.Request) (*multipart.FileHeader, string, error) {
	disk := r.FormValue("disk")
	path := r.FormValue("path")

	if disk != "server" {
		return nil, "", errors.Errorf("unsupported disk: %s, only 'server' disk is supported", disk)
	}

	if path == "" {
		path = "."
	}

	if err := filemanagerpath.ValidatePath(path); err != nil {
		return nil, "", err
	}

	fileHeaders := r.MultipartForm.File["file"]
	if len(fileHeaders) == 0 {
		return nil, "", errNoFileUploaded
	}

	fileHeader := fileHeaders[0]

	if fileHeader.Size > maxUploadSize {
		return nil, "", errors.Errorf("file exceeds maximum size of %d bytes", maxUploadSize)
	}

	if err := validateFilename(fileHeader.Filename); err != nil {
		return nil, "", err
	}

	return fileHeader, path, nil
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

func (h *Handler) updateFile(
	ctx context.Context,
	node *domain.Node,
	server *domain.Server,
	targetPath string,
	fileHeader *multipart.FileHeader,
) (updateFileResponse, error) {
	relativePath := filepath.Join(targetPath, fileHeader.Filename)
	fullPath := filepath.Join(node.WorkPath, server.Dir, relativePath)

	file, err := fileHeader.Open()
	if err != nil {
		return updateFileResponse{}, errors.WithMessage(err, "failed to open uploaded file")
	}
	defer func(file multipart.File) {
		err := file.Close()
		if err != nil {
			//nolint:gosec // G706: slog structured logging safely encodes values
			slog.Warn(
				"failed to close uploaded file",
				slog.String("error", err.Error()),
				slog.String("path", fullPath),
			)
		}
	}(file)

	if fileHeader.Size < 0 {
		return updateFileResponse{}, errInvalidFileSize
	}
	fileSize := uint64(fileHeader.Size)

	err = h.daemonFiles.UploadStream(
		ctx,
		node,
		fullPath,
		file,
		fileSize,
		defaultPerms,
		daemon.OwnerFromServer(server),
	)
	if err != nil {
		return updateFileResponse{}, errors.WithMessage(err, "failed to upload file")
	}

	fileInfo, err := h.daemonFiles.GetFileInfo(ctx, node, fullPath)
	if err != nil {
		return updateFileResponse{}, errors.WithMessage(err, "failed to get file info")
	}

	return newUpdateFileResponse(fileInfo, relativePath), nil
}

func validateFilename(filename string) error {
	if filename == "" {
		return errFilenameEmpty
	}

	if strings.Contains(filename, "..") {
		return errFilenameContainsTraversal
	}

	if strings.ContainsAny(filename, "/\\") {
		return errFilenameContainsPathSeparator
	}

	return nil
}
