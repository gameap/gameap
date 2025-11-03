package upload

import (
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"os"
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

const (
	maxMemory     = 32 << 20 // 32 MB
	defaultPerms  = 0o644
	maxUploadSize = 100 << 20 // 100 MB
)

var (
	errUserNotAuthenticated          = errors.New("user not authenticated")
	errNoFilesUploaded               = errors.New("no files uploaded")
	errInvalidFileSize               = errors.New("invalid file size")
	errPathContainsTraversal         = errors.New("path contains invalid directory traversal")
	errPathEscapesBaseDirectory      = errors.New("path attempts to escape base directory")
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
	) error
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

	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to parse multipart form"),
			http.StatusBadRequest,
		))

		return
	}

	disk := r.FormValue("disk")
	path := r.FormValue("path")
	overwriteStr := r.FormValue("overwrite")

	if disk != "server" {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.Errorf("unsupported disk: %s, only 'server' disk is supported", disk),
			http.StatusBadRequest,
		))

		return
	}

	_ = overwriteStr

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

	node, err := h.getNode(ctx, server.DSID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	files := r.MultipartForm.File["files[]"]
	if len(files) == 0 {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errNoFilesUploaded,
			http.StatusBadRequest,
		))

		return
	}

	err = h.processFiles(ctx, node, server.Dir, path, files)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	h.responder.Write(ctx, rw, newUploadResponse())
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

func (h *Handler) processFiles(
	ctx context.Context,
	node *domain.Node,
	serverDir string,
	targetPath string,
	files []*multipart.FileHeader,
) error {
	for _, fileHeader := range files {
		if fileHeader.Size > maxUploadSize {
			return api.WrapHTTPError(
				errors.Errorf("file %s exceeds maximum size of %d bytes", fileHeader.Filename, maxUploadSize),
				http.StatusBadRequest,
			)
		}

		if err := validateFilename(fileHeader.Filename); err != nil {
			return api.WrapHTTPError(err, http.StatusBadRequest)
		}

		fullPath := filepath.Join(serverDir, targetPath, fileHeader.Filename)

		file, err := fileHeader.Open()
		if err != nil {
			return errors.WithMessage(err, "failed to open uploaded file")
		}

		fileSize := uint64(fileHeader.Size)
		if fileHeader.Size < 0 {
			_ = file.Close()

			return errInvalidFileSize
		}

		err = h.daemonFiles.UploadStream(
			ctx,
			node,
			fullPath,
			file,
			fileSize,
			defaultPerms,
		)

		_ = file.Close()

		if err != nil {
			return errors.WithMessagef(err, "failed to upload file %s", fileHeader.Filename)
		}
	}

	return nil
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
