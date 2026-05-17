package gettoken

import (
	"net/http"
	"strings"
	"time"

	"github.com/gameap/gameap/internal/api/base"
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
}

func NewHandler(
	nodeRepo repositories.NodeRepository,
	connChecker DaemonConnectionChecker,
	responder base.Responder,
) *Handler {
	return &Handler{
		nodeRepo:    nodeRepo,
		connChecker: connChecker,
		responder:   responder,
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

	// gdaemon_api_key is stored as a SHA-256 hash; look the node up by the hash
	// of the presented plaintext so the database never holds a usable key.
	hashedKey := pkgstrings.SHA256(apiKey)
	nodes, err := h.nodeRepo.Find(ctx, filters.FindNodeByGDaemonAPIKey(hashedKey), nil, &filters.Pagination{
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
	// yielding a usable daemon credential. The dedicated atomic update avoids
	// the Find->Save full-row race that let concurrent token rotations clobber
	// each other / unrelated columns.
	now := time.Now()

	err = h.nodeRepo.UpdateGDaemonAPIToken(ctx, node.ID, pkgstrings.SHA256(token), now)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to update node"),
			http.StatusInternalServerError,
		))

		return
	}

	response := newTokenResponse(token, now.Unix())

	h.responder.Write(ctx, rw, response)
}
