package putnode

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"time"

	"github.com/gameap/gameap/internal/api/base"
	"github.com/gameap/gameap/internal/domain"
	"github.com/gameap/gameap/internal/files"
	"github.com/gameap/gameap/internal/filters"
	"github.com/gameap/gameap/internal/repositories"
	"github.com/gameap/gameap/pkg/api"
	"github.com/pkg/errors"
	"github.com/rs/xid"
)

const certificatesPath = "certs"

var (
	ErrFailedToSaveCertificate = errors.New("failed to save certificate")
	ErrFailedToUpdateNode      = errors.New("failed to update node")
	ErrNodeNotFound            = errors.New("node not found")
)

type Handler struct {
	repo        repositories.NodeRepository
	fileManager files.FileManager
	responder   base.Responder
}

func NewHandler(
	repo repositories.NodeRepository,
	fileManager files.FileManager,
	responder base.Responder,
) *Handler {
	return &Handler{
		repo:        repo,
		fileManager: fileManager,
		responder:   responder,
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	input := api.NewInputReader(r)

	nodeID, err := input.ReadUint("id")
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid node id"),
			http.StatusBadRequest,
		))

		return
	}

	node, err := h.findNode(ctx, nodeID)
	if err != nil {
		h.responder.WriteError(ctx, rw, err)

		return
	}

	var updateInput updateNodeInput
	err = json.NewDecoder(r.Body).Decode(&updateInput)
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "invalid request"),
			http.StatusBadRequest,
		))

		return
	}

	err = updateInput.Validate()
	if err != nil {
		h.responder.WriteError(ctx, rw, api.WrapHTTPError(
			errors.WithMessage(err, "validation failed"),
			http.StatusUnprocessableEntity,
		))

		return
	}

	updatedNode, err := h.updateNode(ctx, node, &updateInput)
	if err != nil {
		h.responder.WriteError(ctx, rw, errors.WithMessage(err, "failed to update node"))

		return
	}

	response := newNodeResponse(updatedNode)
	h.responder.Write(ctx, rw, response)
}

func (h *Handler) findNode(ctx context.Context, nodeID uint) (*domain.Node, error) {
	nodes, err := h.repo.Find(ctx, &filters.FindNode{
		IDs: []uint{nodeID},
	}, nil, &filters.Pagination{
		Limit:  1,
		Offset: 0,
	})
	if err != nil {
		return nil, errors.WithMessage(err, "failed to find node")
	}

	if len(nodes) == 0 {
		return nil, api.WrapHTTPError(ErrNodeNotFound, http.StatusNotFound)
	}

	return &nodes[0], nil
}

func (h *Handler) updateNode(ctx context.Context, node *domain.Node, input *updateNodeInput) (*domain.Node, error) {
	oldCertPath := node.GdaemonServerCert

	if input.GdaemonServerCert != nil && *input.GdaemonServerCert != "" {
		certXID := xid.New().String()
		certPath := filepath.Join(certificatesPath, certXID+".crt")

		if err := h.fileManager.Write(ctx, certPath, []byte(*input.GdaemonServerCert)); err != nil {
			return nil, errors.WithMessage(ErrFailedToSaveCertificate, err.Error())
		}

		node.GdaemonServerCert = certPath
	}

	input.ApplyToNode(node)

	now := time.Now()
	node.UpdatedAt = &now

	if err := h.repo.Save(ctx, node); err != nil {
		if input.GdaemonServerCert != nil && *input.GdaemonServerCert != "" {
			_ = h.fileManager.Delete(ctx, node.GdaemonServerCert)
		}

		return nil, errors.WithMessage(ErrFailedToUpdateNode, err.Error())
	}

	if input.GdaemonServerCert != nil && *input.GdaemonServerCert != "" && oldCertPath != "" {
		_ = h.fileManager.Delete(ctx, oldCertPath)
	}

	return node, nil
}
