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
	"math"
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
	"github.com/gameap/gameap/internal/plugin/hostlibrary"
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
	errFilesPermissionRequired = errors.New("plugin permission " + string(domain.PluginPermissionFilesRead) + " required")
	errAuthenticationRequired  = errors.New("authentication required for file responses")
	errInvalidStatusCode       = errors.New("invalid plugin status code")
	errRangeNotSatisfiable     = errors.New("requested range is past the end of the file")
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

// Plugin metadata headers (X-Plugin, X-Plugin-Version and the like) pass as
// well. The namespace is deliberately this narrow: other X-* names are
// reverse-proxy controls (X-Accel-Redirect, X-Sendfile) or browser policy
// (X-Frame-Options), none of which a plugin may set on the panel origin.
const (
	customHeader       = "X-Plugin"
	customHeaderPrefix = customHeader + "-"
)

type Server struct {
	files   FileService
	nodes   repositories.NodeRepository
	checker PermissionChecker
	audit   audit.Logger
	policy  *hostlibrary.PathPolicy
}

// ServerOption tunes a Server.
type ServerOption func(*Server)

// WithPathPolicy applies the node path policy of gameap-nodefs to the files
// a plugin may serve; nil keeps the unrestricted policy.
func WithPathPolicy(policy *hostlibrary.PathPolicy) ServerOption {
	return func(s *Server) {
		if policy != nil {
			s.policy = policy
		}
	}
}

func NewServer(
	files FileService,
	nodes repositories.NodeRepository,
	checker PermissionChecker,
	auditLogger audit.Logger,
	opts ...ServerOption,
) *Server {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	server := &Server{
		files:   files,
		nodes:   nodes,
		checker: checker,
		audit:   auditLogger,
		policy:  hostlibrary.DefaultPathPolicy(),
	}

	for _, opt := range opts {
		opt(server)
	}

	return server
}

// ServeFileRef implements pkgplugin.FileRefServer. Errors are returned only
// before the first byte is written; a transfer that breaks midway is logged.
func (s *Server) ServeFileRef(w http.ResponseWriter, r *http.Request, req pkgplugin.FileRefRequest) error {
	ctx := r.Context()

	if err := validateRef(req.Ref); err != nil {
		return api.WrapHTTPError(err, http.StatusBadRequest)
	}

	// The status is the plugin's, and net/http panics on one it cannot
	// write: a plugin bug becomes a controlled answer before anything is
	// opened on its behalf.
	if err := validateStatusCode(req.StatusCode); err != nil {
		return api.WrapHTTPError(err, http.StatusBadGateway)
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

	if err := s.checkPathPolicy(ctx, node, req); err != nil {
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

	// A range only makes sense for an ordinary 200 answer; a plugin that chose
	// its own status has said something the transfer layer should not rewrite.
	served, ok, satisfiable := byteRange{}, false, false
	if effectiveStatus(req) == http.StatusOK {
		served, ok, satisfiable = parseByteRange(r.Header.Get("Range"), info.Size)
	}

	if ok && !satisfiable {
		w.Header().Set("Content-Range", "bytes */"+strconv.FormatUint(info.Size, 10))

		return api.WrapHTTPError(errRangeNotSatisfiable, http.StatusRequestedRangeNotSatisfiable)
	}

	stream, err := s.open(ctx, node, req, served, ok)
	if err != nil {
		return errors.WithMessage(err, "failed to open file stream")
	}
	defer func() {
		if closeErr := stream.Close(); closeErr != nil {
			slog.WarnContext(ctx, "failed to close plugin file stream", slog.String("error", closeErr.Error()))
		}
	}()

	if ok {
		s.stream(w, r, req, info, stream, &served)
	} else {
		s.stream(w, r, req, info, stream, nil)
	}

	return nil
}

// open picks the whole-file stream or the ranged one. They are different daemon
// calls: the plain stream may use the node's transfer task, which has no start
// offset, so a range has to go the chunked way.
func (s *Server) open(
	ctx context.Context,
	node *domain.Node,
	req pkgplugin.FileRefRequest,
	served byteRange,
	ranged bool,
) (io.ReadCloser, error) {
	if ranged {
		return s.files.DownloadStreamRange(ctx, node, req.Ref.Path, served.start, served.length)
	}

	return s.files.DownloadStream(ctx, node, req.Ref.Path)
}

// effectiveStatus is the status the response will carry: the plugin's, or 200
// when it named none.
func effectiveStatus(req pkgplugin.FileRefRequest) int {
	if req.StatusCode == 0 {
		return http.StatusOK
	}

	return req.StatusCode
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
	served *byteRange,
) {
	ctx := r.Context()

	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		slog.WarnContext(ctx, "failed to disable write deadline", slog.String("error", err.Error()))
	}

	applyHeaders(w.Header(), req, info)

	status := effectiveStatus(req)

	if served != nil {
		w.Header().Set("Content-Length", strconv.FormatUint(served.length, 10))
		w.Header().Set("Content-Range", served.contentRange(info.Size))

		status = http.StatusPartialContent
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

	// Bounded by the length just declared: a stream that grew between the stat
	// and the read must not run past it, or the client keeps a body longer than
	// the header promised. A stream that shrank instead ends the copy short,
	// which net/http reports as a failed response rather than a clean end —
	// which is the point, since a silently truncated archive is worse than a
	// download that visibly fails.
	expected := info.Size
	if served != nil {
		expected = served.length
	}

	declared := int64(min(expected, math.MaxInt64)) //nolint:gosec // G115: clamped to MaxInt64 just before

	written, err := io.Copy(w, io.LimitReader(stream, declared))
	if err != nil {
		slog.ErrorContext(ctx, "failed to write plugin file response",
			slog.Uint64("plugin_id", req.PluginID),
			slog.String("path", req.Ref.Path),
			slog.String("error", err.Error()))

		return
	}

	if written != declared {
		slog.ErrorContext(ctx, "plugin file response is shorter than the file it stat'ed",
			slog.Uint64("plugin_id", req.PluginID),
			slog.String("path", req.Ref.Path),
			slog.Int64("declared", declared),
			slog.Int64("written", written))
	}
}

// applyHeaders copies the plugin headers the allowlist admits and then sets
// the ones the panel owns: the file is always an attachment, and its length is
// the one the stat reported. The stat and the stream are separate daemon calls,
// so the two can disagree — the copy is bounded by this same length and a
// disagreement ends the response as an error, which is what tells the client
// its file is incomplete. Without a declared length the body is chunked and a
// truncated transfer ends looking exactly like a complete one.
func applyHeaders(header http.Header, req pkgplugin.FileRefRequest, info *daemon.FileDetails) {
	for name, value := range req.Headers {
		if pluginHeaderAllowed(name) {
			header.Set(name, value)
		}
	}

	filemanagerhttp.AttachmentContentHeaders(header, attachmentName(req.Ref), header.Get("Content-Type"))
	header.Set("Accept-Ranges", "bytes")
	header.Set("Content-Length", strconv.FormatUint(info.Size, 10))

	if header.Get("Cache-Control") == "" {
		header.Set("Cache-Control", "no-store")
	}
}

func pluginHeaderAllowed(name string) bool {
	canonical := http.CanonicalHeaderKey(name)
	if _, ok := allowedPluginHeaders[canonical]; ok {
		return true
	}

	return canonical == customHeader || strings.HasPrefix(canonical, customHeaderPrefix)
}

func attachmentName(ref *proto.FileRef) string {
	if ref.Filename != "" {
		return ref.Filename
	}

	return path.Base(strings.ReplaceAll(ref.Path, "\\", "/"))
}

// validateStatusCode accepts 0 (meaning 200) and final statuses 200-999.
// Informational 1xx codes are rejected even though WriteHeader can emit
// them: the response always carries a file body, so net/http would send the
// 1xx as an interim response and replace the plugin's status with an
// implicit 200.
func validateStatusCode(code int) error {
	if code == 0 || (code >= 200 && code <= 999) {
		return nil
	}

	return errors.WithMessage(errInvalidStatusCode, strconv.Itoa(code))
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

// authorize checks the plugin's "files_read" grant (which "files" includes)
// before any daemon round trip: a plugin without it must not learn whether a
// path exists. Denials are audited like every other authorization failure.
func (s *Server) authorize(ctx context.Context, req pkgplugin.FileRefRequest) error {
	allowed, err := s.checker.Has(ctx, req.PluginID, domain.PluginPermissionFilesRead)
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
		slog.String("permission", string(domain.PluginPermissionFilesRead)),
		slog.String("path", req.Ref.Path))

	audit.AccessDenied(ctx, s.audit, "plugin", pluginID, "plugin_permission_missing",
		slog.String("permission", string(domain.PluginPermissionFilesRead)),
		slog.String("action", "serve_file"))

	return api.WrapHTTPError(errFilesPermissionRequired, http.StatusForbidden)
}

// checkPathPolicy applies the node path policy to the file the plugin wants
// served; a refusal is audited like a refused host call.
func (s *Server) checkPathPolicy(ctx context.Context, node *domain.Node, req pkgplugin.FileRefRequest) error {
	scope, err := s.policy.ScopeFor(ctx, node, req.PluginID)
	if err != nil {
		return errors.WithMessage(err, "failed to resolve the node path policy")
	}

	denial := scope.Check(req.Ref.Path)
	if denial == nil {
		return nil
	}

	pluginID := strconv.FormatUint(req.PluginID, 10)

	slog.WarnContext(ctx, "plugin file response denied by the node path policy",
		slog.String("plugin_id", pluginID),
		slog.String("plugin", req.PluginName),
		slog.String("mode", string(denial.Mode)),
		slog.String("path", req.Ref.Path))

	audit.AccessDenied(ctx, s.audit, "plugin", pluginID, "plugin_path_policy",
		slog.String("path", req.Ref.Path),
		slog.String("mode", string(denial.Mode)),
		slog.String("action", "serve_file"))

	return api.WrapHTTPError(denial, http.StatusForbidden)
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
