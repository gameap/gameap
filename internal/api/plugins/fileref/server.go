// Package fileref streams node files that plugins reference from their HTTP
// responses (HTTPResponse.file): the bytes go from the daemon straight to the
// client without passing through the plugin's memory or the 1 MB response
// limit. The operation is gated on the plugin's "files" grant, the same one
// that guards gameap-nodefs.
package fileref

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/api/filemanager/filemanagerhttp"
	"github.com/gameap/gameap/internal/api/filemanager/filemanagerpath"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/daemon"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	pkgplugin "github.com/gameap/gameap/pkg/plugin"
	"github.com/gameap/gameap/pkg/plugin/proto"
	"github.com/pkg/errors"
)

var (
	errRefRequired             = errors.New("file reference is required")
	errNodeIDRequired          = errors.New("node id is required")
	errPathRequired            = errors.New("file path is required")
	errNodeNotFound            = errors.New("node not found")
	errPathIsDirectory         = errors.New("path is a directory")
	errFilesPermissionRequired = errors.New("plugin permission " + string(domain.PluginPermissionFiles) + " required")
	errAuthenticationRequired  = errors.New("authentication required for file responses")
)

// allowedPluginHeaders is the only set of plugin-provided headers that
// reaches the client. The response is served from the panel origin to the
// admin's browser, so anything that could act on that origin (Set-Cookie,
// Location, WWW-Authenticate, CSP, …) is dropped; the panel owns
// Content-Length, Content-Disposition and the content-sniffing headers. An
// allowlist (not a denylist) is deliberate, as in the gameap-http host
// library: a header invented next year defaults to "stripped".
var allowedPluginHeaders = map[string]struct{}{
	"Content-Type":     {},
	"Content-Language": {},
	"Cache-Control":    {},
	"Expires":          {},
	"Pragma":           {},
	"Last-Modified":    {},
	"Etag":             {},
	"Vary":             {},
}

// customHeaderPrefix admits plugin metadata headers (X-Plugin-Version and
// the like); the panel's own X-Content-Type-Options is set afterwards.
const customHeaderPrefix = "X-"

type Server struct {
	files   FileService
	nodes   repositories.NodeRepository
	checker PermissionChecker
	audit   audit.Logger
}

func NewServer(
	files FileService,
	nodes repositories.NodeRepository,
	checker PermissionChecker,
	auditLogger audit.Logger,
) *Server {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &Server{
		files:   files,
		nodes:   nodes,
		checker: checker,
		audit:   auditLogger,
	}
}

// ServeFileRef implements pkgplugin.FileRefServer. Errors are returned only
// before the first byte is written; a transfer that breaks midway is logged.
func (s *Server) ServeFileRef(w http.ResponseWriter, r *http.Request, req pkgplugin.FileRefRequest) error {
	ctx := r.Context()

	if err := validateRef(req.Ref); err != nil {
		return api.WrapHTTPError(err, http.StatusBadRequest)
	}

	// Node files never go to anonymous clients, whatever the plugin route's
	// own auth setting says: the panel's file endpoints require a session
	// too, and a plugin must not be able to relax that.
	if !auth.SessionFromContext(ctx).IsAuthenticated() {
		return api.WrapHTTPError(errAuthenticationRequired, http.StatusUnauthorized)
	}

	if err := s.authorize(ctx, req); err != nil {
		return err
	}

	node, err := s.findNode(ctx, req.Ref.NodeId)
	if err != nil {
		return err
	}

	info, err := s.files.GetFileInfo(ctx, node, req.Ref.Path)
	if err != nil {
		return errors.WithMessage(err, "failed to stat file")
	}

	if info == nil {
		return api.WrapHTTPError(daemon.ErrFileNotFound, http.StatusNotFound)
	}

	if info.Type == daemon.FileTypeDir {
		return api.WrapHTTPError(errPathIsDirectory, http.StatusBadRequest)
	}

	stream, err := s.files.DownloadStream(ctx, node, req.Ref.Path)
	if err != nil {
		return errors.WithMessage(err, "failed to open file stream")
	}
	defer func() {
		if closeErr := stream.Close(); closeErr != nil {
			slog.WarnContext(ctx, "failed to close plugin file stream", slog.String("error", closeErr.Error()))
		}
	}()

	s.stream(w, r, req, info, stream)

	return nil
}

// stream writes the headers and copies the file. Nothing can be reported to
// the client once the headers are out; a broken transfer is logged and the
// connection is what tells the client.
func (s *Server) stream(
	w http.ResponseWriter,
	r *http.Request,
	req pkgplugin.FileRefRequest,
	info *daemon.FileDetails,
	stream io.Reader,
) {
	ctx := r.Context()

	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		slog.WarnContext(ctx, "failed to disable write deadline", slog.String("error", err.Error()))
	}

	applyHeaders(w.Header(), req, info)

	status := req.StatusCode
	if status == 0 {
		status = http.StatusOK
	}

	w.WriteHeader(status)

	slog.InfoContext(ctx, "plugin file response served",
		slog.Uint64("plugin_id", req.PluginID),
		slog.String("plugin", req.PluginName),
		slog.Uint64("node_id", req.Ref.NodeId),
		slog.String("path", req.Ref.Path),
		slog.Uint64("size", info.Size),
		slog.Uint64("user_id", requestUserID(ctx)),
	)

	if _, err := io.Copy(w, stream); err != nil {
		slog.ErrorContext(ctx, "failed to write plugin file response",
			slog.Uint64("plugin_id", req.PluginID),
			slog.String("path", req.Ref.Path),
			slog.String("error", err.Error()))
	}
}

// applyHeaders copies the plugin headers the allowlist admits and then
// sets the ones the panel owns: the file is always an attachment with the
// panel's length.
func applyHeaders(header http.Header, req pkgplugin.FileRefRequest, info *daemon.FileDetails) {
	for name, value := range req.Headers {
		if pluginHeaderAllowed(name) {
			header.Set(name, value)
		}
	}

	filemanagerhttp.AttachmentContentHeaders(header, attachmentName(req.Ref), header.Get("Content-Type"))
	header.Set("Accept-Ranges", "none")

	if header.Get("Cache-Control") == "" {
		header.Set("Cache-Control", "no-store")
	}

	if info.Size > 0 {
		header.Set("Content-Length", strconv.FormatUint(info.Size, 10))
	}
}

func pluginHeaderAllowed(name string) bool {
	canonical := http.CanonicalHeaderKey(name)
	if _, ok := allowedPluginHeaders[canonical]; ok {
		return true
	}

	return strings.HasPrefix(canonical, customHeaderPrefix) && canonical != "X-Content-Type-Options"
}

func attachmentName(ref *proto.FileRef) string {
	if ref.Filename != "" {
		return ref.Filename
	}

	return path.Base(strings.ReplaceAll(ref.Path, "\\", "/"))
}

func validateRef(ref *proto.FileRef) error {
	switch {
	case ref == nil:
		return errRefRequired
	case ref.NodeId == 0:
		return errNodeIDRequired
	case ref.Path == "":
		return errPathRequired
	}

	return filemanagerpath.ValidatePath(ref.Path)
}

// authorize checks the plugin's "files" grant before any daemon round trip:
// a plugin without it must not learn whether a path exists. Denials are
// audited like every other authorization failure.
func (s *Server) authorize(ctx context.Context, req pkgplugin.FileRefRequest) error {
	allowed, err := s.checker.Has(ctx, req.PluginID, domain.PluginPermissionFiles)
	if err != nil {
		return errors.WithMessage(err, "failed to check plugin permission")
	}

	if allowed {
		return nil
	}

	pluginID := strconv.FormatUint(req.PluginID, 10)

	slog.WarnContext(ctx, "plugin file response denied: missing permission",
		slog.String("plugin_id", pluginID),
		slog.String("plugin", req.PluginName),
		slog.String("permission", string(domain.PluginPermissionFiles)),
		slog.String("path", req.Ref.Path))

	audit.AccessDenied(ctx, s.audit, "plugin", pluginID, "plugin_permission_missing",
		slog.String("permission", string(domain.PluginPermissionFiles)),
		slog.String("action", "serve_file"))

	return api.WrapHTTPError(errFilesPermissionRequired, http.StatusForbidden)
}

func (s *Server) findNode(ctx context.Context, nodeID uint64) (*domain.Node, error) {
	nodes, err := s.nodes.Find(ctx, filters.FindNodeByIDs(uint(nodeID)), nil, &filters.Pagination{Limit: 1})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find node")
	}

	if len(nodes) == 0 {
		return nil, api.WrapHTTPError(errNodeNotFound, http.StatusNotFound)
	}

	return &nodes[0], nil
}

// requestUserID is the authenticated user behind the request (ServeFileRef
// refuses anonymous requests before this point).
func requestUserID(ctx context.Context) uint64 {
	session := auth.SessionFromContext(ctx)
	if !session.IsAuthenticated() {
		return 0
	}

	return uint64(session.User.ID)
}
