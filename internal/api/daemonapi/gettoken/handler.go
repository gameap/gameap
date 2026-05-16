package gettoken

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/audit"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	pkgstrings "github.com/gameap/gameap/pkg/strings"
	"github.com/pkg/errors"
)

const tokenLength = 64

type Handler struct {
	nodeRepo    repositories.NodeRepository
	connChecker DaemonConnectionChecker
	responder   base.Responder
	audit       audit.Logger
}

func NewHandler(
	nodeRepo repositories.NodeRepository,
	connChecker DaemonConnectionChecker,
	responder base.Responder,
	auditLogger audit.Logger,
) *Handler {
	if auditLogger == nil {
		auditLogger = audit.NopLogger{}
	}

	return &Handler{
		nodeRepo:    nodeRepo,
		connChecker: connChecker,
		responder:   responder,
		audit:       auditLogger,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("invalid api key"),
			http.StatusUnauthorized,
		))

		return
	}

	apiKey := strings.TrimPrefix(strings.TrimSpace(authHeader), "Bearer ")
	if apiKey == "" {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("invalid api key"),
			http.StatusUnauthorized,
		))

		return
	}

	nodes, err := h.nodeRepo.Find(ctx, filters.FindNodeByGDaemonAPIKey(apiKey), nil, &filters.Pagination{
		Limit: 1,
	})
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to find node by api key"),
			http.StatusInternalServerError,
		))

		return
	}

	if len(nodes) == 0 {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("invalid api key"),
			http.StatusUnauthorized,
		))

		return
	}

	node := &nodes[0]

	if h.connChecker.IsConnectedAnywhere(uint64(node.ID)) {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.New("daemon is connected via gRPC bidi stream, HTTP API is disabled for this node"),
			http.StatusConflict,
		))

		return
	}

	token, err := pkgstrings.CryptoRandomString(tokenLength)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to generate token"),
			http.StatusInternalServerError,
		))

		return
	}

	// Persist only the SHA-256 hash; the plaintext is returned to the daemon once
	// in the response below and must never be retrievable from the database.
	// Mirrors the Personal Access Token model in
	// internal/api/tokens/posttoken/handler.go and prevents a DB read from
	// yielding a usable daemon credential.
	node.GdaemonAPIToken = new(pkgstrings.SHA256(token))
	now := time.Now()
	node.UpdatedAt = &now

	err = h.nodeRepo.Save(ctx, node)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to update node"),
			http.StatusInternalServerError,
		))

		return
	}

	audit.SensitiveOp(ctx, h.audit, audit.EventDaemonTokenIssue, audit.CategoryTokenOp,
		"node", strconv.FormatUint(uint64(node.ID), 10), "issue")

	response := newTokenResponse(token, now.Unix())

	h.responder.Write(ctx, rw, response)
}
