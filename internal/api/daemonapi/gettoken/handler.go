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
	nodeRepo  repositories.NodeRepository
	responder base.Responder
}

func NewHandler(
	nodeRepo repositories.NodeRepository,
	responder base.Responder,
) *Handler {
	return &Handler{
		nodeRepo:  nodeRepo,
		responder: responder,
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

	token, err := pkgstrings.CryptoRandomString(tokenLength)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "failed to generate token"),
			http.StatusInternalServerError,
		))

		return
	}

	node.GdaemonAPIToken = &token
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

	response := newTokenResponse(token, now.Unix())

	h.responder.Write(ctx, rw, response)
}
