package createarchive

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/api/filemanager/filemanagerpath"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
)

const (
	minCompressionLevel = 0
	maxCompressionLevel = 9
)

// createExtensions is checked longest-first so "backup.tar.gz" resolves to
// tar_gz, not gz.
var createExtensions = []struct {
	suffix string
	format proto.ArchiveFormat
}{
	{".tar.gz", proto.ArchiveFormat_ARCHIVE_FORMAT_TAR_GZ},
	{".tar.bz2", proto.ArchiveFormat_ARCHIVE_FORMAT_TAR_BZ2},
	{".tar.xz", proto.ArchiveFormat_ARCHIVE_FORMAT_TAR_XZ},
	{".tar.zst", proto.ArchiveFormat_ARCHIVE_FORMAT_TAR_ZSTD},
	{".tgz", proto.ArchiveFormat_ARCHIVE_FORMAT_TAR_GZ},
	{".txz", proto.ArchiveFormat_ARCHIVE_FORMAT_TAR_XZ},
	{".zip", proto.ArchiveFormat_ARCHIVE_FORMAT_ZIP},
	{".tar", proto.ArchiveFormat_ARCHIVE_FORMAT_TAR},
	{".gz", proto.ArchiveFormat_ARCHIVE_FORMAT_GZ},
	{".bz2", proto.ArchiveFormat_ARCHIVE_FORMAT_BZ2},
	{".xz", proto.ArchiveFormat_ARCHIVE_FORMAT_XZ},
	{".zst", proto.ArchiveFormat_ARCHIVE_FORMAT_ZSTD},
}

// singleFileFormats compress exactly one file and carry no directory
// structure.
var singleFileFormats = map[proto.ArchiveFormat]bool{
	proto.ArchiveFormat_ARCHIVE_FORMAT_GZ:   true,
	proto.ArchiveFormat_ARCHIVE_FORMAT_BZ2:  true,
	proto.ArchiveFormat_ARCHIVE_FORMAT_XZ:   true,
	proto.ArchiveFormat_ARCHIVE_FORMAT_ZSTD: true,
}

var extractOnlyFormats = map[proto.ArchiveFormat]bool{
	proto.ArchiveFormat_ARCHIVE_FORMAT_7Z:  true,
	proto.ArchiveFormat_ARCHIVE_FORMAT_RAR: true,
}

type Handler struct {
	serverFinder   *serversbase.ServerFinder
	abilityChecker *serversbase.AbilityChecker
	nodeRepo       repositories.NodeRepository
	daemonArchive  archiveStarter
	responder      base.Responder
	audit          audit.Logger
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	nodeRepo repositories.NodeRepository,
	rbac base.RBAC,
	daemonArchive archiveStarter,
	responder base.Responder,
	auditLogger audit.Logger,
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &Handler{
		serverFinder:   serversbase.NewServerFinder(serverRepo, rbac),
		abilityChecker: serversbase.NewAbilityChecker(rbac),
		nodeRepo:       nodeRepo,
		daemonArchive:  daemonArchive,
		responder:      responder,
		audit:          auditLogger,
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

	var req createArchiveRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request body"),
			http.StatusBadRequest,
		))

		return
	}

	format, err := h.validateRequest(&req)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(err, http.StatusBadRequest))

		return
	}

	node, err := h.getNode(ctx, server.DSID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	operationID, err := h.daemonArchive.StartCreate(ctx, node, buildParams(node, server, &req, format, session.User.ID))
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	audit.SensitiveOp(ctx, h.audit, audit.EventFileArchiveCreate, audit.CategoryFileOp,
		"server", strconv.FormatUint(uint64(serverID), 10), "archive.create",
		slog.String("operation_id", operationID),
		slog.String("format", daemon.ArchiveFormatToAPIName(format)),
		slog.Int("sources", len(req.Sources)))

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusAccepted)
	h.responder.Write(ctx, rw, createArchiveResponse{OperationID: operationID})
}

func buildParams(
	node *domain.Node,
	server *domain.Server,
	req *createArchiveRequest,
	format proto.ArchiveFormat,
	userID uint,
) daemon.CreateArchiveParams {
	root := filepath.Join(node.WorkPath, server.Dir)
	sources := make([]string, 0, len(req.Sources))
	for _, src := range req.Sources {
		sources = append(sources, filepath.Join(root, src))
	}

	return daemon.CreateArchiveParams{
		ArchivePath:      filepath.Join(root, req.Path, req.Name),
		BasePath:         filepath.Join(root, req.Path),
		Sources:          sources,
		Format:           format,
		CompressionLevel: req.CompressionLevel,
		Overwrite:        req.Overwrite,
		Owner:            daemon.OwnerFromServer(server),
		Options: daemon.ArchiveStartOptions{
			ServerID:  server.ID,
			Initiator: "user:" + strconv.FormatUint(uint64(userID), 10),
		},
	}
}

func (h *Handler) validateRequest(req *createArchiveRequest) (proto.ArchiveFormat, error) {
	if req.Disk != "server" {
		return 0, errors.Errorf("unsupported disk: %s, only 'server' disk is supported", req.Disk)
	}

	if err := filemanagerpath.ValidatePath(req.Path); err != nil {
		return 0, err
	}

	if err := filemanagerpath.ValidateFilename(req.Name); err != nil {
		return 0, err
	}

	format, err := resolveFormat(req.Format, req.Name)
	if err != nil {
		return 0, err
	}

	if len(req.Sources) == 0 {
		return 0, errors.New("sources array is empty")
	}

	if singleFileFormats[format] && len(req.Sources) != 1 {
		return 0, errors.Errorf(
			"format %s compresses a single file, %d sources given",
			daemon.ArchiveFormatToAPIName(format), len(req.Sources),
		)
	}

	basePath := normalizeRel(req.Path)
	for _, src := range req.Sources {
		if err := filemanagerpath.ValidatePath(src); err != nil {
			return 0, err
		}
		if filemanagerpath.IsRoot(src) {
			return 0, filemanagerpath.ErrPathIsRoot
		}
		if !isWithinBase(basePath, normalizeRel(src)) {
			return 0, errors.Errorf("source %q is outside the base path %q", src, req.Path)
		}
	}

	if req.CompressionLevel != nil &&
		(*req.CompressionLevel < minCompressionLevel || *req.CompressionLevel > maxCompressionLevel) {
		return 0, errors.Errorf("invalid compression level: %d, must be between 0 and 9", *req.CompressionLevel)
	}

	return format, nil
}

func resolveFormat(explicit, name string) (proto.ArchiveFormat, error) {
	if explicit != "" {
		format, ok := daemon.ArchiveFormatFromAPIName(explicit)
		if !ok {
			return 0, errors.Errorf("unknown archive format: %s", explicit)
		}
		if extractOnlyFormats[format] {
			return 0, errors.Errorf("format %s supports extraction only", explicit)
		}

		return format, nil
	}

	lowered := strings.ToLower(name)
	for _, ext := range createExtensions {
		if strings.HasSuffix(lowered, ext.suffix) && len(lowered) > len(ext.suffix) {
			return ext.format, nil
		}
	}

	return 0, errors.Errorf("cannot infer archive format from name: %s", name)
}

func normalizeRel(p string) string {
	return path.Clean(strings.Trim(strings.ReplaceAll(p, "\\", "/"), "/"))
}

// isWithinBase reports whether the normalized relative path src lives inside
// base ("." = server root). Entry names inside the archive are stored
// relative to the base path, so sources outside it have no representable
// name.
func isWithinBase(base, src string) bool {
	if base == "." {
		return true
	}

	return strings.HasPrefix(src+"/", base+"/") && src != base
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
