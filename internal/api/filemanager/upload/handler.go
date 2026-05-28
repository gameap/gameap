package upload

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/api/filemanager/filemanagermime"
	"github.com/gameap/gameap/internal/api/filemanager/filemanagerpath"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	"github.com/gameap/gameap/internal/audit"
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

	// mimeSniffWindow is the number of bytes inspected by
	// http.DetectContentType. The HTTP spec mandates 512; reading more
	// neither helps the detector nor matches browser sniffing rules.
	mimeSniffWindow = 512
)

var (
	errUserNotAuthenticated = errors.New("user not authenticated")
	errNoFilesUploaded      = errors.New("no files uploaded")
	errInvalidFileSize      = errors.New("invalid file size")
	errUnsupportedMediaType = errors.New("file content type is not on the upload allowlist")
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
}

type Handler struct {
	serverFinder   *serversbase.ServerFinder
	abilityChecker *serversbase.AbilityChecker
	nodeRepo       repositories.NodeRepository
	daemonFiles    fileService
	mimeChecker    *filemanagermime.Checker
	responder      base.Responder
	audit          audit.Logger
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	nodeRepo repositories.NodeRepository,
	rbac base.RBAC,
	daemonFiles fileService,
	mimeChecker *filemanagermime.Checker,
	responder base.Responder,
	auditLogger audit.Logger,
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	// Defence-in-depth: a caller that passes nil for the MIME checker
	// receives the default (deny-by-default) configuration so an
	// integration regression cannot accidentally ship an open upload.
	if mimeChecker == nil {
		mimeChecker = filemanagermime.NewChecker(filemanagermime.Config{})
	}

	return &Handler{
		serverFinder:   serversbase.NewServerFinder(serverRepo, rbac),
		abilityChecker: serversbase.NewAbilityChecker(rbac),
		nodeRepo:       nodeRepo,
		daemonFiles:    daemonFiles,
		mimeChecker:    mimeChecker,
		responder:      responder,
		audit:          auditLogger,
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

	files := r.MultipartForm.File["files[]"]
	if len(files) == 0 {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errNoFilesUploaded,
			http.StatusBadRequest,
		))

		return
	}

	err = h.processFiles(ctx, node, server, path, files)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	audit.SensitiveOp(ctx, h.audit, audit.EventFileUpload, audit.CategoryFileOp,
		"server", strconv.FormatUint(uint64(serverID), 10), "upload",
		slog.Int("files", len(files)))

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
	server *domain.Server,
	targetPath string,
	files []*multipart.FileHeader,
) error {
	owner := daemon.OwnerFromServer(server)

	for _, fileHeader := range files {
		if fileHeader.Size > maxUploadSize {
			return api.WrapHTTPError(
				errors.Errorf("file %s exceeds maximum size of %d bytes", fileHeader.Filename, maxUploadSize),
				http.StatusBadRequest,
			)
		}

		if err := filemanagerpath.ValidateFilename(fileHeader.Filename); err != nil {
			return api.WrapHTTPError(err, http.StatusBadRequest)
		}

		fullPath := filepath.Join(node.WorkPath, server.Dir, targetPath, fileHeader.Filename)

		file, err := fileHeader.Open()
		if err != nil {
			return errors.WithMessage(err, "failed to open uploaded file")
		}

		if fileHeader.Size < 0 {
			_ = file.Close()

			return errInvalidFileSize
		}
		fileSize := uint64(fileHeader.Size)

		// Sniff the first 512 bytes to determine the real MIME (the
		// client-declared Content-Type and the extension are NOT
		// trusted — a malicious upload of <html>… renamed to "logo.png"
		// would otherwise sail through). We then prepend the sniffed
		// bytes back onto the read stream so UploadStream sees the
		// full file. Done before any network IO so a reject is cheap.
		buffered := bufio.NewReaderSize(file, mimeSniffWindow)
		sniff, sniffErr := buffered.Peek(mimeSniffWindow)
		if sniffErr != nil && !errors.Is(sniffErr, io.EOF) {
			_ = file.Close()

			return errors.WithMessage(sniffErr, "failed to read upload header for MIME detection")
		}

		detected := http.DetectContentType(sniff)
		bare, allowed := h.mimeChecker.Allowed(detected)
		if !allowed {
			_ = file.Close()
			audit.SensitiveOp(ctx, h.audit, audit.EventFileUpload, audit.CategoryFileOp,
				"server", strconv.FormatUint(uint64(server.ID), 10), "upload_rejected",
				slog.String("filename", fileHeader.Filename),
				slog.String("detected_mime", bare),
				slog.String("reason", "mime_not_allowed"))

			return api.WrapHTTPError(
				errors.Wrapf(errUnsupportedMediaType,
					"file %s rejected: detected MIME %q is not allowed", fileHeader.Filename, bare),
				http.StatusUnsupportedMediaType,
			)
		}

		err = h.daemonFiles.UploadStream(
			ctx,
			node,
			fullPath,
			// io.MultiReader keeps the sniffed bytes available — they
			// were Peek'ed, not Read, but the bufio reader can be
			// drained either way. We use buffered directly so the
			// remaining Read calls go through the same reader.
			buffered,
			fileSize,
			defaultPerms,
			owner,
		)

		_ = file.Close()

		if err != nil {
			return errors.WithMessagef(err, "failed to upload file %s", fileHeader.Filename)
		}
	}

	return nil
}
