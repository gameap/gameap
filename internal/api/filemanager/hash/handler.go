package hash

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/api/filemanager/filemanagerpath"
	serversbase "github.com/gameap/gameap/internal/api/servers/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/gameap/gameap/pkg/auth"
	"github.com/gameap/gameap/pkg/proto"
	"github.com/pkg/errors"
)

const (
	maxHashPaths = 100
	// hashTimeout bounds the synchronous wait: hashing large files takes far
	// longer than a regular file-manager request.
	hashTimeout      = 5 * time.Minute
	defaultAlgorithm = "sha256"
)

var hashAlgorithms = map[string]proto.HashAlgorithm{
	"md5":    proto.HashAlgorithm_HASH_ALGORITHM_MD5,
	"sha1":   proto.HashAlgorithm_HASH_ALGORITHM_SHA1,
	"sha256": proto.HashAlgorithm_HASH_ALGORITHM_SHA256,
	"sha512": proto.HashAlgorithm_HASH_ALGORITHM_SHA512,
	"crc32":  proto.HashAlgorithm_HASH_ALGORITHM_CRC32,
	"crc64":  proto.HashAlgorithm_HASH_ALGORITHM_CRC64,
}

type Handler struct {
	serverFinder   *serversbase.ServerFinder
	abilityChecker *serversbase.AbilityChecker
	nodeRepo       repositories.NodeRepository
	daemonFiles    fileHasher
	responder      base.Responder
}

func NewHandler(
	serverRepo repositories.ServerRepository,
	nodeRepo repositories.NodeRepository,
	rbac base.RBAC,
	daemonFiles fileHasher,
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

	var req hashRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request body"),
			http.StatusBadRequest,
		))

		return
	}

	algorithm, err := h.validateRequest(&req)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(err, http.StatusBadRequest))

		return
	}

	node, err := h.getNode(ctx, server.DSID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	// Hashing may legitimately outlive the server's write timeout; lift it
	// for this response only (downloadarchive precedent).
	rc := http.NewResponseController(rw)
	if deadlineErr := rc.SetWriteDeadline(time.Time{}); deadlineErr != nil {
		slog.WarnContext(ctx, "failed to disable write deadline", slog.String("error", deadlineErr.Error()))
	}

	hashCtx, cancel := context.WithTimeout(ctx, hashTimeout)
	defer cancel()

	fullPaths := make([]string, 0, len(req.Paths))
	requestPathByRel := make(map[string]string, len(req.Paths))
	for _, p := range req.Paths {
		fullPaths = append(fullPaths, filepath.Join(node.WorkPath, server.Dir, p))
		requestPathByRel[daemonRelPath(server.Dir, p)] = p
	}

	result, err := h.daemonFiles.Hash(hashCtx, node, fullPaths, algorithm)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	h.responder.Write(ctx, rw, newResponse(req.Algorithm, result, requestPathByRel))
}

func (h *Handler) validateRequest(req *hashRequest) (proto.HashAlgorithm, error) {
	if req.Disk != "server" {
		return 0, errors.Errorf("unsupported disk: %s, only 'server' disk is supported", req.Disk)
	}

	if len(req.Paths) == 0 {
		return 0, errors.New("paths array is empty")
	}

	if len(req.Paths) > maxHashPaths {
		return 0, errors.Errorf("too many paths: %d, at most %d are allowed", len(req.Paths), maxHashPaths)
	}

	for _, p := range req.Paths {
		if err := filemanagerpath.ValidatePath(p); err != nil {
			return 0, err
		}
		if filemanagerpath.IsRoot(p) {
			return 0, filemanagerpath.ErrPathIsRoot
		}
	}

	if req.Algorithm == "" {
		req.Algorithm = defaultAlgorithm
	}

	algorithm, ok := hashAlgorithms[req.Algorithm]
	if !ok {
		return 0, errors.Errorf("unknown hash algorithm: %s", req.Algorithm)
	}

	return algorithm, nil
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

// daemonRelPath mirrors how the daemon sees a request path: joined under the
// server directory with forward slashes. Result entries are correlated back
// to the request through it instead of relying on response ordering.
func daemonRelPath(serverDir, requestPath string) string {
	rel := path.Join(
		strings.Trim(strings.ReplaceAll(serverDir, "\\", "/"), "/"),
		strings.Trim(strings.ReplaceAll(requestPath, "\\", "/"), "/"),
	)

	return strings.TrimPrefix(rel, "/")
}

func newResponse(algorithm string, result *proto.HashResult, requestPathByRel map[string]string) hashResponse {
	items := make([]hashItem, 0, len(result.GetHashes()))
	for _, fileHash := range result.GetHashes() {
		itemPath := fileHash.GetPath()
		if requestPath, ok := requestPathByRel[strings.TrimPrefix(itemPath, "/")]; ok {
			itemPath = requestPath
		}

		items = append(items, hashItem{
			Path:  itemPath,
			Hash:  fileHash.GetHash(),
			Size:  fileHash.GetSize(),
			Error: fileHash.GetError(),
		})
	}

	return hashResponse{
		Algorithm: algorithm,
		Items:     items,
	}
}
