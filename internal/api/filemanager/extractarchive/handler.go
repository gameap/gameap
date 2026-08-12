package extractarchive

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"

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

var conflictPolicies = map[string]proto.ArchiveConflictPolicy{
	"":          proto.ArchiveConflictPolicy_ARCHIVE_CONFLICT_POLICY_ERROR,
	"error":     proto.ArchiveConflictPolicy_ARCHIVE_CONFLICT_POLICY_ERROR,
	"skip":      proto.ArchiveConflictPolicy_ARCHIVE_CONFLICT_POLICY_SKIP,
	"overwrite": proto.ArchiveConflictPolicy_ARCHIVE_CONFLICT_POLICY_OVERWRITE,
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

	var req extractArchiveRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request body"),
			http.StatusBadRequest,
		))

		return
	}

	format, policy, err := h.validateRequest(&req)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(err, http.StatusBadRequest))

		return
	}

	node, err := h.getNode(ctx, server.DSID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	params := buildParams(node, server, &req, format, policy, session.User.ID)

	operationID, err := h.daemonArchive.StartExtract(ctx, node, params)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	audit.SensitiveOp(ctx, h.audit, audit.EventFileArchiveExtract, audit.CategoryFileOp,
		"server", strconv.FormatUint(uint64(serverID), 10), "archive.extract",
		slog.String("operation_id", operationID),
		slog.String("conflict_policy", req.ConflictPolicy))

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusAccepted)
	h.responder.Write(ctx, rw, extractArchiveResponse{OperationID: operationID})
}

func buildParams(
	node *domain.Node,
	server *domain.Server,
	req *extractArchiveRequest,
	format proto.ArchiveFormat,
	policy proto.ArchiveConflictPolicy,
	userID uint,
) daemon.ExtractArchiveParams {
	createDestination := true
	if req.CreateDestination != nil {
		createDestination = *req.CreateDestination
	}

	root := filepath.Join(node.WorkPath, server.Dir)

	return daemon.ExtractArchiveParams{
		ArchivePath:         filepath.Join(root, req.Path),
		Destination:         filepath.Join(root, req.Destination),
		Format:              format,
		ConflictPolicy:      policy,
		CreateDestination:   createDestination,
		PreservePermissions: true,
		Owner:               daemon.OwnerFromServer(server),
		Options: daemon.ArchiveStartOptions{
			ServerID:  server.ID,
			Initiator: "user:" + strconv.FormatUint(uint64(userID), 10),
		},
	}
}

func (h *Handler) validateRequest(
	req *extractArchiveRequest,
) (proto.ArchiveFormat, proto.ArchiveConflictPolicy, error) {
	if req.Disk != "server" {
		return 0, 0, errors.Errorf("unsupported disk: %s, only 'server' disk is supported", req.Disk)
	}

	if err := filemanagerpath.ValidatePath(req.Path); err != nil {
		return 0, 0, err
	}
	if filemanagerpath.IsRoot(req.Path) {
		return 0, 0, filemanagerpath.ErrPathIsRoot
	}

	if err := filemanagerpath.ValidatePath(req.Destination); err != nil {
		return 0, 0, err
	}

	format := proto.ArchiveFormat_ARCHIVE_FORMAT_UNSPECIFIED
	if req.Format != "" {
		resolved, ok := daemon.ArchiveFormatFromAPIName(req.Format)
		if !ok {
			return 0, 0, errors.Errorf("unknown archive format: %s", req.Format)
		}
		format = resolved
	}

	policy, ok := conflictPolicies[req.ConflictPolicy]
	if !ok {
		return 0, 0, errors.Errorf("unknown conflict policy: %s", req.ConflictPolicy)
	}

	return format, policy, nil
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
