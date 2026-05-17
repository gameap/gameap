package downloadarchive

import (
	"context"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/api/filemanager/filemanagerpath"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/internal/services/filemanager/archiver"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/pkg/errors"
)

var (
	errUserNotAuthenticated = errors.New("user not authenticated")
	errDiskRequired         = errors.New("disk parameter is required")
	errPathRequired         = errors.New("path parameter is required")
	errInvalidCompress      = errors.New("compress must be an integer between 0 and 9")
)

type Archiver interface {
	BuildManifest(
		ctx context.Context, node *domain.Node, rootAbsPath string, limits archiver.Limits,
	) (*archiver.Manifest, error)
	WriteArchive(
		ctx context.Context, w io.Writer, node *domain.Node, manifest *archiver.Manifest, opts archiver.Options,
	) (*archiver.Result, error)
}

type ConcurrencyGuard interface {
	Acquire(ctx context.Context, serverID uint) (release func(), err error)
}

type Limits struct {
	MaxTotalBytes uint64
	MaxFiles      uint32
}

type Handler struct {
	serverFinder   *serversbase.ServerFinder
	abilityChecker *serversbase.AbilityChecker
	nodeRepo       repositories.NodeRepository
	archiver       Archiver
	guard          ConcurrencyGuard
	limits         Limits
	responder      base.Responder
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	nodeRepo repositories.NodeRepository,
	rbac base.RBAC,
	archive Archiver,
	guard ConcurrencyGuard,
	limits Limits,
	responder base.Responder,
) *Handler {
	return &Handler{
		serverFinder:   serversbase.NewServerFinder(serverRepo, rbac),
		abilityChecker: serversbase.NewAbilityChecker(rbac),
		nodeRepo:       nodeRepo,
		archiver:       archive,
		guard:          guard,
		limits:         limits,
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

	compressLevel, err := readCompressLevel(r)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(err, http.StatusBadRequest))

		return
	}

	node, err := h.getNode(ctx, server.DSID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	fullPath := filepath.Join(node.WorkPath, server.Dir, path)

	manifest, err := h.archiver.BuildManifest(ctx, node, fullPath, archiver.Limits{
		MaxTotalBytes: h.limits.MaxTotalBytes,
		MaxFiles:      h.limits.MaxFiles,
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, mapManifestError(err))

		return
	}

	release, err := h.guard.Acquire(ctx, server.ID)
	if err != nil {
		if errors.Is(err, archiver.ErrTooManyConcurrent) {
			h.responder.WriteError(ctx, rw, api.WrapHTTPError(
				err,
				http.StatusTooManyRequests,
			))

			return
		}
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "acquire archive slot"))

		return
	}
	defer release()

	rc := http.NewResponseController(rw)
	if deadlineErr := rc.SetWriteDeadline(time.Time{}); deadlineErr != nil {
		slog.WarnContext(ctx, "failed to disable write deadline", slog.String("error", deadlineErr.Error()))
	}

	filename := archiveFilename(manifest.RootName, path)
	rw.Header().Set("Content-Type", "application/zip")
	rw.Header().Set("Content-Disposition", contentDispositionHeader(filename))
	rw.Header().Set("X-Archive-Total-Bytes", strconv.FormatUint(manifest.TotalSize, 10))
	rw.Header().Set("X-Archive-Total-Files", strconv.FormatUint(uint64(manifest.TotalFiles), 10))
	rw.Header().Set("X-Archive-Skipped-Count", strconv.Itoa(len(manifest.Skipped)))
	rw.Header().Set("Cache-Control", "no-store")

	_, err = h.archiver.WriteArchive(ctx, rw, node, manifest, archiver.Options{
		CompressLevel: compressLevel,
	})
	if err != nil {
		slog.ErrorContext(
			ctx,
			"failed to write archive",
			slog.String("error", err.Error()),
			slog.Uint64("server_id", uint64(server.ID)),
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

func readCompressLevel(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("compress"))
	if raw == "" {
		return 0, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 || value > 9 {
		return 0, errInvalidCompress
	}

	return value, nil
}

func archiveFilename(rootName, requestedPath string) string {
	if rootName != "" && rootName != "." && rootName != "/" {
		return rootName + ".zip"
	}

	base := filepath.Base(requestedPath)
	if base == "" || base == "." || base == "/" {
		return "archive.zip"
	}

	return base + ".zip"
}

func contentDispositionHeader(filename string) string {
	asciiSafe := stripNonASCII(filename)
	encoded := url.PathEscape(filename)

	return mime.FormatMediaType("attachment", map[string]string{
		"filename":  asciiSafe,
		"filename*": "UTF-8''" + encoded,
	})
}

func stripNonASCII(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x80 && r != '"' && r != '\\' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "archive.zip"
	}

	return out
}

func mapManifestError(err error) error {
	switch {
	case errors.Is(err, archiver.ErrTooLarge):
		return api.WrapHTTPError(err, http.StatusRequestEntityTooLarge)
	case errors.Is(err, archiver.ErrTooManyFiles):
		return api.WrapHTTPError(err, http.StatusRequestEntityTooLarge)
	case errors.Is(err, archiver.ErrEmptyManifest):
		return api.WrapHTTPError(err, http.StatusNotFound)
	case errors.Is(err, archiver.ErrNotADirectory):
		return api.WrapHTTPError(err, http.StatusBadRequest)
	default:
		return errors.WithMessage(err, "build manifest")
	}
}
